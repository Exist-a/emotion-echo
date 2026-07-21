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
	phone := "13800138000"
	email := "u@x.com"
	pw := "hashed"
	nick := "nick"
	avatar := "https://x/a.png"
	bday := now.AddDate(-30, 0, 0)

	u := User{
		ID:           100,
		Username:     "u100",
		Phone:        &phone,
		Email:        &email,
		PasswordHash: &pw,
		Nickname:     &nick,
		AvatarURL:    &avatar,
		Gender:       1,
		Birthday:     &bday,
		Status:       1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if u.ID != 100 || u.Username != "u100" {
		t.Fatalf("field mismatch: %+v", u)
	}
	if *u.Phone != "13800138000" {
		t.Fatalf("phone mismatch: %q", *u.Phone)
	}
	if *u.Email != "u@x.com" {
		t.Fatalf("email mismatch")
	}
	if u.Gender != 1 || u.Status != 1 {
		t.Fatalf("enums mismatch")
	}
}

// TestUser_NullableFields 表驱动：指针字段可空
func TestUser_NullableFields(t *testing.T) {
	u := User{}
	if u.Phone != nil || u.Email != nil {
		t.Fatalf("nullable fields should default to nil")
	}
	if u.Birthday != nil || u.DeletedAt != nil {
		t.Fatalf("nullable fields should default to nil")
	}
}

// TestUser_DeletedAt_SoftDelete 测试软删除字段
func TestUser_DeletedAt_SoftDelete(t *testing.T) {
	u := User{}
	if u.DeletedAt != nil {
		t.Fatalf("default DeletedAt should be nil")
	}
	now := time.Now()
	u.DeletedAt = &now
	if u.DeletedAt == nil {
		t.Fatalf("set DeletedAt failed")
	}
	if !u.DeletedAt.Equal(now) {
		t.Fatalf("DeletedAt not equal")
	}
}
