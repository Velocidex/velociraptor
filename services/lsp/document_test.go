package lsp_test

import (
	"fmt"
	"strings"

	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	documentTC = []struct {
		Name  string
		Query string
	}{{
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
			"\nTest case %d: %s\n%v",
			idx, tc.Name, tc.Query))

		doc, err := lsp.NewDocument(
			self.Ctx, self.ConfigObj, url, tc.Query)
		assert.NoError(self.T(), err)

		golden = append(golden, doc.Debug())
	}

	goldie.Assert(self.T(), "TestDocuments",
		[]byte(strings.Join(golden, "\n")))
}
