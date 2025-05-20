load("@aspect_rules_js//npm:defs.bzl", "npm_package", "stamped_package_json")
load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")
load("@rules_shell//shell:sh_test.bzl", "sh_test")
load(":defs.bzl", "bazelisk_go_binaries")

# gazelle:ignore
# gazelle:prefix github.com/bazelbuild/bazelisk
gazelle(name = "gazelle")

sh_test(
    name = "py_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    args = ["PY"],
    data = [
        "bazelisk.py",
        "releases_for_tests.json",
    ],
    deps = ["@bazel_tools//tools/bash/runfiles"],
)

sh_test(
    name = "py3_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    args = ["PY3"],
    data = [
        "bazelisk.py",
        "releases_for_tests.json",
    ],
    deps = ["@bazel_tools//tools/bash/runfiles"],
)

sh_test(
    name = "go_bazelisk_test",
    size = "large",
    srcs = ["bazelisk_test.sh"],
    args = ["GO"],
    data = [
        "releases_for_tests.json",
        ":bazelisk",
    ],
    deps = ["@bazel_tools//tools/bash/runfiles"],
)

go_library(
    name = "bazelisk_lib",
    srcs = ["bazelisk.go"],
    importpath = "github.com/bazelbuild/bazelisk",
    visibility = ["//visibility:private"],
    deps = [
        "//core",
        "//repositories",
    ],
)

go_test(
    name = "bazelisk_version_test",
    srcs = ["bazelisk_version_test.go"],
    data = [
        "sample-issues-migration.json",
    ],
    embed = [":bazelisk_lib"],
    importpath = "github.com/bazelbuild/bazelisk",
    deps = [
        "//config",
        "//core",
        "//httputil",
        "//repositories",
        "//versions",
    ],
)

go_binary(
    name = "bazelisk",
    embed = [":bazelisk_lib"],
    visibility = ["//visibility:public"],
)

bazelisk_go_binaries()

genrule(
    name = "bazelisk-darwin-universal",
    srcs = [
        ":bazelisk-darwin-amd64",
        ":bazelisk-darwin-arm64",
    ],
    outs = ["bazelisk-darwin_universal"],
    cmd = "lipo -create -output \"$@\" $(SRCS)",
    output_to_bindir = 1,
    target_compatible_with = [
        "@platforms//os:macos",
    ],
)

stamped_package_json(
    name = "package",
    # This key is defined by /stamp.sh
    stamp_var = "BUILD_SCM_VERSION",
)

npm_package(
    name = "npm_package",
    srcs = [
        "LICENSE",
        "README.md",
        "bazelisk.d.ts",
        "bazelisk.js",
        ":bazelisk-darwin-amd64",
        ":bazelisk-darwin-arm64",
        ":bazelisk-linux-amd64",
        ":bazelisk-linux-arm64",
        ":bazelisk-windows-amd64",
        ":bazelisk-windows-arm64",
        ":package",
    ],
    package = "@bazel/bazelisk",
    publishable = True,
)
