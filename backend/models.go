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
	ID               int64     `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email,omitempty"`
	DisplayName      string    `json:"display_name"`
	Bio              string    `json:"bio"`
	AvatarURL        string    `json:"avatar_url"`
	ProfileLink      string    `json:"profile_link"`
	ProfileLinkTitle string    `json:"profile_link_title"`
	IsAdmin          bool      `json:"is_admin"`
	CreatedAt        time.Time `json:"created_at"`
}

type Post struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Kind       string    `json:"kind"`
	Body       string    `json:"body"`
	RefType    *string   `json:"ref_type"`
	RefID      *int64    `json:"ref_id"`
	Visibility string    `json:"visibility"`
	CreatedAt  time.Time `json:"created_at"`
}

type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PasswordResetToken struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

type Invite struct {
	ID          int64      `json:"id"`
	Token       string     `json:"token"`
	CreatorID   int64      `json:"creator_id"`
	Used        bool       `json:"used"`
	UsedByUserID *int64    `json:"used_by_user_id"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type ctxKey string

const userCtxKey ctxKey = "current_user"

func scanUser(row interface{ Scan(...interface{}) error }) (*User, error) {
	var u User
	var bio, avatar, profileLink, profileLinkTitle sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &bio, &avatar, &u.IsAdmin, &profileLink, &profileLinkTitle, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.Bio = bio.String
	u.AvatarURL = avatar.String
	u.ProfileLink = profileLink.String
	u.ProfileLinkTitle = profileLinkTitle.String
	return &u, nil
}

const userCols = "id, username, email, display_name, bio, avatar_url, is_admin, profile_link, profile_link_title, created_at"

func publicUser(u *User) map[string]interface{} {
	if u == nil {
		return nil
	}
	return map[string]interface{}{
		"id":                 u.ID,
		"username":           u.Username,
		"display_name":       u.DisplayName,
		"bio":                u.Bio,
		"avatar_url":         u.AvatarURL,
		"profile_link":       u.ProfileLink,
		"profile_link_title": u.ProfileLinkTitle,
		"is_admin":           u.IsAdmin,
		"created_at":         u.CreatedAt,
	}
}
