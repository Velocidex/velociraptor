package lsp

import (
	"context"
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

// The document represents the parsed VQL document and its analysis
// state. We cache the document in the server by URI so we can serve
// other lsp queries for it quickly.
type Document struct {
	URI           uri.URI
	Text          string
	AnalysisState *launcher.AnalysisState
	Errors        []*launcher.VerifierError
}

func (self *Document) Debug() string {
	res := []string{fmt.Sprintf("Document URI %v\nErrors", self.URI)}
	for _, e := range self.Errors {
		res = append(res, e.Error())
	}
	res = append(res, self.AnalysisState.Debug())
	return strings.Join(res, "\n")
}

func (self *Document) Diagnostics() (res []*protocol.Diagnostic) {
	for _, verify_error := range self.Errors {
		diag := &protocol.Diagnostic{
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional("vql"),
			Message:  protocol.String(verify_error.Error()),
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(verify_error.Pos.Pos.Line),
					Character: uint32(verify_error.Pos.Pos.Column),
				},
				End: protocol.Position{
					Line:      uint32(verify_error.Pos.EndPos.Line),
					Character: uint32(verify_error.Pos.EndPos.Column),
				},
			},
		}
		res = append(res, diag)
	}

	return res
}

func NewDocument(
	ctx context.Context,
	config_obj *config_proto.Config,
	url uri.URI,
	text string) (*Document, error) {

	repo_manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	global_repository, err := repo_manager.GetGlobalRepository(config_obj)

	state := launcher.NewAnalysisState("")
	res := &Document{
		URI:           url,
		Text:          text,
		AnalysisState: state,
	}

	for _, err := range launcher.VerifyVQL(ctx, config_obj,
		text, global_repository, state) {
		verify_error, ok := err.(*launcher.VerifierError)
		if !ok {
			verify_error = &launcher.VerifierError{
				Name:    launcher.GENERIC_ERROR,
				Message: err.Error(),
			}
		}
		res.Errors = append(res.Errors, verify_error)
	}

	return res, nil
}
