# Istio Manager #
[![Build Status](https://travis-ci.org/istio/manager.svg?branch=master)](https://travis-ci.org/istio/manager)
[![Go Report Card](https://goreportcard.com/badge/github.com/istio/manager)](https://goreportcard.com/report/github.com/istio/manager)

The Istio Manager is used to configure Istio and propagate configuration to the
other components of the system, including the Istio mixer and the Istio proxy mesh.

[Contributing to the project](./CONTRIBUTING.md)

## Filing issues ##

If you have a question about the Istio Manager or have a problem using it, please
[file an issue](https://github.com/istio/manager/issues/new).

## Setting up a local dev environment ##

_Note:_ This section applies to Mac and Windows users only.

### Pre-requisites ##

- Setup Go 1.7.5+ on your host machine
- Clone this repository
- Install [Virtualbox](https://github.com/kubernetes/minikube/releases)
- Install [Minikube](https://github.com/kubernetes/minikube/releases)
- Install [Vagrant](https://www.vagrantup.com/downloads.html)
- Install [kubectl](https://kubernetes.io/docs/user-guide/prereqs/)

### Start Minikube

    minikube config set kubernetes-version v1.5.2
    minikube start
    
Create a symlink to your kube config file in the platform/kube directory

    cp ~/.kube/config platform/kube/

### Start Vagrant VM for compiling the code

    vagrant up --provision

The `--provision` is needed for the first time installation of the VM only.
Your local repository clone will be mounted in the Vagrant VM under
`/home/ubuntu/go/src/istio.io/manager`.

### Build once in the VM

    bazel build //...
    ./bin/init.sh

### Use your favorite IDE on the host

You should now have vendor directories in the manager folder on the
host. You can use your favorite IDE on the host to develop, while using
standard `go` tools. In order to compile project, run the commands in the
the build instructions below inside the vagrant VM.

### Before you commit

Run the end to end integration tests in the VM

    ./bin/e2e.sh -h docker.io/<yourusername>

Note that this script will push some images to your dockerhub account.

## Build instructions for Linux##

We are using [Bazel 0.4.4](https://bazel.io) to build Istio Manager:

    bazel build //cmd/...

_Note the three dots_

<!-- _Note_: Due to issues with case-insensitive file systems, macOS is not -->
<!-- supported at the moment by Bazel Go rules. As a workaround, create a case sensitive partition -->
<!-- on your macOS as described [here](https://coderwall.com/p/mgi8ja/case-sensitive-git-in-mac-os-x-like-a-pro), and build using  -->

<!--     bazel --output_base=/Volumes/case-sensitive-volume-name/bazel-out build //cmd/... --spawn_strategy=standalone -->

Bazel uses `BUILD` files to keep track of dependencies between sources.  If you
add a new source file or change the imports, please run the following command
to update all `BUILD` files:

    gazelle -go_prefix "istio.io/manager" --mode fix -repo_root .

Gazelle binary is located in `bazel-bin/external` folder under the manager
repository, after your initial bazel build:

    bazel-bin/external/io_bazel_rules_go_repository_tools/bin/gazelle

_Note_: If you cant find the gazelle binary in the path mentioned above, try to
update the mlocate database (`sudo updatedb` or the equivalent in macOS) and
run `locate gazelle`. The gazelle binary should typically be in

    $HOME/.cache/bazel/_bazel_<username>/<somelongfoldername>/external/io_bazel_rules_go_repository_tools/bin/gazelle

## Test environment ##

Manager tests require access to a Kubernetes v1.5.2 cluster. Each
test operates on a temporary namespace and deletes it on completion.  Please
configure your `kubectl` to point to a development cluster (e.g. minikube)
before building or invoking the tests and add a symbolic link to your
repository pointing to Kubernetes cluster credentials:

    ln -s ~/.kube/config platform/kube/

_Note_: If you are running Bazel in a VM, copy the kube config file on the
host to platform/kube instead of symlinking it.

    cp ~/.kube/config platform/kube/

If you are using GKE, please make sure you are using static client
certificates before fetching cluster credentials:

    gcloud config set container/use_client_certificate True

To run the tests:

    bazel test //...

## Docker images ##

We provide Bazel targets to output Istio runtime images:

    bazel run //docker:runtime
    
The image includes Istio Proxy and Istio Manager.
