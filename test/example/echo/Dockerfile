FROM ubuntu:14.04
ADD envoy_esp /usr/local/bin/envoy
ADD generic_service_config.json /etc/envoy/generic_service_config.json
ADD server_config.pb.txt /etc/envoy/server_config.pb.txt
ADD cmd /cmd
ENTRYPOINT ["/cmd"]
