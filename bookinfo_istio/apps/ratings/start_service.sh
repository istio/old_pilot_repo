suffix=$1

/opt/microservices/ratings 9080 &
su istio -c "/opt/istio/pilot --adapter VMs proxy sidecar --config /etc/config.yaml.$suffix > /tmp/envoy.log" 
