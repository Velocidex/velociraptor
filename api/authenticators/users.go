package authenticators

import (
	"context"

	"www.velocidex.com/golang/velociraptor/services"
)

// Try to store the picture URL in the datastore to avoid making the
// cookie too large. Not critical if it fails, just move on.
func setUserPicture(ctx context.Context, username, url string) {
	user_manager := services.GetUserManager()
	user_record, err := user_manager.GetUserWithHashes(ctx, username, username)
	if err == nil && user_record.Picture != url {
		user_record.Picture = url
		_ = user_manager.SetUser(ctx, user_record)
	}
}
