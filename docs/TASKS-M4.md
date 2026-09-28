# M4 작업 목록 — HTTP

[MILESTONES.md](MILESTONES.md)의 M4를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M4 완료 조건** (MILESTONES)
- 게시판 튜토리얼(M8)에서 이 마일스톤이 받는 커밋이 모두 통과한다 → 12번(SQLite. PG로 바꾸는 단계는 M8)
- 템플릿의 `{{.Post.Titel}}`과 없는 URL 이름이 생성 오류다 → 5번(검사기), 13번(파이프라인: 한 번에, 아무것도 쓰지 않음)
- 잘못된 path 값과 `sql.ErrNoRows`는 404, 허용 출처가 아닌 교차 출처 POST는 403이고 허용 출처는 통과한다 → 3번(런타임), 4번(생성된 어댑터)
- 로그인 필수로 선언한 뷰에 로그인하지 않고 들어가면 HTML은 로그인 화면으로, API는 401(RFC 9457) → 9번
- 빈 숫자 칸, 범위 밖 숫자, 해석 실패가 폼에서 서로 다른 결과를 낸다 → 6번

**M3에서 넘어온 것** ([TASKS-M3.md](TASKS-M3.md) 끝, [TODO.md](TODO.md)의 M3 절)
- 앱의 `go test`는 패키지 디렉터리에서 돌아 루트의 `.env`를 보지 못한다. 게시판 튜토리얼은 절대 경로 환경 변수로 돈다 → D-113, 10번
- 타입 정보를 `go/ast`만으로 얻을지(ARCHITECTURE A-3) → D-117, 1번
- 빌드 제약이 붙은 `.go` 파일을 매퍼가 보통 파일처럼 읽는다 → TODO에 그대로(1번 계획: 매퍼의 이름 검사는 타입 검사보다 앞 단계이고 테스트 파일까지 본다)
- 릴리스 전 `replace`·`go.work`, fix 예시 이름은 TODO에 그대로 둔다(M4와 상관없다)

---

## 먼저 확인한 사실 (2026-09-28, 작은 실험, 리뷰가 소스로 다시 봄)

- **`go/types`는 stdlib만으로 앱 패키지를 검사할 수 있다. 속도가 문제다.** `importer.ForCompiler(fset, "source", nil)`로 `net/http`·`html/template`·`database/sql`을 import하는 패키지 하나를 검사하면 약 1초(stdlib를 소스에서 다시 읽는다). M3의 `gentest/sqlite/blog`(runtime `maago`만)는 약 0.4초. D-65(SQLite 1초)를 앱 하나로 다 쓴다
- **export data는 빠르다.** `importer.ForCompiler(fset, "gc", nil)`는 약 0.25초(처음 한 번은 약 1초). 이 importer는 `go list -export`로 `go` 바이너리를 불러 빌드 캐시의 export 파일을 찾는다(Go 1.26 `go/importer`·`internal/exportdata` 소스). `lookup`이 nil이면 모듈을 몰라 `github.com/maago-web/maago`와 사용자의 다른 앱을 찾지 못한다(실험). `lookup`에 `go list -deps -export -json`의 `Export` 경로를 먹이면 모듈 밖 패키지(stdlib, runtime `maago`, 제3자)를 모두 export data로 읽는다(fable 22, 1번이 잰다)
- **프로젝트 안의 패키지는 소스로, 생성물은 메모리의 것으로.** 소스 importer로 읽은 앱 간 import는 디스크의 낡은 `*_gen.go`를 봤다(실험). 생성기가 같은 실행에서 만든 것을 써야 한다. 표준 importer는 이것을 못 하므로 프로젝트 패키지를 순서대로 검사해 `types.Package`를 돌려주는 작은 importer를 직접 쓴다(codex 2)
- `x/tools/go/packages`는 쓰지 않아도 된다(D-09·D-90이 루트 `go.mod`의 `require`를 막는다). 대가: `maago gen`이 `go` 바이너리와 풀린 모듈 그래프를 요구한다(캐시 없는 오프라인이면 D-64처럼 이유 있는 실패). cgo 파일과 vendor는 `go list`의 파일 선택(`GoFiles`·`CgoFiles`)을 따르고, 뷰가 cgo 타입을 쓰면 "지원하지 않음" 오류
- **`http.CrossOriginProtection`**(Go 1.25): GET·HEAD·OPTIONS는 늘 통과. 그 밖은 `Sec-Fetch-Site`, 없으면 `Origin`과 `Host`를 비교해 교차 출처면 403. 신뢰 출처(`AddTrustedOrigin`, 정확히 같아야 한다)는 통과. 두 헤더가 모두 없는 요청(비브라우저, `httptest`)도 통과 → 시험에서 CSRF 우회가 따로 필요 없고, 상태를 바꾸는 GET은 막지 못한다(생성기가 GET 라우트의 폼 POST를 막는 것은 아니다. 문서에 적는다). CORS 응답 헤더는 만들지 않는다
- **`text/template/parse`**는 노드마다 바이트 위치(`Pos`)를 준다. `ErrorContext`의 칸은 0 기준이라 쓰지 않고 `Pos`에서 1 기준으로 계산한다(fable 15)

## 결정 (2026-09-28, 초안 리뷰 뒤 사람의 결정)

초안의 제안 P1~P6과 codex·fable 리뷰를 보고 사람이 골랐다. P2는 추천(생성된 `init()`이 등록)이 아니라 명시적 등록을 골라 D-46을 고쳤다. 나머지는 추천대로.

