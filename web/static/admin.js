// admin panel: NIP-98 signed requests or a password session cookie.

const $ = (id) => document.getElementById(id);
const MB = 1024 * 1024;

// "nostr" = sign every request with `signer`, "session" = rely on the cookie
let mode = null;
let settings = null;
// the NIP-07 extension or a NIP-46 remote signer, whichever logged in
let signer = null;
let info = null;
let nostrconnect = null;

const fmtDate = (unix) => (unix ? new Date(unix * 1000).toLocaleString() : t("무기한"));
const fmtMB = (bytes) => (bytes / MB).toFixed(1);
const fmtBytes = (n) =>
  n >= MB ? (n / MB).toFixed(1) + " MB" : n >= 1024 ? (n / 1024).toFixed(1) + " KB" : n + " B";
const esc = (s) => String(s).replace(/[<>&"]/g, (c) => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;", '"': "&quot;" }[c]));

// nip98Header comes from /static/nip98.js.
async function api(path, method = "GET", payload) {
  const body = payload === undefined ? undefined : JSON.stringify(payload);
  const headers = {};
  if (body) headers["Content-Type"] = "application/json";
  if (mode === "nostr") headers["Authorization"] = await nip98Header(path, method, body, signer);

  const res = await fetch(path, { method, headers, body });
  const data = res.status === 204 ? null : await res.json();
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

async function loadAll() {
  $("login-card").hidden = true;
  $("panel").hidden = false;
  await Promise.all([
    loadStats(), loadSettings(), loadAccounts(), loadBlobs(), loadPayments(),
    loadGroups(), loadDomains(), loadBlockedNames(), loadNames(), loadInvites(),
  ]);
}

async function loadStats() {
  const s = await api("/api/admin/stats");
  $("stats").innerHTML = [
    [t("계정"), t("{0} (활성 {1})", s.accounts, s.active_accounts)],
    [t("이벤트"), s.events],
    [t("저장량"), `${fmtMB(s.stored_bytes)} MB`],
    [t("결제"), t("{0}건 / {1} sats", s.paid_payments, s.sats_collected.toLocaleString())],
    [t("그룹"), s.groups],
    [t("NIP-05 주소"), s.nip05_names],
  ].map(([k, v]) => `<tr><th>${k}</th><td class="num">${v}</td></tr>`).join("");
}

const settingFields = [
  ["relay_name", "릴레이 이름", "text"],
  ["relay_description", "설명", "text"],
  ["relay_icon", "로고 · 아이콘 URL", "text"],
  ["relay_banner", "배너 URL", "text"],
  ["relay_contact", "운영자 연락처 (mailto: 등)", "text"],
  ["relay_pubkey", "운영자 pubkey (hex, 비우면 릴레이 키)", "text"],
  ["theme_bg_color", "배경색", "color", "#121417"],
  ["theme_accent", "포인트 색", "color", "#f7931a"],
  ["theme_bg_image", "배경 이미지 URL", "text"],
  ["relay_retention_days", "보관 기간 (일, 0 = 무기한)", "number"],
  ["relay_posting_policy", "이용 약관 URL", "text"],
  ["relay_countries", "운영 국가 (ISO 3166-1, 쉼표)", "text"],
  ["relay_languages", "언어 (ISO 639-1, 쉼표)", "text"],
  ["relay_topics", "주제 태그 (쉼표)", "text"],
  ["admission_sats", "가입비 (sats)", "number"],
  ["subscription_sats", "구독료 (sats)", "number"],
  ["period_days", "구독 기간 (일)", "number"],
  ["included_mb", "구독 포함 용량 (MB)", "number"],
  ["price_per_mb_sats", "추가 용량 단가 (sats/MB)", "number"],
  ["nip05_premium_tiers", "짧은 이름 할증 (길이:배수)", "text"],
  ["nip46_relays", "NIP-46 로그인 relay (쉼표 구분)", "text"],
  ["nsite_domains", "nsite 호스팅 도메인 (NIP-5A, 쉼표)", "text"],
  ["accept_zap_receipts", "제3자 zap 영수증 허용 (NIP-57)", "checkbox"],
  ["accept_badge_awards", "제3자 배지 수여 허용 (NIP-58)", "checkbox"],
  ["auto_invite", "누구나 초대 코드 요청 가능 (NIP-43 kind 28935)", "checkbox"],
  ["auto_invite_period_days", "자동 초대 기간 (일)", "number"],
  ["auto_invite_quota_mb", "자동 초대 용량 (MB)", "number"],
  ["read_auth_required", "읽기도 인증·화이트리스트 필요 (NIP-42)", "checkbox"],
  ["min_pow", "최소 작업증명 난이도 (NIP-13, 0 = 끔)", "number"],
  ["created_at_max_past", "created_at 과거 허용치 (초, 0 = 무제한)", "number"],
  ["created_at_max_future", "created_at 미래 허용치 (초, 0 = 무제한)", "number"],
  ["max_blob_size_mb", "파일 1개 최대 크기 (MB)", "number"],
];

// cols renders a <colgroup>; "" means "share whatever is left".
const cols = (...widths) =>
  `<colgroup>${widths.map((w) => (w ? `<col style="width:${w}px">` : "<col>")).join("")}</colgroup>`;

async function loadSettings() {
  settings = await api("/api/admin/settings");
  $("settings").innerHTML = settingFields
    // a colour input has no empty state, so an unset colour shows the value the
    // stylesheet is already using
    .map(([key, label, type, fallback]) =>
      type === "checkbox"
        ? `<label><input id="set-${key}" type="checkbox"${settings[key] ? " checked" : ""}>${t(label)}</label>`
        : `<label class="stack">${t(label)}
             <input id="set-${key}" type="${type}" value="${esc(settings[key] ?? "") || fallback || ""}"></label>`)
    .join("");
  renderPayment();
  renderStorage();
  applyTheme(settings);
}

async function saveSettings() {
  const payload = {};
  for (const [key, , type] of settingFields) {
    const field = $("set-" + key);
    payload[key] = type === "checkbox" ? field.checked
      : type === "number" ? Number(field.value)
      : field.value;
  }
  try {
    settings = await api("/api/admin/settings", "PUT", payload);
    $("settings-msg").textContent = t("저장됨");
    renderPayment();
    applyTheme(settings);
  } catch (err) {
    $("settings-msg").textContent = err.message;
  }
}

// --- payment backend ---

// Secrets come back masked (••••abcd); sending them back unchanged tells the
// server to keep what it already has.
function renderPayment() {
  $("payment-provider").value = settings.payment_provider || "lnbits";
  $("set-lnbits_url").value = settings.lnbits_url || "";
  $("set-lnbits_invoice_key").value = settings.lnbits_invoice_key || "";
  $("set-nwc_uri").value = settings.nwc_uri || "";
  togglePaymentFields();
}

function togglePaymentFields() {
  const provider = $("payment-provider").value;
  $("lnbits-fields").hidden = provider !== "lnbits";
  $("nwc-fields").hidden = provider !== "nwc";
  $("provider-warning").textContent =
    provider === "mock" ? t("mock은 결제 없이 인보이스가 스스로 정산됩니다. 운영에서 쓰지 마세요.") : "";
}

function paymentPayload() {
  return {
    payment_provider: $("payment-provider").value,
    lnbits_url: $("set-lnbits_url").value.trim(),
    lnbits_invoice_key: $("set-lnbits_invoice_key").value.trim(),
    nwc_uri: $("set-nwc_uri").value.trim(),
  };
}

async function testPayment() {
  $("payment-msg").textContent = t("테스트 중…");
  try {
    const res = await api("/api/admin/payments/test", "POST", paymentPayload());
    $("payment-msg").innerHTML = res.ok
      ? `<span class="ok">${esc(res.message)}</span>`
      : `<span class="bad">${esc(res.message)}</span>`;
  } catch (err) {
    $("payment-msg").innerHTML = `<span class="bad">${esc(err.message)}</span>`;
  }
}

async function savePayment() {
  try {
    settings = await api("/api/admin/settings", "PUT", paymentPayload());
    renderPayment();
    $("payment-msg").innerHTML = `<span class="ok">${t("저장됨 — 새 인보이스부터 이 백엔드를 씁니다.")}</span>`;
  } catch (err) {
    $("payment-msg").innerHTML = `<span class="bad">${esc(err.message)}</span>`;
  }
}

async function loadAccounts() {
  const q = encodeURIComponent($("search").value || "");
  const accounts = await api("/api/admin/accounts?q=" + q);
  $("accounts").innerHTML =
    cols("", 110, 190, 160, "", 110) +
    `<tr><th>pubkey</th><th>${t("상태")}</th><th>${t("만료")}</th><th>${t("사용/할당(MB)")}</th><th>${t("메모")}</th><th></th></tr>` +
    accounts.map((a) => `
      <tr data-pubkey="${a.pubkey}">
        <td><code title="${a.pubkey}">${a.pubkey.slice(0, 12)}…</code></td>
        <td>
          <select class="f-status">
            <option value="active"${a.status === "active" ? " selected" : ""}>active</option>
            <option value="banned"${a.status === "banned" ? " selected" : ""}>banned</option>
          </select>
        </td>
        <td><input class="f-expires" type="datetime-local" value="${toLocalInput(a.expires_at)}"></td>
        <td class="num">${fmtMB(a.used_bytes)} / <input class="f-quota" type="number" value="${fmtMB(a.quota_bytes)}"></td>
        <td><input class="f-note" value="${esc(a.note)}"></td>
        <td><button class="save link">${t("저장")}</button> <button class="del link danger">${t("삭제")}</button></td>
      </tr>`).join("") +
    (accounts.length ? "" : `<tr><td colspan="6" class="empty">${t("계정 없음")}</td></tr>`);
}

function toLocalInput(unix) {
  if (!unix) return "";
  const d = new Date(unix * 1000 - new Date().getTimezoneOffset() * 60000);
  return d.toISOString().slice(0, 16);
}

async function saveRow(tr) {
  const expires = tr.querySelector(".f-expires").value;
  await api("/api/admin/accounts/" + tr.dataset.pubkey, "PUT", {
    status: tr.querySelector(".f-status").value,
    expires_at: expires ? Math.floor(new Date(expires).getTime() / 1000) : 0,
    quota_bytes: Math.round(Number(tr.querySelector(".f-quota").value) * MB),
    note: tr.querySelector(".f-note").value,
  });
  loadAccounts();
}

async function addAccount() {
  const pubkey = $("new-pubkey").value.trim().toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(pubkey)) return alert("64자리 hex pubkey를 입력하세요.");
  const days = Number($("new-days").value) || 0;
  await api("/api/admin/accounts/" + pubkey, "PUT", {
    status: "active",
    expires_at: days ? Math.floor(Date.now() / 1000) + days * 86400 : 0,
    quota_bytes: Math.round((Number($("new-mb").value) || 0) * MB),
    note: "manual",
  });
  $("new-pubkey").value = "";
  loadAccounts();
}

