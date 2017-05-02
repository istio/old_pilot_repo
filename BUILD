load("@io_bazel_rules_go//go:def.bzl", "go_prefix")

go_prefix("istio.io/manager")

go_library(
    name = "go_default_library",
    srcs = ["config.pb.go"],
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_golang_protobuf//proto:go_default_library",
        "@com_github_golang_protobuf//ptypes/any:go_default_library",
        "@com_github_golang_protobuf//ptypes/duration:go_default_library",
        "@com_github_golang_protobuf//ptypes/wrappers:go_default_library",
    ],
)
