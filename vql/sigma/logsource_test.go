package sigma

import (
	"context"
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"

	// For items plugin
	_ "www.velocidex.com/golang/velociraptor/vql/common"
	_ "www.velocidex.com/golang/velociraptor/vql/golang"
)

func (self *SigmaTestSuite) TestLogSourceIterator() {
	ctx := context.Background()

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stdout, "", 0))
	defer scope.Close()

	queries := []string{
		"LET X <= sigma_log_sources(`*/windows/application`={SELECT * FROM info()})",
		`
SELECT * FROM foreach(
row={
   SELECT _value FROM items(item=X)
}, query={
   SELECT typeof(a=_value), _value FROM scope()
})
`,
	}

	results := ordereddict.NewDict()
	for _, query := range queries {
		rows := []vfilter.Row{}
		vql, err := vfilter.Parse(query)
		assert.NoError(self.T(), err)

		for row := range vql.Eval(ctx, scope) {
			rows = append(rows, row)
		}
		results.Set(query, rows)
	}

	json.Dump(results)
}
