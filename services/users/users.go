/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2024 Rapid7 Inc.

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
	"crypto/x509"
	"fmt"
	"regexp"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
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

	storage IUserStorageManager
}

func validateUsername(config_obj *config_proto.Config, name string) error {
	if !validUsernameRegEx.MatchString(name) {
		return fmt.Errorf("Unacceptable username %v", name)
	}

	if config_obj.API != nil &&
		config_obj.API.PinnedGwName == name {
		return fmt.Errorf("Username is reserved: %v", name)
	}

	if config_obj.Client != nil &&
		config_obj.Client.PinnedServerName == name {
		return fmt.Errorf("Username is reserved: %v", name)
	}

	if name == constants.PinnedGwName || name == constants.PinnedServerName {
		return fmt.Errorf("Username is reserved: %v", name)
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

func (self UserManager) SetUser(
	ctx context.Context,
	user_record *api_proto.VelociraptorUser) error {
	return self.storage.SetUser(ctx, user_record)
}

func (self UserManager) SetUserOptions(ctx context.Context,
	username string,
	options *api_proto.SetGUIOptionsRequest) error {
	return self.storage.SetUserOptions(ctx, username, options)
}

func (self UserManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	return self.storage.GetUserOptions(ctx, username)
}

func NewUserManager(
	config_obj *config_proto.Config,
	storage IUserStorageManager) *UserManager {
	CA_Pool := x509.NewCertPool()
	if config_obj.Client != nil {
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	}

	return &UserManager{
		ca_pool:    CA_Pool,
		config_obj: config_obj,
		storage:    storage,
	}
}

func StartUserManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> user manager service for org %v", config_obj.OrgId)

	storage, err := NewUserStorageManager(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	service := NewUserManager(config_obj, storage)
	services.RegisterUserManager(service)

	return nil
}

// Make sure there is always something available.
func init() {
	service := NewUserManager(&config_proto.Config{}, &NullStorageManager{})
	services.RegisterUserManager(service)
}