- **D-113 서비스 시험(U-07의 M4 몫)**: 시험 전용 패키지 `github.com/maago-web/maago/maagotest`(stdlib `net/http/httptest`처럼). 런타임 공개 API는 루트 패키지 하나라는 원칙(ARCHITECTURE §2)의 유일한 예외다. 루트에 두면 사용자 바이너리가 `testing`을 끌고 간다. `maagotest.DB(t) *sql.DB`: 주소는 환경 변수 `MAAGO_DATABASE_URL`, 없으면 작업 디렉터리에서 `go.mod`까지 올라간 루트의 `.env`. 주소에서는 방언과 서버만 쓴다. SQLite는 `t.TempDir()`의 새 파일(D-101 `mode=rw`라 먼저 만든다), PG는 개발 DB에 결정적인 이름의 스키마(패키지 경로 + `t.Name()`의 해시, `DROP SCHEMA IF EXISTS … CASCADE; CREATE SCHEMA`로 시작하고 `t.Cleanup`에서 지운다. 남아도 다음 실행이 지운다. 시간으로 추측하지 않는다). 풀의 모든 연결은 주소의 `search_path`로 그 스키마를 본다. 둘 다 `migrations/`를 번호 순으로 적용한 뒤 준다. HTTP는 `httptest` 그대로, 데이터 준비는 생성된 `InsertX`. 메일 확인은 M5(U-07의 나머지). 기각: 트랜잭션 롤백(Django `TestCase`, 앱의 트랜잭션·풀과 부딪힌다), 별도 시험 DB 주소, 앱마다 도우미 생성, 루트 패키지의 `TestDB`
- **D-114 앱을 잇는 법과 뷰의 DB**: 등록은 명시적이다. 앱마다 생성된 `urls_gen.go`가 앱 값(모양은 2번 계획, 예: `board.App`)을 내보내고 `main`이 모은다: `maago.Handler(maago.Config{DB: db, Apps: …})`. contrib(auth·admin)도 같다. D-46의 "앱 등록은 blank import + `init()`"을 고친다. 빠뜨린 앱이 조용히 없는 페이지가 되지 않게 생성기가 잡는 법(예: 라우트가 있는 앱마다 `main`이 그 앱 값을 쓰는지)은 2번 계획에서 정한다. 뷰는 `maago.DBFrom(ctx) *sql.DB`(트랜잭션을 열 수 있게)로 DB를 얻고, 없으면 `maago.WithDB`를 가리키는 panic. `WithDB`는 공개(뷰를 HTTP 없이 시험). `Config`는 한 struct(DB, 앱, 출처, 인증, 오류 보고 훅, 표시 타임존, 하위 경로), 환경 변수로 채우는 것은 `main`. `Handler`의 오류는 `Config`의 잘못된 값뿐이고 라우트 이름·패턴 충돌은 생성기가 본다(contrib 라우트 포함). 사람이 추천(`init()`)이 아니라 이것을 골랐다: 무엇이 들어왔는지 `main`에 다 보인다(PHILOSOPHY 5). 대가: `main`이 앱 목록을 들고 있어야 한다
- **D-115 라우트 표**: `var Routes = maago.Routes{maago.GET("/post/{id}", "post_detail", PostDetail), …}`를 `go/ast`로 읽는다(리터럴만). 경로는 전체 경로(`include`식 접두어 없음), 이름은 프로젝트 전체에서 하나이고 `^[a-z][a-z0-9_]*$`(`app:name` 이름공간 없음). 이름 겹침과 패턴 충돌(메서드 포함, 실제 `ServeMux` 규칙)은 생성 오류. URL 함수는 `PostDetailURL(id int64) string`: 인자는 경로 와일드카드 순서, 타입은 뷰 In의 path 필드 타입, 값은 `url.PathEscape`. 한 라우트의 선언(메서드, API, 로그인 필수)은 표의 한 줄에
- **D-116 뷰의 반환, 템플릿, 상태**: 뷰는 `func(ctx, In) (P, error)`(늘 그림) 또는 `func(ctx, In) (maago.Result[P], error)`(`maago.Show(p)` 또는 `maago.SeeOther(url)` 303). 그 밖의 시그니처는 생성 오류, `func(w, r)`은 탈출구(D-41). 템플릿은 페이지 타입에 붙는다: `PostFormPage` → 앱의 `templates/post_form_page.html`(폼의 GET·POST가 같은 P면 같은 템플릿). 레이아웃은 앱마다 `templates/base.html`(`//go:embed`는 `..`를 못 쓴다, 공유는 v1 오버라이드와 함께). 상태: path 해석 실패와 `sql.ErrNoRows`는 404, query·JSON 본문 해석 실패는 400, 폼 검증 실패는 **200**으로 다시 그림(Django와 같고 HTMX가 설정 없이 바꿔 끼운다, D-45), API의 검증 실패는 422 problem, 그 밖은 500과 오류 보고 훅. In에 없는 query·폼 키는 무시한다(웹의 관례, 의도된 예외). 기각: `Result[P]`만(읽기 뷰까지 감싼다), 이동을 error 값으로(D-71), 라우트 이름에 붙인 템플릿(POST 줄이 GET의 템플릿을 가리킬 옵션이 따로 필요), 폼 실패 422(HTMX 설정이 필요)
- **D-117 타입 정보(ARCHITECTURE A-3의 답)**: `go/types`와 직접 짠 importer. 모듈 밖 패키지(stdlib, runtime `maago`, 제3자)는 `go list -deps -export -json`의 export data, 프로젝트 안은 `go list`가 고른 파일의 소스와 메모리의 생성물. stdlib만, `x/tools` 없음(D-09·D-90). 대가: `maago gen`에 `go` 바이너리와 풀린 모듈 그래프가 필요하고(없으면 D-64처럼 이유 있는 실패), 사용자 코드의 타입 오류가 생성 오류가 된다. D-65(SQLite 1초)를 1번이 재고 넘으면 멈추고 묻는다. 기각: `x/tools/go/packages`(루트 모듈에 `require`, 그 코드를 `cmd/maago`로 옮겨야 한다)
- **D-118 응답 뒤 작업**: 프레임워크는 도구를 주지 않는다. D-72 "비동기 태스크 범위 밖"은 큐·워커를 뜻하고, 응답 뒤 작업은 앱이 `go f(context.WithoutCancel(ctx))`로 쓴다고 문서에 적는다(게시판 튜토리얼의 웹훅이 그렇게 쓴다). 대가: 서버가 꺼질 때 도는 작업을 기다리지 않고, panic은 앱이 받는다
- **마일스톤 옮김**: 게시판의 "작성자만 고치고 지우기"는 M5로(작성자는 `fk=auth.User`가 필요하다, D-78). MILESTONES M8 표를 고쳤다. M4는 9번의 인증 인터페이스를 가짜 인증으로 시험만 한다

---

## 모양 (작업들이 함께 따르는 것)

```
 읽기·해석(ast): 모델, 쿼리, Routes, 폼 struct, 템플릿 파일 ─▶ 검사(DB 없이) ─▶ DB 검사(M2·M3)
   ─▶ 메모리 생성물 1: models_gen·queries_gen(M3), URL 함수·forms_gen(뷰가 부르는 것)
   ─▶ go/types(프로젝트 패키지, 메모리 생성물 포함, 1번)
   ─▶ 검사: 뷰 시그니처·In(4), 템플릿(5), 폼 검증 메서드(6)
   ─▶ 메모리 생성물 2: 어댑터와 앱 값(views_gen, urls_gen, D-114) ─▶ 쓰기 / 비교
 런타임 maago: Handler·Config·Apps·WithDB·DBFrom·Result·폼 값·RFC 9457·CSRF/CORS(3번)   maagotest(10번)
```

