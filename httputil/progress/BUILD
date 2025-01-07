load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

# gazelle:prefix github.com/bazelbuild/bazelisk/httputil/progress
gazelle(name = "gazelle")

go_library(
    name = "progress",
    srcs = ["progress.go"],
    importpath = "github.com/bazelbuild/bazelisk/httputil/progress",
    visibility = ["//visibility:public"],
    deps = [
        "//config",
        "@org_golang_x_term//:term",
    ],
)

go_test(
    name = "progress_test",
    srcs = ["progress_test.go"],
    embed = [":progress"],
)
