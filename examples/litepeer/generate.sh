#!/bin/bash

GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o swrm-client-darwin-amd64 litepeer.go
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o swrm-client-linux-amd64 litepeer.go
GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o swrm-client-windows-amd64.exe litepeer.go
upx --brute swrm-client-*
#gon -log-level=info -log-json ./config.json
