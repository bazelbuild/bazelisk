py_binary(
    name = "bazelisk",
    srcs = ["bazelisk.py"],
)

sh_test(
    name = "bazelisk_test",
    srcs = ["bazelisk_test.sh"],
    data = [":bazelisk"],
    deps = ["@bazel_tools//tools/bash/runfiles"],
)
