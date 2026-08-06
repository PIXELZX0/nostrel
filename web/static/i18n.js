// Panel translations.
//
// The Korean source string is the key, so Korean needs no dictionary and a
// missing translation falls back to it rather than to an empty box. Static
// markup is translated once by walking the document; anything JavaScript
// builds goes through t() instead, which also fills {0}, {1}, … placeholders.
//
// Adding a language: add one object to `dict` and one <option> to the pickers
// in index.html and admin.html. Nothing else knows the list.

const dict = {
  en: {
    // --- shell, navigation ---
    "요금": "Pricing",
    "내 계정": "My account",
    "가입 · 갱신": "Join · renew",
    "초대 코드": "Invite codes",
    "NIP-05 주소": "NIP-05 identifiers",
    "그룹": "Groups",
    "결제": "Payment",
    "관리자": "Admin",
    "현황": "Overview",
    "결제 백엔드": "Payment backend",
    "파일 저장소": "File storage",
    "요금 · 릴레이 정보": "Pricing · relay info",
    "계정": "Accounts",
    "미디어": "Media",
    "결제 내역": "Payment history",
    "로그아웃": "Log out",
    "언어": "Language",

    // --- public panel ---
    "라이트닝으로 결제하면 해당 pubkey가 쓰기 화이트리스트에 등록됩니다. 읽기는 별도 안내가 없으면 공개입니다.":
      "Paying over lightning puts that pubkey on the write whitelist. Reading is public unless stated otherwise.",
    "확장 프로그램 (NIP-07)": "Browser extension (NIP-07)",
    "확장으로 서명 로그인": "Sign in with an extension",
    "또는": "or",
    "연결": "Connect",
    "QR 스캔 (nostrconnect)": "Scan a QR (nostrconnect)",
    "QR 만들기": "Generate a QR",
    "복사": "Copy",
    "읽기 전용": "Read-only",
    "pubkey(hex) 붙여넣기": "paste a pubkey (hex)",
    "코드 사용": "Redeem",
    "초대 코드가 있으면 가입비 없이 등록됩니다.":
      "An invite code registers you without the admission fee.",
    "가입 · 갱신 · 용량 충전": "Join · renew · top up storage",
    "구독 기간": "Subscription",
    "회": "periods",
    "추가 용량": "Extra storage",
    "인보이스 발행": "Create an invoice",
    "기간": "Period",
    "확인": "Check",
    "원하는 이름": "the name you want",
    "그룹 (용량 공유)": "Groups (shared storage)",
    "새 그룹 이름": "new group name",
    "이미 그룹이 있으면 같은 버튼이 연장·충전이 됩니다. 멤버는 자기 계정을 먼저 쓰고, 개인 용량이 없거나 다 쓴 뒤에만 그룹 용량을 씁니다.":
      "If a group already exists, the same button extends and tops it up. Members spend their own account first, and only fall back to the group's storage once they have none of their own left.",

    // --- public panel, built in JavaScript ---
    "무기한": "never",
    "멤버 관리에는 서명 로그인이 필요합니다.": "Managing members needs a signed login.",
    "가입비 (최초 1회)": "Admission fee (once)",
    "구독 ({0}일)": "Subscription ({0} days)",
    "구독 포함 용량": "Storage included",
    "NIP-05 도메인": "NIP-05 domains",
    "{0}개 판매 중": "{0} on sale",
    "아직 등록되지 않음. 아래에서 가입하세요.": "Not registered yet — join below.",
    "만료 {0}": "expires {0}",
    "쓰기 가능": "can write",
    "쓰기 불가": "cannot write",
    "{0} / {1} 사용": "{0} / {1} used",
    "판매 중인 도메인이 없습니다.": "No domains are on sale.",
    "보유 중인 주소": "Identifiers you hold",
    "{0}@{1} 구매 가능 — {2}": "{0}@{1} is available — {2}",
    "아직 그룹이 없습니다. 이름을 정해 만드세요.": "No group yet. Pick a name and create one.",
    "소유자": "owner",
    "멤버 관리는 그룹 소유자만 할 수 있습니다.": "Only the group owner can manage members.",
    "멤버를 보려면 서명이 필요합니다.": "Seeing the members needs a signature.",
    "불러오기": "Load",
    "멤버": "Members",
    "추가할 pubkey (hex)": "pubkey to add (hex)",
    "추가": "Add",
    "64자리 hex pubkey를 입력하세요.": "Enter a 64-character hex pubkey.",
    "구독 {0}회 + 추가 {1}MB ≈ {2} (신규 가입이면 가입비 {3} 추가)":
      "{0} periods + {1} MB extra ≈ {2} (plus a {3} admission fee if you are new)",
    "NIP-07 확장(Alby, nos2x 등)이 없습니다. bunker 연결이나 QR을 쓰거나 pubkey를 붙여넣으세요.":
      "No NIP-07 extension (Alby, nos2x, …). Use a bunker connection or a QR, or paste a pubkey.",
    "서명자 승인 창 열기": "Open the signer's approval window",
    "서명자에 연결하는 중…": "Connecting to the signer…",
    "원격 서명자에 연결되었습니다.": "Connected to the remote signer.",
    "이 릴레이는 nostrconnect relay가 설정되어 있지 않습니다.":
      "This relay has no nostrconnect relay configured.",
    "지갑 앱으로 QR을 스캔하거나 연결 문자열을 붙여넣으세요":
      "Scan the QR with your wallet app, or paste the connection string",
    "{0} 결제 대기 중…": "waiting for {0}…",
    "결제 완료.": "Paid.",
    "인보이스 만료. 다시 발행하세요.": "Invoice expired — issue a new one.",
    "등록되었습니다.": "You are registered.",
    "이 멤버를 그룹에서 뺄까요?": "Remove this member from the group?",
    "읽기 전용입니다. 멤버 관리에는 서명 로그인이 필요합니다.":
      "Read-only. Managing members needs a signed login.",
    "64자리 hex pubkey를 입력하세요 (npub은 확장 연결을 사용하세요).":
      "Enter a 64-character hex pubkey (for an npub, connect an extension instead).",

    // --- admin: login ---
    "NIP-07 서명 로그인 또는 관리자 비밀번호": "NIP-07 signed login, or the admin password",
    "로그인": "Log in",
    "관리자 비밀번호": "admin password",
    "NIP-07 확장이 없습니다.": "No NIP-07 extension found.",

    // --- admin: payment backend ---
    "백엔드": "Backend",
    "mock (테스트 전용)": "mock (testing only)",
    "LNbits 주소": "LNbits address",
    "인보이스/읽기 키": "Invoice / read key",
    "인보이스 키": "invoice key",
    "연결 문자열": "Connection string",
    "연결 테스트": "Test connection",
    "저장": "Save",
    "키는 서버에만 저장되고 화면에는 끝 4자리만 표시됩니다. 그대로 두면 기존 값이 유지됩니다.":
      "Keys are stored server-side only and shown as their last four characters. Leave a field alone to keep its current value.",
    "mock은 결제 없이 인보이스가 스스로 정산됩니다. 운영에서 쓰지 마세요.":
      "mock settles invoices by itself, with no payment. Do not use it in production.",
    "테스트 중…": "Testing…",
    "저장됨 — 새 인보이스부터 이 백엔드를 씁니다.":
      "Saved — this backend is used from the next invoice on.",

    // --- admin: storage ---
    "저장 위치": "Storage location",
    "내장 (서버 디스크)": "Built-in (server disk)",
    "S3 호환 (S3 · MinIO · R2 · B2)": "S3-compatible (S3 · MinIO · R2 · B2)",
    "저장 경로": "Storage path",
    "엔드포인트": "Endpoint",
    "https://s3.amazonaws.com 또는 minio.example.com:9000":
      "https://s3.amazonaws.com or minio.example.com:9000",
    "버킷": "Bucket",
    "리전": "Region",
    "키 접두사 (선택)": "Key prefix (optional)",
    "액세스 키": "Access key",
    "시크릿 키": "Secret key",
    "공개 URL (선택, 설정 시 다운로드를 이 주소로 리다이렉트)":
      "Public URL (optional; downloads are redirected here when set)",
    "저장 위치를 바꿔도 기존 파일은 옮겨지지 않습니다. 옮기려면":
      "Changing the storage location does not move existing files. To move them, run",
    "nostrel migrate-blobs <기존경로>": "nostrel migrate-blobs <old-path>",
    "를 실행하세요.": ".",
    "현재: {0} ({1})": "Currently: {0} ({1})",
    "현재: {0}": "Currently: {0}",
    "기본 경로": "the default path",
    "저장됨 — 새 업로드부터 이 위치를 씁니다. 기존 파일은 migrate-blobs로 옮기세요.":
      "Saved — new uploads go here. Move the existing files with migrate-blobs.",

    // --- admin: settings ---
    "릴레이 이름": "Relay name",
    "설명": "Description",
    "로고 · 아이콘 URL": "Logo / icon URL",
    "배너 URL": "Banner URL",
    "운영자 연락처 (mailto: 등)": "Operator contact (mailto:, …)",
    "운영자 pubkey (hex, 비우면 릴레이 키)":
      "Operator pubkey (hex; empty = the relay's own key)",
    "배경색": "Background colour",
    "포인트 색": "Accent colour",
    "배경 이미지 URL": "Background image URL",
    "보관 기간 (일, 0 = 무기한)": "Retention (days, 0 = forever)",
    "이용 약관 URL": "Posting policy URL",
    "운영 국가 (ISO 3166-1, 쉼표)": "Countries (ISO 3166-1, comma separated)",
    "언어 (ISO 639-1, 쉼표)": "Languages (ISO 639-1, comma separated)",
    "주제 태그 (쉼표)": "Topic tags (comma separated)",
    "가입비 (sats)": "Admission fee (sats)",
    "구독료 (sats)": "Subscription (sats)",
    "구독 기간 (일)": "Subscription period (days)",
    "구독 포함 용량 (MB)": "Storage included (MB)",
    "추가 용량 단가 (sats/MB)": "Extra storage (sats/MB)",
    "짧은 이름 할증 (길이:배수)": "Short-name premium (length:multiplier)",
    "NIP-46 로그인 relay (쉼표 구분)": "NIP-46 login relays (comma separated)",
    "nsite 호스팅 도메인 (NIP-5A, 쉼표)": "nsite hosting domains (NIP-5A, comma separated)",
    "제3자 zap 영수증 허용 (NIP-57)": "Allow third-party zap receipts (NIP-57)",
    "제3자 배지 수여 허용 (NIP-58)": "Allow third-party badge awards (NIP-58)",
    "누구나 초대 코드 요청 가능 (NIP-43 kind 28935)":
      "Anyone may request an invite code (NIP-43 kind 28935)",
    "자동 초대 기간 (일)": "Auto-invite period (days)",
    "자동 초대 용량 (MB)": "Auto-invite storage (MB)",
    "읽기도 인증·화이트리스트 필요 (NIP-42)":
      "Reading also needs auth and a whitelist entry (NIP-42)",
    "최소 작업증명 난이도 (NIP-13, 0 = 끔)":
      "Minimum proof-of-work difficulty (NIP-13, 0 = off)",
    "created_at 과거 허용치 (초, 0 = 무제한)":
      "created_at past limit (seconds, 0 = unbounded)",
    "created_at 미래 허용치 (초, 0 = 무제한)":
      "created_at future limit (seconds, 0 = unbounded)",
    "파일 1개 최대 크기 (MB)": "Maximum size of one file (MB)",
    "짧은 이름 할증은": "The short-name premium is a",
    "길이:배수": "length:multiplier",
    "목록입니다. 예를 들어": "list. For example,",
    "는 한 글자 이름을 도메인 가격의 20배로 팝니다. 비워두면 할증이 없습니다.":
      "prices a one-character name at 20× the domain price. Leave it empty for no premium.",
    "저장됨": "Saved",

    // --- admin: stats ---
    "{0} (활성 {1})": "{0} ({1} active)",
    "이벤트": "Events",
    "저장량": "Stored",
    "{0}건 / {1} sats": "{0} / {1} sats",

    // --- admin: accounts ---
    "pubkey 또는 메모 검색": "search pubkey or note",
    "검색": "Search",
    "수동 추가할 pubkey (hex)": "pubkey to add by hand (hex)",
    "일": "days",
    "용량": "Storage",
    "상태": "Status",
    "만료": "Expires",
    "사용/할당(MB)": "Used / allotted (MB)",
    "메모": "Note",
    "삭제": "Delete",
    "계정 없음": "No accounts",
    "이 계정을 삭제할까요? (이벤트는 남습니다)":
      "Delete this account? (its events stay)",

    // --- admin: invites ---
    "초대 코드 (NIP-43)": "Invite codes (NIP-43)",
    "메모 (예: 베타 초대)": "note (e.g. beta invite)",
    "사용": "Uses",
    "회 (0 = 무제한)": "uses (0 = unlimited)",
    "일 (0 = 없음)": "days (0 = never)",
    "발급": "Issue",
    "초대 코드는": "An invite code",
    "가입비를 면제": "waives the admission fee",
    "합니다. 코드를 클릭하면 복사됩니다.": ". Click a code to copy it. When",
    "가 설정돼 있으면 클라이언트가 kind 28934로 직접 가입할 수도 있습니다.":
      "is set, clients can also join directly with kind 28934.",
    "코드": "Code",
    "클릭하면 복사": "click to copy",
    "{0}일": "{0} days",
    "없음": "none",
    "초대 코드 없음": "No invite codes",
    "이 초대 코드를 삭제할까요? 이미 쓴 사람은 그대로입니다.":
      "Delete this invite code? People who already used it are unaffected.",

    // --- admin: groups ---
    "그룹 이름 · 소유자 · 메모 검색": "search group name, owner or note",
    "소유자 pubkey (hex)": "owner pubkey (hex)",
    "멤버는 자기 계정으로 먼저 쓰고, 개인 용량이 없거나 다 쓴 뒤에만 그룹 용량을 씁니다. 한 pubkey는 그룹 하나에만 속할 수 있습니다.":
      "Members spend their own account first and only fall back to the group's storage once they have none of their own left. A pubkey can belong to only one group.",
    "이름": "Name",
    "{0}명": "{0}",
    "그룹 없음": "No groups",
    "멤버 없음": "No members",
    "소유자 pubkey를 64자리 hex로 입력하세요.": "Enter the owner pubkey as 64 hex characters.",
    "이 그룹을 삭제할까요? 멤버는 각자 계정으로 돌아갑니다.":
      "Delete this group? Members fall back to their own accounts.",
    
    // --- admin: NIP-05 ---
    "도메인 (예: example.com)": "domain (e.g. example.com)",
    "가격": "Price",
    "도메인의 DNS를 이 서버로 향하게 하고, 리버스 프록시가 Host 헤더를 그대로 넘겨야":
      "Point the domain's DNS at this server and have the reverse proxy pass the Host header through, so that",
    "이 그 도메인의 이름을 응답합니다.": "answers with that domain's names.",
    "차단한 이름": "Blocked names",
    "판매 금지할 이름 (예: admin)": "name to keep off the market (e.g. admin)",
    "사유 (선택)": "reason (optional)",
    "차단": "Block",
    "판매된 이름": "Names sold",
    "이름 또는 pubkey 검색": "search name or pubkey",
    "일 (0 = 무기한)": "days (0 = forever)",
    "도메인": "Domain",
    "판매중": "On sale",
    "가격 (sats)": "Price (sats)",
    "기간 (일)": "Period (days)",
    "도메인 없음": "No domains",
    "도메인을 입력하세요.": "Enter a domain.",
    "이름을 입력하세요.": "Enter a name.",
    "이름과 도메인을 입력하세요.": "Enter a name and a domain.",
    "차단한 이름 없음": "No blocked names",
    "주소": "Identifier",
    "결제 대기": "awaiting payment",
    "판매된 이름 없음": "No names sold",
    "이 도메인을 지울까요? 이 도메인으로 판매된 주소가 모두 사라집니다.":
      "Delete this domain? Every identifier sold on it disappears.",
    "이 주소를 회수할까요?": "Reclaim this identifier?",

    // --- admin: media, payments ---
    "신고된 파일 {0}건": "{0} reported files",
    "신고 무시": "Dismiss",
    "파일": "File",
    "업로더": "Uploader",
    "크기": "Size",
    "형식": "Type",
    "올린 시각": "Uploaded",
    "신고됨": "reported",
    "업로드된 파일 없음": "No files uploaded",
    "이 파일을 서버에서 지울까요? 모든 소유자에게서 제거되고 용량이 환불됩니다.":
      "Delete this file from the server? It is removed from every owner and their storage is refunded.",
    "시각": "Time",
    "종류": "Kind",
    "결제 없음": "No payments",
  },
};

