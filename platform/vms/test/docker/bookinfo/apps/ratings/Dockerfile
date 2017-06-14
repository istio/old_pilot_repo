FROM lyft/envoy:latest

RUN mkdir /opt/microservices
RUN mkdir /opt/a8sidecar

ADD ./ratings /opt/microservices
ADD ./a8sidecar /opt/a8sidecar
ADD ./start_service.sh /usr/local/bin/start_service.sh
RUN chmod u+x /usr/local/bin/start_service.sh

ENTRYPOINT /usr/local/bin/start_service.sh v1
