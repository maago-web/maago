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
- **CSRF와 CORS는 한 목록**: `Config.Origins []string`. 이 목록의 출처는 "우리 앱처럼 믿는 출처"다: `AddTrustedOrigin`으로 CSRF 검사를 통과하고, CORS 응답(`Access-Control-Allow-Origin`은 그 출처 그대로, `*`는 받지 않는다)을 받는다. 메서드·헤더·자격 증명은 기본값(라우트의 메서드, `Content-Type`, 자격 증명 켬)으로 시작하고 필드는 첫 소비자인 12번이 더한다. 둘로 나누면 MILESTONES가 말한 "허용한 출처의 POST가 403"이 생긴다. preflight(OPTIONS)는 `ServeMux`보다 앞에서 답한다(메서드 패턴은 405를 낸다). admin은 스스로 건다(D-47, M6)
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

### 계획

먼저 확인한 것: D-116의 `return maago.SeeOther(url), nil`은 컴파일되지 않는다(Go는 호출 결과의 타입 인자를 반환 자리에서 추론하지 않는다, 실험). 사람의 결정(2026-10-01): 타입 인자를 쓴다, `maago.SeeOther[PostFormPage](url)`. `Show(p)`는 추론된다. 기각: P를 쓰지 않는 인터페이스(`SeeOther(url)`이 되지만 다른 페이지의 `Show`도 컴파일되어 실행 때 500, PHILOSOPHY 1). D-116을 이 커밋에서 고친다

파일
- 새 루트 `http.go`(런타임): 아래 API. stdlib만
- 새 루트 `http_test.go`: `httptest`로. DB는 드라이버 없는 가짜 `driver.Connector`의 `sql.OpenDB`(루트 모듈에 드라이버가 없다, D-90)

API
```go
type Config struct {
	DB      *sql.DB
	Apps    []App
	Origins []string                                         // 믿는 출처: CSRF 통과 + CORS
	OnError func(ctx context.Context, r *http.Request, err error) // nil이면 log.Printf
}
func Handler(Config) (http.Handler, error)

type App struct{ name string; routes []AppRoute }            // 생성 코드가 NewApp으로
type AppRoute struct {                                       // 생성 코드만 쓴다
	Method, Path, Name string
	API   bool
	Serve func(w http.ResponseWriter, r *http.Request) error
}
func NewApp(name string, routes ...AppRoute) App

func WithDB(ctx, *sql.DB) context.Context
func DBFrom(ctx) *sql.DB                                     // 없으면 WithDB를 가리키는 panic

type Result[P any] struct{ page P; to string }
func Show[P any](p P) Result[P]
func SeeOther[P any](url string) Result[P]
func (r Result[P]) Page() (P, string)                        // 생성된 어댑터가 읽는다: 이동이면 url != ""

func Status(code int, err error) error                       // 생성 코드: Status(404, 해석 오류), Status(400, …)
```
- `Config`의 `Auth`(9번)·`TimeZone`·`Prefix`(11번)·CORS 메서드·헤더 필드는 그 작업이 더한다(쓰는 곳이 없으면 두지 않는다). 지금 CORS는 모양 절의 기본값만: 요청한 메서드에 맞는 라우트가 있으면 그 메서드, 헤더 `Content-Type`, 자격 증명 켬
- ★ `Status`는 생성 코드가 해석 실패를 404·400으로 알리는 자리다. 뷰가 403·409를 내는 법(4번 열린 질문)에도 쓸 수 있지만 그것은 4번이 정한다
- ★ `OnError`가 nil이면 `log.Printf`(조용히 버리지 않는다, PHILOSOPHY 7)

동작
- `Handler`: `Config.DB`가 nil, `Apps`가 비었음, 같은 앱 두 번(`ServeMux`의 panic을 잡아 오류로), `Origins`의 잘못된 값(`AddTrustedOrigin`의 오류, 끝 `/`·경로 있음)은 오류. 라우트마다 `mux.Handle(Method+" "+Path, …)`
- 감싸는 순서(바깥부터): CORS preflight → `CrossOriginProtection` → `ServeMux` → 라우트마다 recover·오류 처리
- CORS: `Origin`이 `Origins`에 정확히 있을 때만. preflight(`OPTIONS` + `Access-Control-Request-Method`)는 그 메서드로 `mux.Handler`를 물어 라우트가 있으면 204와 `Access-Control-Allow-Origin`(그 출처 그대로)·`-Methods`·`-Headers: Content-Type`·`-Credentials: true`·`Vary: Origin`, 없으면 다음으로(→ 405). 실제 요청에는 `-Allow-Origin`·`-Credentials`·`Vary`
- 오류 → 상태(D-116): `Status`로 감싼 것은 그 코드, `errors.Is(err, sql.ErrNoRows)`는 404, 그 밖은 500과 `OnError`. 4xx는 `OnError`로 보내지 않는다(사용자의 잘못된 요청이지 서버 오류가 아니다)
- 응답: API 라우트는 RFC 9457(`Content-Type: application/problem+json`, `{"type":"about:blank","title":상태 글,"status":코드}`, 4xx면 `detail`에 오류 글. 500은 `detail` 없음: 내부를 새지 않는다). HTML 라우트는 `http.Error(상태 글)`(최종 사용자 문구와 바꾸는 법은 6번)
- panic: recover해서 `http.ErrAbortHandler`는 다시 panic, 그 밖은 `OnError`(panic 값과 `debug.Stack()`)와 500

하지 않기로 한 것
- 생성된 어댑터(4번), 템플릿 렌더(5번), JSON 응답 본문(7번), 인증(9번), 하위 경로·타임존(11번), CORS 설정 필드

