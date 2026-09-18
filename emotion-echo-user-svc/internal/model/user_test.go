package model

import (
	"testing"
	"time"
)

// TestUser_TableName 表名校验
func TestUser_TableName(t *testing.T) {
	u := User{}
	if got := u.TableName(); got != "emotion_echo_user.users" {
		t.Fatalf("want 'emotion_echo_user.users' got %q", got)
	}
}

// TestUser_Fields 表驱动
func TestUser_Fields(t *testing.T) {
	now := time.Now()
	pw := "hashed"
	nick := "nick"
	avatar := "https://x/a.png"
	bday := now.AddDate(-30, 0, 0)

	u := User{
		ID:           100,
		Username:     "u100",
		PasswordHash: &pw,
		Nickname:     &nick,
		AvatarURL:    &avatar,
		Gender:       1,
		Birthday:     &bday,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if u.ID != 100 || u.Username != "u100" {
		t.Fatalf("field mismatch: %+v", u)
	}
	if u.Gender != 1 {
		t.Fatalf("gender mismatch: got %d", u.Gender)
	}
}

// TestUser_NullableFields 表驱动：指针字段可空
func TestUser_NullableFields(t *testing.T) {
	u := User{}
	if u.Birthday != nil {
		t.Fatalf("nullable fields should default to nil")
	}
	if u.DeletedAt.Valid {
		t.Fatalf("DeletedAt should default to zero value (not valid)")
	}
}

// TestUser_DeletedAt_SoftDelete 测试软删除字段
func TestUser_DeletedAt_SoftDelete(t *testing.T) {
	u := User{}
	if u.DeletedAt.Valid {
		t.Fatalf("default DeletedAt should not be valid")
	}
	now := time.Now()
	u.DeletedAt.Time = now
	u.DeletedAt.Valid = true
	if !u.DeletedAt.Valid {
		t.Fatalf("set DeletedAt failed")
	}
	if !u.DeletedAt.Time.Equal(now) {
		t.Fatalf("DeletedAt time not equal")
	}
}
