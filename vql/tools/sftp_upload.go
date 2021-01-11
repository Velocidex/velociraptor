//+build extras

package tools

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type SFTPUploadArgs struct {
	File       string `vfilter:"required,field=file,doc=The file to upload"`
	Name       string `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	User       string `vfilter:"required,field=user,doc=The username to connect to the endpoint with"`
	Path       string `vfilter:"required,field=path,doc=Path on server to upload file to"`
	Accessor   string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	PrivateKey string `vfilter:"required,field=privatekey,doc=The private key to use"`
	Endpoint   string `vfilter:"required,field=endpoint,doc=The Endpoint to use"`
	HostKey    string `vfilter:"optional,field=hostkey,doc=Host key to verify. Blank to disable"`
}

type SFTPUploadFunction struct{}

func (self *SFTPUploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &SFTPUploadArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("upload_sftp: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("upload_SFTP: %s", err)
		return vfilter.Null{}
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_SFTP: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.Open(arg.File)
	if err != nil {
		scope.Log("upload_SFTP: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File
	}

	stat, err := file.Stat()
	if err != nil {
		scope.Log("upload_SFTP: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		// Abort uploading when the scope is destroyed.
		sub_ctx, cancel := context.WithCancel(ctx)
		_ = scope.AddDestructor(cancel)

		upload_response, err := upload_SFTP(
			sub_ctx, scope, file,
			arg.User,
			arg.Path,
			arg.Name,
			arg.PrivateKey,
			arg.Endpoint,
			arg.HostKey)
		if err != nil {
			scope.Log("upload_SFTP: %v", err)
			// Relay the error in the UploadResponse
			return upload_response
		}
		return upload_response
	}

	return vfilter.Null{}
}

func keyString(k ssh.PublicKey) string {
	return k.Type() + " " + base64.StdEncoding.EncodeToString(k.Marshal())
}

func hostkeycallback(trustedkey string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, k ssh.PublicKey) error {
		ks := keyString(k)
		if trustedkey != ks {
			return fmt.Errorf("SSH-key verification: expected %s but got %s", trustedkey, ks)
		}
		return nil
	}
}

func getSFTPClient(scope vfilter.Scope, user string, privateKey string, endpoint string, hostKey string) (*sftp.Client, error) {
	cacheKey := fmt.Sprintf("%s %s", user, endpoint)
	client := vql_subsystem.CacheGet(scope, cacheKey)
	if client == nil {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			vql_subsystem.CacheSet(scope, cacheKey, err)
			return nil, err
		}
		var clientConfig *ssh.ClientConfig
		if hostKey == "" {
			clientConfig = &ssh.ClientConfig{
				User: user,
				Auth: []ssh.AuthMethod{
					ssh.PublicKeys(signer),
				},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			}
		} else {
			clientConfig = &ssh.ClientConfig{
				User: user,
				Auth: []ssh.AuthMethod{
					ssh.PublicKeys(signer),
				},
				HostKeyCallback: hostkeycallback(hostKey),
			}
		}

		conn, err := ssh.Dial("tcp", endpoint, clientConfig)
		if err != nil {
			vql_subsystem.CacheSet(scope, cacheKey, err)
			return nil, err
		}
		client, err := sftp.NewClient(conn)
		if err != nil {
			vql_subsystem.CacheSet(scope, cacheKey, err)
			return nil, err
		}

		remove := func() {
			conn.Close()
			client.Close()
		}
		err = vql_subsystem.GetRootScope(scope).AddDestructor(remove)
		if err != nil {
			remove()
			return nil, err
		}

		vql_subsystem.CacheSet(scope, cacheKey, client)
		return client, nil
	}
	switch t := client.(type) {
	case error:
		return nil, t
	case *sftp.Client:
		return t, nil
	default:
		return nil, errors.New("Error")
	}
}

func upload_SFTP(ctx context.Context, scope vfilter.Scope,
	reader io.Reader,
	user, path, name string,
	privateKey string, endpoint string, hostKey string) (
	*api.UploadResponse, error) {

	scope.Log("upload_SFTP: Uploading %v to %v", name, endpoint)
	client, err := getSFTPClient(scope, user, privateKey, endpoint, hostKey)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	fpath := filepath.Join(path, name)
	file, err := client.Create(fpath)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}
	defer file.Close()
	if _, err := file.ReadFrom(reader); err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	check, err := client.Lstat(fpath)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	response := &api.UploadResponse{
		Path: fpath,
		Size: uint64(check.Size()),
	}
	return response, nil
}

func (self SFTPUploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "upload_sftp",
		Doc:     "Upload files to SFTP.",
		ArgType: type_map.AddType(scope, &SFTPUploadArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SFTPUploadFunction{})
}