// --- storage backend ---

const storageFields = [
  "local_path", "s3_endpoint", "s3_bucket", "s3_region",
  "s3_prefix", "s3_access_key", "s3_secret_key", "s3_public_url",
];

function renderStorage() {
  $("storage-backend").value = settings.storage_backend || "local";
  for (const key of storageFields) $("set-" + key).value = settings[key] || "";
  toggleStorageFields();
  $("storage-current").textContent =
    settings.storage_backend === "s3"
      ? t("현재: {0} ({1})", settings.s3_bucket || "?", settings.s3_endpoint || "?")
      : t("현재: {0}", settings.local_path || t("기본 경로"));
}

function toggleStorageFields() {
  const backend = $("storage-backend").value;
  $("local-fields").hidden = backend !== "local";
  $("s3-fields").hidden = backend !== "s3";
}

function storagePayload() {
  const payload = { storage_backend: $("storage-backend").value };
  for (const key of storageFields) payload[key] = $("set-" + key).value.trim();
  return payload;
}

async function testStorage() {
  $("storage-msg").textContent = t("테스트 중…");
  try {
    const res = await api("/api/admin/storage/test", "POST", storagePayload());
    $("storage-msg").innerHTML = res.ok
      ? `<span class="ok">${esc(res.message)}</span>`
      : `<span class="bad">${esc(res.message)}</span>`;
  } catch (err) {
    $("storage-msg").innerHTML = `<span class="bad">${esc(err.message)}</span>`;
  }
}