const LANGS = ["ko", "en"];

function pickLang() {
  const saved = localStorage.getItem("lang");
  if (LANGS.includes(saved)) return saved;
  // navigator.language is "en-US", "ko-KR", …
  const nav = (navigator.language || "").slice(0, 2);
  return LANGS.includes(nav) ? nav : "ko";
}

let lang = pickLang();

// t translates a Korean source string and fills {0}, {1}, … from the arguments.
function t(ko, ...args) {
  const s = (dict[lang] && dict[lang][ko]) || ko;
  return args.length ? s.replace(/\{(\d+)\}/g, (m, i) => (args[i] ?? m)) : s;
}

// translateDOM rewrites the static markup in place. Text is matched whole and
// trimmed, so surrounding whitespace in the HTML survives; a fragment with no
// entry is left in Korean rather than blanked.
function translateDOM(root = document.body) {
  if (lang === "ko") return;

  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const table = dict[lang];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const raw = node.nodeValue;
    const text = raw.trim();
    // markup wraps long sentences, so the key is normalized to single spaces
    // and the node's own surrounding whitespace is kept
    const key = text.replace(/\s+/g, " ");
    if (!key || !(key in table)) continue;
    node.nodeValue = raw.replace(text, table[key]);
  }

  for (const el of root.querySelectorAll("[placeholder], [title]")) {
    for (const attr of ["placeholder", "title"]) {
      const v = el.getAttribute(attr);
      if (v && v in table) el.setAttribute(attr, table[v]);
    }
  }
}

function setLang(next) {
  localStorage.setItem("lang", next);
  // a reload is the cheap way to re-render everything the panel already drew
  location.reload();
}

document.documentElement.lang = lang;
translateDOM();

// pickers are optional: a page without one still translates
for (const picker of document.querySelectorAll("select.lang")) {
  picker.value = lang;
  picker.onchange = () => setLang(picker.value);
}