- **순서**(fable 3·codex 18): 사용자 `views.go`는 `PostDetailURL`과 생성된 폼 타입을 부르므로 그것들은 타입 검사 **전에** 메모리에 있어야 하고, 어댑터는 타입 검사 **뒤에** 만든다. 앞 단계가 실패하면 뒤는 돌지 않는다(연쇄 오류, D-61)
- **패키지**: 생성기는 `internal/web`, 타입 정보는 `internal/gotypes`
- **어댑터**: 요청 → In(path·query·폼) → 뷰 → P → 템플릿 실행(`html/template`, D-42) 또는 JSON. 해석은 생성 코드가 필드마다 `strconv`로(reflect 없음, D-66). 생성물 import 허용 목록(D-66)을 넓힌다: `embed`·`errors`·`net/http`·`net/url`·`strconv`·`html/template`·`encoding/json`
- **템플릿 싣기**: 생성 코드가 `//go:embed templates`로 싣고 시작할 때 한 번 `html/template`로 파싱한다. 검사도 같은 `html/template` 파싱과 같은 함수 표를 거친 뒤 그 `parse` 트리를 걷는다(codex 14: 검사와 실행이 같은 파서를 본다). 개발 중 다시 읽기는 없다(`maago gen` 뒤 다시 빌드). 오버라이드는 v1(D-42)
- **CSRF와 CORS는 한 목록**: `Config.Origins []string`. 이 목록의 출처는 "우리 앱처럼 믿는 출처"다: `AddTrustedOrigin`으로 CSRF 검사를 통과하고, CORS 응답(`Access-Control-Allow-Origin`은 그 출처 그대로, `*`는 받지 않는다)을 받는다. 메서드·헤더·자격 증명은 따로 필드(기본: 라우트의 메서드, `Content-Type`, 자격 증명 켬). 둘로 나누면 MILESTONES가 말한 "허용한 출처의 POST가 403"이 생긴다. preflight(OPTIONS)는 `ServeMux`보다 앞에서 답한다(메서드 패턴은 405를 낸다). admin은 스스로 건다(D-47, M6)
- **오류 보고는 훅 하나**: `Config.OnError(ctx, r, err)`가 뷰의 500 오류와 recover한 panic을 함께 받는다(미들웨어는 error 값을 볼 수 없다, fable 10). `http.ErrAbortHandler`는 다시 panic
- **타임존**: 저장은 UTC(D-105)·`TIMESTAMPTZ`. 표시는 `Config.TimeZone`(기본 UTC: `time.Local`은 서버 환경에 숨은 동작, PHILOSOPHY 5)과 템플릿 함수 하나, 폼의 시각 입력은 그 타임존으로 해석
- **최종 사용자 문구**(폼 오류, 404·403 화면): 기본 영어, 프로젝트가 문구 표를 통째로 바꿀 수 있게(한 언어, i18n은 D-72 밖). 모양은 6번

---

## 1. 타입 정보 (`internal/gotypes`, D-117)

- 프로젝트 패키지를 `go/types`로 검사한다. 모듈 밖은 `go list -deps -export -json`의 export data, 프로젝트 안은 소스 + 메모리 생성물을 의존 순서대로. 파일 선택은 `go list`의 것(빌드 제약·cgo), `_test.go`는 뺀다
- 사용자 코드의 타입 오류는 위치 있는 진단(fix: `go build`로 보라)

→ 확인: 표 테스트(앱 간 import, 메모리 생성물이 디스크의 낡은 것을 이김, `//go:build ignore` 파일을 거름, 타입 오류의 위치, `go` 바이너리가 없을 때의 오류). 시간: `net/http`·`html/template`을 import하는 앱 셋의 합성 픽스처에서 SQLite `check` 1초 안(D-65)

### 계획

잰 것(2026-09-28, 합성 프로젝트): `go list -e -json=… ./...` 약 60ms, `go list -deps -export -json=… <바깥 import들>` 약 90~180ms(캐시가 찬 뒤). `importer.ForCompiler(fset, "gc", lookup)`에 `lookup`이 `Export` 파일을 열게 하면 된다(실험: importer 하나로 `net/http`·`html/template`·`maago`를 import하는 패키지 셋을 차례로 검사해 약 4ms). 디스크의 `*_gen.go`가 문법 오류여도 `go list`는 `Error`를 내지 않고 `GoFiles`에 그대로 싣는다(실험).

파일
- 새 `internal/gotypes/gotypes.go`: `Check(root string, overlay map[string][]byte) (*Result, []diag.Diagnostic)`. `Result`는 디렉터리(루트 기준 슬래시 경로) → `*types.Package`와 그 `FileSet`. `types.Info`는 채우지 않는다: 4·5번이 쓰는 것(뷰 함수·페이지 타입 찾기, 필드·메서드)은 `Package.Scope()`와 `types` API로 된다. 필요해지면 그 작업이 더한다
- 새 `internal/gotypes/gotypes_test.go`: 임시 모듈(`go.mod`에 `require github.com/maago-web/maago v0.0.0` + 이 저장소로 `replace`. 실험에서 `go.sum` 없이 된다)에 파일을 깔고 `Check`. `go list -export`가 의존을 `GOCACHE`에 컴파일하므로 codex 구현 리뷰에는 쓰기 가능한 `GOCACHE`가 필요하다(fable 14)

모양
1. `go list -e -json=Dir,ImportPath,GoFiles,CgoFiles,IgnoredGoFiles,Module ./...`(루트에서, 환경은 물려받는다). `go`를 찾지 못하면(`exec.ErrNotFound`) 위치 없는 오류 하나(`maago needs the go command to type-check the project`, fix: Go를 설치하거나 `PATH`에). 종료 코드가 0이 아니면(`go.mod` 문법, 풀리지 않은 모듈 등) stderr 첫 줄을 싣은 위치 없는 오류(fix: `go mod tidy`, fable 2). 패키지의 `Error`는 쓰지 않는다: 파일 선택(`GoFiles`)은 오류가 있어도 채워지고, 실제 문제(문법·타입·순환)는 아래가 위치와 함께 낸다(codex 1·2). 이 목록이 곧 "프로젝트 안의 패키지"다. `Module.GoVersion`은 `types.Config.GoVersion`에(`go build`와 같은 언어 판, codex 7. 처음 계획의 "두지 않음"을 고침)
2. 패키지마다 파일 = `GoFiles`·`CgoFiles` 중 디스크의 maago 생성물(첫 줄 표시, `pipeline.IsGenerated`)이 아닌 것 + 그 디렉터리의 `overlay` 파일. `overlay`는 maago 생성물만 담는다(빌드 제약·cgo·테스트 파일이 아닌 `.go`, 불변식으로 적는다, codex 3). 디스크 생성물은 `overlay`에 있으면 그것으로 바뀌고, 없으면 `gen`이 지울 파일이다(D-60). 표시 없는 `_gen.go`는 사용자 파일이다(D-67). `project.GoFiles`는 쓰지 않는다(테스트·제약 파일이 섞인다, codex 9). `overlay`의 디렉터리가 목록에 없으면 maago의 버그로 오류(조용히 건너뛰지 않는다, fable 2). 파일 이름은 루트 기준 슬래시 경로로 파싱한다(D-88)
3. cgo: `types.Config.FakeImportC`로 검사한다. `C.x`는 오류 없이 알 수 없는 타입이 되고, 뷰의 In·P가 그런 타입을 쓰면 4번이 거부한다(fable 3. 처음 계획은 cgo 패키지를 통째로 거부했다). 확인에서 cgo 파일이 오류 없이 통과하는 것을 본다
4. import는 파싱한 파일에서 모은다(`overlay`의 import가 디스크에 없다). 프로젝트 밖 경로를 모아 `go list -e -deps -export -json=ImportPath,Export,Error …` 한 번. `Error`가 있거나 `Export`가 빈 패키지는 그것을 import한 첫 자리에 `go list`의 메시지로 오류(fable 2·codex 5). fix는 경로가 모듈 경로 아래면 "no such package in this module", 아니면 `go get 경로`(fable 13)
5. importer 하나(검사는 차례로, 동시에 하지 않는다, codex 6): 프로젝트 패키지는 먼저 검사한 결과를(의존 순서로, 재귀), 그 밖은 `importer.ForCompiler(fset, "gc", lookup)`. 프로젝트 안의 import 순환은 그 import 자리에 오류
6. 타입 오류는 모두 모은다(D-61, 서로 독립인 패키지의 오류도 모두). 오류가 난 프로젝트 패키지를 import하는 패키지는 검사하지 않는다(연쇄 오류)
7. 오류 위치가 `overlay` 파일이면: 그 패키지에 `IgnoredGoFiles`(빌드 제약으로 빠진 파일)가 있으면 사용자 오류다. 모델·쿼리 읽기는 빌드 제약을 보지 않으므로 거기 선언된 것을 생성물이 쓰지만 Go는 보지 못한다(fable 1, M3 TODO와 같은 뿌리). fix: `generated code cannot see what files with a build constraint declare (x.go); move the model out of them`. 빠진 파일이 없으면 maago의 버그(`dialect.Bug`의 문구)
8. 사용자 코드의 오류 fix: `go build reports the same error; fix it there` (작업의 문장대로, fable 11)

