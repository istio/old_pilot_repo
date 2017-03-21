load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "inject.go",
        "main.go",
    ],
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_ghodss_yaml//:go_default_library",
        "@com_github_spf13_cobra//:go_default_library",
        "@io_k8s_client_go//pkg/api/v1:go_default_library",
        "@io_k8s_client_go//pkg/apis/extensions/v1beta1:go_default_library",
        "@io_k8s_client_go//pkg/apis/meta/v1:go_default_library",
        "@io_k8s_client_go//pkg/util/yaml:go_default_library",
    ],
)

go_binary(
    name = "istioutil",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