async function saveStorage() {
  try {
    settings = await api("/api/admin/settings", "PUT", storagePayload());
    renderStorage();
    $("storage-msg").innerHTML =
      `<span class="ok">${t("저장됨 — 새 업로드부터 이 위치를 씁니다. 기존 파일은 migrate-blobs로 옮기세요.")}</span>`;
    loadBlobs();
  } catch (err) {
    $("storage-msg").innerHTML = `<span class="bad">${esc(err.message)}</span>`;
  }
}

// --- invite codes (NIP-43) ---

async function loadInvites() {
  const invites = await api("/api/admin/invites");
  $("invites").innerHTML =
    cols(160, "", 90, 90, 100, 170, 90) +
    `<tr><th>${t("코드")}</th><th>${t("메모")}</th><th>${t("기간")}</th><th>${t("용량")}</th><th>${t("사용")}</th><th>${t("만료")}</th><th></th></tr>` +
    invites.map((inv) => `
      <tr data-invite="${esc(inv.code)}">
        <td><code class="copy" title="${t("클릭하면 복사")}">${esc(inv.code)}</code></td>
        <td>${esc(inv.note)}</td>
        <td class="num">${t("{0}일", inv.period_days)}</td>
        <td class="num">${inv.quota_mb} MB</td>
        <td class="num ${inv.max_uses && inv.used >= inv.max_uses ? "bad" : ""}">${inv.used} / ${inv.max_uses || "∞"}</td>
        <td class="small">${inv.expires_at ? fmtDate(inv.expires_at) : t("없음")}</td>
        <td><button class="del link danger">${t("삭제")}</button></td>
      </tr>`).join("") +
    (invites.length ? "" : `<tr><td colspan="7" class="empty">${t("초대 코드 없음")}</td></tr>`);
}

