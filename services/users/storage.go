package users

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Responsible for storing the User records
type IUserStorageManager interface {
	GetUserWithHashes(ctx context.Context, username string) (
		*api_proto.VelociraptorUser, error)

	SetUser(ctx context.Context, user_record *api_proto.VelociraptorUser) error

	ListAllUsers(ctx context.Context) ([]*api_proto.VelociraptorUser, error)

	GetUserOptions(ctx context.Context, username string) (
		*api_proto.SetGUIOptionsRequest, error)

	SetUserOptions(ctx context.Context,
		username string, options *api_proto.SetGUIOptionsRequest) error

	// Favourites are stored per org.
	GetFavorites(
		ctx context.Context,
		org_config_obj *config_proto.Config,
		principal, fav_type string) (*api_proto.Favorites, error)

	DeleteUser(ctx context.Context, username string) error
}

// The NullStorage Manager is used for tools and clients. In this
// configuration there are no users and none of the user based VQL
// plugins will work.
type NullStorageManager struct{}

func (self *NullStorageManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	return nil, errors.New("Not Found")
}

func (self *NullStorageManager) SetUser(ctx context.Context,
	user_record *api_proto.VelociraptorUser) error {
	return errors.New("Not Implemented")
}

func (self *NullStorageManager) ListAllUsers(
	ctx context.Context) ([]*api_proto.VelociraptorUser, error) {
	return nil, errors.New("Not Implemented")
}

func (self *NullStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	return nil, errors.New("Not Implemented")
}

func (self *NullStorageManager) SetUserOptions(ctx context.Context,
	username string, options *api_proto.SetGUIOptionsRequest) error {
	return errors.New("Not Implemented")
}

func (self *NullStorageManager) DeleteUser(ctx context.Context, username string) error {
	return errors.New("Not Implemented")
}

func (self *NullStorageManager) GetFavorites(
	ctx context.Context, org_config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	return nil, errors.New("Not Implemented")
}

/*
  The User Manager is responsible for coordinating access to user
  records.
*/

// The object that is cached in the LRU
type _CachedUserObject struct {
	user_record *api_proto.VelociraptorUser
	gui_options *api_proto.SetGUIOptionsRequest
}

type UserStorageManager struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	lru *ttlcache.Cache
	id  int64
}

func (self *UserStorageManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if username == "" {
		return nil, errors.New("Must set a username")
	}

	var cache *_CachedUserObject
	var ok bool

	// Check the LRU for a cache if it is there
	cache_any, err := self.lru.Get(username)
	if err == nil {
		cache, ok = cache_any.(*_CachedUserObject)
		if ok && cache.user_record != nil {
			return proto.Clone(cache.user_record).(*api_proto.VelociraptorUser), nil
		}
	}

	// Otherwise add a new cache
	if cache == nil {
		cache = &_CachedUserObject{}
	}

	err = validateUsername(self.config_obj, username)
	if err != nil {
		return nil, err
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	user_record := &api_proto.VelociraptorUser{}
	err = db.GetSubject(self.config_obj,
		paths.UserPathManager{Name: username}.Path(), user_record)
	if errors.Is(err, os.ErrNotExist) || user_record.Name == "" {
		return nil, services.UserNotFoundError
	}

	if err != nil {
		return nil, err
	}

	// Add the record to the lru
	cache.user_record = proto.Clone(user_record).(*api_proto.VelociraptorUser)
	self.lru.Set(username, cache)

	return user_record, nil
}

// Update the record in the LRU
func (self *UserStorageManager) SetUser(
	ctx context.Context, user_record *api_proto.VelociraptorUser) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if user_record.Name == "" {
		return errors.New("Must set a username")
	}

	err := validateUsername(self.config_obj, user_record.Name)
	if err != nil {
		return err
	}

	var cache *_CachedUserObject

	// Check the LRU for a cache if it is there
	cache_any, err := self.lru.Get(user_record.Name)
	if err == nil {
		cache, _ = cache_any.(*_CachedUserObject)
	}
	if cache == nil {
		cache = &_CachedUserObject{}
	}
	cache.user_record = proto.Clone(user_record).(*api_proto.VelociraptorUser)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(self.config_obj,
		paths.UserPathManager{Name: user_record.Name}.Path(),
		user_record)
	if err != nil {
		return err
	}

	self.lru.Set(user_record.Name, cache)
	return self.notifyChanges(ctx, user_record.Name)
}

// Advertise the changes. This will force all minions to flush their
// caches.
func (self *UserStorageManager) notifyChanges(
	ctx context.Context, username string) error {
	journal_service, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	return journal_service.PushRowsToArtifact(ctx, self.config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().Set("id", self.id).Set("username", username),
		},
		"Server.Internal.UserManager", "server", "")
}

func (self *UserStorageManager) ListAllUsers(
	ctx context.Context) ([]*api_proto.VelociraptorUser, error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	children, err := db.ListChildren(self.config_obj, paths.USERS_ROOT)
	if err != nil {
		return nil, err
	}

	result := make([]*api_proto.VelociraptorUser, 0, len(children))
	for _, child := range children {
		if child.IsDir() {
			continue
		}

		username := child.Base()
		user_record, err := self.GetUserWithHashes(ctx, username)
		if err == nil {
			user_record.PasswordHash = nil
			user_record.PasswordSalt = nil
			user_record.Orgs = nil
			result = append(result, user_record)
		}
	}

	return result, nil
}

