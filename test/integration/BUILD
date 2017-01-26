load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["driver.go"],
    visibility = ["//visibility:private"],
    deps = ["@com_github_spf13_pflag//:go_default_library"],
)

go_binary(
    name = "integration",
    data = [
        "http-service.yaml.tmpl",
        "manager.yaml.tmpl",
        "//docker:app",
        "//docker:runtime",
    ],
    library = ":go_default_library",
    tags = [":manual"],
    visibility = ["//visibility:public"],
)