async function addInvite() {
  const invite = await api("/api/admin/invites", "POST", {
    note: $("new-invite-note").value.trim(),
    period_days: Number($("new-invite-days").value) || 0,
    quota_mb: Number($("new-invite-mb").value) || 0,
    max_uses: Number($("new-invite-uses").value) || 0,
    expires_in_days: Number($("new-invite-expires").value) || 0,
  });
  $("new-invite-note").value = "";
  await loadInvites();
  navigator.clipboard?.writeText(invite.code).catch(() => {});
}

// --- groups (shared storage) ---

// which group's member list is open, so a reload doesn't collapse it
let openGroup = null;

async function loadGroups() {
  const q = encodeURIComponent($("group-search").value || "");
  const groups = await api("/api/admin/groups?q=" + q);
  $("groups").innerHTML =
    cols("", 130, 70, 110, 190, 160, "", 110) +
    `<tr><th>${t("이름")}</th><th>${t("소유자")}</th><th>${t("멤버")}</th><th>${t("상태")}</th><th>${t("만료")}</th><th>${t("사용/할당(MB)")}</th><th>${t("메모")}</th><th></th></tr>` +
    groups.map((g) => `
      <tr data-group="${esc(g.id)}" data-owner="${esc(g.owner)}">
        <td><input class="f-name" value="${esc(g.name)}"></td>
        <td><code title="${esc(g.owner)}">${esc(g.owner.slice(0, 10))}…</code></td>
        <td><button class="members link">${t("{0}명", g.members)}</button></td>
        <td>
          <select class="f-status">
            <option value="active"${g.status === "active" ? " selected" : ""}>active</option>
            <option value="banned"${g.status === "banned" ? " selected" : ""}>banned</option>
          </select>
        </td>
        <td><input class="f-expires" type="datetime-local" value="${toLocalInput(g.expires_at)}"></td>
        <td class="num">${fmtMB(g.used_bytes)} / <input class="f-quota" type="number" value="${fmtMB(g.quota_bytes)}"></td>
        <td><input class="f-note" value="${esc(g.note)}"></td>
        <td><button class="save link">${t("저장")}</button> <button class="del link danger">${t("삭제")}</button></td>
      </tr>`).join("") +
    (groups.length ? "" : `<tr><td colspan="8" class="empty">${t("그룹 없음")}</td></tr>`);

  if (openGroup && groups.some((g) => g.id === openGroup)) {
    await loadMembers(openGroup);
  } else {
    openGroup = null;
    $("group-members").innerHTML = "";
  }
}

async function loadMembers(groupID) {
  openGroup = groupID;
  const members = await api("/api/admin/groups/" + groupID + "/members");
  $("group-members").innerHTML = `
    <h3>${t("멤버")} <code>${esc(groupID.slice(0, 10))}…</code></h3>
    <div class="row">
      <input id="new-member" class="grow" placeholder="${t("추가할 pubkey (hex)")}" spellcheck="false">
      <button id="add-member">${t("추가")}</button>
    </div>
    <div class="chips">` +
    members.map((m) => `
      <span class="chip" data-member="${esc(m.pubkey)}">
        <code>${esc(m.pubkey.slice(0, 24))}…</code>
        <span class="sub">${fmtDate(m.added_at)}</span>
        <svg class="i member-del"><use href="#i-x"/></svg>
      </span>`).join("") +
    (members.length ? "" : `<span class="muted small">${t("멤버 없음")}</span>`) + `</div>`;

  $("add-member").onclick = async () => {
    const pubkey = $("new-member").value.trim().toLowerCase();
    if (!/^[0-9a-f]{64}$/.test(pubkey)) return alert("64자리 hex pubkey를 입력하세요.");
    try {
      await api(`/api/admin/groups/${groupID}/members/${pubkey}`, "PUT", {});
      loadGroups();
    } catch (err) {
      alert(err.message);
    }
  };
}

