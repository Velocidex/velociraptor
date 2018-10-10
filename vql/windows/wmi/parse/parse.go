// Parse the event into a vfilter.Dict. This is a rudimentary parser
// for MOF the is emitted by IWbemClassObject::GetObjectText.
package wmi

import (
	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"www.velocidex.com/golang/vfilter"
)

var (
	mofLexer = lexer.Unquote(lexer.Upper(lexer.Must(lexer.Regexp(
		`(?ms)`+
			`(\s+)`+
			`|(?i)(?P<Instance>INSTANCE OF)`+
			`|(?P<Ident>[a-zA-Z_][a-zA-Z0-9_]*)`+
			`|(?P<Number>[-+]?\d*\.?\d+([eE][-+]?\d+)?)`+
			`|(?P<String>'([^'\\]*(\\.[^'\\]*)*)'|"([^"\\]*(\\.[^"\\]*)*)")`+
			`|(?P<Operators><>|!=|<=|>=|=~|[-;+*/%,.()=<>{}\[\]])`,
	)), "Keyword"), "String")

	mofParser = participle.MustBuild(&MOF{}, mofLexer)
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

func (self *MOF) ToDict() *vfilter.Dict {
	return self.MOF.ToDict()
}

type Instance struct {
	Instance *string  `Instance @Ident "{" `
	Fields   []*Field ` { @@ } "}" `
}

func (self *Instance) ToDict() *vfilter.Dict {
	result := vfilter.NewDict().Set("__Type", self.Instance)
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
