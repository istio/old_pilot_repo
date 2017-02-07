#!/bin/bash

openssl genrsa -out ca.key.pem 2048
openssl req -x509 -new -nodes -key ca.key.pem -sha256 -days 1024 -out ca.cert.pem \
  -subj "/C=US/ST=California/L=Mountain View/O=Istio Team/OU=Istio/CN=Istio Test CA"

openssl genrsa -out istio.key.pem 2048
openssl req -new -key istio.key.pem -out istio.cert.pem \
  -subj "/C=US/ST=California/L=Mountain View/O=Istio Team/OU=Istio/CN=Istio Test Service"

openssl x509 -req -in istio.cert.pem -CA ca.cert.pem -CAkey ca.key.pem -CAcreateserial -out istio.cert.pem -days 500 
