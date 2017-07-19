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

## Bookinfo Demo

The ingress controller is still under construction, rounting functionalities can be tested by going into containers and making requsts to services.

To bring up all containers directly, you can go into the `/docker` directory and run 

    docker-compose up -d 

This will pull images from docker hub to your local computing space.

If you want to fo through the complete building process(build the app binaries, images and bring up the containers to docker), you can run the script 

    ./test_script/run_test.sh

Now you can see all the containers in the mesh by running `docker ps -a`.

Go into one of the productpage containers and run `curl productpage:9080/productpage`, you should see the result returned and displayed in the console.

If you run this command several times, you should see different versions of reviews showing in the productpage response presented in a round robin style. This Proves the functionality of the load balancer of Envoy in the productpage container.

Noted that access log of Envoy is redirected to `/tmp/envoy.log` within each container. 
