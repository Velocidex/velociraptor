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
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
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

	SetUserStats(
		ctx context.Context,
		org_config_obj *config_proto.Config,
		username string,
		stats *api_proto.UserStats) error

	WriteUserMessage(ctx context.Context,
		username, sender string,
		message *ordereddict.Dict) error
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

func (self *NullStorageManager) SetUserStats(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	username string,
	stats *api_proto.UserStats) error {
	return utils.NotImplementedError
}

func (self *NullStorageManager) WriteUserMessage(
	ctx context.Context, username, sender string,
	message *ordereddict.Dict) error {
	return utils.NotImplementedError
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

func (self *UserStorageManager) SetUserStats(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	username string,
	stats *api_proto.UserStats) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	if username == "" {
		return errors.New("Must set a username")
	}

	// Check the LRU for a cache if it is there
	key := makeKey(username)
	cache, pres := self.cache[key]
	if !pres || cache.user_record == nil {
		return utils.NotFoundError
	}

	cache.user_record.Stats = stats
	old_timestamp := cache.timestamp
	now := utils.GetTime().Now()

	if now.Sub(old_timestamp) > 60*time.Minute {
		cache.timestamp = now

		db, err := datastore.GetDB(org_config_obj)
		if err != nil {
			return err
		}

		// Update the user record in the datastore but use the original
		// user name. This is compatible with the previous behavior.
		err = db.SetSubject(org_config_obj,
			paths.UserPathManager{Name: cache.user_record.Name}.Path(),
			cache.user_record)
		if err != nil {
			return err
		}

		self.cache[key] = cache
	}

	return nil
}

func (self *UserStorageManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if username == "" {
		return nil, errors.New("Must set a username")
	}

	// Check the LRU for a cache if it is there
	key := makeKey(username)
	cache, pres := self.cache[key]
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
	key := makeKey(user_record.Name)
	cache, pres := self.cache[key]
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

	self.cache[key] = cache
	return self.sendMutation(ctx, UserMutation{
		Op:       "Update",
		Username: user_record.Name,
	})
}

