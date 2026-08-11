package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var validReactions = map[string]bool{
	"like": true, "love": true, "celebrate": true, "insightful": true, "laugh": true,
}

// ---- shared count helpers ----

func (s *Server) counts(targetType string, targetID int64) map[string]interface{} {
	var rc, cc, sc int
	s.db.QueryRow("SELECT COUNT(*) FROM reactions WHERE target_type=? AND target_id=?", targetType, targetID).Scan(&rc)
	s.db.QueryRow("SELECT COUNT(*) FROM comments WHERE target_type=? AND target_id=?", targetType, targetID).Scan(&cc)
	if targetType != "comment" {
		s.db.QueryRow("SELECT COUNT(*) FROM shares WHERE target_type=? AND target_id=?", targetType, targetID).Scan(&sc)
	}
	byType := map[string]int{}
	rows, err := s.db.Query("SELECT reaction, COUNT(*) FROM reactions WHERE target_type=? AND target_id=? GROUP BY reaction", targetType, targetID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rt string
			var n int
			rows.Scan(&rt, &n)
			byType[rt] = n
		}
	}
	return map[string]interface{}{
		"reactions":    rc,
		"comments":     cc,
		"shares":       sc,
		"reactions_by": byType,
	}
}

func (s *Server) myReaction(userID int64, targetType string, targetID int64) interface{} {
	if userID == 0 {
		return nil
	}
	var rt string
	if err := s.db.QueryRow("SELECT reaction FROM reactions WHERE user_id=? AND target_type=? AND target_id=?",
		userID, targetType, targetID).Scan(&rt); err != nil {
		return nil
	}
	return rt
}

// ---- posts ----

func (s *Server) postJSON(postID int64, userID int64, depth int) map[string]interface{} {
	var (
		id, uid           int64
		kind              string
		body              sql.NullString
		refType           sql.NullString
		refID             sql.NullInt64
		visibility        string
		createdAt         time.Time
		username, display sql.NullString
		avatar            sql.NullString
	)
	err := s.db.QueryRow(`SELECT p.id, p.user_id, p.kind, p.body, p.ref_type, p.ref_id, p.visibility, p.created_at,
		u.username, u.display_name, u.avatar_url
		FROM posts p JOIN users u ON u.id=p.user_id WHERE p.id=?`, postID).
		Scan(&id, &uid, &kind, &body, &refType, &refID, &visibility, &createdAt, &username, &display, &avatar)
	if err != nil {
		return nil
	}
	// Visibility check: if private, only author and admins can see
	if visibility == "private" {
		u := currentUserFromID(userID) // we need a helper
		if u == nil || (u.ID != uid && !u.IsAdmin) {
			return nil
		}
	}
	var ref interface{}
	if depth > 0 && refType.Valid && refID.Valid {
		switch refType.String {
		case "post":
			ref = s.postJSON(refID.Int64, userID, depth-1)
		case "feed_item":
			ref = s.itemJSON(refID.Int64, userID)
		}
	}
	return map[string]interface{}{
		"id":         id,
		"type":       "post",
		"kind":       kind,
		"body":       body.String,
		"visibility": visibility,
		"author": map[string]interface{}{
			"id":           uid,
			"username":     username.String,
			"display_name": display.String,
			"avatar_url":   avatar.String,
		},
		"ref":         ref,
		"created_at":  createdAt,
		"counts":      s.counts("post", id),
		"my_reaction": s.myReaction(userID, "post", id),
	}
}

// Helper to get user by ID
func currentUserFromID(id int64) *User {
	// We'll implement a quick load
	row := s.db.QueryRow("SELECT "+userCols+" FROM users WHERE id=?", id)
	u, _ := scanUser(row)
	return u
}

type createPostReq struct {
	Body       string `json:"body"`
	Visibility string `json:"visibility"` // optional, defaults to global default
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req createPostReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "post cannot be empty")
		return
	}
	if len(req.Body) > 5000 {
		writeError(w, http.StatusBadRequest, "post is too long (max 5000 chars)")
		return
	}
	// Visibility: if provided, use it; else default from settings
	vis := req.Visibility
	if vis == "" || (vis != "public" && vis != "private") {
		def, _ := s.getSetting("default_post_visibility")
		if def != "private" {
			def = "public"
		}
		vis = def
	}
	res, err := s.db.Exec("INSERT INTO posts (user_id, kind, body, visibility) VALUES (?, 'post', ?, ?)", uid, req.Body, vis)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create post")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, s.postJSON(id, uid, 2))
}

