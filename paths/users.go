package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type UserPathManager struct {
	Name string
}

// Where we store user information.
func (self UserPathManager) Path() api.DSPathSpec {
	return USERS_ROOT.AddChild(self.Name).
		SetType(api.PATH_TYPE_DATASTORE_PROTO).
		SetTag("User")
}

// Where we store the user's ACLs
func (self UserPathManager) ACL() api.DSPathSpec {
	return ACL_ROOT.AddChild(self.Name).
		SetTag("UserACLS")
}

// Where we store the user's GUI preferences
func (self UserPathManager) GUIOptions() api.DSPathSpec {
	return USERS_ROOT.AddChild("gui", self.Name).SetType(
		api.PATH_TYPE_DATASTORE_JSON)
}

// Where we store the user's MRU clients
func (self UserPathManager) MRUClient(client_id string) api.DSPathSpec {
	return USERS_ROOT.AddChild(self.Name, "mru", client_id).SetType(
		api.PATH_TYPE_DATASTORE_PROTO)
}

// The directory containing all MRU clients
func (self UserPathManager) MRUIndex() api.DSPathSpec {
	return USERS_ROOT.AddChild(self.Name, "mru")
}

// Where we store the user's favorite collections
func (self UserPathManager) Favorites(name, type_name string) api.DSPathSpec {
	return USERS_ROOT.AddChild(
		self.Name, "Favorites", type_name, name).
		SetTag("Favorites")
}

// The directory that contains all the favorites collections
func (self UserPathManager) FavoriteDir(type_name string) api.DSPathSpec {
	return USERS_ROOT.AddChild(self.Name, "Favorites", type_name)
}

// Controls the schema of user related data.
func NewUserPathManager(username string) *UserPathManager {
	return &UserPathManager{username}
}
