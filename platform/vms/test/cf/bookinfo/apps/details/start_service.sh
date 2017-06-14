suffix=$1

./details 9080 &
./a8sidecar --config config.yaml.$suffix &
./envoy -c details-envoy.json
