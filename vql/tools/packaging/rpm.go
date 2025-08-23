package packaging

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/json"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"github.com/google/rpmpack"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	// rpmArchMap maps ELF machine strings to RPM architectures
	// See https://github.com/torvalds/linux/blob/master/include/uapi/linux/elf-em.h
	//     https://fedoraproject.org/wiki/Architectures
	rpmArchMap = map[string]string{
		"EM_X86_64":  "x86_64",
		"EM_386":     "i386",
		"EM_AARCH64": "aarch64",
		"EM_RISCV":   "riscv64",
		"EM_ARM":     "armhfp",
		"EM_PPC64":   "ppc64le",
	}
)

func getRPMArch(exe_bytes []byte) (string, error) {
	reader := bytes.NewReader(exe_bytes)
	e, err := elf.NewFile(reader)
	if err != nil {
		return "", fmt.Errorf("Unable to parse ELF executable: %w", err)
	}

	arch, ok := rpmArchMap[e.Machine.String()]
	if !ok {
		return "", fmt.Errorf("unknown binary architecture: %q", e.Machine.String())
	}

	return arch, nil
}

func init() {
	vql_subsystem.RegisterPlugin(&CreatePackagePlugin{
		clientSpecFactory: NewClientRPMSpec,
		serverSpecFactory: NewServerRPMSpec,
		getArch:           getRPMArch,
		builder:           BuildRPM,

		name:        "rpm_create",
		description: "Create a deployable RPM package for client or server.",
	})
}

type RPMBuilder struct {
	Spec *PackageSpec

	state    *ordereddict.Dict
	metadata rpmpack.RPMMetaData
	pack     *rpmpack.RPM
}

func (self *RPMBuilder) AddFile(f rpmpack.RPMFile) {
	switch f.Name {
	case "Preun":
		self.AddPreun(string(f.Body))

	case "Postun":
		self.AddPostun(string(f.Body))

	case "Postin":
		self.AddPostin(string(f.Body))

	default:
		self.state.Set(f.Name, f)
		self.pack.AddFile(f)
	}
}

func (self *RPMBuilder) AddPreun(data string) {
	self.state.Set("Preun", data)
	self.pack.AddPreun(data)
}

func (self *RPMBuilder) AddPostun(data string) {
	self.state.Set("Postun", data)
	self.pack.AddPostun(data)
}

func (self *RPMBuilder) AddPostin(data string) {
	self.state.Set("Postin", data)
	self.pack.AddPostin(data)
}

func (self *RPMBuilder) Bytes(scope vfilter.Scope) ([]byte, error) {
	buff := &bytes.Buffer{}
	err := self.pack.Write(buff)
	return buff.Bytes(), err
}

// Build a debug string representing the rpm so we can compare it for
// tests. The output can be checked against the command:
// rpm  -qp --scripts velociraptor_client_0.74.3_x86_64.rpm
func (self *RPMBuilder) Debug() string {
	res := self.Spec.OutputFilename() + "\n"
	for _, i := range self.state.Items() {
		res += fmt.Sprintf("\n>> %v\n", i.Key)
		switch t := i.Value.(type) {

		case rpmpack.RPMFile:
			h := sha256.New()
			h.Write(t.Body)
			res += fmt.Sprintf(`
File %v
Hash %x
Mode %o
Owner %v
Group %v
`, t.Name, h.Sum(nil), t.Mode, t.Owner, t.Group)
			res += "------------\n"

		case string:
			res += t
			res += "------------\n"

		default:
			res += fmt.Sprintf("%T\n", i.Value)
			res += "------------\n"
		}
	}
	return res
}

func BuildRPM(spec *PackageSpec) (Builder, error) {
	metadata := rpmpack.RPMMetaData{}
	rpm_metadata, err := ExpandTemplate("Metadata",
		spec.Expansion, spec.Templates)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(rpm_metadata), &metadata)
	if err != nil {
		return nil, err
	}

	pack, err := rpmpack.NewRPM(metadata)
	if err != nil {
		return nil, fmt.Errorf("Unable to create RPM: %w", err)
	}

	r := &RPMBuilder{
		Spec:     spec,
		state:    ordereddict.NewDict(),
		metadata: metadata,
		pack:     pack,
	}

	for _, i := range spec.Files.Items() {
		path := i.Key
		file_spec, ok := i.Value.(FileSpec)
		if !ok {
			continue
		}

		filename, err := ExpandTemplateString(
			path, spec.Expansion, spec.Templates)
		if err != nil {
			return nil, err
		}
		if file_spec.Template == "" {
			r.AddFile(
				rpmpack.RPMFile{
					Name:  filename,
					Mode:  file_spec.Mode,
					Body:  file_spec.RawData,
					Owner: file_spec.Owner,
					Group: file_spec.Owner,
				})
		} else {
			content, err := ExpandTemplateString(
				file_spec.Template, spec.Expansion, spec.Templates)
			if err != nil {
				return nil, err
			}

			r.AddFile(
				rpmpack.RPMFile{
					Name:  filename,
					Mode:  file_spec.Mode,
					Body:  []byte(content),
					Owner: file_spec.Owner,
					Group: file_spec.Owner,
				})
		}
	}

	return r, nil
}

type Builder interface {
	Bytes(scope vfilter.Scope) ([]byte, error)
	Debug() string
}