결정에 없는 선택 (★)
- ★ **파이프라인에 잇는 것은 4번**(첫 소비자인 뷰 시그니처 검사와 함께), 그리고 라우트가 있는 프로젝트만 검사한다. 지금 이으면 M1~M3만 쓰는 프로젝트도 `go` 바이너리와 풀린 모듈 그래프를 요구하고(`require` 없는 `go.mod`의 시험 픽스처 여럿이 깨진다), 쓰는 곳 없는 검사가 `gen`마다 0.1~0.3초를 쓴다. 4번이 이을 때 D-117에 "라우트가 있는 프로젝트만"을 적는다(fable 4, codex 8). 라우트가 없는 프로젝트의 생성 코드가 컴파일되는지는 지금처럼 골든과 gentest가 본다
- ★ 매퍼의 빌드 제약(TODO의 M3 항목)은 고치지 않는다: 매퍼의 이름 검사는 이 단계보다 앞이고 테스트 파일까지 본다. `go list`를 앞 단계로 옮기면 모든 프로젝트에 `go`가 필요해진다. 대신 모양 7이 그 결과(생성물이 못 보는 선언)를 사용자 오류로 낸다
- 받지 않음: codex 4 vendor의 `ImportMap` — 모듈 모드의 vendor는 `ImportPath`를 바꾸지 않는다(GOPATH의 vendor만 바꾼다). `go list`를 루트에서 부르므로 `-mod=vendor`는 `go`가 스스로 따른다. fable 12 — `go list`의 순환 `Error`를 쓰지 않으므로(모양 1) 순환은 모양 5가 위치와 함께 낸다

하지 않기로 한 것
- 테스트 파일(`_test.go`) 검사, `types.Info`, 파이프라인 연결(4번), 매퍼 빌드 제약

단계
1. `go list` 두 번과 파싱·파일 고르기 → 확인: 디스크의 낡은 생성물(표시 있음)이 `Stale`을, `overlay`가 `Fresh`를 선언할 때 사용자 코드의 `Fresh`는 통과·`Stale`은 `undefined`(fable 9). `overlay`에 없는 디스크 생성물도 빠짐, 표시 없는 `x_gen.go`는 남음, `//go:build ignore` 파일(깨진 코드)이 빠짐
2. importer와 검사 → 확인: 앱 `b`가 앱 `a`의 타입과 runtime `maago`·`net/http`·`html/template`을 쓰고 통과, `b`의 `Scope`에서 `a`의 타입이 든 struct를 찾음, 앱 간 import 순환은 그 import 자리 오류, cgo 파일(`import "C"`, `C.int`)이 오류 없이 통과
3. 진단 → 확인: 한 파일의 타입 오류 둘이 `a/x.go:줄:칸`으로 모두, 그 `a`의 깨진 이름을 **쓰는** `b`에서 추가 오류 없음(fable 7), 서로 독립인 `a`·`c`의 오류가 둘 다(fable 8), 없는 외부 패키지와 모듈 안의 없는 패키지 import는 그 자리 오류와 다른 fix, 없는 모듈을 require한 `go.mod`에서 `go list` 실패 오류, `PATH=""`에서 `go` 없음 문구(fable 10), `overlay` 파일의 오류는 버그 문구, `//go:build ignore` 파일이 선언한 이름을 `overlay`가 쓰면 사용자 오류 문구(fable 1). `GoVersion`: `go.mod`가 `go 1.21`인 모듈에서 `for range 10`(1.22)이 오류
4. 시간 → 확인: 합성 픽스처(앱 셋, 각각 `net/http`·`html/template`·`database/sql`·`maago`, 한 앱에 gentest blog의 모델과 쿼리)에서 **같은 픽스처**의 `build.Check`(SQLite) + `gotypes.Check`(캐시가 찬 뒤) < 1초(fable 4). 넘으면 멈추고 묻는다. 값은 이 계획에 적는다
5. 두 모듈 `go vet`·`go test`, `gofmt -l .`

잰 값(2026-09-29, 캐시가 찬 뒤): 게시판 튜토리얼 마지막 단계(모델·쿼리) `maago check` 0.01초 + 같은 프로젝트의 `gotypes.Check` 0.106초. `net/http`·`html/template`·`database/sql`·`maago`를 import하는 앱 셋(`TestTime`)의 `Check` 0.106초. 합쳐도 D-65(1초)에 한참 안이다. 계획의 "한 픽스처에 앱 셋과 모델·쿼리"는 두 픽스처로 나눠 쟀다(검사 시간은 바깥 import의 `go list`가 거의 다라 합친 값이 달라지지 않는다). 변이: 실패한 의존을 건너뛰지 않게, 생성물 오류를 늘 사용자 오류로 바꾸면 각각 시험이 실패한다