단계
1. `Config`·`App`·`Handler`와 라우팅, `WithDB`·`DBFrom` → 확인: 두 앱의 라우트로 요청이 가고 ctx에서 DB가 나옴, `DBFrom`의 panic 글, `Handler`의 설정 오류 넷(DB 없음, 앱 없음, 같은 앱 두 번, `Origins`의 `https://a.com/`)
2. 오류·panic → 확인: `fmt.Errorf("…: %w", sql.ErrNoRows)` 404, `Status(400, …)` 400, 그 밖 500과 `OnError` 한 번(4xx는 부르지 않음), API 라우트의 problem JSON(4xx의 `detail`, 500의 `detail` 없음), panic이 500과 `OnError`(글에 panic 값), `http.ErrAbortHandler`는 `Serve` 밖으로 다시 panic, `OnError`가 nil이면 로그
3. CSRF·CORS → 확인: `Sec-Fetch-Site: cross-site` POST 403, 헤더 없이 `Origin` 불일치 POST 403, 신뢰 출처 POST 통과(포트가 다르면 403), 헤더 없는 POST 통과, 신뢰 출처 preflight에 `Access-Control-Allow-*`와 204·비신뢰 출처와 라우트 없는 메서드에는 없음, 신뢰 출처의 실제 GET에 `Allow-Origin`
4. `Result`·`Show`·`SeeOther` → 확인: `Page()`가 페이지와 빈 url, 이동이면 url
5. 두 모듈 `go vet`·`go test`, `gofmt -l .`. 런타임 import 게이트(`TestNoReflect`, 런타임이 기대는 내부 패키지 시험)가 통과

계획 리뷰 (codex·fable) — 위 계획을 이렇게 고친다
- `Status(code, err)`(fable 1): 3번에서 정한다. 생성 코드와 **뷰가 함께 쓰는** 4xx·5xx의 유일한 통로다(`return Page{}, maago.Status(http.StatusForbidden, err)`). 403·404·409는 정말 오류라 D-116의 "이동을 error 값으로" 기각(이동은 오류가 아니다)과 부딪히지 않는다. 400~599 밖이면 panic(프로그램 오류, codex 4). `%w`로 감싸도 그 코드(`errors.As`). 열린 질문 "뷰가 다른 상태를 내는 법 → 4번"을 닫는다. D-116을 이 커밋에서 고친다
- 상태 정하는 순서(codex 4): `Status`가 먼저, 다음 `sql.ErrNoRows` 404, 그 밖 500. 최종 500이면(`Status(500, …)` 포함) 원래 오류를 `OnError`에 정확히 한 번
- `Serve`의 계약(fable 6, codex 5): 오류를 돌려줄 때는 아직 쓰지 않았어야 한다. 런타임은 `ResponseWriter`를 감싸 썼는지 보고, 이미 썼으면 상태를 덮지 않고 `OnError`만(500 갈래일 때). panic도 같다. 8번(스트리밍)이 이 계약을 이어받는다. 문서 주석에 적는다
- panic → 오류(codex 6): `error` panic은 `%w`로 감싸고, 그 밖은 `%v`. 글에 `debug.Stack()`. `http.ErrAbortHandler`는 `p == http.ErrAbortHandler`일 때만 다시 panic
- 앱 검사(codex 7·fable 5): 이름을 맵으로 직접 검사해 같은 앱 두 번을 maago의 글로, 빈 이름(`App{}`)·nil `Serve`도 오류. `ServeMux`의 panic은 쓰지 않는다
- `SeeOther("")`는 panic(codex 8, 빈 url이 "그린다"로 읽히지 않게)
- CORS(codex 1·2·3): `Origin`이 있는 모든 응답에 `Vary: Origin`. preflight는 출처·요청 메서드(`mux.Handler`의 pattern이 비지 않아야 한다: 다른 메서드만 있으면 405로 빈 pattern)·요청 헤더(`content-type`만)를 모두 보고, 하나라도 아니면 CORS 헤더 없이 다음으로. preflight 응답의 `Vary`는 `Origin, Access-Control-Request-Method, Access-Control-Request-Headers`
- CSRF 403은 API 라우트면 RFC 9457(fable 2): `SetDenyHandler`에서 같은 `mux.Handler` 조회로 `API`를 본다
- CORS 메서드·헤더·자격 증명 필드: 첫 소비자인 12번(다른 출처의 API 호출)이 더한다. `Authorization`은 9번·M5. 모양 절을 고친다(fable 4)
- 13번 문서 항목: D-47 "admin은 스스로 건다"는 `Handler`의 전역 CSRF로 낡는다. M6에서 정리(fable 8)
- 확인 더함(fable 3·7): `Origins`가 비었을 때 같은 출처 POST(`Sec-Fetch-Site: same-origin`, 헤더 없이 `Origin` 호스트 == `Host`) 통과, API 라우트의 교차 출처 POST는 403 problem, HTML 500 본문에 오류 글이 없음, 비신뢰 출처의 실제 요청에 `Allow-Origin` 없음·`Vary: Origin`은 있음, `%w`로 감싼 `Status`, 다른 메서드만 있는 경로의 preflight, 허용되지 않은 요청 헤더의 preflight, 쓴 뒤의 오류·panic은 상태를 덮지 않음, 빈 이름·nil `Serve`·같은 이름 두 앱
- 받지 않음: 없음. 파일 절의 import 목록은 뺀다(fable 9)

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `csrf`를 `handler` 필드에서 지역 변수로, panic 글의 `debug.Stack()`을 한 번만
- codex(샌드박스가 `/tmp`를 막아 코드만 읽은 리뷰): 받음 — `written`에 `Flush`(8번의 스트리밍 뷰가 `w.(http.Flusher)`로 쓴다, 1), 1xx `WriteHeader`는 응답을 쓴 것이 아니다(2). 받지 않음 — `Hijack`·`ReadFrom`: `Unwrap`으로 `http.ResponseController`가 닿고, `io.Copy`는 `Write`로 된다
- fable(변이 36개): 받음 — `Status(503, …)`이 `OnError`에 가지 않던 것: 보고는 5xx 모두로(1), 라우트 없는 경로의 교차 출처 POST(2), 요청 헤더 없는 DELETE preflight(3), `WriteHeader`만 쓴 뒤의 오류(4), preflight 헤더를 단 POST(5), `Status`의 범위 끝(6), `Status(code, nil)`은 panic(7, 빈 글이 생겼다), `Origins`는 브라우저가 보내는 그대로라는 주석(10). 고친 뒤 여섯 변이를 다시 돌려 모두 잡힌다. 9(쓴 뒤의 4xx는 아무 일도 없음)는 `Serve`의 계약대로 둔다

