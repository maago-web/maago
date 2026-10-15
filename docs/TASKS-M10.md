# M10 작업 목록 — 연구 반영과 첫 릴리스

[MILESTONES.md](MILESTONES.md)의 M10을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).
근거는 [RESEARCH-AGENT-FIT.md](RESEARCH-AGENT-FIT.md), 결정은 D-147~D-153과 아래의 고침.

**M10 완료 조건** (MILESTONES)
- 게이트의 에이전트가 짚은 함정(D-148의 목록)이 각각 시험으로 막혀 있다 → 2·3번
- 뷰의 서명이 사용자와 DB를 말하고, 쓰기 뷰는 요청 트랜잭션 안에서 돈다. 게시판 튜토리얼이 그 꼴로 처음부터 끝까지 걷는다 → 4~9번
- 태그·쿼리 파일·뷰·폼의 문법을 `maago doc`과 `go doc`에서 읽을 수 있고, 문서의 예는 시험이 돌린다 → 10·11번
- 저장소가 공개되고 v0.1.0이 태그되어, 빈 디렉터리에서 `go install …/cmd/maago@v0.1.0`부터 에이전트 게이트를 다시 지난다 → 12·13번

**앞에서 넘어온 것**
- M9 2번(첫 릴리스, 두 모듈 기준이던 계획 → D-151로 태그 하나), 11번(마지막 게이트), 8번(v0 끝) → 12·13·14번
- TODO M8: 운영 DB에 개발 기계가 닿지 않는 곳의 마이그레이션 → 8번(D-153)
- TODO 첫 릴리스 전 셋(`replace`, 튜토리얼의 태그한 판) → 1·12번

**순서의 이유**: 문서(10·11번)는 API가 끝난 뒤에 쓴다. 트랜잭션(5번)이 auth 옮기기(6번)보다 앞이라 auth를 한 번만 고친다(4번 리뷰에서 바꿈). 새 꼴(4~8번)은 옛 꼴과 함께 받아 걷기가 작업마다 초록이고, 게시판을 한 번만 다시 쓰는 9번에서 옛 꼴을 지운다(지우기와 새 생성 오류가 게시판을 고치는 커밋과 같이 들어간다).

---

## 먼저 확인한 사실 (2026-10-15, 초안 리뷰로 고침)

- **바뀔 사용처**: 게시판 마지막 단계의 뷰 16개(그중 탈출구 뷰 2: CSV·다운로드), 라우트 22줄(그중 `maago.LoginRequired` 15), `UserFrom` 4, `DBFrom` 16. 복사되는 auth의 라우트 20줄, GET과 POST를 한 함수가 받는 짝 9(나눌 함수는 게시판 `NewBoard`·`NewPost`·`ShowPost`(댓글)·`EditPost`와 합쳐 13). 게시판 패치 33개, 6,236줄
- **`DBFrom`·`UserFrom`에 기대는 곳**: 탈출구 뷰, admin, `Config.Auth`(`auth.FromSession`이 DB를 읽는다), 라이브러리 코드. 그래서 지우지 않는다(Q4)
- **루트 패키지의 export 약 94개**, 그중 약 40개는 생성 코드만 쓴다(`Bind*`·`Parse*`·`Render`·`Pages`·`URLFunc`…, ARCHITECTURE)
- **사용자에게 보이는 `(D-xx)`**: 진단 글자열 약 20, `go doc -all`에 83. 검사 범위는 진단만이 아니라 `go doc -all`, 생성된 프로젝트, 골든, 사용자 문서
- **이미 되는 것**: 모델 결과의 `SELECT *`(실측), pgx `(*PgError).SQLState()`·modernc `(*Error).Code()`(unique 위반 판정, D-148의 뒤로 미룬 일에 쓴다)
- **모듈 하나**: 런타임 패키지만 import하는 사용자 모듈에 드라이버가 들어오지 않는다(`replace`와 `file://` GOPROXY로 실측, M9 리뷰). 공개 프록시 경로만 12번에 남는다
- **시험의 시간**: `cmd/maago` 로컬 약 7~8분, CI 13분 25초(`-timeout 30m`). 게시판을 다시 쓰면 Catches 줄마다 걷기가 다시 돈다
- **CI의 Go 1.25**: cookiejar가 http에서 `Secure` 쿠키를 버린다(1.26은 지닌다). `TestShortTutorial`의 클라이언트가 지금은 쿠키를 직접 들고 다닌다(6c787ee), 2번이 근본을 고친다
- **리뷰어**: codex는 백그라운드로 부르면 돈다, fable은 사용 한도가 자주 찬다(그때는 general-purpose Opus가 대신하고 기록에 적는다)

## 질문과 답 (사람의 결정, 2026-10-15, 모두 추천)

- **Q1 튜토리얼 글의 언어**: 9번에서 다시 쓰며 영어로(게시판의 커밋 메시지, TUTORIAL-FUTURE). D-103을 고친다
- **Q2 `Secure`**: `auth.SiteURL`의 scheme이 정한다(`https`면 붙이고 `http`면 뺀다). 호스트 이름을 보지 않는다. D-128을 고친다
- **Q3 쓰기 뷰의 경계**: POST(GET이 아닌) 라우트의 타입 있는 뷰가 `maago.DB`만 받으면 생성 오류(`fix: take maago.Tx`). 오류든 panic이든 롤백. 실패해도 남아야 하는 쓰기(시도 횟수 같은)는 `Tx`와 함께 `maago.DB`도 받아 그쪽으로. D-150을 고친다. → 5번 계획 리뷰에서 다시 정함: `Tx`와 `DB`를 함께 받지 않는다(교착), 남아야 하는 쓰기는 `Show`와 nil로 커밋
- **Q4 `DBFrom`·`UserFrom`**: 탈출구 뷰·라이브러리·`Config.Auth`를 위해 남긴다. 타입 있는 뷰 안에서 부르면 생성 오류(`fix: take it as a parameter`). D-149를 고친다

## 작업 (초안 리뷰 뒤)

