package lib

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	ipfslite "github.com/StreamSpace/ss-light-client"
	externalip "github.com/glendc/go-external-ip"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logger "github.com/ipfs/go-log/v2"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/multiformats/go-multiaddr"
)

var log = logger.Logger("ss_light")

// Constants
const (
	repoBase      string = ".ss_light"
	fpSeparator   string = string(os.PathSeparator)
	cmdSeparator  string = "%$#"
	apiAddr       string = "http://bootstrap1.stream.space:4343/v3/execute"
	peerThreshold int    = 5
)

// API objects
type cookie struct {
	ID          string          `json:"id"`
	Count       int             `json:"count"`
	Leaders     []peer.AddrInfo `json:"leaders"`
	DownloadIdx int             `json:"downloadindex"`
	Filename    string          `json:"filename"`
	Filehash    string          `json:"hash"`
}

type info struct {
	Cookie   cookie
	SwarmKey []byte
	Rate     string
}

type apiResp struct {
	Status  int    `json:"status"`
	Data    info   `json:"data"`
	Details string `json:"details"`
}

func (a *apiResp) UnmarshalJSON(b []byte) error {
	val := map[string]string{}
	tmp := struct {
		Status  int             `json:"status"`
		Details string          `json:"details"`
		Data    json.RawMessage `json:"data"`
	}{}
	log.Debugf("Raw response %s", string(b))
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}
	log.Debugf("API response %s", val["val"])
	err = json.Unmarshal([]byte(val["val"]), &tmp)
	if err != nil {
		return err
	}
	if tmp.Status != 200 {
		errStr := tmp.Details
		if len(errStr) == 0 {
			errStr = fmt.Sprintf("Invalid status from server: %s", tmp.Status)
		}
		return errors.New(errStr)
	}
	a.Status = tmp.Status
	return json.Unmarshal(tmp.Data, &a.Data)
}

func combineArgs(separator string, args ...string) (retPath string) {
	for idx, v := range args {
		if idx != 0 {
			retPath += separator
		}
		retPath += v
	}
	return
}

func getExternalIp() string {
	consensus := externalip.DefaultConsensus(nil, nil)
	ip, err := consensus.ExternalIP()
	if err != nil {
		return "0.0.0.0"
	}
	return ip.String()
}

