FROM lyft/envoy:latest

ARG enable_ratings
ARG star_color
ENV ENABLE_RATINGS ${enable_ratings:-false}
ENV STAR_COLOR ${star_color:-black}

RUN mkdir /opt/microservices
ADD ./reviews /opt/microservices
ADD ./start_service.sh /usr/local/bin/start_service.sh
RUN chmod u+x /usr/local/bin/start_service.sh
ENTRYPOINT /usr/local/bin/start_service.sh