1. **모듈 하나**(D-151, D-03을 고침): `cmd/maago`의 `go.mod`를 지우고 루트 모듈에 합친다(드라이버는 루트의 `require`). 런타임 패키지의 import 허용 목록 시험(stdlib + 이 모듈의 런타임 패키지만, D-09·D-66)이 드라이버를 막는지 일부러 어겨 본다. `gen`이 생성기와 런타임의 판을 맞춰 본다. CI·시험의 임시 프로젝트(`replace`)·`steps.sh`·문서 → 확인: 한 `go test ./...`, `go mod tidy -diff`, 허용 목록 시험이 드라이버 import에 실패, 판이 다른 프로젝트에 `gen` 오류

   계획 (리뷰 뒤)
   - 루트 `go.mod`가 `cmd/maago/go.mod`의 `require`를 받고, `cmd/maago/go.mod`·`go.sum`을 지운다. `go mod tidy`. testdata의 모듈(`cmd/maago/testdata/web`, `internal/…/testdata/example`)은 그대로 다른 모듈. 루트 `go.work`는 만들지 않는다. 시험의 임시 프로젝트와 `steps.sh`의 `replace … => 저장소`는 그대로(개발의 일, 릴리스 경로는 12번)
   - **`go 1.25.0`**: 드라이버(pgx, sqlite, libc, x/sys)의 `go.mod`가 `go 1.25.0`이라 tidy가 루트를 `go 1.25` → `go 1.25.0`으로 올리고, maago를 require하는 `go 1.25` 모듈은 `go list`에서 "updates to go.mod needed"로 실패한다(Opus 실측: `internal/gotypes`·`internal/web`의 시험 프로젝트). 시험의 `go 1.25`를 `go 1.25.0`으로. 대가: 드라이버가 `go` 줄을 올리면 런타임만 쓰는 사용자의 최소 Go도 오른다 → D-04·D-151에 적는다(`go mod init`은 패치 판을 쓰니 새 사용자는 걸리지 않는다)
   - **import 허용 목록**(D-09를 모듈 경계 대신): 루트의 `TestRuntimeImports`가 `TestNoReflect`처럼 파일을 걸으며 import만 읽는다(`go list -deps`는 빌드 제약 뒤의 import를 놓친다: `//go:build linux` 파일, Opus 실측). 대상은 `_test.go`가 아닌 `.go` 전부, `cmd/maago` 아래·`testdata`·`.git`은 건너뛴다. 받는 것: 첫 경로 요소에 점이 없는 것(stdlib, `"C"`는 아님), `github.com/maago-web/maago`와 같거나 `…/`로 시작하는 것. 직접 import만 보아도 모든 파일을 보므로 닫힌다. 게이트 시험은 `TestGeneratedImportsGate`처럼 임시 트리로: 빈 드라이버 import, build-tag 파일, `"C"`, `github.com/maago-webx/…`
   - **판 맞추기**(D-151의 "생성물에 생성기 판"을 고침): 생성물의 머리와 런타임 기동 검사 대신, `gen`·`check`가 이른 단계(DB 단계와 타입 검사 앞)에서 `go list -m -e -json github.com/maago-web/maago`를 한 번 부른다(`gotypes.Check`는 라우트가 있을 때만 DB 뒤에 돌고 그 출력에 의존의 판이 없다). 제 판은 `debug.ReadBuildInfo`: `Main.Path`가 이 모듈이면 `Main.Version`, 아니면 `Deps`에서(`go tool`로 지은 경우, `Replace`면 건너뜀). 양쪽이 모두 `^v[0-9]+\.[0-9]+\.[0-9]+$`일 때만 비교한다(의사 판, `+dirty`, `(devel)`, 빈 판(go.work의 `use`), 프리릴리스, 프로젝트의 `Replace`, require 없음·`go list` 실패는 조용히 건너뜀). 다르면 `this maago is vX but the project uses github.com/maago-web/maago vY` / `fix: go install github.com/maago-web/maago/cmd/maago@vY, or go get github.com/maago-web/maago@vX`. D-117("go list는 라우트가 있을 때만")을 고친다
   - CI는 한 단계(`go mod tidy -diff && go vet ./... && go test -timeout 30m ./...`). 문서: CLAUDE.md의 확인 명령, WORKFLOW(두 모듈에서 돌리던 확인), ARCHITECTURE, TODO(첫 릴리스 전의 `replace`·루트 먼저 태그), DECISIONS의 D-03·D-04·D-09·D-89·D-90(실제 DB를 여는 시험은 `cmd/maago`에: 그대로, 허용 목록은 `_test.go`를 보지 않는다)·D-117·D-146·D-151. `TestNoReflect`의 주석
   - 확인: 한 `go test ./...`가 전부(시험 수가 합친 전과 같다), `go mod tidy -diff`, 허용 목록의 게이트 시험, 판 비교의 단위 시험(같음, 다름, 의사 판, `+dirty`, `(devel)`, 빈 판, 프로젝트의 `Replace`, `Deps`의 maago)과 `gen`이 그 오류를 내는 종단 시험 하나(판을 주입)
   - 구현 리뷰: codex, Opus. 받음: ARCHITECTURE에 남은 두 모듈 문구 둘, 드라이버 판의 하한(D-151의 대가), D-09에 "생성기도 stdlib만", WORKFLOW의 `tidy -diff`. Opus가 `file://` 프록시의 v0.2.0 CLI로 v0.1.0 프로젝트에 `gen`·`check`·`makemigrations`가 멈추는 것을 실측. 받지 않음: `migrate`의 판 검사(런타임 코드를 생성하지 않는다), `vendor` 건너뛰기(없다). 10번으로: TUTORIAL-FUTURE에 "`go get -u` 뒤 CLI도 `go install …@latest`"
   - 하지 않는 것: 런타임의 기동 판 검사, 시험의 `replace` 없애기와 태그한 판의 종단 시험(12번), 판을 넣는 빌드 플래그
   - 계획 리뷰: codex, Opus(fable은 사용 한도). 받음: `go 1.25.0`(Opus 실측, 막음), 허용 목록은 파일 걷기와 정확한 접두(빌드 제약, `"C"`, `maago-webx`), 판은 따로 부르는 `go list -m`(Check는 라우트가 있을 때만)과 정식 판의 문자열 규칙, `go tool`의 `Deps`, 루트 `go.work`를 만들지 않음, 문서 목록(D-04·D-117·D-146·TODO). 12번으로 넘김: 태그한 CLI와 태그한 런타임의 종단 시험(codex), `go get -tool …/cmd/maago`와 `go tool maago`(판이 구조상 같다, 대가는 사용자 `go.sum`의 드라이버, Opus의 제안)
