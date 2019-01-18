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

import collections
from contextlib import closing
from distutils.version import LooseVersion
import json
import os.path
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import time

try:
  from urllib.request import urlopen
except ImportError:
  # Python 2.x compatibility hack.
  from urllib2 import urlopen

ONE_HOUR = 1 * 60 * 60

# Bazelisk exits with this code when GPG is installed but the binary
# cannot be authenticated.
AUTHENTICATION_FAILURE_EXIT_CODE = 2


def decide_which_bazel_version_to_use():
  # Check in this order:
  # - env var "USE_BAZEL_VERSION" is set to a specific version.
  # - env var "USE_NIGHTLY_BAZEL" or "USE_BAZEL_NIGHTLY" is set -> latest
  #   nightly. (TODO)
  # - env var "USE_CANARY_BAZEL" or "USE_BAZEL_CANARY" is set -> latest
  #   rc. (TODO)
  # - the file workspace_root/tools/bazel exists -> that version. (TODO)
  # - workspace_root/.bazelversion exists -> read contents, that version.
  # - workspace_root/WORKSPACE contains a version -> that version. (TODO)
  # - fallback: latest release
  if 'USE_BAZEL_VERSION' in os.environ:
    return os.environ['USE_BAZEL_VERSION']

  workspace_root = find_workspace_root()
  if workspace_root:
    bazelversion_path = os.path.join(workspace_root, '.bazelversion')
    if os.path.exists(bazelversion_path):
      with open(bazelversion_path, 'r') as f:
        return f.read().strip()

  return 'latest'


def find_workspace_root(root=None):
  if root is None:
    root = os.getcwd()
  if os.path.exists(os.path.join(root, 'WORKSPACE')):
    return root
  new_root = os.path.dirname(root)
  return find_workspace_root(new_root) if new_root != root else None


def resolve_latest_version():
  res = urlopen('https://api.github.com/repos/bazelbuild/bazel/releases').read()
  return str(
      max(
          LooseVersion(release['tag_name'])
          for release in json.loads(res.decode('utf-8'))
          if not release['prerelease']))


def resolve_version_label_to_number(bazelisk_directory, version):
  if version == 'latest':
    latest_cache = os.path.join(bazelisk_directory, 'latest_bazel')
    if os.path.exists(latest_cache):
      if abs(time.time() - os.path.getmtime(latest_cache)) < ONE_HOUR:
        with open(latest_cache, 'r') as f:
          return f.read().strip()
    latest_version = resolve_latest_version()
    with open(latest_cache, 'w') as f:
      f.write(latest_version)
    return latest_version
  return version


def determine_bazel_filename(version):
  machine = normalized_machine_arch_name()
  if machine != 'x86_64':
    raise Exception(
        'Unsupported machine architecture "{}". Bazel currently only supports x86_64.'
        .format(machine))

  operating_system = platform.system().lower()
  if operating_system not in ('linux', 'darwin', 'windows'):
    raise Exception('Unsupported operating system "{}". '
                    'Bazel currently only supports Linux, macOS and Windows.'
                    .format(operating_system))

  filename_ending = '.exe' if operating_system == 'windows' else ''
  return 'bazel-{}-{}-{}{}'.format(version, operating_system, machine,
                                   filename_ending)


def normalized_machine_arch_name():
  machine = platform.machine().lower()
  if machine == 'amd64':
    machine = 'x86_64'
  return machine


SubprocessResult = collections.namedtuple("SubprocessResult", ("exit_code",))


def subprocess_run(command, input=None, error_message=None):
  """Kind of like Python 3's subprocess.run, but works in Python 2.

  The contents of stdout and stderr are captured. If the command
  succeeds (exit code 0), they are not printed. If the command fails,
  stderr is printed along with the provided error message (if any).

  Args:
    command: The command to be executed, as a list of strings
    input: A bytestring to use as stdin, or None.
    error_message: If not None, will be logged on failure.

  Returns:
    A `SubprocessResult` including the process's exit code.
  """
  process = subprocess.Popen(
      command,
      stdin=subprocess.PIPE,
      stdout=subprocess.PIPE,
      stderr=subprocess.PIPE)
  (stdout, stderr) = process.communicate(input=input)
  exit_code = process.wait()
  if exit_code != 0 and error_message is not None:
    if error_message is not None:
      sys.stderr.write("bazelisk: {}\n".format(error_message))
    write_binary_to_stderr(stderr)
  return SubprocessResult(exit_code=exit_code)


def write_binary_to_stderr(bytestring):
  # Python 2 compatibility hack. In Python 3, you can't write byte
  # strings to stdio; instead, you have to use the `sys.stderr.buffer`
  # attribute, which is not available in Python 2.
  buffer = getattr(sys.stderr, "buffer", sys.stderr)
  buffer.write(bytestring)