## 4. 뷰 시그니처와 어댑터 생성 (`urls_gen.go`, `views_gen.go`)

- 뷰 시그니처(D-116의 두 모양), In의 태그(`path=`·`query=`, 닫힌 목록 D-17), 필드 타입(D-77 기본 타입과 포인터), 경로 와일드카드와 path 필드의 짝
- 생성: URL 함수(타입 검사 전), 어댑터·등록(뒤)

→ 확인: 골든(D-67), 잘못된 시그니처·태그·타입·짝 없는 와일드카드 각각 오류, 생성된 핸들러로 `/post/abc` 404·없는 글 404·`?page=abc` 400·있는 글 200

### 계획

파이프라인 (라우트가 있는 프로젝트만. 없으면 지금과 같다)
```
 … DB 검사 → models_gen·queries_gen(메모리)
   → (1) 뷰의 모양을 ast로: 시그니처의 꼴, In 타입과 그 태그, 와일드카드 짝 → URL 함수와 `var App maago.App`을 담은 urls_gen.go(메모리)
   → (2) gotypes.Check(루트, 메모리 생성물): 사용자 코드가 PostDetailURL·board.App을 써도 타입 검사가 된다
   → (3) 타입으로 뷰 검사: 시그니처, In 필드 타입, P, 템플릿 파일
   → (4) views_gen.go(메모리): 어댑터와 `func init() { App = maago.NewApp(…) }`
```
- ★ `App`은 urls_gen.go의 `var App maago.App`로 타입 검사 전에 선언하고, 어댑터를 가리키는 값은 views_gen.go의 `init()`이 채운다. 어댑터는 타입 정보로 만들어지는데 `main`의 `board.App`은 타입 검사 때 이미 있어야 하기 때문이다(한 번만 검사한다). D-114의 명시적 등록은 그대로(`main`이 목록에 넣지 않으면 그 앱은 없다)
- ★ URL 함수는 타입 검사 전에 ast로 만든다(사용자 뷰가 부른다). 그래서 path 필드의 타입은 ast에서 읽히는 `string`·`int64`·`int32`뿐이다(이름 붙은 타입은 오류). (3)이 같은 것을 타입으로 다시 본다
- D-117에 "라우트가 있는 프로젝트만 타입 검사한다"를 이 커밋에서 적는다(1번 계획의 약속)

파일
- 새 `internal/web/views.go`: (1)의 ast 검사와 (3)의 타입 검사. 새 `internal/web/gen.go`와 `templates/urls.go.tmpl`·`views.go.tmpl`(매퍼처럼 `text/template` + `go/format`, 생성기 안이라 괜찮다)
- `internal/build/build.go`: 생성 단계 뒤에 위 (1)~(4)
- `internal/mapper/names.go`: `declared`를 `Declared`로 내보내 web의 이름 검사가 쓴다
- 루트 `noreflect_test.go`: 생성물 import 허용 목록을 `mapper.Imports`와 `web.Imports`의 합으로(D-66 고침). 게이트 시험의 "어디서도 허용되지 않는" 예를 `strconv`에서 `os`로
- 시험: `internal/web/views_test.go`(오류 표 골든, 생성물 골든 `testdata/*.golden`), 종단은 `cmd/maago/web_test.go`: 픽스처 `cmd/maago/testdata/web/`(go.mod·go.sum은 게시판 튜토리얼과 같은 require, main.go, `site/` 앱: 모델 Post, 마이그레이션, urls.go, views.go, templates, 앱의 `_test.go`)를 임시 디렉터리에 복사 → `build.Gen` → `go test ./...`(튜토리얼 걷기와 같이 `go.work`로 이 저장소). 앱의 시험이 `httptest`로 요청한다(SQLite 메모리. `maagotest`는 10번)

뷰 (D-116)
- 시그니처: `func(context.Context, In) (P, error)`, `func(context.Context, In) (maago.Result[P], error)`, 탈출구 `func(http.ResponseWriter, *http.Request)`. 그 밖은 오류(fix에 세 모양). 탈출구의 어댑터는 부르고 nil(8번이 스트리밍을 확인한다)
- In: 같은 패키지의 이름 붙은 struct, 또는 `struct{}`. 필드는 모두 태그가 있다: `maago:"path=이름"`·`maago:"query=이름"`(닫힌 목록, `model.ParseTag`로 읽고 모르는 키는 did you mean). 태그 없는 필드는 오류(값이 어디서 오는지 모른다. 폼 필드는 6번이 `form`을 더한다)
- path 필드 타입: `string`·`int64`·`int32`. query 필드 타입: `string`·`int64`·`int32`·`bool`·`float64`와 그 포인터. ★ 포인터 아니면 필수: 없으면 400. 포인터면 없을 때 nil. 같은 키 두 번은 400(조용히 첫 값을 쓰지 않는다). 빈 값(`?page=`)은 `string`이면 "", 숫자면 해석 실패 400. `time.Time`은 11번(표시 타임존)과 함께
- 와일드카드 짝: 경로의 `{x}`마다 `path=x` 필드 하나, `path=` 필드마다 와일드카드. `{x...}`는 오류(v0은 쓰는 곳이 없다, fix: 경로를 나눈다). 한 뷰를 두 라우트가 쓰면 각 라우트가 짝을 맞춰야 한다
- P: HTML 라우트는 같은 패키지의 이름 붙은 struct(템플릿 이름이 이름에서 나온다, D-116: `PostDetailPage` → `templates/post_detail_page.html`, 이름 변환은 D-76의 snake). 템플릿 파일이 없으면 오류, `html/template`로 파싱이 안 되면 오류(내용 검사는 5번). API 라우트의 P는 아무 타입(`encoding/json`, 요청 본문·problem의 나머지는 7번)
- ★ 이 작업의 렌더: 앱의 `templates/*.html`을 `//go:embed`로 싣고 시작할 때 `html/template`로 한 번 파싱, 페이지 타입의 파일을 `bytes.Buffer`에 실행한 뒤 쓴다(실패하면 아무것도 쓰지 않고 500, 3번의 `Serve` 계약). 레이아웃(`base.html`)·`{{url}}`·필드 검사·부분 응답은 5번
- `Result[P]`: `Page()`가 url이면 `http.Redirect(w, r, url, 303)`

