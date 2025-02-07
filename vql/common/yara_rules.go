//go:build cgo && yara
// +build cgo,yara

package common

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/VirusTotal/gyp"
	"github.com/VirusTotal/gyp/ast"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	includedFunctions = map[string][]string{
		"pe": []string{
			"calculate_checksum",
			"imphash",
			"section_index",
			"exports",
			"exports_index",
			"imports",
			"import_rva",
			"delayed_import_rva",
			"locale",
			"language",
			"is_dll",
			"is_32bit",
			"is_64bit",
		},
		"math": {
			"in_range",
			"deviation",
			"mean",
			"serial_correlation",
			"monte_carlo_pi",
			"entropy",
			"min",
			"max",
			"to_number",
			"abs",
			"count",
			"percentage",
			"mode",
			"to_string",
		},
		"elf": {
			"telfhash",
		},
		"time": {
			"now",
		},
	}

	includedModules = map[string][]string{
		"pe": []string{
			"MACHINE_UNKNOWN",
			"MACHINE_AM33",
			"MACHINE_AMD64",
			"MACHINE_ARM",
			"MACHINE_ARMNT",
			"MACHINE_ARM64",
			"MACHINE_EBC",
			"MACHINE_I386",
			"MACHINE_IA64",
			"MACHINE_M32R",
			"MACHINE_MIPS16",
			"MACHINE_MIPSFPU",
			"MACHINE_MIPSFPU16",
			"MACHINE_POWERPC",
			"MACHINE_POWERPCFP",
			"MACHINE_R4000",
			"MACHINE_SH3",
			"MACHINE_SH3DSP",
			"MACHINE_SH4",
			"MACHINE_SH5",
			"MACHINE_THUMB",
			"MACHINE_WCEMIPSV2",
			"MACHINE_TARGET_HOST",
			"MACHINE_R3000",
			"MACHINE_R10000",
			"MACHINE_ALPHA",
			"MACHINE_SH3E",
			"MACHINE_ALPHA64",
			"MACHINE_AXP64",
			"MACHINE_TRICORE",
			"MACHINE_CEF",
			"MACHINE_CEE",

			"SUBSYSTEM_UNKNOWN",
			"SUBSYSTEM_NATIVE",
			"SUBSYSTEM_WINDOWS_GUI",
			"SUBSYSTEM_WINDOWS_CUI",
			"SUBSYSTEM_OS2_CUI",
			"SUBSYSTEM_POSIX_CUI",
			"SUBSYSTEM_NATIVE_WINDOWS",
			"SUBSYSTEM_WINDOWS_CE_GUI",
			"SUBSYSTEM_EFI_APPLICATION",
			"SUBSYSTEM_EFI_BOOT_SERVICE_DRIVER",
			"SUBSYSTEM_EFI_RUNTIME_DRIVER",
			"SUBSYSTEM_EFI_ROM_IMAGE",
			"SUBSYSTEM_XBOX",
			"SUBSYSTEM_WINDOWS_BOOT_APPLICATION",

			"HIGH_ENTROPY_VA",
			"DYNAMIC_BASE",
			"FORCE_INTEGRITY",
			"NX_COMPAT",
			"NO_ISOLATION",
			"NO_SEH",
			"NO_BIND",
			"APPCONTAINER",
			"WDM_DRIVER",
			"GUARD_CF",
			"TERMINAL_SERVER_AWARE",

			"RELOCS_STRIPPED",
			"EXECUTABLE_IMAGE",
			"LINE_NUMS_STRIPPED",
			"LOCAL_SYMS_STRIPPED",
			"AGGRESIVE_WS_TRIM",
			"LARGE_ADDRESS_AWARE",
			"BYTES_REVERSED_LO",
			"MACHINE_32BIT",
			"DEBUG_STRIPPED",
			"REMOVABLE_RUN_FROM_SWAP",
			"NET_RUN_FROM_SWAP",
			"SYSTEM",
			"DLL",
			"UP_SYSTEM_ONLY",
			"BYTES_REVERSED_HI",

			"IMAGE_DIRECTORY_ENTRY_EXPORT",
			"IMAGE_DIRECTORY_ENTRY_IMPORT",
			"IMAGE_DIRECTORY_ENTRY_RESOURCE",
			"IMAGE_DIRECTORY_ENTRY_EXCEPTION",
			"IMAGE_DIRECTORY_ENTRY_SECURITY",
			"IMAGE_DIRECTORY_ENTRY_BASERELOC",
			"IMAGE_DIRECTORY_ENTRY_DEBUG",
			"IMAGE_DIRECTORY_ENTRY_ARCHITECTURE",
			"IMAGE_DIRECTORY_ENTRY_COPYRIGHT",
			"IMAGE_DIRECTORY_ENTRY_GLOBALPTR",
			"IMAGE_DIRECTORY_ENTRY_TLS",
			"IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG",
			"IMAGE_DIRECTORY_ENTRY_BOUND_IMPORT",
			"IMAGE_DIRECTORY_ENTRY_IAT",
			"IMAGE_DIRECTORY_ENTRY_DELAY_IMPORT",
			"IMAGE_DIRECTORY_ENTRY_COM_DESCRIPTOR",

			"IMAGE_NT_OPTIONAL_HDR32_MAGIC",
			"IMAGE_NT_OPTIONAL_HDR64_MAGIC",
			"IMAGE_ROM_OPTIONAL_HDR_MAGIC",

			"SECTION_NO_PAD",
			"SECTION_CNT_CODE",
			"SECTION_CNT_INITIALIZED_DATA",
			"SECTION_CNT_UNINITIALIZED_DATA",
			"SECTION_LNK_OTHER",
			"SECTION_LNK_INFO",
			"SECTION_LNK_REMOVE",
			"SECTION_LNK_COMDAT",
			"SECTION_NO_DEFER_SPEC_EXC",
			"SECTION_GPREL",
			"SECTION_MEM_FARDATA",
			"SECTION_MEM_PURGEABLE",
			"SECTION_MEM_16BIT",
			"SECTION_MEM_LOCKED",
			"SECTION_MEM_PRELOAD",
			"SECTION_ALIGN_1BYTES",
			"SECTION_ALIGN_2BYTES",
			"SECTION_ALIGN_4BYTES",
			"SECTION_ALIGN_8BYTES",
			"SECTION_ALIGN_16BYTES",
			"SECTION_ALIGN_32BYTES",
			"SECTION_ALIGN_64BYTES",
			"SECTION_ALIGN_128BYTES",
			"SECTION_ALIGN_256BYTES",
			"SECTION_ALIGN_512BYTES",
			"SECTION_ALIGN_1024BYTES",
			"SECTION_ALIGN_2048BYTES",
			"SECTION_ALIGN_4096BYTES",
			"SECTION_ALIGN_8192BYTES",
			"SECTION_ALIGN_MASK",
			"SECTION_LNK_NRELOC_OVFL",
			"SECTION_MEM_DISCARDABLE",
			"SECTION_MEM_NOT_CACHED",
			"SECTION_MEM_NOT_PAGED",
			"SECTION_MEM_SHARED",
			"SECTION_MEM_EXECUTE",
			"SECTION_MEM_READ",
			"SECTION_MEM_WRITE",
			"SECTION_SCALE_INDEX",

			"RESOURCE_TYPE_CURSOR",
			"RESOURCE_TYPE_BITMAP",
			"RESOURCE_TYPE_ICON",
			"RESOURCE_TYPE_MENU",
			"RESOURCE_TYPE_DIALOG",
			"RESOURCE_TYPE_STRING",
			"RESOURCE_TYPE_FONTDIR",
			"RESOURCE_TYPE_FONT",
			"RESOURCE_TYPE_ACCELERATOR",
			"RESOURCE_TYPE_RCDATA",
			"RESOURCE_TYPE_MESSAGETABLE",
			"RESOURCE_TYPE_GROUP_CURSOR",
			"RESOURCE_TYPE_GROUP_ICON",
			"RESOURCE_TYPE_VERSION",
			"RESOURCE_TYPE_DLGINCLUDE",
			"RESOURCE_TYPE_PLUGPLAY",
			"RESOURCE_TYPE_VXD",
			"RESOURCE_TYPE_ANICURSOR",
			"RESOURCE_TYPE_ANIICON",
			"RESOURCE_TYPE_HTML",
			"RESOURCE_TYPE_MANIFEST",

			"IMAGE_DEBUG_TYPE_UNKNOWN",
			"IMAGE_DEBUG_TYPE_COFF",
			"IMAGE_DEBUG_TYPE_CODEVIEW",
			"IMAGE_DEBUG_TYPE_FPO",
			"IMAGE_DEBUG_TYPE_MISC",
			"IMAGE_DEBUG_TYPE_EXCEPTION",
			"IMAGE_DEBUG_TYPE_FIXUP",
			"IMAGE_DEBUG_TYPE_OMAP_TO_SRC",
			"IMAGE_DEBUG_TYPE_OMAP_FROM_SRC",
			"IMAGE_DEBUG_TYPE_BORLAND",
			"IMAGE_DEBUG_TYPE_RESERVED10",
			"IMAGE_DEBUG_TYPE_CLSID",
			"IMAGE_DEBUG_TYPE_VC_FEATURE",
			"IMAGE_DEBUG_TYPE_POGO",
			"IMAGE_DEBUG_TYPE_ILTCG",
			"IMAGE_DEBUG_TYPE_MPX",
			"IMAGE_DEBUG_TYPE_REPRO",

			"IMPORT_DELAYED",
			"IMPORT_STANDARD",
			"IMPORT_ANY",

			"is_pe",
			"machine",
			"number_of_sections",
			"timestamp",
			"pointer_to_symbol_table",
			"number_of_symbols",
			"size_of_optional_header",
			"characteristics",

			"entry_point",
			"entry_point_raw",
			"image_base",
			"number_of_rva_and_sizes",
			"number_of_version_infos",

			"version_info",

			"version_info_list",

			"opthdr_magic",
			"size_of_code",
			"size_of_initialized_data",
			"size_of_uninitialized_data",
			"base_of_code",
			"base_of_data",
			"section_alignment",
			"file_alignment",

			"linker_version",

			"os_version",

			"image_version",

			"subsystem_version",

			"win32_version_value",
			"size_of_image",
			"size_of_headers",

			"checksum",
			"subsystem",

			"dll_characteristics",
			"size_of_stack_reserve",
			"size_of_stack_commit",
			"size_of_heap_reserve",
			"size_of_heap_commit",
			"loader_flags",

			"data_directories",

			"sections",

			"overlay",

			"rich_signature",

			"number_of_imports",
			"number_of_imported_functions",
			"number_of_delayed_imports",
			"number_of_delayed_imported_functions",
			"number_of_exports",

			"dll_name",
			"export_timestamp",
			"export_details",

			"import_details",

			"delayed_import_details",

			"resource_timestamp",

			"resource_version",

			"resources",
			"number_of_resources",
			"pdb_path",
		},
		"math": {
			"MEAN_BYTES",
		},
		"elf": {
			"ET_NONE",
			"ET_REL",
			"ET_EXEC",
			"ET_DYN",
			"ET_CORE",

			"EM_NONE",
			"EM_M32",
			"EM_SPARC",
			"EM_386",
			"EM_68K",
			"EM_88K",
			"EM_860",
			"EM_MIPS",
			"EM_MIPS_RS3_LE",
			"EM_PPC",
			"EM_PPC64",
			"EM_ARM",
			"EM_X86_64",
			"EM_AARCH64",

			"SHT_NULL",
			"SHT_PROGBITS",
			"SHT_SYMTAB",
			"SHT_STRTAB",
			"SHT_RELA",
			"SHT_HASH",
			"SHT_DYNAMIC",
			"SHT_NOTE",
			"SHT_NOBITS",
			"SHT_REL",
			"SHT_SHLIB",
			"SHT_DYNSYM",

			"SHF_WRITE",
			"SHF_ALLOC",
			"SHF_EXECINSTR",

			"type",
			"machine",
			"entry_point",

			"number_of_sections",
			"sh_offset",
			"sh_entry_size",

			"number_of_segments",
			"ph_offset",
			"ph_entry_size",

			"sections",

			"PT_NULL",
			"PT_LOAD",
			"PT_DYNAMIC",
			"PT_INTERP",
			"PT_NOTE",
			"PT_SHLIB",
			"PT_PHDR",
			"PT_TLS",
			"PT_GNU_EH_FRAME",
			"PT_GNU_STACK",

			"DT_NULL",
			"DT_NEEDED",
			"DT_PLTRELSZ",
			"DT_PLTGOT",
			"DT_HASH",
			"DT_STRTAB",
			"DT_SYMTAB",
			"DT_RELA",
			"DT_RELASZ",
			"DT_RELAENT",
			"DT_STRSZ",
			"DT_SYMENT",
			"DT_INIT",
			"DT_FINI",
			"DT_SONAME",
			"DT_RPATH",
			"DT_SYMBOLIC",
			"DT_REL",
			"DT_RELSZ",
			"DT_RELENT",
			"DT_PLTREL",
			"DT_DEBUG",
			"DT_TEXTREL",
			"DT_JMPREL",
			"DT_BIND_NOW",
			"DT_INIT_ARRAY",
			"DT_FINI_ARRAY",
			"DT_INIT_ARRAYSZ",
			"DT_FINI_ARRAYSZ",
			"DT_RUNPATH",
			"DT_FLAGS",
			"DT_ENCODING",

			"STT_NOTYPE",
			"STT_OBJECT",
			"STT_FUNC",
			"STT_SECTION",
			"STT_FILE",
			"STT_COMMON",
			"STT_TLS",

			"STB_LOCAL",
			"STB_GLOBAL",
			"STB_WEAK",

			"PF_X",
			"PF_W",
			"PF_R",

			"segments",
			"dynamic_section_entries",
			"dynamic",

			"symtab_entries",
			"symtab",

			"dynsym_entries",
			"dynsym",
		},
	}

	moduleLookup = make(map[string]bool)
)

