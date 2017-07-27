/opt/istio/prepare_proxy.sh -p 15001 -u 1337
su istio -c "/opt/istio/pilot --adapter VMs proxy ingress -v 2 > /tmp/envoy.log"