생성 이름과 겹침
- 앱 패키지에 생기는 이름: `App`, 라우트마다 `XxxURL`(이름의 낙타 꼴 + `URL`)과 `serveXxx`, 템플릿이 있으면 `maagoTemplates`. 그 패키지의 사용자 이름(테스트 포함, `mapper.Declared`)과 겹치면 생성 오류. `web.Imports`(`bytes`·`context`·`embed`·`errors`·`fmt`·`html/template`·`encoding/json`·`net/http`·`net/url`·`strconv`, maago)의 이름을 사용자가 최상위에 선언해도 오류(매퍼와 같은 규칙)
- 하지 않기로 한 것: 매퍼의 생성 이름과 web의 생성 이름 사이의 겹침(`GetPost` 같은 CRUD 이름과 `XxxURL`·`serveXxx`는 꼴이 달라 겹칠 수 없다. 쿼리 이름 `PostDetailURL`은 가능하지만 드물다: 그러면 타입 검사가 overlay 오류로 잡는다)

URL 함수
- `func PostDetailURL(id int64) string`: 인자는 와일드카드 순서, `int64`·`int32`는 `strconv`, `string`은 `url.PathEscape`. `{$}`는 지운다. 탈출구 뷰의 와일드카드는 `string`

하지 않기로 한 것
- 템플릿 내용 검사·레이아웃·`{{url}}`(5번), 폼(6번), API 요청 본문과 problem의 `errors`(7번), 인증(9번), `time.Time` query(11번), 2번이 미룬 "board.App 쓰임"을 타입으로 다시 보는 일(지금은 ast 검사로 충분하고, 잘못 씀은 타입 검사가 컴파일 오류로 잡는다)