func getInfo(sharable string, pubKey crypto.PubKey) (*info, error) {
	pubKB, _ := pubKey.Bytes()
	args := map[string]interface{}{
		"val": combineArgs(
			cmdSeparator,
			"streamspace",
			"customer",
			"fetch",
			sharable,
			"--public-key",
			base64.StdEncoding.EncodeToString(pubKB),
			"--source-ip",
			getExternalIp(),
			"-j",
		),
	}
	buf, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(apiAddr, "application/json", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	respData := &apiResp{}
	err = json.Unmarshal(respBuf, &respData)
	if err != nil {
		return nil, err
	}
	return &respData.Data, nil
}

func updateInfo(i *info) error {
	args := map[string]interface{}{
		"val": combineArgs(
			cmdSeparator,
			"streamspace",
			"customer",
			"complete",
			i.Cookie.ID,
			"-j",
		),
	}
	buf, err := json.Marshal(args)
	if err != nil {
		return err
	}
	resp, err := http.Post(apiAddr, "application/json", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	respData := &apiResp{}
	err = json.Unmarshal(respBuf, &respData)
	if err != nil && respData.Status != 200 {
		return err
	}
	return nil
}

type LightClient struct {
	destination string
	repoRoot    string
	timeout     time.Duration

	privKey crypto.PrivKey
	pubKey  crypto.PubKey
	ds      datastore.Batching
}

func NewLightClient(
	destination string,
	repoRoot string,
	timeout string,
) (*LightClient, error) {

	priv, pubk, err := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	if err != nil {
		log.Errorf("Failed generating key pair Err:%s", err.Error())
		return nil, err
	}

	ds := datastore.NewMapDatastore()

	to, err := time.ParseDuration(timeout)
	if err != nil {
		log.Warn("Invalid timeout duration specified. Using default 15m")
		to = time.Minute * 15
	}

	return &LightClient{
		destination: destination,
		repoRoot:    repoRoot,
		timeout:     to,
		privKey:     priv,
		pubKey:      pubk,
		ds:          ds,
	}, nil
}

type ProgressUpdater interface {
	UpdateProgress(int)
}

func (l *LightClient) Start(
	sharable string,
	onlyInfo bool,
	progUpd ProgressUpdater,
) (string, error) {
	metadata, err := getInfo(sharable, l.pubKey)
	if err != nil {
		log.Errorf("Failed getting metadata Err: %s", err.Error())
		return "Failed getting metadata", err
	}
	log.Infof("Got metadata info %+v", metadata)

	if onlyInfo {
		return fmt.Sprintf("%+v\n", metadata), nil
	}

	dst, err := os.Create(
		combineArgs(fpSeparator, l.destination, metadata.Cookie.Filename))
	if err != nil {
		log.Errorf("Failed creating dest file Err: %s", err.Error())
		return "Failed creating destination file", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	psk, err := pnet.DecodeV1PSK(bytes.NewReader(metadata.SwarmKey))
	if err != nil {
		log.Errorf("Failed decoding swarm key Err: %s", err.Error())
		return "Failed decoding swarm key provided", err
	}

	listen, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/45000")

	h, dht, err := ipfslite.SetupLibp2p(
		ctx,
		l.privKey,
		psk,
		[]multiaddr.Multiaddr{listen},
		l.ds,
		ipfslite.Libp2pOptionsExtra...,
	)
	if err != nil {
		log.Errorf("Failed setting up libp2p node Err: %s", err.Error())
		return "Failed setting up p2p peer", err
	}

	cfg := &ipfslite.Config{
		Root: combineArgs(fpSeparator, l.repoRoot, repoBase),
		Mtdt: map[string]interface{}{
			"download_index": strconv.Itoa(metadata.Cookie.DownloadIdx),
		},
	}
	lite, err := ipfslite.New(ctx, l.ds, h, dht, cfg)
	if err != nil {
		log.Errorf("Failed setting up p2p xfer node Err: %s", err.Error())
		return "Failed setting up light client", err
	}

	count := lite.Bootstrap(metadata.Cookie.Leaders)
	if count < peerThreshold {
		go func() {
			start := time.Now()
			for count < peerThreshold {
				select {
				case <-ctx.Done():
					return
				default:
					newMtdt, err := getInfo(sharable, l.pubKey)
					if err == nil && len(newMtdt.Cookie.Leaders) > 0 {
						log.Infof("Got %d new leaders", len(newMtdt.Cookie.Leaders))
						count += lite.Bootstrap(newMtdt.Cookie.Leaders)
					}
					if time.Since(start) > time.Second*15 {
						log.Warn("Tried getting more peers for 15secs")
						return
					}
					<-time.After(time.Second)
				}
			}
			log.Infof("Done lagged bootstrapping. New count %d", count)
		}()
	}
	if count == 0 {
		log.Warn("No nodes connected. Waiting to find more")
		for {
			select {
			case <-ctx.Done():
				log.Info("Client stopped while waiting for more peers")
				return "Stopped while waiting for peers", errors.New("context cancelled")
			case <-time.After(time.Second):
				break
			}
			if count > 0 {
				break
			}
		}
	}
	log.Infof("Connected to %d peers. Starting download", count)

	c, err := cid.Decode(metadata.Cookie.Filehash)
	if err != nil {
		log.Errorf("Failed decoding file hash Err: %s", err.Error())
		return "Failed decoding filehash provided", err
	}

	rsc, err := lite.GetFile(ctx, c)
	if err != nil {
		return "Failed getting file", err
	}
	defer rsc.Close()

	if progUpd != nil {
		go func() {
			for {
				st, err := dst.Stat()
				if err == nil {
					prog := float64(st.Size()) / float64(rsc.Size()) * 100
					log.Infof("Updating progress %d", int(prog))
					progUpd.UpdateProgress(int(prog))
					if prog == 100 {
						log.Infof("Progress complete")
						return
					}
				}
				select {
				case <-ctx.Done():
					log.Warn("Stopping progress updated on context cancel")
				case <-time.After(time.Millisecond * 500):
					break
				}
			}
		}()
	}

	_, err = io.Copy(dst, rsc)
	if err != nil {
		return "Failed writing to destination", err
	}

	// Wait 5 secs for SCP to send all MPs. This can be optimized
	<-time.After(time.Second * 5)

	err = updateInfo(metadata)
	if err != nil {
		log.Warn("Failed updating metadata after download Err: %s", err.Error())
	}
	return "Download complete", nil
}
