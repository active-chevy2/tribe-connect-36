package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

// handleListInvites - list invites (user sees own, admin sees all)
func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	var rows *sql.Rows
	var err error
	if u.IsAdmin {
		rows, err = s.db.Query("SELECT id, token, creator_id, used, used_by_user_id, created_at, expires_at FROM invites ORDER BY created_at DESC")
	} else {
		rows, err = s.db.Query("SELECT id, token, creator_id, used, used_by_user_id, created_at, expires_at FROM invites WHERE creator_id=? ORDER BY created_at DESC", u.ID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load invites")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var inv Invite
		var usedBy sql.NullInt64
		var expires sql.NullTime
		err := rows.Scan(&inv.ID, &inv.Token, &inv.CreatorID, &inv.Used, &usedBy, &inv.CreatedAt, &expires)
		if err != nil {
			continue
		}
		if usedBy.Valid {
			inv.UsedByUserID = &usedBy.Int64
		}
		if expires.Valid {
			inv.ExpiresAt = &expires.Time
		}
		// Get creator username
		var creatorName string
		s.db.QueryRow("SELECT username FROM users WHERE id=?", inv.CreatorID).Scan(&creatorName)
		out = append(out, map[string]interface{}{
			"id":          inv.ID,
			"token":       inv.Token,
			"creator_id":  inv.CreatorID,
			"creator_name": creatorName,
			"used":        inv.Used,
			"used_by_user_id": inv.UsedByUserID,
			"created_at":  inv.CreatedAt,
			"expires_at":  inv.ExpiresAt,
			"link":        s.cfg.PublicBaseURL + "/#/register?invite=" + inv.Token,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateInvite - create a new invite
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	// Check if invites enabled
	enabled, _ := s.getSetting("invites_enabled")
	if enabled != "1" {
		writeError(w, http.StatusForbidden, "invites are disabled")
		return
	}
	// Generate token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate token")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	// Optionally set expiration (1 week)
	expires := time.Now().Add(7 * 24 * time.Hour)
	_, err := s.db.Exec("INSERT INTO invites (token, creator_id, expires_at) VALUES (?, ?, ?)", token, u.ID, expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create invite")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token": token,
		"link":  s.cfg.PublicBaseURL + "/#/register?invite=" + token,
	})
}

// handleRevokeInvite - revoke an invite (owner or admin)
func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	inviteID := urlParamInt(r, "id")
	var creatorID int64
	err := s.db.QueryRow("SELECT creator_id FROM invites WHERE id=?", inviteID).Scan(&creatorID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}
	if creatorID != u.ID && !u.IsAdmin {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	_, err = s.db.Exec("DELETE FROM invites WHERE id=?", inviteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
