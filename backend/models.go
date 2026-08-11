package main

import (
	"database/sql"
	"time"
)

type Server struct {
	db  *sql.DB
	cfg Config
}

type User struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	AvatarURL   string    `json:"avatar_url"`
	IsAdmin     bool      `json:"is_admin"`
	CreatedAt   time.Time `json:"created_at"`
}

type ctxKey string

const userCtxKey ctxKey = "current_user"

func scanUser(row interface{ Scan(...interface{}) error }) (*User, error) {
	var u User
	var bio, avatar sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &bio, &avatar, &u.IsAdmin, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.Bio = bio.String
	u.AvatarURL = avatar.String
	return &u, nil
}

const userCols = "id, username, email, display_name, bio, avatar_url, is_admin, created_at"

func publicUser(u *User) map[string]interface{} {
	if u == nil {
		return nil
	}
	return map[string]interface{}{
		"id":           u.ID,
		"username":     u.Username,
		"display_name": u.DisplayName,
		"bio":          u.Bio,
		"avatar_url":   u.AvatarURL,
		"is_admin":     u.IsAdmin,
		"created_at":   u.CreatedAt,
	}
}