type refReq struct {
	RefType string `json:"ref_type"`
	RefID   int64  `json:"ref_id"`
	Body    string `json:"body"`
}

func (s *Server) refExists(refType string, refID int64) bool {
	var c int
	switch refType {
	case "post":
		s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE id=?", refID).Scan(&c)
	case "feed_item":
		s.db.QueryRow("SELECT COUNT(*) FROM feed_items WHERE id=?", refID).Scan(&c)
	}
	return c > 0
}

func (s *Server) handleRepost(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req refReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validTargetType(req.RefType, false) || !s.refExists(req.RefType, req.RefID) {
		writeError(w, http.StatusBadRequest, "invalid repost target")
		return
	}
	// Visibility: inherit from original post if reposting a post; for feed item, use default
	vis := "public"
	if req.RefType == "post" {
		var v string
		s.db.QueryRow("SELECT visibility FROM posts WHERE id=?", req.RefID).Scan(&v)
		if v == "private" {
			vis = "private"
		}
	}
	res, err := s.db.Exec("INSERT INTO posts (user_id, kind, ref_type, ref_id, visibility) VALUES (?, 'repost', ?, ?, ?)", uid, req.RefType, req.RefID, vis)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not repost")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, s.postJSON(id, uid, 2))
}

func (s *Server) handleQuote(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req refReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "quote needs a comment")
		return
	}
	if !validTargetType(req.RefType, false) || !s.refExists(req.RefType, req.RefID) {
		writeError(w, http.StatusBadRequest, "invalid quote target")
		return
	}
	vis := "public"
	if req.RefType == "post" {
		var v string
		s.db.QueryRow("SELECT visibility FROM posts WHERE id=?", req.RefID).Scan(&v)
		if v == "private" {
			vis = "private"
		}
	}
	res, err := s.db.Exec("INSERT INTO posts (user_id, kind, body, ref_type, ref_id, visibility) VALUES (?, 'quote', ?, ?, ?, ?)", uid, req.Body, req.RefType, req.RefID, vis)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not quote")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, s.postJSON(id, uid, 2))
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	p := s.postJSON(urlParamInt(r, "id"), uid, 2)
	if p == nil {
		writeError(w, http.StatusNotFound, "post not found or private")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	postID := urlParamInt(r, "id")
	var owner int64
	if err := s.db.QueryRow("SELECT user_id FROM posts WHERE id=?", postID).Scan(&owner); err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if owner != u.ID && !u.IsAdmin {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	s.db.Exec("DELETE FROM posts WHERE id=?", postID)
	s.db.Exec("DELETE FROM comments WHERE target_type='post' AND target_id=?", postID)
	s.db.Exec("DELETE FROM reactions WHERE target_type='post' AND target_id=?", postID)
	s.db.Exec("DELETE FROM shares WHERE target_type='post' AND target_id=?", postID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit
	filter := r.URL.Query().Get("filter")

	// Build query with visibility filter
	query := "SELECT id FROM posts"
	args := []interface{}{}
	where := []string{}
	if filter == "following" && uid != 0 {
		where = append(where, "(user_id IN (SELECT following_id FROM follows WHERE follower_id=?) OR user_id=?)")
		args = append(args, uid, uid)
	}
	// Always filter: show public posts + private posts owned by viewer (or admin)
	visCondition := "(visibility='public'"
	if uid != 0 {
		visCondition += " OR (visibility='private' AND user_id=?)"
		args = append(args, uid)
		// Admin sees all
		u := currentUser(r)
		if u != nil && u.IsAdmin {
			visCondition = "1=1" // admin sees all
		}
	}
	visCondition += ")"
	if visCondition != "1=1" {
		where = append(where, visCondition)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load timeline")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		if p := s.postJSON(id, uid, 2); p != nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- comments ----

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	targetType := r.URL.Query().Get("target_type")
	targetID := int64(queryInt(r, "target_id", 0))
	if !validTargetType(targetType, false) || targetID == 0 {
		writeError(w, http.StatusBadRequest, "target_type and target_id are required")
		return
	}
	rows, err := s.db.Query(`SELECT c.id, c.user_id, c.parent_id, c.body, c.created_at, u.username, u.display_name, u.avatar_url
		FROM comments c JOIN users u ON u.id=c.user_id
		WHERE c.target_type=? AND c.target_id=? ORDER BY c.created_at ASC`, targetType, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load comments")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var (
			id, cuid          int64
			parent            sql.NullInt64
			body              string
			createdAt         time.Time
			username, display sql.NullString
			avatar            sql.NullString
		)
		rows.Scan(&id, &cuid, &parent, &body, &createdAt, &username, &display, &avatar)
		var parentID interface{}
		if parent.Valid {
			parentID = parent.Int64
		}
		out = append(out, map[string]interface{}{
			"id":         id,
			"parent_id":  parentID,
			"body":       body,
			"created_at": createdAt,
			"author": map[string]interface{}{
				"id":           cuid,
				"username":     username.String,
				"display_name": display.String,
				"avatar_url":   avatar.String,
			},
			"counts":      s.counts("comment", id),
			"my_reaction": s.myReaction(uid, "comment", id),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type createCommentReq struct {
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	ParentID   *int64 `json:"parent_id"`
	Body       string `json:"body"`
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req createCommentReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "comment cannot be empty")
		return
	}
	if !validTargetType(req.TargetType, false) || !s.refExists(req.TargetType, req.TargetID) {
		writeError(w, http.StatusBadRequest, "invalid comment target")
		return
	}
	var parent interface{}
	if req.ParentID != nil {
		parent = *req.ParentID
	}
	res, err := s.db.Exec("INSERT INTO comments (user_id, target_type, target_id, parent_id, body) VALUES (?,?,?,?,?)",
		uid, req.TargetType, req.TargetID, parent, req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add comment")
		return
	}
	id, _ := res.LastInsertId()
	u := currentUser(r)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         id,
		"parent_id":  req.ParentID,
		"body":       req.Body,
		"created_at": time.Now().UTC(),
		"author": map[string]interface{}{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName, "avatar_url": u.AvatarURL,
		},
		"counts":      s.counts("comment", id),
		"my_reaction": nil,
	})
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id := urlParamInt(r, "id")
	var owner int64
	if err := s.db.QueryRow("SELECT user_id FROM comments WHERE id=?", id).Scan(&owner); err != nil {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}
	if owner != u.ID && !u.IsAdmin {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	s.db.Exec("DELETE FROM comments WHERE id=? OR parent_id=?", id, id)
	s.db.Exec("DELETE FROM reactions WHERE target_type='comment' AND target_id=?", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---- reactions ----

type reactionReq struct {
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	Reaction   string `json:"reaction"`
}

func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req reactionReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Reaction == "" {
		req.Reaction = "like"
	}
	if !validReactions[req.Reaction] {
		writeError(w, http.StatusBadRequest, "invalid reaction type")
		return
	}
	if !validTargetType(req.TargetType, true) || !s.refExists2(req.TargetType, req.TargetID) {
		writeError(w, http.StatusBadRequest, "invalid reaction target")
		return
	}
	_, err := s.db.Exec(`INSERT INTO reactions (user_id, target_type, target_id, reaction) VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE reaction=VALUES(reaction)`, uid, req.TargetType, req.TargetID, req.Reaction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not react")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"counts":      s.counts(req.TargetType, req.TargetID),
		"my_reaction": req.Reaction,
	})
}

func (s *Server) handleUnreact(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req reactionReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validTargetType(req.TargetType, true) {
		writeError(w, http.StatusBadRequest, "invalid target")
		return
	}
	s.db.Exec("DELETE FROM reactions WHERE user_id=? AND target_type=? AND target_id=?", uid, req.TargetType, req.TargetID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"counts":      s.counts(req.TargetType, req.TargetID),
		"my_reaction": nil,
	})
}

// refExists2 also allows comment targets (for reactions).
func (s *Server) refExists2(refType string, refID int64) bool {
	if refType == "comment" {
		var c int
		s.db.QueryRow("SELECT COUNT(*) FROM comments WHERE id=?", refID).Scan(&c)
		return c > 0
	}
	return s.refExists(refType, refID)
}

// ---- shares ----

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req refReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validTargetType(req.RefType, false) || !s.refExists(req.RefType, req.RefID) {
		writeError(w, http.StatusBadRequest, "invalid share target")
		return
	}
	s.db.Exec("INSERT INTO shares (user_id, target_type, target_id) VALUES (?,?,?)", uid, req.RefType, req.RefID)

	base := s.cfg.PublicBaseURL
	if base == "" {
		base = "//" + r.Host
	}
	shareURL := fmt.Sprintf("%s/#/p/%s/%d", strings.TrimRight(base, "/"), req.RefType, req.RefID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"share_url": shareURL,
		"counts":    s.counts(req.RefType, req.RefID),
	})
}
