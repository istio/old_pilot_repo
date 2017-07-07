# Platform Adapter for Istio Pilot on VMs

Make Istio Pilot work on general Virtual Machines by integrating Amalgam8(A8)[Github Repo](https://github.com/amalgam8/amalgam8) registration functionalities.

## Design Principle

The key issue is how to implement the ServiceDiscvery interface functions in Istio. 
This platform adapter uses A8 Registry Server to help Istio monitor service instances running in the underlying platform.
When a service instance is brought up, a configuration file should be provided along and used by the registration client to register the instance to the server. While the instance is up, a registry client periodically sends heart beat to the server to renew the registration. As the instance is down, the client sends termination request and the server will delete the instance record. The Istio VMs controller for Service Discovery functionalities can be realized by calling the Amalgam8 registry client APIs. 
    The design principle is illustrated in the following diagram.

![design diagram](https://cdn.rawgit.com/istio/pilot/platform_vms/bookinfo_istio/vms_design.png)
Noted that Istio pilot is running inside each app container so as to coordinate Envoy and the service mesh.
Currently the bookinfo demo and test for functionalities on Docker is provided. CloudFoundry is targeted for the next step.

## Test and Demo
The bookinfo demo is used to test the functionalities of this platform adapter.Currently all latest related images are pushed to my own docker hub(kimikowang). Containers can be brought up by running `docker-compose up -d` in the docker directory.

Each App image contains a running app process and a pilot binary running as a proxy sidecar. iptables rules for the proxy are setup by running script `prepare_proxy.sh -u 1337 -p 15001` copied from the `pilot/bin` directory.

You can also go through the full procedure --- building images, pushing image to hub and bringing up containers by running `test_scripts/run_test.sh`. 

Noted that currently CDS/SDS/RDS info can be retrived from discovery container successfully. (You can calling the APIs by curling the host `localhost:8080` outside of containers or `istio-pilot:8080` within a container by switching to user `istio`)

However, now the service instances can not talk to each other yet. This issue is still being debugged.
