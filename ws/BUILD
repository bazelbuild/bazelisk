load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "ws",
    srcs = ["ws.go"],
    importpath = "github.com/bazelbuild/bazelisk/ws",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_mitchellh_go_homedir//:go_default_library",
    ],
)
