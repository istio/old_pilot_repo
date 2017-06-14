suffix=$1

./productpage 9080 http://localhost:8080 &
./a8sidecar --config config.yaml.$suffix &
./envoy -c productpage-envoy.json -l trace
