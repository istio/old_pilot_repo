load("@io_bazel_rules_go//proto:go_proto_library.bzl", "go_proto_library")

go_proto_library(
    name = "go_default_library",
    srcs = ["@io_istio_api//:mixer"],
    has_services = 1,
    visibility = ["//visibility:public"],
)

