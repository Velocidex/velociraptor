package packaging

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/xor-gate/debpkg"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	"www.velocidex.com/golang/vfilter"
)

var (

	// debArchMap maps ELF machine strings to Debian architectures
	// See https://github.com/torvalds/linux/blob/master/include/uapi/linux/elf-em.h
	//     https://wiki.debian.org/SupportedArchitectures
	debArchMap = map[string]string{
		"EM_X86_64":  "amd64",
		"EM_386":     "i386",
		"EM_RISCV":   "riscv64",
		"EM_AARCH64": "arm64",
		"EM_ARM":     "armhf",
		"EM_PPC64":   "ppc64",
	}
)

func getDebArch(exe_bytes []byte) (string, error) {
	reader := bytes.NewReader(exe_bytes)
	e, err := elf.NewFile(reader)
	if err != nil {
		return "", fmt.Errorf("Unable to parse ELF executable: %w", err)
	}

	arch, ok := debArchMap[e.Machine.String()]
	if !ok {
		return "", fmt.Errorf("unknown binary architecture: %q", e.Machine.String())
	}

	return arch, nil
}

func init() {
	vql_subsystem.RegisterPlugin(&CreatePackagePlugin{
		clientSpecFactory: NewClientDebSpec,
		serverSpecFactory: NewServerDebSpec,
		getArch:           getDebArch,
		builder:           BuildDeb,

		name:        "deb_create",
		description: "Create a deployable Debian package for client or server.",
	})
}

type DEBBuilder struct {
	Spec *PackageSpec

	*debpkg.DebPkg

	state *ordereddict.Dict
}

func (self *DEBBuilder) AddFileString(data, path string) error {

	switch path {
	case "Preun":
		return self.AddControlExtraString("preinst", data)

	case "Postun":
		return self.AddControlExtraString("postun", data)

	case "Prerm":
		return self.AddControlExtraString("prerm", data)

	case "Postin":
		return self.AddControlExtraString("postinst", data)

	default:
		self.state.Set(path, data)
		return self.DebPkg.AddFileString(data, path)
	}
}

func (self *DEBBuilder) AddControlExtraString(path, data string) error {
	self.state.Set(path, data)
	return self.DebPkg.AddControlExtraString(path, data)
}

func (self *DEBBuilder) Bytes(scope vfilter.Scope) ([]byte, error) {
	tmpfile, err := tempfile.TempFile("tmp*")
	if err != nil {
		return nil, err
	}
	filename := tmpfile.Name()
	tempfile.AddTmpFile(filename)

	defer filesystem.RemoveTmpFile(0, filename, scope)
	defer tmpfile.Close()

	err = self.DebPkg.Write(filename)
	if err != nil {
		return nil, err
	}
	tmpfile.Close()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Build a debug string representing the rpm so we can compare it for
// tests. The output can be checked against the command:
// rpm  -qp --scripts velociraptor_client_0.74.3_x86_64.rpm
func (self *DEBBuilder) Debug() string {
	res := self.Spec.OutputFilename() + "\n"
	for _, i := range self.state.Items() {
		res += fmt.Sprintf("\n>> %v\n", i.Key)
		switch t := i.Value.(type) {

		case string:
			if strings.Contains(t[:10], "ELF") {
				res += "... ELF Data ..."
			} else {
				res += t
			}
			res += "------------\n"

		default:
			res += fmt.Sprintf("%T\n", i.Value)
			res += "------------\n"
		}
	}
	return res
}

func BuildDeb(spec *PackageSpec) (Builder, error) {
	deb := &DEBBuilder{
		Spec:   spec,
		DebPkg: debpkg.New(),
		state:  ordereddict.NewDict(),
	}

	deb.SetName(spec.Expansion.Name)
	deb.SetVersion(spec.Expansion.Version)
	deb.SetArchitecture(spec.Expansion.Arch)
	deb.SetMaintainer(spec.Expansion.Maintainer)
	deb.SetMaintainerEmail(spec.Expansion.MaintainerEmail)
	deb.SetHomepage(spec.Expansion.Homepage)
	deb.SetShortDescription(spec.Expansion.ServiceDescription)
	if spec.Expansion.Depends != "" {
		deb.SetDepends(spec.Expansion.Depends)
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
			err := deb.AddFileString(string(file_spec.RawData), filename)
			if err != nil {
				return nil, err
			}

		} else {
			content, err := ExpandTemplateString(
				file_spec.Template, spec.Expansion, spec.Templates)
			if err != nil {
				return nil, err
			}

			err = deb.AddFileString(content, filename)
			if err != nil {
				return nil, err
			}
		}
	}

	return deb, nil
}
