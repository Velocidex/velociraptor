package inventory

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/go-errors/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

var (
	DeniedAccessError = errors.New("InventoryService: External access is forbidden by configuration https://docs.velociraptor.app/docs/deployment/references/#security.disable_inventory_service_external_access")
)

func DownloadToolFromUpstream(
	ctx context.Context, config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) (io.ReadCloser, error) {

	if config_obj.Security != nil &&
		config_obj.Security.DisableInventoryServiceExternalAccess {
		return nil, DeniedAccessError
	}

	scope := vql_subsystem.MakeScope()
	default_client, err := networking.GetDefaultHTTPClient(
		ctx, config_obj.Client, scope, "", networking.EmptyCookieJar)
	if err != nil {
		return nil, err
	}

	// If we are downloading from github we have to resolve and
	// verify the binary URL now.
	if tool.GithubProject != "" {
		var err error
		tool.Url, err = getGithubRelease(
			ctx, default_client, config_obj, tool)
		if err != nil {
			return nil, fmt.Errorf(
				"While resolving github release %v: %w ",
				tool.GithubProject, err)
		}

		// Set the filename to something sensible so it is always valid.
		if tool.Filename == "" {
			if tool.Url != "" {
				tool.Filename = utils.SanitizeString(path.Base(tool.Url))
			} else {
				tool.Filename = utils.SanitizeString(path.Base(tool.ServeUrl))
			}
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Downloading tool <green>%v</> FROM <cyan>%v</>", tool.Name,
		tool.Url)

	request, err := http.NewRequestWithContext(ctx, "GET", tool.Url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", constants.USER_AGENT)
	res, err := default_client.Do(request)
	if err != nil {
		return nil, err
	}

	// If the download failed, we can not store this tool.
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Unable to download file from %v: %v",
			tool.Url, res.Status)
	}

	switch strings.ToLower(tool.DownloadTransform) {
	case "":
		return res.Body, nil

	case "gunzip":
		gzipReader, err := gzip.NewReader(res.Body)
		if err != nil {
			res.Body.Close()
			return nil, err
		}

		return &gunzipReadCloser{
			Reader: gzipReader,
			Closer: res.Body,
		}, nil

	default:
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("Tool downloader for %v: unsupported tranform %v",
			tool.Name, err)
		return res.Body, nil
	}
}

type gunzipReadCloser struct {
	io.Reader
	io.Closer
}
