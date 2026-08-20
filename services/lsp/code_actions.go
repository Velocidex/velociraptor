package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

// CodeAction offers quick fixes for the diagnostics in the document.
func (self *LSPServer) CodeAction(
	ctx context.Context,
	params *protocol.CodeActionParams) ([]protocol.CommandOrCodeAction, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	actions := []protocol.CommandOrCodeAction{}

	for _, diag := range params.Context.Diagnostics {
		if !isUnknownParameter(diag) {
			continue
		}

		action := self.removeArgumentAction(
			params.TextDocument.URI, doc, diag)
		if action != nil {
			actions = append(actions, protocol.CommandOrCodeAction(action))
		}
	}

	if actionKindRequested(params, protocol.CodeActionKindSource) {
		actions = append(actions, self.formatDocumentAction(
			params.TextDocument.URI, doc))
	}

	return actions, nil
}

// isUnknownParameter returns true if the diagnostic is a call to a
// known plugin or function with an argument it does not declare.
// Plugins use INVALID_ARG while artifact calls use
// UNKNOWN_PARAMETER_IN_CALL.
func isUnknownParameter(diag protocol.Diagnostic) bool {
	msg, ok := diag.Message.(protocol.String)
	if !ok {
		return false
	}

	return strings.Contains(string(msg), launcher.INVALID_ARG) ||
		strings.Contains(string(msg), launcher.UNKNOWN_PARAMETER_IN_CALL)
}

// removeArgumentAction produces a quickfix which removes the unknown
// argument from the call.
func (self *LSPServer) removeArgumentAction(
	doc_uri uri.URI, doc *Document,
	diag protocol.Diagnostic) *protocol.CodeAction {

	// Find the argument in the analysis state whose position matches
	// the diagnostic range.
	for _, cs := range doc.AnalysisState.Callsites {
		for _, arg := range cs.Args {
			arg_range := *protocolRange(arg.Pos)
			if !rangesEqual(arg_range, diag.Range) {
				continue
			}

			// Replace the argument with an empty string. The editor
			// will do its best to clean up the leftover comma.
			return &protocol.CodeAction{
				Title:       "Remove unknown argument '" + arg.Name + "'",
				Kind:        ptr(protocol.CodeActionKindQuickFix),
				Diagnostics: []protocol.Diagnostic{diag},
				IsPreferred: ptr(true),
				Edit: &protocol.WorkspaceEdit{
					Changes: map[uri.URI][]protocol.TextEdit{
						doc_uri: {{
							Range:   arg_range,
							NewText: "",
						}},
					},
				},
			}
		}
	}

	return nil
}

// formatDocumentAction returns a source action which reformats the
// whole document.
func (self *LSPServer) formatDocumentAction(
	doc_uri uri.URI, doc *Document) protocol.CommandOrCodeAction {

	formatted, err := formatVQL(doc.Text)
	if err != nil {
		return nil
	}

	action := &protocol.CodeAction{
		Title: "Format document",
		Kind:  ptr(protocol.CodeActionKindSource),
		Edit: &protocol.WorkspaceEdit{
			Changes: map[uri.URI][]protocol.TextEdit{
				doc_uri: {{
					Range:   fullDocumentRange(doc.Text),
					NewText: formatted,
				}},
			},
		},
	}
	return protocol.CommandOrCodeAction(action)
}

// actionKindRequested reports whether actions of the given kind
// should be returned.
func actionKindRequested(
	params *protocol.CodeActionParams, kind protocol.CodeActionKind) bool {
	only := params.Context.Only
	if len(only) == 0 {
		return true
	}

	for _, k := range only {
		if k == kind || strings.HasPrefix(string(k), string(kind)+".") {
			return true
		}
	}
	return false
}

func rangesEqual(a, b protocol.Range) bool {
	return a.Start.Line == b.Start.Line &&
		a.Start.Character == b.Start.Character &&
		a.End.Line == b.End.Line &&
		a.End.Character == b.End.Character
}

func ptr[T any](value T) *T {
	return &value
}
