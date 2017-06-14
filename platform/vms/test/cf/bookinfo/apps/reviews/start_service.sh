suffix=$1

./reviews 9080 http://localhost:8080 &
./a8sidecar --config config.yaml.$suffix &
./envoy -c reviews-envoy.json
