//
package datastore

import (
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"testing"
	"www.velocidex.com/golang/velociraptor/config"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func TestPaths(t *testing.T) {
	assert.Equal(
		t,
		"test_data/C%2Eb4f82077e4af5ba7.sqlite",
		getDBPathForClient("test_data", "C.b4f82077e4af5ba7"),
	)
}

func xxxxTestMain(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore_location = proto.String("/tmp/velociraptor")

	db, pres := GetImpl("SqliteDataStore")
	if !pres {
		t.Fatalf("No such implementation")
	}

	messages, err := db.GetClientTasks(config_obj, "C.b4f82077e4af5ba7")
	if err != nil {
		t.Fatalf(err.Error())
	}

	utils.Debug(messages)
}