2. **런타임의 작은 함정**(D-148, D-119·D-128을 고침): `Valid()`가 필드의 `Error`도 센다, 쿠키의 `Secure`를 `auth.SiteURL`의 scheme으로(Q2, `TestShortTutorial`의 직접 쿠키를 다시 `cookiejar`로), 성공한 `-createsuperuser`의 한 줄(복사본) → 확인: 함정마다 재현 시험이 먼저 실패, `TestShortTutorial`을 `GOTOOLCHAIN=go1.25.14`로도

   계획 (리뷰 뒤)
   - **`Valid()`**(D-119 고침): 생성기가 쓰이는 폼마다 `func (f XForm) Valid() bool`을 쓴다(`views_gen.go`의 `bindX` 옆): 폼 전체가 맞고(`maago.FormValid(f.Form)`) 모든 필드의 `Error`가 비었을 때. 뷰가 `f.Title.Error = "taken"`을 쓰면 `f.Valid()`가 거짓이 된다(게이트의 500). 값 수신자라 뷰가 받은 사본과 템플릿의 주소 없는 값에서 불린다(`bindX` 안의 클로저는 어댑터의 원본을 보아 틀린다). 런타임의 `maago.Form.Valid` 메서드는 `maago.FormValid(f Form) bool`(생성 코드만 부른다)로 바꾼다: `f.Form.Valid()`가 필드를 세지 않는 둘째 뜻으로 남지 않게 컴파일 오류로(Opus). 쓰이는 폼이 `Valid` 메서드나 필드를 선언하면 생성 오류(`validates`처럼 `typecheck.go`에서, 위치는 사용자의 선언: 그대로 두면 오류가 생성 파일에 나 "maago bug"의 fix가 붙는다, Opus 실측)
   - **`Secure`**(D-128 고침, Q2): `contrib/auth.SessionCookie(token, path, maxAge, secure bool)`. 복사되는 auth가 판단을 한 곳에: `func cookieSecure() bool { site, err := siteURL(); return err != nil || !strings.HasPrefix(site, "http://") }`(틀리거나 빈 `SiteURL`은 붙인다: 안전한 쪽, 그 값은 메일에서 이미 오류). 다섯 호출처(로그인·로그아웃·인증·비밀번호·탈퇴)가 같은 값. `SiteURL`의 문서에 "scheme이 세션 쿠키의 `Secure`도 정한다: 개발은 http://localhost:PORT, 운영은 https". `TestShortTutorial`의 `keepAllJar`를 `cookiejar`로 되돌린다(로그인 뒤 admin, 로그아웃까지)
   - **`-createsuperuser`의 한 줄**: `log.Printf("created superuser %s (id %d)", u.Email, u.ID)`(admin의 `author_id`에 id가 필요했다, short-1). 복사본의 패키지 문서와 TUTORIAL-FUTURE의 `main`, `TestShortTutorial`이 그 출력을 본다. 게시판 0021은 9a에서
   - 확인: 재현 시험이 고치기 전에 실패(폼: 뷰가 필드 오류를 둔 종단 시험, 쿠키: 복사본에서 `SiteURL`이 http·https·`HTTP://`·빈 값, `TestShortTutorial`을 `GOTOOLCHAIN=go1.25.14`로), `Valid` 선언의 오류 표 두 줄(메서드, 필드), 생성기 골든, 전체
   - 하지 않는 것: `Fail` 같은 새 API(쓰던 코드 `f.X.Error = …`가 그대로 맞게), 호스트 이름으로 `Secure` 판단(Q2). 9b로: 0033에서 `BOARD_SITE_URL`이 없으면 http 기본값 대신 오류(운영에서 `Secure`가 조용히 빠지지 않게)
   - 구현에서 바뀐 것: `Valid`는 `views_gen.go`가 아니라 따로 `forms_gen.go`에(뷰가 부르므로 타입 검사 전에 있어야 하고 `views_gen.go`는 그 뒤에 생긴다), `Valid` 선언은 `typecheck.go`가 아니라 `readApp`·`Forms`가 구문으로 찾는다(타입 검사의 오류는 생성 파일에 난다)
   - 구현 리뷰: codex, Opus. 받음: 어느 In도 읽지 않는 폼에도 `Valid`(라우트를 달기 전의 뷰가 막혔다, Opus 실측) → 모든 폼에, 선언 검사도 모든 폼에, 괄호 수신자(`ast.Unparen`), ARCHITECTURE의 생성 코드 전용 목록에 `FormValid`, `TestShortTutorial`이 로그아웃 뒤 admin이 303인지. 받지 않음: 별칭 수신자의 `Valid`(드물다), 지난 TASKS의 옛 API(기록)
   - 계획 리뷰: codex, Opus(fable은 사용 한도). 받음: `Valid` 이름 겹침의 명시 진단, `Form.Valid`를 함수로(Opus), 쿠키는 `url`의 문자열 접두 대신 복사본의 `siteURL()`로(둘 다), id, D-119, 로그아웃 쿠키, 0033의 기본값(9b로)
3. **생성기의 작은 함정과 진단**(D-148): `choices` 구분자의 오류, 모델 폼의 포인터 필드는 optional, `:one` 스칼라(`bool`·`int64`·`string`·`time.Time`), 이름 바꾸기·DROP의 ⚠를 stderr에, `gen`의 성공 한 줄(`generated N files`), `go list` 오류 전문, In의 fix에 `maago:"form"`, 모델 결과의 fix에 `SELECT *` → 확인: 게이트 기록의 오류마다 시험. `(D-xx)` 빼기는 `see:`와 함께 10번에서

   계획 (게이트 기록의 줄마다, 리뷰 뒤)
   - **`choices` 구분자**(board-1·2: `choices=draft|published`를 한 값으로 읽고 `fix: use one of: draft|published`): 값에 `|`·`;`가 있으면 `choices in Post.Status are separated by /` / `fix: list them with /, like choices=draft/published`(`model/check.go`, choices를 받지 않으니 `default`의 둘째 오류가 없다). `,`·공백은 태그 파서가 먼저 막는다(Opus)
   - **모델 폼의 포인터 필드**(board-1: `PinnedFrom *time.Time`이 폼에서 필수): 모델 필드가 `Pointer`면 폼 필드는 `optional`(`web/forms.go` `modelRules`). `*bool`은 빼다: 체크박스는 없으면 false이고 `optional`을 받지 않는다
   - **`:one`·`:many` 스칼라**(board-1·2: `SELECT EXISTS(…)`에 이름 붙인 타입이 필요했다): 결과 타입 자리에 `bool`·`int64`·`int32`·`float64`·`string`·`time.Time`(`[]byte`는 아님). 손댈 곳(리뷰): 헤더가 스칼라를 따로 읽고(소문자의 다른 이름은 `unknown type`의 제안), "has no result type"의 fix에 `or a type, like :one bool`, 열 이름 검사(AS 없는 `EXISTS(…)`)·결과 공유·이름 겹침 검사에서 뺀다, `-- column:` 줄과 함께면 오류, 열은 하나. 모델 필드에서 온 열은 같은 타입이고 포인터(또는 바깥 조인)가 아니어야 한다(`fix: COALESCE it, or name a result type`). 식은 그 타입으로 선언된 것으로 본다: `-- column: x T`처럼 NULL이 아니라고 믿는다(두 드라이버 모두 식의 NULL을 모른다). PG OID 검사는 위치 `-- name:`, fix는 `cast …::T` 또는 다른 스칼라. 생성: 결과 타입 선언 없음, `Scan(&v)`(SQLite 시각은 `ScanSQLiteTime`), 영값 `false`·`0`·`""`·`time.Time{}`
   - **⚠를 stderr에**(board-1: 이름 바꾸기·DROP의 ⚠가 파일 안에만): `makemigrations`가 쓴 파일의 `-- ⚠` 줄을 `created …` 뒤 stderr에 `migrations/0003_x.sql:5: ⚠ …`로(진단이 아니라 exit 0). 그리고 DROP COLUMN을 막는 `migrate`·`check` 진단의 fix에 "이름 바꾸기라면 파일을 지우고 `--rename T.old:new`"(fix만 따르는 에이전트가 `allow-drop`으로 데이터를 잃지 않게, Opus)
   - **`gen`의 성공 한 줄**(short-1·2: 조용한 성공): `generated N files`(N은 생성 파일 수, 쓴 것과 그대로인 것 모두. 쓴 수는 서명을 바꿔야 해 뺀다). `main_test`와 TUTORIAL-FUTURE §5의 출력 블록
   - **`go list` 오류 전문**(M9 3번: `go list failed: \tgo work use` 한 줄): `go: downloading` 줄을 빼고 줄마다 TrimSpace해 `; `로 이은 한 줄(진단은 한 줄, D-62). fix는 `do what go says above, then make sure go build works`
   - **In의 fix에 `form`**: `with fields tagged maago:"path=name", maago:"query=name" or maago:"form"`. 매개변수가 (포인터) 폼 타입이면(게이트의 꼴 `func NewBoard(ctx, f *BoardForm)`) `take it as a field: type NewBoardIn struct{ Form BoardForm \`maago:"form"\` }`
   - **모델 결과의 fix에 `SELECT *`**: `select every column of Post, like SELECT * or SELECT p.*, or declare …`
   - **선언 안 된 파라미터를 모두**(board-1·2 0005: 한 줄의 `limit`만 나오고 `offset`은 다음 실행에): 줄을 끝까지 읽고, fix의 `-- param: id type`에 타입 목록
   - 확인: 줄마다 재현 시험이 먼저 실패(오류 표·골든의 새 줄, 스칼라는 SQLite·PG의 생성과 실행), 전체
   - 하지 않는 것: `(D-xx)` 빼기(10번), 포인터 스칼라(`:one *string`), 집계(`max`·`sum`)의 NULL 판단(10번의 문서에 "COALESCE"), 모델 폼 라벨의 기본값
   - 구현에서 더한 것: 스칼라 쿼리의 변형은 열 이름을 비교하지 않는다(SQLite는 `EXISTS (… IN (?1, ?2))`처럼 식을 글자로 이름 지어 슬라이스의 길이마다 다르다)
   - 구현 리뷰: codex(막는 것 없음), Opus(게이트의 꼴을 빌드한 maago로 실측). 받음: SQLite의 이름 바꾸기 초안은 표를 다시 만들어 막는 진단이 `apply.go`의 것이었다 → 그 fix에도 `--rename 표.열:NEW`를 앞에, DROP의 fix는 rename을 먼저, 소문자 결과 타입은 `unknown result type`과 제안, `go list`의 `:` 뒤는 공백으로, 두 `:one bool`의 시험. 받지 않음: 같은 줄의 같은 파라미터 두 번(전부터), PG의 다른 스칼라 fix에 cast도. 10번으로: 스칼라와 집계의 NULL을 TUTORIAL-FUTURE §6에
   - 계획 리뷰: codex, Opus. 받음: 위의 스칼라 손댈 곳과 NULL의 한계, 진단은 한 줄(`go list`를 `; `로), 쓴 수 빼기, `main_test`·튜토리얼 출력, DROP의 fix, 폼 타입을 In에 둔 꼴, 선언 안 된 파라미터 모두
