//go:build windows
// +build windows

package ntfs

import (
	"context"
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	vconstants "www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/constants"
	"www.velocidex.com/golang/vfilter"
)

const (
	VSS_TAG = "$__NTFS_VSS_Accessor"
)

type WindowsVSSFileSystemAccessor struct {
	*WindowsNTFSFileSystemAccessor

	// A list of the roots we are interested in.
	vss_roots []*accessors.OSPath
}

func (self WindowsVSSFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	// Cache the ntfs accessor for the life of the query.
	cache_time := constants.GetNTFSCacheTime(context.Background(), scope)
	root_scope := vql_subsystem.GetRootScope(scope)
	cached_accessor, ok := vql_subsystem.CacheGet(
		root_scope, VSS_TAG).(*WindowsVSSFileSystemAccessor)

	// Ignore the filesystem if it is too old - drives may have been
	// added or removed.
	if ok && cached_accessor.age.Add(cache_time).After(time.Now()) {
		return cached_accessor, nil
	}

	// TODO: Add mechanism for the user to restrict VSS range by time
	// of interest.
	base_fs_any, err := self.WindowsNTFSFileSystemAccessor.New(scope)
	if err != nil {
		return nil, err
	}

	base_fs := base_fs_any.(*WindowsNTFSFileSystemAccessor)

	result := &WindowsVSSFileSystemAccessor{
		WindowsNTFSFileSystemAccessor: base_fs,
	}

	roots, err := result.getVSSRoots(scope)
	if err != nil {
		return nil, err
	}
	result.vss_roots = roots

	vql_subsystem.CacheSet(root_scope, VSS_TAG, result)
	return result, nil
}

func (self *WindowsVSSFileSystemAccessor) getVSSRoots(
	scope vfilter.Scope) ([]*accessors.OSPath, error) {

	var result []*accessors.OSPath

	max_age := vql_subsystem.GetFloatFromRow(
		scope, scope, vconstants.VSS_MAX_AGE_DAYS)

	root_path, _ := accessors.NewWindowsNTFSPath("")
	roots, err := self.WindowsNTFSFileSystemAccessor.ReadDirWithOSPath(root_path)
	if err != nil {
		return nil, err
	}

	for _, r := range roots {
		device, _ := r.Data().GetString("DeviceObject")
		if strings.Contains(device, "HarddiskVolumeShadowCopy") {
			install_date, _ := r.Data().GetString("InstallDate")
			if len(install_date) < 14 {
				continue
			}

			parsed, err := time.Parse("20060102150405", install_date[:14])
			if err != nil {
				scope.Log("ERROR: Unable to parse time %v", err)
				continue
			}

			if max_age > 0 {
				// Age is too long ago skip it
				if time.Now().Sub(parsed).Seconds() > max_age*24*60*60 {
					continue
				}
				scope.Log("vss: Found VSS %v created %v within Max Age of %v days\n",
					device, parsed.UTC().Format(time.RFC3339), max_age)
			} else {
				scope.Log("vss: Found VSS %v that  was created at %v\n",
					device, parsed.UTC().Format(time.RFC3339))
			}

			result = append(result, r.OSPath())
		}
	}

	return result, nil

}

// Merge the results from all shadows into a single list.
func (self *WindowsVSSFileSystemAccessor) ReadDirWithOSPath(
	fullpath *accessors.OSPath) (res []accessors.FileInfo, err error) {

	root_list, err := self.WindowsNTFSFileSystemAccessor.ReadDirWithOSPath(fullpath)
	if err != nil {
		return nil, err
	}

	if len(fullpath.Components) == 0 {
		return root_list, nil
	}

	by_mft_id := make(map[string][]accessors.FileInfo)
	for _, i := range root_list {
		mft_id, pres := i.Data().GetString("mft")
		if pres {
			existing, _ := by_mft_id[mft_id]
			existing = append(existing, VSSFileInfo{
				FileInfo: i,
				device:   fullpath.Components[0],
			})
			by_mft_id[mft_id] = existing
		}
	}

	// Now list each of the VSS and merge with the by_mft_id list.
	relative_path := fullpath.Components[1:]
	for _, root := range self.vss_roots {
		path := root.Append(relative_path...)

		files, err := self.WindowsNTFSFileSystemAccessor.ReadDirWithOSPath(path)
		if err != nil {
			// Path may not exist in this shadow.
			continue
		}

		for _, i := range files {
			mft_id, pres := i.Data().GetString("mft")
			if pres {
				existing, _ := by_mft_id[mft_id]
				if !self.file_found(existing, i) {
					existing = append(existing, VSSFileInfo{
						FileInfo: i,
						device:   root.Components[0],
					})
				}
				by_mft_id[mft_id] = existing
			}
		}
	}

	result := make([]accessors.FileInfo, 0, len(by_mft_id))
	for _, v := range by_mft_id {
		for _, item := range v {
			result = append(result, item)
		}
	}

	return result, err
}

// Search for the file needle in the file haystack
func (self *WindowsVSSFileSystemAccessor) file_found(
	haystack []accessors.FileInfo, needle accessors.FileInfo) bool {

	name := needle.Name()
	mtime := needle.Mtime().UnixNano()
	size := needle.Size()
	is_dir := needle.IsDir()

	for _, old := range haystack {
		// For directories we need to only return one version because
		// glob will descend it in any case (regardless of version).
		if is_dir && old.Name() == name {
			return true
		}

		if mtime == old.Mtime().UnixNano() && size == old.Size() {
			return true
		}
	}

	return false
}

type VSSFileInfo struct {
	accessors.FileInfo
	device string
}

// Needed for glob - present a unique name for the file for
// deduplication.
func (self VSSFileInfo) UniqueName() string {
	u, ok := self.FileInfo.(accessors.UniqueBasename)
	if ok {
		return self.device + u.UniqueName()
	}
	return self.device + self.FileInfo.Name()
}

func (self WindowsVSSFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "ntfs_vss",
		Description: `Access the NTFS filesystem by considering all VSS.`,
	}
}

func init() {
	accessors.Register(&WindowsVSSFileSystemAccessor{
		WindowsNTFSFileSystemAccessor: &WindowsNTFSFileSystemAccessor{},
	})
}
