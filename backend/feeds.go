package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"io"
	"bytes"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

var imgRe = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)

func extractImage(item *gofeed.Item) string {
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}
	for _, e := range item.Enclosures {
		if strings.HasPrefix(strings.ToLower(e.Type), "image") {
			return e.URL
		}
	}
	if item.Extensions != nil {
		if media, ok := item.Extensions["media"]; ok {
			for _, tag := range []string{"content", "thumbnail"} {
				if arr, ok := media[tag]; ok {
					for _, ext := range arr {
						if u := ext.Attrs["url"]; u != "" {
							return u
						}
					}
				}
			}
		}
	}
	body := item.Content
	if body == "" {
		body = item.Description
	}
	if m := imgRe.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}
	return ""
}

// detectFeedURL attempts to find the actual RSS/Atom feed URL from a given URL.
func detectFeedURL(url string) (string, error) {
	// First, try to parse as feed directly
	fp := gofeed.NewParser()
	fp.Client = &http.Client{Timeout: 10 * time.Second}
	_, err := fp.ParseURL(url)
	if err == nil {
		return url, nil // it's a feed
	}

	// Fetch the HTML page
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse HTML to find <link rel="alternate" type="application/rss+xml" ...>
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	var feedURL string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href, typ string
			for _, attr := range n.Attr {
				if attr.Key == "rel" {
					rel = attr.Val
				}
				if attr.Key == "href" {
					href = attr.Val
				}
				if attr.Key == "type" {
					typ = attr.Val
				}
			}
			if strings.Contains(rel, "alternate") && (strings.Contains(typ, "rss") || strings.Contains(typ, "atom")) {
				feedURL = href
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	if feedURL != "" {
		// Resolve relative URL
		if !strings.HasPrefix(feedURL, "http") {
			base, _ := url.Parse(url)
			ref, _ := url.Parse(feedURL)
			feedURL = base.ResolveReference(ref).String()
		}
		return feedURL, nil
	}

	// Try common paths
	common := []string{"/feed", "/rss", "/atom", "/feed.xml", "/rss.xml", "/atom.xml"}
	for _, path := range common {
		u, _ := url.Parse(url)
		u.Path = path
		testURL := u.String()
		_, err := fp.ParseURL(testURL)
		if err == nil {
			return testURL, nil
		}
	}
	return "", fmt.Errorf("could not detect feed URL")
}

func (s *Server) fetchAndStoreFeed(feedID int64, feedURL string) error {
	fp := gofeed.NewParser()
	fp.Client = &http.Client{Timeout: 20 * time.Second}
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		s.db.Exec("UPDATE feeds SET last_fetched_at=?, fetch_error=? WHERE id=?", time.Now().UTC(), err.Error(), feedID)
		return err
	}

	image := ""
	if feed.Image != nil {
		image = feed.Image.URL
	}
	s.db.Exec("UPDATE feeds SET title=?, site_url=?, description=?, image_url=?, last_fetched_at=?, fetch_error=NULL WHERE id=?",
		trimTo(feed.Title, 255), trimTo(feed.Link, 500), feed.Description, trimTo(image, 500), time.Now().UTC(), feedID)

	for _, item := range feed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			guid = item.Title
		}
		if guid == "" {
			continue
		}
		var published interface{}
		if item.PublishedParsed != nil {
			published = item.PublishedParsed.UTC()
		} else if item.UpdatedParsed != nil {
			published = item.UpdatedParsed.UTC()
		} else {
			published = time.Now().UTC()
		}
		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}
		summary := item.Description
		content := item.Content
		if content == "" {
			content = item.Description
		}
		s.db.Exec(`INSERT INTO feed_items (feed_id, guid, title, link, author, summary, content, image_url, published_at)
			VALUES (?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE title=VALUES(title), link=VALUES(link), summary=VALUES(summary), content=VALUES(content), image_url=VALUES(image_url)`,
			feedID, trimTo(guid, 500), item.Title, trimTo(item.Link, 1000), trimTo(author, 255),
			summary, content, trimTo(extractImage(item), 1000), published)
	}
	return nil
}

