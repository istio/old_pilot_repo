suffix=$1

./ratings 9080 &
./a8sidecar --config config.yaml.$suffix &
./envoy -c ratings-envoy.json
