#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.

# This script requires protoc 3.6 +

CWD=$PWD

if [ -z "$GOPATH" ]; then
    GOPATH="$HOME/go"
fi

for i in $CWD/proto/ $CWD/crypto/proto/ \
                     $CWD/artifacts/proto/ \
                     $CWD/actions/proto/ \
                     $CWD/services/frontend/proto/ \
                     $CWD/config/proto/ \
                     $CWD/acls/proto/ \
                     $CWD/flows/proto/ ; do
    echo Building protos in $i
    echo protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$CWD --go_out=paths=source_relative:$i $i/*.proto
    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$CWD --go_out=paths=source_relative:$i $i/*.proto
done

# Build GRPC servers.
for i in  $CWD/api/proto/ ; do
    echo Building protos in $i
    echo protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
           -I$CWD $i/*.proto --go_out=paths=source_relative,plugins=grpc:$i

    protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
           -I$CWD $i/*.proto --go_out=paths=source_relative,plugins=grpc:$i

    echo protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
           --grpc-gateway_out=paths=source_relative,logtostderr=true:$i $i/*.proto

    protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
           --grpc-gateway_out=paths=source_relative,logtostderr=true:$i $i/*.proto

done
