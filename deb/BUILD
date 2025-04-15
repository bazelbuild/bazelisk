load("@rules_pkg//pkg:deb.bzl", "pkg_deb")
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

ARCHES = [
    "amd64",
    "arm64",
]

[
    pkg_tar(
        name = "bazelisk-{}_tar".format(arch),
        srcs = ["//:bazelisk-linux-{}".format(arch)],
        out = "bazelisk-{}.tar".format(arch),
        package_dir = "/usr/bin",
        remap_paths = {"bazelisk-linux_{}".format(arch): "bazelisk"},
        stamp = -1,
        symlinks = {"bazel": "bazelisk"},
    )
    for arch in ARCHES
]

[
    pkg_deb(
        name = "bazelisk-{}_deb".format(arch),
        out = "bazelisk-{}.deb".format(arch),
        architecture = arch,
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
