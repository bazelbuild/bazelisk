load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "core",
    srcs = ["repositories.go"],
    importpath = "github.com/bazelbuild/bazelisk/core",
    visibility = ["//visibility:public"],
    deps = [
        "//httputil",
        "//platforms",
        "//versions",
    ],
)