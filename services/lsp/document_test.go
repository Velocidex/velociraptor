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

	fmt.Println(strings.Join(golden, "\n"))

	goldie.Assert(self.T(), "TestDocuments",
		[]byte(strings.Join(golden, "\n")))
}
