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

BAZELISK_VERSION=$1
shift 1

# TODO: only the Python version reads bazelbuild-releases.json since it uses
# GitHub by default, whereas the Go version GCS (without this json file)
function setup() {
  unset USE_BAZEL_VERSION
  BAZELISK_HOME="$(mktemp -d $TEST_TMPDIR/home.XXXXXX)"

  cp "$(rlocation __main__/releases_for_tests.json)" "${BAZELISK_HOME}/bazelbuild-releases.json"
  touch "${BAZELISK_HOME}/bazelbuild-releases.json"
  ln -s "${BAZELISK_HOME}/bazelbuild-releases.json" "${BAZELISK_HOME}/releases.json"

  cd "$(mktemp -d $TEST_TMPDIR/workspace.XXXXXX)"
  touch WORKSPACE BUILD
}

function bazelisk() {
  if [[ -n $(rlocation __main__/bazelisk.py) ]]; then
    if [[ $BAZELISK_VERSION == "PY3" ]]; then
      echo "Running Bazelisk with $(python3 -V)..."
      python3 "$(rlocation __main__/bazelisk.py)" "$@"
    else
      echo "Running Bazelisk with $(python -V)..."
      python "$(rlocation __main__/bazelisk.py)" "$@"
    fi
  elif [[ -n $(rlocation __main__/windows_amd64_debug/bazelisk.exe) ]]; then
    "$(rlocation __main__/windows_amd64_debug/bazelisk.exe)" "$@"
  elif [[ -n $(rlocation __main__/darwin_amd64_debug/bazelisk) ]]; then
    "$(rlocation __main__/darwin_amd64_debug/bazelisk)" "$@"
  elif [[ -n $(rlocation __main__/linux_amd64_debug/bazelisk) ]]; then
    "$(rlocation __main__/linux_amd64_debug/bazelisk)" "$@"
  elif [[ -n $(rlocation __main__/windows_amd64_stripped/bazelisk.exe) ]]; then
    "$(rlocation __main__/windows_amd64_stripped/bazelisk.exe)" "$@"
  elif [[ -n $(rlocation __main__/darwin_amd64_stripped/bazelisk) ]]; then
    "$(rlocation __main__/darwin_amd64_stripped/bazelisk)" "$@"
  elif [[ -n $(rlocation __main__/linux_amd64_stripped/bazelisk) ]]; then
    "$(rlocation __main__/linux_amd64_stripped/bazelisk)" "$@"
  elif [[ -n $(rlocation __main__/bazelisk_/bazelisk) ]]; then
    "$(rlocation __main__/bazelisk_/bazelisk)" "$@"
  elif [[ -n $(rlocation __main__/bazelisk_/bazelisk.exe) ]]; then
    "$(rlocation __main__/bazelisk_/bazelisk.exe)" "$@"
  else
    echo "Could not find the bazelisk executable, listing files:"
    find .
    exit 1
  fi
}

function test_bazel_version_py() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.21.0" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_go() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: " log || \
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

