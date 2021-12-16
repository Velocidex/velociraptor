// +build tools

package tools

// Imports necessary packages for generating the .pb.go files. Only used
// to have `go mod tidy' resolve these packages consistently.
import (
	_ "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
