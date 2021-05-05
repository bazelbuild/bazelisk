load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["httputil.go"],
    importpath = "github.com/bazelbuild/bazelisk/httputil",
    visibility = ["//visibility:public"],
    deps = ["@org_golang_x_crypto//openpgp:go_default_library"],
)
