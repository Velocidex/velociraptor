package goldie

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Velocidex/json"
	"github.com/sebdah/goldie/v2"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func Retry(r *assert.R, t *testing.T, filename string, golden []byte) {
	g := goldie.New(t)
	_ = g.WithFixtureDir("fixtures")

	if r.MaxAttempts == r.Attempt {
		g.Assert(t, filename, golden)
		return
	}

	expectedData, err := ioutil.ReadFile(g.GoldenFileName(t, filename))
	if err != nil {
		if os.IsNotExist(err) {
			// Call the original to allow it to update the golden
			// fixture.
			g.Assert(t, filename, golden)
			return
		}
	}

	if !bytes.Equal(golden, expectedData) {
		r.Fatalf("Result did not match the golden fixture. Retrying....")
	}
}

func Assert(t *testing.T, filename string, golden []byte) {
	t.Helper()

	g := goldie.New(t)
	_ = g.WithFixtureDir("fixtures")
	g.Assert(t, filename, golden)
}

func AssertJson(t *testing.T, filename string, golden interface{}) {
	t.Helper()

	g := goldie.New(t)
	_ = g.WithFixtureDir("fixtures")
	g.Assert(t, filename, MustMarshalIndent(golden))
}

func MustMarshalIndent(v interface{}) []byte {
	result, err := MarshalIndent(v)
	if err != nil {
		panic(err)
	}
	return result
}

func MarshalIndent(v interface{}) ([]byte, error) {
	opts := json.NewEncOpts()
	b, err := json.MarshalWithOptions(v, opts)
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	err = json.Indent(buf, b, "", " ")
	return buf.Bytes(), err
}
