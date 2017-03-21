# Proxy sidecar injection

## Automatic injection

Istio's goal is transparent proxy injection into end-user deployments
with minimal effort from the end-user. Ideally, a kubernetes admission
controller would rewrite specs to include the necessary init and proxy
containers before they are committed, but his currently requires
upstreaming changes to kubernetes which we would like to avoid for
now. Instead, it would be better if a dynamic plug-in mechanism
existed whereby admisson controllers could be maintained
out-of-tree. There is no platform support for this yet, but a proposal
has been created to add such a feature
(see [Proposal: Extensible Admission Control](https://github.com/kubernetes/community/pull/132/)).

Long term istio automatic proxy injection is being tracked
by [Kubernetes Admission Controller for proxy injection](https://github.com/istio/manager/issues/57).

## Manual injection

Use istioutil to inject sidecar proxy into resource files (i.e. client-side injection).

    istioutil inject -f deployment.yaml -o deployment-istio.yaml

Or update existing deployments.

    kubectl get deployment -o yaml | istioctl inject -f - | kubectl apply -f -

The Istio project is continually evolving so the low-level proxy
configuration may change unannounced. When it doubt re-run `istioutil
inject` on your original deployments.
