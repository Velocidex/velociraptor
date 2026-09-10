package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// PrepareRename checks whether the identifier under the cursor can be
// renamed. Currently only LET variables can be renamed; registry
// plugins and functions are global names so a rename request is
// rejected.
func (self *LSPServer) PrepareRename(
	ctx context.Context,
	params *protocol.PrepareRenameParams) (
	protocol.PrepareRenameResult, error) {

	doc, err := self.GetDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	cursor := doc.LexerPositionFromProtocol(params.Position)
	rng := doc.WordAtPos(*cursor, ' ')
	name := doc.GetFragmentByRange(rng)
	if name == "" {
		return nil, nil
	}

	// Only LET variables can be renamed.
	_, pres := doc.AnalysisState.Definitions[name]
	if !pres {
		return nil, nil
	}

	return protocolRange(rng), nil
}

// Rename renames a LET variable everywhere it is used in the document.
func (self *LSPServer) Rename(
	ctx context.Context,
	params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {

	doc, err := self.GetDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	cursor := doc.LexerPositionFromProtocol(params.Position)
	rng := doc.WordAtPos(*cursor, ' ')
	name := doc.GetFragmentByRange(rng)
	if name == "" {
		return nil, nil
	}

	// Only LET variables can be renamed.
	def, pres := doc.AnalysisState.Definitions[name]
	if !pres {
		return nil, nil
	}

	edits := []protocol.TextEdit{}
	seen := make(map[protocol.Range]bool)

	addEdit := func(start protocol.Position, length int) {
		rng := protocol.Range{
			Start: start,
			End:   start,
		}
		rng.End.Character += uint32(length)
		if seen[rng] {
			return
		}
		seen[rng] = true
		edits = append(edits, protocol.TextEdit{
			Range:   rng,
			NewText: params.NewName,
		})
	}

	// Rename the definition.
	offset := definitionNameOffset(doc.Text, def)
	if offset < 0 {
		return nil, nil
	}
	addEdit(offsetToPosition(doc.Text, offset), len(name))

	// Rename every bare symbol use.
	for _, cs := range doc.AnalysisState.Callsites {
		if cs.Type == "symbol" && cs.Name == name {
			addEdit(protocolPosition(cs.Pos.Pos), len(cs.Name))
		}
	}

	return &protocol.WorkspaceEdit{
		Changes: map[uri.URI][]protocol.TextEdit{
			params.TextDocument.URI: edits,
		},
	}, nil
}
