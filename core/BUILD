load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "core.go",
        "repositories.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/core",
    visibility = ["//visibility:public"],
    x_defs = {"BazeliskVersion": "{STABLE_VERSION}"},
    deps = [
        "//httputil:go_default_library",
        "//platforms:go_default_library",
        "//versions:go_default_library",
        "@com_github_mitchellh_go_homedir//:go_default_library",
    ],
)
