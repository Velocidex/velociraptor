package collector

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"

	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// An accessor for reading collector containers. The Offline collector
// is designed to store files in a zip container. However the zip
// format is not capable of storing certain attributes (e.g. sparse
// files).  The accessor is designed to read the offline collector
// containers.

// This accessor wraps the zip accessor to provide access to these
// specially formated conatiners. In particular the collector accessor
// handles the following two properties transparently:

// 1. Zip encryption: Velociraptor uses an ecnryption scheme to work
// around Zip encryption limitations. All data is stored in an
// encrypted file called "data.zip" inside the main zip archive. This
// is because Zip encryption does not protect the central directory or
// the filenames - only data content.

// 2. Public Key Encryption: Velociraptor stored metadata in a
// "metadata.json" file containing the encrypted session key. This
// allows private/public key encryption and transparent decryption.

// This accessor facilitates this transparent decryption.

type rangedReader struct {
	delegate io.ReaderAt
	fd       accessors.ReadSeekCloser
	offset   int64
}

func (self *rangedReader) Read(buff []byte) (int, error) {
	n, err := self.delegate.ReadAt(buff, self.offset)
	self.offset += int64(n)
	return n, err
}

func (self *rangedReader) Seek(offset int64, whence int) (int64, error) {
	self.offset = offset
	return self.offset, nil
}

func (self *rangedReader) Close() error {
	return self.fd.Close()
}

type StatWrapper struct {
	accessors.FileInfo
	real_size int64
}

func (self StatWrapper) Size() int64 {
	return self.real_size
}

func (self StatWrapper) Mode() os.FileMode {
	if self.real_size == 0 {
		return os.FileMode(0755)
	}
	return self.FileInfo.Mode()
}

func (self StatWrapper) IsDir() bool {
	return self.real_size == 0
}

type CollectorAccessor struct {
	*zip.ZipFileSystemAccessor
	scope vfilter.Scope

	// If set we automatically pad out sparse files.
	expandSparse bool
}

func (self *CollectorAccessor) New(scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	delegate, err := (&zip.ZipFileSystemAccessor{}).New(scope)
	return &CollectorAccessor{
		expandSparse:          self.expandSparse,
		ZipFileSystemAccessor: delegate.(*zip.ZipFileSystemAccessor),
		scope:                 scope,
	}, err
}

/*
  Go from a pathspec like:

  PathSpec{
    Path: "path/within/zip",
    DelegatePath: "/path/to/zip/collection",
    DelegateAccessor: "accssor_to_zip_collection",
  }

  To a pathspec like
  PathSpec{
     Path: "path/within/zip",
     DelegateAccessor: "collector",
     DelegatePath: PathSpec{
        Path: "data.zip",
        DelegatePath: "/path/to/zip/collection",
        DelegateAccessor: "accssor_to_zip_collection",
     },
  }

*/

func collectorPathToDelegatePath(full_path *accessors.OSPath) *accessors.OSPath {
	// Detect an already transformed path and leave it alone.
	if len(full_path.Components) == 1 &&
		full_path.Components[0] == "data.zip" {
		return full_path
	}

	if full_path.DelegateAccessor() == "collector" {
		return full_path
	}

	collector_pathspec := full_path.PathSpec()

	res := full_path.Copy()
	res.SetPathSpec(&accessors.PathSpec{
		Path:             collector_pathspec.Path,
		DelegateAccessor: "collector",
		DelegatePath: accessors.PathSpec{
			DelegateAccessor: collector_pathspec.DelegateAccessor,
			DelegatePath:     collector_pathspec.DelegatePath,
			Path:             "data.zip",
		}.String(),
	})
	res.Components = full_path.Components

	return res
}

// Try to set a password if it exists in metadata
func (self *CollectorAccessor) maybeSetZipPassword(
	full_path *accessors.OSPath) (*accessors.OSPath, error) {

	// Password is already cached in the context - just return it as is.
	_, pres := self.scope.GetContext(constants.ZIP_PASSWORDS)
	if pres {

		// Transform the path so it is ready to be used by the zip
		// accessor.
		return collectorPathToDelegatePath(full_path), nil
	}

	// If password is already set in the scope, just use it as it is.
	pass, pres := self.scope.Resolve(constants.ZIP_PASSWORDS)
	if pres && !utils.IsNil(pass) {
		return collectorPathToDelegatePath(full_path), nil
	}

	// Check if data.zip exists at the top level.
	root := full_path.Copy()
	root.Components = nil

	datazip := root.Append("data.zip")
	_, err := self.ZipFileSystemAccessor.LstatWithOSPath(datazip)
	if err != nil {
		// Nope - no data.zip so do not transform the pathspec.
		return full_path, nil
	}

	// Check if metadata.json exists. If so, try to extract password
	meta := root.Append("metadata.json")
	mhandle, err := self.ZipFileSystemAccessor.OpenWithOSPath(meta)
	if err != nil {
		// No metadata file is found - this might be a plain
		// collection zip.
		return full_path, nil
	}

	buf, err := ioutil.ReadAll(mhandle)
	if err != nil {
		return nil, err
	}

	rows := []*ordereddict.Dict{}
	err = json.Unmarshal(buf, &rows)
	if err != nil {
		return nil, err
	}

	// metadata.json can be multiple rows
	for _, row := range rows {
		scheme, ok := row.GetString("Scheme")
		if !ok {
			// Maybe multiple rows?
			continue
		}

		if strings.ToLower(scheme) == "x509" {
			ep, ok := row.GetString("EncryptedPass")
			if !ok {
				return nil, errors.New(
					"EncryptedPass must be given and be of type string!")
			}

			err = vql_subsystem.CheckAccess(self.scope, acls.SERVER_ADMIN)
			if err != nil {
				return nil, errors.New(
					"Must be server admin to use private key")
			}

			key, err := crypto_utils.GetPrivateKeyFromScope(self.scope)
			if err != nil {
				return nil, err
			}

			zip_pass, err := crypto_utils.Base64DecryptRSAOAEP(key, ep)
			if err != nil {
				return nil, err
			}

			self.scope.SetContext(constants.ZIP_PASSWORDS, string(zip_pass))
			value, pres := self.scope.Resolve(constants.REPORT_ZIP_PASSWORD)
			if pres && self.scope.Bool(value) {
				self.scope.Log("CollectorAccessor: X509 Decrypted password is %q",
					string(zip_pass))
			}
			// Transform the path so it can be used by the zip
			// collector.
			return collectorPathToDelegatePath(full_path), nil
		}
	}

	// No metadata found - this might be a plain unencrypted
	// collection.
	return full_path, nil
}

