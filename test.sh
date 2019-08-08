#!/bin/bash

set -euxo pipefail

rm -rf "$HOME/Library/Caches/bazelisk"
env -u USE_BAZEL_VERSION ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="latest" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="0.28.0" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="last_green" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="last_downstream_green" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="last_rc" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="bazelbuild/latest" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="bazelbuild/0.27.0" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="philwo/latest" ./bin/bazelisk-darwin-amd64 version
USE_BAZEL_VERSION="philwo/0.25.0" ./bin/bazelisk-darwin-amd64 version
