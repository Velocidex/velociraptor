package wmi_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	wmi_parse "www.velocidex.com/golang/velociraptor/vql/windows/wmi/parse"
)

func TestParseMOF(t *testing.T) {
	fd, err := os.Open("fixtures/sample.txt")
	assert.NoError(t, err, "open file")

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err, "ReadAll")

	mof, err := wmi_parse.Parse(string(data))
	assert.NoError(t, err, "Parse")

	encoded, err := json.MarshalIndent(mof.ToDict(), " ", "")
	assert.NoError(t, err, "json.MarshalIndent")

	goldie.Assert(t, "sample", encoded)
}