func (self *UserStorageManager) SetUserOptions(ctx context.Context,
	username string, options *api_proto.SetGUIOptionsRequest) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	var cache *_CachedUserObject

	// Check the LRU for a cache if it is there
	cache_any, err := self.lru.Get(username)
	if err == nil {
		cache, _ = cache_any.(*_CachedUserObject)
	}
	if cache == nil {
		cache = &_CachedUserObject{}
	}

	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// Merge the old options with the new options
	old_options, err := self.getUserOptions(ctx, username)
	if err != nil {
		old_options = &api_proto.SetGUIOptionsRequest{}
	}

	// For now we do not allow the user to set the links in their
	// profile.
	old_options.Links = nil

	if options.Lang != "" {
		old_options.Lang = options.Lang
	}

	if options.Theme != "" {
		old_options.Theme = options.Theme
	}

	if options.Timezone != "" {
		old_options.Timezone = options.Timezone
	}

	if options.Org != "" {
		old_options.Org = options.Org
	}

	if options.Options != "" {
		old_options.Options = options.Options
	}

	old_options.DefaultPassword = options.DefaultPassword
	old_options.DefaultDownloadsLock = options.DefaultDownloadsLock

	err = db.SetSubject(self.config_obj, path_manager.GUIOptions(), old_options)
	if err != nil {
		return err
	}

	// Update the LRU to hold the latest version from disk.
	cache.gui_options = proto.Clone(old_options).(*api_proto.SetGUIOptionsRequest)
	self.lru.Set(username, cache)

	return self.notifyChanges(ctx, username)
}

func (self *UserStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.getUserOptions(ctx, username)
}

func (self *UserStorageManager) getUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {

	var cache *_CachedUserObject
	var ok bool

	// Check the LRU for a cache if it is there
	cache_any, err := self.lru.Get(username)
	if err == nil {
		cache, ok = cache_any.(*_CachedUserObject)
		if ok && cache.gui_options != nil {
			return proto.Clone(cache.gui_options).(*api_proto.SetGUIOptionsRequest), nil
		}
	}

	// Otherwise add a new cache
	if cache == nil {
		cache = &_CachedUserObject{}
	}

	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	options := &api_proto.SetGUIOptionsRequest{}
	err = db.GetSubject(self.config_obj, path_manager.GUIOptions(), options)
	if options.Options == "" {
		options.Options = default_user_options
	}

	// Add any links in the config file to the user's preferences.
	if self.config_obj.GUI != nil {
		options.Links = MergeGUILinks(options.Links, self.config_obj.GUI.Links)
	}

	// Add the defaults.
	options.Links = MergeGUILinks(options.Links, DefaultLinks)

	// NOTE: It is possible for a user to disable one of the default
	// targets by simply adding an entry with disabled: true - we will
	// not override the configured link from the default and it will
	// be ignored.

	defaults := &config_proto.Defaults{}
	if self.config_obj.Defaults != nil {
		defaults = self.config_obj.Defaults
	}

	// Deprecated - moved to customizations
	options.DisableServerEvents = defaults.DisableServerEvents
	options.DisableQuarantineButton = defaults.DisableQuarantineButton

	if options.Customizations == nil {
		options.Customizations = &api_proto.GUICustomizations{}
	}
	options.Customizations.HuntExpiryHours = defaults.HuntExpiryHours
	options.Customizations.DisableServerEvents = defaults.DisableServerEvents
	options.Customizations.DisableQuarantineButton = defaults.DisableQuarantineButton

	// Specify a default theme if specified in the config file.
	if options.Theme == "" {
		options.Theme = defaults.DefaultTheme
	}

	// Add the record to the lru
	cache.gui_options = proto.Clone(options).(*api_proto.SetGUIOptionsRequest)
	self.lru.Set(username, cache)

	return options, nil
}

func (self *UserStorageManager) GetFavorites(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	result := &api_proto.Favorites{}
	path_manager := paths.NewUserPathManager(principal)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	components := path_manager.FavoriteDir(fav_type)
	children, err := db.ListChildren(config_obj, components)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		if child.IsDir() {
			continue
		}

		fav := &api_proto.Favorite{}
		err = db.GetSubject(config_obj,
			path_manager.Favorites(child.Base(), fav_type), fav)
		if err == nil {
			result.Items = append(result.Items, fav)
		}
	}

	return result, nil
}

func (self *UserStorageManager) DeleteUser(ctx context.Context, username string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// No more orgs for this user, Just remove the user completely
	user_path_manager := paths.NewUserPathManager(username)
	err = db.DeleteSubject(self.config_obj, user_path_manager.Path())
	if err != nil {
		return err
	}

	self.lru.Remove(username)
	return self.notifyChanges(ctx, username)
}

func NewUserStorageManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*UserStorageManager, error) {
	result := &UserStorageManager{
		config_obj: config_obj,
		lru:        ttlcache.NewCache(),
		id:         utils.GetGUID(),
	}

	result.lru.SetCacheSizeLimit(1000)
	result.lru.SetTTL(time.Minute)

	journal_service, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}
	events, cancel := journal_service.Watch(ctx,
		"Server.Internal.UserManager", "UserManagerService")

	// Invalidate the ttl when a username is changed.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}

				// Skip our own messages
				id, pres := event.GetInt64("id")
				if !pres || id == result.id {
					continue
				}

				username, pres := event.GetString("username")
				if pres {
					result.mu.Lock()
					result.lru.Remove(username)
					result.mu.Unlock()
				}
			}
		}
	}()

	return result, nil
}
