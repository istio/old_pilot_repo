#!/bin/bash
set -e

# Generate SHA for the images and push it
TAG=$(git rev-parse HEAD)

# Creation step
create=true

while getopts :t:s arg; do
  case ${arg} in
    t) TAG="${OPTARG}";;
    s) create=false;;
    *) echo "Invalid option: -${OPTARG}"; exit 1;;
  esac
done

# Write template for k8s
rm echo.yaml
sed "s|\$TAG|$TAG|g" manager.yaml.tmpl                    >> echo.yaml
sed "s|\$TAG|$TAG|g;s|\$NAME|a|g" http-service.yaml.tmpl  >> echo.yaml
sed "s|\$TAG|$TAG|g;s|\$NAME|b|g" http-service.yaml.tmpl  >> echo.yaml

if [[ "$create" = true ]]; then
  bazel run //docker:runtime
  docker tag istio/docker:runtime gcr.io/istio-test/runtime:$TAG
  gcloud config set project istio-test
  gcloud docker -- push gcr.io/istio-test/runtime:$TAG
  kubectl apply -f echo.yaml
fi

# Wait for pods to be ready
while : ; do
  kubectl get pods | grep -i "init\|creat\|error" || break
  sleep 1
done

a=$(kubectl get pods -l app=a -o jsonpath='{range .items[*]}{@.metadata.name}')
b=$(kubectl get pods -l app=b -o jsonpath='{range .items[*]}{@.metadata.name}')
t=$(kubectl get pods -l app=t -o jsonpath='{range .items[*]}{@.metadata.name}')
m=$(kubectl get pods -l app=m -o jsonpath='{range .items[*]}{@.metadata.name}')

# try all requests a,b,t
tt=false
for src in a b t; do
  for dst in a b t; do
    echo request from ${src} to ${dst}/${src}

    request=$(kubectl exec ${!src} -it -c echo curl ${dst}/${src} | grep "x-request-id" ||\
      if [[ $src == "t" && $dst == "t" ]]; then
        tt=true
        echo "Expected no request"
      else
        echo Failed injecting envoy
        exit 1
      fi
    )

    id=$(echo $request | cut -d'=' -f2-)

    # query access logs in src and dst
    if [[ $src != "t" ]]; then
      echo checking access log of $src
      kubectl logs ${!src} -c proxy | grep "$id" && (echo "Cannot find request id in access log"; exit 1)
    fi
    if [[ $dst != "t" ]]; then
      echo checking access log of $dst
      kubectl logs ${!dst} -c proxy | grep "$id" && (echo "Cannot find request id in access log"; exit 1)
    fi
  done
done

echo $ab

