load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "core",
    srcs = [
        "core.go",
        "repositories.go",
    ],
    importpath = "github.com/bazelbuild/bazelisk/core",
    visibility = ["//visibility:public"],
    x_defs = {"BazeliskVersion": "{STABLE_VERSION}"},
    deps = [
        "//config",
        "//httputil",
        "//platforms",
        "//versions",
        "//ws",
        "@com_github_gofrs_flock//:flock",
        "@com_github_mitchellh_go_homedir//:go-homedir",
    ],
)

go_test(
    name = "core_test",
    srcs = [
        "core_test.go",
        "repositories_test.go",
    ],
    embed = [":core"],
    deps = [
        "//config",
        "//platforms",
    ],
)
