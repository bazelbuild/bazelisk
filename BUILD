load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")
load("@bazel_gazelle//:def.bzl", "gazelle")
load("@build_bazel_rules_nodejs//:index.bzl", "pkg_npm")

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
    name = "go_default_library",
    srcs = ["bazelisk.go"],
    importpath = "github.com/bazelbuild/bazelisk",
    visibility = ["//visibility:private"],
    deps = [
        "//core:go_default_library",
        "//repositories:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["bazelisk_version_test.go"],
    data = [
        "sample-issues-migration.json",
    ],
    embed = [":go_default_library"],
    importpath = "github.com/bazelbuild/bazelisk",
    deps = [
        "//core:go_default_library",
        "//httputil:go_default_library",
        "//repositories:go_default_library",
        "//versions:go_default_library",
    ],
)

go_binary(
    name = "bazelisk",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bazelisk-darwin-amd64",
    out = "bazelisk-darwin_amd64",
    embed = [":go_default_library"],
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
    embed = [":go_default_library"],
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
    embed = [":go_default_library"],
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
    embed = [":go_default_library"],
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
    embed = [":go_default_library"],
    goarch = "amd64",
    goos = "windows",
    pure = "on",
    visibility = ["//visibility:public"],
)

pkg_npm(
    name = "npm_package",
    package_name = "@bazel/bazelisk",
    srcs = [
        "LICENSE",
        "README.md",
        "bazelisk.d.ts",
        "bazelisk.js",
        "package.json",
    ],
    deps = [
        ":bazelisk-darwin-amd64",
        ":bazelisk-darwin-arm64",
        ":bazelisk-linux-amd64",
        ":bazelisk-linux-arm64",
        ":bazelisk-windows-amd64",
    ],
)
