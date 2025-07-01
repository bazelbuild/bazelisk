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

  cp "$(rlocation _main/releases_for_tests.json)" "${BAZELISK_HOME}/bazelbuild-releases.json"
  touch "${BAZELISK_HOME}/bazelbuild-releases.json"
  ln -s "${BAZELISK_HOME}/bazelbuild-releases.json" "${BAZELISK_HOME}/releases.json"

  cd "$(mktemp -d $TEST_TMPDIR/workspace.XXXXXX)"
  touch WORKSPACE BUILD
}

function bazelisk() {
  if [[ -n $(rlocation _main/bazelisk.py) ]]; then
    if [[ $BAZELISK_VERSION == "PY3" ]]; then
      echo "Running Bazelisk with $(python3 -V)..."
      python3 "$(rlocation _main/bazelisk.py)" "$@"
    else
      echo "Running Bazelisk with $(python -V)..."
      python "$(rlocation _main/bazelisk.py)" "$@"
    fi
  elif [[ -n $(rlocation _main/windows_amd64_debug/bazelisk.exe) ]]; then
    "$(rlocation _main/windows_amd64_debug/bazelisk.exe)" "$@"
  elif [[ -n $(rlocation _main/darwin_amd64_debug/bazelisk) ]]; then
    "$(rlocation _main/darwin_amd64_debug/bazelisk)" "$@"
  elif [[ -n $(rlocation _main/linux_amd64_debug/bazelisk) ]]; then
    "$(rlocation _main/linux_amd64_debug/bazelisk)" "$@"
  elif [[ -n $(rlocation _main/windows_amd64_stripped/bazelisk.exe) ]]; then
    "$(rlocation _main/windows_amd64_stripped/bazelisk.exe)" "$@"
  elif [[ -n $(rlocation _main/darwin_amd64_stripped/bazelisk) ]]; then
    "$(rlocation _main/darwin_amd64_stripped/bazelisk)" "$@"
  elif [[ -n $(rlocation _main/linux_amd64_stripped/bazelisk) ]]; then
    "$(rlocation _main/linux_amd64_stripped/bazelisk)" "$@"
  elif [[ -n $(rlocation _main/bazelisk_/bazelisk) ]]; then
    "$(rlocation _main/bazelisk_/bazelisk)" "$@"
  elif [[ -n $(rlocation _main/bazelisk_/bazelisk.exe) ]]; then
    "$(rlocation _main/bazelisk_/bazelisk.exe)" "$@"
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

  # 7.1.0 is the latest release in releases_for_tests.json
  grep "Build label: 7.1.0" log || \
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

  USE_BAZEL_VERSION="7.0.0" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 7.0.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_prefer_environment_to_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=7.2.1" > .bazeliskrc

  USE_BAZEL_VERSION="7.0.0" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 7.0.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_workspace_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=7.2.1" > .bazeliskrc

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.2.1" log || \
      (echo "FAIL: Expected to find 'Build label: 7.2.1' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_user_home_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=7.2.1" > "${USER_HOME}/.bazeliskrc"

  BAZELISK_HOME="$BAZELISK_HOME" \
      HOME="$USER_HOME" \
      USERPROFILE="$USER_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.2.1" log || \
      (echo "FAIL: Expected to find 'Build label: 7.2.1' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_prefer_workspace_bazeliskrc_to_user_home_bazeliskrc() {
  setup

  echo "USE_BAZEL_VERSION=7.2.1" > .bazeliskrc
  echo "USE_BAZEL_VERSION=7.0.0" > "${USER_HOME}/.bazeliskrc"

  BAZELISK_HOME="$BAZELISK_HOME" \
      HOME="$USER_HOME" \
      USERPROFILE="$USER_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.2.1" log || \
      (echo "FAIL: Expected to find 'Build label: 7.2.1' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_prefer_bazeliskrc_to_bazelversion_file() {
  setup

  echo "USE_BAZEL_VERSION=7.2.1" > .bazeliskrc
  echo "7.0.0" > .bazelversion

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.2.1" log || \
      (echo "FAIL: Expected to find 'Build label: 7.2.1' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_file() {
  setup

  echo "7.0.0" > .bazelversion

  BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 7.0.0' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_format_url() {
  setup

  echo "7.2.1" > .bazelversion

  BAZELISK_FORMAT_URL="https://github.com/bazelbuild/bazel/releases/download/%v/bazel-%v-%o-%m%e" \
      BAZELISK_HOME="$BAZELISK_HOME" \
          bazelisk version 2>&1 | tee log

  grep "Build label: 7.2.1" log || \
      (echo "FAIL: Expected to find 'Build label: 7.2.1' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_version_from_base_url() {
  setup

  echo "7.2.1" > .bazelversion

  BAZELISK_BASE_URL="https://github.com/bazelbuild/bazel/releases/download" \
      BAZELISK_HOME="$BAZELISK_HOME" \
          bazelisk version 2>&1 | tee log

  grep "Build label: 7.2.1" log || \
      (echo "FAIL: Expected to find 'Build label: 7.2.1' in the output of 'bazelisk version'"; exit 1)
}

function test_bazel_latest_minus_3_py() {
  setup

  USE_BAZEL_VERSION="latest-3" \
      BAZELISK_HOME="$BAZELISK_HOME" \
      bazelisk version 2>&1 | tee log

  grep "Build label: 7.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 7.0.0' in the output of 'bazelisk version'"; exit 1)
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

function test_BAZELISK_NOJDK() {
  setup

  # Running the nojdk Bazel without a valid JAVA is expected to fail
  set +e
  BAZELISK_HOME="$BAZELISK_HOME" \
      USE_BAZEL_VERSION="7.0.0" \
      BAZELISK_NOJDK="1" \
      bazelisk --noautodetect_server_javabase version 2>&1 | tee log
  set -e

  grep "FATAL: Could not find embedded or explicit server javabase, and --noautodetect_server_javabase is set." log || \
      (echo "FAIL: nojdk Bazel should fail when no JDK is supplied."; exit 1)

  # Theoretically there could be a cache collision in the Bazelisk cache between nojdk and regular Bazel.
  #
  # Ensure that isn't happening by running the regular Bazel right after nojdk Bazel, just in case. If there is a collision, it will fail.
  BAZELISK_HOME="$BAZELISK_HOME" \
      USE_BAZEL_VERSION="7.0.0" \
      JAVA_HOME="does/not/exist" \
      bazelisk --noautodetect_server_javabase version 2>&1 | tee log

  grep "Build label: 7.0.0" log || \
      (echo "FAIL: Expected to find 'Build label: 7.0.0' in the output of 'bazelisk version'"; exit 1)
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

  echo 8.3.0 > .bazelversion

  cat >MODULE.bazel <<EOF
print_path = use_repo_rule("//:print_path.bzl", "print_path")

print_path(name = "print_path")
EOF

cat >print_path.bzl <<EOF
def _print_path_impl(rctx):
    print("PATH is: {}".format(rctx.os.environ["PATH"]))

    rctx.file("REPO.bazel", "")
    rctx.file("BUILD", "")
    rctx.file("defs.bzl", "def noop(): pass")

print_path = repository_rule(
    implementation = _print_path_impl,
)
EOF

  BAZELISK_HOME="$BAZELISK_HOME" bazelisk fetch --repo=@print_path 2>&1 | tee log1

  BAZELISK_HOME="$BAZELISK_HOME" bazelisk clean --expunge 2>&1

  # We need a separate mirror of bazel binaries, which has identical files.
  # Ideally we wouldn't depend on sourceforge for test runtime, but hey, it exists and it works.
  BAZELISK_HOME="$BAZELISK_HOME" BAZELISK_BASE_URL=https://downloads.sourceforge.net/project/bazel.mirror bazelisk fetch --repo=@print_path 2>&1 | tee log2

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

  echo "7.1.1" > .bazelversion

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
      expected_sha256="dae351f491ead382bfc7c14d8957b9c8d735300c566c2161e34035eab994c1f2"
      ;;
    linux)
      expected_sha256="D93508529d41136065c7b1E5ff555fbfb9d18fd00e768886F2fc7dfb53b05B43"
      ;;
    msys*|mingw*|cygwin*)
      expected_sha256="9fb6f439e2eb646b9bae7bd2c0317165c0b08abc0bba25f6af53180fa1f86997"
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

  # 7.1.0 is the latest release in releases_for_tests.json
  grep "^$BAZELISK_HOME/downloads/bazelbuild/bazel-7.1.0-[a-z0-9_-]*/bin/bazel\(.exe\)\?$" log || \
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

  # 7.1.0 is the latest release in releases_for_tests.json
  PATTERN=$(echo "^PATH=$BAZELISK_HOME/downloads/bazelbuild/bazel-7.1.0-[a-z0-9_-]*/bin[:;]" | sed -e 's/\//\[\/\\\\\]/g')
  grep "$PATTERN" log || \
      (echo "FAIL: Expected PATH to contains bazel binary directory."; exit 1)
}

echo "# test_bazel_version_from_environment"
test_bazel_version_from_environment
echo

echo "# test_bazel_version_prefer_environment_to_bazeliskrc"
test_bazel_version_prefer_environment_to_bazeliskrc
echo

echo "# test_bazel_version_from_workspace_bazeliskrc"
test_bazel_version_from_workspace_bazeliskrc
echo

echo "# test_bazel_version_from_file"
test_bazel_version_from_file
echo

echo "# test_bazel_last_green"
test_bazel_last_green
echo

echo "# test_BAZELISK_NOJDK"
test_BAZELISK_NOJDK
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
