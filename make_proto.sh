#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.

# This script requires protoc 3.6 +

set -e

CWD=$PWD

if [ -z "$GOPATH" ]; then
    GOPATH="$HOME/go"
fi

GOOGLEAPIS_PATH=$CWD/googleapis/
GOOGLEAPIS_COMMIT="82a542279"

if [ ! -d "$GOOGLEAPIS_PATH" ]; then
    git clone https://github.com/googleapis/googleapis/ $GOOGLEAPIS_PATH
    (cd googleapis && git checkout $GOOGLEAPIS_COMMIT)
fi

if ! command -v protoc-gen-go > /dev/null; then
    go install google.golang.org/protobuf/cmd/protoc-gen-go
fi

if ! command -v protoc-gen-go-grpc > /dev/null; then
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
fi

if ! command -v protoc-gen-grpc-gateway > /dev/null; then
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
fi

for i in $CWD/proto/ $CWD/crypto/proto/ \
                     $CWD/artifacts/proto/ \
                     $CWD/actions/proto/ \
                     $CWD/services/frontend/proto/ \
                     $CWD/config/proto/ \
                     $CWD/timelines/proto/ \
                     $CWD/acls/proto/ \
                     $CWD/flows/proto/ ; do
    echo Building protos in $i
    echo protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$GOOGLEAPIS_PATH -I$CWD --go_out=paths=source_relative:$i $i/*.proto
    protoc -I$i -I$GOPATH/src/ -I/usr/local/include/ -I$GOOGLEAPIS_PATH -I$CWD --go_out=paths=source_relative:$i $i/*.proto
done

# Build GRPC servers.
for i in  $CWD/api/proto/ ; do
    echo Building protos in $i
    echo protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS_PATH \
           -I$CWD $i/*.proto --go-grpc_out=paths=source_relative:$i --go_out=paths=source_relative:$i

    protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS_PATH \
           -I$CWD $i/*.proto --go-grpc_out=paths=source_relative:$i --go_out=paths=source_relative:$i

    echo protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS_PATH \
           --grpc-gateway_out=paths=source_relative,logtostderr=true:$i $i/*.proto

    protoc -I$i -I. -I$GOPATH/src/ -I/usr/local/include/ \
           -I$GOOGLEAPIS_PATH \
           --grpc-gateway_out=paths=source_relative,logtostderr=true:$i $i/*.proto

done
