# Conflux

A **social RSS/Atom feed reader** — read the feeds *and* join the conversation.
Subscribe to any RSS/Atom feed, then **comment, react, share, repost and quote**
both feed articles and other members' posts. Follow people, build a timeline,
and discover new feeds.

Built to be **boringly deployable**: a single Go binary that serves both the API
and the Material Design 3 web UI, plus MariaDB. One `docker compose up` and it runs.

---

## Tech stack

| Layer     | Choice                                             |
|-----------|----------------------------------------------------|
| Frontend  | Vanilla HTML / CSS / JS (Material Design 3), no build step |
| Backend   | Go / Golang (`net/http` + `chi`), single binary    |
| Database  | MariaDB (schema auto-migrated on boot)             |
| Feeds     | `mmcdole/gofeed` (RSS 1.0/2.0, Atom, JSON Feed)    |
| Auth      | JWT (HS256) + bcrypt; **first user becomes admin** |
| Deploy    | Docker Compose, Coolify-compatible                 |

The Go binary serves the whole app on **one internal port** (`8080`): `/api/*`
is the JSON API, everything else serves the static frontend from `/app/web`.

---

## Features

- **Feeds** — add any RSS/Atom/JSON feed by URL, auto-subscribe, background
  refresh every N minutes, per-user subscriptions, admin feed deletion.
- **Social** on both feed items *and* posts:
  - **Reactions** (like / love / celebrate / insightful / laugh)
  - **Comments** (threaded, one level of replies)
  - **Share** (records a share + copies a permalink)
  - **Repost** and **Quote**
- **People** — follow / unfollow, profiles, "Following" timeline, edit your profile.
- **Auth** — register / login, the very first account is promoted to **admin**,
  brute-force lockout after repeated failures.
- **UI** — Material Design 3: navigation rail + bottom bar, FAB, tonal surfaces,
  ripples, reaction popovers, light/dark theme toggle.

---

## Run locally

```bash
cp .env.example .env      # edit the secrets
docker compose up --build
```

Then open the app (behind your reverse proxy / the port your proxy maps).
The **first account you register becomes the admin.**

### Run the Go backend without Docker (dev)

```bash
# start a MariaDB and create a db + user, then:
cd backend
DB_HOST=127.0.0.1 DB_USER=feeduser DB_PASSWORD=feedpass DB_NAME=feedsocial \
JWT_SECRET=dev-secret WEB_DIR=../web PORT=8080 go run .
```

---

## Deploying on Coolify (self-hosted PaaS)

This repo is written specifically to avoid the two most common Coolify failures.

1. **Create a new resource → Docker Compose**, point it at this repository.
2. Set **Environment Variables** (`JWT_SECRET`, `DB_PASSWORD`, `DB_ROOT_PASSWORD`, …).
3. Set the **Domain/FQDN** on the `app` service and choose port **8080**.
4. Deploy. The first user you register is the admin.

### Why it "just works" on Coolify

- **No published host ports.** We use `expose:` (not `ports:`). Coolify's built-in
  reverse proxy already owns host ports; publishing `"8080:80"` yourself causes
  `Bind for 0.0.0.0:8080 failed: port is already allocated`. Routing is handled by
  the domain you set in the Coolify UI.
- **No bind-mounted repo files/config.** Coolify pre-creates bind-mount source
  paths as *empty directories* in its own storage, which shadows your files and
  produces crashes like `nginx: [crit] pread() ".../default.conf" failed (21: Is a
  directory)` and endless restart loops ("no available servers"). Instead we **bake
  everything into the image** via the `Dockerfile` (`COPY web/ /app/web/`) and the
  compose service uses `build: .` with `expose:` only. The database uses a *named
  volume* (`db_data`) for its own data — that is safe and Coolify-managed.
- **Stale-container port conflict on redeploy?** A leftover container from a
  previous deploy may still hold the port. Fix:
  ```bash
  docker ps
  docker rm -f <old_container_id>
  ```
  then redeploy.

---

## API overview

All endpoints are under `/api`. Auth via `Authorization: Bearer <token>`.

```
POST   /api/auth/register        {username,email,password,display_name}
POST   /api/auth/login           {identifier,password}
GET    /api/auth/me
GET    /api/auth/bootstrap        -> {needs_admin}

GET    /api/feeds                 POST /api/feeds {feed_url}
POST   /api/feeds/{id}/subscribe  DELETE /api/feeds/{id}/subscribe
POST   /api/feeds/{id}/refresh    DELETE /api/feeds/{id}   (admin)
GET    /api/items?filter=all|subscribed&page=
GET    /api/items/{id}

GET    /api/timeline?filter=all|following&page=
POST   /api/posts {body}          DELETE /api/posts/{id}
POST   /api/repost {ref_type,ref_id}
POST   /api/quote  {ref_type,ref_id,body}

GET    /api/comments?target_type=&target_id=
POST   /api/comments {target_type,target_id,parent_id?,body}
DELETE /api/comments/{id}

POST   /api/reactions {target_type,target_id,reaction}
DELETE /api/reactions {target_type,target_id}
POST   /api/shares    {ref_type,ref_id}

GET    /api/users                 GET /api/users/{username}
GET    /api/users/{username}/posts
POST   /api/users/{id}/follow     DELETE /api/users/{id}/follow
PUT    /api/profile {display_name,bio,avatar_url}
```

`target_type` / `ref_type` is `feed_item` or `post` (reactions also allow `comment`).
