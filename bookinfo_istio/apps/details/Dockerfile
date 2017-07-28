FROM lyft/envoy:latest

RUN mkdir /opt/microservices
RUN mkdir /opt/istio
RUN mkdir /etc/istio
RUN mkdir /etc/istio/proxy/
RUN apt-get -y install iptables
RUN apt-get -y install curl 
RUN adduser -disabled-password --gecos "" --uid 1337 istio
RUN chown istio /etc/istio/proxy

ADD ./config.yaml.v1 /etc/
ADD ./details /opt/microservices
ADD ./pilot-agent /opt/istio
ADD ./prepare_proxy.sh /opt/istio
ADD ./start_service.sh /usr/local/bin/start_service.sh
RUN chmod u+x /usr/local/bin/start_service.sh

ENTRYPOINT /usr/local/bin/start_service.sh v1
