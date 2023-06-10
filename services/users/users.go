/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/users"
)

const (
	// Default settings for reasonable GUI
	default_user_options = `{
  "selectionStyle":"line",
  "highlightActiveLine":true,
  "highlightSelectedWord":true,
  "copyWithEmptySelection":false,
  "cursorStyle":"ace",
  "mergeUndoDeltas":true,
  "behavioursEnabled":true,
  "wrapBehavioursEnabled":true,
  "showLineNumbers":true,
  "relativeLineNumbers":true,
  "hScrollBarAlwaysVisible":false,
  "vScrollBarAlwaysVisible":false,
  "highlightGutterLine":true,
  "animatedScroll":false,
  "showInvisibles":false,
  "showPrintMargin":true,
  "printMarginColumn":80,
  "printMargin":80,
  "fadeFoldWidgets":false,
  "showFoldWidgets":true,
  "displayIndentGuides":true,
  "showGutter":true,
  "fontSize":20,
  "fontFamily":"monospace",
  "scrollPastEnd":0,
  "theme":"ace/theme/xcode",
  "useTextareaForIME":true,
  "scrollSpeed":2,
  "dragDelay":0,
  "dragEnabled":true,
  "focusTimeout":0,
  "tooltipFollowsMouse":true,
  "firstLineNumber":1,
  "overwrite":false,
  "newLineMode":"auto",
  "useSoftTabs":true,
  "navigateWithinSoftTabs":false,
  "tabSize":4,
  "wrap":"free",
  "indentedSoftWrap":true,
  "foldStyle":"markbegin",
  "enableMultiselect":true,
  "enableBlockSelect":true,
  "enableEmmet":true,
  "enableBasicAutocompletion":true,
  "enableLiveAutocompletion":true}`
)

var (
	validUsernameRegEx = regexp.MustCompile("^[a-zA-Z0-9@.\\-_#+]+$")
)

type UserManager struct {
	ca_pool *x509.CertPool

	// This is the root org's config since there is only a single user
	// manager.
	config_obj *config_proto.Config
}

func validateUsername(config_obj *config_proto.Config, name string) error {
	if !validUsernameRegEx.MatchString(name) {
		return fmt.Errorf("Unacceptable username %v", name)
	}

	if config_obj.API != nil &&
		config_obj.API.PinnedGwName == name {
		return fmt.Errorf("Unacceptable username %v", name)
	}

	if config_obj.Client != nil &&
		config_obj.Client.PinnedServerName == name {
		return fmt.Errorf("Unacceptable username %v", name)
	}

	if name == "GRPC_GW" || name == "VelociraptorServer" {
		return fmt.Errorf("Unacceptable username %v", name)
	}

	return nil
}

func NewUserRecord(config_obj *config_proto.Config,
	name string) (*api_proto.VelociraptorUser, error) {
	err := validateUsername(config_obj, name)
	if err != nil {
		return nil, err
	}
	return &api_proto.VelociraptorUser{Name: name}, nil
}

func SetPassword(user_record *api_proto.VelociraptorUser, password string) {
	salt := make([]byte, 32)
	_, err := rand.Read(salt)
	if err != nil {
		return
	}
	hash := sha256.Sum256(append(salt, []byte(password)...))
	user_record.PasswordSalt = salt[:]
	user_record.PasswordHash = hash[:]
	user_record.Locked = false
}

func VerifyPassword(self *api_proto.VelociraptorUser, password string) bool {
	hash := sha256.Sum256(append(self.PasswordSalt, []byte(password)...))
	return subtle.ConstantTimeCompare(hash[:], self.PasswordHash) == 1
}

func (self UserManager) SetUser(
	ctx context.Context,
	user_record *api_proto.VelociraptorUser) error {
	if user_record.Name == "" {
		return errors.New("Must set a username")
	}

	err := validateUsername(self.config_obj, user_record.Name)
	if err != nil {
		return err
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(self.config_obj,
		paths.UserPathManager{Name: user_record.Name}.Path(),
		user_record)
}

func (self UserManager) ListUsers(
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
		user_record, err := self.GetUser(ctx, username)
		if err == nil {
			result = append(result, user_record)
		}
	}

	return result, nil
}

// Fill in the orgs the user has any permissions in.
func normalizeOrgList(
	ctx context.Context,
	user_record *api_proto.VelociraptorUser) {
	orgs := users.GetOrgs(ctx, user_record.Name)
	user_record.Orgs = nil

	// Fill in some information from the orgs but not everything.
	for _, org_record := range orgs {
		user_record.Orgs = append(user_record.Orgs, &api_proto.OrgRecord{
			Id:   org_record.Id,
			Name: org_record.Name,
		})
	}
}

// Returns the user record after stripping sensitive information like
// password hashes.
func (self UserManager) GetUser(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {

	// For the server name we dont have a real user record, we make a
	// hard coded user record instead.
	if username == self.config_obj.Client.PinnedServerName {
		return &api_proto.VelociraptorUser{
			Name: username,
		}, nil
	}

	result, err := self.GetUserWithHashes(ctx, username)
	if err != nil {
		return nil, err
	}

	// Do not divulge the password and hashes.
	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

// Return the user record with hashes - only used in Basic Auth.
func (self UserManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	if username == "" {
		return nil, errors.New("Must set a username")
	}

	err := validateUsername(self.config_obj, username)
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

	normalizeOrgList(ctx, user_record)
	return user_record, nil
}

func (self UserManager) SetUserOptions(ctx context.Context,
	username string,
	options *api_proto.SetGUIOptionsRequest) error {

	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// Merge the old options with the new options
	old_options, err := self.GetUserOptions(ctx, username)
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

	return db.SetSubject(self.config_obj, path_manager.GUIOptions(), old_options)
}

func (self UserManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {

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

	return options, nil
}

func StartUserManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> user manager service for org %v", config_obj.OrgId)

	CA_Pool := x509.NewCertPool()
	if config_obj.Client != nil {
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	}

	service := &UserManager{
		ca_pool:    CA_Pool,
		config_obj: config_obj,
	}
	services.RegisterUserManager(service)

	return nil
}

// Make sure there is always something available.
func init() {
	service := &UserManager{
		ca_pool:    x509.NewCertPool(),
		config_obj: &config_proto.Config{},
	}
	services.RegisterUserManager(service)
}
