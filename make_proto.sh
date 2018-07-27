#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.
CWD=$PWD

for i in $CWD/proto/ $CWD/crypto/proto/ \
                     $CWD/actions/proto/ \
                     $CWD/flows/proto/ ; do
    echo Building protos in $i
    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$CWD --go_out=$i $i/*.proto
done

# Build GRPC servers.
for i in  $CWD/api/proto/ ; do
    echo Building protos in $i
    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
           -I$CWD $i/*.proto --go_out=plugins=grpc:$i

    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
           --grpc-gateway_out=logtostderr=true:$i $i/*.proto

done
