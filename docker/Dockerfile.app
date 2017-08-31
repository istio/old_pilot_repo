FROM ubuntu
RUN apt-get update && apt-get install -y curl
RUN mkdir -p /usr/local/bin
ADD client /usr/local/bin
ADD server /usr/local/bin
ADD certs/cert.crt /cert.crt
ADD certs/cert.key /cert.key
ENTRYPOINT ["/usr/local/bin/server"]