async function saveGroupRow(tr) {
  const expires = tr.querySelector(".f-expires").value;
  await api("/api/admin/groups/" + tr.dataset.group, "PUT", {
    name: tr.querySelector(".f-name").value,
    owner: tr.dataset.owner,
    status: tr.querySelector(".f-status").value,
    expires_at: expires ? Math.floor(new Date(expires).getTime() / 1000) : 0,
    quota_bytes: Math.round(Number(tr.querySelector(".f-quota").value) * MB),
    note: tr.querySelector(".f-note").value,
  });
  loadGroups();
}

async function addGroup() {
  const owner = $("new-group-owner").value.trim().toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(owner)) return alert(t("소유자 pubkey를 64자리 hex로 입력하세요."));
  const days = Number($("new-group-days").value) || 0;
  // the id is ours to pick; a random hex string keeps it unguessable, which
  // matters because knowing it is enough to read the group's usage
  const id = [...crypto.getRandomValues(new Uint8Array(16))]
    .map((b) => b.toString(16).padStart(2, "0")).join("");

  await api("/api/admin/groups/" + id, "PUT", {
    name: $("new-group-name").value.trim(),
    owner,
    status: "active",
    expires_at: days ? Math.floor(Date.now() / 1000) + days * 86400 : 0,
    quota_bytes: Math.round((Number($("new-group-mb").value) || 0) * MB),
    note: "manual",
  });
  $("new-group-name").value = "";
  $("new-group-owner").value = "";
  loadGroups();
}

// --- NIP-05 domains and names ---

async function loadDomains() {
  const domains = await api("/api/admin/nip05/domains");
  $("domains").innerHTML =
    cols("", 80, 120, 100, "", 110) +
    `<tr><th>${t("도메인")}</th><th>${t("판매중")}</th><th>${t("가격 (sats)")}</th><th>${t("기간 (일)")}</th><th>${t("메모")}</th><th></th></tr>` +
    domains.map((d) => `
      <tr data-domain="${esc(d.domain)}">
        <td><code>${esc(d.domain)}</code></td>
        <td><input class="f-enabled" type="checkbox"${d.enabled ? " checked" : ""}></td>
        <td><input class="f-price" type="number" value="${d.price_sats}"></td>
        <td><input class="f-days" type="number" value="${d.period_days}"></td>
        <td><input class="f-note" value="${esc(d.note)}"></td>
        <td><button class="save link">${t("저장")}</button> <button class="del link danger">${t("삭제")}</button></td>
      </tr>`).join("") +
    (domains.length ? "" : `<tr><td colspan="6" class="empty">${t("도메인 없음")}</td></tr>`);
}

async function saveDomainRow(tr) {
  await api("/api/admin/nip05/domains/" + encodeURIComponent(tr.dataset.domain), "PUT", {
    enabled: tr.querySelector(".f-enabled").checked,
    price_sats: Number(tr.querySelector(".f-price").value) || 0,
    period_days: Number(tr.querySelector(".f-days").value) || 0,
    note: tr.querySelector(".f-note").value,
  });
  loadDomains();
}

async function addDomain() {
  const domain = $("new-domain").value.trim().toLowerCase();
  if (!domain) return alert(t("도메인을 입력하세요."));
  await api("/api/admin/nip05/domains/" + encodeURIComponent(domain), "PUT", {
    enabled: true,
    price_sats: Number($("new-domain-price").value) || 0,
    period_days: Number($("new-domain-days").value) || 365,
    note: "",
  });
  $("new-domain").value = "";
  loadDomains();
}

async function loadBlockedNames() {
  const blocked = await api("/api/admin/nip05/blocked");
  $("blocked-names").innerHTML = blocked.length
    ? `<div class="chips">` + blocked.map((b) => `
        <span class="chip" data-blocked="${esc(b.value)}">
          ${esc(b.value)}
          ${b.reason ? `<span class="sub">${esc(b.reason)}</span>` : ""}
          <svg class="i unblock"><use href="#i-x"/></svg>
        </span>`).join("") + `</div>`
    : `<p class="muted small">${t("차단한 이름 없음")}</p>`;
}

async function addBlockedName() {
  const name = $("new-blocked").value.trim().toLowerCase();
  if (!name) return alert(t("이름을 입력하세요."));
  await api("/api/admin/nip05/blocked/" + encodeURIComponent(name), "PUT", {
    reason: $("new-blocked-reason").value.trim(),
  });
  $("new-blocked").value = "";
  $("new-blocked-reason").value = "";
  loadBlockedNames();
}

