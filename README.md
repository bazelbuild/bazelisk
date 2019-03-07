# Bazelisk

**A user-friendly launcher for Bazel.**

## News

- 2018-01-20: I rewrote Bazelisk in Go. It has the same features as the Python version and both versions are tested against the same integration test suite. This version might be easier to use on Windows, because it can be compiled to a native executable that has no other dependencies.

## About Bazelisk

Bazelisk is a wrapper for Bazel. It automatically picks a good version of Bazel given your current working directory, downloads it from the official server (if required) and then transparently passes through all command-line arguments to the real Bazel binary. You can call it just like you would call Bazel.

Bazelisk is currently not an official part of Bazel and is not tested or code reviewed as thoroughly as Bazel itself. It's a personal project that @philwo (a core contributor to Bazel) wrote in his free time. If users like it, we might merge it into the bazelbuild organization and make it an official tool.

Some ideas how to use it:
- Install it as the `bazel` binary in your PATH (e.g. /usr/local/bin). Never worry about upgrading Bazel to the latest version again.
- Check it into your repository and recommend users to build your software via `./bazelisk.py build //my:software`. That way, even someone who has never used Bazel or doesn't have it installed can build your software.
- As a company using Bazel or as a project owner, add a `.bazelversion` file to your repository. This will tell Bazelisk to use the exact version specified in the file when running in your workspace. The fact that it's versioned inside your repository will then allow for atomic upgrades of Bazel including all necessary changes. If you install Bazelisk as `bazel` on your CI machines, too, you can even test Bazel upgrades via a normal presubmit / pull request. It will also ensure that users will not try to build your project with an incompatible version of Bazel, which is often a cause for frustration and failing builds.

## How does Bazelisk know which version to run?

It uses a simple algorithm:
- If the environment variable `USE_BAZEL_VERSION` is set, it will use the version specified in the value.
- Otherwise, if a `.bazelversion` file exists in the current directory or recursively any parent directory, it will read the file and use the the version specified in it.
- Otherwise it will check GitHub for the latest version of Bazel, cache the result for an hour and use that version.

Bazelisk currently understands the following formats for version labels:
- `latest` means the latest stable version of Bazel as released on GitHub. Previous
  releases can be specified via `latest-1`, `latest-2` etc.
- A version number like `0.17.2` means that exact version of Bazel. It can also
  be a release candidate version like `0.20.0rc3`.
- `last_green` refers to the Bazel binary that was built at the most recent commit that passed [Bazel CI](https://buildkite.com/bazel/bazel-bazel). Ideally this binary should be very close to Bazel-at-head.


In the future I will add support for release candidates and for building Bazel from source at a given commit.

## Other features

The Go version of Bazelisk offers two new flags.

`--strict` expands to the set of incompatible flags which may be enabled for the
given version of Bazel.

```shell
bazelisk --strict build //...
```

`--migrate` will run Bazel multiple times to help you identify compatibility
issues. If the code fails with `--strict`, the flag `--migrate` will run Bazel
with each one of the flag separately, and print a report at the end. This will
show you which flags can safely enabled, and which flags require a migration.

## Requirements

For ease of use, the Python version of Bazelisk is written to work with Python 2.7 and 3.x and only uses modules provided by the standard library.

The Go version can be compiled to run natively on Linux, macOS and Windows.

To install the Go version, type:

```shell
go get github.com/philwo/bazelisk
```

To add it to your PATH:

```shell
export PATH=$PATH:$(go env GOPATH)/bin
```

For more information, you may read about the [`GOPATH` environment
variable](https://github.com/golang/go/wiki/SettingGOPATH).

## Ideas for the future

- Add a Homebrew recipe for Bazelisk to make it easy to install on macOS.
- Add support for checked-in Bazel binaries.
- When the version label is set to a commit hash, first download a matching binary version of Bazel, then build Bazel automatically at that commit and use the resulting binary.
- Add support to automatically bisect a build failure to a culprit commit in Bazel. If you notice that you could successfully build your project using version X, but not using version X+1, then Bazelisk should be able to figure out the commit that caused the breakage and the Bazel team can easily fix the problem.

## FAQ

### Where does Bazelisk store the downloaded versions of Bazel?
It creates a directory called "bazelisk" inside your [user cache directory](https://golang.org/pkg/os/#UserCacheDir) and will store them there. Feel free to delete this directory at any time, as it can be regenerated automatically when required.
