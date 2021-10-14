package main

import (
	"net/http"
	"net/url"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/http_comms"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

func ensureProxy(config_obj *config_proto.Config) error {
	http_proxy := getEnvAny("HTTP_PROXY", "http_proxy")
	https_proxy := getEnvAny("HTTPS_PROXY", "https_proxy")

	// If neither environment variables are set, we look to the config file
	if http_proxy == "" && https_proxy == "" {
		if config_obj.Frontend != nil && config_obj.Frontend.Proxy != "" {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Info("Setting proxy to <green>%v</>", config_obj.Frontend.Proxy)

			setUrlProxy(config_obj.Frontend.Proxy)
		} else if config_obj.Client != nil && config_obj.Client.Proxy != "" {
			logger := logging.GetLogger(config_obj, &logging.ClientComponent)
			logger.Info("Setting proxy to <green>%v</>", config_obj.Client.Proxy)

			setUrlProxy(config_obj.Client.Proxy)
		}
	}

	return nil
}

func setUrlProxy(url_str string) {
	url, err := url.Parse(url_str)
	kingpin.FatalIfError(err, "Unable to parse proxy url: "+url_str)

	networking.SetProxy(http.ProxyURL(url))
	http_comms.SetProxy(http.ProxyURL(url))
}

func getEnvAny(names ...string) string {
	for _, n := range names {
		if val := os.Getenv(n); val != "" {
			return val
		}
	}
	return ""
}