def verify_authenticity(binary_path, signature_path):
  """Authenticate a binary and signature against the Bazel public key.

  This will use a fresh temporary keyring populated only with the
  Bazel team's signing key; it is independent of any existing PGP data
  or settings that the user may have.

  Args:
    binary_path: File path to the Bazel binary to be executed.
    signature_path: File path to the detached signature made by the
      Bazel release PGP key to sign the provided binary.

  Returns:
    True if the binary is valid or gpg is not installed; False if gpg is
    installed but we cannot determine that the binary is valid.
  """
  if subprocess_run(
      ["gpg", "--batch", "--version"],
      error_message=
      "Warning: skipping authenticity check because GPG is not installed.",
  ).exit_code != 0:
    return True

  tempdir = tempfile.mkdtemp(prefix="tmp_bazelisk_gpg_")
  try:
    gpg_invocation = [
        "gpg",
        "--batch",
        "--no-default-keyring",
        "--homedir",
        tempdir,
    ]

    # DO NOT SUBMIT: Debugging on Windows and macOS...
    print("tempdir: {}\n".format(tempdir))
    print("gpg version:\n")
    subprocess.call(gpg_invocation + ["--version"])
    print("gpg location:\n")
    subprocess.call(["which", "gpg"])

    if subprocess_run(
        gpg_invocation + ["--import-ownertrust"],
        input=BAZEL_ULTIMATE_OWNERTRUST,
        error_message="Failed to initialize GPG keyring").exit_code != 0:
      return False
    if subprocess_run(
        gpg_invocation + ["--import"],
        input=BAZEL_PUBLIC_KEY,
        error_message="Failed to import Bazel public key").exit_code != 0:
      return False
    if subprocess_run(
        gpg_invocation + ["--verify", signature_path, binary_path],
        error_message="Failed to authenticate binary!").exit_code != 0:
      return False
    sys.stderr.write("Verified authenticity.\n")
    return True

  finally:
    shutil.rmtree(tempdir)


DownloadUrls = collections.namedtuple("DownloadUrls",
                                      ("binary_url", "signature_url"))


def determine_urls(version, bazel_filename):
  # Split version into base version and optional additional identifier.
  # Example: '0.19.1' -> ('0.19.1', None), '0.20.0rc1' -> ('0.20.0', 'rc1')
  (version, rc) = re.match(r'(\d*\.\d*(?:\.\d*)?)(rc\d)?', version).groups()
  binary_url = "https://releases.bazel.build/{}/{}/{}".format(
      version, rc if rc else "release", bazel_filename)
  signature_url = "{}.sig".format(binary_url)
  return DownloadUrls(binary_url=binary_url, signature_url=signature_url)


def download_file(url, destination_path):
  """Download a file from the given URL, saving it to the given path."""
  sys.stderr.write("Downloading {}...\n".format(url))
  with closing(urlopen(url)) as response:
    with open(destination_path, 'wb') as out_file:
      shutil.copyfileobj(response, out_file)


def download_bazel_into_directory(version, directory):
  """Download and authenticate the specified version of Bazel.

  If the binary already exists, it will not be re-downloaded.

  If the binary does not exist, it and its signature will be downloaded.
  The binary will only be saved and made executable if the signature is
  valid (or if we are unable to validate the signature because GPG is
  not installed).

  If the signature is invalid, a `SystemExit` exception will be raised.

  Returns:
    The path to the valid, executable Bazel binary within the provided
    directory.
  """
  bazel_filename = determine_bazel_filename(version)
  urls = determine_urls(version, bazel_filename)
  binary_path = os.path.join(directory, bazel_filename)
  if not os.path.exists(binary_path):
    untrusted_binary_path = "{}.untrusted".format(binary_path)
    signature_path = "{}.sig".format(binary_path)
    download_file(urls.binary_url, untrusted_binary_path)
    download_file(urls.signature_url, signature_path)
    if verify_authenticity(untrusted_binary_path, signature_path):
      os.rename(untrusted_binary_path, binary_path)
    else:
      os.unlink(untrusted_binary_path)
      raise SystemExit(AUTHENTICATION_FAILURE_EXIT_CODE)
  os.chmod(binary_path, 0o755)
  return binary_path


def maybe_makedirs(path):
  """
  Creates a directory and its parents if necessary.
  """
  try:
    os.makedirs(path)
  except OSError as e:
    if not os.path.isdir(path):
      raise e


def execute_bazel(bazel_path, argv):
  # We cannot use close_fds on Windows, so disable it there.
  p = subprocess.Popen([bazel_path] + argv, close_fds=os.name != 'nt')
  while True:
    try:
      return p.wait()
    except KeyboardInterrupt:
      # Bazel will also get the signal and terminate.
      # We should continue waiting until it does so.
      pass


def main(argv=None):
  if argv is None:
    argv = sys.argv

  bazelisk_directory = os.environ.get(
      "BAZELISK_HOME", os.path.join(os.path.expanduser('~'), '.bazelisk'))
  maybe_makedirs(bazelisk_directory)

  bazel_version = decide_which_bazel_version_to_use()
  bazel_version = resolve_version_label_to_number(bazelisk_directory,
                                                  bazel_version)

  bazel_directory = os.path.join(bazelisk_directory, 'bin')
  maybe_makedirs(bazel_directory)
  bazel_path = download_bazel_into_directory(bazel_version, bazel_directory)

  return execute_bazel(bazel_path, argv[1:])


BAZEL_ULTIMATE_OWNERTRUST = b"71A1D0EFCFEB6281FD0437C93D5919B448457EE0:6:\n"

BAZEL_PUBLIC_KEY = b"""\
-----BEGIN PGP PUBLIC KEY BLOCK-----

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
n5VZt+ds9MenAEZScDuR4Usd
=j+Xa
-----END PGP PUBLIC KEY BLOCK-----
"""

if __name__ == '__main__':
  sys.exit(main())