function test_bazel_version_prefer_environment_to_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=0.19.0" > .bazeliskrc

  USE_BAZEL_VERSION="0.20.0" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.20.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.20.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=0.19.0" > .bazeliskrc

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.19.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_prefer_bazeliskrc_to_bazelversion_file() {
  setup

  echo "USE_BAZEL_VERSION=0.20.0" > .bazeliskrc
  echo "0.19.0" > .bazelversion

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

function test_bazel_version_from_url() {
  setup

  echo "0.19.0" > .bazelversion

  BAZELISK_BASE_URL="https://github.com/bazelbuild/bazel/releases/download" \
      BAZELISK_HOME="$BAZELISK_HOME" \
          bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.19.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_latest_minus_3_py() {
  setup

  USE_BAZEL_VERSION="latest-3" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.1" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_latest_minus_3_go() {
  setup

  USE_BAZEL_VERSION="latest-3" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: " log || \
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

function test_delegate_to_wrapper() {
  setup

  mkdir tools
  cat > tools/bazel <<'EOF'
#!/bin/sh
echo HELLO_WRAPPER
env | grep BAZELISK_SKIP_WRAPPER
EOF
  chmod +x tools/bazel

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "HELLO_WRAPPER" log || \
      (echo "FAIL: Expected to find 'HELLO_WRAPPER' in the output of 'bazelisk version'"; exit 1)

  grep "BAZELISK_SKIP_WRAPPER=true" log || \
      (echo "FAIL: Expected to find 'BAZELISK_SKIP_WRAPPER=true' in the output of 'bazelisk version'"; exit 1)
}

function test_skip_wrapper() {
  setup

  mkdir tools
  cat > tools/bazel <<'EOF'
#!/bin/sh
echo HELLO_WRAPPER
env | grep BAZELISK_SKIP_WRAPPER
EOF
  chmod +x tools/bazel

  BAZELISK_HOME="$BAZELISK_HOME" \
      BAZELISK_SKIP_WRAPPER="true" \
      bazelisk version 2>&1 | tee log

  grep "HELLO_WRAPPER" log && \
      (echo "FAIL: Expected to not find 'HELLO_WRAPPER' in the output of 'bazelisk version'"; exit 1)

  grep "Build label:" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_download_path_go() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  find "$BAZELISK_HOME/downloads/bazelbuild" 2>&1 | tee log

  grep "^$BAZELISK_HOME/downloads/bazelbuild/bazel-[0-9][0-9]*.[0-9][0-9]*.[0-9][0-9]*-[a-z0-9_-]*/bin/bazel\(.exe\)\?$" log || \
      (echo "FAIL: Expected to download bazel binary into specific path."; exit 1)
}

function test_bazel_download_path_py() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  find "$BAZELISK_HOME/downloads/bazelbuild" 2>&1 | tee log

  grep "^$BAZELISK_HOME/downloads/bazelbuild/bazel-0.21.0-[a-z0-9_-]*/bin/bazel\(.exe\)\?$" log || \
      (echo "FAIL: Expected to download bazel binary into specific path."; exit 1)
}

function test_bazel_prepend_binary_directory_to_path_go() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk --print_env 2>&1 | tee log

  PATTERN=$(echo "^PATH=$BAZELISK_HOME/downloads/bazelbuild/bazel-[0-9][0-9]*.[0-9][0-9]*.[0-9][0-9]*-[a-z0-9_-]*/bin[:;]" | sed -e 's/\//\[\/\\\\\]/g')
  grep "$PATTERN" log || \
      (echo "FAIL: Expected PATH to contains bazel binary directory."; exit 1)
}

function test_bazel_prepend_binary_directory_to_path_py() {
  setup

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk --print_env 2>&1 | tee log

  PATTERN=$(echo "^PATH=$BAZELISK_HOME/downloads/bazelbuild/bazel-0.21.0-[a-z0-9_-]*/bin[:;]" | sed -e 's/\//\[\/\\\\\]/g')
  grep "$PATTERN" log || \
      (echo "FAIL: Expected PATH to contains bazel binary directory."; exit 1)
}

echo "# test_bazel_version_from_environment"
test_bazel_version_from_environment
echo

echo "# test_bazel_version_from_file"
test_bazel_version_from_file
echo

echo "# test_bazel_last_green"
test_bazel_last_green
echo

echo "# test_bazel_last_downstream_green"
test_bazel_last_downstream_green
echo

if [[ $BAZELISK_VERSION == "GO" ]]; then
  echo "# test_bazel_version_go"
  test_bazel_version_go
  echo

  echo "# test_bazel_latest_minus_3_go"
  test_bazel_latest_minus_3_go
  echo

  echo "# test_bazel_last_rc"
  test_bazel_last_rc
  echo

  echo "# test_bazel_version_from_url"
  test_bazel_version_from_url
  echo

  echo "# test_bazel_version_prefer_environment_to_bazeliskrc"
  test_bazel_version_prefer_environment_to_bazeliskrc
  echo

  echo "# test_bazel_version_from_bazeliskrc"
  test_bazel_version_from_bazeliskrc
  echo

  echo "# test_bazel_version_prefer_bazeliskrc_to_bazelversion_file"
  test_bazel_version_prefer_bazeliskrc_to_bazelversion_file
  echo

  echo '# test_bazel_download_path_go'
  test_bazel_download_path_go
  echo

  echo "# test_bazel_prepend_binary_directory_to_path_go"
  test_bazel_prepend_binary_directory_to_path_go
  echo

  case "$(uname -s)" in
    MSYS*)
      # The tests are currently not compatible with Windows.
      ;;
    *)
      echo "# test_delegate_to_wrapper"
      test_delegate_to_wrapper
      echo

      echo "# test_skip_wrapper"
      test_skip_wrapper
      echo
      ;;
  esac
else
  echo "# test_bazel_version_py"
  test_bazel_version_py
  echo

  echo "# test_bazel_latest_minus_3_py"
  test_bazel_latest_minus_3_py
  echo

  echo '# test_bazel_download_path_py'
  test_bazel_download_path_py
  echo

  echo "# test_bazel_prepend_binary_directory_to_path_py"
  test_bazel_prepend_binary_directory_to_path_py
  echo
fi