// Zip files typically use standard / path separators.
func (self *CollectorAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewZipFilePath(path)
}

func (self *CollectorAccessor) Open(
	filename string) (accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *CollectorAccessor) getIndex(
	full_path *accessors.OSPath) (*actions_proto.Index, error) {

	// Does the file have an idx file?
	idx_reader, err := self.ZipFileSystemAccessor.OpenWithOSPath(
		full_path.Dirname().Append(full_path.Basename() + ".idx"))
	if err != nil {
		return nil, err
	}

	serialized, err := ioutil.ReadAll(idx_reader)
	if err != nil {
		return nil, err
	}

	index := &actions_proto.Index{}
	err = json.Unmarshal(serialized, index)
	if err != nil {
		// Older versions stored idx as a JSONL file instead.
		for _, l := range strings.Split(string(serialized), "\n") {
			if len(l) > 2 {
				r := &actions_proto.Range{}
				err = json.Unmarshal([]byte(l), r)
				if err != nil {
					return nil, err
				}
				index.Ranges = append(index.Ranges, r)
			}
		}
	}

	if len(index.Ranges) == 0 {
		return nil, errors.New("No ranges")
	}

	return index, err
}

func (self *CollectorAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	updated_full_path, err := self.maybeSetZipPassword(full_path)
	if err != nil {
		self.scope.Log(err.Error())
	}

	reader, err := self.ZipFileSystemAccessor.OpenWithOSPath(updated_full_path)
	if err != nil {
		return nil, err
	}

	if self.expandSparse {
		index, err := self.getIndex(updated_full_path)
		if err == nil {
			config_obj, ok := vql_subsystem.GetServerConfig(self.scope)
			if !ok {
				config_obj = &config_proto.Config{}
			}

			if !uploads.ShouldPadFile(config_obj, index) {
				self.scope.Log("Error: File %v is too sparse - unable to expand it.", full_path)
				return reader, nil
			}

			return &rangedReader{
				delegate: &utils.RangedReader{
					ReaderAt: utils.MakeReaderAtter(reader),
					Index:    index,
				},
				fd: reader,
			}, nil
		}
	}
	return reader, nil
}

func (self *CollectorAccessor) Lstat(file_path string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *CollectorAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	updated_full_path, err := self.maybeSetZipPassword(full_path)
	if err != nil {
		self.scope.Log(err.Error())
	}
	stat, err := self.ZipFileSystemAccessor.LstatWithOSPath(updated_full_path)
	if err != nil {
		return nil, err
	}

	index, err1 := self.getIndex(updated_full_path)
	if err1 == nil {
		real_size := int64(0)
		for _, r := range index.Ranges {
			real_size = r.OriginalOffset + r.Length
		}

		return StatWrapper{
			FileInfo:  stat,
			real_size: real_size,
		}, nil
	}

	return stat, err
}

func (self *CollectorAccessor) ReadDir(
	file_path string) ([]accessors.FileInfo, error) {

	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self *CollectorAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {

	updated_full_path, err := self.maybeSetZipPassword(full_path)
	if err != nil {
		return nil, err
	}

	res, err := self.ZipFileSystemAccessor.ReadDirWithOSPath(
		updated_full_path)
	if err != nil {
		return nil, err
	}

	for i := range res {
		res[i] = StatWrapper{
			FileInfo:  res[i],
			real_size: res[i].Size(),
		}
	}

	return res, nil
}

func init() {
	accessors.Register("collector", &CollectorAccessor{
		expandSparse: true,
	}, `Open a collector zip file as if it was a directory - automatically expand sparse files.`)

	accessors.Register("collector_sparse", &CollectorAccessor{
		expandSparse: false,
	}, `Open a collector zip file as if it was a directory - does not expand sparse files.`)
}
