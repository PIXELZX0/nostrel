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
    "미디어 서버": "Media server",
    "인보이스 발행 중…": "Creating the invoice…",
    "로그인 없이 요금만 보기": "Browse the pricing without signing in",
    "미디어 서버 (Blossom · NIP-96)": "Media server (Blossom · NIP-96)",
    "이 릴레이가 미디어 서버를 겸합니다. 아래 주소를 클라이언트의 미디어 서버 설정에 넣으면 업로드한 파일이 여기에 저장되고, 계정 용량에서 차감됩니다.":
      "This relay is its own media server. Put the address below into your client's media server setting and uploads land here, charged against your account's storage.",
    "Blossom 서버 주소": "Blossom server address",
    "NIP-96 서버 정보": "NIP-96 server info",
    "지원: BUD-01 · 02 · 04 · 05 · 06 · 09. 업로드에는 쓰기 권한이 있는 계정이 필요합니다.":
      "Supported: BUD-01 · 02 · 04 · 05 · 06 · 09. Uploading needs an account with write access.",
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
    "구독제": "Subscription plan",
    "영구 용량제": "Lifetime storage plan",
    "구독 기간": "Subscription",
    "회": "periods",
    "추가 용량": "Extra storage",
    "영구 용량": "Lifetime storage",
    "한 번 결제하면 만료도 갱신도 없습니다. 구독 갱신은 더 이상 필요하지 않습니다.":
      "Pay once and nothing expires or has to be renewed. There is no subscription to keep up afterwards.",
    "영구": "Lifetime",
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
    "영구 용량 (1회 결제)": "Lifetime storage (paid once)",
    "영구 (만료 없음)": "lifetime, never expires",
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
    "{0}@{1} 영구 구매 가능 — {2}": "{0}@{1} is available outright — {2}",
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
    "영구 {0}MB ≈ {1} (신규 가입이면 가입비 {2} 추가)":
      "{0} MB for life ≈ {1} (plus a {2} admission fee if you are new)",
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
    "NIP-07 · NIP-46 서명 로그인 또는 관리자 비밀번호":
      "NIP-07 or NIP-46 signed login, or the admin password",
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
    "구독제 판매": "Sell the subscription plan",
    "구독료 (sats)": "Subscription (sats)",
    "구독 기간 (일)": "Subscription period (days)",
    "구독 포함 용량 (MB)": "Storage included (MB)",
    "추가 용량 단가 (sats/MB)": "Extra storage (sats/MB)",
    "영구 용량제 판매 (1회 결제, 만료 없음)":
      "Sell the lifetime plan (paid once, never expires)",
    "영구 용량 단가 (sats/MB)": "Lifetime storage (sats/MB)",
    "짧은 이름 할증 (길이:배수)": "Short-name premium (length:multiplier)",
    "NIP-46 로그인 relay (쉼표 구분)": "NIP-46 login relays (comma separated)",
    "nsite 호스팅 도메인 (NIP-5A, 쉼표)": "nsite hosting domains (NIP-5A, comma separated)",
    "외부에서 오는 DM 허용 (NIP-04 · NIP-17, 끄면 비공개 채팅 불가)":
      "Accept direct messages from outside (NIP-04 · NIP-17; off means no private chat)",
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
    "이 그 도메인의 이름을 응답합니다. 테스트 버튼은 서버가 바깥에서 그 주소를 직접 불러 확인합니다.":
      "answers with that domain's names. The test button has the server fetch that URL from the outside.",
    "Host 헤더를 넘길 수 없는 프록시라면, 도메인을 경로에 담은 아래 주소가 같은 문서를 응답합니다. 프록시에서 그 도메인의 nostr.json 요청을 이쪽으로 넘기세요.":
      "When a proxy in front cannot pass the Host header, the addresses below answer the same document with the domain in the path. Point that domain's nostr.json requests here.",
    "테스트": "Test",
    "{0} 확인 중…": "Checking {0}…",
    "응답 코드": "Status",
    "이 서버의 답": "This server says",
    "도메인의 답": "The domain says",
    "CORS 헤더 없음": "No CORS header",
    "정상": "Working",
    "실패": "Failed",
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
    "영구 가격 (sats, 0 = 안 팜)": "Outright price (sats, 0 = not sold)",
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

  ja: {
    // --- shell, navigation ---
    "요금": "料金",
    "내 계정": "マイアカウント",
    "가입 · 갱신": "登録・更新",
    "초대 코드": "招待コード",
    "NIP-05 주소": "NIP-05 識別子",
    "그룹": "グループ",
    "결제": "決済",
    "미디어 서버": "メディアサーバー",
    "인보이스 발행 중…": "インボイスを作成しています…",
    "로그인 없이 요금만 보기": "ログインせずに料金だけ見る",
    "미디어 서버 (Blossom · NIP-96)": "メディアサーバー (Blossom · NIP-96)",
    "이 릴레이가 미디어 서버를 겸합니다. 아래 주소를 클라이언트의 미디어 서버 설정에 넣으면 업로드한 파일이 여기에 저장되고, 계정 용량에서 차감됩니다.":
      "このリレーはメディアサーバーも兼ねています。下のアドレスをクライアントのメディアサーバー設定に入れると、アップロードしたファイルはここに保存され、アカウントの容量から差し引かれます。",
    "Blossom 서버 주소": "Blossom サーバーアドレス",
    "NIP-96 서버 정보": "NIP-96 サーバー情報",
    "지원: BUD-01 · 02 · 04 · 05 · 06 · 09. 업로드에는 쓰기 권한이 있는 계정이 필요합니다.":
      "対応: BUD-01 · 02 · 04 · 05 · 06 · 09。アップロードには書き込み権限のあるアカウントが必要です。",
    "관리자": "管理者",
    "현황": "概況",
    "결제 백엔드": "決済バックエンド",
    "파일 저장소": "ファイルストレージ",
    "요금 · 릴레이 정보": "料金・リレー情報",
    "계정": "アカウント",
    "미디어": "メディア",
    "결제 내역": "決済履歴",
    "로그아웃": "ログアウト",
    "언어": "言語",

    // --- public panel ---
    "라이트닝으로 결제하면 해당 pubkey가 쓰기 화이트리스트에 등록됩니다. 읽기는 별도 안내가 없으면 공개입니다.":
      "ライトニングで支払うと、その pubkey が書き込みホワイトリストに登録されます。読み取りは特に案内がなければ公開です。",
    "확장 프로그램 (NIP-07)": "拡張機能 (NIP-07)",
    "확장으로 서명 로그인": "拡張機能で署名ログイン",
    "또는": "または",
    "연결": "接続",
    "QR 스캔 (nostrconnect)": "QR をスキャン (nostrconnect)",
    "QR 만들기": "QR を生成",
    "복사": "コピー",
    "읽기 전용": "読み取り専用",
    "pubkey(hex) 붙여넣기": "pubkey (hex) を貼り付け",
    "코드 사용": "コードを使う",
    "초대 코드가 있으면 가입비 없이 등록됩니다.":
      "招待コードがあれば加入料なしで登録できます。",
    "가입 · 갱신 · 용량 충전": "登録・更新・容量チャージ",
    "구독제": "サブスクリプション",
    "영구 용량제": "買い切り容量プラン",
    "구독 기간": "契約期間",
    "회": "回",
    "추가 용량": "追加容量",
    "영구 용량": "買い切り容量",
    "한 번 결제하면 만료도 갱신도 없습니다. 구독 갱신은 더 이상 필요하지 않습니다.":
      "一度支払えば期限も更新もありません。以後サブスクリプションの更新は不要です。",
    "영구": "買い切り",
    "인보이스 발행": "インボイス発行",
    "기간": "期間",
    "확인": "確認",
    "원하는 이름": "希望する名前",
    "그룹 (용량 공유)": "グループ (容量の共有)",
    "새 그룹 이름": "新しいグループ名",
    "이미 그룹이 있으면 같은 버튼이 연장·충전이 됩니다. 멤버는 자기 계정을 먼저 쓰고, 개인 용량이 없거나 다 쓴 뒤에만 그룹 용량을 씁니다.":
      "すでにグループがある場合、同じボタンが延長・チャージになります。メンバーはまず自分のアカウントを使い、個人の容量がないか使い切ったときだけグループの容量を使います。",

    // --- public panel, built in JavaScript ---
    "무기한": "無期限",
    "멤버 관리에는 서명 로그인이 필요합니다.": "メンバー管理には署名ログインが必要です。",
    "가입비 (최초 1회)": "加入料 (初回のみ)",
    "구독 ({0}일)": "サブスクリプション ({0} 日)",
    "구독 포함 용량": "サブスクに含まれる容量",
    "영구 용량 (1회 결제)": "買い切り容量 (1 回払い)",
    "영구 (만료 없음)": "買い切り (期限なし)",
    "NIP-05 도메인": "NIP-05 ドメイン",
    "{0}개 판매 중": "{0} 件販売中",
    "아직 등록되지 않음. 아래에서 가입하세요.": "まだ登録されていません。下から登録してください。",
    "만료 {0}": "期限 {0}",
    "쓰기 가능": "書き込み可",
    "쓰기 불가": "書き込み不可",
    "{0} / {1} 사용": "{0} / {1} 使用",
    "판매 중인 도메인이 없습니다.": "販売中のドメインはありません。",
    "보유 중인 주소": "保有している識別子",
    "{0}@{1} 구매 가능 — {2}": "{0}@{1} は購入できます — {2}",
    "{0}@{1} 영구 구매 가능 — {2}": "{0}@{1} は買い切りで購入できます — {2}",
    "아직 그룹이 없습니다. 이름을 정해 만드세요.": "まだグループがありません。名前を決めて作成してください。",
    "소유자": "オーナー",
    "멤버 관리는 그룹 소유자만 할 수 있습니다.": "メンバー管理はグループのオーナーのみ行えます。",
    "멤버를 보려면 서명이 필요합니다.": "メンバーを見るには署名が必要です。",
    "불러오기": "読み込む",
    "멤버": "メンバー",
    "추가할 pubkey (hex)": "追加する pubkey (hex)",
    "추가": "追加",
    "64자리 hex pubkey를 입력하세요.": "64 桁の hex pubkey を入力してください。",
    "구독 {0}회 + 추가 {1}MB ≈ {2} (신규 가입이면 가입비 {3} 추가)":
      "{0} 期間 + 追加 {1}MB ≈ {2} (新規登録なら加入料 {3} が加算されます)",
    "영구 {0}MB ≈ {1} (신규 가입이면 가입비 {2} 추가)":
      "買い切り {0}MB ≈ {1} (新規登録なら加入料 {2} が加算されます)",
    "NIP-07 확장(Alby, nos2x 등)이 없습니다. bunker 연결이나 QR을 쓰거나 pubkey를 붙여넣으세요.":
      "NIP-07 拡張機能 (Alby、nos2x など) がありません。bunker 接続か QR を使うか、pubkey を貼り付けてください。",
    "서명자 승인 창 열기": "署名者の承認画面を開く",
    "서명자에 연결하는 중…": "署名者に接続しています…",
    "원격 서명자에 연결되었습니다.": "リモート署名者に接続しました。",
    "이 릴레이는 nostrconnect relay가 설정되어 있지 않습니다.":
      "このリレーには nostrconnect 用リレーが設定されていません。",
    "지갑 앱으로 QR을 스캔하거나 연결 문자열을 붙여넣으세요":
      "ウォレットアプリで QR をスキャンするか、接続文字列を貼り付けてください",
    "{0} 결제 대기 중…": "{0} の支払いを待っています…",
    "결제 완료.": "支払いが完了しました。",
    "인보이스 만료. 다시 발행하세요.": "インボイスの期限切れです。もう一度発行してください。",
    "등록되었습니다.": "登録されました。",
    "이 멤버를 그룹에서 뺄까요?": "このメンバーをグループから外しますか？",
    "읽기 전용입니다. 멤버 관리에는 서명 로그인이 필요합니다.":
      "読み取り専用です。メンバー管理には署名ログインが必要です。",
    "64자리 hex pubkey를 입력하세요 (npub은 확장 연결을 사용하세요).":
      "64 桁の hex pubkey を入力してください (npub の場合は拡張機能で接続してください)。",

    // --- admin: login ---
    "NIP-07 · NIP-46 서명 로그인 또는 관리자 비밀번호":
      "NIP-07 · NIP-46 の署名ログイン、または管理者パスワード",
    "로그인": "ログイン",
    "관리자 비밀번호": "管理者パスワード",
    "NIP-07 확장이 없습니다.": "NIP-07 拡張機能がありません。",

    // --- admin: payment backend ---
    "백엔드": "バックエンド",
    "mock (테스트 전용)": "mock (テスト専用)",
    "LNbits 주소": "LNbits アドレス",
    "인보이스/읽기 키": "インボイス / 読み取りキー",
    "인보이스 키": "インボイスキー",
    "연결 문자열": "接続文字列",
    "연결 테스트": "接続テスト",
    "저장": "保存",
    "키는 서버에만 저장되고 화면에는 끝 4자리만 표시됩니다. 그대로 두면 기존 값이 유지됩니다.":
      "キーはサーバーにのみ保存され、画面には末尾 4 桁だけが表示されます。そのままにすると既存の値が保たれます。",
    "mock은 결제 없이 인보이스가 스스로 정산됩니다. 운영에서 쓰지 마세요.":
      "mock は支払いなしにインボイスが自動で決済されます。本番では使わないでください。",
    "테스트 중…": "テスト中…",
    "저장됨 — 새 인보이스부터 이 백엔드를 씁니다.":
      "保存しました — 次のインボイスからこのバックエンドを使います。",

    // --- admin: storage ---
    "저장 위치": "保存先",
    "내장 (서버 디스크)": "内蔵 (サーバーのディスク)",
    "S3 호환 (S3 · MinIO · R2 · B2)": "S3 互換 (S3 · MinIO · R2 · B2)",
    "저장 경로": "保存パス",
    "엔드포인트": "エンドポイント",
    "https://s3.amazonaws.com 또는 minio.example.com:9000":
      "https://s3.amazonaws.com または minio.example.com:9000",
    "버킷": "バケット",
    "리전": "リージョン",
    "키 접두사 (선택)": "キーのプレフィックス (任意)",
    "액세스 키": "アクセスキー",
    "시크릿 키": "シークレットキー",
    "공개 URL (선택, 설정 시 다운로드를 이 주소로 리다이렉트)":
      "公開 URL (任意。設定するとダウンロードをこのアドレスへリダイレクトします)",
    "저장 위치를 바꿔도 기존 파일은 옮겨지지 않습니다. 옮기려면":
      "保存先を変えても既存のファイルは移動しません。移すには",
    "nostrel migrate-blobs <기존경로>": "nostrel migrate-blobs <元のパス>",
    "를 실행하세요.": "を実行してください。",
    "현재: {0} ({1})": "現在: {0} ({1})",
    "현재: {0}": "現在: {0}",
    "기본 경로": "既定のパス",
    "저장됨 — 새 업로드부터 이 위치를 씁니다. 기존 파일은 migrate-blobs로 옮기세요.":
      "保存しました — 次のアップロードからこの場所を使います。既存のファイルは migrate-blobs で移してください。",

    // --- admin: settings ---
    "릴레이 이름": "リレー名",
    "설명": "説明",
    "로고 · 아이콘 URL": "ロゴ・アイコン URL",
    "배너 URL": "バナー URL",
    "운영자 연락처 (mailto: 등)": "運営者の連絡先 (mailto: など)",
    "운영자 pubkey (hex, 비우면 릴레이 키)":
      "運営者の pubkey (hex。空ならリレー自身の鍵)",
    "배경색": "背景色",
    "포인트 색": "アクセント色",
    "배경 이미지 URL": "背景画像 URL",
    "보관 기간 (일, 0 = 무기한)": "保管期間 (日。0 = 無期限)",
    "이용 약관 URL": "利用規約 URL",
    "운영 국가 (ISO 3166-1, 쉼표)": "運営国 (ISO 3166-1、カンマ区切り)",
    "언어 (ISO 639-1, 쉼표)": "言語 (ISO 639-1、カンマ区切り)",
    "주제 태그 (쉼표)": "トピックタグ (カンマ区切り)",
    "가입비 (sats)": "加入料 (sats)",
    "구독제 판매": "サブスクリプションを販売する",
    "구독료 (sats)": "サブスクリプション料金 (sats)",
    "구독 기간 (일)": "サブスクリプション期間 (日)",
    "구독 포함 용량 (MB)": "サブスクに含まれる容量 (MB)",
    "추가 용량 단가 (sats/MB)": "追加容量の単価 (sats/MB)",
    "영구 용량제 판매 (1회 결제, 만료 없음)":
      "買い切り容量プランを販売する (1 回払い、期限なし)",
    "영구 용량 단가 (sats/MB)": "買い切り容量の単価 (sats/MB)",
    "짧은 이름 할증 (길이:배수)": "短い名前の割増 (長さ:倍率)",
    "NIP-46 로그인 relay (쉼표 구분)": "NIP-46 ログイン用リレー (カンマ区切り)",
    "nsite 호스팅 도메인 (NIP-5A, 쉼표)": "nsite ホスティング用ドメイン (NIP-5A、カンマ区切り)",
    "외부에서 오는 DM 허용 (NIP-04 · NIP-17, 끄면 비공개 채팅 불가)":
      "外部からの DM を受け付ける (NIP-04 · NIP-17。オフにすると非公開チャットは使えません)",
    "제3자 zap 영수증 허용 (NIP-57)": "第三者の zap レシートを許可 (NIP-57)",
    "제3자 배지 수여 허용 (NIP-58)": "第三者のバッジ授与を許可 (NIP-58)",
    "누구나 초대 코드 요청 가능 (NIP-43 kind 28935)":
      "誰でも招待コードを要求できる (NIP-43 kind 28935)",
    "자동 초대 기간 (일)": "自動招待の期間 (日)",
    "자동 초대 용량 (MB)": "自動招待の容量 (MB)",
    "읽기도 인증·화이트리스트 필요 (NIP-42)":
      "読み取りにも認証とホワイトリストを要求 (NIP-42)",
    "최소 작업증명 난이도 (NIP-13, 0 = 끔)":
      "最小プルーフオブワーク難易度 (NIP-13。0 = 無効)",
    "created_at 과거 허용치 (초, 0 = 무제한)":
      "created_at の過去許容値 (秒。0 = 無制限)",
    "created_at 미래 허용치 (초, 0 = 무제한)":
      "created_at の未来許容値 (秒。0 = 無制限)",
    "파일 1개 최대 크기 (MB)": "ファイル 1 個の最大サイズ (MB)",
    "짧은 이름 할증은": "短い名前の割増は",
    "길이:배수": "長さ:倍率",
    "목록입니다. 예를 들어": "の一覧です。たとえば",
    "는 한 글자 이름을 도메인 가격의 20배로 팝니다. 비워두면 할증이 없습니다.":
      "は 1 文字の名前をドメイン価格の 20 倍で販売します。空にすると割増はありません。",
    "저장됨": "保存しました",

    // --- admin: stats ---
    "{0} (활성 {1})": "{0} (有効 {1})",
    "이벤트": "イベント",
    "저장량": "保存量",
    "{0}건 / {1} sats": "{0} 件 / {1} sats",

    // --- admin: accounts ---
    "pubkey 또는 메모 검색": "pubkey またはメモを検索",
    "검색": "検索",
    "수동 추가할 pubkey (hex)": "手動で追加する pubkey (hex)",
    "일": "日",
    "용량": "容量",
    "상태": "状態",
    "만료": "期限",
    "사용/할당(MB)": "使用 / 割当 (MB)",
    "메모": "メモ",
    "삭제": "削除",
    "계정 없음": "アカウントなし",
    "이 계정을 삭제할까요? (이벤트는 남습니다)":
      "このアカウントを削除しますか？ (イベントは残ります)",

    // --- admin: invites ---
    "초대 코드 (NIP-43)": "招待コード (NIP-43)",
    "메모 (예: 베타 초대)": "メモ (例: ベータ招待)",
    "사용": "使用",
    "회 (0 = 무제한)": "回 (0 = 無制限)",
    "일 (0 = 없음)": "日 (0 = なし)",
    "발급": "発行",
    "초대 코드는": "招待コードは",
    "가입비를 면제": "加入料を免除します",
    "합니다. 코드를 클릭하면 복사됩니다.": "。コードをクリックするとコピーされます。",
    "가 설정돼 있으면 클라이언트가 kind 28934로 직접 가입할 수도 있습니다.":
      "が設定されていれば、クライアントは kind 28934 で直接登録することもできます。",
    "코드": "コード",
    "클릭하면 복사": "クリックでコピー",
    "{0}일": "{0} 日",
    "없음": "なし",
    "초대 코드 없음": "招待コードなし",
    "이 초대 코드를 삭제할까요? 이미 쓴 사람은 그대로입니다.":
      "この招待コードを削除しますか？ すでに使った人には影響しません。",

    // --- admin: groups ---
    "그룹 이름 · 소유자 · 메모 검색": "グループ名・オーナー・メモを検索",
    "소유자 pubkey (hex)": "オーナーの pubkey (hex)",
    "멤버는 자기 계정으로 먼저 쓰고, 개인 용량이 없거나 다 쓴 뒤에만 그룹 용량을 씁니다. 한 pubkey는 그룹 하나에만 속할 수 있습니다.":
      "メンバーはまず自分のアカウントを使い、個人の容量がないか使い切ったときだけグループの容量を使います。1 つの pubkey は 1 つのグループにしか所属できません。",
    "이름": "名前",
    "{0}명": "{0} 人",
    "그룹 없음": "グループなし",
    "멤버 없음": "メンバーなし",
    "소유자 pubkey를 64자리 hex로 입력하세요.": "オーナーの pubkey を 64 桁の hex で入力してください。",
    "이 그룹을 삭제할까요? 멤버는 각자 계정으로 돌아갑니다.":
      "このグループを削除しますか？ メンバーはそれぞれのアカウントに戻ります。",

    // --- admin: NIP-05 ---
    "도메인 (예: example.com)": "ドメイン (例: example.com)",
    "가격": "価格",
    "도메인의 DNS를 이 서버로 향하게 하고, 리버스 프록시가 Host 헤더를 그대로 넘겨야":
      "ドメインの DNS をこのサーバーに向け、リバースプロキシが Host ヘッダーをそのまま渡すようにすると",
    "이 그 도메인의 이름을 응답합니다. 테스트 버튼은 서버가 바깥에서 그 주소를 직접 불러 확인합니다.":
      "がそのドメインの名前を返します。テストボタンはサーバーが外部からそのアドレスを実際に取得して確認します。",
    "Host 헤더를 넘길 수 없는 프록시라면, 도메인을 경로에 담은 아래 주소가 같은 문서를 응답합니다. 프록시에서 그 도메인의 nostr.json 요청을 이쪽으로 넘기세요.":
      "Host ヘッダーを渡せないプロキシの場合は、ドメインをパスに含めた下のアドレスが同じ文書を返します。そのドメインの nostr.json リクエストをここへ転送してください。",
    "테스트": "テスト",
    "{0} 확인 중…": "{0} を確認中…",
    "응답 코드": "応答コード",
    "이 서버의 답": "このサーバーの応答",
    "도메인의 답": "ドメインの応答",
    "CORS 헤더 없음": "CORS ヘッダーがありません",
    "정상": "正常",
    "실패": "失敗",
    "차단한 이름": "ブロックした名前",
    "판매 금지할 이름 (예: admin)": "販売しない名前 (例: admin)",
    "사유 (선택)": "理由 (任意)",
    "차단": "ブロック",
    "판매된 이름": "販売済みの名前",
    "이름 또는 pubkey 검색": "名前または pubkey を検索",
    "일 (0 = 무기한)": "日 (0 = 無期限)",
    "도메인": "ドメイン",
    "판매중": "販売中",
    "가격 (sats)": "価格 (sats)",
    "기간 (일)": "期間 (日)",
    "영구 가격 (sats, 0 = 안 팜)": "買い切り価格 (sats。0 = 販売しない)",
    "도메인 없음": "ドメインなし",
    "도메인을 입력하세요.": "ドメインを入力してください。",
    "이름을 입력하세요.": "名前を入力してください。",
    "이름과 도메인을 입력하세요.": "名前とドメインを入力してください。",
    "차단한 이름 없음": "ブロックした名前はありません",
    "주소": "識別子",
    "결제 대기": "支払い待ち",
    "판매된 이름 없음": "販売済みの名前はありません",
    "이 도메인을 지울까요? 이 도메인으로 판매된 주소가 모두 사라집니다.":
      "このドメインを削除しますか？ このドメインで販売された識別子はすべて消えます。",
    "이 주소를 회수할까요?": "この識別子を回収しますか？",

    // --- admin: media, payments ---
    "신고된 파일 {0}건": "通報されたファイル {0} 件",
    "신고 무시": "通報を無視",
    "파일": "ファイル",
    "업로더": "アップロード者",
    "크기": "サイズ",
    "형식": "形式",
    "올린 시각": "アップロード日時",
    "신고됨": "通報あり",
    "업로드된 파일 없음": "アップロードされたファイルはありません",
    "이 파일을 서버에서 지울까요? 모든 소유자에게서 제거되고 용량이 환불됩니다.":
      "このファイルをサーバーから削除しますか？ すべての所有者から取り除かれ、容量が返却されます。",
    "시각": "日時",
    "종류": "種類",
    "결제 없음": "決済なし",
  },

  zh: {
    // --- shell, navigation ---
    "요금": "价格",
    "내 계정": "我的账户",
    "가입 · 갱신": "注册 · 续费",
    "초대 코드": "邀请码",
    "NIP-05 주소": "NIP-05 标识",
    "그룹": "群组",
    "결제": "支付",
    "미디어 서버": "媒体服务器",
    "인보이스 발행 중…": "正在生成发票…",
    "로그인 없이 요금만 보기": "不登录，只看价格",
    "미디어 서버 (Blossom · NIP-96)": "媒体服务器 (Blossom · NIP-96)",
    "이 릴레이가 미디어 서버를 겸합니다. 아래 주소를 클라이언트의 미디어 서버 설정에 넣으면 업로드한 파일이 여기에 저장되고, 계정 용량에서 차감됩니다.":
      "本中继同时是媒体服务器。把下面的地址填入客户端的媒体服务器设置，上传的文件就会存在这里，并从账户容量中扣除。",
    "Blossom 서버 주소": "Blossom 服务器地址",
    "NIP-96 서버 정보": "NIP-96 服务器信息",
    "지원: BUD-01 · 02 · 04 · 05 · 06 · 09. 업로드에는 쓰기 권한이 있는 계정이 필요합니다.":
      "支持：BUD-01 · 02 · 04 · 05 · 06 · 09。上传需要具有写入权限的账户。",
    "관리자": "管理员",
    "현황": "概览",
    "결제 백엔드": "支付后端",
    "파일 저장소": "文件存储",
    "요금 · 릴레이 정보": "价格 · 中继信息",
    "계정": "账户",
    "미디어": "媒体",
    "결제 내역": "支付记录",
    "로그아웃": "退出登录",
    "언어": "语言",

    // --- public panel ---
    "라이트닝으로 결제하면 해당 pubkey가 쓰기 화이트리스트에 등록됩니다. 읽기는 별도 안내가 없으면 공개입니다.":
      "通过闪电网络付款后，该 pubkey 会被加入写入白名单。除非另有说明，读取是公开的。",
    "확장 프로그램 (NIP-07)": "浏览器扩展 (NIP-07)",
    "확장으로 서명 로그인": "用扩展签名登录",
    "또는": "或",
    "연결": "连接",
    "QR 스캔 (nostrconnect)": "扫描二维码 (nostrconnect)",
    "QR 만들기": "生成二维码",
    "복사": "复制",
    "읽기 전용": "只读",
    "pubkey(hex) 붙여넣기": "粘贴 pubkey (hex)",
    "코드 사용": "使用邀请码",
    "초대 코드가 있으면 가입비 없이 등록됩니다.":
      "有邀请码即可免加入费注册。",
    "가입 · 갱신 · 용량 충전": "注册 · 续费 · 充值容量",
    "구독제": "订阅制",
    "영구 용량제": "永久容量制",
    "구독 기간": "订阅期数",
    "회": "期",
    "추가 용량": "额外容量",
    "영구 용량": "永久容量",
    "한 번 결제하면 만료도 갱신도 없습니다. 구독 갱신은 더 이상 필요하지 않습니다.":
      "一次付款，既不到期也无需续费。此后不再需要续订。",
    "영구": "永久",
    "인보이스 발행": "生成发票",
    "기간": "期数",
    "확인": "检查",
    "원하는 이름": "想要的名字",
    "그룹 (용량 공유)": "群组 (共享容量)",
    "새 그룹 이름": "新群组名称",
    "이미 그룹이 있으면 같은 버튼이 연장·충전이 됩니다. 멤버는 자기 계정을 먼저 쓰고, 개인 용량이 없거나 다 쓴 뒤에만 그룹 용량을 씁니다.":
      "如果已有群组，同一个按钮就是续期和充值。成员先用自己的账户，只有在个人容量没有或用完后才会动用群组容量。",

    // --- public panel, built in JavaScript ---
    "무기한": "永不过期",
    "멤버 관리에는 서명 로그인이 필요합니다.": "管理成员需要签名登录。",
    "가입비 (최초 1회)": "加入费 (仅首次)",
    "구독 ({0}일)": "订阅 ({0} 天)",
    "구독 포함 용량": "订阅含容量",
    "영구 용량 (1회 결제)": "永久容量 (一次付清)",
    "영구 (만료 없음)": "永久，不会过期",
    "NIP-05 도메인": "NIP-05 域名",
    "{0}개 판매 중": "在售 {0} 个",
    "아직 등록되지 않음. 아래에서 가입하세요.": "尚未注册，请在下方加入。",
    "만료 {0}": "到期 {0}",
    "쓰기 가능": "可写入",
    "쓰기 불가": "无法写入",
    "{0} / {1} 사용": "已用 {0} / {1}",
    "판매 중인 도메인이 없습니다.": "没有在售的域名。",
    "보유 중인 주소": "已持有的标识",
    "{0}@{1} 구매 가능 — {2}": "{0}@{1} 可以购买 — {2}",
    "{0}@{1} 영구 구매 가능 — {2}": "{0}@{1} 可以永久购买 — {2}",
    "아직 그룹이 없습니다. 이름을 정해 만드세요.": "还没有群组，取个名字创建一个吧。",
    "소유자": "所有者",
    "멤버 관리는 그룹 소유자만 할 수 있습니다.": "只有群组所有者可以管理成员。",
    "멤버를 보려면 서명이 필요합니다.": "查看成员需要签名。",
    "불러오기": "加载",
    "멤버": "成员",
    "추가할 pubkey (hex)": "要添加的 pubkey (hex)",
    "추가": "添加",
    "64자리 hex pubkey를 입력하세요.": "请输入 64 位十六进制 pubkey。",
    "구독 {0}회 + 추가 {1}MB ≈ {2} (신규 가입이면 가입비 {3} 추가)":
      "{0} 期 + 额外 {1}MB ≈ {2} (新用户还需加入费 {3})",
    "영구 {0}MB ≈ {1} (신규 가입이면 가입비 {2} 추가)":
      "永久 {0}MB ≈ {1} (新用户还需加入费 {2})",
    "NIP-07 확장(Alby, nos2x 등)이 없습니다. bunker 연결이나 QR을 쓰거나 pubkey를 붙여넣으세요.":
      "没有找到 NIP-07 扩展 (Alby、nos2x 等)。请改用 bunker 连接或二维码，或粘贴 pubkey。",
    "서명자 승인 창 열기": "打开签名器的授权窗口",
    "서명자에 연결하는 중…": "正在连接签名器…",
    "원격 서명자에 연결되었습니다.": "已连接到远程签名器。",
    "이 릴레이는 nostrconnect relay가 설정되어 있지 않습니다.":
      "本中继没有配置 nostrconnect 用的中继。",
    "지갑 앱으로 QR을 스캔하거나 연결 문자열을 붙여넣으세요":
      "用钱包应用扫描二维码，或粘贴连接字符串",
    "{0} 결제 대기 중…": "等待支付 {0}…",
    "결제 완료.": "支付完成。",
    "인보이스 만료. 다시 발행하세요.": "发票已过期，请重新生成。",
    "등록되었습니다.": "已完成注册。",
    "이 멤버를 그룹에서 뺄까요?": "要把这位成员移出群组吗？",
    "읽기 전용입니다. 멤버 관리에는 서명 로그인이 필요합니다.":
      "当前为只读。管理成员需要签名登录。",
    "64자리 hex pubkey를 입력하세요 (npub은 확장 연결을 사용하세요).":
      "请输入 64 位十六进制 pubkey (npub 请改用扩展连接)。",

    // --- admin: login ---
    "NIP-07 · NIP-46 서명 로그인 또는 관리자 비밀번호":
      "NIP-07 · NIP-46 签名登录，或管理员密码",
    "로그인": "登录",
    "관리자 비밀번호": "管理员密码",
    "NIP-07 확장이 없습니다.": "没有找到 NIP-07 扩展。",

    // --- admin: payment backend ---
    "백엔드": "后端",
    "mock (테스트 전용)": "mock (仅测试)",
    "LNbits 주소": "LNbits 地址",
    "인보이스/읽기 키": "发票 / 读取密钥",
    "인보이스 키": "发票密钥",
    "연결 문자열": "连接字符串",
    "연결 테스트": "测试连接",
    "저장": "保存",
    "키는 서버에만 저장되고 화면에는 끝 4자리만 표시됩니다. 그대로 두면 기존 값이 유지됩니다.":
      "密钥只保存在服务器端，界面上只显示后四位。保持原样即可沿用现有值。",
    "mock은 결제 없이 인보이스가 스스로 정산됩니다. 운영에서 쓰지 마세요.":
      "mock 会在没有付款的情况下自动结算发票，请勿用于生产环境。",
    "테스트 중…": "测试中…",
    "저장됨 — 새 인보이스부터 이 백엔드를 씁니다.":
      "已保存 — 从下一张发票起使用该后端。",

    // --- admin: storage ---
    "저장 위치": "存储位置",
    "내장 (서버 디스크)": "内置 (服务器磁盘)",
    "S3 호환 (S3 · MinIO · R2 · B2)": "S3 兼容 (S3 · MinIO · R2 · B2)",
    "저장 경로": "存储路径",
    "엔드포인트": "端点",
    "https://s3.amazonaws.com 또는 minio.example.com:9000":
      "https://s3.amazonaws.com 或 minio.example.com:9000",
    "버킷": "存储桶",
    "리전": "区域",
    "키 접두사 (선택)": "键前缀 (可选)",
    "액세스 키": "访问密钥",
    "시크릿 키": "私密密钥",
    "공개 URL (선택, 설정 시 다운로드를 이 주소로 리다이렉트)":
      "公开 URL (可选；设置后下载会重定向到该地址)",
    "저장 위치를 바꿔도 기존 파일은 옮겨지지 않습니다. 옮기려면":
      "更换存储位置不会移动已有文件。要迁移请执行",
    "nostrel migrate-blobs <기존경로>": "nostrel migrate-blobs <原路径>",
    "를 실행하세요.": "。",
    "현재: {0} ({1})": "当前：{0} ({1})",
    "현재: {0}": "当前：{0}",
    "기본 경로": "默认路径",
    "저장됨 — 새 업로드부터 이 위치를 씁니다. 기존 파일은 migrate-blobs로 옮기세요.":
      "已保存 — 新的上传会存到这里。已有文件请用 migrate-blobs 迁移。",

    // --- admin: settings ---
    "릴레이 이름": "中继名称",
    "설명": "说明",
    "로고 · 아이콘 URL": "标志 · 图标 URL",
    "배너 URL": "横幅 URL",
    "운영자 연락처 (mailto: 등)": "运营者联系方式 (mailto: 等)",
    "운영자 pubkey (hex, 비우면 릴레이 키)":
      "运营者 pubkey (hex；留空则用中继自己的密钥)",
    "배경색": "背景色",
    "포인트 색": "强调色",
    "배경 이미지 URL": "背景图片 URL",
    "보관 기간 (일, 0 = 무기한)": "保留期限 (天，0 = 永久)",
    "이용 약관 URL": "发布政策 URL",
    "운영 국가 (ISO 3166-1, 쉼표)": "运营国家 (ISO 3166-1，逗号分隔)",
    "언어 (ISO 639-1, 쉼표)": "语言 (ISO 639-1，逗号分隔)",
    "주제 태그 (쉼표)": "主题标签 (逗号分隔)",
    "가입비 (sats)": "加入费 (sats)",
    "구독제 판매": "出售订阅制",
    "구독료 (sats)": "订阅费 (sats)",
    "구독 기간 (일)": "订阅周期 (天)",
    "구독 포함 용량 (MB)": "订阅含容量 (MB)",
    "추가 용량 단가 (sats/MB)": "额外容量单价 (sats/MB)",
    "영구 용량제 판매 (1회 결제, 만료 없음)":
      "出售永久容量制 (一次付清，永不过期)",
    "영구 용량 단가 (sats/MB)": "永久容量单价 (sats/MB)",
    "짧은 이름 할증 (길이:배수)": "短名字加价 (长度:倍数)",
    "NIP-46 로그인 relay (쉼표 구분)": "NIP-46 登录中继 (逗号分隔)",
    "nsite 호스팅 도메인 (NIP-5A, 쉼표)": "nsite 托管域名 (NIP-5A，逗号分隔)",
    "외부에서 오는 DM 허용 (NIP-04 · NIP-17, 끄면 비공개 채팅 불가)":
      "接受外部私信 (NIP-04 · NIP-17；关闭后无法使用私密聊天)",
    "제3자 zap 영수증 허용 (NIP-57)": "允许第三方 zap 收据 (NIP-57)",
    "제3자 배지 수여 허용 (NIP-58)": "允许第三方颁发徽章 (NIP-58)",
    "누구나 초대 코드 요청 가능 (NIP-43 kind 28935)":
      "任何人都可申请邀请码 (NIP-43 kind 28935)",
    "자동 초대 기간 (일)": "自动邀请期限 (天)",
    "자동 초대 용량 (MB)": "自动邀请容量 (MB)",
    "읽기도 인증·화이트리스트 필요 (NIP-42)":
      "读取也需要认证和白名单 (NIP-42)",
    "최소 작업증명 난이도 (NIP-13, 0 = 끔)":
      "最低工作量证明难度 (NIP-13，0 = 关闭)",
    "created_at 과거 허용치 (초, 0 = 무제한)":
      "created_at 过去容差 (秒，0 = 不限)",
    "created_at 미래 허용치 (초, 0 = 무제한)":
      "created_at 未来容差 (秒，0 = 不限)",
    "파일 1개 최대 크기 (MB)": "单个文件最大大小 (MB)",
    "짧은 이름 할증은": "短名字加价是一个",
    "길이:배수": "长度:倍数",
    "목록입니다. 예를 들어": "列表。例如",
    "는 한 글자 이름을 도메인 가격의 20배로 팝니다. 비워두면 할증이 없습니다.":
      "会把单字名字卖到域名价格的 20 倍。留空则不加价。",
    "저장됨": "已保存",

    // --- admin: stats ---
    "{0} (활성 {1})": "{0} (活跃 {1})",
    "이벤트": "事件",
    "저장량": "存储量",
    "{0}건 / {1} sats": "{0} 笔 / {1} sats",

    // --- admin: accounts ---
    "pubkey 또는 메모 검색": "搜索 pubkey 或备注",
    "검색": "搜索",
    "수동 추가할 pubkey (hex)": "手动添加的 pubkey (hex)",
    "일": "天",
    "용량": "容量",
    "상태": "状态",
    "만료": "到期",
    "사용/할당(MB)": "已用 / 配额 (MB)",
    "메모": "备注",
    "삭제": "删除",
    "계정 없음": "没有账户",
    "이 계정을 삭제할까요? (이벤트는 남습니다)":
      "要删除这个账户吗？(事件会保留)",

    // --- admin: invites ---
    "초대 코드 (NIP-43)": "邀请码 (NIP-43)",
    "메모 (예: 베타 초대)": "备注 (例如：内测邀请)",
    "사용": "使用",
    "회 (0 = 무제한)": "次 (0 = 不限)",
    "일 (0 = 없음)": "天 (0 = 无期限)",
    "발급": "签发",
    "초대 코드는": "邀请码会",
    "가입비를 면제": "免除加入费",
    "합니다. 코드를 클릭하면 복사됩니다.": "。点击邀请码即可复制。当",
    "가 설정돼 있으면 클라이언트가 kind 28934로 직접 가입할 수도 있습니다.":
      "已配置时，客户端也可以用 kind 28934 直接注册。",
    "코드": "邀请码",
    "클릭하면 복사": "点击复制",
    "{0}일": "{0} 天",
    "없음": "无",
    "초대 코드 없음": "没有邀请码",
    "이 초대 코드를 삭제할까요? 이미 쓴 사람은 그대로입니다.":
      "要删除这个邀请码吗？已经使用过的人不受影响。",

    // --- admin: groups ---
    "그룹 이름 · 소유자 · 메모 검색": "搜索群组名称、所有者或备注",
    "소유자 pubkey (hex)": "所有者 pubkey (hex)",
    "멤버는 자기 계정으로 먼저 쓰고, 개인 용량이 없거나 다 쓴 뒤에만 그룹 용량을 씁니다. 한 pubkey는 그룹 하나에만 속할 수 있습니다.":
      "成员先用自己的账户，只有在个人容量没有或用完后才会动用群组容量。一个 pubkey 只能属于一个群组。",
    "이름": "名称",
    "{0}명": "{0} 人",
    "그룹 없음": "没有群组",
    "멤버 없음": "没有成员",
    "소유자 pubkey를 64자리 hex로 입력하세요.": "请以 64 位十六进制输入所有者 pubkey。",
    "이 그룹을 삭제할까요? 멤버는 각자 계정으로 돌아갑니다.":
      "要删除这个群组吗？成员会退回各自的账户。",

    // --- admin: NIP-05 ---
    "도메인 (예: example.com)": "域名 (例如：example.com)",
    "가격": "价格",
    "도메인의 DNS를 이 서버로 향하게 하고, 리버스 프록시가 Host 헤더를 그대로 넘겨야":
      "把域名的 DNS 指向这台服务器，并让反向代理原样传递 Host 头，这样",
    "이 그 도메인의 이름을 응답합니다. 테스트 버튼은 서버가 바깥에서 그 주소를 직접 불러 확인합니다.":
      "才会返回该域名下的名字。测试按钮会让服务器从外部实际请求该地址来验证。",
    "Host 헤더를 넘길 수 없는 프록시라면, 도메인을 경로에 담은 아래 주소가 같은 문서를 응답합니다. 프록시에서 그 도메인의 nostr.json 요청을 이쪽으로 넘기세요.":
      "如果前置代理无法传递 Host 头，下面这些把域名放在路径里的地址会返回同样的文档。请把该域名的 nostr.json 请求转发到这里。",
    "테스트": "测试",
    "{0} 확인 중…": "正在检查 {0}…",
    "응답 코드": "响应码",
    "이 서버의 답": "本服务器的回答",
    "도메인의 답": "域名的回答",
    "CORS 헤더 없음": "缺少 CORS 头",
    "정상": "正常",
    "실패": "失败",
    "차단한 이름": "已屏蔽的名字",
    "판매 금지할 이름 (예: admin)": "禁止出售的名字 (例如：admin)",
    "사유 (선택)": "原因 (可选)",
    "차단": "屏蔽",
    "판매된 이름": "已售出的名字",
    "이름 또는 pubkey 검색": "搜索名字或 pubkey",
    "일 (0 = 무기한)": "天 (0 = 永久)",
    "도메인": "域名",
    "판매중": "在售",
    "가격 (sats)": "价格 (sats)",
    "기간 (일)": "周期 (天)",
    "영구 가격 (sats, 0 = 안 팜)": "永久价格 (sats，0 = 不出售)",
    "도메인 없음": "没有域名",
    "도메인을 입력하세요.": "请输入域名。",
    "이름을 입력하세요.": "请输入名字。",
    "이름과 도메인을 입력하세요.": "请输入名字和域名。",
    "차단한 이름 없음": "没有被屏蔽的名字",
    "주소": "标识",
    "결제 대기": "等待支付",
    "판매된 이름 없음": "没有售出的名字",
    "이 도메인을 지울까요? 이 도메인으로 판매된 주소가 모두 사라집니다.":
      "要删除这个域名吗？该域名下售出的所有标识都会消失。",
    "이 주소를 회수할까요?": "要收回这个标识吗？",

    // --- admin: media, payments ---
    "신고된 파일 {0}건": "被举报的文件 {0} 个",
    "신고 무시": "忽略举报",
    "파일": "文件",
    "업로더": "上传者",
    "크기": "大小",
    "형식": "类型",
    "올린 시각": "上传时间",
    "신고됨": "已被举报",
    "업로드된 파일 없음": "没有上传的文件",
    "이 파일을 서버에서 지울까요? 모든 소유자에게서 제거되고 용량이 환불됩니다.":
      "要从服务器删除这个文件吗？它会从所有持有者处移除，并退还容量。",
    "시각": "时间",
    "종류": "种类",
    "결제 없음": "没有支付记录",
  },
};

const LANGS = ["ko", "en", "ja", "zh"];

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
