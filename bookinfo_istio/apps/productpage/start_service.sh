suffix=$1

/opt/istio/prepare_proxy.sh -p 15001 -u 1337
/opt/microservices/productpage 9080 http://localhost:6379 &
su istio -c "/opt/istio/pilot-agent proxy --adapter VMs --vmsconfig /etc/config.yaml.$suffix > /tmp/envoy.log" 