단계
1. `mapper.Declared`, 생성물 import 허용 목록 → 확인: 게이트 시험(`os`가 걸림, `strconv`·`net/http`는 통과), 매퍼 시험 그대로
2. (1) ast 검사와 urls_gen.go → 확인: 오류 표(뷰가 함수가 아님은 2번이 봄; 여기서는 매개변수 수, In이 다른 패키지, 태그 없는 필드, 모르는 태그 키 did you mean, `{x...}`, 짝 없는 와일드카드와 짝 없는 `path=`, path 필드 타입 `float64`·이름 붙은 타입), URL 함수 골든(정수·문자열·`{$}`·탈출구), 생성 이름 겹침(`App`을 사용자가 선언, `PostDetailURL` 선언, 최상위 `url`)
3. 파이프라인과 타입 검사 → 확인: 라우트 없는 프로젝트는 `go list`를 부르지 않음(`PATH`를 비워도 `gen`이 지나간다), 라우트가 있으면 사용자 코드의 타입 오류가 위치와 함께, `main`의 `board.App`과 뷰의 `PostDetailURL`이 타입 검사를 지남
4. (3) 타입 검사와 views_gen.go → 확인: 오류 표(반환 둘이 아님, 첫 인자가 `context.Context`가 아님, P가 이름 없는 타입·다른 패키지, 템플릿 파일 없음, 템플릿 문법 오류), 어댑터 골든(path 정수, query 필수·포인터·bool, `Result`, 탈출구, API)
5. 종단 → 확인: `cmd/maago` 픽스처의 앱 시험이 `/posts/abc` 404, 없는 글 404(`sql.ErrNoRows`), 있는 글 200과 제목, `?page=abc` 400, `?page=` 없음(필수) 400·포인터 필드는 nil, 같은 키 두 번 400, POST 뷰의 `SeeOther` 303과 `Location`, API 라우트의 JSON. `maago check`가 그 뒤 통과(D-60)
6. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable) — 위 계획을 이렇게 고친다
- **App은 init이 아니다**(codex 1): 타입 검사의 overlay에는 urls_gen.go가 `var App maago.App`(값 없음)로 들어가고, 검사 뒤 **쓰는** urls_gen.go는 `var App = maago.NewApp(…)`(어댑터는 views_gen.go)로 바꾼다. 같은 패키지의 변수 초기화·`init()`이 빈 `App`을 보는 일이 없다. 타입 검사는 여전히 한 번이다(검사 뒤 만든 두 파일은 골든과 종단 시험이 지킨다). 위의 ★ 첫째를 고친다
- **(1)의 검사는 DB 앞**(fable 1): 시그니처의 꼴·In 태그·와일드카드 짝·생성 이름 겹침은 `early`(`web.Check`·`mapper.Names` 옆)에서, `web.Read`가 이미 파싱한 ast(`App`에 뷰의 `*ast.FuncDecl`과 그 패키지의 타입 선언을 든다)로. 생성 단계에서는 urls_gen.go를 만들기만 한다. 세 번째 파싱은 늘지 않는다(fable 11, 2번이 미룬 질문). gotypes는 자기 fset을 쓴다
- **생성 이름 겹침은 `mapper.Names` 하나로**(fable 2·4, codex 4): web의 이름(`App`·`XxxURL`·`serveXxx`·`maagoTemplates`)을 `mapper.Names`의 `seen` 맵에 먹인다(인자 하나). 쿼리 이름 `PostDetailURL`과의 겹침, `post_id`·`post_i_d`가 같은 `PostIDURL`이 되는 것, import 이름(`mapper.Imports`·`web.Imports`의 합)을 한 번에 한 문구로. 사용자가 낸 겹침이 overlay의 "maago 버그"로 보이지 않는다. `mapper.Declared`를 내보낼 필요가 없다. 낙타 꼴은 D-110의 거꾸로 snake(`mapper.fieldName`, `id`만 약어)
- **query 해석**(codex 3, fable 6): `url.ParseQuery(r.URL.RawQuery)`의 오류는 400(`r.URL.Query()`는 깨진 쌍을 조용히 버린다). 키의 값이 둘 이상이면 400. 없으면 포인터 아닌 필드 400·포인터 nil. 빈 값: `string`은 "", `bool`·숫자는 400. `bool`은 `strconv.ParseBool`, `float64`의 `NaN`·`Inf`는 400. D-116을 이 커밋에서 고친다("해석 실패 400"에 "필수 query가 없음·같은 키 둘"을 더한다). 열린 질문 "In 필드에 슬라이스"는 닫는다: 하지 않는다(쓰는 곳이 없다, 같은 키 둘은 400)(fable 5)
- **템플릿 싣기**(codex 2): HTML 라우트가 있는 앱만 `//go:embed templates/*.html`과 `maagoTemplates`·`html/template`. API·탈출구만 있는 앱은 만들지 않는다. 하위 디렉터리는 보지 않는다. `html/template` 파싱 오류는 `template: x.html:3: …`를 루트 기준 경로(D-88)의 위치로 바꿔 낸다(fable 11)
- **URL 함수의 빈 문자열**(codex 5): 빈 `string` 인자는 panic(`/post/`은 `{id}`에 맞지 않는다). 확인에 `/`·`%`·공백·`?`·`#`이 든 값으로 만든 URL이 실제 `ServeMux`에서 같은 값으로 돌아오는 것
- **종단 시험의 환경**(codex 6): 임시 프로젝트와 저장소 경로를 `EvalSymlinks`한 뒤 `go.work`를 먼저 쓰고, `build.Gen`(안의 `go list`)과 `go test` 모두에 같은 `GOWORK=<임시>/go.work`·`GOPROXY=off`. 튜토리얼 걷기와 같은 모양
- `Handler`의 "maago gen did not make" 문구에 "or run maago gen"(fable 11)
- 확인 더함: 렌더 실패(nil 역참조) → 500·본문에 부분 HTML 없음·`OnError` 한 번(fable 7). D-61: 서로 독립인 뷰 오류 둘이 한 번에, `early`에서 겹친 라우트 이름과 태그 없는 In 필드가 함께, 사용자 타입 오류가 있으면 (3)의 오류는 안 나옴(fable 8). query 필드 `[]string`·`time.Time`·`**int64`, path 필드 `*int64`(fable 9). `int32` path의 범위 밖 404, 디스크의 낡은 `views_gen.go`가 지운 뷰를 가리켜도 통과(fable 10). 깨진 query(`%zz`) 400, `?draft` bool 400, `NaN` 400
- ARCHITECTURE §3 표(생성 뒤에 검사가 오는 모양)는 13번 목록에
- 받지 않음: 없음

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `typeText`를 `types.ExprString`으로, `GoName`을 지우고 `mapper.FieldName`(내보냄, fable 4와 같다), `Generated`를 `More`에 넣음, 쓰이지 않던 `App.files`
- codex(샌드박스가 `/tmp`를 막아 코드만 읽은 리뷰): 받음 — 생성 코드는 `templates/*.html` 전체를 한 세트로 파싱하는데 검사는 페이지 파일만 봐, 쓰이지 않는 파일의 문법 오류가 시작 때 panic이 되던 것
- fable(변이 39개): 받음 — 템플릿을 런타임과 같은 `ParseFS(…, "*.html")` 한 세트로 파싱하고 페이지는 `Lookup`으로(1, macOS의 대소문자로 다른 이름이 통과하던 것도 닫힘, 7), 빌드 제약으로 빠진 파일의 뷰에서 nil 역참조(2), path 필드 타입 오류 뒤의 "reads no {x}" 연쇄 오류(3), 문구 둘: 필드 있는 익명 In, 소스 둘(8), 생존 변이마다 시험(`more.Imports`, `id`, 제네릭 페이지, struct 아닌 페이지, 익명 In, 소스 둘, 끼워 넣은 필드, 제네릭 뷰, `maagoTemplates`, sniff되지 않는 본문의 Content-Type), 타입 오류가 템플릿 없음을 가리는 것이 시험에 보임(6). 고친 뒤 열 변이를 다시 돌려 모두 잡힌다(남은 하나는 같은 오류를 두 번 내고 `diag.Write`가 지우는 같은 동작). 받지 않음 — 9 In 필드 타입을 타입으로 다시 보기(`type int64 string` 같은 패키지 수준 재선언만 뚫고, 그때는 생성 코드의 컴파일 오류가 overlay 버그 문구로 난다. 하지 않기로 적는다)

## 5. 템플릿 검사와 렌더 (D-42, D-45)

- 페이지 타입마다 템플릿을 찾고(없으면 오류), `parse` 트리를 걸으며 `.필드` 사슬을 P 타입에 맞춘다: `range`·`with`·변수(`$x`)의 점 바뀜, 인자 없는 메서드, 슬라이스·맵 원소. 타입을 정할 수 없는 식(`index`·`printf`·파이프로 넘긴 값 등)은 검사기가 타입을 정하거나 "cannot check" 오류다. 조용히 통과하는 갈래는 없다(PHILOSOPHY 7)
- `{{url "이름" 인자…}}`: 이름이 있고 인자 수·타입이 URL 함수와 맞음. 모르는 함수는 오류(닫힌 목록)
- 부분 응답(D-45)의 블록 이름. 요청이 고르는지(HTMX 헤더) 뷰가 고르는지는 5번 계획에서

→ 확인: `{{.Post.Titel}}` → `…:줄:칸: PostDetailPage.Post has no field "Titel"` + did you mean(칸 값을 박는다, 1 기준), `post_detial` → did you mean. `range` 안의 점, `$` 변수, 인자 수가 틀린 `url`, 없는 블록, "cannot check" 한 경우 각각. 렌더 골든

### 계획