type ExpressionState struct {
	Vars []string
}

type RuleLinter struct {
	ruleset *ast.RuleSet

	ClearMetadata bool
}

func NewRuleLinter(rules string) (*RuleLinter, error) {
	ruleset, err := gyp.Parse(strings.NewReader(rules))
	if err != nil {
		return nil, err
	}

	return &RuleLinter{
		ruleset: ruleset,
	}, nil
}

func (self *RuleLinter) String() string {
	res := &bytes.Buffer{}

	_ = self.ruleset.WriteSource(res)
	return string(res.Bytes())
}

// Create a new rule linter with only valid rules in it.
func (self *RuleLinter) Lint() (*RuleLinter, []error) {
	result := &RuleLinter{
		ruleset: &ast.RuleSet{},
	}

	var errors []error

	for _, r := range self.ruleset.Rules {
		state := &ExpressionState{}

		// First validate the condition
		err := self.walkExpression(r.Condition, r, state)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		// Rule is OK, include it
		if self.ClearMetadata {
			result.ruleset.Rules = append(result.ruleset.Rules, &ast.Rule{
				Identifier: r.Identifier,
				Strings:    r.Strings,
				Condition:  r.Condition,
			})
		} else {
			result.ruleset.Rules = append(result.ruleset.Rules, r)
		}
	}

	result.ruleset.Imports = nil

	// Only include valid imports
	for _, imp := range self.ruleset.Imports {
		_, pres := includedFunctions[imp]
		if pres {
			result.ruleset.Imports = append(result.ruleset.Imports, imp)
		}
	}
	return result, errors
}

