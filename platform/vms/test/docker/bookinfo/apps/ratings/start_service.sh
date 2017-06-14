suffix=$1

/opt/microservices/ratings 9080 &
/opt/a8sidecar/a8sidecar --config /etc/config.yaml.$suffix &
envoy -c /etc/ratings-envoy.json
