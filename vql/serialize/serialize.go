package serialize

import (
	"context"
	"errors"
	"fmt"
	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	"strings"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	"www.velocidex.com/golang/vfilter"
)

type ScopeItems struct {
	Scope map[string]*ScopeItem `json:"Scope"`
}

type ScopeItem struct {
	Type string      `json:"type"`
	Lazy *Lazy       `json:"lazy,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

type StarlData struct {
	Code    string      `json:"code"`
	Globals interface{} `json:"globals,omitempty"`
}

type Lazy struct {
	Expr       string   `json:"expr"`
	Parameters []string `json:"parameters"`
}

func SerializeScope(scope vfilter.Scope) []byte {

	data := make(map[string]*ScopeItem, 0)
	for _, vars := range scope.GetVars() {
		for _, k := range scope.GetMembers(vars) {
			switch {
			case strings.HasPrefix(k, "$"):
				continue
			case k == "NULL":
				continue
			case k == "Artifact":
				continue
			default:
				res, _ := scope.Resolve(k)
				si := &ScopeItem{}
				switch t := res.(type) {
				case *vfilter.StoredExpression:
					lz := &Lazy{}
					lz.Expr = t.ToString(scope)
					params := t.GetParams()
					if params == nil {
						lz.Parameters = make([]string, 0)
					} else {
						lz.Parameters = params
					}
					si.Lazy = lz
					si.Type = "StoredExpression"
				case vfilter.StoredQuery:
					lz := &Lazy{}
					lz.Expr = t.ToString(scope)
					params := t.GetParams()
					if params == nil {
						lz.Parameters = make([]string, 0)
					} else {
						lz.Parameters = params
					}
					si.Type = "StoredQuery"
					si.Lazy = lz

				default:
					si.Type = "Data"
					si.Data = t
				}
				data[k] = si
			}
		}
	}

	cb := func(v interface{}, opts *json.EncOpts) ([]byte, error) {
		switch t := v.(type) {
		case *ordereddict.Dict:
			{
				starl, ok := t.Get("__starlark")
				if !ok {
					return nil, json.EncoderCallbackSkip
				}
				code, _ := starl.(*ordereddict.Dict).Get("code")
				globals, _ := starl.(*ordereddict.Dict).Get("globals")
				sd := &StarlData{Code: code.(string), Globals: globals}
				return json.MarshalWithOptions(sd, opts)
			}
		}
		return nil, json.EncoderCallbackSkip
	}

	enc_opts := json.NewEncOpts().WithCallback(ordereddict.Dict{}, cb)
	serialized, _ := json.MarshalWithOptions(&ScopeItems{Scope: data}, enc_opts)
	return serialized
}

func mapToStarl(ctx context.Context, scope vfilter.Scope, starl interface{}) (*ordereddict.Dict, error) {
	code, ok := starl.(map[string]interface{})["code"].(string)
	if !ok {
		return nil, errors.New("Missing Starlark Code")
	}
	globals, ok := starl.(map[string]interface{})["globals"]
	if !ok {
		return nil, errors.New("Missing Starlark Code")
	}
	compiled, err := tools.CompileStarlark(ctx, scope, code, globals)
	if err != nil {
		return nil, err
	}
	return compiled, err
}

func findStarl(ctx context.Context, scope vfilter.Scope, data interface{}) error {
	switch t := data.(type) {
	case map[string]interface{}:
		{
			for k, val := range t {
				switch v := val.(type) {
				case map[string]interface{}:
					{
						if starl, ok := v["__starlark"]; ok {
							compiled, err := mapToStarl(ctx, scope, starl)
							if err != nil {
								return err
							}
							t[k] = compiled
						}
						err := findStarl(ctx, scope, t[k])
						if err != nil {
							return err
						}
					}
				case []interface{}:
					{
						err := findStarl(ctx, scope, t[k])
						if err != nil {
							return err
						}
					}
				}
			}
		}
	case []interface{}:
		{
			for k, val := range t {
				switch v := val.(type) {
				case map[string]interface{}:
					{
						if starl, ok := v["__starlark"]; ok {
							compiled, err := mapToStarl(ctx, scope, starl)
							if err != nil {
								return err
							}
							t[k] = compiled
						}
						err := findStarl(ctx, scope, &t[k])
						if err != nil {
							return err
						}
					}
				case []interface{}:
					{
						err := findStarl(ctx, scope, &t[k])
						if err != nil {
							return err
						}
					}
				}
			}

		}
	}
	return nil
}

func DeserializeScope(ctx context.Context, scope vfilter.Scope, mapping map[string]*ScopeItem) error {
	queries := make([]string, 0)
	for k, val := range mapping {
		switch val.Type {
		case "StoredExpression":
			{
				expr := "LET " + k
				lazy := val.Lazy
				if lazy == nil {
					return errors.New("deserialize: Lazy Struct Missing")
				}
				if len(lazy.Parameters) > 0 {
					expr += "("
					for index, param := range lazy.Parameters {
						expr += param
						if index < len(lazy.Parameters)-1 {
							expr += ", "
						}
					}
					expr += ")"

				}
				expr += " = " + lazy.Expr
				queries = append(queries, expr)
			}
		case "StoredQuery":
			{
				expr := "LET " + k
				lazy := val.Lazy
				if lazy == nil {
					return errors.New("deserialize: Lazy Struct Missing")
				}
				if len(lazy.Parameters) > 0 {
					expr += "("
					for index, param := range lazy.Parameters {
						expr += param
						if index < len(lazy.Parameters)-1 {
							expr += ", "
						}
					}
					expr += ")"

				}
				expr += " = " + lazy.Expr
				queries = append(queries, expr)
			}
		case "Data":
			{
				dict := ordereddict.NewDict()
				if val.Data == nil {
					return errors.New("deserialize: Empty Data Field")
				}
				dt, ok := val.Data.(map[string]interface{})
				if ok {
					if starl, ok := dt["__starlark"]; ok {
						compiled, err := mapToStarl(ctx, scope, starl)
						if err != nil {
							return err
						}
						if compiled != nil {
							dict.Set(k, compiled)
							scope.AppendVars(dict)
							continue
						}
					}
				}
				err := findStarl(ctx, scope, val.Data)
				if err != nil {
					return err
				}
				dict.Set(k, val.Data)
				scope.AppendVars(dict)
			}
		default:
			return errors.New(fmt.Sprintf("deserialize: Invalid Scope Type %s", val.Type))

		}
	}

	for _, query := range queries {
		vql, err := vfilter.Parse(query)
		if err != nil {
			return err
		}
		for range vql.Eval(ctx, scope) {
		}
	}
	return nil
}