계획 리뷰 (codex·fable)
- 받음: codex 1·2(패키지 `Error`를 쓰지 않음), 3(`overlay` 불변식), 5(`Export` 빈 것), 6(importer 하나·차례로), 7(`GoVersion`), 8(D-117에 적음), 9(`go list`의 파일만). fable 1(제약 붙은 파일의 모델), 2(`go list` 실패·`Export` 없음·목록에 없는 `overlay` 디렉터리), 3(`FakeImportC`), 4(같은 픽스처로 시간), 7~11, 13, 14
- 받지 않음: codex 4(위 ★ 절), fable 6(`GoVersion`은 codex 7을 받았다: 한 줄이고 `go build`와 같아진다), fable 12(위 ★ 절)

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `exportData`의 두 맵과 "go list did not report it" 갈래를 `cmp.Or`로
- codex(샌드박스가 `/tmp` 쓰기를 막아 코드만 읽은 리뷰): 받음 — `go list` 실패의 stderr는 마지막 줄(첫 줄이 `go: downloading …`일 수 있다). 받지 않음 — 1 외부 import 진단의 순서: `diag.Write`가 위치로 정렬한다
- fable(변이로 잼): 받음 — 여러 줄 타입 오류(`have …\n\twant …`)가 `diag.New`에서 panic하던 것을 한 줄로(1), 한 패키지의 문법 오류가 다른 패키지의 타입 오류를 가리던 것: 문법 오류는 그 패키지만 실패시킨다(2), 생성물의 문법 오류는 버그 문구(3), 생성물 자리의 표시 없는 사용자 파일은 `pipeline.Check`와 같은 문구로 그 자리에서(4, 거짓 `undefined`가 먼저 나오던 것), 의존의 의존이 깨진 것은 `DepsErrors`의 이유와 다른 fix(5), 살아남은 변이 여섯(cgo 파일, 사용자 파일 자리, `_test.go` 거르기, `unsafe`, `DepsErrors`, 첫 자리)에 시험, `overlay` 디렉터리 버그 진단에 파일 위치(11), `root`는 절대 경로라는 주석(14). 고친 뒤 여덟 변이를 다시 돌려 모두 잡힌다. 13(`TestTime`은 로그만)은 그대로: 시간 단언은 흔들린다

## 2. 라우트 표 (`internal/web`, D-115)

- `Routes` 리터럴 읽기(메서드·경로·이름·뷰·옵션). 경로 패턴은 `ServeMux` 문법 그대로
- 앱 값(D-114)의 모양과, 라우트가 있는 앱을 `main`이 빠뜨렸을 때 생성 때 잡는 법
- 검사: 이름 꼴·겹침(프로젝트와 contrib), 패턴 충돌(메서드 포함), 리터럴이 아닌 표, 없는 뷰 함수

→ 확인: 표 테스트. 다른 앱끼리 겹친 이름, 충돌하는 두 패턴과 충돌하지 않는 `GET /x`·`POST /x`, 모르는 메서드, 없는 뷰(did you mean), 변수로 만든 표, `main`이 빠뜨린 앱 각각 위치와 fix로

### 계획

파일
- 새 루트 `route.go`(런타임, `package maago`): `type Routes []Route`, `type Route struct{}`, `type RouteOption int`와 `const LoginRequired, API`, `func GET/POST/PUT/PATCH/DELETE(path, name string, view any, opts ...RouteOption) Route`(인자를 버린다). 런타임은 이 값을 쓰지 않는다: 표는 `maago gen`이 `go/ast`로 읽는 선언이고, 요청을 받는 것은 4번이 생성하는 코드다. 사용자 코드가 컴파일되려면 이름만 있으면 된다. 필드는 쓰는 곳이 생기면 더한다(fable 10). `view any`: 뷰의 시그니처가 여럿(D-116, `func(w, r)`)이라 Go 타입으로 묶을 수 없다. 문서 주석에 "`var Routes = maago.Routes{…}` 리터럴 안에서만 뜻이 있고 `maago gen`이 검사한다, 표 밖의 호출은 아무 일도 하지 않는다"고 적는다(codex 6)
- 새 `internal/web/routes.go`: `Read(fset, root, files, modulePath) ([]*App, []diag.Diagnostic)`는 표의 **모양**만(fable 1), `Check(apps) []diag.Diagnostic`는 **뜻**을 모두 본다. 그래서 겹친 이름과 없는 뷰가 한 단계에서 한 번에 나온다(D-61). `App{Dir, Name, ImportPath, Pos, Routes []Route, Funcs(최상위 함수 이름), Used(다른 패키지가 App을 쓴 곳이 있음)}`, `Route{Method, Path, Name, View string; Pos, PathPos, NamePos, ViewPos token.Pos; Options []Option}`(모르는 메서드·옵션도 글 그대로 담아 `Check`에 넘긴다)
- 새 `internal/web/routes_test.go`: 글 → `diag.Write` 출력 전체의 표(골든 `testdata/routes.golden`)와 정상 표의 `App` 값 비교
- `internal/project`: `ModulePath(root)`(`go.mod`의 `module` 줄, 따옴표면 `strconv.Unquote`. `golang.org/x/mod`는 쓰지 않는다)
- `internal/build/build.go`: 해석 단계에 `web.Read`, DB 없는 검사 단계에 `web.Check`. 라우트 오류는 DB 검사 전에 멈춘다(ARCHITECTURE §3). 파일은 `model.Read`처럼 다시 파싱한다(세 번째 파싱. 4번이 `gotypes`를 이을 때 합칠지 본다, fable 5)

