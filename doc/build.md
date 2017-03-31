# Build instructions for Linux

(For Windows and Mac, we recommend using a Linux virtual machine and/or [Vagrant-specific build instructions](build-vagrant.md); Go code compiles on Mac but docker and proxy tests will fail on Mac)

Install additional build dependencies before trying to build.

    bin/install-prereqs.sh

We are using [Bazel 0.4.4](https://github.com/bazelbuild/bazel/releases) as the main build system in Istio Manager. The following command builds all targets in Istio Manager:

    bazel build //...

Bazel uses `BUILD` files to keep track of dependencies between sources.  If you
add a new source file, change the imports, or add a data dependency, please run the following command
to update all `BUILD` files:

    gazelle -go_prefix "istio.io/manager" -mode fix -repo_root .

Install [Gazelle tool](https://github.com/bazelbuild/rules_go/tree/master/go/tools/gazelle) that automatically generates Bazel `BUILD` files as follows:

    go install github.com/bazelbuild/rules_go/go/tools/gazelle/gazelle

_Note_: Gazelle incorrectly rewrites Istio API dependency. You need to manually replace:

    @io_istio_api//proxy/v1/config:go_default_library

with:

    @io_istio_api//:go_default_library

## Go tooling compatibility

Istio Manager requires Go1.8+ toolchain.

Bazel build environment is compatible with the standard Golang tooling, except you need to vendorize all dependencies in Istio Manager. If you have successfully built with Bazel, run the following script to put dependencies fetched by Bazel into `vendor` directory:

    bin/init.sh

After running this command, you should be able to use all standard go tools:

    go generate istio.io/manager/...
    go build istio.io/manager/...
    go test -v istio.io/manager/...

_Note_: these commands assume you have placed the repository clone into `$GOPATH`.
