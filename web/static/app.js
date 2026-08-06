// public panel: pricing, NIP-07 login, lightning checkout.

const $ = (id) => document.getElementById(id);
let info = null;
let pubkey = null;
let pollTimer = null;
let domains = [];
let group = null; // the group this pubkey belongs to, if any
let signer = null; // whoever can sign for `pubkey`: NIP-07 or a NIP-46 bunker
let nostrconnect = null; // a pending nostrconnect invitation

const fmtSats = (n) => n.toLocaleString() + " sats";
const fmtBytes = (n) => {
  if (n >= 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + " MB";
  if (n >= 1024) return (n / 1024).toFixed(1) + " KB";
  return n + " B";
};
const fmtDate = (unix) => (unix ? new Date(unix * 1000).toLocaleString() : t("무기한"));

const esc = (s) =>
  String(s).replace(/[<>&"]/g, (c) => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;", '"': "&quot;" }[c]));

async function api(path, options) {
  const res = await fetch(path, options);
  const body = res.status === 204 ? null : await res.json();
  if (!res.ok) throw new Error((body && body.error) || res.statusText);
  return body;
}

const postJSON = (payload) => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(payload),
});

// signedAPI is for the few routes only a group owner may call: the request
// carries a NIP-98 signature instead of a session. Whoever holds the key — a
// NIP-07 extension or a NIP-46 remote signer — is behind `signer`.
async function signedAPI(path, method, payload) {
  if (!signer) throw new Error(t("멤버 관리에는 서명 로그인이 필요합니다."));
  const body = payload === undefined ? undefined : JSON.stringify(payload);
  const headers = { Authorization: await nip98Header(path, method, body, signer) };
  if (body) headers["Content-Type"] = "application/json";
  return api(path, { method, headers, body });
}

async function loadInfo() {
  info = await api("/api/info");
  applyTheme(info);
  document.title = info.name;
  $("relay-name").textContent = info.name;
  $("brand-name").textContent = info.name;
  $("login-name").textContent = info.name;
  $("relay-desc").textContent = info.description;
  $("login-desc").textContent = info.description;
  $("relay-url").textContent = info.relay_url || location.origin.replace(/^http/, "ws");

  const rows = [
    [t("가입비 (최초 1회)"), fmtSats(info.admission_sats)],
    [t("구독 ({0}일)", info.period_days), fmtSats(info.subscription_sats)],
    [t("구독 포함 용량"), `${info.included_mb} MB`],
    [t("추가 용량"), `${fmtSats(info.price_per_mb_sats)} / MB`],
  ];
  if (info.nip05_domains > 0) rows.push([t("NIP-05 도메인"), t("{0}개 판매 중", info.nip05_domains)]);

  // the relay is its own Blossom server, so clients only need this address
  if (info.media_enabled) {
    const base = (info.blossom_url || location.origin).replace(/\/$/, "");
    $("blossom-url").value = base;
    $("nip96-url").value = base + "/.well-known/nostr/nip96.json";
    $("sec-blossom").hidden = false;
    $("nav-blossom").hidden = false;
  }

  $("prices").innerHTML = rows
    .map(([k, v]) => `<tr><th>${k}</th><td class="num">${v}</td></tr>`).join("");
}

async function showAccount() {
  if (!pubkey) return;
  const acct = await api("/api/account/" + pubkey);
  for (const id of ["sec-order", "sec-group", "sec-nip05"]) $(id).hidden = false;
  // an invite has to be spent by whoever holds the key, so it needs a signer
  $("sec-invite").hidden = !signer;

  if (!acct.exists) {
    $("account").innerHTML =
      `<div class="row"><span class="chip"><code>${pubkey.slice(0, 16)}…</code></span>
       <span class="muted small">${t("아직 등록되지 않음. 아래에서 가입하세요.")}</span></div>`;
  } else {
    const pct = acct.quota_bytes ? Math.min(100, (acct.used_bytes / acct.quota_bytes) * 100) : 100;
    $("account").innerHTML = `
      <div class="row">
        <span class="chip"><code>${pubkey.slice(0, 16)}…</code>
          <span class="sub">${t("만료 {0}", fmtDate(acct.expires_at))}</span></span>
        <span class="${acct.can_write ? "ok" : "bad"} small">${acct.can_write ? t("쓰기 가능") : t("쓰기 불가")}</span>
      </div>
      <div class="bar"><span style="width:${pct}%"></span></div>
      <p class="small muted">${t("{0} / {1} 사용", fmtBytes(acct.used_bytes), fmtBytes(acct.quota_bytes))}</p>`;
  }

  group = acct.group || null;
  showGroup();
  showMyNames();
  quote();
}