async function loadNames() {
  const q = encodeURIComponent($("name-search").value || "");
  const names = await api("/api/admin/nip05/names?q=" + q);
  $("names").innerHTML =
    cols("", "", 190, 80, 110) +
    `<tr><th>${t("주소")}</th><th>pubkey</th><th>${t("만료")}</th><th>${t("무기한")}</th><th></th></tr>` +
    names.map((n) => `
      <tr data-domain="${esc(n.domain)}" data-name="${esc(n.name)}">
        <td><code>${esc(n.name)}@${esc(n.domain)}</code>
            ${!n.permanent && !n.expires_at ? `<span class="bad small"> ${t("결제 대기")}</span>` : ""}</td>
        <td><input class="f-pubkey" value="${esc(n.pubkey)}" spellcheck="false"
                   title="${esc(n.pubkey)}"></td>
        <td><input class="f-expires" type="datetime-local" value="${toLocalInput(n.expires_at)}"></td>
        <td><input class="f-permanent" type="checkbox"${n.permanent ? " checked" : ""}></td>
        <td><button class="save link">${t("저장")}</button> <button class="del link danger">${t("삭제")}</button></td>
      </tr>`).join("") +
    (names.length ? "" : `<tr><td colspan="5" class="empty">${t("판매된 이름 없음")}</td></tr>`);
}

async function saveNameRow(tr) {
  const expires = tr.querySelector(".f-expires").value;
  await api(nameURL(tr.dataset.domain, tr.dataset.name), "PUT", {
    pubkey: tr.querySelector(".f-pubkey").value.trim().toLowerCase(),
    expires_at: expires ? Math.floor(new Date(expires).getTime() / 1000) : 0,
    permanent: tr.querySelector(".f-permanent").checked,
  });
  loadNames();
}

const nameURL = (domain, name) =>
  `/api/admin/nip05/names/${encodeURIComponent(domain)}/${encodeURIComponent(name)}`;

async function addName() {
  const name = $("new-name").value.trim().toLowerCase();
  const domain = $("new-name-domain").value.trim().toLowerCase();
  const pubkey = $("new-name-pubkey").value.trim().toLowerCase();
  if (!name || !domain) return alert(t("이름과 도메인을 입력하세요."));
  if (!/^[0-9a-f]{64}$/.test(pubkey)) return alert("64자리 hex pubkey를 입력하세요.");

  const days = Number($("new-name-days").value) || 0;
  await api(nameURL(domain, name), "PUT", {
    pubkey,
    expires_at: days ? Math.floor(Date.now() / 1000) + days * 86400 : 0,
    permanent: days === 0,
  });
  $("new-name").value = "";
  $("new-name-pubkey").value = "";
  loadNames();
}

// --- media (Blossom / NIP-96) ---

async function loadBlobs() {
  const [blobs, reports] = await Promise.all([
    api("/api/admin/blobs?limit=100"),
    api("/api/admin/blobs/reports"),
  ]);

  $("blob-reports").innerHTML = reports.length
    ? `<p class="bad small">${t("신고된 파일 {0}건", reports.length)}</p>` +
      reports.map((rep) => `
        <div class="row" data-report="${rep.value}">
          <code>${rep.value.slice(0, 12)}…</code>
          <span class="muted small grow">${esc(rep.reason)}</span>
          <button class="dismiss secondary">${t("신고 무시")}</button>
        </div>`).join("")
    : "";

  $("blobs").innerHTML =
    cols("", 130, 90, 130, 170, 90) +
    `<tr><th>${t("파일")}</th><th>${t("업로더")}</th><th>${t("크기")}</th><th>${t("형식")}</th><th>${t("올린 시각")}</th><th></th></tr>` +
    blobs.map((b) => `
      <tr data-blob="${b.sha256}">
        <td><a href="${esc(b.url)}" target="_blank" rel="noopener"><code>${b.sha256.slice(0, 12)}…</code></a>
            ${b.reported ? `<span class="bad small"> ${t("신고됨")}</span>` : ""}</td>
        <td><code>${b.pubkey.slice(0, 10)}…</code></td>
        <td class="num">${fmtBytes(b.size)}</td>
        <td class="small">${esc(b.type || "")}</td>
        <td class="small">${fmtDate(b.uploaded)}</td>
        <td><button class="blob-del link danger">${t("삭제")}</button></td>
      </tr>`).join("") +
    (blobs.length ? "" : `<tr><td colspan="6" class="empty">${t("업로드된 파일 없음")}</td></tr>`);
}

async function loadPayments() {
  const payments = await api("/api/admin/payments?limit=50");
  $("payments").innerHTML =
    cols(170, 160, "", 110, 90) +
    `<tr><th>${t("시각")}</th><th>pubkey</th><th>${t("종류")}</th><th>sats</th><th>${t("상태")}</th></tr>` +
    payments.map((p) => `
      <tr>
        <td class="small">${fmtDate(p.created_at)}</td>
        <td><code>${p.pubkey.slice(0, 12)}…</code></td>
        <td>${p.kind}</td>
        <td class="num">${p.sats.toLocaleString()}</td>
        <td class="${p.status === "paid" ? "ok" : p.status === "expired" ? "bad" : "muted"}">${p.status}</td>
      </tr>`).join("") +
    (payments.length ? "" : `<tr><td colspan="5" class="empty">${t("결제 없음")}</td></tr>`);
}

