load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "config",
    srcs = ["config.go"],
    importpath = "github.com/bazelbuild/bazelisk/config",
    visibility = ["//visibility:public"],
    deps = ["//ws"],
)

go_test(
    name = "config_test",
    srcs = ["config_test.go"],
    embed = [":config"],
)
