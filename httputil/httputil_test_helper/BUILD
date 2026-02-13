load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "httputil_test_helper",
    testonly = True,
    srcs = [
        "httputil_test_helper.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/httputil/httputil_test_helper",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_protonmail_gopenpgp_v3//crypto",
    ],
)