// --- NIP-05 ---

async function loadDomains() {
  const res = await api("/api/nip05/domains");
  domains = res.domains || [];
  $("nip05-domain").innerHTML = domains
    .map((d) => `<option value="${esc(d.domain)}">${esc(d.domain)}</option>`)
    .join("");
  if (!domains.length) {
    $("nip05-quote").textContent = t("판매 중인 도메인이 없습니다.");
  }
}

async function showMyNames() {
  if (!pubkey) return;
  const names = await api("/api/nip05/names/" + pubkey).catch(() => []);
  $("my-names").innerHTML = names.length
    ? `<p class="small muted">${t("보유 중인 주소")}</p><div class="chips">` +
      names.map((n) => `<span class="chip"><code>${esc(n.name)}@${esc(n.domain)}</code>
        <span class="sub">${n.permanent ? t("무기한") : t("만료 {0}", fmtDate(n.expires_at))}</span></span>`).join("") +
      `</div>`
    : "";
}

// checkName asks the server, because only it knows about reservations, blocked
// names and the premium multipliers.
async function checkName() {
  const name = $("nip05-name").value.trim().toLowerCase();
  const domain = $("nip05-domain").value;
  const periods = Number($("nip05-periods").value) || 1;
  if (!name || !domain) return;

  $("order-name").disabled = true;
  const res = await api(
    `/api/nip05/check?name=${encodeURIComponent(name)}&domain=${encodeURIComponent(domain)}` +
    `&periods=${periods}&pubkey=${pubkey || ""}`);

  if (res.available) {
    $("nip05-quote").innerHTML =
      `<span class="ok">${t("{0}@{1} 구매 가능 — {2}", esc(name), esc(domain), fmtSats(res.sats))}</span>`;
    $("order-name").disabled = false;
  } else {
    $("nip05-quote").innerHTML = `<span class="bad">${esc(res.reason)}</span>`;
  }
}

// --- groups ---

function showGroup() {
  if (!group) {
    $("group-status").innerHTML = `<p class="muted small">${t("아직 그룹이 없습니다. 이름을 정해 만드세요.")}</p>`;
    $("group-members").innerHTML = "";
    $("group-name").disabled = false;
    return;
  }

  const pct = group.quota_bytes ? Math.min(100, (group.used_bytes / group.quota_bytes) * 100) : 100;
  $("group-status").innerHTML = `
    <div class="row">
      <span class="chip">${esc(group.name || group.id.slice(0, 12))}
        <span class="sub">${t("만료 {0}", fmtDate(group.expires_at))}</span></span>
      ${group.owner === pubkey ? `<span class="ok small">${t("소유자")}</span>` : ""}
    </div>
    <div class="bar"><span style="width:${pct}%"></span></div>
    <p class="small muted">${t("{0} / {1} 사용", fmtBytes(group.used_bytes), fmtBytes(group.quota_bytes))}</p>`;
  // the name is fixed once the group exists; top-ups go to the same one
  $("group-name").value = group.name || "";
  $("group-name").disabled = true;

  if (group.owner === pubkey) {
    showMembers();
  } else {
    $("group-members").innerHTML = `<p class="muted small">${t("멤버 관리는 그룹 소유자만 할 수 있습니다.")}</p>`;
  }
}

async function showMembers() {
  let members;
  try {
    members = await signedAPI(`/api/group/${group.id}/members`, "GET");
  } catch (err) {
    $("group-members").innerHTML =
      `<p class="muted small">${t("멤버를 보려면 서명이 필요합니다.")} <button id="load-members" class="secondary">${t("불러오기")}</button></p>`;
    $("load-members").onclick = () => showMembers().catch((e) => alert(e.message));
    return;
  }

  $("group-members").innerHTML = `
    <h3>${t("멤버")}</h3>
    <div class="row">
      <input id="new-member" class="grow" placeholder="${t("추가할 pubkey (hex)")}" spellcheck="false">
      <button id="add-member">${t("추가")}</button>
    </div>
    <div class="chips">` +
    members.map((m) => `
      <span class="chip" data-member="${esc(m.pubkey)}">
        <code>${esc(m.pubkey.slice(0, 24))}…</code>
        ${m.pubkey === group.owner ? `<span class="sub">${t("소유자")}</span>`
          : '<svg class="i member-del"><use href="#i-x"/></svg>'}
      </span>`).join("") + `</div>`;

  $("add-member").onclick = async () => {
    const member = $("new-member").value.trim().toLowerCase();
    if (!/^[0-9a-f]{64}$/.test(member)) return alert(t("64자리 hex pubkey를 입력하세요."));
    try {
      await signedAPI(`/api/group/${group.id}/members/${member}`, "PUT", {});
      showMembers();
    } catch (err) {
      alert(err.message);
    }
  };
}

