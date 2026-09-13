// LedgerWallet operations console — vanilla JS, no build step (static, Vercel-friendly).
'use strict';

const $ = (id) => document.getElementById(id);
const state = {
  apiBase: localStorage.getItem('lw_api') || '',
  token: localStorage.getItem('lw_token') || '',
  username: localStorage.getItem('lw_user') || '',
  walletId: localStorage.getItem('lw_wallet') || '',
};

function setConn(ok, text) {
  const el = $('conn');
  el.textContent = text;
  el.className = 'pill ' + (ok === null ? 'pill-idle' : ok ? 'pill-ok' : 'pill-err');
}

function paise(n) {
  const v = Number(n);
  if (!isFinite(v)) return '—';
  return `₹${(v / 100).toLocaleString('en-IN', { minimumFractionDigits: 2 })} (${v} p)`;
}

// Compact rupee formatting for very large values (e.g. the conservation gauge incl. treasury).
function moneyCompact(paiseVal) {
  const r = Number(paiseVal) / 100;
  if (!isFinite(r)) return '—';
  const abs = Math.abs(r);
  if (abs >= 1e7) return '₹' + (r / 1e7).toLocaleString('en-IN', { maximumFractionDigits: 2 }) + ' Cr';
  if (abs >= 1e5) return '₹' + (r / 1e5).toLocaleString('en-IN', { maximumFractionDigits: 2 }) + ' L';
  return '₹' + r.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

async function api(method, path, body, auth = true) {
  if (!state.apiBase) throw new Error('Set the API base URL first');
  const headers = { 'Content-Type': 'application/json' };
  if (auth && state.token) headers['Authorization'] = 'Bearer ' + state.token;
  const res = await fetch(state.apiBase.replace(/\/$/, '') + path, {
    method, headers, body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data; try { data = JSON.parse(text); } catch { data = { raw: text }; }
  return { ok: res.ok, status: res.status, data };
}

function refreshEnabled() {
  const hasToken = !!state.token;
  const hasWallet = !!state.walletId;
  $('walletBtn').disabled = !hasToken;
  $('refreshWalletBtn').disabled = !hasWallet;
  $('topupBtn').disabled = !hasWallet;
  $('transferBtn').disabled = !hasWallet;
  $('statusBtn').disabled = !hasToken;
  $('reverseBtn').disabled = !hasToken;
}

function showUser() {
  if (!state.token) return;
  $('activeUser').classList.remove('hidden');
  $('auName').textContent = state.username || '—';
  $('auToken').textContent = state.token;
}

function showWallet(w) {
  if (!w) return;
  state.walletId = w.id;
  localStorage.setItem('lw_wallet', w.id);
  $('walletBox').classList.remove('hidden');
  $('wId').textContent = w.id;
  $('wBal').textContent = paise(w.balance_paise);
  refreshEnabled();
}

function out(el, obj, ok) {
  el.classList.remove('hidden');
  el.textContent = typeof obj === 'string' ? obj : JSON.stringify(obj, null, 2);
  el.style.borderColor = ok ? 'rgba(52,211,153,.5)' : 'rgba(248,113,113,.5)';
}

// --- helpers: button loading state + wallet loaders ---
async function busy(btn, fn) {
  if (!btn) return fn();
  const prevText = btn.textContent, prevDisabled = btn.disabled;
  btn.disabled = true;
  btn.classList.add('loading');
  btn.textContent = '⏳ …';
  try { return await fn(); }
  finally { btn.textContent = prevText; btn.disabled = prevDisabled; btn.classList.remove('loading'); }
}

async function loadWallet() {
  const { ok, data } = await api('POST', '/wallets');
  if (ok) showWallet(data); else alert('Wallet failed: ' + JSON.stringify(data));
}

async function refreshWallet() {
  if (!state.walletId) return;
  const { ok, data } = await api('GET', '/wallets/' + state.walletId);
  if (ok) showWallet(data);
}

// --- wiring ---
$('saveApi').onclick = () => busy($('saveApi'), async () => {
  state.apiBase = $('apiBase').value.trim();
  localStorage.setItem('lw_api', state.apiBase);
  try {
    const r = await fetch(state.apiBase.replace(/\/$/, '') + '/healthz');
    setConn(r.ok, r.ok ? 'connected' : 'unreachable');
  } catch { setConn(false, 'unreachable'); }
});

$('registerBtn').onclick = () => busy($('registerBtn'), async () => {
  const username = $('username').value.trim();
  if (!username) return;
  try {
    const { ok, data } = await api('POST', '/auth/register', { username }, false);
    if (!ok) throw new Error(JSON.stringify(data));
    state.token = data.token; state.username = data.username;
    localStorage.setItem('lw_token', state.token);
    localStorage.setItem('lw_user', state.username);
    showUser(); refreshEnabled();
    await loadWallet(); // auto-load the active user's wallet so the panel is never stale
  } catch (e) { alert('Register failed: ' + e.message); }
});

$('walletBtn').onclick = () => busy($('walletBtn'), loadWallet);

$('refreshWalletBtn').onclick = () => busy($('refreshWalletBtn'), refreshWallet);

$('topupBtn').onclick = () => busy($('topupBtn'), async () => {
  const amt = parseInt($('topupAmt').value, 10);
  if (!(amt > 0)) return alert('Enter a positive paise amount');
  const { ok, data } = await api('POST', `/wallets/${state.walletId}/topup`, { amount_paise: amt });
  if (ok) showWallet(data.wallet); else alert('Top up failed: ' + JSON.stringify(data));
});

$('genKey').onclick = () => { $('idemKey').value = 'idem-' + Math.random().toString(36).slice(2) + Date.now(); };

$('transferBtn').onclick = () => busy($('transferBtn'), async () => {
  const to = $('toWallet').value.trim();
  const amount_paise = parseInt($('transferAmt').value, 10);
  let key = $('idemKey').value.trim();
  if (!key) { key = 'idem-' + Math.random().toString(36).slice(2) + Date.now(); $('idemKey').value = key; }
  if (!to || !(amount_paise > 0)) return alert('Need destination wallet and positive amount');
  const { ok, status, data } = await api('POST', '/transfers', {
    from: state.walletId, to, amount_paise, idempotency_key: key,
  });
  out($('transferOut'), { http: status, ...data }, ok);
  await refreshWallet();
});

$('statusBtn').onclick = () => busy($('statusBtn'), async () => {
  const id = $('statusId').value.trim();
  if (!id) return;
  const { ok, status, data } = await api('GET', '/transfers/' + id);
  out($('statusOut'), { http: status, ...data }, ok);
});

$('reverseBtn').onclick = () => busy($('reverseBtn'), async () => {
  const id = $('statusId').value.trim();
  let key = $('reverseKey').value.trim();
  if (!id) return alert('Put the transfer id in the status field');
  if (!key) { key = 'reverse-' + Math.random().toString(36).slice(2) + Date.now(); $('reverseKey').value = key; }
  const { ok, status, data } = await api('POST', `/transfers/${id}/reverse`, { idempotency_key: key });
  out($('statusOut'), { http: status, ...data }, ok);
  await refreshWallet();
});

// --- live metrics ---
function sumMetric(text, name) {
  let total = 0, found = false;
  for (const line of text.split('\n')) {
    if (line.startsWith('#')) continue;
    if (line.startsWith(name + ' ') || line.startsWith(name + '{')) {
      const val = parseFloat(line.slice(line.lastIndexOf(' ') + 1));
      if (isFinite(val)) { total += val; found = true; }
    }
  }
  return found ? total : null;
}

const METRIC_CARDS = [
  { name: 'wallet_total_balance_paise', label: 'Total balance (conservation)', cls: 'hl', fmt: moneyCompact },
  { name: 'transfers_created_total', label: 'Transfers created', cls: 'good' },
  { name: 'transfers_idempotent_replays_total', label: 'Idempotent replays', cls: 'hl' },
  { name: 'transfers_declined_insufficient_funds_total', label: 'Declined (funds)', cls: 'warn' },
  { name: 'transfers_conflicts_total', label: 'Conflicts (409)', cls: 'warn' },
  { name: 'wallets_created_total', label: 'Wallets created', cls: '' },
  { name: 'reversals_created_total', label: 'Reversals', cls: 'good' },
  { name: 'reversals_already_done_total', label: 'Double-reverse blocked', cls: 'warn' },
  { name: 'http_requests_total', label: 'HTTP requests', cls: '' },
];

async function pollMetrics() {
  if (!state.apiBase) return;
  try {
    const r = await fetch(state.apiBase.replace(/\/$/, '') + '/metrics');
    if (!r.ok) throw new Error('metrics ' + r.status);
    const text = await r.text();
    const grid = $('metricsGrid');
    grid.innerHTML = '';
    for (const m of METRIC_CARDS) {
      const v = sumMetric(text, m.name);
      const div = document.createElement('div');
      div.className = 'metric ' + (m.cls || '');
      const shown = v === null ? '—' : (m.fmt ? m.fmt(v) : Math.round(v).toLocaleString());
      const title = (m.name === 'wallet_total_balance_paise' && v !== null) ? paise(v) : '';
      div.innerHTML = `<div class="label">${m.label}</div><div class="value" title="${title}">${shown}</div>`;
      grid.appendChild(div);
    }
    $('metricsPulse').className = 'pill pill-ok';
    $('metricsPulse').textContent = 'live';
    setConn(true, 'connected');
  } catch {
    $('metricsPulse').className = 'pill pill-err';
    $('metricsPulse').textContent = 'no metrics';
  }
}

// --- copy-to-clipboard for long ids/tokens ---
document.addEventListener('click', async (e) => {
  const btn = e.target.closest('.copybtn');
  if (!btn) return;
  const target = document.getElementById(btn.dataset.copy);
  const text = target ? target.textContent.trim() : '';
  if (!text || text === '—') return;
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // fallback for non-HTTPS / older browsers
    const ta = document.createElement('textarea');
    ta.value = text; document.body.appendChild(ta); ta.select();
    try { document.execCommand('copy'); } catch {}
    ta.remove();
  }
  const prev = btn.textContent;
  btn.textContent = '✓';
  setTimeout(() => { btn.textContent = prev; }, 1200);
});

// --- init ---
(function init() {
  // Default to same-origin: when the UI is served by the API host, it just works (no CORS,
  // no manual config). When hosted elsewhere (e.g. Vercel), the user sets the API base once.
  if (!state.apiBase) {
    state.apiBase = window.location.origin;
    localStorage.setItem('lw_api', state.apiBase);
  }
  $('apiBase').value = state.apiBase;
  if (state.token) { showUser(); }
  if (state.walletId) { $('walletBox').classList.remove('hidden'); $('wId').textContent = state.walletId; }
  refreshEnabled();
  $('saveApi').click();
  pollMetrics();
  setInterval(pollMetrics, 3000);
})();
