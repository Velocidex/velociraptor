package users

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
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
	return nil, utils.NotFoundError
}

func (self *NullStorageManager) SetUser(ctx context.Context,
	user_record *api_proto.VelociraptorUser) error {
	return utils.NotImplementedError
}

func (self *NullStorageManager) ListAllUsers(
	ctx context.Context) ([]*api_proto.VelociraptorUser, error) {
	return nil, utils.NotImplementedError
}

func (self *NullStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	return nil, utils.NotImplementedError
}

func (self *NullStorageManager) SetUserOptions(ctx context.Context,
	username string, options *api_proto.SetGUIOptionsRequest) error {
	return utils.NotImplementedError
}

func (self *NullStorageManager) DeleteUser(ctx context.Context, username string) error {
	return utils.NotImplementedError
}

func (self *NullStorageManager) GetFavorites(
	ctx context.Context, org_config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	return nil, utils.NotImplementedError
}

/*
  The User Manager is responsible for coordinating access to user
  records.
*/

// The object that is cached in the LRU
type _CachedUserObject struct {
	user_record *api_proto.VelociraptorUser
	gui_options *api_proto.SetGUIOptionsRequest
	timestamp   time.Time
}

type UserStorageManager struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	// Sync the datastore records into memory - this is the source of
	// truth for all operations. We refresh it from the datastore
	// periodically and ensure writes are also sent to the datastore
	// immediately.
	// Key: ToLower(username), value: Cached User Record
	cache map[string]*_CachedUserObject

	id int64

	validator Validator
}

func (self *UserStorageManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if username == "" {
		return nil, errors.New("Must set a username")
	}

	// Check the LRU for a cache if it is there
	lower_user_name := utils.ToLower(username)
	cache, pres := self.cache[lower_user_name]
	if pres && cache.user_record != nil {
		// Return a copy to protect our version.
		return proto.Clone(cache.user_record).(*api_proto.VelociraptorUser), nil
	}

	return nil, fmt.Errorf("%w: %v", services.UserNotFoundError, username)
}

// Update the record in the LRU
func (self *UserStorageManager) SetUser(
	ctx context.Context, user_record *api_proto.VelociraptorUser) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if user_record.Name == "" {
		return errors.New("Must set a username")
	}

	err := ValidateUsername(self.config_obj, user_record.Name)
	if err != nil {
		return err
	}

	var cache *_CachedUserObject

	// Is there an existing cache?
	lower_user_name := utils.ToLower(user_record.Name)
	cache, pres := self.cache[lower_user_name]
	if !pres {
		cache = &_CachedUserObject{
			timestamp: utils.GetTime().Now(),
		}
	}

	// Cache a copy of the new record in memory.
	cache.user_record = proto.Clone(user_record).(*api_proto.VelociraptorUser)

	// Remove the org list because that will be built at runtime so it
	// does not need to be stored.
	cache.user_record.Orgs = nil

	cache.timestamp = utils.GetTime().Now()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// Update the user record in the datastore but use the original
	// user name. This is compatible with the previous behavior.
	err = db.SetSubject(self.config_obj,
		paths.UserPathManager{Name: user_record.Name}.Path(),
		cache.user_record)
	if err != nil {
		return err
	}

	self.cache[lower_user_name] = cache
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

