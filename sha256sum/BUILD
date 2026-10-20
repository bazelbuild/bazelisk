load("@io_bazel_rules_go//go:def.bzl", "go_binary")

go_binary(
    name = "sha256sum",
    srcs = ["main.go"],
    gotags = ["bazelisk_internal_checksum"],
    visibility = ["//visibility:public"],
)
