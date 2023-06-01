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

  USER_HOME="$(mktemp -d $TEST_TMPDIR/user.XXXXXX)"
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

  USE_BAZEL_VERSION="5.0.0" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 5.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 5.0.0' in the output of 'bazelisk version'"; exit 1)
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

function test_bazel_version_from_workspace_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=0.19.0" > .bazeliskrc

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.19.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_user_home_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=0.19.0" > "${USER_HOME}/.bazeliskrc"

  BAZELISK_HOME="$BAZELISK_HOME" \
      HOME="$USER_HOME" \
      USERPROFILE="$USER_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.19.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_prefer_workspace_bazeliskrc_to_user_home_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=0.19.0" > .bazeliskrc
  echo "USE_BAZEL_VERSION=0.20.0" > "${USER_HOME}/.bazeliskrc"

  BAZELISK_HOME="$BAZELISK_HOME" \
      HOME="$USER_HOME" \
      USERPROFILE="$USER_HOME" \
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

  echo "5.0.0" > .bazelversion

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 5.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 5.0.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_format_url() {
  setup

  echo "0.19.0" > .bazelversion

  BAZELISK_FORMAT_URL="https://github.com/bazelbuild/bazel/releases/download/%v/bazel-%v-%o-%m%e" \
      BAZELISK_HOME="$BAZELISK_HOME" \
          bazelisk version 2>&1 | tee log

  grep "Build label: 0.19.0" log || \
      (echo "FAIL: Expected to find 'Build label: 0.19.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_base_url() {
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

function test_path_is_consistent_regardless_of_base_url() {
  setup

  echo 6.2.0 > .bazelversion

  cat >WORKSPACE <<EOF
load("//:print_path.bzl", "print_path")

print_path(name = "print_path")

load("@print_path//:defs.bzl", "noop")

noop()
EOF

cat >print_path.bzl <<EOF
def _print_path_impl(rctx):
    print("PATH is: {}".format(rctx.os.environ["PATH"]))

    rctx.file("WORKSPACE", "")
    rctx.file("BUILD", "")
    rctx.file("defs.bzl", "def noop(): pass")

print_path = repository_rule(
    implementation = _print_path_impl,
)
EOF

  BAZELISK_HOME="$BAZELISK_HOME" bazelisk sync --only=print_path 2>&1 | tee log1

  BAZELISK_HOME="$BAZELISK_HOME" bazelisk clean --expunge 2>&1

  # We need a separate mirror of bazel binaries, which has identical files.
  # Ideally we wouldn't depend on sourceforge for test runtime, but hey, it exists and it works.
  BAZELISK_HOME="$BAZELISK_HOME" BAZELISK_BASE_URL=https://downloads.sourceforge.net/project/bazel.mirror bazelisk sync --only=print_path 2>&1 | tee log2

  path1="$(grep "PATH is:" log1)"
  path2="$(grep "PATH is:" log2)"

  [[ -n "${path1}" && -n "${path2}" ]] || \
      (echo "FAIL: Expected PATH to be non-empty, got path1=${path1}, path2=${path2}"; exit 1)

  [[ "${path1}" == "${path2}" ]] || \
      (echo "FAIL: Expected PATH to be the same regardless of which mirror was used, got path1=${path1}, path2=${path2}"; exit 1)
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

  find "$BAZELISK_HOME/downloads/metadata/bazelbuild" 2>&1 | tee log

  grep "^$BAZELISK_HOME/downloads/metadata/bazelbuild/bazel-[0-9][0-9]*.[0-9][0-9]*.[0-9][0-9]*-[a-z0-9_-]*$" log || \
      (echo "FAIL: Expected to download bazel metadata in specific path."; exit 1)
}

function test_bazel_verify_sha256() {
  setup

  echo "6.1.1" > .bazelversion

  # First try to download and expect an invalid hash (it doesn't matter what it is).
  if BAZELISK_HOME="$BAZELISK_HOME" BAZELISK_VERIFY_SHA256="invalid-hash" \
      bazelisk version 2>&1 | tee log; then
    echo "FAIL: Command should have errored out"; exit 1
  fi

  grep "need sha256=invalid-hash" log || \
      (echo "FAIL: Expected to find hash mismatch"; exit 1)

  # IMPORTANT: The mixture of lowercase and uppercase letters in the hashes below is
  # intentional to ensure the variable contents are normalized before comparison.
  # If updating these values, re-introduce randomness.
  local os="$(uname -s | tr A-Z a-z)"
  case "${os}" in
    darwin)
      expected_sha256="038e95BAE998340812562ab8d6ada1a187729630bc4940a4cd7920cc78acf156"
      ;;
    linux)
      expected_sha256="651a20d85531325df406b38f38A1c2578c49D5e61128fba034f5b6abdb3d303f"
      ;;
    msys*|mingw*|cygwin*)
      expected_sha256="1d997D344936a1d98784ae58db1152d083569556f85cd845e6e340EE855357f9"
      ;;
    *)
      echo "FAIL: Unknown OS ${os} in test"
      exit 1
      ;;
  esac

  # Now try the same download as before but with the correct hash expectation. Note that the
  # hash has a random uppercase / lowercase mixture to ensure this does not impact equality
  # checks.
  BAZELISK_HOME="$BAZELISK_HOME" BAZELISK_VERIFY_SHA256="${expected_sha256}" \
      bazelisk version 2>&1 | tee log

  grep "Build label:" log || \
      (echo "FAIL: Expected to find 'Build label' in the output of 'bazelisk version'"; exit 1)
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

  local os="$(uname -s | tr A-Z a-z)"
  case "${os}" in
      darwin|linux)
        path_entry_delimiter=":"
        path_delimiter="/"
        extension=""
        ;;
      msys*|mingw*|cygwin*)
        path_entry_delimiter=";"
        path_delimiter="\\"
        extension=".exe"
        ;;
      *)
        echo "FAIL: Unknown OS ${os} in test"
        exit 1
        ;;
    esac
  path_entry="$(grep "^PATH=" log | cut -d= -f2- | cut -d"${path_entry_delimiter}" -f1)"

  [[ -x "${path_entry}${path_delimiter}bazel${extension}" ]] || \
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

  echo "# test_bazel_version_from_format_url"
  test_bazel_version_from_format_url
  echo

  echo "# test_bazel_version_from_base_url"
  test_bazel_version_from_base_url
  echo

  echo "# test_bazel_version_prefer_environment_to_bazeliskrc"
  test_bazel_version_prefer_environment_to_bazeliskrc
  echo

  echo "# test_bazel_version_from_workspace_bazeliskrc"
  test_bazel_version_from_workspace_bazeliskrc
  echo

  echo "# test_bazel_version_from_user_home_bazeliskrc"
  test_bazel_version_from_user_home_bazeliskrc
  echo

  echo "# test_bazel_version_prefer_workspace_bazeliskrc_to_user_home_bazeliskrc"
  test_bazel_version_prefer_workspace_bazeliskrc_to_user_home_bazeliskrc
  echo

  echo "# test_bazel_version_prefer_bazeliskrc_to_bazelversion_file"
  test_bazel_version_prefer_bazeliskrc_to_bazelversion_file
  echo

  echo '# test_bazel_download_path_go'
  test_bazel_download_path_go
  echo

  echo '# test_bazel_verify_sha256'
  test_bazel_verify_sha256
  echo

  echo "# test_bazel_prepend_binary_directory_to_path_go"
  test_bazel_prepend_binary_directory_to_path_go
  echo

  echo "# test_path_is_consistent_regardless_of_base_url"
  test_path_is_consistent_regardless_of_base_url
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