4. **뷰의 서명, 생성기**(D-149): `internal/web`이 서명에서 사용자(`U` 필수·`*U` 선택)와 DB(`maago.DB`·`maago.Tx`)를 읽고 어댑터가 넘긴다. 매개변수의 순서와 꼴, 사용자 타입은 서명에서 추론하고 뷰들 사이에 어긋나면 생성 오류, GET 라우트의 `Tx`는 생성 오류, 사용자 매개변수가 있는 뷰에 `LoginRequired`는 중복 오류. 옛 꼴(`LoginRequired`·`UserFrom`·`DBFrom`)은 아직 받는다 → 확인: 생성기의 골든과 오류 표, 비로그인 요청이 사용자 필수 뷰에서 로그인으로 보내지는 시험, 걷기 초록

   계획 (리뷰 뒤)
   - **꼴**: `func V(ctx context.Context, [u U | u *U], [db maago.DB], in In) (P | maago.Result[P], error)`. 차례는 ctx, 사용자, DB 종류, In으로 고정(문서와 fix가 하나의 꼴을 말한다). 5번이 DB 종류에 `tx maago.Tx`를 더한다(`ctx, [u], [tx], [db], in`): 판독은 "선택인 사용자 하나, 그다음 DB 종류 0~2개"로 써 둔다. In은 늘 있다(없으면 `struct{}`): 매개변수가 둘인 `(ctx, User)`를 자리 말고는 가를 수 없다
   - **구문(Views)**: In은 마지막. 가운데: `maago.DB`는 DB, 그 밖은 사용자 후보. 마지막이 In의 꼴(이 패키지의 struct 이름이나 `struct{}`)이 아니면 `view Me takes no In: In comes last, even when empty` / `fix: func Me(ctx context.Context, u auth.User, in struct{}) …`(그대로 두면 fix가 "take struct{}"라 사용자를 지우고 로그인 필수가 조용히 사라진다, Opus). 차례가 틀림·둘은 오류, fix는 그 뷰의 이름으로 다시 정렬한 서명
   - **타입(Types)**: 사용자는 프로젝트 모듈 안에 선언된 struct `T`나 `*T`(제네릭·별칭의 다른 패키지 타입 아님). `*sql.DB`·`*sql.Tx`·`*sql.Conn`은 `fix: take db maago.DB`, `context.Context`·`*http.Request`·기본 타입 등은 "is not a user type"(가장 흔할 실수가 선택 사용자로 받아들여지지 않게, 두 리뷰). 프로젝트의 모든 뷰가 같은 `T`(`view X takes a.User, but view Y takes b.User` / `Config.Auth returns one user type; take it in every view`). DB는 `maago.DB`와 같은 타입
   - **라우트**: 필수 `U`의 `AppRoute`는 `LoginRequired: true`(지금의 303·401 길), `*U`는 아니다. 한 뷰가 `U`이고 라우트 옵션 `maago.LoginRequired`도 있으면 오류(`remove maago.LoginRequired`), `*U`와 함께면 오류. 사용자 매개변수가 있는 뷰의 라우트 중 일부에만 `maago.LoginRequired`가 있으면 오류 `split the view: the GET takes u *auth.User, the POST takes u auth.User`(게시판 `ShowPost`: 공개 GET과 로그인 POST, fix대로 옵션을 지우면 GET까지 로그인이 된다, Opus). 사용자를 받지 않는 뷰의 `maago.LoginRequired`는 아직 받는다(9c). `AppRoute.User`(사용자를 받는 라우트)를 더해 `Config.Auth`가 nil이면 `Handler`가 거절(`*U`만 쓰는 프로젝트가 조용히 늘 nil이지 않게)
   - **어댑터**: `u, _ := maago.UserFrom[T](r.Context())`, `*U`는 `var u *T; if v, ok := maago.UserFrom[T](r.Context()); ok { u = &v }`, DB는 `maago.DBFrom(r.Context())`. `T`가 다른 패키지면 `views_gen.go`가 `import maagouser "경로"`. `maagouser`는 늘 예약(이름 겹침 검사는 생성 전에 돈다). D-66 게이트: 생성 파일의 import는 지금의 목록이거나, 별칭이 `maagouser`이고 경로가 `reflect`·`text/template`·`unsafe`가 아닌 것(게이트 시험에 `maagouser "reflect"`). 점 없는 모듈 경로(`go mod init board`)도 받는다
   - **런타임**: `Config.Auth`는 값 타입을 돌려준다(`*T`를 돌려주면 `u *T`가 선택으로 읽혀 늘 어긋난다): `UserFrom`의 panic 글을 "a view's user parameter (u T or u *T) or UserFrom must be the type Config.Auth returns, here %T"로. `testdata/web`의 `Auth`는 `*Account` → `Account`. 문서(10번)에 같은 말
   - 확인: 생성기 골든(필수·선택·DB·다른 패키지·같은 패키지의 사용자), 오류 표(In 없음, 차례, 둘, `*sql.DB`, `context.Context`, 기본 타입, 뷰 사이의 다른 `T`, `LoginRequired`와 함께, 일부 라우트만 `LoginRequired`), 게이트 시험, `testdata/web`의 새 꼴 뷰와 HTTP(비로그인 HTML GET 303·POST 401·API 401, 선택은 nil), `Auth` 없는 `*U` 프로젝트의 `Handler` 오류, 걷기 초록
   - 하지 않는 것: `maago.Tx`(5번), 탈출구 뷰의 꼴, In 생략, 사용자 타입을 한 곳에 선언하기(`Config.Auth`가 `any`라 새 API가 필요하다)
   - 구현 리뷰: codex, Opus(짧은 튜토리얼 꼴의 프로젝트에서 새 꼴과 흔한 실수를 빌드한 maago로 실측: 303·401·nil 맞음). 받음: 사용자와 함께 받은 `*sql.DB`·`*maago.DB`의 fix가 DB를 지웠다 → 구문에서 DB로 알아보고 `takes the database as *sql.DB, not maago.DB`와 DB를 넣은 fix, 필수 `U` 뷰의 일부 라우트만 `LoginRequired`이면 "나누라"가 아니라 "지우라", 다른 패키지 타입을 마지막에 받으면 그것을 사용자로 바꾸던 fix를 되돌리고(`*http.Request`) 옛 진단에 "사용자라면 In이 그 뒤"를 더함, 별칭 사용자 타입(`types.Unalias`), 메시지의 타입을 패키지 이름으로(`auth.User`)와 포인터 그대로, fix에 실제 타입, 필수 사용자 API의 401 시험. 받지 않음: 같은 패키지의 모델 아닌 struct 사용자에 In을 빠뜨린 꼴(10번 문서), 5개 이상 매개변수의 shapes 글, 다른 모델을 사용자로 쓰는 것(런타임 panic, 한 곳 선언은 하지 않기로)
   - 계획 리뷰: codex, Opus. 받음: 사용자 판정을 프로젝트의 struct로 좁힘과 `*sql.DB`의 fix, In 없음의 진단, 일부 라우트만 `LoginRequired`, panic 글, `Config.Auth` 값 타입, `AppRoute.User`, 게이트를 별칭으로, 차례를 5↔6(트랜잭션 먼저: auth를 한 번만 고친다, `BeginTx`), 판독을 DB 종류 0~2개로. 9c로: `WithDB`·`WithUser`는 남는 라이브러리 코드(`Current`·`Can`)를 시험하는 데 필요하다(지우지 않거나 maagotest로), 탈출구 뷰의 로그인(문서와 예)
