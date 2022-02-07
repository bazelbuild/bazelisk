load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["platforms.go"],
    importpath = "github.com/bazelbuild/bazelisk/platforms",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_hashicorp_go_version//:go_default_library",
    ],
)