// Update fixed fields in the options to override user choices. This
// ensures we have known fields.
func setDefaultGUIOptions(
	options *api_proto.SetGUIOptionsRequest,
	config_obj *config_proto.Config) {

	if options.Options == "" {
		// If the record is not found we need to create one from scratch.
		options.Options = default_user_options
	}

	// options.Links can not be set by the user it must be derived
	// from the config file and the default links. So we force them
	// each time.

	// Add any links in the config file to the user's preferences.
	if config_obj.GUI != nil {
		options.Links = MergeGUILinks(options.Links, config_obj.GUI.Links)
	}

	// Add the defaults.

	// NOTE: It is possible for a user to disable one of the default
	// targets by simply adding an entry with disabled: true - we will
	// not override the configured link from the default and it will
	// be ignored.
	options.Links = MergeGUILinks(options.Links, DefaultLinks)

	// Force the below settings from the config file. They can not be
	// overridden by a user. This is just a mechanism to communicate
	// the defaults to the GUI.
	defaults := &config_proto.Defaults{}
	if config_obj.Defaults != nil {
		defaults = config_obj.Defaults
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
	options.Customizations.IndexedClientMetadata = defaults.IndexedClientMetadata

	// Specify a default theme if specified in the config file.
	if options.Theme == "" {
		options.Theme = defaults.DefaultTheme
	}

	// Default theme if not set is veloci-light
	if options.Theme == "" {
		options.Theme = "veloci-light"
	}
}

func (self *UserStorageManager) loadUserRecrodIntoCache(
	ctx context.Context, username string) (*_CachedUserObject, error) {
	path_manager := paths.UserPathManager{Name: username}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	user_record := &api_proto.VelociraptorUser{}
	err = db.GetSubject(self.config_obj, path_manager.Path(), user_record)
	if err != nil {
		return nil, err
	}

	options := &api_proto.SetGUIOptionsRequest{}
	err = db.GetSubject(self.config_obj, path_manager.GUIOptions(), options)
	if err != nil {
		options = &api_proto.SetGUIOptionsRequest{}
	}

	setDefaultGUIOptions(options, self.config_obj)

	return &_CachedUserObject{
		user_record: user_record,
		gui_options: options,
		timestamp:   utils.GetTime().Now(),
	}, nil
}

// Build an in memory cache of all usernames and their lower cases so
// we can compare quickly.
func (self *UserStorageManager) buildCache(ctx context.Context) error {

	cache := make(map[string]*_CachedUserObject)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		// Not an error - without a datastore we dont have any users.
		return nil
	}

	children, err := db.ListChildren(self.config_obj, paths.USERS_ROOT)
	if err != nil {
		return err
	}

	for _, child := range children {
		if child.IsDir() {
			continue
		}

		username := child.Base()
		lower_user_name := utils.ToLower(username)
		cache_obj, err := self.loadUserRecrodIntoCache(ctx, username)
		if err == nil {

			// Detect User records files with multiple casing - we
			// reject one to avoid User record confusion. This should
			// not occur in normal operation!
			old_record, pres := cache[lower_user_name]
			if pres && old_record.user_record.Name != username {
				logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
				logger.Error("<red>UserManager</>: Multiple casing detected for User %v, will use record for %v.",
					username, old_record.user_record.Name)
				continue
			}

			cache[lower_user_name] = cache_obj
		}
	}

	// Swap the new cache quickly
	self.mu.Lock()
	self.cache = cache
	self.mu.Unlock()

	return nil
}

