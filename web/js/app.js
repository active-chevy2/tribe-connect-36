/* Conflux — SPA controller (vanilla JS, hash routing). */
const state = { user: null, authMode: 'login', needsAdmin: false, theme: localStorage.getItem('fs_theme') || '' };

if (state.theme) document.documentElement.setAttribute('data-theme', state.theme);

/* ---------------- boot ---------------- */
async function boot() {
  if (API.token) {
    try { state.user = await API.me(); } catch (_) { API.setToken(null); }
  }
  window.addEventListener('hashchange', render);
  document.addEventListener('click', onGlobalAction);
  render();
}

function go(hash) {
  if (location.hash === hash) render();
  else location.hash = hash;
}

/* ---------------- router ---------------- */
async function render() {
  if (!state.user) { renderAuth(); return; }
  const hash = location.hash || '#/';
  const parts = hash.slice(2).split('/'); // drop "#/"
  const [seg, arg] = parts;
  try {
    if (!seg || seg === '') return viewHome();
    if (seg === 'explore') return viewExplore();
    if (seg === 'people') return viewPeople();
    if (seg === 'profile') return viewProfile(arg || state.user.username);
    if (seg === 'post') return viewDetail('post', arg);
    if (seg === 'feed_item') return viewDetail('feed_item', arg);
    return viewHome();
  } catch (e) { toast(e.message || 'Something went wrong'); }
}

/* ---------------- shell ---------------- */
const NAV = [
  { key: 'home', icon: 'home', label: 'Home', href: '#/' },
  { key: 'explore', icon: 'rss_feed', label: 'Explore', href: '#/explore' },
  { key: 'people', icon: 'groups', label: 'People', href: '#/people' },
  { key: 'profile', icon: 'person', label: 'Profile', href: '#/profile/' },
];

function navHTML(active, cls, itemCls) {
  return NAV.map(n => {
    const href = n.key === 'profile' ? '#/profile/' + state.user.username : n.href;
    const on = n.key === active ? ' active' : '';
    return `<a class="${itemCls}${on}" href="${href}" data-testid="nav-${n.key}">
      <span class="icon-pill"><span class="material-symbols-rounded">${n.icon}</span></span>${n.label}</a>`;
  }).join('');
}

function shell(active, topbarHTML, opts) {
  opts = opts || {};
  const app = document.getElementById('app');
  app.className = opts.wide ? 'wide' : '';
  app.innerHTML = `
    <div class="shell">
      <nav class="rail" data-testid="rail-nav">
        <div class="brand" title="Conflux">C</div>
        ${navHTML(active, 'rail', 'rail-item')}
        <div class="spacer"></div>
        <button class="icon-btn" data-act="toggle-theme" title="Toggle theme" data-testid="theme-toggle"><span class="material-symbols-rounded">dark_mode</span></button>
        <button class="icon-btn" data-act="logout" title="Log out" data-testid="logout-btn"><span class="material-symbols-rounded">logout</span></button>
      </nav>
      <div class="main">
        <header class="topbar">${topbarHTML}</header>
        <div class="content" id="view"></div>
      </div>
    </div>
    <nav class="navbar" data-testid="bottom-nav">${navHTML(active, 'nav', 'nav-item')}</nav>
    <button class="fab" data-act="compose" data-testid="fab-compose"><span class="material-symbols-rounded">edit</span>Post</button>`;
  return document.getElementById('view');
}

function emptyHTML(msg, icon) {
  return `<div class="empty"><span class="material-symbols-rounded">${icon || 'inbox'}</span><div>${esc(msg)}</div></div>`;
}
function spinnerHTML() { return '<div class="spinner"></div>'; }

