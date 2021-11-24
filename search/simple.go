package search

import (
	"errors"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// The simple index is a legacy index that is ok for small number of
// items but it is mostly kept for backwards compatibility. Instead of
// partitioning the index, the entire path is kept as a single
// path. This is ok for smalish number of items but does not really
// scale.

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func SetSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	for _, keyword := range keywords {
		// The entity and keywords are not trusted because
		// they are user provided.
		keyword = strings.ToLower(keyword)
		subject := index_urn.AddChild(keyword, entity)
		err := db.SetSubjectWithCompletion(
			config_obj, subject, &empty.Empty{}, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func UnsetSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	for _, keyword := range keywords {
		keyword = strings.ToLower(keyword)
		subject := index_urn.AddChild(keyword, entity)
		err := db.DeleteSubject(config_obj, subject)
		if err != nil {
			return err
		}
	}
	return nil
}

func CheckSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	for _, keyword := range keywords {
		message := &empty.Empty{}
		keyword = strings.ToLower(keyword)
		subject := index_urn.AddChild(keyword, entity)
		return db.GetSubject(config_obj, subject, message)
	}
	return errors.New("Client does not have label")
}
