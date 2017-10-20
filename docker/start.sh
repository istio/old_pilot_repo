#!/bin/bash

count=0

if [ -f /usr/local/bin/pilot-discovery ]; then
while true; do

rm /etc/istio/start.$count
count=$(($count+1))
touch /etc/istio/start.$count

   if [ -x /etc/istio/pilot-discovery ]; then
/etc/istio/pilot-discovery            "$@"
   else
/usr/local/bin/pilot-discovery  "$@"
   fi

done
fi

function ln_filename() {
   typepod=$1
   if [[ $POD_NAMESPACE == *noauth ]]; then
     auth_str=""
   else
     auth_str="_auth"
   fi
set -x
 ln -s /etc/istio/envoy_$typepod$auth_str.json /etc/istio/proxy/envoy$2.json
 cp  /etc/istio/envoy_$typepod$auth_str.json /etc/istio/proxy/envoy_cp.json
  #  ln -s /etc/istio/envoy_$typepod$auth_str.json /etc/istio/proxy/envoy.json
set +x
}

set -x
if [[ $HOSTNAME =~ .*istio-pilot* ]]; then
  ln_filename "pilot"                   "use_generated"
elif [[ $HOSTNAME =~ .*istio-mixer* ]]; then
  ln_filename "mixer"                   "use_generated"
elif [[ $HOSTNAME =~ .*istio-ingress* ]]; then
  ln_filename "ingress"                 "use_generated"
elif [[ $HOSTNAME =~ .*istio-egress* ]]; then
  ln_filename "egress"                  "use_generated"
else
  ln_filename "proxy"                   "use_generated"
fi
set +x


if [ -f /usr/local/bin/pilot-agent ]; then
while true; do

rm /etc/istio/start.$count
count=$(($count+1))
touch /etc/istio/start.$count

   if [ -x /etc/istio/pilot-agent ]; then
/etc/istio/pilot-agent            "$@"
   else
/usr/local/bin/pilot-agent  "$@"
   fi

done
fi
