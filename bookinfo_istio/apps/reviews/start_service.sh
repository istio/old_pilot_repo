suffix=$1

/opt/microservices/reviews 9080 http://localhost:6379 &
su istio -c "/opt/istio/pilot --adapter VMs proxy sidecar --config /etc/config.yaml.$suffix > /tmp/envoy.log" 
