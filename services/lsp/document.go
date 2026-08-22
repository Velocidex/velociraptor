package lsp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alecthomas/participle/v2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/vfilter"
)

// The document represents the parsed VQL document and its analysis
// state. We cache the document in the server by URI so we can serve
// other lsp queries for it quickly.
type Document struct {
	mu sync.Mutex

	URI           uri.URI
	Text          string
	AnalysisState *launcher.AnalysisState
	Errors        []*launcher.VerifierError
	tokens        []vfilter.Token
}

func (self *Document) Tokenize() []vfilter.Token {
	self.mu.Lock()
	defer self.mu.Unlock()

	tokens, err := vfilter.Tokenize(self.Text)
	if err != nil {
		return self.tokens
	}

	self.tokens = tokens
	return tokens
}

func (self *Document) Debug() string {
	res := []string{fmt.Sprintf("Document URI %v\nErrors", self.URI)}
	for _, e := range self.Errors {
		res = append(res, e.Error())
	}
	res = append(res, self.AnalysisState.Debug())
	return strings.Join(res, "\n")
}

// Update the current analysis state to the new text. We try to keep
// as many of the callsite as possible by verifying them against the
// new text.
func (self *Document) UpdateTextFromDocument(other *Document) {
	state := self.AnalysisState

	new_state := &launcher.AnalysisState{
		Definitions:   make(map[string]vfilter.DefinitionSite),
		FailedToParse: other.AnalysisState.FailedToParse,
	}

	// Update the top level queries
	for _, tlp := range state.TopLevelQueries {
		old_text := self.getFragment(tlp.Pos.Pos.Offset,
			tlp.Pos.EndPos.Offset)
		new_text := other.getFragment(tlp.Pos.Pos.Offset,
			tlp.Pos.EndPos.Offset)
		if old_text == new_text {
			new_state.TopLevelQueries = append(
				new_state.TopLevelQueries, tlp)
		}
	}

	// Update the callsites
	for _, cs := range state.Callsites {
		old_text := self.getFragment(cs.Pos.Pos.Offset,
			cs.Pos.Pos.Offset+len(cs.Name))
		new_text := other.getFragment(cs.Pos.Pos.Offset,
			cs.Pos.Pos.Offset+len(cs.Name))
		if old_text == new_text {
			new_state.Callsites = append(new_state.Callsites, cs)
		}
	}

	// Update the definitions
	for key, desc := range state.Definitions {
		old_text := self.getFragment(desc.Pos.Pos.Offset,
			desc.Pos.EndPos.Offset)
		new_text := other.getFragment(desc.Pos.Pos.Offset,
			desc.Pos.EndPos.Offset)
		if old_text == new_text {
			new_state.Definitions[key] = desc
		}
	}

	// Merge the new state to the current state.
	state.TopLevelQueries = new_state.TopLevelQueries
	state.Callsites = new_state.Callsites
	state.Definitions = new_state.Definitions
	self.Text = other.Text
}

func (self *Document) Diagnostics() (res []*protocol.Diagnostic) {
	for _, verify_error := range self.Errors {
		diag := &protocol.Diagnostic{
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional("vql"),
			Message:  protocol.String(verify_error.Error()),
			Range:    *protocolRange(verify_error.Pos),
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

			// Syntax errors carry the position of the
			// offending token - extract it so the diagnostic
			// points at the broken statement instead of the
			// top of the document.
			var parse_err participle.Error
			if errors.As(err, &parse_err) {
				pos := parse_err.Position()
				verify_error.Pos = vfilter.RangePosition{
					Pos:    pos,
					EndPos: pos,
				}

				// The unadorned message is cleaner than
				// the wrapped string which contains a
				// source snippet.
				verify_error.Message = parse_err.Message()
			}
		}
		res.Errors = append(res.Errors, verify_error)
	}

	return res, nil
}
