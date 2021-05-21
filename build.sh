#!/bin/bash
#
# Copyright 2019 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

### Build release artifacts using Bazel.
rm -rf bazelisk bin
mkdir bin

go build
./bazelisk build --config=release \
    //:bazelisk-darwin-amd64 \
    //:bazelisk-darwin-arm64 \
    //:bazelisk-darwin-universal \
    //:bazelisk-linux-amd64 \
    //:bazelisk-linux-arm64 \
    //:bazelisk-windows-amd64
echo

cp bazel-out/*-opt-*/bin/bazelisk-darwin_amd64 bin/bazelisk-darwin-amd64
cp bazel-out/*-opt-*/bin/bazelisk-darwin_arm64 bin/bazelisk-darwin-arm64
cp bazel-out/*-opt/bin/bazelisk-darwin_universal bin/bazelisk-darwin
cp bazel-out/*-opt-*/bin/bazelisk-linux_amd64 bin/bazelisk-linux-amd64
cp bazel-out/*-opt-*/bin/bazelisk-linux_arm64 bin/bazelisk-linux-arm64
cp bazel-out/*-opt-*/bin/bazelisk-windows_amd64.exe bin/bazelisk-windows-amd64.exe
rm -f bazelisk

### Build release artifacts using `go build`.
# GOOS=linux GOARCH=amd64 go build -o bin/bazelisk-linux-amd64
# GOOS=linux GOARCH=arm64 go build -o bin/bazelisk-linux-arm64
# GOOS=darwin GOARCH=amd64 go build -o bin/bazelisk-darwin-amd64
# GOOS=darwin GOARCH=arm64 go build -o bin/bazelisk-darwin-arm64
# lipo -create -output bin/bazelisk-darwin bin/bazelisk-darwin-amd64 bin/bazelisk-darwin-arm64
# GOOS=windows GOARCH=amd64 go build -o bin/bazelisk-windows-amd64.exe

### Print some information about the generated binaries.
echo "== Bazelisk binaries are ready =="
ls -lh bin/*
file bin/*
echo

echo "== Bazelisk version output =="
echo "Before releasing, make sure that this is the correct version string:"
"bin/bazelisk-$(uname -s | tr [:upper:] [:lower:])-amd64" version | grep "Bazelisk version"
echo

# Non-googlers: you should run this script with NPM_REGISTRY=https://registry.npmjs.org
readonly REGISTRY=${NPM_REGISTRY:-https://wombat-dressing-room.appspot.com}
echo "== NPM releases =="
echo "After testing, publish to NPM via these commands:"
echo "$ npm login --registry $REGISTRY"
echo "$ ./bazelisk run --config=release //:npm_package.publish -- --access=public --tag latest --registry $REGISTRY"
