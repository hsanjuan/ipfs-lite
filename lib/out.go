package lib

import (
	"encoding/json"
	"fmt"
)

type Out struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Details string      `json:"details,omitempty"`
}

func NewOut(status int, message, err string, data interface{}) *Out {
	o := &Out{
		Status:  status,
		Message: message,
		Data:    data,
	}
	if err != "" {
		o.Details = err
	}
	if data != "" {
		o.Data = data
	}
	return o
}

var (
	GeneralErr      = "Something went wrong"
	DownloadSuccess = "Download complete"
	MetaInfo        = "Metadata"
)

func OutMessage(cliOut *Out, jFlag bool) {
	if jFlag {
		j, _ := json.MarshalIndent(cliOut, "", "\t")
		fmt.Println(string(j))
		return
	}
	fmt.Printf("%s ", cliOut.Message)
	if cliOut.Data != nil {
		fmt.Println(cliOut.Data)
		return
	}
	fmt.Println()
}
