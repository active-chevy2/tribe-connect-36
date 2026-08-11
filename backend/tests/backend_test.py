"""Backend API tests for Conflux (Go + MariaDB)."""
import os
import time
import uuid
import pytest
import requests

BASE_URL = os.environ.get("REACT_APP_BACKEND_URL", "https://feed-share-110.preview.emergentagent.com").rstrip("/")
API = f"{BASE_URL}/api"

ADMIN = {"identifier": "admin@conflux.app", "password": "admin123"}


@pytest.fixture(scope="session")
def admin_token():
    r = requests.post(f"{API}/auth/login", json=ADMIN, timeout=15)
    assert r.status_code == 200, f"admin login failed: {r.status_code} {r.text}"
    return r.json()["token"]


@pytest.fixture(scope="session")
def admin_headers(admin_token):
    return {"Authorization": f"Bearer {admin_token}"}


@pytest.fixture(scope="session")
def new_user():
    uid = uuid.uuid4().hex[:8]
    payload = {
        "username": f"test_{uid}",
        "email": f"test_{uid}@example.com",
        "password": "password123",
        "display_name": f"Test {uid}",
    }
    r = requests.post(f"{API}/auth/register", json=payload, timeout=15)
    assert r.status_code == 201, f"register failed: {r.status_code} {r.text}"
    data = r.json()
    assert "token" in data
    assert data["user"]["is_admin"] is False, "new user should NOT be admin"
    payload["token"] = data["token"]
    payload["id"] = data["user"]["id"]
    return payload


@pytest.fixture(scope="session")
def user_headers(new_user):
    return {"Authorization": f"Bearer {new_user['token']}"}


# ---------- health / bootstrap ----------

def test_health():
    r = requests.get(f"{API}/health", timeout=10)
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_bootstrap():
    r = requests.get(f"{API}/auth/bootstrap", timeout=10)
    assert r.status_code == 200
    data = r.json()
    assert data["needs_admin"] is False
    assert data["user_count"] >= 1


# ---------- auth ----------

def test_admin_login_and_is_admin(admin_headers):
    r = requests.get(f"{API}/auth/me", headers=admin_headers, timeout=10)
    assert r.status_code == 200
    me = r.json()
    assert me["is_admin"] is True
    assert me["email"] == "admin@conflux.app"


def test_new_user_not_admin(new_user, user_headers):
    r = requests.get(f"{API}/auth/me", headers=user_headers, timeout=10)
    assert r.status_code == 200
    assert r.json()["is_admin"] is False


def test_login_invalid():
    r = requests.post(f"{API}/auth/login", json={"identifier": "admin@conflux.app", "password": "wrong"}, timeout=10)
    assert r.status_code == 401


def test_register_validation():
    r = requests.post(f"{API}/auth/register", json={"username": "x", "email": "bad", "password": "123"}, timeout=10)
    assert r.status_code == 400


# ---------- feeds ----------

def test_add_and_list_feed(user_headers):
    r = requests.post(f"{API}/feeds", headers=user_headers, json={"feed_url": "https://hnrss.org/frontpage"}, timeout=30)
    assert r.status_code in (200, 201), f"add feed: {r.status_code} {r.text}"
    feed = r.json()
    assert feed.get("id")
    fid = feed["id"]

    # subscribe
    r = requests.post(f"{API}/feeds/{fid}/subscribe", headers=user_headers, timeout=10)
    assert r.status_code in (200, 201, 204)

    # list feeds
    r = requests.get(f"{API}/feeds", headers=user_headers, timeout=15)
    assert r.status_code == 200
    feeds = r.json()
    assert any(f["id"] == fid for f in feeds)

    # items
    time.sleep(1)
    r = requests.get(f"{API}/items", headers=user_headers, timeout=15)
    assert r.status_code == 200
    items = r.json()
    assert isinstance(items, list)
    return fid, items


def test_items_subscribed_only(user_headers):
    r = requests.get(f"{API}/items?filter=subscribed", headers=user_headers, timeout=15)
    assert r.status_code == 200
    assert isinstance(r.json(), list)


# ---------- posts / social ----------

@pytest.fixture(scope="session")
def created_post(user_headers):
    r = requests.post(f"{API}/posts", headers=user_headers, json={"body": "TEST_post hello world"}, timeout=10)
    assert r.status_code == 201, r.text
    return r.json()


