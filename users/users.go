/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"path"
	"regexp"
	"strings"

	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

const (
	// Default settings for reasonable GUI
	default_user_options = `{"selectionStyle":"line","highlightActiveLine":true,"highlightSelectedWord":true,"copyWithEmptySelection":false,"cursorStyle":"ace","mergeUndoDeltas":true,"behavioursEnabled":true,"wrapBehavioursEnabled":true,"showLineNumbers":true,"relativeLineNumbers":true,"hScrollBarAlwaysVisible":false,"vScrollBarAlwaysVisible":false,"highlightGutterLine":true,"animatedScroll":false,"showInvisibles":false,"showPrintMargin":true,"printMarginColumn":80,"printMargin":80,"fadeFoldWidgets":false,"showFoldWidgets":true,"displayIndentGuides":true,"showGutter":true,"fontSize":12,"fontFamily":"monospace","scrollPastEnd":0,"theme":"ace/theme/xcode","useTextareaForIME":true,"scrollSpeed":2,"dragDelay":0,"dragEnabled":true,"focusTimeout":0,"tooltipFollowsMouse":true,"firstLineNumber":1,"overwrite":false,"newLineMode":"auto","useSoftTabs":true,"navigateWithinSoftTabs":false,"tabSize":4,"wrap":"free","indentedSoftWrap":true,"foldStyle":"markbegin","enableMultiselect":true,"enableBlockSelect":true,"enableEmmet":true,"enableBasicAutocompletion":true,"enableLiveAutocompletion":true}`
)

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

func SetUser(config_obj *config_proto.Config, user_record *api_proto.VelociraptorUser) error {
	if user_record.Name == "" {
		return errors.New("Must set a username")
	}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(config_obj,
		paths.UserPathManager{Name: user_record.Name}.Path(),
		user_record)
}

func ListUsers(config_obj *config_proto.Config) ([]*api_proto.VelociraptorUser, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	children, err := db.ListChildren(config_obj, constants.USER_URN, 0, 500)
	if err != nil {
		return nil, err
	}

	result := make([]*api_proto.VelociraptorUser, 0, len(children))
	for _, child := range children {
		username := path.Base(child)
		user_record, err := GetUser(config_obj, username)
		if err == nil {
			result = append(result, user_record)
		}
	}

	return result, nil
}

// Returns the user record after stripping sensitive information like
// password hashes.
func GetUser(config_obj *config_proto.Config, username string) (
	*api_proto.VelociraptorUser, error) {
	result, err := GetUserWithHashes(config_obj, username)
	if err != nil {
		return nil, err
	}

	// Do not divulge the password and hashes.
	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

// Return the user record with hashes - only used in Basic Auth.
func GetUserWithHashes(config_obj *config_proto.Config, username string) (
	*api_proto.VelociraptorUser, error) {
	if username == "" {
		return nil, errors.New("Must set a username")
	}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	user_record := &api_proto.VelociraptorUser{}
	err = db.GetSubject(config_obj,
		paths.UserPathManager{Name: username}.Path(), user_record)

	if user_record.Name == "" {
		return nil, errors.New("User not found")
	}

	return user_record, err
}

func GetUserNotificationCount(config_obj *config_proto.Config, username string) (uint64, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return 0, err
	}

	result, err := db.ListChildren(config_obj,
		constants.USER_URN+username+"/notifications/pending", 0, 50)
	if err != nil {
		return 0, nil
	}

	return uint64(len(result)), nil
}

func GetUserNotifications(config_obj *config_proto.Config, username string, clear_pending bool) (
	*api_proto.GetUserNotificationsResponse, error) {
	result := &api_proto.GetUserNotificationsResponse{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// First read the pending notifications.
	urn := fmt.Sprintf("%s/%s/notifications/pending",
		constants.USER_URN, username)
	notification_urns, err := db.ListChildren(
		config_obj, urn, 0, 50)
	if err != nil {
		return nil, err
	}

	to_clear := make(map[string]*api_proto.UserNotification)
	for _, notification_urn := range notification_urns {
		item := &api_proto.UserNotification{}
		err = db.GetSubject(config_obj, notification_urn, item)
		if err != nil {
			continue
		}

		if clear_pending {
			to_clear[notification_urn] = item
		}

		result.Items = append(result.Items, item)
	}

	// Now get some already read notifications.
	if len(result.Items) < 50 {
		read_urn := fmt.Sprintf("%s/%s/notifications/read",
			constants.USER_URN, username)

		read_urns, _ := db.ListChildren(
			config_obj, read_urn, 0, uint64(50-len(result.Items)))
		for _, read_notification_urn := range read_urns {
			item := &api_proto.UserNotification{}
			err = db.GetSubject(config_obj, read_notification_urn, item)
			if err != nil {
				continue
			}
			result.Items = append(result.Items, item)
		}
	}

	if len(to_clear) > 0 {
		for urn, item := range to_clear {
			err = db.DeleteSubject(config_obj, urn)
			if err != nil {
				return nil, err
			}
			new_urn := strings.Replace(urn, "pending", "read", -1)
			item.State = api_proto.UserNotification_STATE_NOT_PENDING
			err = db.SetSubject(config_obj, new_urn, item)
			if err != nil {
				return nil, err
			}
		}

	}

	return result, nil
}

func Notify(config_obj *config_proto.Config, notification *api_proto.UserNotification) error {
	mu.Lock()
	defer mu.Unlock()

	if gUserNotificationManager == nil {
		return errors.New("Uninitialized UserNotificationManager")
	}
	gUserNotificationManager.Notify(notification)
	return nil
}

func SetUserOptions(config_obj *config_proto.Config,
	username string,
	options *api_proto.SetGUIOptionsRequest) error {

	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	// Merge the old options with the new options
	old_options, err := GetUserOptions(config_obj, username)
	if err != nil {
		old_options = &api_proto.SetGUIOptionsRequest{}
	}

	if options.Theme != "" {
		old_options.Theme = options.Theme
	}

	if options.Options != "" {
		old_options.Options = options.Options
	}

	return db.SetSubject(config_obj, path_manager.GUIOptions(), old_options)
}

func GetUserOptions(config_obj *config_proto.Config, username string) (
	*api_proto.SetGUIOptionsRequest, error) {

	path_manager := paths.UserPathManager{Name: username}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	options := &api_proto.SetGUIOptionsRequest{}
	err = db.GetSubject(config_obj, path_manager.GUIOptions(), options)
	if options.Options == "" {
		options.Options = default_user_options
	}
	return options, err
}
