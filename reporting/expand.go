// Produce reports of collected artifacts.

package reporting

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/template"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type Expansions struct {
	config_obj *api_proto.Config
	rows       []vfilter.Row
}

// Support a number of expansions in description strings.
func FormatDescription(
	config_obj *api_proto.Config,
	description string,
	rows []vfilter.Row) string {

	expansion := &Expansions{
		config_obj: config_obj,
		rows:       rows,
	}

	tmpl, err := template.New("description").Funcs(
		template.FuncMap{
			"DocFrom": expansion.DocFrom,
			"Query":   expansion.Query,
		}).Parse(description)
	if err != nil {
		return description
	}

	buffer := &bytes.Buffer{}

	err = tmpl.Execute(buffer, expansion)
	if err != nil {
		utils.Debug(err)
		return description
	}

	return string(buffer.Bytes())
}

func (self *Expansions) DocFrom(artifact string) string {
	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return ""
	}

	artifact_definition, pres := repository.Get(artifact)
	if !pres {
		return ""
	}

	return artifact_definition.Description
}

func (self *Expansions) Query(queries ...string) string {
	result := &bytes.Buffer{}

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	env := vfilter.NewDict().Set("Rows", self.rows)

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = log.New(os.Stderr, "velociraptor: ", log.Lshortfile)

	for _, query := range queries {
		vql, err := vfilter.Parse(query)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		table := EvalQueryToTable(context.Background(), scope, vql, result)
		table.Render()
	}

	return string(result.Bytes())
}
