#!/usr/bin/env python3
import os
import unittest
import subprocess
import tempfile

import bazelisk


def execute_bazel(bazel_path, argv):
  return subprocess.check_output(
      [bazel_path] + argv, close_fds=os.name != 'nt',
      stderr=subprocess.STDOUT).decode("utf-8")


class TestBazelisk(unittest.TestCase):
  """Integration tests for Bazelisk."""

  def setUp(self):
    # Override Bazelisk's default function to execute Bazel, so that we can grab the output.
    bazelisk.execute_bazel = execute_bazel
    os.environ["BAZELISK_HOME"] = tempfile.mkdtemp(
        dir=os.environ["TEST_TMPDIR"])

  def test_bazel_version(self):
    output = bazelisk.main(["bazelisk", "version"])
    self.assertTrue("Build label" in output)

  def test_bazel_version_from_environment(self):
    os.environ["USE_BAZEL_VERSION"] = "0.20.0"
    try:
      output = bazelisk.main(["bazelisk", "version"])
    finally:
      del os.environ["USE_BAZEL_VERSION"]
    self.assertTrue("Build label: 0.20.0" in output)

  def test_bazel_version_from_file(self):
    with open("WORKSPACE", "w") as f:
      pass
    with open(".bazelversion", "w") as f:
      f.write("0.19.0")
    try:
      output = bazelisk.main(["bazelisk", "version"])
    finally:
      os.unlink("WORKSPACE")
      os.unlink(".bazelversion")
    self.assertTrue("Build label: 0.19.0" in output)


if __name__ == "__main__":
  unittest.main()
