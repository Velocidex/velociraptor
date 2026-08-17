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
	semanticTokenTC = []struct {
		Name  string
		Query string
	}{{
		Name: "Selecting a plugin with arguments",
		Query: "SELECT * FROM pslist(pid=1)",
	}, {
		Name: "A LET variable and a function call",
		Query: "// only own processes\n" +
			"LET MyVar = SELECT upcase(string=X) FROM pslist(pid=1)\n" +
			"SELECT MyVar.Foo FROM scope()",
	}, {
		Name: "An artifact reference",
		Query: "SELECT * FROM Artifact.Linux.Sys.Users()",
	}, {
		Name: "Comments and strings",
		Query: "/* header */\nSELECT * FROM glob(globs='/etc/passwd')\n" +
			"-- footer\n",
	}}
)

// decodeSemanticTokens turns the delta encoded token data into
// readable lines "line start_char length type".
func decodeSemanticTokens(data []uint32, legend []string) []string {
	var lines []string
	line := uint32(0)
	char := uint32(0)

	for i := 0; i+4 < len(data); i += 5 {
		delta_line := data[i]
		delta_char := data[i+1]
		length := data[i+2]
		token_type := data[i+3]

		if delta_line != 0 {
			char = 0
			line += delta_line
			char += delta_char
		} else {
			char += delta_char
		}

		name := fmt.Sprintf("%d", token_type)
		if int(token_type) < len(legend) {
			name = legend[token_type]
		}
		lines = append(lines, fmt.Sprintf(
			"%d:%d len=%d %v", line, char, length, name))
	}

	return lines
}

func (self *LSPTestSuite) TestSemanticTokens() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	capabilities, err := lsp_service.Initialize(self.Ctx,
		&protocol.InitializeParams{})
	assert.NoError(self.T(), err)

	var golden []string
	var legend []string

	tokens_provider, ok := capabilities.Capabilities.SemanticTokensProvider.
		(*protocol.SemanticTokensOptions)
	assert.True(self.T(), ok)
	legend = tokens_provider.Legend.TokenTypes

	for idx, tc := range semanticTokenTC {
		doc := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n%v",
			idx, tc.Name, tc.Query))

		// Load the document.
		_, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc,
					Text: tc.Query,
				},
			})
		assert.NoError(self.T(), err)

		tokens, err := lsp_service.SemanticTokensFull(self.Ctx,
			&protocol.SemanticTokensParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
			})
		assert.NoError(self.T(), err)

		golden = append(golden, "Semantic tokens:")
		for _, line := range decodeSemanticTokens(tokens.Data, legend) {
			golden = append(golden, line)
		}
	}

	goldie.Assert(self.T(), "TestSemanticTokens",
		[]byte(strings.Join(golden, "\n")))
}