규칙 (결정에 없는 선택은 ★)
- **읽는 파일**: 테스트 아닌 `.go`, 첫 줄이 생성 표시인 파일은 뺀다(4번의 `urls_gen.go`가 `App`을 선언한다, fable 6)
- **모양(`Read`)**: 패키지 최상위의 `var Routes = maago.Routes{…}` 하나. `maago`는 그 파일이 maago를 import한 이름(`model.Read`와 같은 규칙), 점 import는 오류. 오류: `maago.Routes{` 리터럴이 다른 자리(다른 이름의 변수, 함수 안, 타입 붙은 `var Routes maago.Routes = …`), 우변이 리터럴이 아님(`var Routes = other`, `= build()`), 한 패키지의 `var Routes` 둘, 원소가 `maago.X(…)` 호출이 아님, 경로·이름이 문자열 리터럴이 아님, 뷰가 식별자가 아님(`other.View`·메서드 값·함수 리터럴·`nil`), 옵션이 `maago.X`가 아님, `package main`의 표
- **뜻(`Check`)**: 모르는 메서드·옵션(did you mean), 같은 옵션 두 번, 없는 뷰(같은 패키지 최상위 함수 중 did you mean), 빈 표(`maago.Routes{}`, fix: 표를 지우거나 라우트를 더한다, PHILOSOPHY 7), 이름 꼴·겹침, 경로, 충돌, 앱 값의 쓰임
- **옵션**(열린 질문의 답 ★): 가변 인자 `maago.LoginRequired`·`maago.API`, 닫힌 목록. `Login`은 "로그인 화면 라우트"로 읽혀 `LoginRequired`로(fable 7). 다른 생성자(`maago.APIGET`…)는 메서드 수 × 옵션 수로 는다. 뜻은 7·9번이 채운다. ARCHITECTURE §2의 `maago.GET/POST/API`는 13번에서 고친다
- **메서드** ★: `GET`·`POST`·`PUT`·`PATCH`·`DELETE`. `HEAD`는 `ServeMux`가 GET으로 받고, `OPTIONS`는 3번이 `ServeMux` 앞에서 답한다
- **탈출구**(D-41): 같은 생성자로 올린다. `func(w, r)` 뷰인지는 4번이 시그니처로 가른다(`maago.Handle`을 따로 두지 않는다)
- **이름** ★: `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`(`post__detail`과 `post_detail`이 같은 `PostDetailURL`이 되지 않게). D-115의 `^[a-z][a-z0-9_]*$`보다 좁다 → D-115를 이 커밋에서 고친다(fable 8). 프로젝트 전체에서 하나: 둘째부터 오류, 메시지에 첫 자리. 폼의 GET·POST는 이름이 둘이다
- **경로**: `/`로 시작(호스트 패턴은 오류). **`{$}` 없이 `/`로 끝나는 패턴은 오류**(사람의 결정 2026-09-29, D-115 고침): `ServeMux`에서 `GET /`는 다른 패턴에 걸리지 않은 모든 경로를 받아 모르는 URL이 404가 되지 않는다(fable 2). fix: `/{$}`는 그 경로만. 하위 트리가 필요해지면(M7 정적 파일) 그때 명시적인 표기를 더한다. 문법은 라우트마다 새 `http.ServeMux`에 `"METHOD 경로"`를 등록해 panic 여부로 가린다. panic 글은 공개 계약이 아니므로 판정에 쓰지 않고, 한 줄이고 `parsing `으로 시작할 때만 그 뒤를 도움말로 싣는다(codex 1). 충돌은 문법이 맞은 라우트끼리 짝으로 새 mux에 등록해 둘째가 panic하면 충돌이다(글을 해석하지 않는다). 메시지는 maago가 짠다: `route conflicts with post_detail (blog/urls.go:6): a request could match both`(`ServeMux`의 여러 줄 글과 생성기 소스 위치가 새지 않는다, fable 9). ponytail: 짝마다 mux라 O(n²), 라우트 수백 개면 수 ms라 둔다
- 받지 않음: codex 2 `GODEBUG=httpmuxgo121=1` — 사용자가 일부러 옛 `ServeMux`를 고른 것이고, `go.mod`의 `go` 줄은 maago(1.25, D-04)를 require하면 1.22보다 낮을 수 없다(`go mod tidy`가 올린다)
- **앱 값**(D-114, 열린 질문의 답 ★): 4번이 앱의 `urls_gen.go`에 `var App maago.App`을 생성한다(그 패키지의 사용자 이름과 겹치면 4번의 이름 검사). 라우트가 있는 앱은 다른 패키지의 테스트 아닌 파일이 그 앱을 import한 이름으로 `X.App`을 써야 한다(별칭 import도 셈). 없으면 `Routes` 자리에 오류(`app board has routes, but no package uses board.App`, fix: `add board.App to maago.Config.Apps in main (D-114), then run maago gen: go build works after gen`, 첫 `gen` 전에는 `App`이 없어 빌드가 깨지는 순서를 말한다, fable 4). `main` 말고 도우미 패키지가 모아도 된다. 시험은 셈하지 않는다. 앱 패키지를 점 import하면 오류(명시적 등록, codex 3). **한계**(fable 4·codex 3·4): `go/ast`만 보므로 "잊음"은 잡고 "잘못 씀"(`_ = board.App`, 지역 변수가 import 이름을 가림, 빌드 제약 파일)은 못 잡는다. 빌드 제약은 M3 TODO와 같은 뿌리. 4번이 타입 정보를 이을 때 다시 본다
- ★ `package main`의 라우트는 오류(fix: 앱 패키지로 옮기고 `main`이 `board.App`을 쓴다). 앱은 import할 수 있는 패키지라는 제약이고, 13번에서 D-114에 적는다(codex 5)
- **contrib**: contrib(auth·admin)의 라우트는 M5·M6에 생긴다. 생기면 같은 `Read`로 maago 소스의 표를 읽어 이름·충돌 검사에 더한다. 지금은 없다

하지 않기로 한 것
- 뷰 시그니처, 경로 와일드카드와 In 필드의 짝, `App`·URL 함수 생성과 이름 충돌(4번), 옵션의 뜻(7·9번)

단계
1. `route.go`, `project.ModulePath` → 확인: `ModulePath` 표(따옴표, 주석, `module` 줄 없음은 오류)
2. `web.Read` → 확인: 정상 표 두 앱의 `App` 값(메서드·경로·이름·뷰·옵션·위치), 디스크의 낡은 `urls_gen.go`(표시 있음, 깨진 코드)가 있어도 같음. 모양 오류 표: 변수 이름이 다른 표, 함수 안의 표, 타입 붙은 `var`, 리터럴 아닌 우변 둘, `var Routes` 둘, 원소가 호출이 아님, 경로·이름이 리터럴 아님, 뷰가 `other.View`·함수 리터럴·`nil`, 옵션이 식별자 아님, 점 import, `package main`의 표
3. `web.Check` → 확인: 모르는 메서드·옵션(did you mean), 옵션 두 번, 없는 뷰(did you mean), 빈 표, 이름 꼴(`Post`, `post__x`, `x_`), 다른 앱끼리 겹친 이름(첫 자리가 메시지에), 문법이 틀린 경로(`/x/{id`), `/`로 시작하지 않는 경로, `/`로 끝나는 `GET /`·`GET /posts/`(fix `/{$}`)와 통과하는 `GET /{$}`, 충돌하는 `GET /a/{x}`·`GET /{y}/b`와 같은 `GET /x` 두 줄(메시지에 `registered at`·maago 소스 경로가 없음), 충돌하지 않는 `GET /x`·`POST /x`·`GET /posts/{id}`·`GET /posts/new`. 앱 값: 아무도 안 씀·시험만 씀·다른 앱 `other.App`만 씀은 오류, 별칭 import `b.App`·도우미 패키지·`package main`이 쓰면 통과, 앱을 점 import하면 오류
4. 파이프라인 → 확인: `internal/build` 시험 하나: 예제 프로젝트에 겹친 라우트 이름과 없는 뷰를 넣으면 두 오류가 한 번에 나오고(D-61, 둘 다 `Check`) 아무것도 쓰지 않음(D-60)
5. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: codex 1(panic 글을 판정에 쓰지 않음), 3(점 import 오류, 한계를 적음), 4(빌드 제약 한계), 5(`package main` 금지를 D-114에), 6(`view any` 주석). fable 1(모양은 `Read`·뜻은 `Check`, 단계 4가 닿게), 2(끝 `/`, 사람이 막기로), 3(리터럴 아닌 우변, `var` 둘, 빈 표, 같은 경로 두 줄), 4(한계와 fix의 순서, 별칭·`other.App`·`main` 확인), 5(세 번째 파싱은 적어 둠), 6(생성물 건너뛰기), 7(`LoginRequired`, §2는 13번), 8(D-115 고침), 9(메시지를 maago가 짬), 10(`Route struct{}`)
- 받지 않음: codex 2(위 경로 규칙)

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `pkgFiles` struct를 디렉터리 → 파일 맵으로, `methods`·`options`를 비공개로
- codex(샌드박스가 `/tmp`를 막아 코드만 읽은 리뷰): 받음 — `go.mod`의 `module (` 블록을 경로 `"("`로 읽던 것(3). 받지 않음 — 1 maago를 import한 파일의 다른 타입 `var Routes`를 표가 아니라고 보는 것: 한 번 넣었다가 fable 3을 따라 뺐다. 같은 자리의 `= other`는 거부하면서 `= []string{…}`만 짐작하게 된다(PHILOSOPHY 6), 이름을 바꾸는 값은 작다. 2 지역 변수가 import 이름을 가리는 것: 계획의 한계 절대로(4번이 타입 정보로 다시 본다)
- fable(변이 34개): 받음 — D-115 고침이 빠져 있던 것(1), 모르는 메서드면 경로를 건너뛰던 줄: `ServeMux`는 어떤 메서드든 받아 보호가 없고, 틀린 패턴이 충돌 검사에 들어가 거짓 충돌을 냈다(2, 시험 더함), 다른 타입 리터럴 갈래 삭제(3), `importPath`를 `path.Join` 한 줄로(4), 낡은 생성물 픽스처를 컴파일되는 코드로(5, 깨진 코드는 파싱 오류가 대신 걸렀다), 틀린 패턴 뒤 정상 라우트(6), 메서드는 뷰가 아님(7). 받지 않음 — 7의 파이프라인 모양 오류 시험(`Read` 오류는 `TestErrors`가 본다), `maagoNames` 합치기(4번에서 `gotypes`와 함께)

