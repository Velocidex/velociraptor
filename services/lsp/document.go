package lsp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	OutOfBoundError = utils.Wrap(utils.NotFoundError,
		"Position outside document")
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

	// All the lines in the text, used for quick lookup
	Lines []vfilter.RangePosition
}

func isWhitespace(c uint8) bool {
	return c == ' ' || c == 0 || c == '\n' || c == '\t' || c == '\r'
}

func isIdentifier(c uint8) bool {
	return c == '.' || c == '_' || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// Return the text specified by the exclusive range `rng`.  Note that
// the last char in the returned string will be the same as
// self.getChar(rng.EndPos.Offset-1)
func (self *Document) GetFragmentByRange(rng vfilter.RangePosition) string {
	return self.GetFragment(rng.Pos.Offset, rng.EndPos.Offset)
}

// WordAtPos returns the identifier range at the specified
// position. Identifiers in VQL can contain letters, digits,
// underscores and dots (for dotted plugin names like
// Artifact.Linux.Sys.Users).
//
// If the cursor is on a white space, the function seeks back through
// the whitespace to locate the identifier. If the identifier goes
// ahead past the cursor then the function seeks forward to find the
// entire identifier.
//
// If a sentinel is provided it is added to the identifier if it is
// found.
func (self *Document) WordAtPos(pos lexer.Position, sentinel uint8) (
	res vfilter.RangePosition) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if pos.Offset >= len(self.Text) {
		pos = self._FullRange().EndPos
	}

	// Search back to skip any white space before the cursor.
	for pos.Offset > 0 && pos.Column > 0 {
		c := self.getChar(pos.Offset)
		if !isWhitespace(c) {
			break
		}
		pos.Offset--
		pos.Column--
	}

	res.Pos = pos
	res.EndPos = pos

	// Search backwards for the start of the identifier.
	if isIdentifier(self.getChar(res.Pos.Offset)) ||
		(sentinel > 0 && self.getChar(res.EndPos.Offset) == sentinel) {
		for res.Pos.Offset > 0 && res.Pos.Column > 0 {
			c := self.getChar(res.Pos.Offset - 1)
			if !isIdentifier(c) {
				break
			}
			res.Pos.Offset--
			res.Pos.Column--
		}
	}

	// Search forward to the end of the identifier.
	for res.EndPos.Offset < len(self.Text) {
		c := self.getChar(res.EndPos.Offset)
		if !isIdentifier(c) {
			break
		}
		res.EndPos.Offset++
		res.EndPos.Column++
	}

	// If a sentinel is specified try to find it immediately ahead and
	// include it into the identifier string.
	if sentinel > 0 {
		c := self.getChar(res.EndPos.Offset)
		if c == sentinel {
			res.EndPos.Offset++
			res.EndPos.Column++
		}
	}

	return res
}

func (self *Document) getChar(offset int) uint8 {
	// Accessing past the end of the query text returns a space. This
	// ensures we never panic, even when accessing past the end of the
	// query.
	if offset > len(self.Text)-1 {
		return ' '
	}
	return self.Text[offset]
}

// The full range of the document.
func (self *Document) FullRange() (res vfilter.RangePosition) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._FullRange()
}

func (self *Document) _FullRange() (res vfilter.RangePosition) {
	if len(self.Lines) > 0 {
		res.Pos = self.Lines[0].Pos
		res.EndPos = self.Lines[len(self.Lines)-1].EndPos
	}
	return res
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
	res := []string{fmt.Sprintf(
		"Document URI %v\nErrors", self.URI)}
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
		Definitions: make(map[string]vfilter.DefinitionSite),
	}

	// Update the top level queries
	for _, tlp := range state.TopLevelQueries {
		old_text := self.GetFragmentByRange(tlp.Pos)
		new_text := other.GetFragmentByRange(tlp.Pos)
		if old_text == new_text {
			new_state.TopLevelQueries = append(
				new_state.TopLevelQueries, tlp)
		}
	}

	// Update the callsites
	for _, cs := range state.Callsites {
		old_text := self.GetFragment(cs.Pos.Pos.Offset,
			cs.Pos.Pos.Offset+len(cs.Name))
		new_text := other.GetFragment(cs.Pos.Pos.Offset,
			cs.Pos.Pos.Offset+len(cs.Name))
		if old_text == new_text {
			new_state.Callsites = append(new_state.Callsites, cs)
		}
	}

	// Update the definitions
	for key, desc := range state.Definitions {
		old_text := self.GetFragmentByRange(desc.Pos)
		new_text := other.GetFragmentByRange(desc.Pos)
		if old_text == new_text {
			new_state.Definitions[key] = desc
		}
	}

	// Merge the new state to the current state.
	state.TopLevelQueries = new_state.TopLevelQueries
	state.Callsites = new_state.Callsites
	state.Definitions = new_state.Definitions
	state.FailedToParse = other.AnalysisState.FailedToParse

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

func buildLineIndex(in string) (res []vfilter.RangePosition) {
	current := vfilter.RangePosition{
		Pos: lexer.Position{
			Column: 1,
			Line:   1,
		},
		EndPos: lexer.Position{
			Column: 1,
			Line:   1,
		},
	}
	for idx := 0; idx < len(in); idx++ {
		c := in[idx]

		// EndPos is exclusive range, so the offset represents the
		// index of the next char.
		current.EndPos.Offset = idx + 1

		// Column is a 1 based counter so +1
		current.EndPos.Column = current.EndPos.Offset - current.Pos.Offset + 1

		// Push the next line when we hit the \n
		if c == '\n' {
			res = append(res, current)

			new_line := vfilter.RangePosition{}
			new_line.Pos.Column = 1

			// Next line starts in the next char.
			new_line.Pos.Offset = idx + 1
			new_line.Pos.Line = current.Pos.Line + 1
			new_line.EndPos = new_line.Pos
			current = new_line
		}
	}

	// Last line has no \n
	res = append(res, current)
	return res
}

// Convert from 0 based lsp protocol positions to 1 based lexer
// positions.
func (self *Document) LexerPositionFromProtocol(
	in protocol.Position) *lexer.Position {
	self.mu.Lock()
	defer self.mu.Unlock()

	if len(self.Lines) <= int(in.Line) {
		res := self._FullRange().EndPos
		return &res
	}

	res := &lexer.Position{
		Line:   int(in.Line + 1),
		Column: int(in.Character + 1),
	}

	// Fill in the offset from the line index.
	line := self.Lines[in.Line]
	if line.EndPos.Column < res.Column {
		// Clamp to the end of the line
		return &line.EndPos
	}

	res.Offset = line.Pos.Offset + int(in.Character)
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
		Lines:         buildLineIndex(text),
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

// Calculate the full range of the document by counting the lines and
// columns.
func getFullRange(text string) (res vfilter.RangePosition) {
	for _, c := range text {
		if c == '\n' {
			res.EndPos.Line++
			res.EndPos.Column = 0
		} else {
			res.EndPos.Column++
		}
		res.EndPos.Offset++
	}
	return res
}
