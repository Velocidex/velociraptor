package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/utils"
)

// PrepareRename checks whether the identifier under the cursor can be
// renamed. Currently only LET variables can be renamed; registry
// plugins and functions are global names so a rename request is
// rejected.
func (self *LSPServer) PrepareRename(
	ctx context.Context,
	params *protocol.PrepareRenameParams) (
	protocol.PrepareRenameResult, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	position_mapper := newPositionMapper(doc.Text)
	offset_at_point := position_mapper.positionToOffset(
		int(params.Position.Line), int(params.Position.Character))

	name := wordAtOffset(doc.Text, offset_at_point)
	if name == "" {
		return nil, nil
	}

	definitions, _, err := analyseDocument(doc.Text)
	if err != nil {
		return nil, err
	}

	// Only LET variables can be renamed.
	if _, pres := definitions[name]; !pres {
		return nil, nil
	}

	return &protocol.Range{
		Start: position_mapper.mapOffset(offset_at_point),
		End: position_mapper.mapOffset(
			offset_at_point + len(name)),
	}, nil
}

// Rename renames a LET variable everywhere it is used in the document.
func (self *LSPServer) Rename(
	ctx context.Context,
	params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	position_mapper := newPositionMapper(doc.Text)
	offset_at_point := position_mapper.positionToOffset(
		int(params.Position.Line), int(params.Position.Character))

	name := wordAtOffset(doc.Text, offset_at_point)
	if name == "" {
		return nil, nil
	}

	definitions, callsites, err := analyseDocument(doc.Text)
	if err != nil {
		return nil, err
	}

	// Only LET variables can be renamed.
	def, pres := definitions[name]
	if !pres {
		return nil, nil
	}

	edits := []protocol.TextEdit{}
	seen := make(map[protocol.Range]bool)

	addEdit := func(offset int, length int) {
		if offset < 0 || offset+length > len(doc.Text) {
			return
		}
		rng := protocol.Range{
			Start: position_mapper.mapOffset(offset),
			End:   position_mapper.mapOffset(offset + length),
		}
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
	addEdit(offset, len(name))

	// Rename every bare symbol use.
	for _, cs := range callsites {
		if cs.Type == "symbol" && cs.Name == name {
			addEdit(cs.Pos.Pos.Offset, len(cs.Name))
		}
	}

	return &protocol.WorkspaceEdit{
		Changes: map[uri.URI][]protocol.TextEdit{
			params.TextDocument.URI: edits,
		},
	}, nil
}
