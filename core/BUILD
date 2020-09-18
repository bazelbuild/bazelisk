load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "core",
    srcs = [
        "core.go",
        "repositories.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/core",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_hashicorp_go_version//:go_default_library",
        "@com_github_mitchellh_go_homedir//:go_default_library",
        "//httputil",
        "//platforms",
        "//repositories",
        "//versions",
    ],
)