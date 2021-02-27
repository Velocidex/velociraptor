#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.

# This script requires protoc 3.6 +

CWD=$PWD

if [ -z "$GOPATH" ]; then
    GOPATH="$HOME/go"
fi

# Stupid workaround due to
# https://github.com/grpc-ecosystem/grpc-gateway/issues/1065 May need
# to adjust version - luckily we check in generated go files so we
# only need to do this once on dev box.
GOOGLEAPIS=~/go/pkg/mod/github.com/grpc-ecosystem/grpc-gateway@v1.14.7/third_party/googleapis/

for i in $CWD/proto/ $CWD/crypto/proto/ \
                     $CWD/artifacts/proto/ \
                     $CWD/actions/proto/ \
                     $CWD/services/frontend/proto/ \
                     $CWD/config/proto/ \
                     $CWD/acls/proto/ \
                     $CWD/flows/proto/ ; do
    echo Building protos in $i
    echo protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$GOOGLEAPIS -I$CWD --go_out=paths=source_relative:$i $i/*.proto
    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$GOOGLEAPIS -I$CWD --go_out=paths=source_relative:$i $i/*.proto
done

# Build GRPC servers.
for i in  $CWD/api/proto/ ; do
    echo Building protos in $i
    echo protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS \
           -I$CWD $i/*.proto --go-grpc_out=paths=source_relative:$i --go_out=paths=source_relative:$i

    protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS \
           -I$CWD $i/*.proto --go-grpc_out=paths=source_relative:$i --go_out=paths=source_relative:$i

    echo protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS \
           --grpc-gateway_out=paths=source_relative,logtostderr=true:$i $i/*.proto

    protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS \
           --grpc-gateway_out=paths=source_relative,logtostderr=true:$i $i/*.proto

done
