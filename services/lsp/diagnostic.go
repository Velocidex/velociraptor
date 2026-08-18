package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (self *LSPServer) Diagnostic(ctx context.Context,
	params *protocol.DocumentDiagnosticParams) (
	protocol.DocumentDiagnosticReport, error) {

	doc, err := self.getDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	diagnostics := doc.Diagnostics()

	result := &protocol.RelatedFullDocumentDiagnosticReport{
		FullDocumentDiagnosticReport: protocol.FullDocumentDiagnosticReport{
			ResultID: params.Identifier,
			Kind:     "full",
		},
	}

	for _, d := range diagnostics {
		result.Items = append(result.Items, *d)
	}

	return result, nil
}
