package authenticators

import (
	"context"
	"fmt"

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

func checkUserOID(
	ctx context.Context,
	oid, username string) error {

	user_manager := services.GetUserManager()
	user_record, err := user_manager.GetUserWithHashes(
		ctx, username, username)
	if err != nil {
		return err
	}

	// Set the oid for next time.
	if user_record.Oid == "" {
		user_record.Oid = oid
		_ = user_manager.SetUser(ctx, user_record)
	} else if user_record.Oid != oid {
		return fmt.Errorf(
			"Unexpected user ID %v for user %v, should be %v",
			oid, username, user_record.Oid)
	}
	return nil
}