func (s *Server) refreshAllFeeds() {
	rows, err := s.db.Query("SELECT id, feed_url FROM feeds")
	if err != nil {
		return
	}
	type f struct {
		id  int64
		url string
	}
	var feeds []f
	for rows.Next() {
		var x f
		rows.Scan(&x.id, &x.url)
		feeds = append(feeds, x)
	}
	rows.Close()
	for _, x := range feeds {
		if err := s.fetchAndStoreFeed(x.id, x.url); err != nil {
			log.Printf("feed refresh error [%s]: %v", x.url, err)
		}
	}
}

func (s *Server) feedWorker() {
	time.Sleep(5 * time.Second)
	s.refreshAllFeeds()
	ticker := time.NewTicker(time.Duration(s.cfg.RefreshMinutes) * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.refreshAllFeeds()
	}
}

// ---- handlers ----

func (s *Server) feedJSON(feedID int64, userID int64) map[string]interface{} {
	var (
		id             int64
		feedURL        string
		title, site    sql.NullString
		desc, img, ferr sql.NullString
		last           sql.NullTime
		createdAt      time.Time
	)
	err := s.db.QueryRow(`SELECT id, feed_url, title, site_url, description, image_url, fetch_error, last_fetched_at, created_at
		FROM feeds WHERE id=?`, feedID).Scan(&id, &feedURL, &title, &site, &desc, &img, &ferr, &last, &createdAt)
	if err != nil {
		return nil
	}
	var itemCount, subCount int
	s.db.QueryRow("SELECT COUNT(*) FROM feed_items WHERE feed_id=?", id).Scan(&itemCount)
	s.db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE feed_id=?", id).Scan(&subCount)
	subscribed := false
	if userID != 0 {
		var c int
		s.db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE feed_id=? AND user_id=?", id, userID).Scan(&c)
		subscribed = c > 0
	}
	var lastFetched interface{}
	if last.Valid {
		lastFetched = last.Time
	}
	return map[string]interface{}{
		"id":              id,
		"feed_url":        feedURL,
		"title":           title.String,
		"site_url":        site.String,
		"description":     desc.String,
		"image_url":       img.String,
		"fetch_error":     ferr.String,
		"last_fetched_at": lastFetched,
		"created_at":      createdAt,
		"item_count":      itemCount,
		"subscriber_count": subCount,
		"subscribed":      subscribed,
	}
}