5. **요청 트랜잭션**(D-150): `maago.Tx`를 받는 뷰의 어댑터가 Begin·Commit·Rollback. 순서를 못 박는다: 뷰가 돌려준 뒤 오류면 롤백, panic이면 롤백하고 다시 panic(복구 미들웨어가 500), 렌더·리다이렉트는 커밋 뒤(커밋이 실패하면 500이고 응답은 쓰이지 않았다). SQLite는 `BEGIN IMMEDIATE`(읽고 쓰는 트랜잭션의 `SQLITE_BUSY` 피하기). `maago.AfterCommit(ctx, f)`: 커밋 뒤에만, 롤백이면 버림, 그 ctx는 요청이 끝나도 취소되지 않음, panic은 잡아 기록. `Tx`와 `DB`를 함께 받는 뷰 → 확인: panic 뒤 롤백, 오류 뒤 롤백, `AfterCommit`이 롤백 뒤에는 돌지 않음, 커밋 실패의 500, SQLite·PG

   계획 (리뷰와 사람의 결정 뒤)
   - **`Tx`와 `DB`는 함께 받지 않는다**(사람의 결정 2026-10-15, Q3를 고침): 서명은 `ctx, [u], [tx maago.Tx | db maago.DB], in`. 둘 다면 생성 오류. 이유(Opus 실측): SQLite는 IMMEDIATE가 쓰기 락을 먼저 잡아 같은 요청의 `db` 쓰기가 5초 뒤 BUSY, PG는 트랜잭션을 쥔 요청들이 풀의 연결을 서로 기다려 교착. 실패해도 남아야 하는 쓰기(auth의 시도 기록)는 오류 대신 `Show`와 nil로 돌려주면 커밋된다(6번)
   - **`maago.Tx`**: `type Tx struct{ tx *sql.Tx }`, `ExecContext`·`QueryContext`·`QueryRowContext`만(그래서 `maago.DB`이고 생성된 쿼리를 그대로 받는다). `Commit`·`Rollback`·`PrepareContext`는 없다. 손으로 만든 `maago.Tx{}`의 메서드는 "use maago.WithTx"라고 panic
   - **`maago.WithTx(ctx, db *sql.DB, f func(ctx context.Context, tx Tx) error) error`**: Begin, `f`, 오류면 롤백하고 그 오류(어떤 오류든: 422도, Q3. 롤백의 오류가 원래 오류를 덮지 않는다), panic이면 롤백하고 같은 값으로 다시 panic, 아니면 Commit. 커밋이 PG의 중단된 트랜잭션 때문에 실패하면(쿼리 오류를 잡아 폼 오류로 바꾸고 nil을 돌려준 꼴, Opus 실측) 그 원인을 말하는 오류(`a query failed earlier in this transaction, so PostgreSQL rolled it back; return that query's error`). `WithTx` 안의 `WithTx`는 panic(같은 요청이 자신을 교착시킨다). 요청 ctx가 취소되면(클라이언트가 끊음) `database/sql`이 롤백한다: 문서에 한 줄
   - **어댑터**: `Tx`를 받는 뷰는 `WithTx` 안에서 뷰를 부르고 **응답을 버퍼에 그린 뒤** 커밋한다(템플릿·JSON 오류면 롤백, 커밋된 데이터에 500이 붙지 않는다, codex). 커밋 뒤 버퍼를 쓴다. 리다이렉트는 커밋 뒤
   - **`maago.AfterCommit(ctx, f func(context.Context))`**: `WithTx`가 모아 커밋 직후, 응답을 쓰기 전에 **차례대로 동기로** 돈다(Django `on_commit`, Opus: 시험이 기다릴 손잡이가 필요 없고 8번의 기다림은 `Shutdown`이 진행 중 요청을 기다리는 것으로 끝난다). 롤백이면 버림. ctx는 `context.WithoutCancel`, panic은 잡아 스택과 함께 `log`에(런타임이 `Config.OnError`를 모른다), 응답은 그대로. `WithTx` 밖이면 바로 돈다(탈출구 뷰·`-createsuperuser`가 함께 쓰는 헬퍼를 위해, Opus)
   - **SQLite**: `OpenDB`의 DSN에 `_txlock=immediate`(읽고 쓰는 트랜잭션 둘이 BUSY 없이 차례로, Opus 실측: deferred는 하나가 51ms에 BUSY로 갱신을 잃었다). 읽기 전용 `TxOptions`에는 걸리지 않는다. practice DB·migrate는 따로 연다(그대로). 문서: Tx 뷰는 뷰 내내 쓰기 락을 쥔다, 무거운 일은 밖에서
   - **생성기**: 서명의 DB 종류 자리에 `tx maago.Tx`. GET·HEAD 라우트의 `Tx`는 오류(`a GET does not write; take db maago.DB`), `Tx`와 `DB`를 함께는 오류, `*maago.Tx`·`*sql.Tx`는 4번의 DB와 같은 fix. POST의 `DB`만은 아직 받는다(9c)
   - 확인(SQLite·PG): 커밋, 오류 뒤 롤백, panic 뒤 롤백과 같은 값의 panic, PG의 쿼리 오류 뒤 nil의 커밋 오류 글, 렌더 실패면 롤백, `AfterCommit`이 커밋 뒤에만·차례대로·롤백이면 안 돌고·panic이 응답을 망치지 않음·밖에서는 바로, 중첩 `WithTx`의 panic, 동시 쓰기 둘이 SQLite에서 BUSY 없이, 생성기 골든과 오류 표(GET의 Tx, Tx와 DB, `*sql.Tx`), `testdata/web`의 Tx 뷰가 HTTP로 커밋·롤백
   - 하지 않는 것: 격리 수준 옵션(PG는 서버 기본, 보통 READ COMMITTED: 문서), lazy Begin, 잃어버린 갱신(7번)
   - 계획 리뷰: codex, Opus. 받음: 위의 모두. 사람이 정함: Tx와 DB를 함께 받지 않는다
   - 구현 리뷰: codex, Opus(빌드한 maago로 auth 프로젝트에서 커밋·오류·422·템플릿 실패·panic·쿠키·`AfterCommit`의 메일·`WithTx`로 직접 부르는 시험을 두 방언에서 실측). 받음: 생성된 Insert와 `:one`은 `QueryRowContext`라 PG의 삼킨 오류를 놓쳤다 → `row.Err()`, 그 시험, 커밋 오류의 글에 "쓰기 전에 쿼리로 검사", `Tx` 문서에 쓰기 락·격리 수준·고루틴 하나. 받지 않음: `nil` 값의 `context.WithValue`가 panic한다(codex: 값은 nil이어도 된다, 시험이 지난다), `AfterCommit` panic의 `OnError`(런타임이 cfg를 모른다: `log`로, 계획을 고침). 넘김: SQLite와 PG의 갈라짐(같은 "중복이면 폼 오류로 Show"가 PG에서만 500)과 savepoint·unique→필드 오류는 v0.2의 결정(10번 문서에), `maago.Buffer`는 11번에서 하위 패키지로, 6번에서 Tx 뷰 안의 `DBFrom`이 남지 않게 확인, Tx+DB 오류의 위치
