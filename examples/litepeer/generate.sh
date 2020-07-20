#!/bin/bash

GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o ss-light-darwin-amd64 litepeer.go
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o ss-light-linux-amd64 litepeer.go
GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o ss-light-windows-amd64.exe litepeer.go
upx --brute ss-light-*