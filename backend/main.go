package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func spaHandler(webDir string) http.HandlerFunc {
	index := filepath.Join(webDir, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean("/" + r.URL.Path) // leading "/" prevents escaping webDir
		p := filepath.Join(webDir, clean)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		http.ServeFile(w, r, index)
	}
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors)

	// Manifest.json endpoint (PWA)
	r.Get("/manifest.json", s.handleManifest)

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			if err := s.db.Ping(); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status": "unavailable",
					"error":  err.Error(),
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})

		// public auth
		api.Get("/auth/bootstrap", s.handleBootstrap)
		api.Post("/auth/register", s.handleRegister)
		api.Post("/auth/login", s.handleLogin)
		api.Post("/auth/forgot", s.handleForgotPassword)
		api.Post("/auth/reset", s.handleResetPassword)

		// public reads (optional auth for personalization)
		api.Group(func(pub chi.Router) {
			pub.Use(s.optionalAuth)
			pub.Get("/feeds", s.handleListFeeds)
			pub.Get("/feeds/{id}/items", s.handleListItems)
			pub.Get("/items", s.handleListItems)
			pub.Get("/items/{id}", s.handleGetItem)
			pub.Get("/timeline", s.handleTimeline)
			pub.Get("/posts/{id}", s.handleGetPost)
			pub.Get("/comments", s.handleListComments)
			pub.Get("/users", s.handleListUsers)
			pub.Get("/users/{username}", s.handleGetProfile)
			pub.Get("/users/{username}/posts", s.handleUserPosts)
			pub.Get("/users/{username}/feed", s.handleUserFeed) // RSS
		})

		// authenticated writes
		api.Group(func(pr chi.Router) {
			pr.Use(s.requireAuth)
			pr.Get("/auth/me", s.handleMe)
			pr.Post("/auth/logout", s.handleLogout)
			pr.Put("/profile", s.handleUpdateProfile)

			pr.Post("/feeds", s.handleAddFeed)
			pr.Post("/feeds/{id}/subscribe", s.handleSubscribe)
			pr.Delete("/feeds/{id}/subscribe", s.handleUnsubscribe)
			pr.Post("/feeds/{id}/refresh", s.handleRefreshFeed)
			pr.Delete("/feeds/{id}", s.handleDeleteFeed)

			pr.Post("/posts", s.handleCreatePost)
			pr.Delete("/posts/{id}", s.handleDeletePost)
			pr.Post("/repost", s.handleRepost)
			pr.Post("/quote", s.handleQuote)

			pr.Post("/comments", s.handleCreateComment)
			pr.Delete("/comments/{id}", s.handleDeleteComment)

			pr.Post("/reactions", s.handleReact)
			pr.Delete("/reactions", s.handleUnreact)

			pr.Post("/shares", s.handleShare)

			pr.Post("/users/{id}/follow", s.handleFollow)
			pr.Delete("/users/{id}/follow", s.handleUnfollow)

			// Invites
			pr.Get("/invites", s.handleListInvites)
			pr.Post("/invites", s.handleCreateInvite)
			pr.Delete("/invites/{id}", s.handleRevokeInvite)

			// Admin endpoints
			pr.Get("/settings", s.handleGetSettings)
			pr.Put("/settings", s.handleUpdateSettings)
			pr.Put("/admin/posts/{id}", s.handleAdminUpdatePost)
		})
	})

	r.NotFound(spaHandler(s.cfg.WebDir))
	return r
}

func main() {
	cfg := loadConfig()
	db, err := connectDB(cfg.DSN)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	if err := migrate(db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	s := &Server{db: db, cfg: cfg}
	if cfg.FeedWorker {
		go s.feedWorker()
	}
	log.Printf("feedsocial listening on :%s (feed_worker=%v, web_dir=%s)", cfg.Port, cfg.FeedWorker, cfg.WebDir)
	if err := http.ListenAndServe(":"+cfg.Port, s.routes()); err != nil {
		log.Fatal(err)
	}
}
