# Conflux — PRD

## Problem statement (verbatim intent)
Web app: Docker Compose; HTML/CSS/JS + Go/Golang + MariaDB; social RSS/Atom feed
syndication reader + social media (comments, reactions, share, repost, quote).
Minimise bugs/errors, maximise deployability, **Coolify PaaS-compatible**.

## User choices (from clarification)
- Core product: **equal blend** of feed reader + social network.
- Auth: **full auth** (register + login); **first user becomes admin** (admin
  account created via the normal registration UI).
- Social actions apply to **both feed items and user posts**.
- Scope v1: **fully deployable end-to-end** (Docker Compose + Coolify) with core
  reading + basic social.
- Design: **Material Design 3-like**.

## Architecture
- Single Go binary serves `/api/*` (JSON API) and the static MD3 frontend (`/web`)
  on one port (8080 prod / 3000 preview). Background goroutine refreshes feeds.
- MariaDB, schema auto-migrated from embedded `backend/schema.sql` on boot.
- Coolify-compatible: `Dockerfile` bakes web + binary; compose uses `build: .`,
  `expose:` only (no host `ports:`), named volume for DB data (no repo bind-mounts).

### Backend files (Go, package main)
- `main.go` router + static SPA fallback + feed worker start
- `config.go` env config, `db.go` connect + migrate (`//go:embed schema.sql`)
- `auth.go` bcrypt + JWT + middleware + register/login/me/bootstrap + brute force
- `feeds.go` gofeed fetch/store + feed & item handlers
- `social.go` posts, repost, quote, comments, reactions, shares, timeline
- `users.go` profiles, follow/unfollow, list users, edit profile
- `models.go`, `helpers.go`, `schema.sql`

### Frontend (`/web`)
- `index.html`, `css/style.css` (MD3 tokens + components, light/dark)
- `js/api.js` (fetch client), `js/ui.js` (helpers/dialog/popover/ripple),
  `js/app.js` (hash router + all views)

## Implemented (2026-06)
- Auth: register/login/me/bootstrap, first-user-admin, bcrypt, JWT, lockout. ✅
- Feeds: add (validated), subscribe/unsubscribe, refresh, delete (admin),
  background refresh worker, item list (all/subscribed), item detail. ✅
- Social on feed_item + post: reactions (5 types, toggle), threaded comments,
  share (+permalink), repost, quote. ✅
- People: follow/unfollow, profiles + counts, following timeline, edit profile. ✅
- MD3 UI: rail + bottom nav, FAB, composer, cards, reaction popover, dialogs,
  snackbar, theme toggle. ✅
- Deployment: Dockerfile (multi-stage), docker-compose.yml (Coolify-safe),
  .env.example, .dockerignore, README with Coolify guidance. ✅

## Implemented (2026-10, continuation)
- Restored broken preview env after container reset: reinstalled Go 1.23 +
  MariaDB, recreated DB/user (feeduser/feedsocial), rebuilt Go binary, added a
  supervisor-managed `mariadb` program. Core flows re-verified (register→admin,
  post, heart/like, comment, repost, quote, timeline, settings). ✅
- Added **"About this instance"** page (`#/about`, public) + nav entry. ✅
- Added admin-editable **instance description** (`app_description` setting) that
  feeds the About page and PWA `manifest.json` description. ✅

## Preview wiring (not part of the deliverable)
- IMPORTANT: this is a Go+MariaDB app. `frontend` supervisor program's
  `yarn start` is repurposed (see frontend/package.json) to `exec` the Go binary
  `/app/backend/feedsocial` on :3000. `backend` runs server.py (reverse proxy
  :8001 → :3000). `mariadb` runs via /etc/supervisor/conf.d/mariadb.conf.
- To rebuild after Go changes: `PATH=$PATH:/usr/local/go/bin go build -o feedsocial .`
  then `supervisorctl restart frontend`. Static JS/CSS in /app/web need no rebuild.
- `frontend` supervisor program runs the Go binary on :3000 (full app).
- `backend` supervisor program runs `server.py` = reverse proxy → :3000 (so the
  ingress `/api`→8001 route reaches the Go app).
- `mariadb` supervisor program added.

## Backlog / next
- P1: OPML import/export, feed folders UI, unread tracking / mark-as-read.
- P1: notifications (replies, follows, reactions).
- P2: full-text search, bookmarks/saved, rich media in posts, pagination cursors.
- P2: server-side HTML sanitization of feed content (currently client-side).
