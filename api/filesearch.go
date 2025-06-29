package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"regexp"
	"strings"

	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	unsupportedSearchType = errors.New("Unsupported Search Type")
)

func (self *ApiServer) SearchFile(ctx context.Context,
	in *api_proto.SearchFileRequest) (*api_proto.SearchFileResponse, error) {

	defer Instrument("SearchFile")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to search files.")
	}

	if len(in.VfsComponents) == 0 {
		return nil, PermissionDenied(err, "No file specified")
	}

	matcher, err := newMatcher(in.Term, in.Type)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	path_spec := path_specs.NewUnsafeFilestorePath(in.VfsComponents...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	file, err := file_store.GetFileStore(org_config_obj).ReadFile(path_spec)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	defer file.Close()

	var reader_at io.ReaderAt = utils.MakeReaderAtter(file)
	index, err := getIndex(org_config_obj, path_spec)

	// If the file is sparse, we use the sparse reader.
	if err == nil && in.Padding && len(index.Ranges) > 0 {
		if !uploads.ShouldPadFile(org_config_obj, index) {
			return nil, Status(self.verbose, errors.New(
				"Sparse file is too sparse - unable to pad"))
		}

		reader_at = &utils.RangedReader{
			ReaderAt: reader_at,
			Index:    index,
		}
	}

	offset := int64(in.Offset)
	var buf []byte

	if in.Forward {
		buf = pool.Get().([]byte)
		defer pool.Put(buf)

		// To search backwards we need to rewind before the current
		// offset. We may hit the start of the file, in which case we will
		// get a smaller buffer.
	} else {
		base_offset := offset - BUFSIZE

		// The buffer is too small for the pool buffer - just allocate
		// it from heap.
		if base_offset < 0 {
			buf = make([]byte, offset)
			base_offset = 0

		} else {
			// Buffer is large enough so we can use the pool.
			buf = pool.Get().([]byte)
			defer pool.Put(buf)
		}

		// Offset now reflects the start of the buffer.
		offset = base_offset
	}

	for {
		// Allow for cancellations
		select {
		case <-ctx.Done():
			return &api_proto.SearchFileResponse{}, nil
		default:
		}

		n, err := reader_at.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return nil, Status(self.verbose, err)
		}
		if n <= 0 {
			return &api_proto.SearchFileResponse{}, nil
		}

		if in.Forward {
			hit := matcher.index(buf[:n])
			if hit >= 0 {
				return &api_proto.SearchFileResponse{
					VfsComponents: in.VfsComponents,
					Hit:           uint64(hit + offset),
				}, nil
			}
			offset += int64(n)

		} else {
			hit := matcher.last_index(buf[:n])
			if hit >= 0 {
				return &api_proto.SearchFileResponse{
					VfsComponents: in.VfsComponents,
					Hit:           uint64(hit + offset),
				}, nil
			}
			offset -= int64(n)

			// Offset went backwards before the start of the file - we
			// didnt find it.
			if offset < 0 {
				return &api_proto.SearchFileResponse{}, nil
			}
		}
	}
}

type matcher interface {
	index(buff []byte) int64
	last_index(buff []byte) int64
}

type literal_matcher struct {
	bytes []byte
}

func (self *literal_matcher) index(buff []byte) int64 {
	return int64(bytes.Index(buff, self.bytes))
}

func (self *literal_matcher) last_index(buff []byte) int64 {
	return int64(bytes.LastIndex(buff, self.bytes))
}

type regex_matcher struct {
	regex *regexp.Regexp
}

func (self *regex_matcher) index(buff []byte) int64 {
	match := self.regex.FindIndex(buff)
	if len(match) == 0 {
		return -1
	}
	return int64(match[0])
}

// This is not super efficient for now because there is no easy way to
// regex search from the end.
func (self *regex_matcher) last_index(buff []byte) int64 {
	matches := self.regex.FindAllIndex(buff, 1000)
	if len(matches) == 0 {
		return -1
	}
	last_match := matches[len(matches)-1]
	return int64(last_match[0])
}

func newMatcher(term, search_type string) (matcher, error) {
	switch search_type {
	case "", "string":
		return &literal_matcher{[]byte(term)}, nil

	case "regex":
		re, err := regexp.Compile("(?ism)" + term)
		if err != nil {
			return nil, err
		}
		return &regex_matcher{re}, nil

	case "hex":
		str := strings.Replace(term, " ", "", -1)

		hex, err := hex.DecodeString(strings.TrimPrefix(str, "0x"))
		if err != nil {
			return nil, err
		}

		// If the string has a 0x prefix, we assume it means a little
		// endian integer so we need to reverse it.
		if strings.HasPrefix(term, "0x") {
			reversed := make([]byte, 0, len(hex))
			for i := len(hex) - 1; i >= 0; i-- {
				reversed = append(reversed, hex[i])
			}
			hex = reversed
		}

		return &literal_matcher{hex}, nil

	default:
		return nil, unsupportedSearchType
	}
}
