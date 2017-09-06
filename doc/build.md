# Build instructions



## Environment
We build on Linux. For Windows and Mac, we recommend using a Linux virtual machine and [minikube build pod](minikube.md). Go code compiles on Mac but docker and proxy tests will fail.

## Prerequisites

* [Setting up Go](https://github.com/istio/istio/blob/master/devel/README.md#setting-up-go)
* [Setting up Bazel](https://github.com/istio/istio/blob/master/devel/README.md#setting-up-bazel) - the main build system of Istio. The minimal version for Istio Pilot is [0.5.2](https://github.com/bazelbuild/bazel/releases/tag/0.5.2).

## Initial Setup

1. [Create a forked repository](https://github.com/istio/istio/blob/master/devel/README.md#fork-the-main-repository) in `$GOPATH/src/istio.io`.
2. Run `make setup`. It well install the required tools and will [vendorize](https://golang.org/cmd/go/#hdr-Vendor_Directories) the dependencies.

## Workflow

After the prerequisites are installed and initial setup is performed, run the following commands:

* `make build` to build the project
* `make fmt` to format the code
* `make test` to run the tests
* `make lint` to run lint checks. If the checks fail, your PR will not be accepted.
* `make gazelle` to fix the build if you add a new source file or change the imports

_Note:_ Data dependencies such as the ones used by tests require manual declaration in
the `BUILD` files.

See also [Istio Development](https://github.com/istio/istio/blob/master/devel/README.md) for setup and general tips.