func (s *Server) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	rows, err := s.db.Query("SELECT id FROM feeds ORDER BY title ASC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load feeds")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		if f := s.feedJSON(id, uid); f != nil {
			out = append(out, f)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type addFeedReq struct {
	FeedURL string `json:"feed_url"`
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	var req addFeedReq
	if !decodeJSON(r, &req) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.FeedURL = strings.TrimSpace(req.FeedURL)
	if req.FeedURL == "" || !strings.HasPrefix(req.FeedURL, "http") {
		writeError(w, http.StatusBadRequest, "a valid feed URL (http/https) is required")
		return
	}
	if len(req.FeedURL) > 500 {
		writeError(w, http.StatusBadRequest, "feed URL is too long")
		return
	}

	// Auto-detect feed if needed
	detected, err := detectFeedURL(req.FeedURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not find a feed at that URL: "+err.Error())
		return
	}
	req.FeedURL = detected

	var feedID int64
	err = s.db.QueryRow("SELECT id FROM feeds WHERE feed_url=?", req.FeedURL).Scan(&feedID)
	if err == sql.ErrNoRows {
		// Validate by fetching before inserting.
		fp := gofeed.NewParser()
		fp.Client = &http.Client{Timeout: 20 * time.Second}
		if _, perr := fp.ParseURL(req.FeedURL); perr != nil {
			writeError(w, http.StatusBadRequest, "could not read that feed: "+perr.Error())
			return
		}
		res, ierr := s.db.Exec("INSERT INTO feeds (feed_url, created_by) VALUES (?,?)", req.FeedURL, uid)
		if ierr != nil {
			writeError(w, http.StatusInternalServerError, "could not save feed")
			return
		}
		feedID, _ = res.LastInsertId()
		if ferr := s.fetchAndStoreFeed(feedID, req.FeedURL); ferr != nil {
			log.Printf("initial fetch error: %v", ferr)
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	s.db.Exec("INSERT IGNORE INTO subscriptions (user_id, feed_id) VALUES (?,?)", uid, feedID)
	writeJSON(w, http.StatusCreated, s.feedJSON(feedID, uid))
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	feedID := urlParamInt(r, "id")
	var exists int
	s.db.QueryRow("SELECT COUNT(*) FROM feeds WHERE id=?", feedID).Scan(&exists)
	if exists == 0 {
		writeError(w, http.StatusNotFound, "feed not found")
		return
	}
	s.db.Exec("INSERT IGNORE INTO subscriptions (user_id, feed_id) VALUES (?,?)", uid, feedID)
	writeJSON(w, http.StatusOK, s.feedJSON(feedID, uid))
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	feedID := urlParamInt(r, "id")
	s.db.Exec("DELETE FROM subscriptions WHERE user_id=? AND feed_id=?", uid, feedID)
	writeJSON(w, http.StatusOK, s.feedJSON(feedID, uid))
}

func (s *Server) handleRefreshFeed(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	feedID := urlParamInt(r, "id")
	var feedURL string
	if err := s.db.QueryRow("SELECT feed_url FROM feeds WHERE id=?", feedID).Scan(&feedURL); err != nil {
		writeError(w, http.StatusNotFound, "feed not found")
		return
	}
	if err := s.fetchAndStoreFeed(feedID, feedURL); err != nil {
		writeError(w, http.StatusBadGateway, "refresh failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.feedJSON(feedID, uid))
}

func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !u.IsAdmin {
		writeError(w, http.StatusForbidden, "only admins can delete feeds")
		return
	}
	feedID := urlParamInt(r, "id")
	s.db.Exec("DELETE FROM feeds WHERE id=?", feedID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit
	filter := r.URL.Query().Get("filter")
	feedID := int64(queryInt(r, "feed_id", 0))

	base := "SELECT id FROM feed_items"
	where := []string{}
	args := []interface{}{}
	if feedID != 0 {
		where = append(where, "feed_id=?")
		args = append(args, feedID)
	}
	if filter == "subscribed" && uid != 0 {
		where = append(where, "feed_id IN (SELECT feed_id FROM subscriptions WHERE user_id=?)")
		args = append(args, uid)
	}
	if len(where) > 0 {
		base += " WHERE " + strings.Join(where, " AND ")
	}
	base += " ORDER BY published_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(base, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load items")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		if it := s.itemJSON(id, uid); it != nil {
			out = append(out, it)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	it := s.itemJSON(urlParamInt(r, "id"), uid)
	if it == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) itemJSON(itemID int64, userID int64) map[string]interface{} {
	var (
		id, feedID              int64
		title, link, author     sql.NullString
		summary, content, image sql.NullString
		published               sql.NullTime
		feedTitle               sql.NullString
	)
	err := s.db.QueryRow(`SELECT fi.id, fi.feed_id, fi.title, fi.link, fi.author, fi.summary, fi.content, fi.image_url, fi.published_at, f.title
		FROM feed_items fi JOIN feeds f ON f.id=fi.feed_id WHERE fi.id=?`, itemID).
		Scan(&id, &feedID, &title, &link, &author, &summary, &content, &image, &published, &feedTitle)
	if err != nil {
		return nil
	}
	var pub interface{}
	if published.Valid {
		pub = published.Time
	}
	return map[string]interface{}{
		"id":           id,
		"type":         "feed_item",
		"feed_id":      feedID,
		"feed_title":   feedTitle.String,
		"title":        title.String,
		"link":         link.String,
		"author":       author.String,
		"summary":      summary.String,
		"content":      content.String,
		"image_url":    image.String,
		"published_at": pub,
		"counts":       s.counts("feed_item", id),
		"my_reaction":  s.myReaction(userID, "feed_item", id),
	}
}
