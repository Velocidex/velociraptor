package functions

import (
	"encoding"
	"fmt"
	"reflect"

	"github.com/Velocidex/ordereddict"
	"go.yaml.in/yaml/v4"
)

// ordereddict's MarshalYAML returns a v2 MapSlice. Build a v4 mapping
// for VQL dictionaries, including rows nested inside queries and lists.
func yamlValue(value interface{}, depth int) (interface{}, error) {
	if depth > 256 {
		return nil, fmt.Errorf("YAML value exceeds maximum nesting depth")
	}
	switch v := value.(type) {
	case *ordereddict.Dict:
		if v == nil {
			return nil, nil
		}
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, item := range v.Items() {
			key := &yaml.Node{}
			if err := key.Encode(item.Key); err != nil {
				return nil, err
			}
			child, err := yamlValue(item.Value, depth+1)
			if err != nil {
				return nil, err
			}
			childNode, ok := child.(*yaml.Node)
			if !ok {
				childNode = &yaml.Node{}
				if err := childNode.Dump(child, yaml.WithV2Defaults()); err != nil {
					return nil, err
				}
			}
			node.Content = append(node.Content, key, childNode)
		}
		return node, nil
	case yaml.Marshaler, encoding.TextMarshaler, []byte:
		return value, nil
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		items := make([]interface{}, rv.Len())
		for i := range items {
			item, err := yamlValue(rv.Index(i).Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return items, nil
	case reflect.Map:
		items := make(map[interface{}]interface{}, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			item, err := yamlValue(iter.Value().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			items[iter.Key().Interface()] = item
		}
		return items, nil
	}
	return value, nil
}
