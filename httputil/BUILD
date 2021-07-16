load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

# gazelle:prefix github.com/bazelbuild/bazelisk/httputil
gazelle(name = "gazelle")

go_library(
    name = "go_default_library",
    srcs = [
        "fake.go",
        "httputil.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/httputil",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["httputil_test.go"],
    embed = [":go_default_library"],
)
