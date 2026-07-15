load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "ws",
    srcs = ["ws.go"],
    importpath = "github.com/bazelbuild/bazelisk/ws",
    visibility = ["//visibility:public"],
)

go_test(
    name = "ws_test",
    srcs = ["ws_test.go"],
    embed = [":ws"],
)