모양 (결정에 없는 선택은 ★)
- ★ **템플릿 파일**: 앱의 `templates/`에는 페이지 템플릿(페이지 타입의 snake 이름, D-116)과 레이아웃 `base.html`(있으면)뿐이다. 그 밖의 `.html`은 오류(닫힌 목록, PHILOSOPHY 6: fix는 "페이지 타입을 만들거나, 공유할 것은 base.html의 `{{define}}`으로"). 4번의 "디렉터리 전체를 한 세트로"는 바꾼다: 페이지마다 세트 하나 = `base.html` + 그 페이지 파일. 한 세트에 페이지 여럿을 넣으면 모두 `{{define "content"}}`를 쓸 때 마지막 파일이 이긴다(`text/template`의 재정의 규칙)
- ★ **렌더**: `base.html`이 있으면 모든 페이지는 `base.html`로 시작하고(페이지는 `{{define "content"}}` 같은 블록을 채운다), 없으면 페이지 파일로 시작한다. 런타임 도우미 `maago.Pages(fsys, funcs, pages...)`가 시작할 때 세트를 만들고 `maago.Render`가 고른 세트를 실행한다
- ★ **부분 응답(D-45, 열린 질문의 답)**: 요청도 뷰도 아니고 **라우트 표**가 고른다. 옵션 `maago.Block("rows")`: 같은 뷰·같은 페이지 타입으로 그 블록만 그린다(`maago.GET("/posts/rows", "post_rows", PostList, maago.Block("rows"))`). 표 한 줄에 보이고(PHILOSOPHY 5), 블록 이름이 리터럴이라 생성 때 그 페이지 세트에 있는지 검사한다. 기각: 요청 헤더(`HX-Request`·`HX-Target`은 블록 이름이 아니다, 같은 URL이 헤더로 다른 것을 낸다), 뷰가 고름(`Show(p).Block(x)`: 이름이 식이면 검사할 수 없다). `RouteOption`은 `int` 상수에서 비교할 수 있는 struct 값으로 바뀐다(`maago.LoginRequired`·`maago.API`는 그대로 쓴다). `Block`과 `API`는 함께 쓸 수 없다
- **`{{url "이름" 인자…}}`**: 이름은 문자열 리터럴(아니면 "cannot check"), 프로젝트의 라우트 이름(did you mean), 인자 수는 와일드카드 수, 인자 타입은 URL 함수의 매개변수 타입(`int64`·`int32`·`string`, 상수는 맞으면 된다). 런타임: 앱마다 생성된 `maagoRoutes`(프로젝트 모든 라우트의 이름 → 경로 패턴 글. 다른 앱 패키지를 import하지 않아 순환이 없다)와 `maago.URLFunc(maagoRoutes)`가 패턴의 와일드카드를 채운다(정수는 10진, 글은 `PathSegment`)
- **함수는 닫힌 목록**: `text/template`의 내장 함수와 `url`. 생성 때도 같은 `FuncMap` 이름으로 파싱하므로 모르는 함수는 `html/template`의 "function not defined"가 위치와 함께 잡는다. `time`(11번)·폼 함수(6번)는 그 작업이 더한다
- **필드 검사**: 페이지 세트마다 시작 템플릿(base 또는 페이지)을 페이지 타입을 점으로 걷는다. 노드: 출력·`if`·`range`(슬라이스·배열·맵·채널 아님: 원소 타입, `$i, $v` 변수)·`with`·`{{template "x" .}}`·`{{block}}`(그 정의를 그 점 타입으로 걷는다, (이름, 타입)으로 한 번만)·변수(`$` = 페이지, `$x := …`). 필드 사슬: struct 필드(포인터는 따라간다)·인자 없는 메서드(값 하나 또는 값과 `error`)·문자열 키 맵의 원소. 타입: `eq`·`ne`·`lt`…·`not`·`and`·`or`의 결과는 조건으로만 쓰이면 상관없다. 결과 타입을 정할 수 없는 식(`index`·`slice`·`call`·`and`·`or`·`interface` 값·`printf` 결과의 필드 등) 뒤에 필드 접근·`range`·`with`의 점 사용이 오면 그 자리에 "cannot check"(fix: 페이지 struct에 그 값을 필드로 계산해 넣는다). 정할 수 없는 값을 그대로 출력하는 것은 된다(무엇이든 출력된다)
- **이스케이프 문맥**: `html/template`의 문맥 오류(`{{.X}}`가 속성 이름 자리 등)는 실행 때 처음 나므로, 생성 때 세트마다 영값 데이터로 `io.Discard`에 실행해 `*template.Error`(이스케이프 오류)만 위치와 함께 낸다. 실행 오류(영값의 nil 등)는 버린다(필드 검사가 따로 본다)
- **위치**: `parse.Node.Position()`의 바이트 오프셋을 파일 글에서 1 기준 줄·칸으로(D-62, D-88). 메시지는 `PostPage.Post has no field "Titel"` + did you mean(필드·메서드 이름 중)

파일
- `internal/web/templates.go`(새): 세트 만들기, 걷기, `url` 검사. `typecheck.go`의 `parseTemplates`를 대신한다
- 루트 `route.go`(`RouteOption` struct, `Block`), `view.go`(`Pages`, `URLFunc`, `Render`의 시작 이름), `internal/web/routes.go`(`maago.Block("x")` 읽기, `Block`+`API` 오류, 블록 라우트도 페이지 타입이 필요), 생성 틀(`maagoRoutes`, `maago.Pages`)
- 시험: `internal/web/templates_test.go`(오류 표 골든, 렌더 골든은 생성된 코드를 종단으로), `cmd/maago/testdata/web`에 `base.html`·블록 라우트·`url` 링크를 더해 종단

하지 않기로 한 것
- `time`·폼 함수(11·6번), 템플릿 오버라이드(v1, D-42), 생성 때 실제 데이터로 실행하는 것