func (self *RuleLinter) walkExpression(
	node ast.Node, rule *ast.Rule, state *ExpressionState) error {

	switch t := node.(type) {
	case *ast.MemberAccess:
		err := self.checkModuleAccess(t, rule, state)
		if err != nil {
			return err
		}

	case *ast.ForIn:
		state.Vars = append(state.Vars, t.Variables...)
	}

	// fmt.Printf("Node %T: %s\n", node, node)
	for _, c := range node.Children() {
		err := self.walkExpression(c, rule, state)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *RuleLinter) checkModuleAccess(
	m *ast.MemberAccess, rule *ast.Rule, state *ExpressionState) error {
	id, ok := m.Container.(*ast.Identifier)
	if ok {
		module := id.Identifier
		field := m.Member

		// If this is a local variable it is ok
		if utils.InString(state.Vars, module) {
			return nil
		}

		// First check that the module is imported, if not just add
		// the import because why not?
		if !utils.InString(self.ruleset.Imports, module) {
			self.ruleset.Imports = append(self.ruleset.Imports, module)
		}

		_, ok := moduleLookup[module]
		if !ok {
			return fmt.Errorf("Rule %v: accesses undefined module %v",
				rule.Identifier, module)
		}

		key := fmt.Sprintf("%s/%s", module, field)
		_, ok = moduleLookup[key]
		if !ok {
			return fmt.Errorf("Rule %v: accesses undefined field %v in module %v",
				rule.Identifier, field, module)
		}
	}
	return nil
}

type YaraLintFunctionArgs struct {
	Rules         string `vfilter:"required,field=rules,doc=A string containing Yara Rules."`
	ClearMetadata bool   `vfilter:"optional,field=clean,doc=Remove metadata to make rules smaller."`
}

type YaraLintFunction struct{}

func (self *YaraLintFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &YaraLintFunctionArgs{}

	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("yara_lint: %v", err)
		return &vfilter.Null{}
	}

	linter, err := NewRuleLinter(arg.Rules)
	if err != nil {
		scope.Log("yara_lint: %v", err)
		return &vfilter.Null{}
	}

	linter.ClearMetadata = arg.ClearMetadata
	clean, errors := linter.Lint()
	for _, err := range errors {
		scope.Log("yara_lint: %v", err)
	}

	return clean.String()
}

func (self *YaraLintFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "yara_lint",
		Doc:      "Clean a set of yara rules. This removes invalid or unsupported rules.",
		ArgType:  type_map.AddType(scope, &YaraLintFunctionArgs{}),
		Metadata: vql.VQLMetadata().Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&YaraLintFunction{})

	moduleLookup = make(map[string]bool)
	for k, values := range includedFunctions {
		moduleLookup[k] = true
		for _, v := range values {
			moduleLookup[fmt.Sprintf("%s/%s", k, v)] = true
		}
	}

	for k, values := range includedModules {
		moduleLookup[k] = true
		for _, v := range values {
			moduleLookup[fmt.Sprintf("%s/%s", k, v)] = true
		}
	}
}