## 3. 런타임: Handler, Config, 오류와 상태, CSRF·CORS (`maago`)

- 앱 값의 타입(생성 코드가 채운다), `Handler(Config) (http.Handler, error)`, `WithDB`·`DBFrom`, `Result`·`Show`·`SeeOther`, RFC 9457 응답, 오류 → 상태(D-116), `OnError`, CSRF·CORS(모양 절)

→ 확인: `httptest`로 `Sec-Fetch-Site: cross-site` POST 403, 헤더 없이 `Origin` 불일치 POST 403, 신뢰 출처 POST 통과(정확히 같을 때만: 포트가 다르면 403), 헤더 없는 POST 통과, 신뢰 출처의 OPTIONS preflight에 `Access-Control-Allow-*`·비신뢰 출처에는 없음, `sql.ErrNoRows`를 감싼 오류 404, panic이 500과 `OnError`로, `http.ErrAbortHandler`는 다시 panic

## 4. 뷰 시그니처와 어댑터 생성 (`urls_gen.go`, `views_gen.go`)

- 뷰 시그니처(D-116의 두 모양), In의 태그(`path=`·`query=`, 닫힌 목록 D-17), 필드 타입(D-77 기본 타입과 포인터), 경로 와일드카드와 path 필드의 짝
- 생성: URL 함수(타입 검사 전), 어댑터·등록(뒤)

→ 확인: 골든(D-67), 잘못된 시그니처·태그·타입·짝 없는 와일드카드 각각 오류, 생성된 핸들러로 `/post/abc` 404·없는 글 404·`?page=abc` 400·있는 글 200

## 5. 템플릿 검사와 렌더 (D-42, D-45)

- 페이지 타입마다 템플릿을 찾고(없으면 오류), `parse` 트리를 걸으며 `.필드` 사슬을 P 타입에 맞춘다: `range`·`with`·변수(`$x`)의 점 바뀜, 인자 없는 메서드, 슬라이스·맵 원소. 타입을 정할 수 없는 식(`index`·`printf`·파이프로 넘긴 값 등)은 검사기가 타입을 정하거나 "cannot check" 오류다. 조용히 통과하는 갈래는 없다(PHILOSOPHY 7)
- `{{url "이름" 인자…}}`: 이름이 있고 인자 수·타입이 URL 함수와 맞음. 모르는 함수는 오류(닫힌 목록)
- 부분 응답(D-45)의 블록 이름. 요청이 고르는지(HTMX 헤더) 뷰가 고르는지는 5번 계획에서

→ 확인: `{{.Post.Titel}}` → `…:줄:칸: PostDetailPage.Post has no field "Titel"` + did you mean(칸 값을 박는다, 1 기준), `post_detial` → did you mean. `range` 안의 점, `$` 변수, 인자 수가 틀린 `url`, 없는 블록, "cannot check" 한 경우 각각. 렌더 골든

## 6. 폼 (D-43, D-15)

- 모델에서 만드는 폼과 모델 없는 struct 폼. 필드마다 (값, 있음, 오류): 빈 칸, 해석 실패, 범위·길이·선택지 밖을 가른다. 태그는 모델과 같은 목록
- 폼 전체 검증과 사용자 검증 메서드(D-15), DB를 보는 검증(ctx와 DB를 받는 메서드)
- unique 위반을 폼 오류로: 런타임은 드라이버를 모르므로(D-90) 방언의 오류 글에서 D-86의 제약 이름을 찾는다. "DB를 보는 검증 메서드"와 장치가 둘이 되지 않게 게시판 이름 중복은 하나만 쓴다(fable 9, 계획에서 고른다)
- 템플릿에서 폼과 오류를 그리는 법, 최종 사용자 문구와 바꾸는 법, 검증 실패는 200으로 다시 그림(D-116)

→ 확인: `int64` 칸에 빈 값·`abc`·범위 밖의 세 결과가 **서로 모두 다름**(값·문구를 쌍으로 비교), 필수 문자열의 빈 값, 선택지 밖, 사용자 메서드의 오류, 게시판 이름 중복이 두 방언에서 그 필드의 오류, 다시 그린 폼이 입력값을 유지

## 7. API 라우트 (D-44)

- JSON 응답, 요청 본문 JSON을 In으로(`encoding/json`, D-66 범위 밖), 오류는 RFC 9457(폼 오류는 `errors` 확장 멤버)

→ 확인: 골든, 잘못된 JSON 400 problem, 없는 글 404 problem, 검증 실패 422 problem

## 8. 표준 핸들러 탈출구와 스트리밍 (D-41)

- `func(w, r)` 뷰도 같은 생성자로 표에 올린다(2번. 이름·URL 함수는 같다, 템플릿 검사 없음). 스트리밍 응답(CSV)이 되는지

→ 확인: 탈출구 라우트의 URL 함수와 `{{url}}`. 스트리밍은 `httptest.NewServer`로 열고, 핸들러가 둘째 조각을 쓰기 전에 클라이언트가 첫 조각을 읽는다(채널로 맞춘다. `ResponseRecorder`로는 보이지 않는다)

## 9. 현재 사용자와 인증 인터페이스

