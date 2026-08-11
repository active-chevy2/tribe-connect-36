package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	viewer := currentUserID(r)
	username := chi.URLParam(r, "username")
	row := s.db.QueryRow("SELECT "+userCols+" FROM users WHERE username=?", username)
	u, err := scanUser(row)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var posts, followers, following int
	s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id=?", u.ID).Scan(&posts)
	s.db.QueryRow("SELECT COUNT(*) FROM follows WHERE following_id=?", u.ID).Scan(&followers)
	s.db.QueryRow("SELECT COUNT(*) FROM follows WHERE follower_id=?", u.ID).Scan(&following)
	isFollowing := false
	if viewer != 0 && viewer != u.ID {
		var c int
		s.db.QueryRow("SELECT COUNT(*) FROM follows WHERE follower_id=? AND following_id=?", viewer, u.ID).Scan(&c)
		isFollowing = c > 0
	}
	profile := publicUser(u)
	profile["post_count"] = posts
	profile["follower_count"] = followers
	profile["following_count"] = following
	profile["is_following"] = isFollowing
	profile["is_self"] = viewer == u.ID
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleUserPosts(w http.ResponseWriter, r *http.Request) {
	viewer := currentUserID(r)
	username := chi.URLParam(r, "username")
	var uid int64
	if err := s.db.QueryRow("SELECT id FROM users WHERE username=?", username).Scan(&uid); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	limit := 20
	rows, err := s.db.Query("SELECT id FROM posts WHERE user_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		uid, limit, (page-1)*limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load posts")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		if p := s.postJSON(id, viewer, 2); p != nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFollow(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	targetID := urlParamInt(r, "id")
	if targetID == uid {
		writeError(w, http.StatusBadRequest, "you cannot follow yourself")
		return
	}
	var exists int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE id=?", targetID).Scan(&exists)
	if exists == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	s.db.Exec("INSERT IGNORE INTO follows (follower_id, following_id) VALUES (?,?)", uid, targetID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"following": true})
}

func (s *Server) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	targetID := urlParamInt(r, "id")
	s.db.Exec("DELETE FROM follows WHERE follower_id=? AND following_id=?", uid, targetID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"following": false})
}

type updateProfileReq struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var req updateProfileReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	display := trimTo(req.DisplayName, 100)
	if display == "" {
		display = u.DisplayName
	}
	s.db.Exec("UPDATE users SET display_name=?, bio=?, avatar_url=? WHERE id=?",
		display, trimTo(req.Bio, 2000), trimTo(req.AvatarURL, 500), u.ID)
	writeJSON(w, http.StatusOK, s.loadUser(u.ID))
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	viewer := currentUserID(r)
	rows, err := s.db.Query("SELECT " + userCols + " FROM users ORDER BY created_at DESC LIMIT 50")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load users")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			continue
		}
		p := publicUser(u)
		if viewer != 0 && viewer != u.ID {
			var c int
			s.db.QueryRow("SELECT COUNT(*) FROM follows WHERE follower_id=? AND following_id=?", viewer, u.ID).Scan(&c)
			p["is_following"] = c > 0
		} else {
			p["is_following"] = false
		}
		p["is_self"] = viewer == u.ID
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}
