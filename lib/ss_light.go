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
	"time"

	ipfslite "github.com/StreamSpace/ss-light-client"
	"github.com/StreamSpace/ss-light-client/scp"
	"github.com/StreamSpace/ss-light-client/scp/engine"
	externalip "github.com/glendc/go-external-ip"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
	logger "github.com/ipfs/go-log/v2"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/multiformats/go-multiaddr"
)

var log = logger.Logger("ss_light")

// Constants
const (
	fpSeparator   string = string(os.PathSeparator)
	cmdSeparator  string = "%$#"
	apiAddr       string = "http://bootstrap.streamspace.me"
	fetchPath     string = "v1/fetch"
	completePath  string = "v1/complete"
	peerThreshold int    = 5

	success        = 200
	internalError  = 500
	timeoutError   = 504
	serviceError   = 503
	destinationErr = 404
)

// API objects
type cookie struct {
	Id            string
	Leaders       []peer.AddrInfo
	DownloadIndex string
	Filename      string
	Hash          string
	Link          string
}

type StatOut struct {
	ConnectedPeers []string            `json:"connected_peers"`
	Ledgers        []*engine.SSReceipt `json:"ledger"`
	DownloadTime   int                 `json:"download_time"`
}

type ProgressOut struct {
	Percentage int    `json:"percentage"`
	Downloaded string `json:"downloaded"`
	TotalSize  string `json:"total_size"`
}

type info struct {
	Cookie   cookie
	SwarmKey []byte
	Rate     string
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
		"public_key": base64.StdEncoding.EncodeToString(pubKB),
		"src_ip":     getExternalIp(),
	}
	fetchUrl := fmt.Sprintf("%s/%s?link=%s", apiAddr, fetchPath, sharable)
	buf, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(fetchUrl, "application/json", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(string(respBuf))
	}
	respData := &info{}
	err = json.Unmarshal(respBuf, respData)
	if err != nil {
		log.Errorf("Failed unmarshaling result Err:%s Resp:%s", err.Error(), string(respBuf))
		return nil, err
	}
	return respData, nil
}

func updateInfo(i *info, timeConsumed int64) error {
	completeUrl := fmt.Sprintf("%s/%s?cookie=%s&time=%d",
		apiAddr, completePath, i.Cookie.Id, timeConsumed)
	resp, err := http.Post(completeUrl, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New(string(respBuf))
	}
	return nil
}

type LightClient struct {
	destination string
	repoRoot    string
	jsonOut     bool
	timeout     time.Duration

	privKey crypto.PrivKey
	pubKey  crypto.PubKey
	ds      datastore.Batching
}

func NewLightClient(
	destination string,
	timeout string,
	jsonOut bool,
) (*LightClient, error) {

	priv, pubk, err := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	if err != nil {
		log.Errorf("Failed generating key pair Err:%s", err.Error())
		return nil, err
	}

	ds := syncds.MutexWrap(datastore.NewMapDatastore())

	to, err := time.ParseDuration(timeout)
	if err != nil {
		log.Warn("Invalid timeout duration specified. Using default 15m")
		to = time.Minute * 45
	}

	return &LightClient{
		destination: destination,
		jsonOut:     jsonOut,
		timeout:     to,
		privKey:     priv,
		pubKey:      pubk,
		ds:          ds,
	}, nil
}

type ProgressUpdater interface {
	UpdateProgress(ProgressOut)
}