/* ---------------- auth ---------------- */
async function renderAuth() {
  const app = document.getElementById('app');
  try { const b = await API.bootstrap(); state.needsAdmin = b.needs_admin; } catch (_) {}
  const isReg = state.authMode === 'register' || state.needsAdmin;
  app.className = '';
  app.innerHTML = `
    <div class="auth-wrap">
      <div class="auth-card">
        <div class="auth-brand">Conflux</div>
        <div class="auth-tag">Read the feeds. Join the conversation.</div>
        ${state.needsAdmin ? `<div class="auth-note" data-testid="admin-note"><span class="material-symbols-rounded">bolt</span>You're the first here — this account becomes the <b>&nbsp;admin</b>.</div>` : ''}
        <div class="form-error" id="auth-err" style="display:none"></div>
        <form id="auth-form">
          ${isReg ? `
            <div class="field"><label>Username</label><input id="f-username" autocomplete="username" placeholder="jane_doe" data-testid="username-input"/></div>
            <div class="field"><label>Display name</label><input id="f-display" placeholder="Jane Doe" data-testid="displayname-input"/></div>
            <div class="field"><label>Email</label><input id="f-email" type="email" autocomplete="email" placeholder="jane@example.com" data-testid="email-input"/></div>
            <div class="field"><label>Password</label><input id="f-password" type="password" autocomplete="new-password" placeholder="At least 6 characters" data-testid="password-input"/></div>
          ` : `
            <div class="field"><label>Email or username</label><input id="f-identifier" autocomplete="username" placeholder="jane@example.com" data-testid="identifier-input"/></div>
            <div class="field"><label>Password</label><input id="f-password" type="password" autocomplete="current-password" data-testid="password-input"/></div>
          `}
          <button type="submit" class="btn btn-filled" style="width:100%;height:48px" data-testid="auth-submit">
            ${state.needsAdmin ? 'Create admin account' : (isReg ? 'Create account' : 'Sign in')}
          </button>
        </form>
        ${state.needsAdmin ? '' : `<div class="auth-switch">
          ${isReg ? 'Already have an account?' : "Don't have an account?"}
          <button data-act="switch-auth" data-testid="switch-auth">${isReg ? 'Sign in' : 'Sign up'}</button>
        </div>`}
      </div>
    </div>`;
  document.getElementById('auth-form').addEventListener('submit', (e) => { e.preventDefault(); submitAuth(isReg); });
}

async function submitAuth(isReg) {
  const err = document.getElementById('auth-err');
  err.style.display = 'none';
  const $ = (id) => (document.getElementById(id) || {}).value || '';
  try {
    let res;
    if (isReg) {
      res = await API.register({ username: $('f-username'), display_name: $('f-display'), email: $('f-email'), password: $('f-password') });
    } else {
      res = await API.login({ identifier: $('f-identifier'), password: $('f-password') });
    }
    API.setToken(res.token);
    state.user = res.user;
    state.needsAdmin = false;
    toast('Welcome, ' + res.user.display_name + '!');
    go('#/');
  } catch (e) {
    err.textContent = e.message;
    err.style.display = 'block';
  }
}

/* ---------------- cards ---------------- */
function actionBar(tt, id, counts, myReaction) {
  const c = counts || {};
  const reacted = myReaction ? ' on' : '';
  const inner = myReaction
    ? `<span style="font-size:17px;line-height:1">${REACTIONS[myReaction] ? REACTIONS[myReaction].emoji : '👍'}</span>`
    : `<span class="material-symbols-rounded">add_reaction</span>`;
  return `<div class="actions" data-tt="${tt}" data-id="${id}" data-my="${myReaction || ''}">
    <button class="action${reacted}" data-act="react" data-testid="react-${tt}-${id}">${inner}<span>${c.reactions || 0}</span></button>
    <button class="action" data-act="comment" data-testid="comment-${tt}-${id}"><span class="material-symbols-rounded">mode_comment</span><span>${c.comments || 0}</span></button>
    <button class="action" data-act="repost" data-testid="repost-${tt}-${id}"><span class="material-symbols-rounded">repeat</span></button>
    <button class="action" data-act="quote" data-testid="quote-${tt}-${id}"><span class="material-symbols-rounded">format_quote</span></button>
    <button class="action" data-act="share" data-testid="share-${tt}-${id}"><span class="material-symbols-rounded">share</span><span>${c.shares || 0}</span></button>
  </div>`;
}

function embedHTML(ref) {
  if (!ref) return '<div class="embed"><div class="embed-pad" style="color:var(--md-on-surface-variant)">Original content unavailable</div></div>';
  if (ref.type === 'feed_item') {
    return `<div class="embed clickable" data-act="open" data-href="#/feed_item/${ref.id}">
      ${ref.image_url ? `<img class="embed-img" src="${esc(ref.image_url)}" alt="" onerror="this.remove()"/>` : ''}
      <div class="embed-pad">
        <div class="embed-source">${esc(ref.feed_title || 'Feed')}</div>
        <div class="embed-title">${esc(ref.title)}</div>
        <div class="embed-sum">${esc(stripTags(ref.summary))}</div>
      </div></div>`;
  }
  // post
  return `<div class="embed clickable" data-act="open" data-href="#/post/${ref.id}">
    <div class="embed-pad">
      <div class="post-head" style="margin-bottom:6px">${avatarHTML(ref.author, true)}
        <div class="post-meta"><div class="name">${esc(ref.author.display_name)}</div>
        <div class="handle">@${esc(ref.author.username)}</div></div></div>
      <div class="post-body">${esc(ref.body)}</div>
    </div></div>`;
}

function stripTags(html) {
  const d = document.createElement('div'); d.innerHTML = html || ''; return d.textContent || '';
}

function ownerMenu(canDelete, kind, refId) {
  if (!canDelete) return '';
  return `<button class="icon-btn" data-act="${kind}" data-refid="${refId}" title="Delete" data-testid="delete-${kind}-${refId}"><span class="material-symbols-rounded">delete</span></button>`;
}

function postCard(p) {
  const a = p.author;
  const canDelete = state.user && (state.user.id === a.id || state.user.is_admin);
  let kindChip = '';
  if (p.kind === 'repost') kindChip = `<div class="kind-chip"><span class="material-symbols-rounded">repeat</span>${esc(a.display_name)} reposted</div>`;
  if (p.kind === 'quote') kindChip = `<div class="kind-chip"><span class="material-symbols-rounded">format_quote</span>Quoted</div>`;
  const body = (p.kind !== 'repost' && p.body) ? `<div class="post-body">${esc(p.body)}</div>` : '';
  const embed = (p.kind === 'repost' || p.kind === 'quote') ? embedHTML(p.ref) : '';
  return `<div class="card interactive" data-testid="post-card-${p.id}">
    ${kindChip}
    <div class="post-head">
      ${avatarHTML(a)}
      <div class="post-meta grow">
        <div class="name" data-act="open" data-href="#/profile/${esc(a.username)}">${esc(a.display_name)}${a.is_admin ? '<span class="badge-admin">ADMIN</span>' : ''}</div>
        <div class="handle">@${esc(a.username)} · ${timeAgo(p.created_at)}</div>
      </div>
      ${ownerMenu(canDelete, 'delete-post', p.id)}
    </div>
    ${body}${embed}
    ${actionBar('post', p.id, p.counts, p.my_reaction)}
  </div>`;
}

function itemCard(it) {
  return `<div class="card interactive" data-testid="item-card-${it.id}">
    <div class="kind-chip"><span class="material-symbols-rounded">rss_feed</span>${esc(it.feed_title || 'Feed')}${it.author ? ' · ' + esc(it.author) : ''} · ${timeAgo(it.published_at)}</div>
    <div class="item-title" data-act="open" data-href="#/feed_item/${it.id}">${esc(it.title || 'Untitled')}</div>
    ${it.image_url ? `<img class="item-thumb" src="${esc(it.image_url)}" alt="" onerror="this.remove()"/>` : ''}
    <div class="item-sum">${esc(stripTags(it.summary || it.content))}</div>
    ${actionBar('feed_item', it.id, it.counts, it.my_reaction)}
  </div>`;
}

/* ---------------- views ---------------- */
async function viewHome() {
  const filter = window._homeFilter || 'all';
  const v = shell('home', `<div><h1>Home</h1><div class="sub">Your social timeline</div></div>`);
  v.innerHTML = `
    <div class="composer">
      <div style="display:flex;gap:10px">${avatarHTML(state.user)}
        <textarea id="home-composer" rows="2" placeholder="Share something with the community…" data-testid="home-composer"></textarea></div>
      <div class="composer-foot"><span class="count" id="home-count">0 / 5000</span>
        <button class="btn btn-filled btn-sm" id="home-post" data-testid="home-post-btn">Post</button></div>
    </div>
    <div class="tabs">
      <button class="chip ${filter === 'all' ? 'active' : ''}" data-act="home-filter" data-f="all" data-testid="tab-foryou">For you</button>
      <button class="chip ${filter === 'following' ? 'active' : ''}" data-act="home-filter" data-f="following" data-testid="tab-following">Following</button>
    </div>
    <div id="feed"></div>`;
  const ta = v.querySelector('#home-composer');
  ta.addEventListener('input', () => v.querySelector('#home-count').textContent = ta.value.length + ' / 5000');
  v.querySelector('#home-post').addEventListener('click', async () => {
    const b = ta.value.trim(); if (!b) { toast('Write something first'); return; }
    try { await API.createPost(b); toast('Posted'); render(); } catch (e) { toast(e.message); }
  });
  paginate(v.querySelector('#feed'), postCard, (page) => API.timeline(filter, page), 'Say something or follow people to fill your timeline', 'forum');
}

async function viewExplore() {
  const filter = window._exFilter || 'all';
  const v = shell('explore', `<div><h1>Explore</h1><div class="sub">Latest from every feed</div></div><div class="grow"></div>
    <button class="btn btn-tonal btn-sm" data-act="manage-feeds" data-testid="manage-feeds-btn"><span class="material-symbols-rounded">tune</span>Manage feeds</button>`);
  v.innerHTML = `
    <div class="addfeed">
      <input id="feed-url" placeholder="Paste an RSS / Atom feed URL…" data-testid="feed-url-input"/>
      <button class="btn btn-filled" id="add-feed" data-testid="add-feed-btn"><span class="material-symbols-rounded">add</span>Add</button>
    </div>
    <div class="tabs">
      <button class="chip ${filter === 'all' ? 'active' : ''}" data-act="ex-filter" data-f="all" data-testid="tab-all-items">All items</button>
      <button class="chip ${filter === 'subscribed' ? 'active' : ''}" data-act="ex-filter" data-f="subscribed" data-testid="tab-subscribed">Subscribed</button>
    </div>
    <div id="feed"></div>`;
  const input = v.querySelector('#feed-url');
  const addBtn = v.querySelector('#add-feed');
  const doAdd = async () => {
    const url = input.value.trim(); if (!url) { toast('Enter a feed URL'); return; }
    addBtn.disabled = true; addBtn.innerHTML = '<span class="material-symbols-rounded">hourglass_top</span>Adding';
    try { const f = await API.addFeed(url); toast('Subscribed to ' + (f.title || 'feed')); input.value = ''; render(); }
    catch (e) { toast(e.message); addBtn.disabled = false; addBtn.innerHTML = '<span class="material-symbols-rounded">add</span>Add'; }
  };
  addBtn.addEventListener('click', doAdd);
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') doAdd(); });
  paginate(v.querySelector('#feed'), itemCard, (page) => API.items(filter, page),
    filter === 'subscribed' ? 'Subscribe to feeds to see items here' : 'No feed items yet — add a feed above', 'rss_feed');
}

async function viewPeople() {
  const v = shell('people', `<div><h1>People</h1><div class="sub">Discover and follow members</div></div>`);
  v.innerHTML = spinnerHTML();
  const users = await API.users();
  if (!users.length) { v.innerHTML = emptyHTML('No members yet', 'groups'); return; }
  v.innerHTML = users.map(u => `
    <div class="card">
      <div class="post-head" style="margin:0">
        ${avatarHTML(u)}
        <div class="post-meta grow">
          <div class="name" data-act="open" data-href="#/profile/${esc(u.username)}">${esc(u.display_name)}${u.is_admin ? '<span class="badge-admin">ADMIN</span>' : ''}</div>
          <div class="handle">@${esc(u.username)}</div>
          ${u.bio ? `<div class="handle" style="margin-top:4px">${esc(u.bio)}</div>` : ''}
        </div>
        ${u.is_self ? '' : followBtn(u)}
      </div>
    </div>`).join('');
}

function followBtn(u) {
  if (u.is_following) return `<button class="btn btn-tonal btn-sm" data-act="unfollow" data-uid="${u.id}" data-testid="unfollow-${u.id}"><span class="material-symbols-rounded">done</span>Following</button>`;
  return `<button class="btn btn-filled btn-sm" data-act="follow" data-uid="${u.id}" data-testid="follow-${u.id}"><span class="material-symbols-rounded">person_add</span>Follow</button>`;
}

async function viewProfile(username) {
  const v = shell('profile', `<div><h1>Profile</h1><div class="sub">@${esc(username)}</div></div>`);
  v.innerHTML = spinnerHTML();
  let p;
  try { p = await API.profile(username); } catch (e) { v.innerHTML = emptyHTML('User not found', 'person_off'); return; }
  v.innerHTML = `
    <div class="card">
      <div class="profile-head">
        ${avatarHTML(p)}
        <div class="grow">
          <div style="font-family:var(--font-display);font-size:24px;font-weight:800">${esc(p.display_name)}${p.is_admin ? '<span class="badge-admin">ADMIN</span>' : ''}</div>
          <div class="handle">@${esc(p.username)}</div>
        </div>
        ${p.is_self
          ? `<button class="btn btn-outline btn-sm" data-act="edit-profile" data-testid="edit-profile-btn">Edit profile</button>`
          : followBtn(Object.assign({}, p))}
      </div>
      ${p.bio ? `<div class="post-body" style="margin-top:6px">${esc(p.bio)}</div>` : ''}
      <div class="profile-stats">
        <div><b>${p.post_count}</b> <span>Posts</span></div>
        <div><b>${p.follower_count}</b> <span>Followers</span></div>
        <div><b>${p.following_count}</b> <span>Following</span></div>
      </div>
    </div>
    <div class="section-title">Posts</div>
    <div id="uposts"></div>`;
  paginate(v.querySelector('#uposts'), postCard, (page) => API.userPosts(username, page), 'No posts yet', 'edit_note');
}

async function viewDetail(tt, id) {
  const v = shell(tt === 'feed_item' ? 'explore' : 'home',
    `<button class="icon-btn back-btn" data-act="back" data-testid="back-btn"><span class="material-symbols-rounded">arrow_back</span></button>
     <div><h1>${tt === 'feed_item' ? 'Article' : 'Post'}</h1></div>`);
  v.innerHTML = spinnerHTML();
  let obj;
  try { obj = tt === 'feed_item' ? await API.item(id) : await API.post_(id); }
  catch (e) { v.innerHTML = emptyHTML('Not found', 'error'); return; }

  let head;
  if (tt === 'feed_item') {
    head = `<div class="card">
      <div class="kind-chip"><span class="material-symbols-rounded">rss_feed</span>${esc(obj.feed_title || 'Feed')}${obj.author ? ' · ' + esc(obj.author) : ''} · ${timeAgo(obj.published_at)}</div>
      <div class="item-title" style="cursor:default">${esc(obj.title || 'Untitled')}</div>
      ${obj.image_url ? `<img class="item-thumb" src="${esc(obj.image_url)}" alt="" onerror="this.remove()"/>` : ''}
      ${obj.link ? `<a class="btn btn-text btn-sm" href="${esc(obj.link)}" target="_blank" rel="noopener" style="padding-left:0" data-testid="read-original"><span class="material-symbols-rounded">open_in_new</span>Read original</a>` : ''}
      <div class="article-content">${sanitize(obj.content || obj.summary)}</div>
      ${actionBar('feed_item', obj.id, obj.counts, obj.my_reaction)}
    </div>`;
  } else {
    head = postCard(obj);
  }
  v.innerHTML = `${head}
    <div class="section-title">Comments</div>
    <div class="composer">
      <div style="display:flex;gap:10px">${avatarHTML(state.user)}
        <textarea id="c-input" rows="2" placeholder="Add a comment…" data-testid="comment-input"></textarea></div>
      <div class="composer-foot"><span class="count" id="c-parent-lbl"></span>
        <button class="btn btn-filled btn-sm" id="c-post" data-testid="comment-submit">Comment</button></div>
    </div>
    <div id="comments"></div>`;

  let parentId = null;
  const cinput = v.querySelector('#c-input');
  const plbl = v.querySelector('#c-parent-lbl');
  v._setReply = (pid, name) => {
    parentId = pid; plbl.innerHTML = pid ? `Replying to <b>${esc(name)}</b> · <button class="btn btn-text btn-sm" style="height:auto;padding:0" id="cancel-reply">cancel</button>` : '';
    if (pid) { cinput.focus(); const cr = v.querySelector('#cancel-reply'); if (cr) cr.addEventListener('click', () => v._setReply(null)); }
  };
  v.querySelector('#c-post').addEventListener('click', async () => {
    const b = cinput.value.trim(); if (!b) { toast('Write a comment'); return; }
    try {
      await API.addComment({ target_type: tt, target_id: Number(id), parent_id: parentId, body: b });
      cinput.value = ''; v._setReply(null); loadComments(tt, id, v.querySelector('#comments'), v);
      toast('Comment added');
    } catch (e) { toast(e.message); }
  });
  loadComments(tt, id, v.querySelector('#comments'), v);
}

async function loadComments(tt, id, container, view) {
  container.innerHTML = spinnerHTML();
  const rows = await API.comments(tt, id);
  if (!rows.length) { container.innerHTML = emptyHTML('No comments yet — be the first', 'chat_bubble'); return; }
  const top = rows.filter(r => !r.parent_id);
  const kids = {};
  rows.forEach(r => { if (r.parent_id) { (kids[r.parent_id] = kids[r.parent_id] || []).push(r); } });
  const render1 = (c, child) => {
    const a = c.author;
    const canDel = state.user && (state.user.id === a.id || state.user.is_admin);
    const liked = c.my_reaction ? ' on like' : '';
    return `<div class="comment ${child ? 'child' : ''}" data-testid="comment-${c.id}">
      ${avatarHTML(a, true)}
      <div class="c-body">
        <div><span class="c-name" data-act="open" data-href="#/profile/${esc(a.username)}">${esc(a.display_name)}</span><span class="c-time">${timeAgo(c.created_at)}</span></div>
        <div class="c-text">${esc(c.body)}</div>
        <div class="c-actions">
          <button class="action${liked}" data-act="like-comment" data-cid="${c.id}" data-my="${c.my_reaction || ''}" data-testid="like-comment-${c.id}"><span class="material-symbols-rounded">favorite</span><span>${(c.counts && c.counts.reactions) || 0}</span></button>
          ${child ? '' : `<button class="action" data-act="reply-comment" data-cid="${c.id}" data-name="${esc(a.display_name)}" data-testid="reply-comment-${c.id}"><span class="material-symbols-rounded">reply</span>Reply</button>`}
          ${canDel ? `<button class="action" data-act="delete-comment" data-cid="${c.id}" data-testid="delete-comment-${c.id}"><span class="material-symbols-rounded">delete</span></button>` : ''}
        </div>
      </div></div>`;
  };
  container.innerHTML = top.map(c => render1(c, false) + (kids[c.id] || []).map(k => render1(k, true)).join('')).join('');
  container._ctx = { tt, id, view };
}

/* ---------------- pagination ---------------- */
async function paginate(container, cardFn, fetchFn, emptyMsg, emptyIcon) {
  let page = 1;
  container.innerHTML = spinnerHTML();
  const list = document.createElement('div');
  const more = document.createElement('button');
  more.className = 'btn btn-tonal loadmore';
  more.innerHTML = 'Load more';
  more.setAttribute('data-testid', 'load-more');
  async function load() {
    more.disabled = true;
    let data;
    try { data = await fetchFn(page); } catch (e) { toast(e.message); more.disabled = false; return; }
    data = data || [];
    if (page === 1) {
      container.innerHTML = '';
      container.appendChild(list);
      container.appendChild(more);
      if (!data.length) { container.innerHTML = emptyHTML(emptyMsg, emptyIcon); return; }
    }
    data.forEach(x => list.insertAdjacentHTML('beforeend', cardFn(x)));
    more.disabled = false;
    more.style.display = data.length < 20 ? 'none' : 'block';
    page++;
  }
  more.addEventListener('click', load);
  await load();
}

/* ---------------- global action handler ---------------- */
async function onGlobalAction(e) {
  const el = e.target.closest('[data-act]');
  if (!el) return;
  const act = el.dataset.act;

  if (act === 'open') { e.preventDefault(); go(el.dataset.href); return; }
  if (act === 'back') { history.length > 1 ? history.back() : go('#/'); return; }
  if (act === 'logout') { API.setToken(null); state.user = null; go('#/'); toast('Logged out'); return; }
  if (act === 'toggle-theme') { toggleTheme(); return; }
  if (act === 'switch-auth') { state.authMode = state.authMode === 'login' ? 'register' : 'login'; renderAuth(); return; }
  if (act === 'compose') { openCompose(); return; }
  if (act === 'manage-feeds') { openFeeds(); return; }
  if (act === 'edit-profile') { openEditProfile(); return; }

  if (act === 'home-filter') { window._homeFilter = el.dataset.f; viewHome(); return; }
  if (act === 'ex-filter') { window._exFilter = el.dataset.f; viewExplore(); return; }

  // social actions on cards
  if (['react', 'comment', 'repost', 'quote', 'share'].includes(act)) {
    const bar = el.closest('.actions');
    const tt = bar.dataset.tt, id = Number(bar.dataset.id), my = bar.dataset.my;
    return cardAction(act, tt, id, my, el);
  }

  if (act === 'follow' || act === 'unfollow') {
    const uid = Number(el.dataset.uid);
    try { await (act === 'follow' ? API.follow(uid) : API.unfollow(uid)); render(); } catch (err) { toast(err.message); }
    return;
  }
  if (act === 'delete-post') {
    confirmDialog('Delete this post?', async () => { await API.deletePost(Number(el.dataset.refid)); toast('Deleted'); go('#/'); });
    return;
  }
  if (act === 'like-comment') {
    const cid = Number(el.dataset.cid), my = el.dataset.my;
    try {
      if (my) await API.unreact('comment', cid); else await API.react('comment', cid, 'like');
      const c = el.closest('#comments'); if (c && c._ctx) loadComments(c._ctx.tt, c._ctx.id, c, c._ctx.view);
    } catch (err) { toast(err.message); }
    return;
  }
  if (act === 'reply-comment') {
    const c = el.closest('#comments'); if (c && c._ctx && c._ctx.view._setReply) c._ctx.view._setReply(Number(el.dataset.cid), el.dataset.name);
    return;
  }
  if (act === 'delete-comment') {
    const cid = Number(el.dataset.cid);
    confirmDialog('Delete this comment?', async () => {
      await API.deleteComment(cid); const c = el.closest('#comments'); closeDialog();
      if (c && c._ctx) loadComments(c._ctx.tt, c._ctx.id, c, c._ctx.view); toast('Deleted');
    });
    return;
  }
  // feed management (inside dialog)
  if (act === 'sub-feed') { await API.subscribe(Number(el.dataset.fid)); refreshFeedsDialog(); return; }
  if (act === 'unsub-feed') { await API.unsubscribe(Number(el.dataset.fid)); refreshFeedsDialog(); return; }
  if (act === 'refresh-feed') { toast('Refreshing…'); try { await API.refreshFeed(Number(el.dataset.fid)); toast('Feed refreshed'); refreshFeedsDialog(); } catch (err) { toast(err.message); } return; }
  if (act === 'delete-feed') { confirmDialog('Delete this feed for everyone?', async () => { await API.deleteFeed(Number(el.dataset.fid)); closeDialog(); openFeeds(); toast('Feed deleted'); }); return; }
}

async function cardAction(act, tt, id, my, btn) {
  if (act === 'react') {
    reactionPopover(btn, async (r) => {
      try {
        if (r === my) await API.unreact(tt, id); else await API.react(tt, id, r);
        render();
      } catch (e) { toast(e.message); }
    });
    return;
  }
  if (act === 'comment') { go('#/' + tt + '/' + id); return; }
  if (act === 'repost') {
    confirmDialog('Repost this to your followers?', async () => { await API.repost(tt, id); closeDialog(); toast('Reposted'); go('#/'); });
    return;
  }
  if (act === 'quote') { openQuote(tt, id); return; }
  if (act === 'share') {
    try {
      const res = await API.share(tt, id);
      const url = location.origin + '/#/' + tt + '/' + id;
      if (navigator.clipboard) { await navigator.clipboard.writeText(url); toast('Link copied to clipboard'); }
      else toast('Share link: ' + url);
      render();
    } catch (e) { toast(e.message); }
    return;
  }
}

/* ---------------- dialogs ---------------- */
function confirmDialog(title, onYes) {
  dialog({ title, confirmText: 'Confirm', onConfirm: async () => { try { await onYes(); } catch (e) { toast(e.message); } } });
}

function openCompose() {
  dialog({
    title: 'New post', confirmText: 'Post',
    bodyHTML: `<div class="field"><textarea id="cmp" rows="5" placeholder="What's happening?" data-testid="compose-input"></textarea></div>`,
    onConfirm: async (layer) => {
      const b = layer.querySelector('#cmp').value.trim(); if (!b) { toast('Write something first'); return; }
      try { await API.createPost(b); closeDialog(); toast('Posted'); go('#/'); } catch (e) { toast(e.message); }
    },
  });
}

function openQuote(tt, id) {
  dialog({
    title: 'Quote', confirmText: 'Post quote',
    bodyHTML: `<div class="field"><textarea id="qbody" rows="4" placeholder="Add your thoughts…" data-testid="quote-input"></textarea></div>`,
    onConfirm: async (layer) => {
      const b = layer.querySelector('#qbody').value.trim(); if (!b) { toast('Add a comment to quote'); return; }
      try { await API.quote(tt, id, b); closeDialog(); toast('Quoted'); go('#/'); } catch (e) { toast(e.message); }
    },
  });
}

function openEditProfile() {
  const u = state.user;
  dialog({
    title: 'Edit profile', confirmText: 'Save',
    bodyHTML: `
      <div class="field"><label>Display name</label><input id="ep-name" value="${esc(u.display_name)}" data-testid="edit-name"/></div>
      <div class="field"><label>Bio</label><textarea id="ep-bio" rows="3" data-testid="edit-bio">${esc(u.bio || '')}</textarea></div>
      <div class="field"><label>Avatar image URL</label><input id="ep-avatar" value="${esc(u.avatar_url || '')}" placeholder="https://…" data-testid="edit-avatar"/></div>`,
    onConfirm: async (layer) => {
      try {
        state.user = await API.updateProfile({
          display_name: layer.querySelector('#ep-name').value,
          bio: layer.querySelector('#ep-bio').value,
          avatar_url: layer.querySelector('#ep-avatar').value,
        });
        closeDialog(); toast('Profile updated'); render();
      } catch (e) { toast(e.message); }
    },
  });
}

async function openFeeds() {
  dialog({ title: 'Manage feeds', bodyHTML: '<div id="feeds-body">' + spinnerHTML() + '</div>', cancelText: 'Close' });
  refreshFeedsDialog();
}

async function refreshFeedsDialog() {
  const body = document.getElementById('feeds-body');
  if (!body) return;
  const feeds = await API.feeds();
  if (!feeds.length) { body.innerHTML = emptyHTML('No feeds yet', 'rss_feed'); return; }
  body.innerHTML = feeds.map(f => `
    <div class="list-row" data-testid="feed-row-${f.id}">
      <div class="grow">
        <div class="title">${esc(f.title || f.feed_url)}</div>
        <div class="desc">${f.item_count} items · ${f.subscriber_count} subscribers${f.fetch_error ? ' · ⚠ error' : ''}</div>
      </div>
      <button class="icon-btn" data-act="refresh-feed" data-fid="${f.id}" title="Refresh" data-testid="refresh-feed-${f.id}"><span class="material-symbols-rounded">refresh</span></button>
      ${f.subscribed
        ? `<button class="btn btn-tonal btn-sm" data-act="unsub-feed" data-fid="${f.id}" data-testid="unsub-${f.id}">Unsubscribe</button>`
        : `<button class="btn btn-filled btn-sm" data-act="sub-feed" data-fid="${f.id}" data-testid="sub-${f.id}">Subscribe</button>`}
      ${state.user.is_admin ? `<button class="icon-btn" data-act="delete-feed" data-fid="${f.id}" title="Delete" data-testid="delete-feed-${f.id}"><span class="material-symbols-rounded">delete</span></button>` : ''}
    </div>`).join('');
}

/* ---------------- theme ---------------- */
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme');
  const dark = cur === 'dark' || (!cur && matchMedia('(prefers-color-scheme: dark)').matches);
  const next = dark ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('fs_theme', next);
  state.theme = next;
}

boot();
