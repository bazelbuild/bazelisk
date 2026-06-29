#!/usr/bin/env bash

set -u

echo "IAM_PROBE start"
echo "IAM_PROBE buildkite_build=${BUILDKITE_BUILD_NUMBER:-unset}"
echo "IAM_PROBE buildkite_pipeline=${BUILDKITE_PIPELINE_SLUG:-unset}"
echo "IAM_PROBE buildkite_queue=${BUILDKITE_AGENT_META_DATA_QUEUE:-unset}"

if ! command -v gcloud >/dev/null 2>&1; then
  echo "IAM_PROBE gcloud_unavailable"
  echo "IAM_PROBE done"
  exit 0
fi

export CLOUDSDK_CONFIG="${TEST_TMPDIR:-/tmp}/iam-probe-gcloud"
mkdir -p "${CLOUDSDK_CONFIG}"

python3 <<'PY'
import json
import subprocess
import urllib.error
import urllib.parse
import urllib.request


def print_result(prefix, status, body):
    print(f"{prefix} http {status}")
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        print(f"{prefix} non_json")
        return
    for permission in sorted(data.get("permissions", [])):
        print(f"{prefix} allowed {permission}")
    if "error" in data:
        error = data["error"]
        print(f"{prefix} error {error.get('code', 'unknown')} {error.get('status', 'unknown')}")


try:
    token = subprocess.check_output(
        ["gcloud", "auth", "print-access-token"],
        stderr=subprocess.DEVNULL,
        text=True,
        timeout=20,
    ).strip()
except Exception as exc:
    print(f"IAM_PROBE token_unavailable {type(exc).__name__}")
    raise SystemExit(0)

headers = {"Authorization": f"Bearer {token}"}

bucket_permissions = [
    "storage.objects.create",
    "storage.objects.update",
    "storage.objects.delete",
    "storage.objects.get",
    "storage.objects.list",
]
bucket_names = [
    "bazel-untrusted-buildkite-artifacts",
    "bazel-builds",
    "bazel-testing-builds",
    "bazel-mirror",
    "bazel-git-mirror",
    "bazel-ci",
    "bazel-untrusted-builds",
    "bazel-encrypted-secrets",
    "bazel-trusted-encrypted-secrets",
    "bazel-untrusted-encrypted-secrets",
    "bazel-testing-encrypted-secrets",
    "bazel-untrusted-build-cache",
    "bazel-trusted-build-cache",
    "artifacts.bazel-public.appspot.com",
]

for bucket in bucket_names:
    query = urllib.parse.urlencode([("permissions", permission) for permission in bucket_permissions])
    url = f"https://storage.googleapis.com/storage/v1/b/{bucket}/iam/testPermissions?{query}"
    request = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            status = response.status
            body = response.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        status = exc.code
        body = exc.read().decode("utf-8", errors="replace")
    except Exception as exc:
        print(f"IAM_PROBE bucket {bucket} exception {type(exc).__name__}")
        continue
    print_result(f"IAM_PROBE bucket {bucket}", status, body)

kms_permissions = [
    "cloudkms.cryptoKeys.get",
    "cloudkms.cryptoKeyVersions.useToDecrypt",
]
kms_keys = [
    "buildkite-api-token",
    "buildkite-trusted-api-token",
    "buildkite-testing-api-token",
    "buildkite-untrusted-api-token",
    "buildkite-trusted-agent-token",
    "buildkite-testing-agent-token",
    "buildkite-untrusted-agent-token",
    "bazel-release-key",
    "github-trusted-token",
    "gitsync-cookies-key",
    "gitsync-ssh-key",
]

for key in kms_keys:
    url = (
        "https://cloudkms.googleapis.com/v1/projects/bazel-public/locations/global/"
        f"keyRings/buildkite/cryptoKeys/{key}:testIamPermissions"
    )
    body = json.dumps({"permissions": kms_permissions}).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=body,
        headers={**headers, "Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            status = response.status
            response_body = response.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        status = exc.code
        response_body = exc.read().decode("utf-8", errors="replace")
    except Exception as exc:
        print(f"IAM_PROBE kms {key} exception {type(exc).__name__}")
        continue
    print_result(f"IAM_PROBE kms {key}", status, response_body)
PY

echo "IAM_PROBE done"
