load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "versions",
    srcs = ["versions.go"],
    importpath = "github.com/bazelbuild/bazelisk/versions",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_hashicorp_go_version//:go-version",
    ],
)
