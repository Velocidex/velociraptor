package lsp

import (
	"bytes"
	"encoding/json"

	"go.lsp.dev/protocol"
)

func DumpProtool(in interface{}) string {
	serialized, err := protocol.Marshal(in)
	if err != nil {
		return ""
	}
	dst := &bytes.Buffer{}
	err = json.Indent(dst, serialized, "", " ")
	if err != nil {
		return ""
	}
	return dst.String()
}
