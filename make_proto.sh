#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.
CWD=$PWD

for i in $CWD/proto/ $CWD/crypto/proto/ $CWD/actions/proto/ $CWD/flows/proto/; do
    echo Building protos in $i
    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$CWD --go_out=$i $i/*.proto
done
