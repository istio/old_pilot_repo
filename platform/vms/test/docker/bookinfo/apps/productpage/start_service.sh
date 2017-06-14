suffix=$1

/opt/microservices/productpage 9080 http://localhost:6379 &
/opt/a8sidecar/a8sidecar --config /etc/config.yaml.$suffix &
envoy -c /etc/productpage-envoy.json
