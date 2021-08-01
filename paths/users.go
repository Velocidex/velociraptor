package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type UserPathManager struct {
	Name string
}

// Where we store user information.
func (self UserPathManager) Path() api.UnsafeDatastorePath {
	return USER_URN.AddComponent(self.Name)
}

// The directory containing all user related info.
func (self UserPathManager) Directory() api.UnsafeDatastorePath {
	return USER_URN.AddComponent(self.Name)
}

// Where we store the user's ACLs
func (self UserPathManager) ACL() api.UnsafeDatastorePath {
	return api.NewSafeDatastorePath("acl").AddComponent(self.Name)
}

// Where we store the user's GUI preferences
func (self UserPathManager) GUIOptions() api.UnsafeDatastorePath {
	return USER_URN.AddComponent("gui", self.Name)
}

// Where we store the user's MRU clients
func (self UserPathManager) MRUClient(client_id string) api.UnsafeDatastorePath {
	return USER_URN.AddComponent(self.Name, "mru", client_id)
}

// The directory containing all MRU clients
func (self UserPathManager) MRUIndex() api.UnsafeDatastorePath {
	return USER_URN.AddComponent(self.Name, "mru")
}

// Where we store the user's favorite collections
func (self UserPathManager) Favorites(name, type_name string) api.UnsafeDatastorePath {
	return USER_URN.AddComponent(
		self.Name, "Favorites", type_name, name)
}

// The directory that contains all the favorites collections
func (self UserPathManager) FavoriteDir(type_name string) api.UnsafeDatastorePath {
	return USER_URN.AddComponent(self.Name, "Favorites", type_name)
}

// Controls the schema of user related data.
func NewUserPathManager(username string) *UserPathManager {
	return &UserPathManager{username}
}
