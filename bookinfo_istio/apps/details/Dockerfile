FROM lyft/envoy:latest

RUN mkdir /opt/microservices
RUN mkdir /opt/istio
RUN mkdir /etc/envoy/

ADD ./details /opt/microservices
ADD ./pilot /opt/istio
ADD ./start_service.sh /usr/local/bin/start_service.sh
RUN chmod u+x /usr/local/bin/start_service.sh

ENTRYPOINT /usr/local/bin/start_service.sh v1
