//go:build !codeanalysis
// +build !codeanalysis

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
// Parse the event into a ordereddict.Dict. This is a rudimentary parser
// for MOF the is emitted by IWbemClassObject::GetObjectText.
package wmi

import (
	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"www.velocidex.com/golang/vfilter"
)

var (
	mofLexer = lexer.Must(lexer.Regexp(
		`(?ms)` +
			`(\s+)` +
			`|(?P<Bool>FALSE|TRUE)` +
			`|(?P<Null>NULL)` +
			`|(?i)(?P<Instance>INSTANCE OF)` +
			`|(?P<Ident>[a-zA-Z_][a-zA-Z0-9_]*)` +
			`|(?P<Number>[-+]?\d*\.?\d+([eE][-+]?\d+)?)` +
			`|(?P<String>'([^'\\]*(\\.[^'\\]*)*)'|"([^"\\]*(\\.[^"\\]*)*)")` +
			`|(?P<Operators><>|!=|<=|>=|=~|[-;+*/%,.()=<>{}\[\]])`,
	))

	mofParser = participle.MustBuild(
		&MOF{},
		participle.Lexer(mofLexer),
		participle.Unquote("String"),
	)
)

func Parse(expression string) (*MOF, error) {
	mof := &MOF{}
	err := mofParser.ParseString(expression, mof)
	if err != nil {
		return nil, err
	}

	return mof, nil
}

type MOF struct {
	MOF *Instance ` @@ ";" `
}

func (self *MOF) ToDict() *ordereddict.Dict {
	return self.MOF.ToDict()
}

type Instance struct {
	Instance *string  `Instance @Ident "{" `
	Fields   []*Field ` { @@ } "}" `
}

func (self *Instance) ToDict() *ordereddict.Dict {
	result := ordereddict.NewDict().Set("__Type", self.Instance)
	if self.Fields != nil {
		for _, field := range self.Fields {
			result.Set(field.Name, field.Value.Interface())
		}
	}

	return result
}

type Field struct {
	Name  string ` @Ident "=" `
	Value *Value ` @@ ";" `
}

type Value struct {
	String   *string          ` ( @String `
	Number   *int64           ` | @Number `
	Bool     *bool            ` | @Bool `
	Null     *string          ` | @Null `
	Array    *CommaExpression ` | "{" @@ "}" `
	Instance *Instance        ` | @@ )`
}

func (self *Value) Interface() interface{} {
	if self.String != nil {
		return *self.String
	}

	if self.Number != nil {
		return *self.Number
	}

	if self.Null != nil {
		return vfilter.Null{}
	}

	if self.Bool != nil {
		return *self.Bool
	}

	if self.Array != nil {
		return self.Array.ToArray()
	}

	if self.Instance != nil {
		return self.Instance.ToDict()
	}

	return nil
}

type CommaExpression struct {
	Left  *Value           `@@`
	Right *CommaExpression ` [ "," @@ ] `
}

func (self *CommaExpression) ToArray() []interface{} {
	result := []interface{}{self.Left.Interface()}

	if self.Right == nil {
		return result
	}
	return append(result, self.Right.ToArray()...)
}