// Advertise the changes. This will force all minions to flush their
// caches.
func (self *UserStorageManager) sendMutation(
	ctx context.Context, mutation UserMutation) error {
	journal_service, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	return journal_service.PushRowsToArtifact(ctx, self.config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("id", self.id).
				Set("op", mutation.Op).
				Set("message", mutation.Message).
				Set("username", mutation.Username).
				Set("timestamp", mutation.Timestamp).
				Set("sender", mutation.From),
		},
		artifacts.USER_MANAGER)
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

	file_store_factory := file_store.GetFileStore(self.config_obj)
	reader, err := result_sets.NewResultSetReader(file_store_factory,
		path_manager.Notifications())
	if err == nil {
		options.Messages = reader.TotalRows()
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
func (self *UserStorageManager) BuildCache(ctx context.Context) error {

	// Build the new cache without a lock, and then swap it quickly
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
		key := makeKey(username)
		cache_obj, err := self.loadUserRecrodIntoCache(ctx, username)
		if err == nil {

			// Detect User records files with multiple casing - we
			// reject one to avoid User record confusion. This should
			// not occur in normal operation!
			old_record, pres := cache[key]
			if pres && old_record.user_record.Name != username {
				logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
				logger.Error("<red>UserManager</>: Multiple casing detected for User %v, will use record for %v.",
					username, old_record.user_record.Name)
				continue
			}

			cache[key] = cache_obj
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

	key := makeKey(username)
	cache, pres := self.cache[key]
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

	path_manager := paths.NewUserPathManager(username)
	// Means to clear the messages
	if options.Messages < 0 {
		file_store_factory := file_store.GetFileStore(self.config_obj)

		rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
			path_manager.Notifications(), json.DefaultEncOpts(),
			utils.SyncCompleter, result_sets.TruncateMode)
		if err == nil {
			// Just close it - we rely on truncate mode to remove all
			// rows.
			rs_writer.Close()
		}
		old_options.Messages = 0
	}

	// Update the cache and write to disk.
	cache.gui_options = old_options
	cache.timestamp = utils.GetTime().Now()

	self.cache[key] = cache

	// Store the user records with the original casing - this is
	// compatible with the old behavior.

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(self.config_obj, path_manager.GUIOptions(), old_options)
	if err != nil {
		return err
	}

	return self.sendMutation(ctx, UserMutation{
		Op:       "Update",
		Username: key,
	})
}

func (self *UserStorageManager) ClearUserMessages() {

}

func (self *UserStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := makeKey(username)

	cache, pres := self.cache[key]
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

	key := makeKey(username)
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

	delete(self.cache, key)
	return self.sendMutation(ctx, UserMutation{
		Op:       "Update",
		Username: username,
	})
}

func (self *UserStorageManager) WriteUserMessage(
	ctx context.Context, username, sender string,
	message *ordereddict.Dict) error {

	serialized, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return self.sendMutation(ctx, UserMutation{
		Op:        "Message",
		Username:  username,
		Message:   string(serialized),
		Timestamp: utils.GetTime().Now().Unix(),
		From:      sender,
	})
}

// Internal message queue uses user mutations
type UserMutation struct {
	Id        int64  `json:"id"`
	Op        string `json:"op"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	From      string `json:"sender"`
}

func (self *UserStorageManager) handleMessageEvent(
	ctx context.Context,
	config_obj *config_proto.Config,
	event *ordereddict.Dict) error {

	op, _ := event.GetString("op")
	switch op {

	// Update the user record from disk.
	case "Update":
		// Skip our own messages since we already have the freshest
		// version
		id, _ := event.GetInt64("id")
		if id == self.id {
			return nil
		}

		username, _ := event.GetString("username")
		key := makeKey(username)

		cache_obj, err := self.loadUserRecrodIntoCache(ctx, username)
		if err == nil {
			self.mu.Lock()
			self.cache[key] = cache_obj
			self.mu.Unlock()

			// Delete the user account if we cant load it from disk.
		} else {
			self.mu.Lock()
			delete(self.cache, key)
			self.mu.Unlock()
		}
		return nil

	case "Message":
		username, _ := event.GetString("username")
		message_str, _ := event.GetString("message")
		timestamp, _ := event.GetInt64("timestamp")
		sender, _ := event.GetString("sender")

		message := ordereddict.NewDict()
		err := message.UnmarshalJSON([]byte(message_str))
		if err != nil {
			return err
		}

		path_manager := paths.NewUserPathManager(username)
		journal, err := services.GetJournal(config_obj)
		if err != nil {
			return err
		}

		// Just update the cache - no need to flush it to disk because
		// Messages will be updated at start up from disk already in
		// loadUserRecrodIntoCache().
		self.mu.Lock()
		key := makeKey(username)
		cache, pres := self.cache[key]
		if !pres || cache.user_record == nil {
			return utils.NotFoundError
		}
		if cache.gui_options == nil {
			cache.gui_options = &api_proto.SetGUIOptionsRequest{}
		}
		cache.gui_options.Messages++
		self.mu.Unlock()

		return journal.AppendToResultSet(self.config_obj,
			path_manager.Notifications(),
			[]*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Timestamp", time.Unix(timestamp, 0)).
					Set("From", sender).
					Set("Message", message),
			},
			artifacts.USER_MANAGER)

	default:
		return fmt.Errorf("Unhandled message on UserManager queue: %v", event)
	}
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
	err := result.BuildCache(ctx)
	if err != nil {
		return nil, err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		artifacts.USER_MANAGER, "UserManagerService", result.handleMessageEvent)
	if err != nil {
		return nil, err
	}

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

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(utils.Jitter(refresh_duration)):
				err := result.BuildCache(ctx)
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("<red>UserManager</>: BuildCache %v", err)
				}
			}
		}
	}()

	return result, nil
}

func (self *UserManager) Storage() IUserStorageManager {
	return self.storage
}

func NewNullStorageManager() *UserManager {
	return &UserManager{
		config_obj: &config_proto.Config{},
		storage:    &NullStorageManager{},
	}
}

// Normalize the username to the cache key
func makeKey(username string) string {
	return utils.ToLower(username)
}
