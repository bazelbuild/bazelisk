#!/bin/bash

set -euxo pipefail

GOOS=linux GOARCH=amd64 go build -o bin/bazelisk-linux-amd64
GOOS=darwin GOARCH=amd64 go build -o bin/bazelisk-darwin-amd64
GOOS=windows GOARCH=amd64 go build -o bin/bazelisk-windows-amd64.exe
