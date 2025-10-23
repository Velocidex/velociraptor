package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"google.golang.org/protobuf/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type Dummy struct {
	mu        sync.Mutex
	binaries  *artifacts_proto.ThirdParty
	Client    networking.HTTPClient
	Clock     utils.Clock
	filenames []string
}

func (self *Dummy) getTempFile(
	config_obj *config_proto.Config,
	filename, url string) (*os.File, error) {

	file, err := tempfile.TempFile("tmp*" + filename + "." + filepath.Ext(url))
	if err != nil {
		return nil, err
	}
	utils_tempfile.AddTmpFile(file.Name())

	self.filenames = append(self.filenames, file.Name())

	logger := logging.GetLogger(config_obj, &logging.GenericComponent)
	logger.Info("Creating tempfile %v", file.Name())

	return file, nil
}

func (self *Dummy) Close(config_obj *config_proto.Config) {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(config_obj, &logging.GenericComponent)

	removal := func(filename string) {
		logger.Info("tempfile: removing tempfile %v", filename)

		// On windows especially we can not remove files that
		// are opened by something else, so we keep trying for
		// a while.
		var err error
		for i := 0; i < 10; i++ {
			err = os.Remove(filename)
			if err == nil {
				break
			}
			time.Sleep(time.Second)
		}
		utils_tempfile.RemoveTmpFile(filename, err)
	}

	for _, filename := range self.filenames {
		removal(filename)
	}
}

func (self *Dummy) Get() *artifacts_proto.ThirdParty {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	return proto.Clone(self.binaries).(*artifacts_proto.ThirdParty)
}

func (self *Dummy) ProbeToolInfo(
	ctx context.Context, config_obj *config_proto.Config,
	name, version string) (*artifacts_proto.Tool, error) {
	for _, tool := range self.Get().Tools {
		if tool.Name != name {
			continue
		}
		if version != "" && tool.Version != version {
			continue
		}

		return tool, nil
	}
	return nil, errors.New("Not Found")
}

func (self *Dummy) GetToolInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool, version string) (*artifacts_proto.Tool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	for _, item := range self.binaries.Tools {
		// If a version is specified skip tools that are not the
		// correct version.
		if version != "" && (item.Name != tool || item.Version != version) {
			continue
		}

		if item.Name == tool {
			// Currently we require to know all tool's
			// hashes. If the hash is missing then the
			// tool is not tracked. We have to materialize
			// it in order to track it.
			if item.Hash == "" {
				// Try to download the item.
				err := self.materializeTool(ctx, config_obj, item)
				if err != nil {
					return nil, err
				}
			}
			return proto.Clone(item).(*artifacts_proto.Tool), nil
		}
	}
	return nil, fmt.Errorf("Dummy inventory: Tool %v not declared in inventory.", tool)
}