단계
1. 런타임: `RouteOption` struct·`Block`, `Pages`, `URLFunc`, `Render` → 확인: 레이아웃 있음·없음의 렌더, 블록만 렌더, `URLFunc`의 정수·글·없는 이름 오류·인자 수 오류
2. 라우트 표의 `Block` → 확인: 읽기, 리터럴 아님·`API`와 함께·같은 옵션 두 번 오류
3. 세트와 파일 목록, 이스케이프 문맥 → 확인: 페이지 아닌 파일 오류, 레이아웃 있음·없음, 문맥 오류 위치
4. 걷기 → 확인(작업의 확인 그대로): `{{.Post.Titel}}`이 `…:줄:칸: PostPage.Post has no field "Titel"` + did you mean(칸 값을 박는다), `range` 안의 점, `$`·`$x` 변수, 맵 원소, 포인터 필드, 메서드, `with`, `{{template}}`·`{{block}}`, 없는 블록(`maago.Block`), "cannot check"(`index` 뒤의 필드), 오류 둘이 한 번에
5. `url` → 확인: `post_detial` did you mean, 인자 수, 인자 타입(`"x"`를 `int64` 자리에), 이름이 리터럴 아님
6. 종단 → 확인: 픽스처의 레이아웃과 블록 라우트와 `url` 링크가 생성된 코드로 렌더(본문 비교), `maago check`가 그 뒤 통과
7. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **부분 응답은 `maago.Block("rows")`**: 사람의 결정(2026-10-01, fable 1이 묻게 함). 대가: 부분 응답마다 URL·이름이 하나 더 생기고, 뷰는 페이지 전체를 계산한 뒤 한 블록만 그린다. D-45를 이 커밋에서 고친다
- **`RouteOption`**(codex 7): 불투명한 struct. `maago.LoginRequired`·`maago.API`는 값(변수), `maago.Block(name)`은 생성자. 표에서: 같은 옵션 두 번, `Block` 둘, `Block`과 `API` 함께는 오류
- **페이지 세트의 순서**(codex 1·6, fable 4·5·6): 런타임과 생성기가 같은 알고리즘: 페이지마다 새 `html/template`에 `base.html`(있으면)과 페이지 파일을 `ParseFS(fsys, "templates/base.html", "templates/page.html")`로(`Clone`을 쓰지 않는다). 시작 이름은 `base.html`이 있으면 그것, 없으면 페이지 파일. 생성기는 **먼저 트리를 걷고, 그다음** `Execute(io.Discard, nil)`로 이스케이프만 본다(실행은 트리를 제자리에서 고친다. 영값이 아니라 nil: 생성기에는 P의 값이 없고, 이스케이프는 데이터를 보기 전에 끝난다, 사용자 메서드도 돌지 않는다). `errors.As(err, *template.Error)`만 진단하고 위치는 `Error.Node.Position()`과 그 트리의 `ParseName`(define 이름이 아니라 파일)
- **블록 라우트의 블록은 단독으로도 실행해 본다**(fable 7): 열린 태그로 끝나는 조각은 단독에서만 `ErrEndContext`
- **파일 규칙**(fable 2·3): `base.html`이 있으면 페이지 파일의 최상위(`{{define}}` 밖)는 공백만(아니면 "never rendered" 오류), 없으면 최상위에 글이 있어야 한다(`{{define}}`만이면 빈 화면이라 오류). HTML 라우트가 없는 앱의 `templates/*.html`과 `templates/`의 하위 디렉터리도 오류. 대가: 레이아웃을 건너뛰는 페이지는 없다(맨 조각은 `Block` 라우트로)
- **걷기 규칙**(codex 3·4·5, fable 8~11):
  - `$`는 그 템플릿 호출의 인자(페이지가 아니다). `{{template "x" .Post}}` 안에서 `$`·`.`은 `Post`. `{{template "x"}}`(인자 없음)의 점은 nil이라 안의 필드 접근은 오류. (이름, 타입)마다 한 번, 재귀는 멈춘다
  - 변수는 `if`·`with`·`range`의 끝까지(스코프를 쌓고 뺀다), `else if`는 중첩, `break`·`continue`는 가장 안쪽 `range`
  - `range`: 슬라이스·배열(원소), 맵(`$k, $v`), 채널(원소), 정수(`range N`, 원소 int), 함수 반복자 `iter.Seq[T]`(T)·`iter.Seq2[K,V]`(변수 하나면 K). 그 밖은 오류
  - 필드 사슬: 내보낸 struct 필드(포인터는 따라간다, 비공개 필드는 오류), 메서드는 **주소를 취할 수 없는 값의 메서드 집합**으로 찾는다(`LookupFieldOrMethod(…, addressable=false)`: 어댑터는 페이지를 값으로 넘겨 포인터 리시버 메서드는 실행 때 오류다), 인자 없고 값 하나 또는 (값, `error`). 인자 있는 메서드 호출은 오류(닫힌 목록). 문자열 키 맵의 원소
  - 함수의 **인자는 늘 걷는다**: `{{printf "%s" .Post.Titel}}`·`{{if .Post.Titel}}`의 오타도 잡힌다. 결과 타입을 정할 수 없는 값(`index`·`slice`·`call`·`and`·`or`·`printf`의 결과, `interface` 값)에 필드 접근·`range`·`with`의 점 사용이 이어지면 "cannot check"
  - 칸(fable 13): 틀린 이름 자체(`.Post.Titel`이면 `Titel`의 칸). 13번이 TUTORIAL-FUTURE §8의 칸을 맞춘다
- **`url`**(codex 2, fable 12): 런타임 `maago.URLFunc(map[이름]패턴)`은 `func(name string, args ...any) (string, error)`이고 인자의 실제 타입(`int`·`int64`·`int32`·`string`)으로 채운다. 생성 때 검사가 막는 것의 두 번째 방어선이다. 생성된 `XxxURL`과 같은 일을 두 번 하므로 종단 확인에 "픽스처의 모든 라우트에서 `{{url}}` == `XxxURL()`"을 넣는다. 대가: 앱 B의 라우트가 바뀌면 앱 A의 `views_gen.go`도 바뀐다(`check`가 A를 낡았다고 본다)
- **D-61의 확인**(fable 14): 한 파일의 `{{.Post.Titel}}`과 다른 파일의 `post_detial`이 한 번에(완료 조건 2의 모양, 13번 `TestM4Done`도)
- 확인 더함: 위 규칙마다 한 경우(하위 템플릿의 `$`, 인자 없는 `template`, 포인터 리시버 메서드, 비공개 필드, 인자 있는 메서드, 인자 속 오타, `range` 정수·맵·`iter.Seq2`, `break`), 파일 규칙 넷, 이스케이프 오류의 위치, 블록 단독의 `ErrEndContext`, 걷기와 실행의 순서를 바꾸면 깨지는 시험
- 받지 않음: 없음

