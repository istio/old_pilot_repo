FROM lyft/envoy:latest

RUN mkdir /opt/microservices
ADD ./productpage /opt/microservices
ADD ./start_service.sh /usr/local/bin/start_service.sh
RUN chmod u+x /usr/local/bin/start_service.sh
ENTRYPOINT /usr/local/bin/start_service.sh
