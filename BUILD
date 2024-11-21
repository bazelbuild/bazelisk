load("@aspect_rules_js//npm:defs.bzl", "npm_package", "stamped_package_json")
load("@gazelle//:def.bzl", "gazelle")
load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

# gazelle:ignore
# gazelle:prefix github.com/bazelbuild/bazelisk
gazelle(name = "gazelle")

sh_test(
    name = "py_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    args = ["PY"],
    data = [
        "bazelisk.py",
        "releases_for_tests.json",
    ],
    deps = ["@bazel_tools//tools/bash/runfiles"],
)

sh_test(
    name = "py3_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    args = ["PY3"],
    data = [
        "bazelisk.py",
        "releases_for_tests.json",
    ],
    deps = ["@bazel_tools//tools/bash/runfiles"],
)

sh_test(
    name = "go_bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    args = ["GO"],
    data = [
        "releases_for_tests.json",
        ":bazelisk",
    ],
    deps = ["@bazel_tools//tools/bash/runfiles"],
    size = "large",
)

go_library(
    name = "bazelisk_lib",
    srcs = ["bazelisk.go"],
    importpath = "github.com/bazelbuild/bazelisk",
    visibility = ["//visibility:private"],
    deps = [
        "//core",
        "//repositories",
    ],
)

go_test(
    name = "bazelisk_version_test",
    srcs = ["bazelisk_version_test.go"],
    data = [
        "sample-issues-migration.json",
    ],
    embed = [":bazelisk_lib"],
    importpath = "github.com/bazelbuild/bazelisk",
    deps = [
        "//config",
        "//core",
        "//httputil",
        "//repositories",
        "//versions",
    ],
)

go_binary(
    name = "bazelisk",
    embed = [":bazelisk_lib"],
    visibility = ["//visibility:public"],
)

[
    go_binary(
        name = "bazelisk-%s-%s" % (os, arch),
        out = "bazelisk-%s_%s" % (os, arch),
        embed = [":bazelisk_lib"],
        gc_linkopts = [
            "-s",
            "-w",
        ],
        goarch = arch,
        goos = os,
        pure = "on",
        visibility = ["//visibility:public"],
    )
    for os, arch in [
        ("darwin", "amd64"),
        ("darwin", "arm64"),
        ("linux", "amd64"),
        ("linux", "arm64")
    ]
]

[
    go_binary(
        name = "bazelisk-%s-%s" % (os, arch),
        out = "bazelisk-%s_%s.exe" % (os, arch),
        embed = [":bazelisk_lib"],
        goarch = arch,
        goos = os,
        pure = "on",
        visibility = ["//visibility:public"],
    )
    for os, arch in [
        ("windows", "amd64"),
        ("windows", "arm64")
    ]
]

genrule(
    name = "bazelisk-darwin-universal",
    srcs = [
        ":bazelisk-darwin-amd64",
        ":bazelisk-darwin-arm64",
    ],
    outs = ["bazelisk-darwin_universal"],
    cmd = "lipo -create -output \"$@\" $(SRCS)",
    output_to_bindir = 1,
    target_compatible_with = [
        "@platforms//os:macos",
    ],
)

stamped_package_json(
    name = "package",
    # This key is defined by /stamp.sh
    stamp_var = "BUILD_SCM_VERSION",
)

npm_package(
    name = "npm_package",
    srcs = [
        "LICENSE",
        "README.md",
        "bazelisk.d.ts",
        "bazelisk.js",
        ":bazelisk-darwin-amd64",
        ":bazelisk-darwin-arm64",
        ":bazelisk-linux-amd64",
        ":bazelisk-linux-arm64",
        ":bazelisk-windows-amd64",
        ":bazelisk-windows-arm64",
        ":package",
    ],
    package = "@bazel/bazelisk",
    publishable = True,
)
