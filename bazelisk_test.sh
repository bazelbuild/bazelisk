#!/bin/bash

# Copyright 2018 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail
# --- begin runfiles.bash initialization ---
if [[ ! -d "${RUNFILES_DIR:-/dev/null}" && ! -f "${RUNFILES_MANIFEST_FILE:-/dev/null}" ]]; then
    if [[ -f "$0.runfiles_manifest" ]]; then
      export RUNFILES_MANIFEST_FILE="$0.runfiles_manifest"
    elif [[ -f "$0.runfiles/MANIFEST" ]]; then
      export RUNFILES_MANIFEST_FILE="$0.runfiles/MANIFEST"
    elif [[ -f "$0.runfiles/bazel_tools/tools/bash/runfiles/runfiles.bash" ]]; then
      export RUNFILES_DIR="$0.runfiles"
    fi
fi
if [[ -f "${RUNFILES_DIR:-/dev/null}/bazel_tools/tools/bash/runfiles/runfiles.bash" ]]; then
  source "${RUNFILES_DIR}/bazel_tools/tools/bash/runfiles/runfiles.bash"
elif [[ -f "${RUNFILES_MANIFEST_FILE:-/dev/null}" ]]; then
  source "$(grep -m1 "^bazel_tools/tools/bash/runfiles/runfiles.bash " \
            "$RUNFILES_MANIFEST_FILE" | cut -d ' ' -f 2-)"
else
  echo >&2 "ERROR: cannot find @bazel_tools//tools/bash/runfiles:runfiles.bash"
  exit 1
fi
# --- end runfiles.bash initialization ---

function setup() {
  BAZELISK_HOME="$(mktemp -d $TEST_TMPDIR/home.XXXXXX)"

  cp "$(rlocation __main__/releases_for_tests.json)" "${BAZELISK_HOME}/releases.json"
  touch "${BAZELISK_HOME}/releases.json"

  cd "$(mktemp -d $TEST_TMPDIR/workspace.XXXXXX)"
  touch WORKSPACE BUILD
}

function bazelisk() {
  if [[ -n $(rlocation __main__/bazelisk.py) ]]; then
    python "$(rlocation __main__/bazelisk.py)" "$@"
  elif [[ -n $(rlocation __main__/windows_amd64_stripped/bazelisk.exe) ]]; then
    "$(rlocation __main__/windows_amd64_stripped/bazelisk.exe)" "$@"
  elif [[ -n $(rlocation __main__/darwin_amd64_stripped/bazelisk) ]]; then
    "$(rlocation __main__/darwin_amd64_stripped/bazelisk)" "$@"
  elif [[ -n $(rlocation __main__/linux_amd64_stripped/bazelisk) ]]; then
    "$(rlocation __main__/linux_amd64_stripped/bazelisk)" "$@"
  else
    echo "Could not find the bazelisk executable, listing files:"
    find .
  fi
}

function test_bazel_version() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.21.0" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_environment() {
  setup

  USE_BAZEL_VERSION="0.20.0" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.20.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.20.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_file() {
  setup

  echo "0.19.0" > .bazelversion

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.19.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_latest_minus_3() {
  setup

  USE_BAZEL_VERSION="latest-3" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.1" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_last_green() {
  setup

  USE_BAZEL_VERSION="last_green" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  ! grep "Build label:" log || \
      (echo "FAIL: 'bazelisk version' of an unreleased binary must not print a build label."; exit 1)
}

function test_bazel_last_downstream_green() {
  setup

  USE_BAZEL_VERSION="last_downstream_green" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  ! grep "Build label:" log || \
      (echo "FAIL: 'bazelisk version' of an unreleased binary must not print a build label."; exit 1)
}

function test_bazel_last_rc() {
  setup

  USE_BAZEL_VERSION="last_rc" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label:" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
}

echo "# test_bazel_version"
test_bazel_version
echo

echo "# test_bazel_version_from_environment"
test_bazel_version_from_environment
echo

echo "# test_bazel_version_from_file"
test_bazel_version_from_file
echo

echo "# test_bazel_latest_minus_3"
test_bazel_latest_minus_3
echo

echo "# test_bazel_last_green"
test_bazel_last_green
echo

echo "# test_bazel_last_downstream_green"
test_bazel_last_downstream_green
echo

echo "# test_bazel_last_rc"
test_bazel_last_rc
echo