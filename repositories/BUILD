load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "gcs.go",
        "github.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/repositories",
    visibility = ["//visibility:public"],
    deps = [
        "//httputil:go_default_library",
        "//platforms:go_default_library",
        "//versions:go_default_library",
    ],
)
