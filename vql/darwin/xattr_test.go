package darwin

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/ivaxer/go-xattr"
	"github.com/stretchr/testify/suite"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type XattrTestSuite struct {
	suite.Suite
}

func (self *XattrTestSuite) TestXAttr() {
	filepath := "/tmp/xattr.test"
	attr := "vr.test"

	_, err := os.Create(filepath)
	if err != nil {
		self.T().Errorf("xattr: failed to create test file: %s", err)
		return
	}

	xattr.Set(filepath, attr, []byte("test-value"))

	ctx := context.Background()
	sub_ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	testcases := []struct {
		name string
		attr string
		pass bool
	}{
		{
			name: filepath,
			attr: attr,
			pass: true,
		},
		{
			name: filepath,
			attr: "vr.wrong",
			pass: false,
		},
	}

	for _, test := range testcases {
		defer scope.Close()
		output_chan := XAttrPlugin{}.Call(sub_ctx, scope, ordereddict.NewDict().Set("filename", test.name).Set("attribute", test.attr))
		select {
		case event, ok := <-output_chan:
			if ok != test.pass {
				self.T().Errorf("xattr: Unexpected OK status. Expected %t, got %t", test.pass, ok)
				continue
			}

			if (event == nil) == test.pass {
				self.T().Errorf("xattr: Unexpected event type, got %T", event)
			}
		}
	}

	os.Remove(filepath)
}

func TestXAttrPlugin(t *testing.T) {
	suite.Run(t, &XattrTestSuite{})
}
