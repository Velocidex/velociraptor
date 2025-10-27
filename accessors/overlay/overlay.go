/* The overlay accessor merges a number of other paths */

package overlay

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/utils/dict"
)

type OverlayFileSystemAccessorArgs struct {
	Paths    []*accessors.OSPath `vfilter:"required,field=paths,doc=A list of paths to try to resolve each path."`
	Accessor string              `vfilter:"optional,field=accessor,doc=File accessor"`
}

type OverlayFileSystemAccessor struct {
	ctx   context.Context
	scope vfilter.Scope
}

func (self OverlayFileSystemAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self OverlayFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	result := &OverlayFileSystemAccessor{
		ctx:   context.TODO(),
		scope: scope,
	}
	return result, nil
}

func (self OverlayFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "overlay",
		Description: `Merges several paths into a single path.`,
		// Permissions are actually enforced by the delegated accessors
		Permissions: []acls.ACL_PERMISSION{},
		ScopeVar:    constants.OVERLAY_ACCESSOR_DELEGATES,
		ArgType:     &OverlayFileSystemAccessorArgs{},
	}
}

func (self OverlayFileSystemAccessor) ReadDir(
	path string) ([]accessors.FileInfo, error) {

	parsed_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(parsed_path)
}

func (self OverlayFileSystemAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) (res []accessors.FileInfo, err error) {

	overlayer, err := GetOverlayConfig(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	accessor, err := accessors.GetAccessor(overlayer.Accessor, self.scope)
	if err != nil {
		return nil, err
	}

	base, _ := accessors.NewGenericOSPath("/")

	for _, basepath := range overlayer.Paths {
		delegate_path := basepath.Append(path.Components...)
		delegate_dir, err := accessor.ReadDirWithOSPath(delegate_path)
		if err != nil {
			continue
		}
		for _, fsinfo := range delegate_dir {
			res = append(res, accessors.NewFileInfoWrapper(
				fsinfo, base, basepath.Copy()))
		}
	}

	return res, nil
}

func (self OverlayFileSystemAccessor) OpenWithOSPath(
	path *accessors.OSPath) (res accessors.ReadSeekCloser, res_err error) {

	overlayer, err := GetOverlayConfig(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	accessor, err := accessors.GetAccessor(overlayer.Accessor, self.scope)
	if err != nil {
		return nil, err
	}

	for _, basepath := range overlayer.Paths {
		res, res_err = accessor.OpenWithOSPath(
			basepath.Append(path.Components...))
		// Return the first successful opened file
		if res_err == nil {
			break
		}
	}

	if res_err == nil && res == nil {
		return nil, utils.NotFoundError
	}
	return res, res_err
}

func (self OverlayFileSystemAccessor) Open(
	filename string) (accessors.ReadSeekCloser, error) {

	parsed_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(parsed_path)
}

func (self OverlayFileSystemAccessor) Lstat(path string) (accessors.FileInfo, error) {

	parsed_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(parsed_path)
}

func (self OverlayFileSystemAccessor) LstatWithOSPath(
	path *accessors.OSPath) (res accessors.FileInfo, res_err error) {

	overlayer, err := GetOverlayConfig(self.ctx, self.scope)
	if err != nil {
		return nil, err
	}

	accessor, err := accessors.GetAccessor(overlayer.Accessor, self.scope)
	if err != nil {
		return nil, err
	}

	for _, basepath := range overlayer.Paths {
		res, res_err = accessor.LstatWithOSPath(
			basepath.Append(path.Components...))
		// Return the first successful opened file
		if res_err == nil {
			break
		}
	}

	if res_err == nil && res == nil {
		return nil, utils.NotFoundError
	}
	return res, res_err
}

func init() {
	accessors.Register(&OverlayFileSystemAccessor{})
}

func GetOverlayConfig(
	ctx context.Context,
	scope vfilter.Scope) (res *OverlayFileSystemAccessorArgs, err error) {

	setting, pres := scope.Resolve(constants.OVERLAY_ACCESSOR_DELEGATES)
	if !pres {
		setting = ordereddict.NewDict()
	}

	args := dict.RowToDict(ctx, scope, setting)
	arg := &OverlayFileSystemAccessorArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		return nil, err
	}

	return arg, nil
}
