suffix=$1

/opt/microservices/details 9080 &
/opt/a8sidecar/a8sidecar --config /etc/config.yaml.$suffix &
envoy -c /etc/details-envoy.json