// signIn proves to the server that `next` signs for an admin pubkey. The same
// signer then signs every later request, so a remote one has to stay open.
async function signIn(next) {
  mode = "nostr";
  signer = next;
  $("login-error").textContent = "";
  try {
    await api("/api/admin/session");
    loadAll();
  } catch (err) {
    mode = null;
    signer = null;
    $("login-error").textContent = err.message;
  }
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
  if (update.message) $("login-status").textContent = update.message;
}

$("login-nostr").onclick = () => {
  if (!window.nostr) return ($("login-error").textContent = t("NIP-07 확장이 없습니다."));
  signIn(window.nostr);
};

$("bunker-connect").onclick = async () => {
  const uri = $("bunker-uri").value.trim();
  if (!uri) return;
  $("bunker-connect").disabled = true;
  $("login-status").textContent = t("서명자에 연결하는 중…");
  try {
    const remote = await window.nip46.connectBunker(uri, onSignerStatus);
    $("login-status").textContent = t("원격 서명자에 연결되었습니다.");
    await signIn(remote);
  } catch (err) {
    $("login-error").textContent = err.message;
  } finally {
    $("bunker-connect").disabled = false;
  }
};

$("nc-start").onclick = async () => {
  const relays = (info?.nip46_relays || "").split(",").map((r) => r.trim()).filter(Boolean);
  if (!relays.length) {
    return ($("login-error").textContent = t("이 릴레이는 nostrconnect relay가 설정되어 있지 않습니다."));
  }

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
  $("login-status").textContent = t("지갑 앱으로 QR을 스캔하거나 연결 문자열을 붙여넣으세요");

  try {
    const remote = await nostrconnect.connected;
    $("login-status").textContent = t("원격 서명자에 연결되었습니다.");
    await signIn(remote);
  } catch (err) {
    $("login-error").textContent = err.message;
  }
};

$("nc-copy").onclick = () => navigator.clipboard.writeText($("nc-uri").value);

$("login-password").onclick = async () => {
  try {
    await fetch("/api/admin/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: $("password").value }),
    }).then(async (res) => {
      if (!res.ok) throw new Error((await res.json()).error || res.statusText);
    });
    mode = "session";
    loadAll();
  } catch (err) {
    $("login-error").textContent = err.message;
  }
};

$("logout").onclick = async (e) => {
  e.preventDefault();
  await fetch("/api/admin/logout", { method: "POST" });
  location.reload();
};

// One nav entry, one page: the hash picks the single card the main column
// shows, and the card's own <h2> becomes the page heading (CSS hides the h2).
const navLinks = [...document.querySelectorAll('.nav a[href^="#sec-"]')];
const pages = [...document.querySelectorAll('.main > section[id^="sec-"]')];
const main = document.querySelector(".main");

function route() {
  const id = location.hash.slice(1);
  const page = pages.find((p) => p.id === id) || pages[0];
  for (const p of pages) p.hidden = p !== page;
  for (const link of navLinks) link.classList.toggle("active", link.hash === "#" + page.id);
  $("page-title").textContent = page.querySelector("h2").textContent;
  main.scrollTop = 0;
}
addEventListener("hashchange", route);
route();

$("save-settings").onclick = saveSettings;
$("payment-provider").onchange = togglePaymentFields;
$("storage-backend").onchange = toggleStorageFields;
$("test-storage").onclick = testStorage;
$("save-storage").onclick = saveStorage;
$("test-payment").onclick = testPayment;
$("save-payment").onclick = savePayment;
$("do-search").onclick = loadAccounts;
$("add-account").onclick = addAccount;
$("accounts").addEventListener("click", (e) => {
  const tr = e.target.closest("tr[data-pubkey]");
  if (!tr) return;
  if (e.target.classList.contains("save")) saveRow(tr).catch((err) => alert(err.message));
  if (e.target.classList.contains("del") && confirm(t("이 계정을 삭제할까요? (이벤트는 남습니다)"))) {
    api("/api/admin/accounts/" + tr.dataset.pubkey, "DELETE").then(loadAccounts).catch((err) => alert(err.message));
  }
});