6. **뷰의 서명, 복사되는 auth와 admin**(D-149): auth의 9짝을 GET·POST로 나누고 새 꼴로, admin과 `Config.Auth`는 `DBFrom`·`UserFrom` 그대로(Q4). 권한 회귀를 막는 시험: 비로그인·다른 사용자·staff 아님이 각 auth 라우트에서 막히는지; auth의 `RequireStaff(ctx)`는 `RequireStaff(u User)`로(뷰가 받은 사용자를 넘긴다, 4번 리뷰), auth의 POST 뷰는 5번의 `maago.Tx`로 한 번에 옮긴다(`PasswordResetConfirm`의 `BeginTx`도) → 확인: `TestAdmin`·`TestStaff`·`TestAPIToken`, 그 권한 시험이 나누기 전과 뒤에 같은 결과, 걷기 초록

   계획 (사람의 결정 2026-10-15: POST의 `Tx` 규칙에 라우트마다 빠져나갈 길 `maago.NoTx`)
   - **왜**: auth의 비밀번호 폼은 요청 트랜잭션 안에서 틀린다. 시도 기록과 세기(`limit`)가 PG의 READ COMMITTED에서 동시 요청끼리 보이지 않아 병렬 추측이 한도를 지나고, SQLite에서는 비밀번호 해시(약 100ms) 내내 쓰기 락을 쥔다. 지금처럼 쓰기마다 바로 커밋해야 맞다
   - **`maago.NoTx`**(라우트 옵션, D-150 고침): POST·PUT·PATCH·DELETE 라우트의 뷰가 `db maago.DB`를 받고 트랜잭션 없이 돈다고 말한다. `NoTx`인데 뷰가 `Tx`를 받거나 DB를 받지 않으면 오류, GET·HEAD의 `NoTx`는 오류. 9c의 규칙은 "쓰는 라우트의 뷰가 `maago.DB`만 받으면 `NoTx`가 없을 때 생성 오류"가 된다(grep되는 한 낱말)
   - **`WithTx(ctx, db maago.DB, f)`**: `*sql.DB`가 아니라 `maago.DB`를 받아(속은 `BeginTx`가 있는 것, 곧 `*sql.DB`) `NoTx` 뷰가 짧은 쓰기 묶음을 직접 트랜잭션으로. `maago.Tx`를 넘기면 "inside a transaction" panic
   - **auth를 나누기**(9짝): GET 뷰는 그리기만(`Login(ctx, in LoginIn) (LoginPage, error)` 등, 로그인 필수 화면은 `u User`), POST 뷰는 `XSubmit`. 같은 페이지 타입이라 템플릿은 그대로
     - `NoTx`와 `db maago.DB`: `LoginSubmit`, `SignupSubmit`, `VerifySubmit`, `VerifyResendSubmit`, `PasswordChangeSubmit`(u), `PasswordResetSubmit`, `PasswordResetConfirmSubmit`, `APITokensSubmit`(u), `AccountCloseSubmit`(u): `limit`과 해시는 밖에서, 여러 행을 함께 바꾸는 곳만 `maago.WithTx(ctx, db, …)`(지금의 `BeginTx` 자리)
     - `tx maago.Tx`: `Logout(ctx, u *User, tx, in struct{})`, `APITokenDelete(ctx, u User, tx, in)`
     - `maago.LoginRequired`와 `UserFrom`·`DBFrom`이 뷰에서 사라진다. `Current`·`Can`(라이브러리, `Config.Auth`·admin이 부른다)은 그대로(Q4). `RequireStaff(ctx)` → `RequireStaff(u User) error`(뷰가 받은 사용자를, 4번 리뷰)
   - **권한 회귀 시험**: auth의 라우트 표 전부를 걸으며 비로그인 GET(로그인 화면이면 200, 로그인 필수면 303)·POST(401 또는 그 폼의 422/200)와 남의 토큰 지우기(404)가 나누기 전과 같은지. 나누기 전에 먼저 써서 지금의 auth로 통과시킨다
   - **WithTx로 옮길 때의 꼴**(리뷰): `WithTx`는 nil이면 커밋한다. 지금의 `defer tx.Rollback()` 뒤 `return Show(page), nil`(토큰을 쓰지 않고 다시 그림: 리셋의 규칙 거절·비활성, Verify의 `n == 0`)은 클로저가 패키지의 표지 오류 `errRefused`를 돌려 롤백하고 바깥에서 `errors.Is`면 `Show(page)`. 클로저는 트랜잭션을 `db`라는 이름으로 받아 바깥 `db`를 가린다(`func(ctx context.Context, db maago.Tx) error`: Tx와 DB를 함께 쓰는 교착이 클로저 안에서 열리지 않게). `limit`은 늘 클로저 밖. `WithTx`에 `BeginTx`가 없는 DB를 주면 panic 글 "pass the db a NoTx view takes"
   - **NoTx의 오류 더**: 한 뷰가 쓰는 라우트 둘을 섬기고 한쪽만 `NoTx`(`loginOptions`처럼 뷰 단위), 탈출구 뷰의 `NoTx`. 9c의 fix는 `take tx maago.Tx`만(NoTx는 이유와 함께 문서에: fix에 나란히 두면 반사가 된다, Opus)
   - **권한 시험**: `TestAccess`(작업 전에 써서 지금의 auth로 통과시켰다)에 Bearer 토큰으로 온 요청의 줄과, 표의 줄 수가 `urls.go`의 `maago.GET(`·`maago.POST(` 수와 같은지
   - **함께 바꿀 것**: `cmd/maago/add_test.go`의 `Staff` 뷰(`u auth.User`, 라우트의 `LoginRequired` 빼기), auth.go의 낡는 문서 둘(`Current`, `RequireStaff`). `urls.go`의 머리 줄 수는 그대로(게시판 0020의 `Maago-Expect: auth/urls.go:5:5`)
   - 확인: 복사본의 시험(`auth_test.go`, 두 방언)과 위의 권한 시험, `TestAdmin`·`TestStaff`·`TestAPIToken`, 생성기 오류 표(`NoTx`의 셋), `grep -n "UserFrom\|DBFrom\|LoginRequired" auth/views.go`가 0, 걷기 초록(게시판은 `maago add auth`의 복사본을 쓴다)
   - 하지 않는 것: 9c의 규칙 켜기, 게시판의 뷰(9번), admin의 서명(라이브러리가 만든 `AppRoute`), Verify의 `n == 0` 경쟁 시험(검사와 tx 사이의 비밀번호 바뀜)
   - 구현 리뷰: codex(옛·새 views.go를 줄마다 대조: 보안 회귀 없음), Opus(빌드한 maago로 두 방언에서 가입→인증→로그인→비밀번호 변경→토큰→닫기·리셋·약한 비밀번호 뒤 같은 토큰을 실측, 병렬 틀린 로그인 30개 중 정확히 10개 통과). 받음: auth의 `limit`을 Tx 뷰가 부르면 컴파일되고 한도가 뚫린다(PG 25/30) → `limit`이 `maago.Tx`를 받으면 panic과 시험, `WithTx`의 두 panic(Tx, `BeginTx` 없는 DB)의 시험, 리셋 확인이 토큰을 읽기 전에 해시하던 것(이전부터: 아무 토큰으로 요청마다 해시) → `TokenUser`로 먼저 읽기만. 9c로: `cmd/maago/add_test.go`의 `Me`(옛 꼴)
   - 계획 리뷰: codex, Opus. 받음: 위의 셋(롤백의 뜻, 클로저의 `db`, NoTx 오류와 fix), 권한 시험 늘리기, 함께 바꿀 것. 받지 않음: NoTx 대신 `db *sql.DB`(Opus의 제안: 사람이 NoTx로 정했다)
