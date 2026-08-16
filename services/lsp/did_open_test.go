package lsp_test

import (
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	didOpenTC = []struct {
		Name  string
		Query string
	}{{
		Name:  "Invalid plugin",
		Query: "SELECT * FROM scopeXXX()",
	}, {
		Name:  "Invalid arg",
		Query: "SELECT * FROM scope(Foo=1)",
	}, {
		Name:  "Valid query",
		Query: "SELECT * FROM scope()",
	}, {
		Name:  "Invalid syntax",
		Query: "SELECT *",
	}, {
		Name:  "Missing required arg",
		Query: "SELECT * FROM parse_evtx()",
	}}
)

func (self *LSPTestSuite) TestDidOpen() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range didOpenTC {
		doc := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n%v", idx, tc.Name, tc.Query))

		diagnostics, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc,
					Text: tc.Query,
				},
			})
		assert.NoError(self.T(), err)
		golden = append(golden, lsp.DumpProtool(diagnostics))
	}

	goldie.Assert(self.T(), "TestDidOpen",
		[]byte(strings.Join(golden, "\n")))
}
