/* Conflux UI helpers: escaping, time, sanitize, snackbar, dialog, ripple, reactions. */

const REACTIONS = {
  like: { emoji: '👍', label: 'Like' },
  love: { emoji: '❤️', label: 'Love' },
  celebrate: { emoji: '🎉', label: 'Celebrate' },
  insightful: { emoji: '💡', label: 'Insightful' },
  laugh: { emoji: '😂', label: 'Laugh' },
};

function esc(s) {
  if (s === null || s === undefined) return '';
  return String(s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function initials(name) {
  if (!name) return '?';
  const parts = name.trim().split(/\s+/);
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

function timeAgo(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  const s = Math.floor((Date.now() - d.getTime()) / 1000);
  if (isNaN(s)) return '';
  if (s < 60) return 'just now';
  if (s < 3600) return Math.floor(s / 60) + 'm';
  if (s < 86400) return Math.floor(s / 3600) + 'h';
  if (s < 604800) return Math.floor(s / 86400) + 'd';
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/* Very small HTML sanitizer for feed article content. */
function sanitize(html) {
  if (!html) return '';
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.querySelectorAll('script, style, iframe, object, embed, link, meta, form, input').forEach(n => n.remove());
  doc.querySelectorAll('*').forEach(el => {
    [...el.attributes].forEach(a => {
      const n = a.name.toLowerCase();
      if (n.startsWith('on')) el.removeAttribute(a.name);
      if (n === 'href' || n === 'src') {
        const val = a.value.trim().toLowerCase();
        if (val.startsWith('javascript:') || val.startsWith('data:') || val.startsWith('vbscript:')) {
          el.removeAttribute(a.name);
        }
      }
    });
    if (el.tagName === 'A') { el.setAttribute('target', '_blank'); el.setAttribute('rel', 'noopener noreferrer'); }
  });
  return doc.body.innerHTML;
}

/* ---- Minimal, safe Markdown renderer (subset) ----
   Escapes all HTML first, then applies a limited markdown subset, then runs
   the result through sanitize() as defense-in-depth. XSS-safe by construction. */
function mdInline(line) {
  let t = esc(line);
  // inline code (protect contents from other transforms)
  const codes = [];
  t = t.replace(/`([^`]+)`/g, (_m, c) => { codes.push(c); return '\u0000C' + (codes.length - 1) + '\u0000'; });
  // links [text](http(s)://... or /relative)
  t = t.replace(/\[([^\]]+)\]\(((?:https?:\/\/|\/)[^\s)]+)\)/g,
    (_m, text, url) => `<a href="${url}" target="_blank" rel="noopener noreferrer">${text}</a>`);
  // bold then italic
  t = t.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  t = t.replace(/__([^_]+)__/g, '<strong>$1</strong>');
  t = t.replace(/(^|[^\w*])\*([^*\n]+)\*(?!\w)/g, '$1<em>$2</em>');
  t = t.replace(/(^|[^\w_])_([^_\n]+)_(?!\w)/g, '$1<em>$2</em>');
  // strikethrough
  t = t.replace(/~~([^~]+)~~/g, '<del>$1</del>');
  // restore code
  t = t.replace(/\u0000C(\d+)\u0000/g, (_m, i) => `<code>${codes[i]}</code>`);
  return t;
}

function renderMarkdown(src) {
  if (!src) return '';
  const lines = String(src).split(/\r?\n/);
  let html = '', i = 0;
  const isBlockStart = l =>
    /^```/.test(l.trim()) || /^\s*(#{1,3})\s+/.test(l) || /^\s*[-*+]\s+/.test(l) ||
    /^\s*\d+\.\s+/.test(l) || /^\s*>\s?/.test(l) || /^\s*(---|\*\*\*|___)\s*$/.test(l);
  while (i < lines.length) {
    const line = lines[i];
    if (/^```/.test(line.trim())) {
      const buf = []; i++;
      while (i < lines.length && !/^```/.test(lines[i].trim())) { buf.push(lines[i]); i++; }
      i++;
      html += `<pre class="md-pre"><code>${esc(buf.join('\n'))}</code></pre>`;
      continue;
    }
    if (/^\s*(---|\*\*\*|___)\s*$/.test(line)) { html += '<hr/>'; i++; continue; }
    const hm = line.match(/^\s*(#{1,3})\s+(.*)$/);
    if (hm) { const lvl = Math.min(hm[1].length + 3, 6); html += `<h${lvl}>${mdInline(hm[2])}</h${lvl}>`; i++; continue; }
    if (/^\s*>\s?/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s*>\s?/.test(lines[i])) { buf.push(lines[i].replace(/^\s*>\s?/, '')); i++; }
      html += `<blockquote>${buf.map(mdInline).join('<br>')}</blockquote>`;
      continue;
    }
    if (/^\s*[-*+]\s+/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s*[-*+]\s+/.test(lines[i])) { buf.push(lines[i].replace(/^\s*[-*+]\s+/, '')); i++; }
      html += '<ul>' + buf.map(x => `<li>${mdInline(x)}</li>`).join('') + '</ul>';
      continue;
    }
    if (/^\s*\d+\.\s+/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s*\d+\.\s+/.test(lines[i])) { buf.push(lines[i].replace(/^\s*\d+\.\s+/, '')); i++; }
      html += '<ol>' + buf.map(x => `<li>${mdInline(x)}</li>`).join('') + '</ol>';
      continue;
    }
    if (line.trim() === '') { i++; continue; }
    const buf = [];
    while (i < lines.length && lines[i].trim() !== '' && !isBlockStart(lines[i])) { buf.push(lines[i]); i++; }
    html += `<p>${buf.map(mdInline).join('<br>')}</p>`;
  }
  return sanitize(html);
}


function avatarHTML(user, sm) {
  const cls = 'avatar' + (sm ? ' sm' : '');
  if (user && user.avatar_url) return `<div class="${cls}"><img src="${esc(user.avatar_url)}" alt=""/></div>`;
  return `<div class="${cls}">${esc(initials(user ? user.display_name || user.username : '?'))}</div>`;
}

let snackTimer;
function toast(msg) {
  const el = document.getElementById('snackbar');
  el.textContent = msg;
  el.classList.add('show');
  clearTimeout(snackTimer);
  snackTimer = setTimeout(() => el.classList.remove('show'), 3200);
}

function closeDialog() { document.getElementById('dialog-layer').innerHTML = ''; }

/* dialog({title, bodyHTML, confirmText, onConfirm, onRender}) */
function dialog(opts) {
  const layer = document.getElementById('dialog-layer');
  layer.innerHTML = `
    <div class="scrim" data-close="1">
      <div class="dialog" role="dialog" aria-modal="true" data-testid="dialog">
        <h3>${esc(opts.title)}</h3>
        <div class="dialog-body">${opts.bodyHTML || ''}</div>
        <div class="dialog-actions">
          <button class="btn btn-text" data-close="1" data-testid="dialog-cancel">${esc(opts.cancelText || 'Cancel')}</button>
          ${opts.confirmText ? `<button class="btn btn-filled" data-testid="dialog-confirm" id="dlg-confirm">${esc(opts.confirmText)}</button>` : ''}
        </div>
      </div>
    </div>`;
  layer.querySelector('.scrim').addEventListener('click', (e) => { if (e.target.dataset.close) closeDialog(); });
  layer.querySelectorAll('[data-close]').forEach(b => { if (b.tagName === 'BUTTON') b.addEventListener('click', closeDialog); });
  const c = layer.querySelector('#dlg-confirm');
  if (c && opts.onConfirm) c.addEventListener('click', () => opts.onConfirm(layer));
  if (opts.onRender) opts.onRender(layer);
}

function closePopover() { document.getElementById('popover-layer').innerHTML = ''; }

/* reaction popover anchored to a button element */
function reactionPopover(anchor, onPick) {
  const layer = document.getElementById('popover-layer');
  const r = anchor.getBoundingClientRect();
  const pop = document.createElement('div');
  pop.className = 'reaction-pop';
  pop.setAttribute('data-testid', 'reaction-popover');
  pop.innerHTML = Object.keys(REACTIONS).map(k =>
    `<button data-r="${k}" title="${REACTIONS[k].label}" data-testid="reaction-${k}">${REACTIONS[k].emoji}</button>`).join('');
  layer.innerHTML = '';
  layer.appendChild(pop);
  const top = r.top + window.scrollY - 52;
  pop.style.left = Math.max(8, r.left + window.scrollX) + 'px';
  pop.style.top = top + 'px';
  pop.querySelectorAll('button').forEach(b =>
    b.addEventListener('click', (e) => { e.stopPropagation(); closePopover(); onPick(b.dataset.r); }));
  setTimeout(() => document.addEventListener('click', closePopover, { once: true }), 0);
}

/* ripple on click for .btn and .action */
document.addEventListener('pointerdown', (e) => {
  const t = e.target.closest('.btn, .fab, .rail-item, .nav-item');
  if (!t) return;
  const rect = t.getBoundingClientRect();
  const rip = document.createElement('span');
  const size = Math.max(rect.width, rect.height);
  Object.assign(rip.style, {
    position: 'absolute', borderRadius: '50%', pointerEvents: 'none',
    width: size + 'px', height: size + 'px',
    left: (e.clientX - rect.left - size / 2) + 'px',
    top: (e.clientY - rect.top - size / 2) + 'px',
    background: 'currentColor', opacity: '.18', transform: 'scale(0)',
    transition: 'transform .4s ease, opacity .6s ease',
  });
  const prevPos = getComputedStyle(t).position;
  if (prevPos === 'static') t.style.position = 'relative';
  t.appendChild(rip);
  requestAnimationFrame(() => { rip.style.transform = 'scale(2.2)'; rip.style.opacity = '0'; });
  setTimeout(() => rip.remove(), 600);
});
