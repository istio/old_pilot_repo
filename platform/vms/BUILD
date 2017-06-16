load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "controller.go",
        "conversion.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
	"//model:go_default_library",
	"@com_github_golang_protobuf//proto:go_default_library",
        "@com_github_amalgam8_amalgam8//pkg/adapters/discovery/amalgam8:go_default_library",
        "@com_github_amalgam8_amalgam8//pkg/api:go_default_library",
        "@com_github_amalgam8_amalgam8//registry/client:go_default_library",
        "@com_github_amalgam8_amalgam8//registry/client/test/model:go_default_library",
        "@io_istio_api//:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["controller_test.go"],
    library = ":go_default_library",
    deps = [
        "@com_github_amalgam8_amalgam8//registry/client:go_default_library",
        "@com_github_amalgam8_amalgam8//registry/client/test/model:go_default_library",
        "@com_github_stretchr_testify//assert:go_default_library",
    ],
)
