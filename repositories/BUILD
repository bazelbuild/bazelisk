load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "repositories",
    srcs = [
        "gcs.go",
        "github.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/repositories",
    visibility = ["//visibility:public"],
    deps = [
        "//config",
        "//core",
        "//httputil",
        "//platforms",
        "//versions",
    ],
)
