#!/bin/bash
cat <<EOF
syntax = "proto3";
package pb;
import "google/protobuf/duration.proto";
import "google/protobuf/timestamp.proto";
import "google/rpc/status.proto";
EOF

cat - |\
  sed '/^\/\//d' |\
  sed '/^package /d' |\
  sed '/^option (gogoproto/d' |\
  sed '/^import "/d' |\
  sed '/^syntax =/d' |\
  sed 's/\[.*\];$/;/'