구현 리뷰 (ponytail·codex·fable)
- 시험을 쓰며 잡은 것: 필드가 둘 이상인 노드의 위치는 `text/template`이 둘째 필드에 둔다(칸이 틀렸음), 이스케이프에 실패하면 트리가 지워져 파일 이름이 빠짐, 페이지 실행이 블록을 이미 이스케이프해 "블록 단독" 검사가 돌지 않음(새 세트에서 실행), 루트 `noreflect_test.go`는 `internal/web`이 루트를 import해 순환이라 `maago_test`로. 변이: 걷기와 실행의 순서, 필드 위치, `$`, `with`, `nil` 점, 포인터 리시버, 비공개, 파이프, `=` 대입, `iter.Seq2`, 맵 키·원소, 정수 range, 블록 단독, 파일 규칙 둘이 모두 잡힌다
- ponytail: 받음 — `isPointer` 인라인, `exported`를 rune으로(fable 6과 같다), `URLFunc`의 인자 수 세기를 끝의 한 곳으로. 받지 않음 — `orLabel` 인라인, 맵 둘 합치기(고친 뒤 하나가 됨)
- codex: 받음 — `iter.Seq2`를 변수 하나로 돌면 변수와 점이 키(1), 정수·`iter.Seq`에 변수 둘은 오류(2), 이름 붙은 키 타입의 맵은 `.x`로도 `index`로도 못 읽어 오류(3), 포인터의 포인터는 필드까지 따라간다(메서드는 값 자신에서만, 4). 받지 않음 — 5 `Render`에 없는 페이지(생성 코드만 부르고 이름은 생성기가 안다)
- fable: 받음 — 이스케이프 오류의 `Name`은 실행한 템플릿이라 레이아웃 아래 페이지의 노드를 base.html의 오프셋으로 잘라 panic(1: 실행 전에 노드 (종류, 위치) → 파일 표, 두 파일이면 파일 없이), 블록 라우트의 블록을 페이지를 점으로 걷지 않았음(2: 걷기에 더하고 "없는 블록"도 같은 곳에서), 런타임은 한 세트를 공유해 블록 라우트가 먼저 오면 페이지의 URL 문맥이 이중 이스케이프(4: `Pages`가 시작 때 페이지마다 한 번 실행해 생성기가 검사한 순서로 이스케이프), 인터페이스 range는 "cannot check"(5), 이스케이프 문구에서 문맥 덤프를 뺌(7), 파일 오류가 다른 페이지의 걷기를 가리지 않음(8, D-61), `maago.Block(b)`·`Block()`에 전용 오류(9), 시험 더함(10: 레이아웃 아래 노드 있는 이스케이프 오류, 블록 걷기, `Block` 둘, 하위 디렉터리, 페이지 없는 앱의 템플릿, 채널 range, 블록 먼저인 런타임 렌더). **계획 고침(3)**: 포인터 리시버 메서드는 "주소를 취할 수 없는 값"에서만 오류다. `text/template`은 주소를 취할 수 있는 값이면 `Addr()`로 부른다: 슬라이스 원소, 포인터 너머, 그 struct 필드. 페이지 자신·맵 원소·결과·채널·반복자 원소는 아니다. 걷기가 값마다 이를 들고 다닌다. 받지 않음 — 닿지 않는 `{{define}}` 금지(base.html의 정의는 어떤 페이지에서는 쓰이지 않는 것이 정상이다)


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
- ~~부분 응답을 요청이 고르는지 뷰가 고르는지~~ → 5번: 라우트 표의 `maago.Block("x")`(사람의 결정)
- ~~In 필드에 슬라이스(`?tag=a&tag=b`)를 받는지~~ → 4번: 받지 않는다
- ~~뷰가 404·400·500 밖의 상태(403·409)를 내는 법~~ → 3번: `maago.Status(code, err)`
- 게시판 이름 중복을 unique 위반으로 잡을지 검증 메서드로 잡을지 → 6번

## 초안 리뷰 (2026-09-28)

- codex: 받음 — `gc` importer는 `go list -export`로 `go` 바이너리를 부른다(1), 두 갈래 importer는 직접 짜야 한다(2), 파일 선택은 `go list`를 따르고 cgo는 지원하지 않음 오류(3), CSRF 확인 경우들(4), CORS의 메서드·헤더·자격 증명은 따로 필드, 한 목록의 보안 뜻을 적음(5), maagotest 주소 규칙과 PG `search_path`의 연결마다 확인(6), U-07을 M4·M5로 나눔(7), `maagotest`는 원칙의 예외로 명시(9), 충돌은 실제 `ServeMux` 규칙·URL 함수 인자와 이스케이프(10), 폼 흐름(12, 템플릿을 페이지 타입에), query·본문 400(13), 검사도 `html/template` 파싱을 거침·`embed`의 `..`(14), 응답 뒤 작업의 (b) 선택지(15), 작성자 검사를 사람에게(16, M5로), 확인 보강·정적 파일 URL은 M7(17), 파이프라인 순서(18). 8 명시적 등록은 초안이 받지 않았고(D-46) 사람이 골랐다(D-114). 받지 않음 — 11 `app:name`(D-115의 기각 이유)
- fable: 받음 — `embed` import(1), 레이아웃 자리(2), 생성물 두 번(3), POST의 템플릿(4), 폼 실패 상태를 사람에게(5), 해석 실패의 상태와 모르는 키(6), `WithDB`·`*sql.DB`·panic(7), contrib 라우트도 생성 때(8), unique 위반의 대가와 장치 하나(9), 오류 보고 훅 하나·`ErrAbortHandler`(10), 두 시그니처의 이유와 기각(11), 조건 1은 SQLite(12), `TestM4Done`(13), "cannot check"(14), 칸 1 기준(15), 1번 시간은 합성 픽스처·목록 둘(16), 스트리밍 시험(17), preflight 문구(18), 셋을 쌍으로(19), 결정적 스키마 이름·주소 규칙 셋·`mode=rw`·시험 자리(20), 10번의 숨은 일(21), `lookup`으로 export data(22), 웹훅 실패·재시도(23), `next` 검증을 M5로(24), 타임존 UTC(25), 12번이 4번을 기다림(26)
