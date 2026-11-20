//go:build dev
// +build dev

/*
   How to use this tool:

   In a build environment with go installed and C compilers, build the debug binary:

   ```
   make linux_debug
   ./output/velociraptor-v0.77.4-linux-amd64 perm_check
   ```

   On Windows export the env flags so it can use the CC compilers -
   otherwise it wont be able to load C plugins:
   ```
   set CC=F:\projects\velociraptor\build\toolchain\llvm-mingw-20260908-ucrt-i686\bin\i686-w64-mingw32-clang
   set CGO_ENABLED=1
   ```

*/

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"regexp"
	"strings"

	kingpin "github.com/alecthomas/kingpin/v2"
	"golang.org/x/tools/go/packages"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/types"
)

/*
   This command checks that plugins implement the same checks as
   declared in their metadata. We statically analyze the plugin's code
   for indications that the permission is checked. The analysis is
   very simple and does not check that the permission is check
   properly in all code paths, but at least this gives an indication
   that the permission is considered at all and not forgotten.
*/

var (
	permission_check_cmd = app.Command("perm_check",
		"Check permissions in VQL plugins.")

	permission_check_cmd_start = permission_check_cmd.Flag(
		"start", "index where to start").Int()
)

func doPermCheck() error {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	for def_idx, definition := range lsp.LoadApiDescriptions() {
		if def_idx < *permission_check_cmd_start {
			continue
		}

		metadata := definition.Metadata

		if metadata == nil {
			continue
		}

		// Check if the plugin is compiled into this binary - we can
		// not check a plugin which is not compiled for this architecture.

		switch definition.Type {
		case "Plugin":
			plugin, pres := scope.GetPlugin(definition.Name)
			if !pres {
				// Plugin is not available on this architecture.
				continue
			}

			// Get the latest metadata
			type_map := types.NewTypeMap()
			plugin_info := plugin.Info(scope, type_map)
			metadata = make(map[string]string)
			for _, item := range plugin_info.Metadata.Items() {
				metadata[item.Key] = utils.ToString(item.Value)
			}

		case "Function":
			function, pres := scope.GetFunction(definition.Name)
			if !pres {
				// Plugin is not available on this architecture.
				continue
			}

			// Get the latest metadata
			type_map := types.NewTypeMap()
			plugin_info := function.Info(scope, type_map)
			metadata = make(map[string]string)
			for _, item := range plugin_info.Metadata.Items() {
				metadata[item.Key] = utils.ToString(item.Value)
			}

			// Do not handle other types.
		default:
			continue
		}

		declared_perms, _ := metadata["permissions"]

		// The plugin does not need any special permissions so don't
		// bother to check it.
		if declared_perms == "" {
			continue
		}

		fn, _ := metadata["Fn"]
		if fn == "" || !strings.HasSuffix(fn, ".Info") {
			continue
		}

		fn = strings.TrimSuffix(fn, ".Info")
		idx := strings.LastIndex(fn, ".")
		module := fn[:idx]
		type_name := strings.Trim(fn[idx+1:], "()*")

		fmt.Printf("Checking plugin %v module %v, type %v ... ", def_idx, module, type_name)

		src, err := extractSource(module, type_name)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		extracted_perms := checkPermissions(src)

		if !checkDeclaredPermissions(declared_perms, extracted_perms) {
			fmt.Printf("File %v\n", metadata["File"])
			fmt.Printf("Extracted %v\n", strings.Join(extracted_perms, ","))
			fmt.Printf("Perms %v\n", declared_perms)
			fmt.Println(src)
		} else {
			fmt.Println("OK!")
		}
	}

	return nil
}

// It is ok if the plugin checks more permissions than declared since
// many of those checks are inferred and may not be actually relevant.
func checkDeclaredPermissions(declared_perms string, extracted_perms []string) bool {
	found := make(map[string]bool)
	for _, perm := range extracted_perms {
		found[perm] = true
	}

	for _, p := range strings.Split(declared_perms, ",") {
		_, pres := found[p]
		if !pres {
			return false
		}
	}
	return true
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case permission_check_cmd.FullCommand():
			err := doPermCheck()
			kingpin.FatalIfError(err, "%s", fs_command_ls.FullCommand())

		default:
			return false
		}
		return true
	})
}

type permCheckStep struct {
	re   *regexp.Regexp
	perm string
	cb   func(hits []string) []string
}

func getCapture(hits []string) []string {
	if len(hits) >= 2 {
		return []string{hits[1]}
	}
	return nil
}

