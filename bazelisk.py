#!/usr/bin/env python3
"""
Copyright 2018 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

from distutils.version import LooseVersion
import json
import platform
import os.path
import subprocess
import shutil
import sys
import time
import urllib.request

ONE_HOUR = 1 * 60 * 60

def decide_which_bazel_version_to_use():
    # Check in this order:
    # - env var "USE_BAZEL_VERSION" is set to a specific version.
    # - env var "USE_NIGHTLY_BAZEL" or "USE_BAZEL_NIGHTLY" is set -> latest nightly. (TODO)
    # - env var "USE_CANARY_BAZEL" or "USE_BAZEL_CANARY" is set -> latest rc. (TODO)
    # - the file workspace_root/tools/bazel exists -> that version. (TODO)
    # - workspace_root/.bazelversion exists -> read contents, that version.
    # - workspace_root/WORKSPACE contains a version -> that version. (TODO)
    # - fallback: latest release
    if 'USE_BAZEL_VERSION' in os.environ:
        return os.environ['USE_BAZEL_VERSION']
    try:
        workspace_root = find_workspace_root()
        if workspace_root:
            with open(os.path.join(workspace_root, '.bazelversion'), 'r') as f:
                return f.read().strip()
    except FileNotFoundError:
        pass
    return "latest"

def find_workspace_root(root=None):
    if root is None:
        root = os.getcwd()
    if os.path.exists(os.path.join(root, 'WORKSPACE')):
        return root
    new_root = os.path.dirname(root)
    return find_workspace_root(new_root) if new_root != root else None

def resolve_latest_version():
    req = urllib.request.Request('https://api.github.com/repos/bazelbuild/bazel/releases', method='GET')
    res = urllib.request.urlopen(req).read()
    releases = json.loads(res.decode('utf-8'))

    latest_version = LooseVersion(releases[0]["tag_name"])
    for release in releases:
        latest_version = max(latest_version, LooseVersion(release["tag_name"]))
    return latest_version.__str__()

def resolve_version_label_to_number(bazelisk_directory, version):
    if version == "latest":
        latest_cache = os.path.join(bazelisk_directory, 'latest_bazel')
        try:
            if abs(time.time() - os.path.getmtime(latest_cache)) < ONE_HOUR:
                with open(latest_cache, 'r') as f:
                    return f.read().strip()
        except FileNotFoundError:
            pass
        latest_version = resolve_latest_version()
        with open(latest_cache, 'w') as f:
            f.write(latest_version)
        return latest_version
    return version

def determine_bazel_filename(version):
    machine = platform.machine()
    if machine != "x86_64":
        raise Exception("Unsupported machine architecture '{}'. Bazel currently only supports x86_64.".format(machine))

    operating_system = platform.system().lower()
    if operating_system not in ("linux", "darwin", "windows"):
        raise Exception("Unsupported operating system '{}'. Bazel currently only supports Linux, macOS and Windows.".format(operating_system))

    return "bazel-{}-{}-{}".format(version, operating_system, machine)

def determine_release_or_rc(version):
    parts = version.lower().split("rc")
    if len(parts) == 1:
        # e.g. ("0.20.0", "release") for 0.20.0
        return (version, "release")
    elif len(parts) == 2:
        # e.g. ("0.20.0", "rc2") for 0.20.0rc2
        return (parts[0], "rc" + parts[1])
    else:
        raise Exception("Invalid version: {}. Versions must be in the form <x>.<y>.<z>[rc<rc-number>]".format(version))


def download_bazel_into_directory(version, directory):
    bazel_filename = determine_bazel_filename(version)
    (parsed_version, release_or_rc) = determine_release_or_rc(version)
    url = "https://releases.bazel.build/{}/{}/{}".format(
            parsed_version,
            release_or_rc,
            bazel_filename)
    destination_path = os.path.join(directory, bazel_filename)
    if not os.path.exists(destination_path):
        sys.stderr.write("Downloading {}...\n".format(url))
        with urllib.request.urlopen(url) as response, open(destination_path, 'wb') as out_file:
            shutil.copyfileobj(response, out_file)
    os.chmod(destination_path, 0o755)
    return destination_path

def main(argv=None):
    if argv is None:
        argv = sys.argv

    bazelisk_directory = os.path.join(os.path.expanduser("~"), ".bazelisk")
    os.makedirs(bazelisk_directory, exist_ok=True)

    bazel_version = decide_which_bazel_version_to_use()
    bazel_version = resolve_version_label_to_number(bazelisk_directory, bazel_version)
    sys.stderr.write('Using Bazel {}...\n'.format(bazel_version))

    bazel_directory = os.path.join(bazelisk_directory, "bin")
    os.makedirs(bazel_directory, exist_ok=True)
    bazel_path = download_bazel_into_directory(bazel_version, bazel_directory)

    return subprocess.Popen([bazel_path] + argv[1:], close_fds=True).wait()

if __name__ == "__main__":
    sys.exit(main())
