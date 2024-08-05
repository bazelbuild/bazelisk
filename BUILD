load("@aspect_rules_js//npm:defs.bzl", "npm_package", "stamped_package_json")
load("@gazelle//:def.bzl", "gazelle")
load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

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

go_binary(
    name = "bazelisk-darwin-amd64",
    out = "bazelisk-darwin_amd64",
    embed = [":bazelisk_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "amd64",
    goos = "darwin",
    pure = "on",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bazelisk-darwin-arm64",
    out = "bazelisk-darwin_arm64",
    embed = [":bazelisk_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "arm64",
    goos = "darwin",
    pure = "on",
    visibility = ["//visibility:public"],
)

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

go_binary(
    name = "bazelisk-linux-amd64",
    out = "bazelisk-linux_amd64",
    embed = [":bazelisk_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "amd64",
    goos = "linux",
    pure = "on",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bazelisk-linux-arm64",
    out = "bazelisk-linux_arm64",
    embed = [":bazelisk_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "arm64",
    goos = "linux",
    pure = "on",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bazelisk-windows-amd64",
    out = "bazelisk-windows_amd64.exe",
    embed = [":bazelisk_lib"],
    goarch = "amd64",
    goos = "windows",
    pure = "on",
    visibility = ["//visibility:public"],
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
        ":package",
    ],
    package = "@bazel/bazelisk",
)
