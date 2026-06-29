#!/usr/bin/env bash
set -euo pipefail

echo "PERMISSION_PROBE start $(date -u +%Y-%m-%dT%H:%M:%SZ)"

if ! command -v gcloud >/dev/null 2>&1; then
  echo "PERMISSION_PROBE gcloud_missing"
  exit 0
fi

echo "PERMISSION_PROBE gcloud_present"
export CLOUDSDK_CONFIG="${TEST_TMPDIR:-/tmp}/gcloud-config"
mkdir -p "${CLOUDSDK_CONFIG}"

python3 <<'PY'
import json
import subprocess
import urllib.error
import urllib.parse
import urllib.request

permissions = [
    "storage.objects.create",
    "storage.objects.update",
    "storage.objects.delete",
    "storage.objects.get",
    "storage.objects.list",
]

try:
    token = subprocess.check_output(
        ["gcloud", "auth", "print-access-token"],
        stderr=subprocess.DEVNULL,
        text=True,
    ).strip()
except Exception as exc:
    print(f"PERMISSION_PROBE token_unavailable {type(exc).__name__}")
    raise SystemExit(0)

query = urllib.parse.urlencode([("permissions", permission) for permission in permissions])
url = f"https://storage.googleapis.com/storage/v1/b/bazel-builds/iam/testPermissions?{query}"
request = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})

try:
    with urllib.request.urlopen(request, timeout=20) as response:
        status = response.status
        body = response.read().decode("utf-8", errors="replace")
except urllib.error.HTTPError as exc:
    status = exc.code
    body = exc.read().decode("utf-8", errors="replace")
except Exception as exc:
    print(f"PERMISSION_PROBE api_exception {type(exc).__name__}")
    raise SystemExit(0)

print(f"PERMISSION_PROBE api_http {status}")
try:
    data = json.loads(body)
except json.JSONDecodeError:
    print("PERMISSION_PROBE api_non_json")
    raise SystemExit(0)

allowed = sorted(data.get("permissions", []))
for permission in allowed:
    print(f"PERMISSION_PROBE allowed {permission}")
print(f"PERMISSION_PROBE allowed_count {len(allowed)}")

if "error" in data:
    error = data["error"]
    code = error.get("code", "unknown")
    status_text = error.get("status", "unknown")
    print(f"PERMISSION_PROBE api_error {code} {status_text}")
PY

echo "PERMISSION_PROBE done"
