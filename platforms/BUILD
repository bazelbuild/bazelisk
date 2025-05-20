load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "platforms",
    srcs = ["platforms.go"],
    importpath = "github.com/bazelbuild/bazelisk/platforms",
    visibility = ["//visibility:public"],
    deps = [
        "//config",
        "//versions",
        "@com_github_hashicorp_go_version//:go-version",
    ],
)

go_test(
    name = "platforms_test",
    srcs = ["platforms_test.go"],
    embed = [":platforms"],
)