func (l *LightClient) Start(
	sharable string,
	onlyInfo bool,
	stat bool,
	progUpd ProgressUpdater,
) *Out {
	metadata, err := getInfo(sharable, l.pubKey)
	if err != nil {
		log.Errorf("Failed getting metadata Err: %s", err.Error())
		return NewOut(serviceError, "Failed getting metadata", err.Error(), nil)
	}
	// STEP : Got metadata
	showStep(success, "Got metadata", l.jsonOut)

	log.Infof("Got metadata info %+v", metadata)
	if onlyInfo {
		return NewOut(success, MetaInfo, "", metadata)
	}
	if l.destination == "." {
		l.destination = combineArgs(fpSeparator, l.destination, metadata.Cookie.Filename)
	}
	dst, err := os.Create(l.destination)
	if err != nil {
		log.Errorf("Failed creating dest file Err: %s", err.Error())
		return NewOut(destinationErr, "Failed creating destination file", err.Error(), nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	psk, err := pnet.DecodeV1PSK(bytes.NewReader(metadata.SwarmKey))
	if err != nil {
		log.Errorf("Failed decoding swarm key Err: %s", err.Error())
		return NewOut(internalError, "Failed decoding swarm key provided", err.Error(), nil)
	}
	listenIP4, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/45000")
	listenIP6, _ := multiaddr.NewMultiaddr("/ip6/::/tcp/45000")
	h, dht, err := ipfslite.SetupLibp2p(
		ctx,
		l.privKey,
		psk,
		[]multiaddr.Multiaddr{listenIP4, listenIP6},
		l.ds,
		ipfslite.Libp2pOptionsExtra...,
	)
	if err != nil {
		log.Errorf("Failed setting up libp2p node Err: %s", err.Error())
		return NewOut(internalError, "Failed setting up p2p peer", err.Error(), nil)
	}
	cfg := &ipfslite.Config{
		Mtdt: map[string]interface{}{
			"download_index": metadata.Cookie.DownloadIndex,
		},
		Rate: metadata.Rate,
	}
	lite, err := ipfslite.New(ctx, l.ds, h, dht, cfg)
	if err != nil {
		log.Errorf("Failed setting up p2p xfer node Err: %s", err.Error())
		return NewOut(internalError, "Failed setting up light client", err.Error(), nil)
	}
	lite.Scp.AddHook(scp.PeerConnected, func() {
		lite.Dht.Bootstrap(ctx)
	})
	// STEP : Download agent created
	showStep(success, "Download agent initialized", l.jsonOut)

	count := lite.Bootstrap(metadata.Cookie.Leaders)
	// STEP : Bootstrap done
	showStep(success, "Bootstrapped agent", l.jsonOut)

	if count < peerThreshold {
		go func() {
			start := time.Now()
			for count < peerThreshold {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second * 30):
					if time.Since(start) > time.Minute*15 {
						log.Warn("Tried getting more peers for 15mins")
						showStep(timeoutError, "Download timed out", l.jsonOut)
						return
					}
					// Try to re-bootstrap if client was unable to bootstrap previously
					oldCount := count
					if count < len(metadata.Cookie.Leaders) {
						count = lite.Bootstrap(metadata.Cookie.Leaders)
						// STEP : Re-Bootstrap done
						if count > oldCount {
							showStep(success, "Found more peers to connect", l.jsonOut)
						}
					}
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
				return NewOut(internalError, "Stopped while waiting for peers", "context cancelled", nil)
			case <-time.After(time.Second):
				break
			}
			if count > 0 {
				break
			}
		}
	}
	log.Infof("Connected to %d peers. Starting download", count)

	c, err := cid.Decode(metadata.Cookie.Hash)
	if err != nil {
		log.Errorf("Failed decoding file hash Err: %s", err.Error())
		return NewOut(internalError, "Failed decoding filehash provided", err.Error(), nil)
	}
	// STEP : Starting Download
	showStep(success, "Starting download", l.jsonOut)

	startTime := time.Now().Unix()
	rsc, err := lite.GetFile(ctx, c)
	if err != nil {
		return NewOut(500, "Failed getting file", err.Error(), nil)
	}
	defer rsc.Close()

	if progUpd != nil {
		go func() {
			for {
				st, err := dst.Stat()
				if err == nil {
					prog := float64(st.Size()) / float64(rsc.Size()) * 100
					log.Infof("Updating progress %d", int(prog))
					progOut := ProgressOut{
						Percentage: int(prog),
						Downloaded: fmt.Sprintf("%.2fMB", float32(st.Size())/(1024*1024)),
						TotalSize:  fmt.Sprintf("%.2fMB", float32(rsc.Size())/(1024*1024)),
					}
					progUpd.UpdateProgress(progOut)
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
		if err == context.DeadlineExceeded {
			return NewOut(timeoutError, "Unable to fetch data", err.Error(), nil)
		}
		return NewOut(internalError, "Failed writing to destination", err.Error(), nil)
	}
	downloadTime := time.Now().Unix() - startTime

	// STEP : Waiting for micropayments clean up
	showStep(success, "Finishing download", l.jsonOut)
	// Wait 5 secs for SCP to send all MPs. This can be optimized
	<-time.After(time.Second * 5)

	err = updateInfo(metadata, downloadTime)
	if err != nil {
		log.Warn("Failed updating metadata after download Err: %s", err.Error())
	}
	if !stat {
		return NewOut(200, DownloadSuccess, "", nil)
	}
	connectedPeers := []string{}
	for _, pID := range lite.Host.Network().Peers() {
		connectedPeers = append(connectedPeers, pID.String())
	}
	ledgers, _ := lite.Scp.GetMicroPayments()
	out := StatOut{
		ConnectedPeers: connectedPeers,
		Ledgers:        ledgers,
		DownloadTime:   int(downloadTime),
	}
	return NewOut(success, "Stats", "", out)
}

func showStep(status int, message string, jsonOut bool) {
	out := NewOut(success, message, "", nil)
	OutMessage(out, jsonOut)
}
