package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// DidChange handles the textDocument/didChange notification. We
// advertise full sync so each change contains the whole document text.
//
// Typing trigger characters like '.' or '(' often makes the VQL
// temporarily invalid (e.g. "pslist(?" or "Artifact."). When the parse
// fails the analysis state is empty, so completion and other features
// have nothing to work with. To keep the IDE usable while typing we
// first try to salvage the largest valid prefix of the new text (so
// positions stay correct for the current document), and only fall back
// to the analysis state from the last good parse when nothing in the
// new text parses.
func (self *LSPServer) DidChange(
	ctx context.Context,
	params *protocol.DidChangeTextDocumentParams) ([]*protocol.Diagnostic, error) {

	self.mu.Lock()
	old_doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	if len(params.ContentChanges) == 0 {
		return nil, utils.NotFoundError
	}

	// We advertise full sync so the client sends the whole document
	// in a TextDocumentContentChangeWholeDocument.
	text := ""
	if change, ok := params.ContentChanges[len(params.ContentChanges)-1].(*protocol.TextDocumentContentChangeWholeDocument); ok {
		text = change.Text
	}

	document, err := NewDocument(ctx, self.config_obj, params.TextDocument.URI, text)
	if err != nil {
		return nil, err
	}

	// If the new document failed to parse, the analysis state is
	// empty. Try to salvage the largest valid prefix of the new text
	// first so the analysis reflects what the user is currently
	// typing. Only fall back to the old analysis state when nothing
	// in the new text parses.
	if len(document.AnalysisState.Callsites) == 0 &&
		len(document.AnalysisState.Definitions) == 0 &&
		len(document.Errors) > 0 {
		if prefix := largestParseablePrefix(text); prefix != "" {
			prefix_doc, err := NewDocument(
				ctx, self.config_obj, params.TextDocument.URI, prefix)
			if err == nil &&
				(len(prefix_doc.AnalysisState.Callsites) > 0 ||
					len(prefix_doc.AnalysisState.Definitions) > 0) {
				document.AnalysisState = prefix_doc.AnalysisState

				// Also report any semantic errors found
				// in the valid prefix.
				document.Errors = append(
					document.Errors, prefix_doc.Errors...)
			} else {
				document.AnalysisState = old_doc.AnalysisState
			}
		} else {
			document.AnalysisState = old_doc.AnalysisState
		}
	}

	self.mu.Lock()
	self.documents[params.TextDocument.URI] = document
	self.mu.Unlock()

	return document.Diagnostics(), nil
}

// largestParseablePrefix returns the largest prefix of text that parses
// cleanly, backing up to previous statement boundaries (newline or
// semicolon) if the exact prefix fails to parse. Returns "" if no
// non-empty prefix parses.
func largestParseablePrefix(text string) string {
	end := len(text)
	for end > 0 {
		prefix := text[:end]
		_, err := vfilter.MultiParse(prefix)
		if err == nil {
			return prefix
		}

		// Back up to the previous statement boundary.
		newline := strings.LastIndex(prefix, "\n")
		semicolon := strings.LastIndex(prefix, ";")
		boundary := newline
		if semicolon > boundary {
			boundary = semicolon
		}
		if boundary <= 0 {
			// Only whole-line candidates left; stop.
			return ""
		}
		end = boundary
	}
	return ""
}
