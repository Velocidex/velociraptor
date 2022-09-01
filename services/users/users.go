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
	"fmt"
	"os"
	"regexp"
	"sync"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
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
  "fontSize":14,
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

type UserManager struct {
	ca_pool *x509.CertPool

	// This is the root org's config since there is only a single user
	// manager.
	config_obj *config_proto.Config
}

func NewUserRecord(name string) (*api_proto.VelociraptorUser, error) {
	if !regexp.MustCompile("^[a-zA-Z0-9@.\\-_#]+$").MatchString(name) {
		return nil, errors.New(fmt.Sprintf(
			"Unacceptable username %v", name))
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

func (self UserManager) SetUser(user_record *api_proto.VelociraptorUser) error {
	if user_record.Name == "" {
		return errors.New("Must set a username")
	}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(self.config_obj,
		paths.UserPathManager{Name: user_record.Name}.Path(),
		user_record)
}

func (self UserManager) ListUsers() ([]*api_proto.VelociraptorUser, error) {
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
		user_record, err := self.GetUser(username)
		if err == nil {
			result = append(result, user_record)
		}
	}

	return result, nil
}

// Fill in the orgs the user has any permissions in.
func normalizeOrgList(user_record *api_proto.VelociraptorUser) error {
	user_record.Orgs = nil

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	// Fill in the org names if needed
	for _, org_record := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org_record.OrgId)
		if err != nil {
			continue
		}

		ok, _ := acls.CheckAccess(org_config_obj, user_record.Name, acls.READ_RESULTS)
		if !ok {
			continue
		}

		user_record.Orgs = append(user_record.Orgs, &api_proto.Org{
			Id:   org_record.OrgId,
			Name: org_record.Name,
		})
	}

	return nil
}

// Returns the user record after stripping sensitive information like
// password hashes.
func (self UserManager) GetUser(username string) (
	*api_proto.VelociraptorUser, error) {

	// For the server name we dont have a real user record, we make a
	// hard coded user record instead.
	if username == self.config_obj.Client.PinnedServerName {
		return &api_proto.VelociraptorUser{
			Name: username,
		}, nil
	}

	result, err := self.GetUserWithHashes(username)
	if err != nil {
		return nil, err
	}

	// Do not divulge the password and hashes.
	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

// Return the user record with hashes - only used in Basic Auth.
func (self UserManager) GetUserWithHashes(username string) (
	*api_proto.VelociraptorUser, error) {
	if username == "" {
		return nil, errors.New("Must set a username")
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

	err = normalizeOrgList(user_record)
	return user_record, err
}

func (self UserManager) SetUserOptions(username string,
	options *api_proto.SetGUIOptionsRequest) error {

	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// Merge the old options with the new options
	old_options, err := self.GetUserOptions(username)
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

func (self UserManager) GetUserOptions(username string) (
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

	return options, err
}

func StartUserManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

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
		ca_pool: x509.NewCertPool(),
	}
	services.RegisterUserManager(service)
}
