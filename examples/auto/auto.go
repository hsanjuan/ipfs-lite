package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/StreamSpace/ss-light-client/examples/auto/objects"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

func main() {
	res, err := http.Get( "http://bootstrap.swrmlabs.io/v1/customer_files" )
	if err != nil {
		log.Fatal( err )
	}
	data, _ := ioutil.ReadAll( res.Body )
	res.Body.Close()

	sharedObjects := []*objects.FileObj{}

	err = json.Unmarshal(data, &sharedObjects)
	//if err != nil {
	//	log.Fatal( err )
	//}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	for  {
		rand.Seed(time.Now().UnixNano())
		n := rand.Intn(30)
		select{
			case <-c:
				return
			case <-time.After(time.Duration(30+n)*time.Second):
				randomIndex := rand.Intn(len(sharedObjects))
				log.Println("Downloading ", randomIndex, " after ", 30+n, " second interval")
				cmd := exec.Command("lite", "-sharable", sharedObjects[randomIndex].GetLink())
				var out bytes.Buffer
				cmd.Stdout = &out
				err = cmd.Run()
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println(out.String())
		}
	}
}
