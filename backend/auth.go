package main

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxLoginAttempts = 5
	lockoutMinutes   = 15
	tokenTTL         = 7 * 24 * time.Hour
)

func (s *Server) makeToken(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID, 10),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(tokenTTL).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.cfg.JWTSecret)
}

func (s *Server) parseToken(tokenStr string) (int64, bool) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return s.cfg.JWTSecret, nil
	})
	if err != nil || !tok.Valid {
		return 0, false
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, false
	}
	sub, _ := claims["sub"].(string)
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func (s *Server) loadUser(id int64) *User {
	row := s.db.QueryRow("SELECT "+userCols+" FROM users WHERE id=?", id)
	u, err := scanUser(row)
	if err != nil {
		return nil
	}
	return u
}

func (s *Server) userFromRequest(r *http.Request) *User {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil
	}
	id, ok := s.parseToken(strings.TrimSpace(auth[7:]))
	if !ok {
		return nil
	}
	return s.loadUser(id)
}

func (s *Server) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := s.userFromRequest(r); u != nil {
			r = r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := s.userFromRequest(r)
		if u == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userCtxKey).(*User)
	return u
}

func currentUserID(r *http.Request) int64 {
	if u := currentUser(r); u != nil {
		return u.ID
	}
	return 0
}

// ---- handlers ----

type registerReq struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"needs_admin": count == 0,
		"user_count":  count,
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	if len(req.Username) < 3 || !isValidUsername(req.Username) {
		writeError(w, http.StatusBadRequest, "username must be at least 3 characters (letters, numbers, underscore)")
		return
	}
	if !strings.Contains(req.Email, "@") {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	var exists int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username=? OR email=?", req.Username, req.Email).Scan(&exists)
	if exists > 0 {
		writeError(w, http.StatusConflict, "username or email already taken")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	// First registered user becomes the admin.
	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total)
	isAdmin := total == 0

	res, err := s.db.Exec(
		"INSERT INTO users (username, email, password_hash, display_name, is_admin) VALUES (?,?,?,?,?)",
		req.Username, req.Email, string(hash), req.DisplayName, isAdmin,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}
	id, _ := res.LastInsertId()
	token, _ := s.makeToken(id)
	u := s.loadUser(id)
	writeJSON(w, http.StatusCreated, map[string]interface{}{"token": token, "user": u})
}

type loginReq struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := strings.ToLower(strings.TrimSpace(req.Identifier))
	if id == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "identifier and password are required")
		return
	}

	if locked, until := s.isLocked(id); locked {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again after "+until.Format("15:04:05"))
		return
	}

	var userID int64
	var hash string
	err := s.db.QueryRow(
		"SELECT id, password_hash FROM users WHERE email=? OR username=?", id, id,
	).Scan(&userID, &hash)
	if err == sql.ErrNoRows {
		s.recordFailed(id)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		s.recordFailed(id)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.clearAttempts(id)
	token, _ := s.makeToken(userID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": s.loadUser(userID)})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// ---- brute force helpers ----

func (s *Server) isLocked(identifier string) (bool, time.Time) {
	var lockedUntil sql.NullTime
	err := s.db.QueryRow("SELECT locked_until FROM login_attempts WHERE identifier=?", identifier).Scan(&lockedUntil)
	if err != nil {
		return false, time.Time{}
	}
	if lockedUntil.Valid && lockedUntil.Time.After(time.Now()) {
		return true, lockedUntil.Time
	}
	return false, time.Time{}
}

func (s *Server) recordFailed(identifier string) {
	s.db.Exec(`INSERT INTO login_attempts (identifier, attempts) VALUES (?, 1)
		ON DUPLICATE KEY UPDATE attempts = attempts + 1`, identifier)
	var attempts int
	s.db.QueryRow("SELECT attempts FROM login_attempts WHERE identifier=?", identifier).Scan(&attempts)
	if attempts >= maxLoginAttempts {
		until := time.Now().Add(lockoutMinutes * time.Minute)
		s.db.Exec("UPDATE login_attempts SET locked_until=?, attempts=0 WHERE identifier=?", until, identifier)
	}
}

func (s *Server) clearAttempts(identifier string) {
	s.db.Exec("DELETE FROM login_attempts WHERE identifier=?", identifier)
}

func isValidUsername(s string) bool {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