- `Config.Auth`(인터페이스 하나: 요청 → 사용자 또는 없음), 뷰가 `maago.UserFrom(ctx)`로 얻음, 라우트 표에서 로그인 필수 선언(D-115), 실패 응답: HTML은 `Config`의 로그인 URL로 `next`와 함께 303, API는 401 problem. 세션·토큰은 M5. M5에 넘길 것: `next`를 쓸 때 같은 출처의 경로인지 검증(열린 이동 막기)

→ 확인: 가짜 인증으로 로그인 필수 HTML 뷰 → 로그인 URL로 303(`next` 포함), API → 401 problem, 로그인한 요청은 통과

## 10. 서비스 시험 도구 (`maagotest`, D-113)

- `maagotest.DB(t)`, 두 방언(D-113). 자체 시험은 드라이버가 필요해 `cmd/maago`에(D-90)
- 게시판 튜토리얼 2~7단계의 시험(절대 경로 `MAAGO_DATABASE_URL` 우회)을 `maagotest`로 다시 쓴다: `steps.sh`로 풀어 rebase, `examples/board/README.md`, `TestBoardTutorialCatches`, D-103의 문구(13번에서)

→ 확인: 패키지 디렉터리에서 돌린 시험이 루트의 `.env`를 찾음, 시험마다 새 DB(한 시험의 쓰기가 다른 시험에 안 보임), PG 풀의 연결 여럿이 모두 그 스키마를 봄, 끝나면 스키마가 없음, 남은 스키마를 다음 실행이 지움, 마이그레이션이 적용됨

## 11. 작은 할일

- 하위 경로(`Config.Prefix`: URL 함수·`{{url}}`·`SeeOther`·로그인 이동에 붙는다), 표시 타임존과 템플릿 시각 함수, 오류 보고 훅의 예시, 응답 뒤 작업의 문서(D-118)

→ 확인: `/app` 아래에서 `{{url}}`·`SeeOther`·로그인 이동이 접두어를 가짐, 시각 표시가 `Config.TimeZone`을 따름, `OnError`가 뷰 오류와 panic을 받음

## 12. 게시판 튜토리얼의 M4 단계 (D-103)

M8 대응표에서 M4가 받는 것. 각 단계는 앱의 `maagotest` 시험과 함께. 상태가 걸린 단계(상태 전이)는 4번의 열린 질문을 기다린다.
- 게시판·글 목록(페이지)·상세 뷰와 템플릿, 일부러 틀리기(`{{.Post.Titel}}`, `Maago-Expect`)
- 글·게시판 쓰기 폼, 게시판 이름 중복(DB를 보는 검증), 공지 고정 기간 겹침·과거 시각 예약 게시 막기(폼 검증)
- 글 상태 전이(규칙 밖은 오류), 댓글, 댓글 달린 글은 숨기기
- API 라우트와 다른 출처의 호출(CORS), CSV 내보내기 스트리밍, 새 글 웹훅 알림(토큰, 실패와 재시도, 가짜 서버), 오류 보고, `/app` 하위 경로
- M5로 간 것: 작성자만 고치고 지우기(결정 절), 로그인 필수 화면

→ 확인: `TestBoardTutorial`이 새 단계까지 통과(SQLite)

## 13. 완료 조건 종단 테스트와 문서

- `TestM4Done`(M3의 `TestM3Done`처럼 파이프라인 수준): 템플릿 오류 둘이 한 번에 나오고(D-61) 아무것도 쓰지 않음(D-60), 나머지 조건은 두 방언에서. 튜토리얼로 D-65 시간을 다시 잰다
- 계획에서 정한 것을 DECISIONS로, TUTORIAL-FUTURE §5·7·8·9의 가정 목록을 고침, ARCHITECTURE A-3·§2·§5·§6

→ 확인: 완료 조건마다 시험 이름, CI에서 PG가 돈 실행

---

## M4 끝

- [ ] 1~13 완료
- [ ] 위 M4 완료 조건 다섯 개를 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로)
- [ ] MILESTONES.md의 M4 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M5 초안(U-06, `fk=` 다른 앱이 선행 결정)

## 열린 질문 (계획 단계에서 정한다)

- 앱 값의 모양과 빠뜨린 앱 검사(D-114) → 2번
- 라우트 옵션의 모양(`maago.Login` 옵션인지 다른 생성자인지) → 2번
- 부분 응답을 요청이 고르는지 뷰가 고르는지 → 5번
- In 필드에 슬라이스(`?tag=a&tag=b`)를 받는지 → 4번
- 뷰가 404·400·500 밖의 상태(403·409)를 내는 법 → 4번
- 게시판 이름 중복을 unique 위반으로 잡을지 검증 메서드로 잡을지 → 6번

## 초안 리뷰 (2026-09-28)

- codex: 받음 — `gc` importer는 `go list -export`로 `go` 바이너리를 부른다(1), 두 갈래 importer는 직접 짜야 한다(2), 파일 선택은 `go list`를 따르고 cgo는 지원하지 않음 오류(3), CSRF 확인 경우들(4), CORS의 메서드·헤더·자격 증명은 따로 필드, 한 목록의 보안 뜻을 적음(5), maagotest 주소 규칙과 PG `search_path`의 연결마다 확인(6), U-07을 M4·M5로 나눔(7), `maagotest`는 원칙의 예외로 명시(9), 충돌은 실제 `ServeMux` 규칙·URL 함수 인자와 이스케이프(10), 폼 흐름(12, 템플릿을 페이지 타입에), query·본문 400(13), 검사도 `html/template` 파싱을 거침·`embed`의 `..`(14), 응답 뒤 작업의 (b) 선택지(15), 작성자 검사를 사람에게(16, M5로), 확인 보강·정적 파일 URL은 M7(17), 파이프라인 순서(18). 8 명시적 등록은 초안이 받지 않았고(D-46) 사람이 골랐다(D-114). 받지 않음 — 11 `app:name`(D-115의 기각 이유)
- fable: 받음 — `embed` import(1), 레이아웃 자리(2), 생성물 두 번(3), POST의 템플릿(4), 폼 실패 상태를 사람에게(5), 해석 실패의 상태와 모르는 키(6), `WithDB`·`*sql.DB`·panic(7), contrib 라우트도 생성 때(8), unique 위반의 대가와 장치 하나(9), 오류 보고 훅 하나·`ErrAbortHandler`(10), 두 시그니처의 이유와 기각(11), 조건 1은 SQLite(12), `TestM4Done`(13), "cannot check"(14), 칸 1 기준(15), 1번 시간은 합성 픽스처·목록 둘(16), 스트리밍 시험(17), preflight 문구(18), 셋을 쌍으로(19), 결정적 스키마 이름·주소 규칙 셋·`mode=rw`·시험 자리(20), 10번의 숨은 일(21), `lookup`으로 export data(22), 웹훅 실패·재시도(23), `next` 검증을 M5로(24), 타임존 UTC(25), 12번이 4번을 기다림(26)
