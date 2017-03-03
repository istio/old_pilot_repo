# Istio Manager #
[![Build Status](https://travis-ci.org/istio/manager.svg?branch=master)](https://travis-ci.org/istio/manager)
[![Go Report Card](https://goreportcard.com/badge/github.com/istio/manager)](https://goreportcard.com/report/github.com/istio/manager)
[![GoDoc](https://godoc.org/github.com/istio/manager?status.svg)](https://godoc.org/github.com/istio/manager)
[![codecov.io](https://codecov.io/github/istio/manager/coverage.svg?branch=master)](https://codecov.io/github/istio/manager?branch=master)

Istio Manager is used to configure Istio and propagate configuration to the
other components of the system, including the Istio mixer and the Istio proxies. 

[Contributing to the project](./CONTRIBUTING.md)

## Filing issues ##

If you have a question about the Istio Manager or have a problem using it, please
[file an issue](https://github.com/istio/manager/issues/new).

## Getting started ##

Istio Manager [design](doc/design.md) gives an architectural overview of the manager components - cluster platform abstractions, service model, and the proxy controllers.

If you are interested in contributing to the project, please take a look at the [build instructions](doc/build.md) and the [testing infrastructure](doc/testing.md).

## Test environment ##

Manager tests require access to a Kubernetes cluster (version 1.5.2 or higher). Each
test operates on a temporary namespace and deletes it on completion.  Please
configure your `kubectl` to point to a development cluster (e.g. minikube)
before building or invoking the tests and add a symbolic link to your
repository pointing to Kubernetes cluster credentials:

    ln -s ~/.kube/config platform/kube/

_Note1_: If you are running Bazel in a Vagrant VM (as described below), copy
the kube config file on the host to platform/kube instead of symlinking it,
and change the paths to minikube certs.

    cp ~/.kube/config platform/kube/
    sed -i 's!/Users/<username>!/home/ubuntu!' platform/kube/config

Also, copy the same file to `/home/ubuntu/.kube/config` in the VM, and make
sure that the file is readable to user `ubuntu`.

_Note2_: If you are using GKE, please make sure you are using static client
certificates before fetching cluster credentials:

    gcloud config set container/use_client_certificate True

To run the tests:

    bazel test //...

## Docker images (Linux-only) ##

We provide Bazel targets to output Istio runtime images:

    bazel run //docker:runtime

The image includes Istio Proxy and Istio Manager.

For interactive debug and development it is often useful to 'exec' into the
application pod, modify iptable rules, ping, curl, etc. For this purpose the
prebuilt *docker.io/istio/debug:test* image has been created. To use this image
add the following snippet to your deployment spec alongside the proxy and app
containers.

    - name: debug
      image: docker.io/istio/debug:test
      imagePullPolicy: Always
      securityContext:
          privileged: true

Use `kubectl exec` to access the debug container.

    kubectl exec -it <pod-name> bash -c debug

The proxy injection process redirects *all* inbound and outbound traffic through
the proxy via iptables. This can sometimes be undesirable while debugging, e.g.
trying to install additional test tools via apt-get. Use
`proxy-redirection-clear` to temporarily disable the iptable redirection rules
and `proxy-redirection-restore` to restore them.

