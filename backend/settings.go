package main

import (
	"database/sql"
	"net/http"
)

func (s *Server) getSetting(key string) (string, error) {
	var val string
	err := s.db.QueryRow("SELECT `value` FROM settings WHERE `key`=?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (s *Server) setSetting(key, value string) error {
	_, err := s.db.Exec("INSERT INTO settings (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `value`=?", key, value, value)
	return err
}

// ---- handlers ----

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	rows, err := s.db.Query("SELECT `key`, `value` FROM settings")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	defer rows.Close()
	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		settings[k] = v
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	var req map[string]string
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	allowed := map[string]bool{
		"allow_registration":      true,
		"invites_enabled":         true,
		"smtp_enabled":            true,
		"smtp_host":               true,
		"smtp_port":               true,
		"smtp_user":               true,
		"smtp_password":           true,
		"smtp_from":               true,
		"smtp_from_name":          true,
		"smtp_tls":                true,
		"app_name":                true,
		"app_short_name":          true,
		"app_description":         true,
		"app_theme_color":         true,
		"app_icon_url":            true,
		"default_post_visibility": true,
	}
	for k, v := range req {
		if !allowed[k] {
			continue
		}
		if err := s.setSetting(k, v); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update setting "+k)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "settings updated"})
}

// Admin override post visibility
func (s *Server) handleAdminUpdatePost(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	postID := urlParamInt(r, "id")
	var req struct {
		Visibility string `json:"visibility"`
	}
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Visibility != "public" && req.Visibility != "private" {
		writeError(w, http.StatusBadRequest, "visibility must be public or private")
		return
	}
	_, err := s.db.Exec("UPDATE posts SET visibility=? WHERE id=?", req.Visibility, postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update post")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
