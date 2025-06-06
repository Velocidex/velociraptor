#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.

# This script requires protoc 3.6 +

set -e

CWD=$PWD
PROTOC=${PROTOC:-"protoc"}
QUIET=${QUIET:-}
PROTOC=protoc

if [ -z "$GOPATH" ]; then
    GOPATH="$HOME/go"
fi

function debug() {
    if [ -z "$QUIET" ]; then
        echo "$@"
    fi
}

#GOOGLEAPIS_PATH=$CWD/googleapis/
#GOOGLEAPIS_COMMIT="82a542279"

#if [ ! -d "$GOOGLEAPIS_PATH" ]; then
#    git clone --shallow-since 2021-12-15  https://github.com/googleapis/googleapis/ $GOOGLEAPIS_PATH
#    (cd googleapis && git checkout $GOOGLEAPIS_COMMIT)
#fi

# Instead of checking out the latest git project, we just manually
# copy the few files we actually need into third party.
GOOGLEAPIS_PATH=$CWD/third_party/googleapis/

PROTOC_GEN_GO_VERSION="v1.34.1"
if ! [ -x "$(command -v protoc-gen-go)" ] || [ "$(protoc-gen-go --version)" != "protoc-gen-go $PROTOC_GEN_GO_VERSION" ]; then
    debug "Installing protoc-gen-go $PROTOC_GEN_GO_VERSION"
    go install "google.golang.org/protobuf/cmd/protoc-gen-go@$PROTOC_GEN_GO_VERSION"
fi

if ! command -v protoc-gen-go-grpc > /dev/null; then
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
fi

if ! command -v protoc-gen-grpc-gateway > /dev/null; then
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
fi

PROTO_DIRECTORIES="$CWD/proto/ \
                     $CWD/crypto/proto/ \
                     $CWD/artifacts/proto/ \
                     $CWD/actions/proto/ \
                     $CWD/services/frontend/proto/ \
                     $CWD/config/proto/ \
                     $CWD/timelines/proto/ \
                     $CWD/acls/proto/ \
                     $CWD/flows/proto/"

COMMON_OPTS="-I."
if [ -d "$GOPATH/src/" ]; then
    COMMON_OPTS="$COMMON_OPTS -I$GOPATH/src/"
fi
if [ -d "/usr/include/" ]; then
    COMMON_OPTS="$COMMON_OPTS -I/usr/include/"
fi
if [ -d "/usr/local/include/" ]; then
    COMMON_OPTS="$COMMON_OPTS -I/usr/local/include/"
fi
COMMON_OPTS="$COMMON_OPTS -I$GOOGLEAPIS_PATH -I$CWD"

for i in  $PROTO_DIRECTORIES ; do
    debug Building protos in $i
    debug $PROTOC -I$i $COMMON_OPTS $i/*.proto --go_out=paths=source_relative:$i
    $PROTOC -I$i $COMMON_OPTS $i/*.proto --go_out=paths=source_relative:$i

    # Clean up extra version information the proto compiler adds to
    # the files.
    sed -i -e '1h;2,$H;$!d;g' -re 's|// versions.+// source:|// source:|' $i/*.pb.go
done

# Build GRPC servers.
for i in  $CWD/api/proto/ ; do
    debug Building protos in $i
    debug $PROTOC -I$i $COMMON_OPTS $i/*.proto --go-grpc_out=paths=source_relative:$i --go_out=paths=source_relative:$i

    $PROTOC -I$i -I. $COMMON_OPTS $i/*.proto --go-grpc_out=paths=source_relative:$i --go_out=paths=source_relative:$i

    debug $PROTOC -I$i -I. $COMMON_OPTS $i/*.proto --grpc-gateway_out=paths=source_relative,logtostderr=true:$i
    $PROTOC -I$i -I. $COMMON_OPTS $i/*.proto --grpc-gateway_out=paths=source_relative,logtostderr=true:$i

    # Clean up extra version information the proto compiler adds to
    # the files.
    sed -i -e '1h;2,$H;$!d;g' -re 's|// versions.+// source|// source|' $i/*.pb.go
done
