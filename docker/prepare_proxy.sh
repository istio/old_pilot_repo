#!/bin/bash
# Proxy initialization script responsible for setting up port forwarding.

set -o errexit
set -o nounset
set -o pipefail

usage() {
  echo "${0} -p PORT -u UID [-h]"
  echo ''
  echo '  -p: Specify the proxy port to which redirect all TCP traffic'
  echo '  -u: Specify the UID of the user for which the redirection is not'
  echo '  -e: Comma seperated list of IP ranges in CIDR form to exclude from outbound redirection to proxy (optional)'
  echo '  -i: Comma seperated list of IP ranges in CIDR form to redirection to proxy (optional)'
  echo '      applied. Typically, this is the UID of the proxy container'
  echo ''
}

ISTIO_IP_RANGES_INCLUDE=""
ISTIO_IP_RANGES_EXCLUDE=""

while getopts ":p:u:e:i:h" opt; do
  case ${opt} in
    p)
      ISTIO_PROXY_PORT=${OPTARG}
      ;;
    u)
      ISTIO_PROXY_UID=${OPTARG}
      ;;
    e)
      ISTIO_IP_RANGES_EXCLUDE=${OPTARG}
      ;;
    i)
      ISTIO_IP_RANGES_INCLUDE=${OPTARG}
      ;;
    h)
      usage
      exit 0
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${ISTIO_PROXY_PORT-}" ]] || [[ -z "${ISTIO_PROXY_UID-}" ]]; then
  echo "Please set both -p and -u parameters"
  usage
  exit 1
fi

iptables -t nat -F

# Create a new chain for redirecting inbound and outbound traffic to
# the common Istio proxy port.
iptables -t nat -N ISTIO_REDIRECT                                                   -m comment --comment "istio/redirect-common-chain"
iptables -t nat -A ISTIO_REDIRECT -p tcp -j REDIRECT --to-ports ${ISTIO_PROXY_PORT} -m comment --comment "istio/redirect-to-proxy-port"

# Redirect all inbound traffic to the proxy.
iptables -t nat -A PREROUTING -j ISTIO_REDIRECT                                     -m comment --comment "istio/install-istio-prerouting"

# Create a new chain for selectively redirecting outbound packets to
# istio and attach it the OUTPUT rule for all tcp traffic. '-j RETURN'
# bypasses the proxy port; '-j ISTIO_REDIRECT' redirects to the proxy
# port.
iptables -t nat -N ISTIO_OUTPUT
iptables -t nat -A OUTPUT -p tcp -j ISTIO_OUTPUT                                    -m comment --comment "istio/install-istio-output"

# Locally routed traffic is not captured by PREROUTING chain. Redirect
# loopback back traffic when the DST_IP is not explicitly the loopback
# address to handles appN => proxy (client) => proxy (server) => appN.
iptables -t nat -A ISTIO_OUTPUT -o lo ! -d 127.0.0.1/32 -j ISTIO_REDIRECT           -m comment --comment "istio/implicit-loopback"

# Avoid infinite loops. Don't redirect proxy traffic directly back to
# the proxy for non-loopback traffic.
iptables -t nat -A ISTIO_OUTPUT -m owner --uid-owner ${ISTIO_PROXY_UID} -j RETURN   -m comment --comment "istio/exclude-proxy"

# Skip redirection for proxy-aware applications and
# container-to-container traffic which explicitly use localhost.
iptables -t nat -A ISTIO_OUTPUT -d 127.0.0.1/32 -j RETURN                           -m comment --comment "istio/explicit-loopback"

# The default outobund redirection policy is opt-out. IP ranges may be
# selectively excluded from redirection with ISTIO_IP_RANGES_EXCLUDE.
#
# If ISTIO_IP_RANGES_INCLUDE is defined the outbound redirection
# policy is default opt-in. IP ranges may be selectively included for
# redirection with ISTIO_IP_RANGES_INCLUDE.
IFS=,
if [ "${ISTIO_IP_RANGES_INCLUDE}" = "" ]; then
    for cidr in ${ISTIO_IP_RANGES_EXCLUDE}; do
        iptables -t nat -A ISTIO_OUTPUT -d ${cidr} -j RETURN                        -m comment --comment "istio/exclude-cidr-block-${cidr}"
    done
    iptables -t nat -A ISTIO_OUTPUT -j ISTIO_REDIRECT                               -m comment --comment "istio/default-opt-in-redirect"
else
    for cidr in ${ISTIO_IP_RANGES_INCLUDE}; do
        iptables -t nat -A ISTIO_OUTPUT -d ${cidr} -j ISTIO_REDIRECT                -m comment --comment "istio/include-cidr-block-${cidr}"
    done
    iptables -t nat -A ISTIO_OUTPUT -j RETURN                                       -m comment --comment "istio/default-opt-out-redirect"
fi

exit 0
