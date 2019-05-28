load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@bazel_gazelle//:def.bzl", "gazelle")

# gazelle:prefix github.com/philwo/bazelisk
gazelle(name = "gazelle")

sh_test(
    name = "py_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    data = ["bazelisk.py", "releases_for_tests.json"],
    deps = ["@bazel_tools//tools/bash/runfiles"],
    args = ["PY"]
)

sh_test(
    name = "go_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    data = [":bazelisk", "releases_for_tests.json"],
    deps = ["@bazel_tools//tools/bash/runfiles"],
    args = ["GO"]
)

go_library(
    name = "go_default_library",
    srcs = ["bazelisk.go"],
    importpath = "github.com/philwo/bazelisk",
    visibility = ["//visibility:private"],
    deps = ["@com_github_hashicorp_go_version//:go_default_library"],
)

go_binary(
    name = "bazelisk",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)
