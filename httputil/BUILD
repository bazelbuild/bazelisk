load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

# gazelle:prefix github.com/bazelbuild/bazelisk/httputil
gazelle(name = "gazelle")

go_library(
    name = "httputil",
    srcs = [
        "fake.go",
        "httputil.go",
    ],
    embedsrcs = [
        "bazel_key.pub.gpg",
    ],
    importpath = "github.com/bazelbuild/bazelisk/httputil",
    visibility = ["//visibility:public"],
    deps = [
        "//config",
        "//httputil/progress",
        "@com_github_bgentry_go_netrc//netrc",
        "@com_github_mitchellh_go_homedir//:go-homedir",
        "@com_github_protonmail_gopenpgp_v3//crypto",
    ],
)

go_test(
    name = "httputil_test",
    srcs = [
        "httputil_test.go",
    ],
    embed = [":httputil"],
    deps = [
        "//httputil/httputil_test_helper",
    ],
)
