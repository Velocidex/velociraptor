package lsp_test

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
)

var (
	documentTC = []struct {
		Name  string
		Query string
	}{{
		Name:  "Query on one line",
		Query: "SELECT * FROM info()",
	}, {
		Name: "Query with errors",
		Query: `
LET X = 1
SELECT geoip(db='Foo', ip='127.0.0.1', XXX=1) AS Foo
FROM glob(globs='*')
`,
	}, {
		Name: "Sub Query",
		Query: `
LET Add(X) = X + 1

SELECT {
   SELECT geoip(db='Foo', ip='127.0.0.1', XXX=1) AS Foo
   FROM glob(globs='*', accessor=Add(X='file'))
} AS B
FROM scope()
`,
	}, {
		Name: "Columns",
		Query: `
LET Session <= shell_session(argv=["XXXbash"])

SELECT Session.IsRunning, version(function="shell_session")
FROM scope()

LET _ <= background(query={
  SELECT * FROM chain(a={
    SELECT sleep(time=1), shell_session_control(
       stdin=format(format="echo hello %v\n\n", args=_value)) AS Session
    FROM range(end=10)
    WHERE log(dedup= -1,
       message="Session.IsRunning = %v", args=Session.IsRunning)
 }, b={
   SELECT shell_session_control(close_stdin=TRUE)
   FROM scope()
 })
})

SELECT * FROM foreach(row=Session.Query)
`,
	}}
)

func (self *LSPTestSuite) TestDocuments() {

	var golden []string

	for idx, tc := range documentTC {
		url := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n---%v---",
			idx, tc.Name, tc.Query))

		doc, err := lsp.NewDocument(
			self.Ctx, self.ConfigObj, url, tc.Query)
		assert.NoError(self.T(), err)

		golden = append(golden, doc.Debug()+
			fmt.Sprintf("\nFullRange %v (length %d)",
				DebugRange(doc.FullRange()), len(doc.Text)))
		golden = append(golden, "Line Ranges:")
		for _, l := range doc.Lines {
			golden = append(golden, DebugRange(l))
			golden = append(golden,
				strconv.Quote(doc.GetFragmentByRange(l)))
		}
	}

	goldie.Assert(self.T(), "TestDocuments",
		[]byte(strings.Join(golden, "\n")))
}

/*
WordAtPos searched for the identifier just before the cursor. If the
cursor is currently on a whitespace, the function seeks back to the
previous identifier. If the identifier also goes forward past the
cursor it is also extracted. The function can accept a sentinel
character, which will be considered part of the identifier if it is
present.
*/
func (self *LSPTestSuite) TestDocumentWordAtPos() {
	text := "SELECT * FROM     info(foo='hello')"
	doc, err := lsp.NewDocument(self.Ctx,
		self.ConfigObj, uri.URI("http://XXX"), text)
	assert.NoError(self.T(), err)

	golden := ""
	for i := 0; i < len(text); i++ {
		rng := doc.WordAtPos(lexer.Position{
			Offset: i,
			Column: i,
		}, '(')

		golden += fmt.Sprintf("%c: to cursor: '%v' identifier: '%v'\n",
			text[i],
			doc.GetFragment(rng.Pos.Offset, i),
			doc.GetFragmentByRange(rng))
	}

	goldie.Assert(self.T(), "TestDocumentWordAtPos", []byte(golden))
}

func DebugRange(in vfilter.RangePosition) string {
	return fmt.Sprintf("%d,%d (%d) -> %d,%d (%d)",
		in.Pos.Line, in.Pos.Column, in.Pos.Offset,
		in.EndPos.Line, in.EndPos.Column, in.EndPos.Offset)
}
