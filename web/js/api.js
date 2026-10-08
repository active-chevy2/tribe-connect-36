/* Conflux API client — talks to the Go backend at /api (same origin). */
const API = {
  base: '/api',
  token: localStorage.getItem('fs_token') || null,

  setToken(t) {
    this.token = t;
    if (t) localStorage.setItem('fs_token', t);
    else localStorage.removeItem('fs_token');
  },

  async request(method, path, body) {
    const headers = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.token) headers['Authorization'] = 'Bearer ' + this.token;
    let res;
    try {
      res = await fetch(this.base + path, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
      });
    } catch (e) {
      throw new Error('Network error — is the server reachable?');
    }
    let data = null;
    const text = await res.text();
    if (text) { try { data = JSON.parse(text); } catch (_) { data = { error: text }; } }
    if (!res.ok) {
      if (res.status === 401 && this.token) {
        this.setToken(null);
      }
      throw new Error((data && data.error) || ('Request failed (' + res.status + ')'));
    }
    return data;
  },

  get(p) { return this.request('GET', p); },
  post(p, b) { return this.request('POST', p, b === undefined ? {} : b); },
  put(p, b) { return this.request('PUT', p, b); },
  del(p, b) { return this.request('DELETE', p, b === undefined ? {} : b); },

  // auth
  bootstrap() { return this.get('/auth/bootstrap'); },
  register(d) { return this.post('/auth/register', d); },
  login(d) { return this.post('/auth/login', d); },
  me() { return this.get('/auth/me'); },
  forgotPassword(email) { return this.post('/auth/forgot', { email }); },
  resetPassword(token, newPassword) { return this.post('/auth/reset', { token, new_password: newPassword }); },

  // feeds & items
  feeds() { return this.get('/feeds'); },
  addFeed(url) { return this.post('/feeds', { feed_url: url }); },
  subscribe(id) { return this.post('/feeds/' + id + '/subscribe'); },
  unsubscribe(id) { return this.del('/feeds/' + id + '/subscribe'); },
  refreshFeed(id) { return this.post('/feeds/' + id + '/refresh'); },
  deleteFeed(id) { return this.del('/feeds/' + id); },
  items(filter, page) { return this.get('/items?filter=' + filter + '&page=' + page); },
  item(id) { return this.get('/items/' + id); },

  // social
  timeline(filter, page) { return this.get('/timeline?filter=' + filter + '&page=' + page); },
  createPost(body, visibility, format) { return this.post('/posts', { body, visibility, format }); },
  post_(id) { return this.get('/posts/' + id); },
  deletePost(id) { return this.del('/posts/' + id); },
  repost(t, id) { return this.post('/repost', { ref_type: t, ref_id: id }); },
  quote(t, id, body, format) { return this.post('/quote', { ref_type: t, ref_id: id, body, format }); },
  comments(t, id) { return this.get('/comments?target_type=' + t + '&target_id=' + id); },
  addComment(d) { return this.post('/comments', d); },
  deleteComment(id) { return this.del('/comments/' + id); },
  react(t, id, r) { return this.post('/reactions', { target_type: t, target_id: id, reaction: r }); },
  unreact(t, id) { return this.del('/reactions', { target_type: t, target_id: id }); },
  share(t, id) { return this.post('/shares', { ref_type: t, ref_id: id }); },

  // people
  users() { return this.get('/users'); },
  profile(u) { return this.get('/users/' + u); },
  userPosts(u, page) { return this.get('/users/' + u + '/posts?page=' + page); },
  follow(id) { return this.post('/users/' + id + '/follow'); },
  unfollow(id) { return this.del('/users/' + id + '/follow'); },
  updateProfile(d) { return this.put('/profile', d); },
  userFeed(username) { return this.get('/users/' + username + '/feed'); }, // returns XML

  // invites
  invites() { return this.get('/invites'); },
  createInvite() { return this.post('/invites'); },
  revokeInvite(id) { return this.del('/invites/' + id); },

  // settings (admin)
  getSettings() { return this.get('/settings'); },
  updateSettings(settings) { return this.put('/settings', settings); },
  adminUpdatePost(postId, visibility) { return this.put('/admin/posts/' + postId, { visibility }); },
};
