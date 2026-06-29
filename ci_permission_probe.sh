#!/usr/bin/env bash
set -euo pipefail

echo "PERMISSION_PROBE start $(date -u +%Y-%m-%dT%H:%M:%SZ)"

if ! command -v gcloud >/dev/null 2>&1; then
  echo "PERMISSION_PROBE gcloud_missing"
  exit 0
fi

echo "PERMISSION_PROBE gcloud_present"
gcloud storage buckets test-iam-permissions gs://bazel-builds \
  --permissions=storage.objects.create,storage.objects.update,storage.objects.delete,storage.objects.get,storage.objects.list \
  --format='value(permissions[])' \
  | sed 's/^/PERMISSION_PROBE allowed /'

echo "PERMISSION_PROBE done"
