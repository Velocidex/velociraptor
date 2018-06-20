package api

import (
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"net/http"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
)

func StartHTTPProxy(config_obj *config.Config) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := api_proto.RegisterAPIHandlerFromEndpoint(
		ctx, mux,
		fmt.Sprintf("%s:%d",
			*config_obj.API_bind_address,
			*config_obj.API_bind_port),
		opts)
	if err != nil {
		return err
	}

	return http.ListenAndServe(
		fmt.Sprintf("%s:%d",
			*config_obj.API_proxy_bind_address,
			*config_obj.API_proxy_bind_port),
		mux)
}
