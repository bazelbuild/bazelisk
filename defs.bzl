load("@io_bazel_rules_go//go:def.bzl", "go_binary")

def bazelisk_go_binaries():
    for os in ("linux", "darwin", "windows"):
        ext = ".exe" if os == "windows" else ""

        # Don't strip debugging symbols on Windows, as it makes binaries more
        # likely to be flagged as malware.
        gc_linkopts = [] if os == "windows" else ["-s", "-w"]

        for arch in ("amd64", "arm64"):
            go_binary(
                name = "bazelisk-%s-%s" % (os, arch),
                out = "bazelisk-%s_%s%s" % (os, arch, ext),
                embed = [":bazelisk_lib"],
                gc_linkopts = gc_linkopts,
                goarch = arch,
                goos = os,
                pure = "on",
                visibility = ["//visibility:public"],
            )