func (self *UserStorageManager) ListAllUsers(
	ctx context.Context) ([]*api_proto.VelociraptorUser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]*api_proto.VelociraptorUser, 0, len(self.cache))
	for _, cache := range self.cache {
		user_record := proto.Clone(cache.user_record).(*api_proto.VelociraptorUser)
		result = append(result, user_record)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (self *UserStorageManager) SetUserOptions(ctx context.Context,
	username string, options *api_proto.SetGUIOptionsRequest) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	lower_user_name := utils.ToLower(username)
	cache, pres := self.cache[lower_user_name]
	if !pres {
		// User not known - it is a new user
		cache = &_CachedUserObject{
			user_record: &api_proto.VelociraptorUser{
				Name: username,
			},
			gui_options: &api_proto.SetGUIOptionsRequest{},
		}
	}

	// Merge the old options with the new options
	old_options := cache.gui_options
	if old_options == nil {
		old_options = &api_proto.SetGUIOptionsRequest{}
	}

	setDefaultGUIOptions(old_options, self.config_obj)

	// For now we do not allow the user to set the links in their
	// profile.
	// old_options.Links = options.Links

	if options.Lang != "" {
		lang, err := self.validator.validateLang(options.Lang)
		if err != nil {
			return err
		}
		old_options.Lang = lang
	}

	if options.Theme != "" {
		theme, err := self.validator.validateTheme(options.Theme)
		if err != nil {
			return err
		}
		old_options.Theme = theme
	}

	if options.Timezone != "" {
		tz, err := self.validator.validateTimezone(options.Timezone)
		if err != nil {
			return err
		}
		old_options.Timezone = tz
	}

	if options.Org != "" {
		org, err := self.validator.validateOrg(options.Org)
		if err != nil {
			return err
		}
		old_options.Org = org
	}

	if len(options.Links) > 0 {
		links, err := self.validator.validateLinks(self.config_obj, options.Links)
		if err != nil {
			return err
		}
		old_options.Links = links
	}

	if options.Options != "" {
		old_options.Options = options.Options
	}

	// We need to distinguish between the case where the password is
	// reset to the empty string and the password is simply not
	// updated at all. In both cases the password will be an empty
	// string. Therefore in the JS code we force the password of "-"
	// to mean reset the password to empty string. If the field is
	// empty we do not update the password at all.
	if options.DefaultPassword != "" {
		// Means to reset the password.
		if options.DefaultPassword == "-" {
			old_options.DefaultPassword = ""
		} else {
			// Set the password to something.
			old_options.DefaultPassword = options.DefaultPassword
		}
	}
	old_options.DefaultDownloadsLock = options.DefaultDownloadsLock

	// Update the cache and write to disk.
	cache.gui_options = old_options
	cache.timestamp = utils.GetTime().Now()

	self.cache[lower_user_name] = cache

	// Store the user records with the original casing - this is
	// compatible with the old behavior.
	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(self.config_obj, path_manager.GUIOptions(), old_options)
	if err != nil {
		return err
	}

	return self.notifyChanges(ctx, lower_user_name)
}

func (self *UserStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	lower_user_name := utils.ToLower(username)

	cache, pres := self.cache[lower_user_name]
	if !pres {
		return nil, fmt.Errorf("%w: %v", services.UserNotFoundError, username)
	}

	if cache.gui_options == nil {
		cache.gui_options = &api_proto.SetGUIOptionsRequest{}
	}

	// Enforce the fixed fields
	setDefaultGUIOptions(cache.gui_options, self.config_obj)

	// Return a copy of the options to preserve the integrity of the cache
	return proto.Clone(cache.gui_options).(*api_proto.SetGUIOptionsRequest), nil
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

	lower_user_name := utils.ToLower(username)
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

	delete(self.cache, lower_user_name)
	return self.notifyChanges(ctx, username)
}

func NewUserStorageManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*UserStorageManager, error) {
	result := &UserStorageManager{
		config_obj: config_obj,
		cache:      make(map[string]*_CachedUserObject),
		id:         utils.GetGUID(),
	}

	// Get initial mapping between lower case usernames and correct usernames
	err := result.buildCache(ctx)
	if err != nil {
		return nil, err
	}

	journal_service, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}
	events, cancel := journal_service.Watch(ctx,
		"Server.Internal.UserManager", "UserManagerService")

	refresh_duration := time.Duration(300 * time.Second)
	if config_obj.Defaults != nil &&
		config_obj.Defaults.AclLruTimeoutSec > 0 {
		refresh_duration = time.Duration(
			config_obj.Defaults.AclLruTimeoutSec) * time.Second
	}

	// Invalidate the ttl when a username is changed.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(utils.Jitter(refresh_duration)):
				err := result.buildCache(ctx)
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("<red>UserManager</>: buildCache %v", err)
				}

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
					lower_user_name := utils.ToLower(username)

					cache_obj, err := result.loadUserRecrodIntoCache(ctx, username)
					if err == nil {
						result.mu.Lock()
						result.cache[lower_user_name] = cache_obj
						result.mu.Unlock()
					} else {
						result.mu.Lock()
						delete(result.cache, lower_user_name)
						result.mu.Unlock()
					}
				}
			}
		}
	}()

	return result, nil
}

func (self *UserManager) Storage() IUserStorageManager {
	return self.storage
}
