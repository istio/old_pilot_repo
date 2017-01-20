#!/bin/bash

gometalinter --deadline=300s --disable-all\
  --enable=gofmt\
  --enable=vet\
  --enable=errcheck\
  ./...
