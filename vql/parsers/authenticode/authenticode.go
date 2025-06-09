/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

// Get Authenticode information from signed binaries. Currently only
// using the windows API. It is now possible to read authenticode
// certificates and cat files using the parse_pkcs7() and the
// parse_pe() vql functions. Those will return more information, but
// do not perform the verification against the root store.
package authenticode

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/go-pe"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AuthenticodeArgs struct {
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=The filename to parse."`
	Verbose  bool              `vfilter:"optional,field=verbose,doc=Set to receive verbose information about all the certs."`
}

type AuthenticodeFunction struct{}

func (self *AuthenticodeFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "authenticode", args)()

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("authenticode: %s", err)
		return vfilter.Null{}
	}

	arg := &AuthenticodeArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("authenticode: %v", err)
		return vfilter.Null{}
	}

	lru_size := vql_subsystem.GetIntFromRow(scope, scope, constants.BINARY_CACHE_SIZE)
	paged_reader, err := readers.NewAccessorReader(
		scope, arg.Accessor, arg.Filename, int(lru_size))
	if err != nil {
		scope.Log("authenticode: %v", err)
		return vfilter.Null{}
	}
	defer paged_reader.Close()

	pe_file, err := pe.NewPEFileWithSize(paged_reader, paged_reader.MaxSize())
	if err != nil {
		// Suppress logging for invalid PE files.
		// scope.Log("parse_pe: %v for %v", err, arg.Filename)
		return &vfilter.Null{}
	}

	normalized_path := arg.Filename.String()

	output := ordereddict.NewDict().
		Set("Filename", normalized_path).
		Set("ProgramName", vfilter.Null{}).
		Set("PublisherLink", vfilter.Null{}).
		Set("MoreInfoLink", vfilter.Null{}).
		Set("SerialNumber", vfilter.Null{}).
		Set("IssuerName", vfilter.Null{}).
		Set("SubjectName", vfilter.Null{}).
		Set("Timestamp", vfilter.Null{}).
		Set("Trusted", "untrusted").
		Set("_ExtraInfo", vfilter.Null{}) // Only populated with verbose = TRUE

	pkcs7_obj, err := pe.ParseAuthenticode(pe_file)
	if err == nil {
		signer := pe.PKCS7ToOrderedDict(pkcs7_obj)
		output.Update("SubjectName", utils.GetString(signer, "Signer.Subject")).
			Update("IssuerName", utils.GetString(signer, "Signer.IssuerName")).
			Update("SerialNumber", utils.GetString(signer, "Signer.SerialNumber")).
			Update("ProgramName", utils.GetString(signer, "Signer.AuthenticatedAttributes.ProgramName")).
			Update("MoreInfoLink", utils.GetString(signer, "Signer.AuthenticatedAttributes.MoreInfo")).
			Update("Timestamp", utils.GetAny(signer, "Signer.AuthenticatedAttributes.SigningTime")).
			Update("Trusted", func() vfilter.Any {
				return VerifyFileSignature(scope, normalized_path)
			})

		if arg.Verbose {
			output.Update("_ExtraInfo", signer)
		}

		// Normalize the output to make it easier to access
		// important fields.
		return output
	}

	// Maybe the file is in the cat file?
	fd, err := os.Open(normalized_path)
	if err == nil {
		defer fd.Close()

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			return &vfilter.Null{}
		}

		cat_file, err := VerifyCatalogSignature(
			config_obj, scope, fd, normalized_path, output)
		if err == nil {
			_ = ParseCatFile(cat_file, output, arg.Verbose)
		}
	}

	return output
}

func (self AuthenticodeFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "authenticode",
		Doc: "This plugin uses the Windows API to extract authenticode " +
			"signature details from PE files.",
		ArgType:  type_map.AddType(scope, &AuthenticodeArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AuthenticodeFunction{})
}