// Actually download and resolve the tool and make sure it is
// available. If successful this function updates the tool's datastore
// representation to track it (in particular the hash). Subsequent
// calls to this function will just retrieve those fields directly.
func (self *Dummy) materializeTool(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) error {

	if self.Client == nil {
		return errors.New("HTTP Client not configured")
	}

	// If we are downloading from github we have to resolve and
	// verify the binary URL now.
	if tool.GithubProject != "" {
		var err error
		tool.Url, err = getGithubRelease(ctx, self.Client, config_obj, tool)
		if err != nil {
			return fmt.Errorf("While resolving github release %v: %w",
				tool.GithubProject, err)
		}
	}

	// We have no idea where the file is.
	if tool.Url == "" {
		return fmt.Errorf(
			"Tool %v has no url defined - upload it manually.", tool.Name)
	}

	fd, err := self.getTempFile(config_obj, tool.Filename, tool.Url)
	if err != nil {
		return err
	}
	defer fd.Close()

	err = fd.Truncate(0)
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.GenericComponent)
	logger.Info("Downloading tool <green>%v</> FROM <cyan>%v</>", tool.Name, tool.Url)
	request, err := http.NewRequestWithContext(ctx, "GET", tool.Url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", constants.USER_AGENT)
	res, err := self.Client.Do(request)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// If the download failed, we can not store this tool.
	if res.StatusCode != 200 {
		return fmt.Errorf("Unable to download file from %v: %v",
			tool.Url, res.Status)
	}
	sha_sum := sha256.New()

	_, err = utils.Copy(ctx, fd, io.TeeReader(res.Body, sha_sum))
	if err == nil {
		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	tool.ServePath = fd.Name()
	tool.Filename = filepath.Base(fd.Name())

	return nil
}

func getGithubRelease(ctx context.Context, Client networking.HTTPClient,
	config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) (string, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest",
		tool.GithubProject)
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	request.Header.Set("User-Agent", constants.USER_AGENT)
	logger.Info("Resolving latest Github release for <green>%v</>", tool.Name)
	res, err := Client.Do(request)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("Error: %v", res.Status)
	}

	response, err := utils.ReadAllWithLimit(res.Body, constants.MAX_MEMORY)
	if err != nil {
		return "", fmt.Errorf(
			"While making Github API call to %v: %w ", url, err)
	}

	api_obj := &githubReleasesAPI{}
	err = json.Unmarshal(response, &api_obj)
	if err != nil {
		return "", fmt.Errorf(
			"While making Github API call to  %v: %w ", url, err)
	}

	release_re, err := regexp.Compile(tool.GithubAssetRegex)
	if err != nil {
		return "", err
	}

	for _, asset := range api_obj.Assets {
		if release_re.MatchString(asset.Name) {
			logger.Info("Tool <green>%v</> can be found at <cyan>%v</>",
				tool.Name, asset.BrowserDownloadUrl)
			return asset.BrowserDownloadUrl, nil
		}
	}

	return "", errors.New("Release not found from github API " + url)
}

func (self *Dummy) AddTool(
	ctx context.Context, config_obj *config_proto.Config,
	tool_request *artifacts_proto.Tool,
	opts services.ToolOptions) error {
	if opts.Upgrade {
		existing_tool, err := self.ProbeToolInfo(
			ctx, config_obj, tool_request.Name,
			tool_request.Version)
		if err == nil {
			// Ignore the request if the existing
			// definition is better than the new one.
			if isDefinitionBetter(existing_tool, tool_request) {
				return nil
			}
		}
	}

	if opts.AdminOverride {
		tool_request.AdminOverride = true
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	// Obfuscate the public directory path.
	// Make a copy to work on.
	tool := proto.Clone(tool_request).(*artifacts_proto.Tool)
	// Replace the tool in the inventory.
	found := false
	for i, item := range self.binaries.Tools {
		// Definition has a version
		if tool.Version != "" {
			if item.Name == tool.Name && item.Version == tool.Version {
				found = true
				self.binaries.Tools[i] = tool
				break
			}
			continue
		}

		// Definition does not have a version - equivalent to an empty
		// version.
		if item.Name == tool.Name {
			found = true
			self.binaries.Tools[i] = tool
			break
		}
	}

	if !found {
		self.binaries.Tools = append(self.binaries.Tools, tool)
	}

	self.binaries.Version = uint64(self.Clock.Now().UnixNano())

	return nil
}

func (self *Dummy) RemoveTool(
	config_obj *config_proto.Config, tool_name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	tools := []*artifacts_proto.Tool{}
	for _, item := range self.binaries.Tools {
		if item.Name != tool_name {
			tools = append(tools, item)
		}
	}

	self.binaries.Tools = tools

	return nil
}

func NewInventoryDummyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Inventory, error) {

	inventory_service := &Dummy{
		Clock:    utils.RealClock{},
		binaries: &artifacts_proto.ThirdParty{},
		Client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   300 * time.Second,
					KeepAlive: 300 * time.Second,
					DualStack: true,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       300 * time.Second,
				TLSHandshakeTimeout:   100 * time.Second,
				ExpectContinueTimeout: 10 * time.Second,
				ResponseHeaderTimeout: 100 * time.Second,
			},
		},
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inventory_service.Close(config_obj)

		<-ctx.Done()
	}()

	logger := logging.GetLogger(config_obj, &logging.GenericComponent)
	logger.Info("Installing <green>Dummy inventory_service</>. Will download tools to temp directory.")

	return inventory_service, nil
}

type DummyHTTPClient struct{}

func (self DummyHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return nil, errors.New("External tool access is disabled on this server. You can try to manually upload tools in the Tool Setup GUI")
}
