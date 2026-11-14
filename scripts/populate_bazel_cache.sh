#!/usr/bin/env bash
# populate_bazel_cache.sh — Pre-populate a bazelisk download cache with Bazel binaries.
#
# The cache layout matches what bazelisk expects when BAZELISK_BASE_URL is set
# to point at the cache root:
#   <cache-root>/<version>/<filename>
#
# Usage example (in a Dockerfile):
#   RUN ./populate_bazel_cache.sh \
#       --cache-root /opt/bazel-cache \
#       --version 7.4.1 --version 8.0.0 \
#       --os linux --arch x86_64
#   ENV BAZELISK_BASE_URL=file:///opt/bazel-cache

set -euo pipefail

BAZEL_BASE_URL="https://github.com/bazelbuild/bazel/releases/download"

# ------------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") --cache-root <path> --version <ver> [options]

Required:
  --cache-root <path>   Root directory for the bazelisk cache
  --version <version>   Bazel version to download (repeatable)

Optional:
  --os <os>             Target OS: linux, darwin, windows
                        (repeatable; default: host OS)
  --arch <arch>         Target arch: x86_64, arm64
                        (repeatable; default: host arch)
  --nojdk               Also download bazel_nojdk variants
  -h, --help            Show this help message

Example:
  $(basename "$0") --cache-root /opt/bazel-cache \\
      --version 7.4.1 --version 8.0.0 \\
      --os linux --os darwin --arch x86_64 --arch arm64
EOF
}

# ------------------------------------------------------------------------------
detect_os() {
    case "$(uname -s)" in
        Linux)              echo "linux" ;;
        Darwin)             echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) echo "ERROR: Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "x86_64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *) echo "ERROR: Unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac
}

# ------------------------------------------------------------------------------
sha256_of_file() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        echo "ERROR: no sha256 tool found (need sha256sum or shasum)" >&2
        exit 1
    fi
}

download_file() {
    local url="$1"
    local dest="$2"
    if command -v curl &>/dev/null; then
        curl -fsSL --retry 3 --retry-delay 2 -o "$dest" "$url"
    elif command -v wget &>/dev/null; then
        wget -q --tries=3 -O "$dest" "$url"
    else
        echo "ERROR: no download tool found (need curl or wget)" >&2
        exit 1
    fi
}

# ------------------------------------------------------------------------------
CACHE_ROOT=""
VERSIONS=()
OSES=()
ARCHS=()
NOJDK=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --cache-root)   CACHE_ROOT="$2"; shift 2 ;;
        --version)      VERSIONS+=("$2"); shift 2 ;;
        --os)           OSES+=("$2"); shift 2 ;;
        --arch)         ARCHS+=("$2"); shift 2 ;;
        --nojdk)        NOJDK=true; shift ;;
        -h|--help)      usage; exit 0 ;;
        *) echo "ERROR: Unknown option: $1" >&2; echo; usage >&2; exit 1 ;;
    esac
done

if [[ -z "$CACHE_ROOT" ]]; then
    echo "ERROR: --cache-root is required" >&2; echo; usage >&2; exit 1
fi
if [[ ${#VERSIONS[@]} -eq 0 ]]; then
    echo "ERROR: at least one --version is required" >&2; echo; usage >&2; exit 1
fi

[[ ${#OSES[@]}  -eq 0 ]] && OSES=("$(detect_os)")
[[ ${#ARCHS[@]} -eq 0 ]] && ARCHS=("$(detect_arch)")

FLAVORS=("bazel")
[[ "$NOJDK" == "true" ]] && FLAVORS+=("bazel_nojdk")

# ------------------------------------------------------------------------------
errors=0

for version in "${VERSIONS[@]}"; do
    for os in "${OSES[@]}"; do
        for arch in "${ARCHS[@]}"; do
            for flavor in "${FLAVORS[@]}"; do

                suffix=""
                [[ "$os" == "windows" ]] && suffix=".exe"

                filename="${flavor}-${version}-${os}-${arch}${suffix}"
                dest_dir="${CACHE_ROOT}/${version}"
                bin_dest="${dest_dir}/${filename}"
                sha_dest="${bin_dest}.sha256"

                if [[ -f "$bin_dest" && -f "$sha_dest" ]]; then
                    echo "  [skip] ${filename} (already cached)"
                    continue
                fi

                mkdir -p "$dest_dir"

                bin_url="${BAZEL_BASE_URL}/${version}/${filename}"
                sha_url="${bin_url}.sha256"

                echo "Downloading ${filename}..."

                tmp_bin="$(mktemp "${dest_dir}/.tmp.XXXXXX")"
                tmp_sha="$(mktemp "${dest_dir}/.tmp.XXXXXX")"
                # Clean up temp files on any exit from this iteration.
                trap 'rm -f "$tmp_bin" "$tmp_sha"' RETURN

                if ! download_file "$bin_url" "$tmp_bin"; then
                    echo "  ERROR: failed to download $bin_url" >&2
                    errors=$((errors + 1))
                    continue
                fi

                if ! download_file "$sha_url" "$tmp_sha"; then
                    echo "  ERROR: failed to download $sha_url" >&2
                    errors=$((errors + 1))
                    continue
                fi

                expected="$(awk '{print $1}' "$tmp_sha")"
                actual="$(sha256_of_file "$tmp_bin")"

                if [[ "$expected" != "$actual" ]]; then
                    echo "  ERROR: SHA256 mismatch for ${filename}" >&2
                    echo "    expected: $expected" >&2
                    echo "    actual:   $actual" >&2
                    errors=$((errors + 1))
                    continue
                fi

                echo "  verified: $actual"

                [[ "$os" != "windows" ]] && chmod +x "$tmp_bin"

                mv "$tmp_bin" "$bin_dest"
                mv "$tmp_sha" "$sha_dest"

                trap - RETURN
            done
        done
    done
done

echo
if [[ $errors -gt 0 ]]; then
    echo "Completed with $errors error(s). Cache may be incomplete." >&2
    exit 1
fi
echo "Done. Cache populated at ${CACHE_ROOT}"