function quote() {
  if (!info) return;
  const periods = Number($("periods").value) || 0;
  const extra = Number($("extra-mb").value) || 0;
  const total = periods * info.subscription_sats + extra * info.price_per_mb_sats;
  $("quote").textContent =
    t("구독 {0}회 + 추가 {1}MB ≈ {2} (신규 가입이면 가입비 {3} 추가)",
       periods, extra, fmtSats(total), fmtSats(info.admission_sats));
}

// --- login ---

const status = (text, kind = "muted") =>
  ($("login-status").innerHTML = `<span class="${kind}">${esc(text)}</span>`);

// signedIn hides the login options once a key is behind us, whichever route
// got us there.
function signedIn(pk, how) {
  pubkey = pk.toLowerCase();
  showPanel();
  showAccount();
  if (how) status(how);
}

// the login screen is its own page; the panel only appears behind a key, or
// when someone just wants to read the prices
function showPanel() {
  $("login-card").hidden = true;
  $("panel").hidden = false;
}

async function connect() {
  if (!window.nostr) {
    alert(t("NIP-07 확장(Alby, nos2x 등)이 없습니다. bunker 연결이나 QR을 쓰거나 pubkey를 붙여넣으세요."));
    return;
  }
  signer = window.nostr;
  signedIn(await window.nostr.getPublicKey());
}

// onSignerStatus surfaces what the remote signer is asking for — most often an
// approval page the user has to open.
function onSignerStatus(update) {
  if (update.authUrl) {
    $("login-status").innerHTML =
      `<a href="${esc(update.authUrl)}" target="_blank" rel="noopener">${t("서명자 승인 창 열기")}</a>`;
    window.open(update.authUrl, "_blank", "noopener,width=600,height=700");
    return;
  }
  if (update.message) status(update.message);
}

async function connectBunker() {
  const uri = $("bunker-uri").value.trim();
  if (!uri) return;
  $("bunker-connect").disabled = true;
  status(t("서명자에 연결하는 중…"));
  try {
    signer = await window.nip46.connectBunker(uri, onSignerStatus);
    signedIn(signer.pubkey, t("원격 서명자에 연결되었습니다."));
  } catch (err) {
    status(err.message, "bad");
  } finally {
    $("bunker-connect").disabled = false;
  }
}

async function startNostrconnect() {
  const relays = (info?.nip46_relays || "").split(",").map((r) => r.trim()).filter(Boolean);
  if (!relays.length) return status(t("이 릴레이는 nostrconnect relay가 설정되어 있지 않습니다."), "bad");

  nostrconnect?.cancel();
  nostrconnect = window.nip46.startNostrconnect(
    { relays, name: info.name, url: location.origin, perms: "sign_event:27235" },
    onSignerStatus);

  const qr = qrcode(0, "M");
  qr.addData(nostrconnect.uri);
  qr.make();
  $("nc-box").innerHTML = qr.createImgTag(4, 0);
  $("nc-uri").value = nostrconnect.uri;
  $("nc-uri-row").hidden = false;
  status(t("지갑 앱으로 QR을 스캔하거나 연결 문자열을 붙여넣으세요"));

  try {
    signer = await nostrconnect.connected;
    signedIn(signer.pubkey, t("원격 서명자에 연결되었습니다."));
  } catch (err) {
    status(err.message, "bad");
  }
}

// placeOrder is the one checkout path: relay access, a group top-up and a
// NIP-05 name all end up as an invoice shown in the same card.
async function placeOrder(button, fields) {
  button.disabled = true;
  // the backend has to reach the lightning node first, so the modal opens on
  // a spinner rather than after the wait
  showInvoice("loading");
  $("invoice-status").textContent = "";
  try {
    const inv = await api("/api/order", postJSON({ pubkey, ...fields }));
    $("qr").src = inv.qr;
    $("bolt11").value = inv.bolt11;
    $("invoice-status").textContent = t("{0} 결제 대기 중…", fmtSats(inv.sats));
    showInvoice("invoice");
    watch(inv.payment_hash);
  } catch (err) {
    closeInvoice();
    alert(err.message);
  } finally {
    button.disabled = false;
  }
}

// The invoice modal has three faces: the spinner while the invoice is minted,
// the QR to pay, and the check once the payment lands.
function showInvoice(state) {
  $("invoice-loading").hidden = state !== "loading";
  $("invoice-body").hidden = state !== "invoice";
  $("invoice-done").hidden = state !== "done";
  if (!$("invoice-modal").open) $("invoice-modal").showModal();
}

function closeInvoice() {
  clearInterval(pollTimer);
  $("invoice-modal").close();
}

