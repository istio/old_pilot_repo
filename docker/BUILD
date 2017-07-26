load("@bazel_tools//tools/build_defs/pkg:pkg.bzl", "pkg_tar")
load("@io_bazel_rules_docker//docker:docker.bzl", "docker_build")
load("//docker:debug_docker.bzl", "debug_docker_build")

pkg_tar(
    name = "app_tar",
    extension = "tar.gz",
    files = [
        "//test/client",
        "//test/server",
    ],
    mode = "0755",
    package_dir = "/usr/local/bin",
)

pkg_tar(
    name = "prepare_proxy_tar",
    extension = "tar.gz",
    files = ["prepare_proxy.sh"],
    mode = "0755",
    package_dir = "/usr/local/bin/",
)

docker_build(
    name = "pilot",
    entrypoint = ["/usr/local/bin/pilot-discovery"],
    repository = "istio",
    tars = ["//cmd/pilot-discovery:pilot-discovery-tar"],
)

docker_build(
    name = "app",
    entrypoint = ["/usr/local/bin/server"],
    repository = "istio",
    tars = [":app_tar"],
)

## Marking these rules as manual as the base image is large

docker_build(
    name = "proxy_init",
    base = "@istio_iptables//image",
    entrypoint = ["/usr/local/bin/prepare_proxy.sh"],
    repository = "istio",
    tags = ["manual"],
    tars = [":prepare_proxy_tar"],
)

debug_docker_build(
    entrypoint = ["/usr/local/bin/pilot-agent"],
    images = [
        {
            "name": "proxy",
            "base": "@istio_proxy//image",
        },
        {
            "name": "proxy_debug",
            "base": "@istio_proxy_debug//image",
        },
    ],
    repository = "istio",
    tags = ["manual"],
    tars = ["//cmd/pilot-agent:pilot-agent-tar"],
)
