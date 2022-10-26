package accessors

import (
	"net/url"

	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	InvalidPathSpec = errors.New("Invalid PathSpec")
)

/*
  A PathSpec is a more precise indication of a path to open a source
  of data. In Velociraptor, access to data is provided by the use of
  "Accessors" - a registered driver capable of reading data from
  certain sources.

  Accessors can delegate to other accessors using the PathSpec. This
  delegation allows an accessor to receive additional information in
  order to properly create the filesystem abstraction.

  For example, consider the "zip" accessor which is responsible for
  reading compressed archives. In order to retrieve a file inside the
  zip file, the accessor needs the following pieces of data:

  1. A delegate accessor to use to open the underlying zip file.
  2. A path to provide to the delegate accessor.
  3. The name of the zip member to open.

  For example the following path spec:

  {"Accessor": "file",
   "DelegatePath": "/tmp/file.zip",
   "Path": "zip_member.exe"}

  Provides all this information.

  PathSpecs are supposed to be serialized into strings and passed as
  the filename to plugins that require file paths. The PathSpec is
  just a more detailed path representation and is treated everywhere
  as a plain string (json encoded).

  Therefore the following path spec is valid for a recursive path

  {"Accessor": "zip",
   "DelegatePath": "{\"Accessor\": \"file\", \"DelegatePath\": \"/tmp/file.zip\", \"Path\": \"embedded.zip\"}",
   "Path": "zip_member.exe"}

  Given to the zip accessor, this PathSpec means to use the "zip"
  accessor to open a member "embedded.zip" inside a file
  "/tmp/file.zip", then to search within that embedded zip for a
  "zip_member.exe"

  For convenience, the PathSpec also supports a structured delegate so
  the following serialization is also valid.

  {"Accessor": "zip",
   "Delegate": {
      "Accessor": "file",
      "DelegatePath": "/tmp/file.zip",
      "Path": "embedded.zip"
   },
   "Path": "zip_member.exe"}

  ## Note:

  In previous versions, the PathSpec abstraction was provided by
  mapping URL parts to the fields above. This proved problematic
  because URL encoding is lossy and not robust enough for round
  tripping of all paths.

  It also produces difficult to read paths. The old URL way is
  deprecated but still supported - it will eventually be dropped.
*/
type PathSpec struct {
	DelegateAccessor string `json:"DelegateAccessor,omitempty"`
	DelegatePath     string `json:"DelegatePath,omitempty"`

	// This standard for DelegatePath above and allows a more
	// convenient way to pass recursive pathspecs down.
	Delegate *PathSpec `json:"Delegate,omitempty"`
	Path     string    `json:"Path,omitempty"`

	// Keep track of if the pathspec came from a URL based for
	// backwards compatibility.
	url_based bool
}

func (self PathSpec) Copy() *PathSpec {
	result := self
	if result.Delegate != nil {
		result.Delegate = result.Delegate.Copy()
	}

	return &result
}

func (self PathSpec) GetDelegatePath() string {
	// We allow the delegate path to be encoded as a nested pathspec
	// for covenience.
	if self.Delegate != nil {
		return self.Delegate.String()
	}
	return self.DelegatePath
}

func (self PathSpec) GetDelegateAccessor() string {
	return self.DelegateAccessor
}

func (self PathSpec) GetPath() string {
	return self.Path
}

func (self PathSpec) String() string {
	if self.url_based {
		result := url.URL{
			Scheme:   self.DelegateAccessor,
			Path:     self.DelegatePath,
			Fragment: self.Path,
		}

		return result.String()
	}

	return json.MustMarshalString(self)
}

func PathSpecFromString(parsed string) (*PathSpec, error) {
	if len(parsed) == 0 {
		return &PathSpec{}, nil
	}

	// It is a serialized JSON object.
	if parsed[0] == '{' {
		result := &PathSpec{}
		err := json.Unmarshal([]byte(parsed), result)
		return result, err
	}

	// It can be a URL
	parsed_url, err := url.Parse(parsed)
	if err != nil {
		return nil, InvalidPathSpec
	}

	// It looks like a windows path not a URL
	if len(parsed_url.Scheme) == 1 {
		return &PathSpec{
			DelegatePath: parsed,
		}, nil
	}

	// Support urls for backwards compatibility.
	return &PathSpec{
		DelegateAccessor: parsed_url.Scheme,
		DelegatePath:     parsed_url.Path,
		Path:             parsed_url.Fragment,
		url_based:        true,
	}, nil
}
