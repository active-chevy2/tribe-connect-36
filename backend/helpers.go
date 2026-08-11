package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return false
	}
	return true
}

func urlParamInt(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	return v
}

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	return v
}

func nullStr(ns interface{}) string {
	switch v := ns.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func validTargetType(t string, allowComment bool) bool {
	if t == "feed_item" || t == "post" {
		return true
	}
	if allowComment && t == "comment" {
		return true
	}
	return false
}

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
