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
package functions

import (
	"context"
	"crypto/rc4"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type Crypto_rc4Args struct {
	String string `vfilter:"required,field=string,doc=String to apply Rc4 encryption"`
	Key    string `vfilter:"required,field=key,doc=Rc4 key (1-256bytes)."`
}

type Crypto_rc4 struct{}

func (self *Crypto_rc4) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "crypto_rc4", args)()

	arg := &Crypto_rc4Args{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("Crypto_rc4: %s", err.Error())
		return false
	}

	cipher, err := rc4.NewCipher([]byte(arg.Key))
	if err != nil {
		scope.Log("Crypto_rc4: %s", err.Error())
		return false
	}

	byte_data := []byte(arg.String)
	cipher.XORKeyStream(byte_data, byte_data)

	return string(byte_data)
}

func (self Crypto_rc4) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "crypto_rc4",
		Doc:     "Apply rc4 to the string and key.",
		ArgType: type_map.AddType(scope, &Crypto_rc4Args{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&Crypto_rc4{})
}