def test_create_post_and_timeline(created_post, user_headers):
    pid = created_post["id"]
    assert created_post["body"] == "TEST_post hello world"
    r = requests.get(f"{API}/timeline", headers=user_headers, timeout=15)
    assert r.status_code == 200
    assert any(p["id"] == pid for p in r.json())


def test_react_on_post(created_post, user_headers):
    pid = created_post["id"]
    r = requests.post(f"{API}/reactions", headers=user_headers,
                      json={"target_type": "post", "target_id": pid, "reaction": "love"}, timeout=10)
    assert r.status_code == 200
    data = r.json()
    assert data["my_reaction"] == "love"
    assert data["counts"]["reactions"] >= 1


def test_comment_and_reply_and_delete(created_post, user_headers):
    pid = created_post["id"]
    r = requests.post(f"{API}/comments", headers=user_headers,
                      json={"target_type": "post", "target_id": pid, "body": "first"}, timeout=10)
    assert r.status_code == 201, r.text
    parent_id = r.json()["id"]

    # threaded reply
    r = requests.post(f"{API}/comments", headers=user_headers,
                      json={"target_type": "post", "target_id": pid, "parent_id": parent_id, "body": "reply"}, timeout=10)
    assert r.status_code == 201
    reply_id = r.json()["id"]

    # list comments
    r = requests.get(f"{API}/comments?target_type=post&target_id={pid}", headers=user_headers, timeout=10)
    assert r.status_code == 200
    comments = r.json()
    assert len(comments) >= 2

    # like a comment (react)
    r = requests.post(f"{API}/reactions", headers=user_headers,
                      json={"target_type": "comment", "target_id": parent_id, "reaction": "like"}, timeout=10)
    assert r.status_code == 200

    # delete own comment
    r = requests.delete(f"{API}/comments/{reply_id}", headers=user_headers, timeout=10)
    assert r.status_code == 200


def test_repost_and_quote(created_post, user_headers):
    pid = created_post["id"]
    r = requests.post(f"{API}/repost", headers=user_headers,
                      json={"ref_type": "post", "ref_id": pid}, timeout=10)
    assert r.status_code == 201, r.text
    assert r.json()["kind"] == "repost"
    assert r.json()["ref"]["id"] == pid

    r = requests.post(f"{API}/quote", headers=user_headers,
                      json={"ref_type": "post", "ref_id": pid, "body": "my thoughts"}, timeout=10)
    assert r.status_code == 201
    assert r.json()["kind"] == "quote"
    assert r.json()["body"] == "my thoughts"


def test_share(created_post, user_headers):
    pid = created_post["id"]
    r = requests.post(f"{API}/shares", headers=user_headers,
                      json={"ref_type": "post", "ref_id": pid}, timeout=10)
    assert r.status_code == 200
    assert "share_url" in r.json()


# ---------- users / follow / profile ----------

def test_list_users_and_follow(new_user, user_headers, admin_headers):
    # admin follows new user
    r = requests.get(f"{API}/users", headers=admin_headers, timeout=10)
    assert r.status_code == 200
    users = r.json()
    assert any(u["id"] == new_user["id"] for u in users)

    r = requests.post(f"{API}/users/{new_user['id']}/follow", headers=admin_headers, timeout=10)
    assert r.status_code in (200, 201, 204)

    r = requests.get(f"{API}/users/{new_user['username']}", headers=admin_headers, timeout=10)
    assert r.status_code == 200
    prof = r.json()
    assert prof.get("follower_count", 0) >= 1 or prof.get("followers", 0) >= 1


def test_update_profile(user_headers, new_user):
    r = requests.put(f"{API}/profile", headers=user_headers,
                     json={"display_name": "Updated Name", "bio": "new bio", "avatar_url": ""}, timeout=10)
    assert r.status_code == 200, r.text

    r = requests.get(f"{API}/users/{new_user['username']}", headers=user_headers, timeout=10)
    assert r.status_code == 200
    prof = r.json()
    # display name might be under 'display_name' or nested; accept either
    dn = prof.get("display_name") or (prof.get("user") or {}).get("display_name")
    assert dn == "Updated Name"


def test_unauthenticated_write_rejected():
    r = requests.post(f"{API}/posts", json={"body": "nope"}, timeout=10)
    assert r.status_code == 401
