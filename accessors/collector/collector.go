package collector

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
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

type CollectorAccessor struct {
	*zip.ZipFileSystemAccessor
	scope           vfilter.Scope
	passwordChecked bool
	isEncrypted     bool
}

func (self *CollectorAccessor) New(scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	delegate, err := (&zip.ZipFileSystemAccessor{}).New(scope)
	return &CollectorAccessor{
		ZipFileSystemAccessor: delegate.(*zip.ZipFileSystemAccessor),
		scope:                 scope,
	}, err
}

// Try to set a password if it exists in metadata
func (self *CollectorAccessor) maybeSetZipPassword(full_path *accessors.OSPath) error {
	if self.passwordChecked {
		return nil
	}
	self.passwordChecked = true
	ps := full_path.PathSpec().Copy()
	if !strings.HasSuffix(ps.Path, "zip") {
		return nil
	}
	ps.DelegatePath = ps.Path
	ps.Path = "metadata.json"

	meta, err := self.ParsePath(ps.String())
	if err != nil {
		return err
	}

	// Check if metadata.json exists. If so, try to extract password
	if finfo, _ := self.LstatWithOSPath(meta); finfo != nil {
		mhandle, err := self.OpenWithOSPath(meta)
		if err != nil {
			return err
		}
		buf, err := ioutil.ReadAll(mhandle)
		if err != nil {
			return err
		}
		rows := []*ordereddict.Dict{}
		err = json.Unmarshal(buf, &rows)
		if err != nil {
			return err
		}
		// metadata.json can be multiple rows
		for _, row := range rows {
			scheme, _ := row.Get("Scheme")
			scheme_str, ok := scheme.(string)
			if !ok {
				// Maybe multiple rows?
				continue
			}
			if strings.ToLower(scheme_str) == "x509" {
				ep, _ := row.Get("EncryptedPass")

				ep_str, ok := ep.(string)
				if !ok {
					return errors.New("EncryptedPass must be of type string!")
				}

				err = vql_subsystem.CheckAccess(self.scope, acls.SERVER_ADMIN)
				if err != nil {
					return errors.New("Must be server admin to use private key")
				}

				key, err := crypto_utils.GetPrivateKeyFromScope(self.scope)
				if err != nil {
					return err
				}

				zip_pass, err := crypto_utils.Base64DecryptRSAOAEP(key, ep_str)
				if err != nil {
					return err
				}
				self.scope.SetContext(constants.ZIP_PASSWORDS, string(zip_pass))
				self.isEncrypted = true
				return nil
			}
		}
	}
	return nil
}

// Update pathspec if encrypted to automatically read data.zip
func (self *CollectorAccessor) updatePathSpec(full_path *accessors.OSPath) {
	path_spec_override := full_path.PathSpec()
	if strings.HasSuffix(path_spec_override.Path, "zip") {
		path_spec_override.DelegatePath = path_spec_override.Path
		path_spec_override.Path = "data.zip"

	}
	full_path.SetPathSpec(path_spec_override)
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
	err := self.maybeSetZipPassword(full_path)
	if err != nil {
		self.scope.Log("OpenWithOSPath: %s, %s", full_path.String(), err.Error())
	}

	if self.isEncrypted {
		self.updatePathSpec(full_path)
	}

	reader, err := self.ZipFileSystemAccessor.OpenWithOSPath(full_path)
	if err != nil {
		return nil, err
	}

	index, err := self.getIndex(full_path)
	if err == nil {
		return &rangedReader{
			delegate: &utils.RangedReader{
				ReaderAt: utils.MakeReaderAtter(reader),
				Index:    index,
			},
			fd: reader,
		}, nil
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

	err := self.maybeSetZipPassword(full_path)
	if err != nil {
		self.scope.Log("LstatWithOSPath: %s", err.Error())
	}
	if self.isEncrypted {
		self.updatePathSpec(full_path)
	}
	stat, err := self.ZipFileSystemAccessor.LstatWithOSPath(full_path)
	if err != nil {
		return nil, err
	}

	index, err1 := self.getIndex(full_path)
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

func init() {
	accessors.Register("collector", &CollectorAccessor{},
		`Open a collector zip file as if it was a directory.`)
}