$("add-invite").onclick = () => addInvite().catch((err) => alert(err.message));
$("invites").addEventListener("click", (e) => {
  const tr = e.target.closest("tr[data-invite]");
  if (!tr) return;
  if (e.target.classList.contains("copy")) {
    navigator.clipboard?.writeText(tr.dataset.invite).catch(() => {});
  }
  if (e.target.classList.contains("del") && confirm(t("이 초대 코드를 삭제할까요? 이미 쓴 사람은 그대로입니다."))) {
    api("/api/admin/invites/" + encodeURIComponent(tr.dataset.invite), "DELETE")
      .then(loadInvites).catch((err) => alert(err.message));
  }
});

$("do-group-search").onclick = loadGroups;
$("add-group").onclick = () => addGroup().catch((err) => alert(err.message));
$("groups").addEventListener("click", (e) => {
  const tr = e.target.closest("tr[data-group]");
  if (!tr) return;
  if (e.target.classList.contains("members")) {
    loadMembers(tr.dataset.group).catch((err) => alert(err.message));
  }
  if (e.target.classList.contains("save")) saveGroupRow(tr).catch((err) => alert(err.message));
  if (e.target.classList.contains("del") &&
      confirm(t("이 그룹을 삭제할까요? 멤버는 각자 계정으로 돌아갑니다."))) {
    api("/api/admin/groups/" + tr.dataset.group, "DELETE").then(loadGroups).catch((err) => alert(err.message));
  }
});

$("group-members").addEventListener("click", (e) => {
  const row = e.target.closest("[data-member]");
  if (!row || !e.target.closest(".member-del")) return;
  if (!confirm(t("이 멤버를 그룹에서 뺄까요?"))) return;
  api(`/api/admin/groups/${openGroup}/members/${row.dataset.member}`, "DELETE")
    .then(loadGroups).catch((err) => alert(err.message));
});

$("add-domain").onclick = () => addDomain().catch((err) => alert(err.message));
$("domains").addEventListener("click", (e) => {
  const tr = e.target.closest("tr[data-domain]");
  if (!tr) return;
  if (e.target.classList.contains("save")) saveDomainRow(tr).catch((err) => alert(err.message));
  if (e.target.classList.contains("del") &&
      confirm(t("이 도메인을 지울까요? 이 도메인으로 판매된 주소가 모두 사라집니다."))) {
    api("/api/admin/nip05/domains/" + encodeURIComponent(tr.dataset.domain), "DELETE")
      .then(() => { loadDomains(); loadNames(); }).catch((err) => alert(err.message));
  }
});

$("add-blocked").onclick = () => addBlockedName().catch((err) => alert(err.message));
$("blocked-names").addEventListener("click", (e) => {
  const row = e.target.closest("[data-blocked]");
  if (!row || !e.target.closest(".unblock")) return;
  api("/api/admin/nip05/blocked/" + encodeURIComponent(row.dataset.blocked), "DELETE")
    .then(loadBlockedNames).catch((err) => alert(err.message));
});

$("do-name-search").onclick = loadNames;
$("add-name").onclick = () => addName().catch((err) => alert(err.message));
$("names").addEventListener("click", (e) => {
  const tr = e.target.closest("tr[data-name]");
  if (!tr) return;
  if (e.target.classList.contains("save")) saveNameRow(tr).catch((err) => alert(err.message));
  if (e.target.classList.contains("del") && confirm(t("이 주소를 회수할까요?"))) {
    api(nameURL(tr.dataset.domain, tr.dataset.name), "DELETE")
      .then(loadNames).catch((err) => alert(err.message));
  }
});

$("blobs").addEventListener("click", (e) => {
  const tr = e.target.closest("tr[data-blob]");
  if (!tr || !e.target.classList.contains("blob-del")) return;
  if (!confirm(t("이 파일을 서버에서 지울까요? 모든 소유자에게서 제거되고 용량이 환불됩니다."))) return;
  api("/api/admin/blobs/" + tr.dataset.blob, "DELETE").then(loadBlobs).catch((err) => alert(err.message));
});

$("blob-reports").addEventListener("click", (e) => {
  const row = e.target.closest("[data-report]");
  if (!row || !e.target.classList.contains("dismiss")) return;
  api("/api/admin/blobs/reports/" + row.dataset.report, "DELETE").then(loadBlobs).catch((err) => alert(err.message));
});

// the theme is public, so the login screen wears it too; the same document
// carries the nostrconnect relays the QR login needs
fetch("/api/info").then((res) => res.json()).then((data) => {
  info = data;
  applyTheme(data);
}).catch(() => {});

// already logged in with a password session?
fetch("/api/admin/session").then((res) => {
  if (res.ok) {
    mode = "session";
    loadAll();
  }
});
