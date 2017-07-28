FROM lyft/envoy:latest

RUN mkdir /opt/istio
RUN mkdir /etc/envoy/

ADD ./config.yaml /etc/
ADD ./pilot-discovery /opt/istio
ADD ./start_service.sh /usr/local/bin/start_service.sh
RUN chmod u+x /usr/local/bin/start_service.sh

ENTRYPOINT /usr/local/bin/start_service.sh
