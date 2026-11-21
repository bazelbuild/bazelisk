load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@rules_pkg//pkg:deb.bzl", "pkg_deb")
load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")
load("@rules_pkg//pkg:tar.bzl", "pkg_tar")

config_setting(
    name = "stamp_detect",
    values = {"stamp": "1"},
)

genrule(
    name = "parse_version",
    outs = ["version.txt"],
    cmd_bash = select({
        ":stamp_detect": "sed -ne 's#^STABLE_VERSION v##p' bazel-out/stable-status.txt > $@",
        "//conditions:default": "echo 0.0.0 > $@",
    }),
    cmd_bat = "echo 0.0.0 >> $@",
    stamp = True,
)

run_binary(
    name = "bazelisk_completion_bash",
    args = [
        "completion",
        "bash",
    ],
    env = {
        "HOME": "/tmp",
        "USE_BAZEL_VERSION": "9.2.0",
    },
    silent_on_success = True,
    stdout = "completion.bash",
    tool = "//:bazelisk",
    tags = ["manual"],
)

pkg_files(
    name = "bash_completion",
    srcs = ["completion.bash"],
    attributes = pkg_attributes(mode = "0644"),
    prefix = "/etc/bash_completion.d",
    renames = {"completion.bash": "bazel"},
    tags = ["manual"],
)

ARCHES = [
    "amd64",
    "arm64",
]

[
    pkg_tar(
        name = "bazelisk-{}_tar".format(arch),
        srcs = [
            "//:bazelisk-linux-{}".format(arch),
        ] + select({
            "@platforms//os:windows": [],
            "//conditions:default": [":bash_completion"],
        }),
        out = "bazelisk-{}.tar".format(arch),
        remap_paths = {"bazelisk-linux_{}".format(arch): "/usr/bin/bazelisk"},
        stamp = -1,
        symlinks = {"/usr/bin/bazel": "bazelisk"},
    )
    for arch in ARCHES
]

[
    pkg_deb(
        name = "bazelisk-{}_deb".format(arch),
        out = "bazelisk-{}.deb".format(arch),
        architecture = arch,
        conffiles = ["/etc/bash_completion.d/bazel"],
        conflicts = [
            "bazel",
            "bazel-bootstrap",
        ],
        data = ":bazelisk-{}_tar".format(arch),
        depends = ["ca-certificates"],
        description_file = ":description.txt",
        homepage = "https://github.com/bazelbuild/bazelisk",
        license = "Apache-2.0",
        maintainer = "The Bazel Authors <bazel-dev@googlegroups.com>",
        package = "bazelisk",
        section = "contrib/devel",
        version_file = ":version.txt",
        visibility = ["//visibility:public"],
    )
    for arch in ARCHES
]

# TODO: cross-platform implementation is required!
[
    genrule(
        name = "bazelisk-{}-deb-sha256".format(arch),
        srcs = [":bazelisk-{}.deb".format(arch)],
        outs = ["bazelisk-{}.deb.sha256".format(arch)],
        cmd = "$(execpath //sha256sum) $< > $@",
        tools = ["//sha256sum"],
        visibility = ["//visibility:public"],
    )
    for arch in ARCHES
]
