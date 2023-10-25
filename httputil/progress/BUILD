load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

# gazelle:prefix github.com/bazelbuild/bazelisk/httputil/progress
gazelle(name = "gazelle")

go_library(
    name = "go_default_library",
    srcs = [
        "progress.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/httputil/progress",
    visibility = ["//visibility:public"],
    deps = [
        "//config:go_default_library",
        "@org_golang_x_term//:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "progress_test.go",
    ],
    embed = [":go_default_library"],
)
