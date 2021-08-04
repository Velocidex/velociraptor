package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type UserPathManager struct {
	Name string
}

// Where we store user information.
func (self UserPathManager) Path() api.PathSpec {
	return USERS_ROOT.AddChild(self.Name).
		SetType(api.PATH_TYPE_DATASTORE_PROTO)
}

// The directory containing all user related info.
func (self UserPathManager) Directory() api.PathSpec {
	return USERS_ROOT.AddChild(self.Name)
}

// Where we store the user's ACLs
func (self UserPathManager) ACL() api.PathSpec {
	return ACL_ROOT.AddChild(self.Name)
}

// Where we store the user's GUI preferences
func (self UserPathManager) GUIOptions() api.PathSpec {
	return USERS_ROOT.AddChild("gui", self.Name).SetType(
		api.PATH_TYPE_DATASTORE_JSON)
}

// Where we store the user's MRU clients
func (self UserPathManager) MRUClient(client_id string) api.PathSpec {
	return USERS_ROOT.AddChild(self.Name, "mru", client_id).SetType(
		api.PATH_TYPE_DATASTORE_PROTO)
}

// The directory containing all MRU clients
func (self UserPathManager) MRUIndex() api.PathSpec {
	return USERS_ROOT.AddChild(self.Name, "mru")
}

// Where we store the user's favorite collections
func (self UserPathManager) Favorites(name, type_name string) api.PathSpec {
	return USERS_ROOT.AddChild(
		self.Name, "Favorites", type_name, name)
}

// The directory that contains all the favorites collections
func (self UserPathManager) FavoriteDir(type_name string) api.PathSpec {
	return USERS_ROOT.AddChild(self.Name, "Favorites", type_name)
}

// Where user notifications will be written.
func (self UserPathManager) Notifications() api.PathSpec {
	return USERS_ROOT.AddChild(self.Name, "notifications")
}

// Controls the schema of user related data.
func NewUserPathManager(username string) *UserPathManager {
	return &UserPathManager{username}
}
