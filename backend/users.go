package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

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
	s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id=? AND visibility='public'", u.ID).Scan(&posts)
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
	query := "SELECT id FROM posts WHERE user_id=? AND (visibility='public'"
	args := []interface{}{uid}
	if viewer != 0 && (viewer == uid || (currentUser(r) != nil && currentUser(r).IsAdmin)) {
		query += " OR visibility='private'"
	}
	query += ") ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, (page-1)*limit)

	rows, err := s.db.Query(query, args...)
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
	DisplayName      string `json:"display_name"`
	Bio              string `json:"bio"`
	AvatarURL        string `json:"avatar_url"`
	ProfileLink      string `json:"profile_link"`
	ProfileLinkTitle string `json:"profile_link_title"`
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
	s.db.Exec("UPDATE users SET display_name=?, bio=?, avatar_url=?, profile_link=?, profile_link_title=? WHERE id=?",
		display, trimTo(req.Bio, 2000), trimTo(req.AvatarURL, 500), trimTo(req.ProfileLink, 255), trimTo(req.ProfileLinkTitle, 100), u.ID)
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

// ---- User RSS Feed ----
func (s *Server) handleUserFeed(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	var uid int64
	var displayName string
	var avatarURL string
	err := s.db.QueryRow("SELECT id, display_name, avatar_url FROM users WHERE username=?", username).Scan(&uid, &displayName, &avatarURL)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	rows, err := s.db.Query("SELECT id, body, created_at FROM posts WHERE user_id=? AND visibility='public' ORDER BY created_at DESC LIMIT 50", uid)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type item struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Guid        string `xml:"guid"`
		PubDate     string `xml:"pubDate"`
		Description string `xml:"description"`
	}
	items := []item{}
	for rows.Next() {
		var id int64
		var body string
		var created time.Time
		rows.Scan(&id, &body, &created)
		items = append(items, item{
			Title:       displayName + "'s post",
			Link:        s.cfg.PublicBaseURL + "/#/p/post/" + fmt.Sprintf("%d", id),
			Guid:        fmt.Sprintf("%d", id),
			PubDate:     created.Format(time.RFC1123Z),
			Description: body,
		})
	}

	feed := struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Items       []item `xml:"item"`
		} `xml:"channel"`
	}{
		Version: "2.0",
	}
	feed.Channel.Title = displayName + "'s feed"
	feed.Channel.Link = s.cfg.PublicBaseURL + "/#/profile/" + username
	feed.Channel.Description = "Public posts from " + displayName
	feed.Channel.Items = items

	w.Header().Set("Content-Type", "application/rss+xml")
	xml.NewEncoder(w).Encode(feed)
}