7. **잃어버린 갱신**(D-150, D-21·D-111·D-118을 고침): 생성된 Update의 전제 조건(`auto=update`가 있는 모델은 `WHERE id=… AND updated_at=…`, 0행이면 `ErrConflict`, 409), 고친 열만 쓰는 갱신, admin의 409(D-132의 "나중 저장이 이긴다"를 고침) → 확인: 동시 갱신 시험(둘째가 409), 고친 열만 바뀜, SQLite·PG

   계획 (리뷰와 사람의 결정 뒤)
   - **`maago.ErrConflict`**: `status`가 409로. 글은 "the row changed since it was read; read it again and apply the change, or change one column with a query that sets the version too". API면 RFC 9457, HTML이면 평문 409. 뷰가 `errors.Is(err, maago.ErrConflict)`면 폼 오류로 `Show`할 수 있다(문서 10번에 그 꼴)
   - **생성된 `UpdateX`**(D-111 고침): 모델의 `auto=update` 필드(여럿이면 첫째, 이하 "버전")를 전제 조건으로: `WHERE id = ? AND <버전> = ?`. 값은 SQLite `maago.SQLiteTime(o.V)`(저장된 글과 같은 꼴), PG `o.V`(µs로 자른 것). 영값의 버전은 그대로 비교되어 409(ID만 채운 구조체가 행을 영값으로 덮어쓰던 것을 막는다). 0행이면 같은 `db`로 `SELECT 1 … WHERE id`: 없으면 `sql.ErrNoRows`, 있으면 `maago.ErrConflict`(트랜잭션 밖의 호출에서는 둘 사이의 삭제와 경쟁: 문서). 성공하면 지금처럼 `o.V = now`. 문서 주석에 "o is what GetX returned; o.V is the version you read". 버전 없는 모델은 그대로
   - **고친 열만 쓰는 갱신은 생성하지 않는다**(사람의 결정 2026-10-15, D-150을 고침): 손으로 쓴 `:exec`가 그 길. 대신 **생성기 검사**: 버전 필드가 있는 모델의 표를 `UPDATE`하는 쿼리는 그 열을 `time.Time` 매개변수로 SET해야 한다. 없으면 생성 오류 `query SetStatus updates board_post without its version updated_at, so an UpdatePost that read before it would undo it` / `fix: add , updated_at = :now with -- param: now time.Time`. SQL의 시각 함수(`CURRENT_TIMESTAMP`, `now()`, `datetime(…)`)로 SET하면 오류(SQLite에서 글꼴이 달라 그 행이 늘 409가 된다)
   - **요청을 건너는 폼**(두 탭): 폼이 버전을 숨은 정수로 싣는다(`UnixMicro`, `time.UnixMicro(v)`로 되돌리면 두 방언 모두 같다고 비교된다). 7번은 꼴을 정하고 시험한다(gentest의 왕복), 게시판 `EditPost`는 9번, 문서는 10번
   - **admin**(D-132 고침): 고치기 폼이 숨은 `_version`(같은 `UnixMicro`)을 싣고 `UPDATE … WHERE id = ? AND <버전> = ?`에 제출된 값을. 버전 있는 모델에서 `_version`이 없거나 둘이거나 정수가 아니면 400. 0행이면 존재를 갈라 409로 폼을 다시 그리며 "This row changed since you opened it; reload to see the change"(고칠 칸의 입력은 남기고 읽기 전용 칸은 DB의 지금 값). `admin.Updated`가 `ErrConflict`를 404로 바꾸지 않는다. 버전 없는 모델은 그대로
   - **기존 호출 감사**: gentest의 `UpdatedAt`을 영값으로 덮는 시험을 "읽은 값 그대로"로, auth·게시판의 `UpdateX` 호출이 모두 읽은 뒤인지, 손 `UPDATE`가 버전 표를 건드리는지(auth의 `auth_user`는 버전이 없다), makemigrations가 SQLite에서 버전 열을 채우는 안내의 글꼴
   - 확인: 생성기 골든(`UpdateOrder`, admin_gen), 오류 표(버전 없는 손 UPDATE, 시각 함수), gentest 두 방언: 같은 행을 두 번 읽어 고치면 둘째가 `ErrConflict`, 없는 행 `ErrNoRows`, Insert가 돌려준 값으로 바로 Update, ID만 채운 구조체 409, `UnixMicro` 왕복, PG에서 손 `:exec`(버전 SET)와 겹친 `UpdateX`가 409. admin의 HTTP(연 뒤 다른 쪽이 고치면 409와 글, `_version` 없음 400), 걷기 초록
   - 하지 않는 것: `GetXForUpdate`(PG의 `FOR UPDATE`, 요청 안의 409를 없앤다: v0.2 후보), 버전 필드 둘 이상의 오류
   - 계획 리뷰: codex, Opus(실측: SQLite·PG에서 Insert·Get의 값 비교 1행, `CURRENT_TIMESTAMP`로 채운 행 0행, 버전을 안 바꾸는 손 `:exec`와 겹친 통째 쓰기가 잃어버린 갱신을 남김). 받음: 위의 모두. 사람이 정함: 손 UPDATE와 생성기 검사
   - 구현: `model.(*Model).Version()`(첫 `auto=update`, `UpdateX`·검사·admin이 함께), `mapper.Versions`(`build`의 이른 단계, DB 전에), admin의 `admin.Version`(GET은 읽은 값, POST는 `_version`, 없으면 400)·`admin.Changed`(0행이면 `SELECT 1`로 `ErrConflict`와 `ErrNoRows`를 가름)·`Form.Version`, 사이트는 `ErrConflict`면 같은 폼을 409로, SQLite `makemigrations`는 새 버전 열에 `strftime('%Y-%m-%d %H:%M:%f', 'now') || '000'`의 안내. 픽스처 `orders`에 손 `SetOrderStatus`(버전 SET), 게시판 0025의 admin 시험이 `_version`을 보내고 먼저 연 폼의 409를 본다(글에도). 감사에서: `TestAccess`의 틀린 토큰이 1/64로 진짜와 같던 것
   - 구현 리뷰: codex, Opus(두 방언에서 admin 두 탭의 HTTP 실측: 303 뒤 409와 입력 남김, 같은 버전 8개 동시 저장에 정확히 하나만 303, `UnixMicro` 왕복 200회에 거짓 충돌 0, 게시판 끝 단계 PG 초록). 받음: 검사가 `{cond}` 줄의 버전 SET과 선택 매개변수를 받던 것 → 조건 줄이면 오류, `SET {`의 `{`를 조건으로 읽어 영역을 삼키던 것(흔한 부분 갱신 꼴이 거짓 오류) → 조건은 한 줄 안의 `{…}`만, `INSERT … ON CONFLICT DO UPDATE`가 검사 밖이던 것 → `INTO`의 표로, `WITH`의 `UPDATE` 뒤 `)`, `(a, b) = (…)`는 "`updated_at = :now`로 쓰라"로, 409 페이지의 `WriteHeader` 한 번. 남긴 것: 손 `INSERT`가 SQLite에서 버전을 `CURRENT_TIMESTAMP`로 채우면 그 행은 늘 409(시드·마이그레이션 SQL은 gen이 보지 않는다, 문서 10번에 채우는 꼴)
