package packaging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCustomServerUserGroup(t *testing.T) {
	t.Run("empty_user_and_group", func(t *testing.T) {
		user, group, err := ValidateCustomServerUserGroup("", "")
		assert.NoError(t, err)
		assert.Equal(t, "", user)
		assert.Equal(t, "", group)
	})

	t.Run("user_only_defaults_group", func(t *testing.T) {
		user, group, err := ValidateCustomServerUserGroup("myuser", "")
		assert.NoError(t, err)
		assert.Equal(t, "myuser", user)
		assert.Equal(t, "myuser", group)
	})

	t.Run("user_and_group", func(t *testing.T) {
		user, group, err := ValidateCustomServerUserGroup("myuser", "mygroup")
		assert.NoError(t, err)
		assert.Equal(t, "myuser", user)
		assert.Equal(t, "mygroup", group)
	})

	t.Run("group_without_user_errors", func(t *testing.T) {
		_, _, err := ValidateCustomServerUserGroup("", "mygroup")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--server_group requires --server_user")
	})
}