# nostrel

*[English](README.md) · 한국어*

라이트닝 결제로 쓰기 화이트리스트를 운영하는 Nostr 릴레이. 웹 패널 포함, 단일 바이너리 + 단일 SQLite 파일.

- 릴레이 엔진: `internal/relaycore` (khatru 기반, 흡수)
- 결제: LNbits, NWC(NIP-47), 로컬 테스트용 mock
- 과금: 가입비(1회) + 구독(기간제) + 용량(MB) 선불 — 이벤트와 업로드 파일이 같은 용량을 소비
- 판매 상품: 릴레이 접근, **NIP-05 주소**(도메인별 가격, 기간제), **그룹**(여러 pubkey가 용량 공유)
- 관리자 인증: NIP-98 서명 로그인 + 비밀번호 백업
- 미디어: Blossom(BUD-01/02) + NIP-96

## NIP 지원 체크리스트

체크 = 릴레이가 프로토콜 차원에서 구현한다. `조건부`는 설정이 갖춰졌을 때만 켜진다.

- [x] **NIP-01** 기본 프로토콜
- [x] **NIP-04** 암호화 DM — 열람 제한을 릴레이가 강제
- [x] **NIP-05** DNS 기반 식별자 — 도메인별로 이름 판매 *(조건부)*
- [x] **NIP-09** 이벤트 삭제 — 용량 환불 포함
- [x] **NIP-11** 릴레이 정보 문서 — 설정에서 실시간 생성
- [x] **NIP-13** Proof of Work *(조건부: 패널 `min_pow > 0`)*
- [x] **NIP-17** 비공개 DM
- [x] **NIP-40** 이벤트 만료 — 만료 후 GC가 용량 회수
- [x] **NIP-42** AUTH
- [x] **NIP-43** 릴레이 접근·초대 코드 *(조건부: `RELAY_SECRET_KEY`)*
- [x] **NIP-44** 암호화 (v2)
- [x] **NIP-45** COUNT
- [x] **NIP-46** 원격 서명 (bunker) — 공개 패널 로그인
- [x] **NIP-50** 검색 — `domain:` 확장 포함
- [x] **NIP-56** 신고 — NIP-86 모더레이션 큐로
- [x] **NIP-57** 라이트닝 zap 영수증 *(조건부: 기본 꺼짐)*
- [x] **NIP-58** 배지 수여 *(조건부: 기본 꺼짐)*
- [x] **NIP-59** gift wrap — 열람 제한을 릴레이가 강제
- [x] **NIP-62** 잊혀질 권리
- [x] **NIP-70** 보호 이벤트
- [x] **NIP-77** negentropy 동기화
- [x] **NIP-86** 릴레이 관리 API — 전체 메서드
- [x] **NIP-96** HTTP 파일 저장 *(조건부: 파일 저장소 사용 가능)*
- [x] **NIP-98** HTTP 인증 — 패널·NIP-96·NIP-86
- [x] **NIP-5A** 정적 사이트 호스팅 (nsites) *(조건부: nsite 도메인 설정)*
- [ ] **NIP-29** 릴레이 기반 그룹 — 미지원, 유료 화이트리스트와 권한 모델이 충돌 ([아래](#gift-wrap-nip-59))

Blossom BUD-01·02·04·05·06·09도 구현되어 있다 ([미디어 호스팅](#미디어-호스팅)).

**릴레이가 구현할 것이 없는 NIP** — 작성자가 자기 pubkey로 쓰는 일반 이벤트라, 화이트리스트만 통과하면 그대로 저장·전달된다: 02, 07, 19, 23, 25, 28, 51, 65, 75(zap goal), 7D(포럼 스레드), 84(하이라이트), 85(신뢰 어설션), B7(Blossom 서버 목록) 등. 특정 kind를 막고 싶으면 NIP-86 `allowkind`/`disallowkind`를 쓴다. NIP-47(NWC)은 릴레이가 **클라이언트로서** 결제 백엔드에 쓴다.

### 상세

| NIP | 기능 | 비고 |
|---|---|---|
| 01 | 기본 프로토콜 | EVENT/REQ/CLOSE, 구독 |
| 04 | 암호화 DM | kind 4는 당사자만 조회 가능 |
| 05 | DNS 기반 식별자 | `/.well-known/nostr.json`, 도메인별로 이름을 판매 |
| 09 | 이벤트 삭제 | 삭제 시 용량 환불 |
| 11 | 릴레이 정보 문서 | 요금·제한·정책을 설정에서 실시간 생성 |
| 13 | Proof of Work | 패널 `min_pow` 설정 시 활성화, committed difficulty 검사 |
| 17 | 비공개 DM | NIP-59 gift wrap 위에서 동작 |
| 40 | 이벤트 만료 | 만료 즉시 조회에서 제외, 이후 GC가 디스크·용량 회수 |
| 42 | AUTH | DM 조회, 패널 `read_auth_required`, NIP-43 요청 |
| 43 | 릴레이 접근·초대 코드 | `RELAY_SECRET_KEY` 설정 시 (아래 참고) |
| 44 | 암호화 (v2) | 패널의 NIP-46 통신에 사용 |
| 45 | COUNT | |
| 46 | 원격 서명 (bunker) | 공개 패널 로그인. 브라우저가 서명자와 직접 통신 |
| 50 | 검색 | `content` 부분 일치 + `domain:` 확장 |
| 56 | 신고 | kind 1984 → NIP-86 모더레이션 큐 |
| 57 | 라이트닝 zap | kind 9735 영수증 수용 (기본 꺼짐, 아래 참고) |
| 58 | 배지 | kind 8 수여 수용 (기본 꺼짐, 아래 참고) |
| 59 | gift wrap | kind 1059·13은 발신자·수신자만 조회, 백데이트 허용 |
| 62 | 잊혀질 권리 | kind 62 → 그 pubkey의 이벤트·파일 삭제 + 재업로드 차단 |
| 5A | 정적 사이트 (nsites) | 판매한 NIP-05 이름으로 `<이름>.<도메인>` 호스팅 |
| 70 | 보호 이벤트 | `-` 태그 |
| 77 | negentropy 동기화 | `nak sync`로 검증 |
| 86 | 릴레이 관리 API | 아래 전체 메서드 |
| 96 | HTTP 파일 저장 | `/.well-known/nostr/nip96.json` |
| 98 | HTTP 인증 | 패널·NIP-96·NIP-86 |

NIP-11 `supported_nips`는 **실제 상태를 그대로** 내보낸다. 항상 켜져 있는 것(01·04·09·11·17·40·42·45·50·56·59·62·70·77·86·98)은 고정이고, 나머지는 설정에 따라 붙었다 떨어진다:

| NIP | 조건 |
|---|---|
| 13 | 패널 `min_pow > 0` |
| 43 | `RELAY_SECRET_KEY` 설정됨 |
| 05 | 판매 중인 도메인이 하나 이상 |
| 57 / 58 | 각 제3자 이벤트 토글 |
| 96 | 파일 저장소 사용 가능 |

04·17·59가 여기 있는 이유는 단순히 저장해서가 아니라, **누가 읽을 수 있는지를 릴레이가 강제**하기 때문이다.

### 제3자가 고객 앞으로 쓰는 이벤트 (NIP-57 · NIP-58)

두 종류만 예외다. 작성자가 고객이 아닌데 **고객에 관한** 이벤트라, 그대로 두면 전부 거부된다.

| 이벤트 | 작성자 | 설정 |
|---|---|---|
| zap 영수증 (kind 9735) | 결제를 받은 LNURL 서버 | **제3자 zap 영수증 허용** |
| 배지 수여 (kind 8) | 배지 발행자 | **제3자 배지 수여 허용** |

켜면 다음을 만족할 때만 통과한다.

- **공통**: `p` 태그가 가리키는 pubkey 중 이 릴레이에 계정(또는 그룹)이 있는 **첫 번째** 사람이 지불자가 되고, 그 사람의 쓰기 권한·용량을 통과해야 한다. `p`가 하나도 고객이 아니면 거부.
- **zap 영수증**: `p`는 정확히 하나. `bolt11` 태그 필요. `description`이 kind 9734 zap 요청이고 **id·서명이 유효**하며 그 `p`가 수신자와 같을 것.
- **배지 수여**: `a` 태그가 `30009:<작성자 pubkey>:<slug>` 형식일 것 — 발행자가 **자기 배지 정의**를 가리켜야 하므로 남의 배지를 대신 수여할 수 없다. `p`는 여러 명 가능(NIP-58 규격).

용량은 **수신자에게** 청구된다(올린 쪽이 아니라). 결제나 수여가 실제로 일어났는지는 릴레이가 알 수 없으므로, 켜는 순간 모르는 사람이 고객의 용량을 쓸 수 있다는 뜻이다 — 그래서 둘 다 기본값이 꺼짐이고, 악용하는 작성자는 NIP-86으로 밴한다. 켜져 있을 때만 NIP-11 `supported_nips`에 각각 57·58이 올라간다.

배지의 나머지 kind(30009 정의, 10008 프로필 배지, 30008 배지 세트)는 본인이 쓰는 일반 이벤트라 설정과 무관하게 동작한다.

### 초대 코드 (NIP-43)

결제 없이 들여보내는 경로다. 베타 초대·지인 초대·행사 쿠폰. **결제를 대체하지 않고 가입비를 면제**한다 — NIP-43 자체는 가격에 중립이다.

**켜는 법**: `RELAY_SECRET_KEY`에 hex 64자 개인키를 넣는다. 릴레이가 자기 이름으로 서명할 수 있어야 NIP-43이 켜지고, 없으면 전부 비활성이며 `supported_nips`에도 43이 올라가지 않는다.

```bash
openssl rand -hex 32   # RELAY_SECRET_KEY
```

**이 키는 릴레이의 신원이다.** 유출되면 남이 우리 이름으로 멤버십 목록을 위조할 수 있다. 그래서 DB에 넣지 않고 패널에서도 편집하지 않는다 — 환경변수 전용이다. 패널의 운영자 pubkey가 비어 있으면 NIP-11은 이 키의 pubkey로 대체한다.

| 흐름 | 어떻게 |
|---|---|
| 관리자가 코드 발급 | 패널 `초대 코드` — 기간·용량·사용 횟수·만료 지정 |
| 클라이언트가 코드 요청 | `kind 28935` 구독. **자동 초대**를 켰을 때만, 요청마다 새 1회용 코드(10분 만료) |
| 코드로 가입 | `kind 28934` + `claim` 태그 → 계정 생성, 가입비 면제 |
| 탈퇴 | `kind 28936` → 접근 차단 |
| 릴레이가 발행 | `33534` 역할(member·admin), `13534` 멤버십(최대 1000명), `8000`/`8001` 가입·탈퇴 알림 |

웹에서는 서명 로그인 후 공개 패널의 `초대 코드` 카드에 넣으면 된다(`POST /api/invite/claim`, NIP-98 서명). 지갑 없이도 같은 결과다.

**탈퇴는 데이터를 지우지 않는다.** 상태를 `banned`로 내려 쓰기만 막는다 — 결제한 사용자의 이벤트를 "접근을 끊어달라"는 요청만으로 지우면 안 된다. 완전 삭제는 NIP-62다.

**동시성**: 코드 하나를 여러 명이 동시에 청구해도 한 트랜잭션에서 만료·잔여·중복을 함께 처리하므로 정원을 넘겨 발급되지 않는다. 같은 사람이 같은 코드를 두 번 써서 횟수를 소진시킬 수도 없다.

**AUTH가 필요하다.** NIP-43은 요청에 NIP-70 `-` 태그를 필수로 요구하고, 보호 이벤트는 NIP-42 인증을 거쳐야 발행된다. 즉 가입·탈퇴 요청 전에 클라이언트가 AUTH를 마쳐야 한다 — 릴레이가 `AUTH` 챌린지를 먼저 보낸다. AUTH 이벤트의 `relay` 태그는 `SERVICE_URL`과 일치해야 한다.

응답 문구는 스펙 예시 그대로다:

```
["OK", <id>, false, "restricted: that is an invalid invite code."]
["OK", <id>, true,  "info: welcome to ws://localhost:3399!"]
["OK", <id>, true,  "duplicate: you are already a member of this relay."]
```

성공 문구를 우리가 정할 수 있는 건 릴레이 엔진이 `internal/relaycore`에 있기 때문이다. 원본(khatru)은 ephemeral 이벤트의 OK 사유를 무조건 `"broadcasted to N listeners"`로 덮어쓰고, `WebSocket.conn`이 비공개라 패키지 밖에서 가로챌 수도 없다. `OverwriteOK` 훅을 더해 해결했다 — `internal/relaycore/ORIGIN.md` 참고.

### 잊혀질 권리 (NIP-62)

사용자가 kind 62를 보내면 **계정 여부와 무관하게** 그 pubkey를 잊는다. 스펙이 유료·제한 릴레이도 예외가 아니라고 못박고 있다.

요청은 `relay` 태그에 이 릴레이의 `SERVICE_URL`(스킴·끝 슬래시 무시) 또는 `ALL_RELAYS`가 있어야 처리된다. 처리 내용:

1. 요청의 `created_at` **이하** 이벤트를 전부 삭제
2. 업로드한 파일의 소유권 해제 — 다른 사람도 올린 파일은 바이트가 남지만 이 사용자 몫의 용량은 환불된다
3. 삭제분만큼 용량 재계산
4. **컷오프를 기록**해서 같은 이벤트가 다시 올라오지 못하게 막는다 (스펙의 "MUST ensure the deleted events cannot be re-broadcasted")

컷오프 이후의 새 이벤트는 정상 동작한다 — 잊어달라고 한 뒤에도 계속 릴레이를 쓸 수 있다. 실수를 되돌리려면 `store.Unvanish`가 있지만 삭제된 이벤트는 돌아오지 않는다.

**결제 기록(`payments`)은 지우지 않는다.** 회계 자료라 보존 정책이 다르다. 지워야 하면 별도로 판단할 일이다.

### 정적 사이트 호스팅 (NIP-5A nsites)

패널 `요금 · 릴레이 정보`에 **nsite 호스팅 도메인**을 넣으면, 여기서 판 NIP-05 이름이 그대로 사이트 주소가 된다:

```
alice@sites.example.com  →  https://alice.sites.example.com
```

사용자는 파일을 Blossom으로 올리고 kind 15128 매니페스트(`["path", "/index.html", "<sha256>"]`)를 이 릴레이에 발행하면 끝이다. 우리가 릴레이이자 Blossom 서버라 **다른 relay나 `server` 힌트를 조회하지 않는다** — 매니페스트도 파일도 로컬에 있다.

- `/` 와 확장자 없는 경로는 `index.html`을 붙인다
- 없는 경로는 사이트의 `/404.html`로, 그것도 없으면 404
- blob은 콘텐츠 주소라 `Cache-Control: max-age=3600` + `X-Content-Type-Options: nosniff`

DNS는 `*.sites.example.com`을 이 서버로 향하게 하면 된다. 패널 자신의 호스트는 영향받지 않는다(Host로만 판별).

### 검색 확장 (NIP-50)

`domain:<도메인>`을 지원한다. **여기서 판 NIP-05 이름을 직접 조회**하므로 프로필 메타데이터를 믿는 다른 릴레이보다 정확하다.

```
["REQ","x",{"search":"domain:example.com 고양이","kinds":[1]}]
```

`domain:`은 작성자 필터로 바뀌고 나머지 단어만 본문 검색에 쓰인다. 이미 `authors`가 있으면 교집합이며, 해당 도메인에 유효한 이름이 하나도 없으면 **빈 결과**를 돌려준다(필터를 무시하지 않는다). 만료된 이름은 세지 않는다.

`include:spam`·`language:`·`sentiment:`·`nsfw:`는 스펙대로 **무시**한다 — 검색어에서 제거만 하고 결과에 영향을 주지 않는다. 제거하지 않으면 `nsfw:false 고양이`가 리터럴로 검색돼 아무것도 안 나온다.

### gift wrap (NIP-59)

kind 1059·13은 발신자·수신자만 조회할 수 있다(패널 `read_auth_required`와 무관하게 NIP-42 인증 필요). NIP-17 비공개 DM이 이 위에서 동작한다.

gift wrap은 실제 발송 시각을 숨기려고 `created_at`을 **최대 이틀까지 과거로** 무작위 설정한다. 그래서 `created_at` 과거 허용치를 설정해도 kind 1059에는 적용하지 않는다 — 적용하면 그 값을 켜는 순간 비공개 DM이 조용히 깨진다.

**미지원**: NIP-29(릴레이 기반 그룹). 별도 프레임워크([relay29](https://github.com/fiatjaf/relay29))가 필요하고 그룹 자체 권한 모델이 유료 화이트리스트와 충돌한다. 필요하면 그룹 전용 인스턴스를 따로 띄우는 편이 낫다.

## 빠른 시작

```bash
go build -o nostrel ./cmd/nostrel        # CGO 필요 (go-sqlite3)
cp .env.example .env                     # 값 채우기
export $(grep -v '^#' .env | xargs)
./nostrel
```

릴레이 엔진은 외부 의존성이 아니라 `internal/relaycore`에 들어 있다. khatru에서 출발했지만 우리 코드로 흡수했고, 따라갈 상류가 없다 — 배경은 `internal/relaycore/ORIGIN.md`.

로컬에서 결제 없이 흐름만 보려면 그냥 실행하면 된다. 새 DB는 `mock` 결제 백엔드로 시작하고, mock은 발행 즉시 결제된 것으로 처리한다:

```bash
PANEL_URL=http://localhost:3334 ./nostrel
```

**운영에서 mock을 켜둔 채로 두지 말 것.** 돈을 받기 전에 `관리자 → 결제 백엔드`에서 바꾼다. 바꾸기 전까지는 시작할 때마다 경고를 남긴다.

### 설정이 어디에 있나

환경변수에는 DB를 열고 관리자가 로그인하기 전에 알아야 하는 값만 둔다. **나머지는 전부 관리자 패널에서 편집하고 DB에 저장**하므로 재시작 없이 바뀐다.

| 변수 | 의미 |
|---|---|
| `RELAY_PORT` | 수신 포트 (모든 인터페이스) |
| `DB_PATH` | SQLite 파일. 이벤트·계정·설정이 전부 여기 들어간다 |
| `PANEL_URL` | 패널의 공개 https 주소. NIP-98 로그인을 이 값과 대조한다 |
| `SERVICE_URL` | 클라이언트에 광고할 공개 wss:// 주소 |
| `RELAY_SECRET_KEY` | 릴레이의 NIP-43 서명키 — 릴레이의 신원이라 DB에 넣지 않는다 |
| `ADMIN_PUBKEYS` | 패널·NIP-86 접근을 허용할 hex pubkey (쉼표 구분) |
| `ADMIN_PASSWORD_HASH` | 대체 비밀번호 로그인 (`nostrel hash-password '…'`) |
| `SESSION_SECRET` | 비밀번호 세션 쿠키 서명용. 없으면 재시작 시 세션이 사라진다 |

패널이 소유하는 값(`관리자`): 릴레이 이름·설명·아이콘·배너, 운영자 연락처·pubkey, 테마, 보관 기간, 국가·언어·주제, 요금, NIP-05 할증, NIP-46 relay, nsite 도메인, 자동 초대, kind 정책, 제3자 zap·배지 허용, `read_auth_required`, `min_pow`, `created_at` 허용치, 결제 백엔드와 자격증명, 미디어 백엔드와 `max_blob_size_mb`.

> **환경변수로 이 값들을 읽던 빌드에서 올라올 때:** 옮겨간 설정은 `.env`에서 자동으로 이전되지 *않는다*. 기본값으로 올라온 뒤 패널이 소유한다. `READ_AUTH_REQUIRED=true`, `MIN_POW`, `created_at` 허용치, 릴레이 이름, 결제 백엔드를 환경변수로 쓰고 있었다면 첫 시작 후 `관리자`에서 다시 설정한다. 비공개 릴레이를 다시 설정하지 않으면 **누구나 읽을 수 있는 상태로 올라온다.**

Docker:

```bash
docker build -t nostrel .
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env nostrel
```

### docker compose

`docker-compose.yml`이 저장소에 들어 있다. 기본값은 GHCR의 `latest` 이미지 +
이름 있는 볼륨(`nostrel-data`)이다.

```bash
cp .env.example .env      # 값 채우기 (PANEL_URL·SERVICE_URL·ADMIN_PUBKEYS는 필수)
docker compose up -d
docker compose logs -f
```

DB(`/data/nostrel.db`)와 업로드 파일(`/data/blobs`)이 같은 볼륨에 들어가므로,
컨테이너를 지웠다 다시 만들어도 볼륨만 남아 있으면 그대로 이어진다.

| 하고 싶은 것 | 명령 |
|---|---|
| 최신 이미지로 갱신 | `docker compose pull && docker compose up -d` |
| 중지 (데이터 유지) | `docker compose down` |
| 데이터까지 삭제 | `docker compose down -v` |
| 관리자 비밀번호 해시 생성 | `docker compose run --rm nostrel hash-password '비밀번호'` |
| 사용량 재계산 | `docker compose run --rm nostrel recompute-usage` |
| 로컬 소스로 빌드 | `image:` 줄을 `build: .`로 바꾸고 `docker compose up -d --build` |

버전을 고정하려면 `image:`를 `ghcr.io/pixelzx0/nostrel:rc-1.2.3`처럼 바꾼다 —
운영에서는 이쪽을 권한다. 이유는 아래 태그 설명 참고.

`.env`에서 `RELAY_PORT`를 바꿨다면 `ports:`의 컨테이너 쪽 포트도 같이 바꿔야 한다.
리버스 프록시 뒤에 둘 때는 `ports:`를 `- "127.0.0.1:3334:3334"`로 좁히고,
프록시가 `Host` 헤더와 websocket 업그레이드를 그대로 넘기게 한다 — NIP-05 도메인과
nsite 호스팅이 `Host`로 라우팅된다.

호스트 디렉터리를 직접 물리고 싶으면(`- ./data:/data`) 이미지가 uid 10001로 돌기
때문에 미리 `mkdir -p data && sudo chown 10001:10001 data`가 필요하다.

직접 빌드하지 않으려면 GHCR 이미지를 쓴다.

| 무엇을 하면 | 붙는 태그 |
|---|---|
| `main`에 push | `latest-test`, `test-2026-08-06-150102` (UTC) |
| `v1.2.3` 태그 push (릴리즈 생성 포함) | `latest`, `latest-rc`, `rc-1.2.3` |

```bash
# 최신 릴리즈
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env \
  ghcr.io/pixelzx0/nostrel

# 최신 개발본
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env \
  ghcr.io/pixelzx0/nostrel:latest-test

# 운영: 버전을 고정해서
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env \
  ghcr.io/pixelzx0/nostrel:rc-1.2.3
```

`latest`는 **릴리즈 태그를 올릴 때만** 움직인다 — `main`에 push해도 바뀌지 않으므로, 태그 없이 pull한 사람이 개발본을 받는 일은 없다. 다만 운영에는 `rc-<버전>` 고정을 권한다. 움직이는 태그는 롤백할 대상이 없다.

`test-<타임스탬프>`는 빌드마다 고유하다. 커밋은 이미지 라벨(`org.opencontainers.image.revision`)로 확인한다:

```bash
docker buildx imagetools inspect ghcr.io/pixelzx0/nostrel:latest-test --format '{{json .Provenance}}'
```

`.github/workflows/docker.yml`이 `linux/amd64` · `linux/arm64` · `linux/arm/v7`를 빌드해 하나의 매니페스트로 합친다. amd64와 arm64는 같은 아키텍처의 러너에서 **네이티브로** 빌드된다 — cgo(go-sqlite3)를 QEMU로 돌리면 몇 분씩 걸리기 때문이다. 32비트 ARM만 에뮬레이션이고, 필요 없으면 매트릭스에서 그 줄을 지우면 된다(`linux/386`도 같은 방식으로 추가 가능). 배포 전에 `go vet`과 `go test`가 먼저 통과해야 한다.

## 동작 방식

1. 사용자가 패널에서 pubkey를 연결(NIP-07)하고 기간·용량을 고른다.
2. `POST /api/order` → 라이트닝 인보이스 발행, `payments` 테이블에 pending 기록.
3. 결제되면 LNbits webhook 또는 30초 폴러가 이를 감지한다. **webhook 페이로드는 신뢰하지 않고** 결제 백엔드에 다시 조회한 뒤에만 권한을 부여한다.
4. 권한 부여는 단일 트랜잭션에서 `pending → paid` 전환과 함께 일어나므로 중복 적립이 불가능하다.
5. 이후 이벤트 수신 시 `RejectEvent`가 화이트리스트 · 만료 · 용량을 검사한다.

거부 메시지:

| 상황 | 클라이언트에 가는 메시지 |
|---|---|
| 미등록 pubkey | `restricted: not whitelisted — <PANEL_URL>` |
| 구독 만료 | `restricted: subscription expired — <PANEL_URL>` |
| 용량 초과 | `blocked: storage quota exceeded — <PANEL_URL>` |
| 밴 | `blocked: this pubkey is banned — <PANEL_URL>` |
| 그룹 구독 만료 | `restricted: the group subscription expired — <PANEL_URL>` |
| 그룹 용량 소진 | `blocked: the group storage quota is exhausted — <PANEL_URL>` |

## 용량 과금

이벤트 하나의 비용 = 그 이벤트의 JSON 바이트 수. 저장 시 `usage_events`에 기록하고 `accounts.used_bytes`를 올린다. 삭제하면 되돌려주고, replaceable 이벤트가 교체되면 옛 행을 정리한다. 드리프트가 의심되면:

```bash
./nostrel recompute-usage
```

구독을 갱신할 때마다 `included_mb`가 **누적**된다(선불 용량 모델). 갱신 시 초기화하고 싶다면 `internal/billing/billing.go`의 `grant`에서 `quota_bytes` 갱신을 대입으로 바꾸면 된다.

## 그룹 (용량 공유)

여러 pubkey가 하나의 용량 풀을 나눠 쓴다. 그룹은 개인 계정처럼 구독 기간과 용량을 사고, 좌석 제한 없이 멤버를 초대한다.

**공유되는 것은 저장 용량뿐이다.** 어떤 풀에서 빠져나갈지는 이벤트마다 정해진다:

1. 글쓴이 본인 계정이 활성·미만료이고 용량이 남아 있으면 → **개인 용량**에서 차감
2. 아니면 그룹이 활성·미만료이고 용량이 남아 있으면 → **그룹 용량**에서 차감
3. 둘 다 아니면 거부

개인을 먼저 쓰는 이유는 이미 개인 구독을 산 멤버의 결제분이 낭비되지 않게 하기 위함이다. 어느 풀이 냈는지는 `usage_events.group_id`에 남고, 이벤트를 지우면 **낸 쪽에** 환불된다.

한 pubkey는 그룹 하나에만 속한다(다른 그룹에 넣으면 이전 그룹에서 자동으로 빠진다). 어느 풀에 청구할지가 항상 한 가지로 정해지도록 하기 위한 제약이다.

- 구매: 패널 `그룹` 카드에서 이름을 정해 인보이스를 받는다. 이미 그룹이 있으면 같은 버튼이 연장·충전이 된다. 그룹에는 가입비가 붙지 않는다.
- 멤버 관리: **소유자**가 NIP-07으로 서명해 직접(`/api/group/{id}/members/...`), 또는 관리자가 패널에서.
- 그룹 구독만으로도 릴레이에 글을 쓸 수 있다. 개인 계정이 없어도 된다.

## 로그인 (공개 패널)

키를 넘겨받지 않고 pubkey만 확인하면 되므로, 공개 패널은 세 가지 경로를 제공한다.

| 방식 | 동작 |
|---|---|
| NIP-07 확장 | Alby·nos2x 등 브라우저 확장에 서명을 맡긴다 |
| **NIP-46 bunker** | 지갑에서 받은 `bunker://…` 문자열을 붙여넣는다 |
| **NIP-46 nostrconnect** | 패널이 QR/연결 문자열을 만들고 지갑 앱이 스캔한다 |
| pubkey 붙여넣기 | 읽기 전용. 그룹 멤버 관리는 불가 |

**NIP-46은 전부 브라우저에서 처리한다.** 릴레이가 서명자와의 세션을 대신 들고 있으면 운영자가 사용자의 서명자에게 임의의 서명을 요청할 수 있기 때문이다. 페이지는 일회용 키를 만들어 서명자와 kind 24133 이벤트를 주고받고(NIP-44 암호화, NIP-04 응답도 수용), 필요한 순간에만 `sign_event`를 요청한다. 사용자 키는 서버에 절대 오지 않는다.

암호화·서명은 벤더링한 [@noble](https://github.com/paulmillr/noble-curves) 모듈(`web/static/vendor/`)이 담당하고, 그 위의 NIP 계층만 `web/static/nostr.js`에 있다. 외부 CDN 요청은 없다.

**relay 설정이 필요하다.** nostrconnect 초대는 서명자가 닿을 수 있는 relay를 거쳐야 하는데, 이 릴레이는 화이트리스트라 계정 없는 pubkey의 이벤트를 받지 않는다. 그래서 별도 relay를 쓴다 — 패널 `요금 · 릴레이 정보`의 **NIP-46 로그인 relay** (기본 `wss://relay.nsec.app,wss://nos.lol`, 쉼표 구분, 하나라도 연결되면 동작). `bunker://` 문자열은 자체 relay를 포함하므로 이 설정과 무관하다.

## NIP-05 주소 판매

`name@domain` 형태의 식별자를 기간제로 판다. 한 서버가 여러 도메인을 동시에 서비스하며, 어느 도메인 요청인지는 **Host 헤더**로 판별한다.

```bash
curl -H 'Host: example.com' 'https://relay.example.com/.well-known/nostr.json?name=bob'
# {"names":{"bob":"<hex>"},"relays":{"<hex>":["wss://relay.example.com"]}}
```

- 요청한 이름 하나만 응답한다. 전체 목록을 내주면 고객 명단이 그대로 공개되기 때문이다.
- 브라우저에서 조회하므로 `Access-Control-Allow-Origin: *`가 항상 붙는다.
- 만료되면 응답에서 사라지고, 폴러가 행을 지워 다시 판매 대상이 된다. 재구매는 남은 기간 위에 쌓인다.

**도메인 추가**: 패널 `NIP-05 도메인`에서 도메인·가격(sats)·기간(일)을 넣는다. 그 도메인의 DNS를 이 서버로 향하게 하고, 리버스 프록시가 `Host` 헤더를 그대로 넘겨야 한다(caddy는 기본값, nginx는 `proxy_set_header Host $host;`).

**이름 정책**:

- 형식은 NIP-05 규격대로 `[a-z0-9_.-]`, 최대 30자. 입력은 소문자로 정규화된다.
- 차단 목록: `admin`, `support` 같은 이름을 모든 도메인에서 판매 금지로 등록한다. 이미 팔린 이름은 계속 동작한다(회수는 명시적 삭제).
- 짧은 이름 할증: `요금` 섹션의 `길이:배수` 목록. `1:20,2:10,3:5`면 한 글자 이름이 도메인 가격의 20배다. 비우면 할증 없음.
- 관리자가 결제 없이 이름을 직접 발급할 수 있고, 기간을 0으로 두면 무기한이 된다.

**동시 구매**: 인보이스를 발행하는 시점에 이름을 선점한다(인보이스 수명과 같은 1시간). 그래서 두 사람이 같은 이름에 각각 결제하고 한 명만 받는 상황이 생기지 않는다 — 두 번째 주문은 409로 거부된다.

릴레이 계정이 없어도 이름만 살 수 있다. 이름 구매는 계정 행을 만들지 않으므로, 나중에 릴레이에 가입할 때 가입비는 그대로 내야 한다.

## 결제 백엔드 설정

패널 `관리자 → 결제 백엔드`에서 LNbits / NWC / mock을 고르고 자격증명을 넣는다. **저장하면 재시작 없이 다음 인보이스부터 적용**된다. 이에 해당하는 환경변수는 없다. 새로 설치하면 `mock`으로 시작하고, 실제 백엔드를 설정할 때까지 부팅 로그가 매번 경고한다.

- **연결 테스트** 버튼: LNbits는 지갑 조회(`GET /api/v1/wallet`), NWC는 `get_info`를 호출해 자격증명·권한을 확인한다. 돈은 움직이지 않는다. 저장 전에 시험할 수 있다.
- **비밀값 취급**: 인보이스 키와 NWC 연결 문자열은 서버에만 있고 API는 끝 4자리만(`••••1234`) 돌려준다. 그대로 두고 저장하면 기존 값이 유지된다.
- 요금(가입비·구독료·기간·포함 용량·MB 단가)과 릴레이 이름·설명·아이콘도 같은 화면에서 수정하며, NIP-11에 즉시 반영된다.
- 백엔드를 만들 수 없는 설정(예: URL 없는 LNbits)은 저장 단계에서 거부한다.

## 미디어 호스팅

업로드는 **이벤트와 같은 용량 쿼터**를 소비하므로, 사용자가 산 MB 안에서 글과 파일을 함께 쓴다. 쿼터를 넘기면 402를 돌려준다. 파일 개당 상한은 패널의 `max_blob_size_mb`(기본 25MB).

### 저장 위치

패널 `관리자 → 파일 저장소`에서 고른다. 저장 즉시 **재시작 없이** 새 업로드부터 적용된다.

| 백엔드 | 설정 | 비고 |
|---|---|---|
| 내장 | 저장 경로 | 서버 디스크. 기본값, 패널에서 바꾸기 전까지 `./blobs` |
| S3 호환 | 엔드포인트·버킷·리전·접두사·키 | AWS S3, MinIO, Cloudflare R2, Backblaze B2 등 |

- **연결 테스트**: 프로브 객체를 쓰고 읽고 지워 자격증명과 권한을 실제로 확인한다.
- **시크릿 키**는 서버에만 저장되고 API는 끝 4자리만 반환한다. 그대로 두면 기존 값 유지.
- **공개 URL**(선택): 설정하면 다운로드를 그 주소로 302 리다이렉트해 바이트가 릴레이를 거치지 않는다. CDN·공개 버킷용.
- 엔드포인트는 `https://host`, `http://host:9000`, `host:9000` 모두 받는다. 스킴이 없으면 https로 본다.

**저장 위치를 바꿔도 기존 파일은 옮겨지지 않는다.** 옮기려면:

```bash
./nostrel migrate-blobs ./blobs   # 기존 로컬 디렉터리 → 현재 설정된 백엔드
```

이미 대상에 있는 파일은 건너뛰므로 여러 번 돌려도 안전하다.

### Blossom BUD

kind 24242 인증 이벤트(`t` 태그로 동작 지정, `expiration` 필수)를 `Authorization: Nostr <base64>`로 보낸다.

| BUD | 경로 | 설명 |
|---|---|---|
| BUD-01 | `GET /<sha256>[.ext]`, `HEAD /<sha256>` | 다운로드·존재 확인. 저장된 확장자와 요청 확장자가 달라도 해석 |
| BUD-02 | `PUT /upload` | 업로드. 쿼터 초과 시 402 |
| BUD-02 | `GET /list/<pubkey>` | 사용자 파일 목록 |
| BUD-02 | `DELETE /<sha256>` | 삭제. **업로더 본인 또는 관리자만**, 용량 환불 |
| BUD-04 | `PUT /mirror` | 다른 서버의 URL을 가져와 저장 |
| BUD-05 | `PUT /media` | 미디어 업로드 경로(`/upload`로 위임) |
| BUD-06 | `HEAD /upload` | 업로드 전 사전 승인(`X-SHA-256`, `X-Content-Length`) |
| BUD-09 | `PUT /report` | kind 1984 신고 → 관리자 큐에 적재 |

BUD-03(사용자 서버 목록, kind 10063)은 클라이언트가 쓰는 이벤트라 릴레이가 그냥 저장한다.

두 군데는 엔진의 기본 구현을 쓰지 않고 `internal/relay`에서 직접 처리한다:

- `PUT /report` — 상류 핸들러가 `r.Body.Read(nil)`로 읽어 본문이 항상 비어 파싱에 실패한다.
- `PUT /mirror` — 상류가 사용자가 준 URL을 정책 훅보다 먼저 `http.Get`으로 그대로 가져와, 결제한 사용자가 릴레이를 통해 내부망·클라우드 메타데이터(169.254.169.254)를 찌를 수 있다. 여기서는 루프백·사설·링크로컬 주소로의 연결을 거부하는 다이얼러로 가져오고, 저장 전에 크기와 쿼터를 확인한다.

NIP-96 (NIP-98 인증):

| 메서드 | 경로 | 설명 |
|---|---|---|
| GET | `/.well-known/nostr/nip96.json` | 서버 설정 |
| POST | `/nip96` | multipart `file` 업로드 → NIP-94 태그 반환 |
| DELETE | `/nip96/<sha256>` | 업로더 본인 또는 관리자만 |

두 프로토콜은 같은 저장소·같은 소유권 인덱스(kind 24242 이벤트)를 쓴다. Blossom으로 올린 파일을 NIP-96 URL로 내려받을 수 있고, 용량은 한 번만 청구된다. 같은 내용을 여러 사람이 올리면 디스크에는 한 벌만 남고 첫 업로더에게만 청구된다.

관리자 패널 `미디어` 섹션에서 전체 파일 목록·크기·업로더를 보고 강제 삭제할 수 있으며, 신고된 파일은 상단에 따로 표시된다(무시 또는 삭제 선택).

예시:

```bash
SHA=$(shasum -a 256 pic.png | cut -d' ' -f1)
AUTH=$(nak event -k 24242 -t t=upload -t x=$SHA -t expiration=$(($(date +%s)+300)) --sec $SK | base64)
curl -X PUT https://relay.example.com/upload -H "Authorization: Nostr $AUTH" --data-binary @pic.png
```

## 관리자

`PANEL_URL/admin`

- NIP-98: `ADMIN_PUBKEYS`에 있는 키로 서명. 요청마다 서명하며 URL·메서드·본문 해시가 모두 검증되고, 60초 창 + 재사용 차단이 적용된다.
- 비밀번호: `nostrel hash-password '비밀번호'`로 해시를 만들어 `ADMIN_PASSWORD_HASH`에 넣는다. 세션 쿠키는 HttpOnly · SameSite=Strict · 12시간.

**주의:** NIP-98의 `u` 태그는 `PANEL_URL` + 요청 경로와 정확히 일치해야 한다(다른 사이트에서 받은 서명의 재사용을 막기 위함). 브라우저에서 접속하는 주소와 `PANEL_URL`이 다르면 서명 로그인이 실패한다.

### NIP-86 관리 API

같은 관리자 키로 표준 릴레이 관리 클라이언트에서 전체 메서드를 쓸 수 있다.

| 분류 | 메서드 | 동작 |
|---|---|---|
| pubkey | `allowpubkey` / `banpubkey` / `listallowedpubkeys` / `listbannedpubkeys` | `allowpubkey`는 현재 요금표 기준 1주기를 무상 지급 |
| 이벤트 | `banevent` / `allowevent` / `listbannedevents` / `listallowedevents` | 밴하면 저장본을 삭제하고 재업로드도 차단 |
| 모더레이션 | `listeventsneedingmoderation` | kind 1984(NIP-56) 신고 중 미처리 항목 |
| kind | `allowkind` / `disallowkind` / `listallowedkinds` / `listdisallowedkinds` | allow 목록이 비어있지 않으면 그것만 허용 |
| IP | `blockip` / `unblockip` / `listblockedips` | 웹소켓 연결 자체를 거부 |
| 메타 | `changerelayname` / `changerelaydescription` / `changerelayicon` | NIP-11에 즉시 반영 |
| 관리자 | `grantadmin` / `revokeadmin` | 런타임 관리자 추가·회수. `ADMIN_PUBKEYS`로 들어온 키는 회수 불가 |
| 기타 | `stats` | 계정·이벤트·저장량·결제 통계 |

`grantadmin`/`revokeadmin`은 go-nostr의 NIP-86 디코더가 파라미터를 `[]string`으로 무검사 단언해 panic이 나므로(`internal/relay/nip86fix.go` 참고) 우리가 직접 처리한다. 상류가 고쳐지면 그 파일을 지우면 된다.

## API

| 메서드 | 경로 | 설명 |
|---|---|---|
| GET | `/api/info` | 릴레이 정보 + 요금표 |
| POST | `/api/order` | 인보이스 발행. 아래 세 조합 중 하나 |
| GET | `/api/invoice/{hash}` | 결제 상태(백엔드 재조회, 2초 쿨다운) |
| GET | `/api/invoice/{hash}/qr.png` | 인보이스 QR |
| GET | `/api/account/{pubkey}` | 구독 상태 · 용량 · 소속 그룹 |
| GET | `/.well-known/nostr.json?name=` | NIP-05 조회 (CORS 허용) |
| GET | `/api/nip05/domains` | 판매 중인 도메인·가격 |
| GET | `/api/nip05/check?domain=&name=` | 구매 가능 여부 + 가격 |
| GET | `/api/nip05/names/{pubkey}` | 그 pubkey가 가진 주소 |
| GET | `/api/group/{id}` | 그룹 상태 · 용량 · 멤버 수 |
| GET·PUT·DELETE | `/api/group/{id}/members[/{pubkey}]` | 멤버 관리 (**소유자 NIP-98 서명** 또는 관리자) |
| POST | `/webhook/lnbits` | LNbits 결제 알림(재검증 후 처리) |
| — | `/api/admin/*` | 통계 · 계정 · 그룹 · NIP-05 · 결제 · 설정 (관리자 전용) |

`POST /api/order` 본문:

```jsonc
{"pubkey": "<hex>", "periods": 1, "extra_mb": 0}                    // 릴레이 접근
{"pubkey": "<hex>", "group_name": "team", "periods": 1}             // 새 그룹 (연장은 group_id)
{"pubkey": "<hex>", "nip05_domain": "example.com",
 "nip05_name": "bob", "nip05_periods": 1}                           // NIP-05 주소
```

LNbits 인보이스에는 `PANEL_URL/webhook/lnbits`가 webhook으로 자동 등록된다. LNbits에서 패널로 접근할 수 없어도 폴러가 30초 안에 결제를 잡는다.

## 패널 언어

웹 패널은 한국어와 영어를 지원한다. 첫 방문 때 브라우저 언어를 따르고 이후에는
선택을 기억한다. 전환 메뉴는 사이드바 하단(관리자 로그인 화면에도)에 있다.

번역은 `web/static/i18n.js` 한 파일에 있다. **한국어 원문이 키**라서 한국어는
사전이 필요 없고, 번역이 없는 문자열은 빈칸이 아니라 한국어로 남는다. 정적
마크업은 문서를 한 번 훑어 번역하고, 자바스크립트가 만드는 문자열은 `t()`를
거친다 — `{0}`, `{1}` 자리표시자도 함께 채운다.

언어를 추가하려면 그 파일의 `dict`에 객체 하나, `LANGS`에 코드 하나,
`web/index.html`과 `web/admin.html`의 선택 메뉴에 `<option>` 하나를 넣으면 된다.
빠뜨린 게 없는지는:

```bash
node web/i18n_test.mjs   # 두 페이지를 렌더링해 번역 안 된 문자열이 있으면 실패
```

## 운영 메모

- 리버스 프록시(caddy/nginx)로 TLS 종료 후 `X-Forwarded-Proto`/`X-Forwarded-For`를 넘길 것.
- 백업은 릴레이를 멈추고 `nostrel.db*` 세 파일을 복사하거나 `sqlite3 nostrel.db ".backup"` 사용.
- LNbits 응답 필드는 버전마다 다르다. 연동 전 대상 인스턴스의 `/docs`(Swagger)로 확인할 것 — 현재 코드는 `payment_request`/`bolt11`, `paid`/`status` 양쪽을 모두 받는다.

## 테스트

```bash
go test ./...   # 과금 멱등성 · 용량 경계 · NIP-98 · DM 필터 권한 · blob 경로/쿼터
                # · NIP-05 선점/만료/할증 · 그룹 개인우선 과금 · 스키마 마이그레이션
```

E2E는 [nak](https://github.com/fiatjaf/nak)으로:

```bash
nak event -k 1 -c "hello" --sec $SK ws://localhost:3334        # 미결제면 거부
nak event -k 1 -c "hi" --pow 8 --sec $SK ws://localhost:3334   # min_pow가 8일 때
nak req -k 1059 -p $PK --auth --sec $SK ws://localhost:3334    # DM은 당사자만
nak count -k 1 ws://localhost:3334                             # NIP-45
nak req -k 1 --search "keyword" ws://localhost:3334            # NIP-50
nak sync ws://relay-a ws://relay-b -k 1                        # NIP-77
curl -s -H 'Accept: application/nostr+json' http://localhost:3334/ | jq '.supported_nips, .fees'
```
