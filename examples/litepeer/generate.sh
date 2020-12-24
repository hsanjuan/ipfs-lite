#!/bin/bash

# Constants required for environments
EPOCH=2020-07-01T00:00:00Z
CYCLE=24h
API=http://bootstrap.swrmlabs.io
OUT=prod

if [[ "$1" = "qa" ]]; then
   EPOCH=2020-03-02T00:00:00Z
   CYCLE=15m
   API=http://bootstrap.streamspace.me
   OUT=qa
fi

# PKG path for overriding constants
SCP=github.com/StreamSpace/scp/config
LIB=github.com/StreamSpace/ss-light-client/lib

LDFLAGS="-w -s -X $SCP.Epoch=$EPOCH -X $SCP.CycleDuration=$CYCLE -X $LIB.ApiAddr=$API"

echo "Generating binaries with Config $EPOCH $CYCLE $API"
GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o $OUT/swrm-client-darwin-amd64 litepeer.go
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o $OUT/swrm-client-linux-amd64 litepeer.go
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o $OUT/swrm-client-windows-amd64.exe litepeer.go

# Generate packed binaries
upx --brute $OUT/swrm-client-*

if [[ "$2" = "no" ]]; then
   echo "Not notarising"
   exit
fi

# Sign with entitlements
codesign -s "Developer ID Application: StreamSpace, LLC" -f -v -o runtime --entitlements entitlements.xml $OUT/swrm-client-darwin-amd64

# Zip the binary
ditto -c -k $OUT/swrm-client-darwin-amd64 $OUT/swrm-client-darwin-amd64.zip

# Notarise
gon -log-level=info -log-json $OUT/config.json