const order = () => placeOrder($("order"), {
  periods: Number($("periods").value) || 0,
  extra_mb: Number($("extra-mb").value) || 0,
});

const orderName = () => placeOrder($("order-name"), {
  nip05_domain: $("nip05-domain").value,
  nip05_name: $("nip05-name").value.trim().toLowerCase(),
  nip05_periods: Number($("nip05-periods").value) || 1,
});

function orderGroup() {
  const fields = {
    periods: Number($("group-periods").value) || 0,
    extra_mb: Number($("group-mb").value) || 0,
  };
  // an existing group is topped up by id; a new one is created by name
  if (group) {
    fields.group_id = group.id;
  } else {
    fields.group_name = $("group-name").value.trim() || "group";
  }
  return placeOrder($("order-group"), fields);
}

function watch(hash) {
  clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    const inv = await api("/api/invoice/" + hash).catch(() => null);
    if (!inv) return;
    if (inv.paid) {
      clearInterval(pollTimer);
      $("invoice-status").textContent = "";
      showInvoice("done");
      showAccount();
    } else if (inv.status === "expired") {
      clearInterval(pollTimer);
      $("invoice-status").innerHTML = `<span class="bad">${t("인보이스 만료. 다시 발행하세요.")}</span>`;
    }
  }, 3000);
}

$("browse").onclick = showPanel;
$("logout").onclick = (e) => {
  e.preventDefault();
  location.reload();
};
$("connect").onclick = () => connect().catch((err) => status(err.message, "bad"));
$("bunker-connect").onclick = connectBunker;
$("nc-start").onclick = startNostrconnect;
$("nc-copy").onclick = () => navigator.clipboard.writeText($("nc-uri").value);
$("order").onclick = order;
$("periods").oninput = quote;
$("extra-mb").oninput = quote;
$("copy").onclick = () => navigator.clipboard.writeText($("bolt11").value);
$("invoice-close").onclick = closeInvoice;
// Esc closes the dialog on its own; stop polling with it
$("invoice-modal").addEventListener("close", () => clearInterval(pollTimer));
$("copy-blossom").onclick = () => navigator.clipboard.writeText($("blossom-url").value);
$("copy-nip96").onclick = () => navigator.clipboard.writeText($("nip96-url").value);

$("claim-invite").onclick = async () => {
  const code = $("invite-code").value.trim();
  if (!code) return;
  $("claim-invite").disabled = true;
  try {
    await signedAPI("/api/invite/claim", "POST", { code });
    $("invite-status").innerHTML = `<span class="ok">${t("등록되었습니다.")}</span>`;
    $("invite-code").value = "";
    showAccount();
  } catch (err) {
    $("invite-status").innerHTML = `<span class="bad">${esc(err.message)}</span>`;
  } finally {
    $("claim-invite").disabled = false;
  }
};

$("check-name").onclick = () => checkName().catch((err) => alert(err.message));
$("order-name").onclick = orderName;
$("order-group").onclick = orderGroup;
// any edit invalidates the last availability answer
for (const id of ["nip05-name", "nip05-domain", "nip05-periods"]) {
  $(id).oninput = () => {
    $("order-name").disabled = true;
    $("nip05-quote").textContent = "";
  };
}
$("group-members").addEventListener("click", (e) => {
  const row = e.target.closest("[data-member]");
  if (!row || !e.target.closest(".member-del")) return;
  if (!confirm(t("이 멤버를 그룹에서 뺄까요?"))) return;
  signedAPI(`/api/group/${group.id}/members/${row.dataset.member}`, "DELETE")
    .then(showMembers).catch((err) => alert(err.message));
});
$("pubkey-input").onchange = (e) => {
  const v = e.target.value.trim();
  if (/^[0-9a-f]{64}$/i.test(v)) {
    // read-only: we know who they are but cannot sign for them
    signer = null;
    signedIn(v, t("읽기 전용입니다. 멤버 관리에는 서명 로그인이 필요합니다."));
  } else if (v) {
    alert(t("64자리 hex pubkey를 입력하세요 (npub은 확장 연결을 사용하세요)."));
  }
};

// The sidebar mirrors the page: highlight whichever card is on screen.
const navLinks = [...document.querySelectorAll('.nav a[href^="#sec-"]')];
const spy = new IntersectionObserver(
  (entries) => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      for (const link of navLinks) {
        link.classList.toggle("active", link.hash === "#" + entry.target.id);
      }
    }
  },
  { rootMargin: "-10% 0px -80% 0px" });
for (const link of navLinks) spy.observe($(link.hash.slice(1)));

loadInfo().catch((err) => alert(err.message));
loadDomains().catch(() => {});
