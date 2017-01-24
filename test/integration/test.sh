#!/bin/bash
set -e

# Generate SHA for the images and push it
TAG=$(git rev-parse HEAD)
HUB="gcr.io/istio-test"

# Creation step
create=true

while getopts :h:t:s arg; do
  case ${arg} in
    h) HUB="${OPTARG}";;
    t) TAG="${OPTARG}";;
    s) create=false;;
    *) echo "Invalid option: -${OPTARG}"; exit 1;;
  esac
done

# Write template for k8s
rm -f echo.yaml
sed "s|\$HUB|$HUB|g;s|\$TAG|$TAG|g" manager.yaml.tmpl                    >> echo.yaml
sed "s|\$HUB|$HUB|g;s|\$TAG|$TAG|g;s|\$NAME|a|g;s|\$PORT|8080|g" http-service.yaml.tmpl  >> echo.yaml
sed "s|\$HUB|$HUB|g;s|\$TAG|$TAG|g;s|\$NAME|b|g;s|\$PORT|8080|g" http-service.yaml.tmpl  >> echo.yaml

if [[ "$create" = true ]]; then
  gcloud docker --authorize-only
  for image in runtime app; do
    bazel run //docker:$image
    docker tag istio/docker:$image $HUB/$image:$TAG
    docker push $HUB/$image:$TAG
  done
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
    echo request from ${src} to ${dst}

    request=$(kubectl exec ${!src} -c app client http://${dst}/${src})

    echo $request | grep "X-Request-Id" ||\
      if [[ $src == "t" && $dst == "t" ]]; then
        tt=true
        echo "Expected no request"
      else
        echo Failed injecting envoy
        exit 1
      fi

    id=$(echo $request | grep -o "X-Request-Id=\S*" | cut -d'=' -f2-)
    echo x-request-id=$id

    # query access logs in src and dst
    for log in $src $dst; do
      if [[ $log != "t" ]]; then
        echo checking access log of $log

        n=1
        while : ; do
          if [[ $n == 30 ]]; then
            break
          fi
          kubectl logs ${!log} -c proxy | grep "$id" && break
          sleep 1
          ((n++))
        done

        if [[ $n == 30 ]]; then
          echo failed to find request $id in access log of $log after $n attempts
          exit 1
        fi
      fi
    done
  done
done

