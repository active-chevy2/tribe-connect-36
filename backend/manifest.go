package main

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	appName, _ := s.getSetting("app_name")
	if appName == "" {
		appName = "Conflux"
	}
	shortName, _ := s.getSetting("app_short_name")
	if shortName == "" {
		shortName = "Conflux"
	}
	themeColor, _ := s.getSetting("app_theme_color")
	if themeColor == "" {
		themeColor = "#006a6a"
	}
	iconURL, _ := s.getSetting("app_icon_url")
	if iconURL == "" {
		iconURL = "/logo192.png" // default maybe
	}

	manifest := map[string]interface{}{
		"name":             appName,
		"short_name":       shortName,
		"start_url":        "/",
		"display":          "standalone",
		"theme_color":      themeColor,
		"background_color": themeColor,
		"icons": []map[string]interface{}{
			{
				"src":   iconURL,
				"sizes": "192x192",
				"type":  "image/png",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(manifest)
}
