suffix=$1

/opt/microservices/details 9080 &
/opt/istio/pilot --adapter VMs proxy sidecar --config /etc/config.yaml.$suffix 
#envoy -c /etc/details-envoy.json
