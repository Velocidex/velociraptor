//go:build extras
// +build extras

package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SFTPUploadArgs struct {
	File       *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name       string            `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server (may contain the path)"`
	User       string            `vfilter:"required,field=user,doc=The username to connect to the endpoint with"`
	Path       string            `vfilter:"optional,field=path,doc=Path on server to upload file to (will be prepended to name)"`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	PrivateKey string            `vfilter:"required,field=privatekey,doc=The private key to use"`
	Endpoint   string            `vfilter:"required,field=endpoint,doc=The Endpoint to use including port number (e.g. 192.168.1.1:22 )"`
	HostKey    string            `vfilter:"optional,field=hostkey,doc=Host key to verify. Blank to disable"`
}

type SFTPUploadFunction struct{}

func (self *SFTPUploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload_sftp", args)()

	arg := &SFTPUploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_sftp: %s", err.Error())
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_SFTP: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_SFTP: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File.String()
	}

	stat, err := accessor.LstatWithOSPath(arg.File)
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

func getSFTPClient(scope vfilter.Scope, user string, privateKey string,
	endpoint string, hostKey string) (*sftp.Client, error) {
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
	user, filepath, name string,
	privateKey string, endpoint string, hostKey string) (
	*uploads.UploadResponse, error) {

	scope.Log("upload_SFTP: Uploading %v to %v", name, endpoint)
	client, err := getSFTPClient(scope, user, privateKey, endpoint, hostKey)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	// The sftp spec requires a forward slash for separators, but some
	// servers also accept backslash while some do not. To be safe we
	// use the unix join in all cases.
	fpath := path.Join(filepath, name)
	file, err := client.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}
	if _, err := file.ReadFrom(reader); err != nil {
		scope.Log("upload_SFTP: while reading file: %v", err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	file.Close()
	check, err := client.Lstat(fpath)
	if e, ok := err.(*sftp.StatusError); ok && e.FxCode() == sftp.ErrSSHFxPermissionDenied {
		scope.Log("upload_SFTP: Unable to verify size of uploaded file due to insufficient read permissions.")
		response := &uploads.UploadResponse{
			Path: fpath,
		}
		return response, nil
	} else if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	response := &uploads.UploadResponse{
		Path: fpath,
		Size: uint64(check.Size()),
	}
	return response, nil
}

func (self SFTPUploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "upload_sftp",
		Doc:      "Upload files to SFTP.",
		ArgType:  type_map.AddType(scope, &SFTPUploadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SFTPUploadFunction{})
}