func getACLPermissions(hits []string) (res []string) {
	for _, m := range aclRegex.FindAllStringSubmatch(hits[1], -1) {
		res = append(res, m[1])
	}
	return res
}

var (
	aclRegex = regexp.MustCompile(`acls.([A-Z_]+)`)

	permChecks = []permCheckStep{{
		re: regexp.MustCompile(`(?sm)\.CheckAccess\(([^\)]+)\)`),
		cb: getACLPermissions,
	}, {
		re: regexp.MustCompile(`(?sm)vql_subsystem.ImplyPermissions\(([^\)]+)\)`),
		cb: getACLPermissions,
	}, {
		re: regexp.MustCompile(`(?sm)\.CheckAccessWithArgs\(([^\)]+)\)`),
		cb: getACLPermissions,
	}, {
		// Code appears to check for these permissions
		re: regexp.MustCompile(`permissions? :?= acls.([A-Z_]+)`),
		cb: getCapture,
	}, {
		// Central function for checking writability of files
		re:   regexp.MustCompile(`file.OpenFileForWriting`),
		perm: "FILESYSTEM_WRITE",
	}, {
		// These functions enforces read checks for relevant accessors
		// because they go through the `accessors.GetAccessor` gate
		re:   regexp.MustCompile(`(readers.NewAccessorReader|accessors.GetAccessor)`),
		perm: "FILESYSTEM_READ",
	}, {
		// Already checks access (wraps accessors.GetAccessor)
		re:   regexp.MustCompile(`accessors.GetUnderlyingAPIFilename`),
		perm: "FILESYSTEM_READ",
	}, {
		// ScheduleArtifactCollection already does its own checks
		re:   regexp.MustCompile(`launcher.ScheduleArtifactCollection`),
		perm: "COLLECT_CLIENT,COLLECT_SERVER,COLLECT_BASIC",
	}, {
		re:   regexp.MustCompile(`accessors.GetUnderlyingAPIFilename`),
		perm: "FILESYSTEM_READ",
	}, {
		// User manager takes care of its own permission checks.
		re:   regexp.MustCompile(`users_managers?\.(AddUserToOrg|DeleteUser)`),
		perm: "SERVER_ADMIN",
	}, {
		re:   regexp.MustCompile(`services.GrantUserToOrg`),
		perm: "SERVER_ADMIN",
	}}
)

func checkPermissions(source string) (res []string) {
	for _, check := range permChecks {
		for _, m := range check.re.FindAllStringSubmatch(source, -1) {
			if check.perm != "" {
				res = append(res, strings.Split(check.perm, ",")...)
				continue
			}

			if check.cb != nil {
				res = append(res, check.cb(m)...)
				continue
			}
		}
	}
	return utils.Sort(utils.Uniquify(res))
}

func extractSource(targetPkg, targetType string) (string, error) {
	output := &strings.Builder{}

	// 1. Load the package with full AST and file set information
	cfg := &packages.Config{
		Mode:       packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes,
		BuildFlags: []string{"-tags", "velomain server_vql extras release cgo yara "},
	}
	pkgs, err := packages.Load(cfg, targetPkg)
	if err != nil || packages.PrintErrors(pkgs) > 0 || len(pkgs) == 0 {
		return "", fmt.Errorf("Failed to load package: %v", err)
	}

	pkg := pkgs[0]

	// 2. Walk every file's AST in the package
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {

			switch d := decl.(type) {
			// Check for the type definition: `type TargetType ...`
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						typeSpec := spec.(*ast.TypeSpec)
						if typeSpec.Name.Name == targetType {
							printNode(pkg.Fset, d, output)
						}
					}
				}

			// Check for methods: `func (r TargetType) ...` or `func (r *TargetType) ...`
			case *ast.FuncDecl:
				if d.Recv != nil && len(d.Recv.List) > 0 {
					if receiverTypeName(d.Recv.List[0].Type) == targetType {
						printNode(pkg.Fset, d, output)
					}
				}
			}
		}
	}

	return output.String(), nil
}

// receiverTypeName unwraps pointer receivers (*T -> T) and generic type parameters (T[K] -> T)
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr: // Generic type with 1 parameter: T[K]
		return receiverTypeName(t.X)
	case *ast.IndexListExpr: // Generic type with multiple parameters: T[K, V]
		return receiverTypeName(t.X)
	default:
		return ""
	}
}

// printNode formats an AST node back into valid Go source code
func printNode(fset *token.FileSet, node any, output *strings.Builder) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, node); err != nil {
		log.Printf("Error printing node: %v", err)
		return
	}
	output.WriteString(buf.String())
	output.WriteString("\n---\n")
}