8. **운영의 최소선**(D-153): `maago.Serve(ctx, addr, h)`(서버 타임아웃, SIGINT·SIGTERM에 graceful shutdown, 진행 중 요청 기다리기(`AfterCommit`은 5번에서 요청 안에서 동기로 돈다), 기다림에 상한), `maago.Migrate(ctx, db, fsys)`(바이너리에 실린 마이그레이션, CLI의 `migrate`와 같은 기록표·검사: 적용된 파일의 체크섬이 다르면 오류, 모르는 기록이 있으면 오류) → 확인: 시험이 신호를 보내 진행 중인 요청과 커밋 뒤의 일이 끝나는지, 바이너리의 `-migrate`가 CLI와 같은 기록표, 두 검사의 오류
9. **게시판 튜토리얼 다시 쓰기와 옛 꼴 지우기**(D-148, D-103을 고침). 단계가 커서 셋으로 나눈다. 각 커밋에서 걷기가 초록
   - 9a. 0001~0016을 새 서명·`Tx`로, 메시지는 영어(Q1), GET·POST 나누기
   - 9b. 0017~0033을 같은 꼴로, 0033은 `maago.Serve`와 바이너리의 마이그레이션. MAP.md의 번호
   - 9c. 옛 꼴 지우기(`cmd/maago/add_test.go`의 `Me`도): `maago.LoginRequired`·`WithDB`, 타입 있는 뷰의 `UserFrom`·`DBFrom`은 생성 오류(Q4), 쓰는 라우트의 `DB`만 받는 뷰는 `maago.NoTx`가 없으면 생성 오류(Q3, 6번에서 고침). `TestBoardTutorialCatches`의 줄을 새 꼴로(권한 빼기 줄: 서명에서 사용자 빼기가 생성 오류인지 시험 실패인지 줄마다 적는다), TUTORIAL-FUTURE와 `TestShortTutorial` → 확인: `TestBoardTutorial`(PG), Catches, `TestShortTutorial`, `TestBoardMap`
10. **`maago doc`**(D-152, D-06·D-07·D-62를 고침): 검사기의 닫힌 목록(모델 태그, 폼 태그, In 태그, 뷰의 서명, 쿼리 머리, 조건 줄 규칙, 라우트 옵션, 템플릿 함수, admin URL·필드)을 하나의 표로 모으고 검사기와 `maago doc <주제>`가 함께 읽는다. 주제마다 영어로 짧은 설명과 완전한 예 하나. 진단의 `(D-xx)`를 `see: maago doc <주제>`로 → 확인: 표에 없는 키를 검사기가 받으면 시험이 실패(표가 원천), 주제의 예를 시험이 빈 프로젝트에서 `gen`·`check`, 진단에 `(D-`가 0

   앞 작업에서 넘어온 것: 스칼라 결과(`:one bool`)와 집계(`max`·`sum`)의 NULL은 COALESCE(3번), TUTORIAL-FUTURE에 "`go get -u` 뒤 CLI도 `go install …@latest`"(1번)
11. **`go doc`과 생성 코드 전용 export**(D-152): 루트의 패키지 문서를 10번의 표로 생성(사용자 API의 지도와 주제마다 한 단락), 생성 코드만 쓰는 export를 하위 패키지로 → 확인: 생성 코드가 하위 패키지만 부르는지(골든), `go doc maago`에 생성 전용 이름이 없음, 패키지 문서의 예가 `Example` 시험, `go doc -all`·생성된 프로젝트·골든·사용자 문서에 `(D-`가 0
12. **첫 릴리스**(M9 2번, D-146·D-151): 사람이 저장소를 공개하고 `v0.1.0`을 태그. 확인 셋(`git ls-remote`, `GOPROXY=direct`, 기본 프록시), 빈 `GOPATH`·`GOWORK=off`에서 `go install github.com/maago-web/maago/cmd/maago@v0.1.0`, 게시판의 `go.mod`가 v0.1.0, TODO "첫 릴리스 전" 체크. 공개 뒤 고칠 것이 나오면 태그를 옮기지 않고 v0.1.1 → 확인: 그 설치와 `maago help`
13. **마지막 게이트**(M9 11번): `go install`부터 짧은 튜토리얼과 게시판(메시지만)을 서로 모르는 실행 둘씩, M9과 같은 프롬프트에서 `replace` 줄을 뺀 것. 게시판은 0001~0012에 더해 0020~0022(PG)와 0032~0033(초안·배포)까지 → 확인: 기록, `TestTutorialProject`, 게이트 기록의 함정이 다시 나오지 않음
14. **v0 끝**(M9 8번): MILESTONES의 M10 체크, README, 다음은 v0.2(D-148의 뒤로 미룬 것)와 v1의 순서

## 초안 리뷰 (2026-10-15, codex와 Opus가 fable 대신)

- **순서**: 문서 표가 최종 API 앞에 있었다 → 10·11번으로. 서명·트랜잭션 작업이 걷기를 빨갛게 둔다 → 새 꼴을 옛 꼴과 함께 받고 9번에서 지운다
- **빠진 의존**: 탈출구 뷰·admin·`Config.Auth`가 `DBFrom`·`UserFrom`에 기댄다 → Q4. 사용자 타입을 어떻게 아는지는 설계 결정 → 서명에서 추론(D-149 고침)
- **계약을 못 박지 않았다**: 트랜잭션과 panic의 순서, 커밋 실패, `AfterCommit`의 ctx와 shutdown, SQLite의 `BEGIN IMMEDIATE`, 실린 마이그레이션의 검사 → 6·8번에 적음
- **권한 회귀**: auth를 나누는 동안 막던 것이 풀릴 수 있다 → 5번의 권한 시험
- **크기**: 6·7·9번이 컸다 → 서명 둘(4·5), 트랜잭션 둘(6·7), 게시판 셋(9a~c)
- **사실**: 뷰 16(탈출구 2), auth 짝 9, `go doc -all`의 D-번호 83, 프록시 경로는 `file://` GOPROXY로 이미 연습 → 위에 고침. D-번호 검사 범위를 넓힘(11번). 릴리스에 v0.1.1의 길(12번). 게이트 범위를 PG·배포 단계까지(13번)
- **작업마다 고칠 결정**을 각 작업에 적음

---

## M10 끝

- [ ] 1~14 완료
- [ ] M10 완료 조건을 실제로 돌려 확인, 조건마다 시험 이름
- [ ] MILESTONES.md의 M10 체크박스
- [ ] push 뒤 GitHub Actions
