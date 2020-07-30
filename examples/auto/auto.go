package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/StreamSpace/ss-light-client/scp/engine"
	"github.com/olivere/elastic"
	"github.com/teris-io/shortid"
)

const (
	elasticIndexName = "user_documents"
	elasticTypeName  = "user_document"
)

type Document struct {
	ID        string                 `json:"id"`
	CreatedAt time.Time              `json:"created_at"`
	Content   interface{}            `json:"content"`
	Object    map[string]interface{} `json:"object"`
}

type StatOut struct {
	ConnectedPeers []string
	Ledgers        []*engine.SSReceipt
	Duration       time.Duration
}

func main() {
	jsonfile, err := os.Open("/home/env.json")
	if err != nil {
		log.Fatalf("Unable to open json file %s", err.Error())
	}
	url, err := ioutil.ReadAll(jsonfile)
	if err != nil {
		log.Fatalf("Unable to read data from json file %s", err.Error())
	}
	var str map[string]string
	err = json.Unmarshal(url, &str)
	if err != nil {
		log.Fatalf("Failed to Unmarshal url %s", err.Error())
	}
	res, err := http.Get(str["customer_files"])
	if err != nil {
		log.Fatal("Customer api failed ", err.Error())
	}
	data, _ := ioutil.ReadAll(res.Body)
	res.Body.Close()

	sharedObjects := []map[string]interface{}{}

	err = json.Unmarshal(data, &sharedObjects)
	if err != nil {
		log.Fatal(err)
	}
	elasticClient, err := elastic.NewClient(
		elastic.SetURL(str["elastic_url"]),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(false),
		elastic.SetBasicAuth(str["elastic_user"], str["elastic_password"]),
	)
	if err != nil {
		log.Fatalf("Unable to connect to elasticdb : %s. Exiting...", err.Error())
	}
	bulk := elasticClient.Bulk().Index(elasticIndexName).Type(elasticTypeName)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for {
		rand.Seed(time.Now().UnixNano())
		n := rand.Intn(30)
		select {
		case <-c:
			return
		case <-time.After(time.Duration(30+n) * time.Second):
			randomIndex := rand.Intn(len(sharedObjects))
			log.Println("Downloading ", randomIndex, " after ", 30+n, " second interval")
			start := time.Now()
			cmd := exec.Command("lite", "-sharable", fmt.Sprintf("%s", sharedObjects[randomIndex]["link"]), "-stat", "-logToStderr")
			var out bytes.Buffer
			cmd.Stdout = &out
			err = cmd.Run()
			if err != nil {
				log.Fatal("command failed ", err.Error())
			}
			output := &StatOut{}
			err = json.Unmarshal(out.Bytes(), output)
			output.Duration = time.Now().Sub(start)
			doc := Document{
				ID:        shortid.MustGenerate(),
				CreatedAt: time.Now().UTC(),
				Content:   output,
				Object:    sharedObjects[randomIndex],
			}
			bulk.Add(elastic.NewBulkIndexRequest().Id(doc.ID).Doc(doc))
			if _, err := bulk.Do(context.Background()); err != nil {
				log.Fatal("Failed to create document ", err)
			}
			fmt.Println(output)
			err = os.Remove(fmt.Sprintf("%s", sharedObjects[randomIndex]["name"]))
			if err != nil {
				log.Fatalf("unable to remove file")
			}
		}
	}
}
