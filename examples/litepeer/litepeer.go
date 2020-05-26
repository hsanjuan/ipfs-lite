package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	_ "net/http"
	"os"
	"time"

	ipfslite "github.com/StreamSpace/ss-light-client"
	"github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/multiformats/go-multiaddr"
)

// Constants
const (
	RepoBase     string = ".ss_light"
	FpSeparator  string = string(os.PathSeparator)
	CmdSeparator string = "#$%"
	ApiAddr      string = "http://bootstraps.stream.space/v3/execute"
)

// Command arguments
var (
	repo        = flag.String("repo", ".", "Path for storing intermediate data")
	destination = flag.String("dst", "~/Downloads/", "Path to store downloaded file")
	sharable    = flag.String("sharable", "", "Sharable string provided for file")
	timeout     = flag.String("timeout", "15m", "Timeout duration")
	onlyInfo    = flag.Bool("info", false, "Get only fetch info")
)

// API objects
type cookie struct {
	ID          string          `json:"id"`
	Count       int             `json:"count"`
	Leaders     []peer.AddrInfo `json:"leaders"`
	DownloadIdx string          `json:"downloadindex"`
	Filename    string          `json:"filename"`
	Filehash    string          `json:"filehash"`
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
	tmp := struct {
		Status  int             `json:"status"`
		Details string          `json:"details"`
		Data    json.RawMessage `json:"data"`
	}{}
	err := json.Unmarshal(b, &tmp)
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

func getInfo(sharable string, pubKey crypto.PubKey) (*info, error) {
	pubKB, _ := pubKey.Bytes()
	args := map[string]interface{}{
		"args": combineArgs(
			CmdSeparator,
			sharable,
			"--public-key "+base64.StdEncoding.EncodeToString(pubKB),
		),
	}
	_, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	// resp, err := http.Post(ApiAddr, "application/json", bytes.NewReader(buf))
	// if err != nil {
	// 	return nil, err
	// }
	// defer resp.Body.Close()
	f, err := os.Open("example2.json")
	if err != nil {
		return nil, err
	}
	respBuf, err := ioutil.ReadAll(f)
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

func returnError(err string, printUsage bool) {
	fmt.Println("ERR: " + err)
	if printUsage {
		fmt.Println(`
Usage:
	./ss-light <OPTIONS>

Options:
		`)
		flag.PrintDefaults()
	}
	os.Exit(1)
}

func main() {
	flag.Parse()

	if len(*sharable) == 0 {
		returnError("Sharable string not provided", true)
	}

	timeout, err := time.ParseDuration(*timeout)
	if err != nil {
		fmt.Println("Invalid timeout duration specified. Using default 15m")
		timeout = time.Minute * 15
	}

	priv, pubk, err := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	if err != nil {
		returnError(err.Error(), false)
	}

	metadata, err := getInfo(*sharable, pubk)
	if err != nil {
		returnError(err.Error(), false)
	}

	if *onlyInfo {
		fmt.Printf("%+v\n", metadata)
		os.Exit(0)
	}

	dst, err := os.Create(
		combineArgs(FpSeparator, *destination, metadata.Cookie.Filename))
	if err != nil {
		returnError(
			"Unable to create file in provided destination reason: "+err.Error(), true)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ds, err := ipfslite.BadgerDatastore(combineArgs(FpSeparator, *repo, RepoBase))
	if err != nil {
		returnError("Failed to initialize repository reason: "+err.Error(), true)
	}

	psk, err := pnet.DecodeV1PSK(bytes.NewReader(metadata.SwarmKey))
	if err != nil {
		returnError("Internal reason: "+err.Error(), false)
	}

	listen, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/4342")

	h, dht, err := ipfslite.SetupLibp2p(
		ctx,
		priv,
		psk,
		[]multiaddr.Multiaddr{listen},
		ds,
		ipfslite.Libp2pOptionsExtra...,
	)

	cfg := &ipfslite.Config{
		Root: combineArgs(FpSeparator, *repo, RepoBase),
		Mtdt: map[string]interface{}{
			"download_index": metadata.Cookie.DownloadIdx,
		},
	}

	if err != nil {
		returnError("Internal reason: "+err.Error(), false)
	}

	lite, err := ipfslite.New(ctx, ds, h, dht, cfg)
	if err != nil {
		returnError("Internal reason: "+err.Error(), false)
	}

	lite.Bootstrap(metadata.Cookie.Leaders)

	c, err := cid.Decode(metadata.Cookie.Filehash)
	if err != nil {
		returnError("Internal reason: "+err.Error(), false)
	}

	rsc, err := lite.GetFile(ctx, c)
	if err != nil {
		returnError("Internal reason: "+err.Error(), false)
	}
	defer rsc.Close()

	_, err = io.Copy(dst, rsc)
	if err != nil {
		returnError("Internal reason: "+err.Error(), false)
	}

	return
}
