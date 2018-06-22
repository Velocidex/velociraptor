package api

import (
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"time"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func StartHTTPProxy(config_obj *config.Config) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := http.NewServeMux()
	h, err := GetAPIHandler(ctx, config_obj)
	if err != nil {
		return err
	}
	mux.Handle("/api/", h)

	// Install static file handler.
	if config_obj.AdminUI_document_root != nil {
		// FIXME: Check if path exists.
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(
			http.Dir(*config_obj.AdminUI_document_root))))

		h, err := GetTemplateHandler(config_obj, path.Join(
			*config_obj.AdminUI_document_root, "templates", "index.html"))
		if err != nil {
			return err
		}
		mux.Handle("/index.html", h)
	}

	return http.ListenAndServe(
		fmt.Sprintf("%s:%d",
			*config_obj.API_proxy_bind_address,
			*config_obj.API_proxy_bind_port),
		logging.GetLoggingHandler(config_obj)(mux))
}

type _templateArgs struct {
	Timestamp  int64
	Heading    string
	Help_url   string
	Report_url string
	Version    string
}

func GetAPIHandler(
	ctx context.Context,
	config_obj *config.Config) (http.Handler, error) {
	grpc_proxy_mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := api_proto.RegisterAPIHandlerFromEndpoint(
		ctx, grpc_proxy_mux,
		fmt.Sprintf("%s:%d",
			*config_obj.API_bind_address,
			*config_obj.API_bind_port),
		opts)
	if err != nil {
		return nil, err
	}

	reverse_proxy_mux := http.NewServeMux()
	reverse_proxy_mux.Handle("/api/v1/LaunchFlow", grpc_proxy_mux)
	reverse_proxy_mux.Handle("/api/v1/SearchClients", grpc_proxy_mux)

	reverse_url, err := url.Parse("http://localhost:8000/")
	if err != nil {
		return nil, err
	}
	reverse_proxy_mux.Handle("/api/", httputil.NewSingleHostReverseProxy(
		reverse_url))

	return reverse_proxy_mux, nil
}

func GetTemplateHandler(
	config_obj *config.Config,
	template_path string) (http.Handler, error) {

	tmpl, err := template.ParseFiles(path.Join(
		*config_obj.AdminUI_document_root, "templates", "index.html"))
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		args := _templateArgs{
			Timestamp: time.Now().UnixNano(),
			Heading:   "Heading",
		}
		utils.Debug(args)
		err := tmpl.Execute(w, args)
		if err != nil {
			w.WriteHeader(500)
		}
	}), nil
}
