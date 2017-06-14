suffix=$1

/opt/microservices/reviews 9080 http://localhost:6379 &
/opt/a8sidecar/a8sidecar --config /etc/config.yaml.$suffix &
envoy -c /etc/reviews-envoy.json
