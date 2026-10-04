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
- 폼 전체 검증과 사용자 검증 메서드(D-15), ~~DB를 보는 검증(ctx와 DB를 받는 메서드)~~ → 뷰가 쿼리로(D-119)
- ~~unique 위반을 폼 오류로: 런타임은 드라이버를 모르므로(D-90) 방언의 오류 글에서 D-86의 제약 이름을 찾는다. "DB를 보는 검증 메서드"와 장치가 둘이 되지 않게 게시판 이름 중복은 하나만 쓴다(fable 9, 계획에서 고른다)~~ → 장치를 만들지 않는다: 뷰의 쿼리 하나(D-119, 사람의 결정)
- 템플릿에서 폼과 오류를 그리는 법, 최종 사용자 문구와 바꾸는 법, 검증 실패는 200으로 다시 그림(D-116)

→ 확인: `int64` 칸에 빈 값·`abc`·범위 밖의 세 결과가 **서로 모두 다름**(값·문구를 쌍으로 비교), 필수 문자열의 빈 값, 선택지 밖, 사용자 메서드의 오류, ~~게시판 이름 중복이 두 방언에서 그 필드의 오류~~ 뷰가 찾은 중복이 그 필드의 오류(두 방언은 12번의 쿼리 파일), 다시 그린 폼이 입력값을 유지

### 계획

모양 (결정에 없는 선택은 ★)
- ★ **폼 선언**: 폼은 앱 패키지의 struct로, `maago.Form`을 임베드한다(모델이 `maago.Model`을 임베드하듯 `go/ast`로 찾는다). 필드 타입은 런타임 제네릭 `maago.Field[T]`이고 T는 `string`·`int64`·`int32`·`float64`·`bool`(모델 타입에서 `time.Time`·`[]byte`를 뺀 것: 시각은 표시 타임존과 함께 11번, 파일은 M7). 필드 하나가 (값, 있음, 오류)를 다 든다: `Value T`, `Raw string`(받은 글, 다시 그릴 때 쓴다), `Set bool`(빈 칸이 아님), `Error string`, 그리고 그리는 데 쓰는 `Name`(폼 키: 필드 이름의 snake_case)·`Label`·`Help`·`Choices`. 생성되는 폼 타입은 없다: 사용자 타입 하나를 뷰도 템플릿도 본다. 기각: 사용자가 값 struct를 쓰고 생성기가 `XxxFormState`를 만듦(타입 둘, 이름 규칙 하나 더)
- ★ **모델 폼**(D-11, Django `ModelForm`): `maago.Form`의 태그 `maago:"model=Board"`. 필드는 그 모델의 필드 이름이고 T는 모델 필드 타입과 같아야 한다(nullable 포인터 `*T`는 `Field[T]`와 `optional`). 규칙(`max`·`choices`·`label`·`help`)은 모델에서 오고 폼 필드에는 태그를 쓰지 않는다(쓰면 "모델 Board가 선언한다" 오류: 한 곳). 어떤 필드를 받을지는 폼이 필드를 나열해 보인다(Django `fields =`). 입력 struct로 옮기는 것은 뷰가 쓴다(생성된 `Input()` 없음, 필요해지면 그때)
- ★ **태그**(D-17 닫힌 목록, 모델과 같은 이름은 같은 뜻): `label`·`help`·`choices`(문자열)·`max`·`min`(문자열은 글자 수, 숫자는 값)·`optional`. 폼 필드는 **기본이 필수**다: 빈 칸은 `Required` 오류, `optional`이면 빈 칸은 `Set=false`의 영값(PHILOSOPHY 7: 조용히 영값을 넣지 않는다). `bool`은 체크박스라 늘 optional(빈 칸 = false, `optional`을 쓰면 오류). 모델의 `max`가 숫자에는 없으므로(문자열 길이) `min`·`max`를 숫자에 쓰는 것은 폼에서 넓힌다
- **세 결과가 다름**(완료 조건): `int64` 칸의 빈 값 → `Required`("this field is required"), `abc` → `NotNumber`, `99999999999999999999`(`strconv`의 범위 오류)·`min`/`max` 밖 → `OutOfRange`(문구에 범위). `Raw`는 셋 다 받은 글 그대로
- ★ **In에서 받기**: In 필드 `Form BoardForm `maago:"form"``(4번의 `path=`·`query=`와 같은 줄의 셋째 소스, 키 없음, In에 하나). 요청이 GET·HEAD면 폼은 **묶이지 않는다**: `Name`·`Label`·`Help`·`Choices`만 채우고 검증하지 않는다(빈 폼 화면). 그 밖의 메서드면 `r.ParseForm`의 본문(`application/x-www-form-urlencoded`. multipart는 M7)을 필드마다 해석·검증한다. 같은 In을 GET·POST 두 줄이 같이 쓴다. 본문이 깨졌으면(해석 실패) 400(D-116의 query와 같다), 모르는 키는 무시(D-116)
- ★ **사용자 검증**(D-15): 폼 타입의 메서드 `func (f *BoardForm) Validate(ctx context.Context) error`가 있으면 어댑터가 부른다. **모든 필드가 통과했을 때만**(Django `clean()`은 늘 부르지만 그러면 사용자가 필드마다 유효한지 다시 봐야 한다). 검증 실패는 필드의 `Error`를 쓰거나 폼 전체 오류 `f.AddError(msg)`(임베드한 `maago.Form`의 메서드)로 알리고, 돌려준 `error`는 검증이 아닌 실패(DB 오류 → 500). DB를 보는 검증은 같은 메서드가 `maago.DBFrom(ctx)`로(ctx에 DB가 있다, D-114). 시그니처가 다르면 생성 오류(go/types, 4번처럼)
- **뷰**: `in.Form.Valid()`(묶였고 오류가 없음). 실패면 뷰가 `maago.Show(page)`로 다시 그린다(200, D-116). 생성기는 흐름을 강제하지 않는다
- ★ **unique 위반 → 폼 오류**(열린 질문의 답 제안): 장치는 **위반 잡기 하나**. 검사 후 삽입은 경쟁에서 지고 쿼리가 하나 더다. 모델의 `unique`·복합 unique마다 `models_gen.go`에 값 `var BoardNameKey = maago.Unique{Table: "board", Columns: []string{"name"}}`(이름은 D-86 제약 이름의 CamelCase)이 생기고 `BoardNameKey.Violated(err) bool`. 런타임은 드라이버를 모르므로(D-90) 오류 글로 본다: PG `violates unique constraint "board_name_key"`, SQLite `UNIQUE constraint failed: board.name`(SQLite 글에는 제약 이름이 없어 테이블.컬럼들). 게시판 이름 중복은 뷰가 `InsertBoard`의 오류를 `BoardNameKey.Violated(err)`로 보고 `in.Form.Name.Error`를 쓴다. 대가: 검사가 뷰 코드에 있고(폼 정의가 아니라), 오류 글 형식에 기대므로 두 드라이버(modernc sqlite, pgx)를 `cmd/maago` 시험이 고정한다
- ★ **최종 사용자 문구**: 런타임 `maago.Messages` struct(필드마다 `Required`·`NotNumber`·`OutOfRange`·`TooLong`·`TooShort`·`NotChoice`·`BadBool`, `fmt` 자리 있는 영어 기본값). `Config.Messages`로 통째로 바꾸고 빈 필드는 기본값(한 언어, i18n은 D-72 밖). 어댑터는 `Handler`가 ctx에 넣은 표를 쓴다. 기각: `map[string]string`(키 오타를 컴파일러가 못 잡는다)
- **템플릿**: 생성되는 HTML 조각 없음(사용자가 그린다, 5번의 걷기가 `.Form.Name.Raw` 같은 접근을 검사한다: 제네릭 인스턴스의 필드도 `LookupFieldOrMethod`로 된다). 문서 예: `{{with .Form.Name}}<label for="{{.Name}}">{{.Label}}</label><input name="{{.Name}}" value="{{.Raw}}">{{if .Error}}<p>{{.Error}}</p>{{end}}{{end}}`, 폼 전체 오류는 `{{range .Form.Errors}}`

파일
- 루트 `form.go`(새): `Form`(`Errors`, `AddError`, `Valid`, 묶임 표시), `Field[T]`, `Messages`와 기본값, 필드 해석 도우미(`FormString`·`FormInt64`…: 생성 코드가 부른다, reflect 없음 D-66), `Unique`·`Violated`. `http.go`: `Config.Messages`를 ctx로
- `internal/web/forms.go`(새): 폼 struct 찾기·태그 검사(모델 폼은 `internal/model`의 필드 정보로), In의 `form` 소스. `views.go`: 셋째 소스. `typecheck.go`: `Validate` 시그니처. 생성 틀 `views.go.tmpl`: 폼 묶기와 검증(폼마다 `bindBoardForm(r, &f) error`를 한 번)
- 모델 생성(`internal/mapper` 또는 모델 생성 자리): `XxxKey` 값
- 시험: `internal/web/forms_test.go`(오류 표 골든), 루트 `form_test.go`(해석 도우미의 세 결과, 문구 바꾸기), `cmd/maago/testdata/web`에 폼 페이지와 unique 모델(두 방언에서 `Violated`)

하지 않기로 한 것
- `time.Time` 칸(11번: 표시 타임존과 함께 `datetime-local`), 파일 칸·multipart(M7), 폼 HTML 생성, 폼에서 입력 struct로 옮기는 생성 메서드, API 폼(7번이 JSON 본문으로, 422의 `errors`)

단계
1. 런타임 `Field`·`Form`·도우미·`Messages` → 확인: `int64` 빈 값·`abc`·범위 밖의 (값, 있음, 오류)가 셋 다 다름(쌍 비교), 문자열 필수·길이·선택지, `bool` 체크박스, 문구 바꾸기
2. 폼 struct 읽기와 태그 검사(모델 폼 포함) → 확인: 모르는 태그(did you mean), 지원하지 않는 T, 모델에 없는 필드·타입 다름·모델 폼 필드의 태그, `bool`에 `optional`, 숫자 아닌 칸의 `min`
3. In의 `form` 소스와 어댑터 → 확인: GET은 묶지 않음, POST는 묶고 검증, 깨진 본문 400, `Validate` 시그니처 오류, 필드가 하나라도 틀리면 `Validate`를 부르지 않음
4. `XxxKey`와 `Violated` → 확인: 두 방언에서 실제 위반을 잡고 다른 제약·다른 오류는 아님(PG는 `MAAGO_TEST_PG_URL`)
5. 종단(`cmd/maago/testdata/web`) → 확인: 다시 그린 폼이 입력값을 유지, 사용자 메서드의 오류, 이름 중복이 그 필드의 오류, 200
6. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **DB를 보는 검증은 뷰가 쿼리로**(사람의 결정 2026-10-01, fable 1·2): `in.Form.Valid()` 뒤 뷰가 SQL 쿼리(M3 쿼리 파일)로 보고 `Field.Error`·`AddError`를 쓴다. 게시판 이름 중복도, 공지 기간 겹침도(DB 제약이 없다), 글 고치기에서 자기 ID를 빼는 것도(path 값은 뷰만 안다) 같은 장치다. 경쟁에서 지면 unique 제약이 막아 500. `maago.Unique`·`Violated`·`XxxKey`는 만들지 않는다(드라이버 오류 글을 읽지 않는다, D-90). 12번의 "게시판 이름 중복(DB를 보는 검증)"과 그대로 맞는다
- **`Validate`는 순수**(fable 2): `func (f *BoardForm) Validate()`, 반환 없음. DB를 보지 않는 폼 전체 검증(시작 < 끝, 과거 시각 막기)만. 필드가 모두 통과했을 때만 부르고, 필드의 `Error`나 `AddError`로 알린다. 메서드는 `*BoardForm`의 메서드 집합에서 찾는다(codex 7). 시그니처가 다르면 생성 오류
- **폼 모양은 `Field[T]`**(사람의 결정, fable 7). TUTORIAL-FUTURE §9는 13번이 고친다
- **모델 폼은 `optional`만**(사람의 결정, fable 4·5): 모델 폼 필드의 태그는 `optional` 하나(폼의 관심사), 나머지 규칙은 모델에서. 모델 수준 검증 메서드(D-15)는 admin이 필요한 M6에서 정한다. 모델 폼에 `ID`·`auto=` 필드는 오류(D-102). `default`가 있는 필드도 폼에서는 기본이 필수다(빈 칸 = 기본값으로 하려면 `optional`을 쓰고 뷰가 `Opt`로 옮긴다)
- **숫자 범위는 `min_value`·`max_value`**(사람의 결정, fable 6, PHILOSOPHY 4): `min`·`max`는 문자열 글자 수만(모델의 `max`와 같은 뜻). 숫자의 `OutOfRange`는 타입 범위(`strconv`)와 `min_value`·`max_value` 둘 다
- **시각 칸은 6번에서**(fable 3): `Field[time.Time]`은 `datetime-local`(`2006-01-02T15:04`)을 `Config.TimeZone`(기본 UTC, ctx로)에서 `time.ParseInLocation`. 표시 함수는 11번 그대로
- **`Set` → `Given`**(fable 10: `maago.Set`과 헷갈림)
- **값 표**(codex 3): 키 없음과 빈 값(`x=`)은 같다: `Raw ""`, `Given false`, 필수면 `Required`, optional이면 영값·오류 없음. 해석 실패: `Raw`는 받은 글, `Given true`, `Value` 영값, 오류. 공백은 다듬지 않는다
- **체크박스**(codex 2): `bool`은 없음·빈 값·`false`·`off`·`0` → false, `on`·`true`·`1` → true, 그 밖은 `BadBool`. `optional`을 쓰면 오류
- **본문만**(codex 1, fable 8): 묶을 때 `r.PostForm`만 본다(query는 섞지 않는다). `Content-Type`이 `application/x-www-form-urlencoded`가 아니면 400(multipart는 M7이 연다), 깨진 본문 400, 같은 키 둘 400(D-116의 query와 같다). GET·HEAD는 묶지 않는다: `?name=x`가 와도 `Given false`, 오류 없음, `Valid()` false
- **순서**(codex 5·7): 폼 struct·태그 검사는 `go/ast` 단계(뷰 In처럼, `maago.Field[X]`의 X는 닫힌 목록), `Validate`는 `go/types` 단계, 묶는 코드는 타입 검사 뒤 `views_gen.go`. 생성 코드는 필드를 묶고 → 필드 오류가 없으면 `Validate` → `maago.FinishForm`이 유효 여부를 고정한다(`Valid()`는 그것과 폼 오류를 본다)
- **문구·시간대는 ctx로**(codex 13, fable 9): `Handler`가 `Config.Messages`(빈 필드는 기본값으로 채운다)와 `TimeZone`을 ctx에 넣고, 없으면(`WithDB`만 쓴 시험) 영어 기본값과 UTC. panic하지 않는다
- 확인 더함: 세 결과를 **HTTP POST로도**(종단), 틀린 `Content-Type`·같은 키 둘 400, GET `?name=x`, 한 필드의 오류에도 다른 필드의 `Raw` 유지, `AddError`가 `.Form.Errors`로 그려짐, `Raw`의 `<`·`"`·`&`가 이스케이프됨, In에 `form` 둘, 다른 패키지의 폼 타입, 모델 폼의 `ID`·`auto`·타입 다름·태그, 두 폼의 오류가 한 번에(D-61), `Validate`가 필드 오류가 있으면 불리지 않음, `optional` 숫자의 빈 칸, `min_value`·`max_value`
- 받지 않음: codex 9·10·11·fable 8의 unique 쪽(장치를 만들지 않는다), codex 8(`Validate`가 DB를 보지 않아 사라짐), codex 12의 textarea·select 종단(그리는 것은 사용자 HTML이고 5번의 걷기가 본다. 이스케이프 하나만 둔다), fable 10의 `Fill`(12번이 필요하면)

구현 리뷰 (ponytail·codex·fable)
- 만들며 고친 것: D-81이 옵션 이름에 `_`를 막아 `min_value`가 부딪혔다 → 사람의 결정으로 D-81을 넓힘(소문자 단어를 `_`로, `min__value`·`_min`·`min_`은 오류). `{{if .Form.Valid}}`가 포인터 리시버라 5번의 걷기가 막았다 → `Valid`·`Bound`는 값 리시버. 변이 일곱(GET도 묶음, query 섞기, `Content-Type` 검사 없음, 늘 묶임, 정수 범위, `Validate`를 부르지 않음, 필드 오류 무시)이 모두 잡힌다
- ponytail(스스로): 받음 — 문구 기본값을 `cmp.Or`로, 한 번 쓰는 `fieldsOf` 인라인, `Forms`의 겹친 조건
- codex(샌드박스가 쓰기를 막아 코드만 읽음): 받지 않음 — 1 `Validate` 뒤 `ok()`를 다시 보지 않는다(클로저라 `FinishForm`에서 다시 부른다. 대신 `Validate`가 필드 오류를 쓰는 종단 경우를 더했고, 미리 계산하는 변이가 잡힌다), 2 nullable 모델 필드를 자동 optional로(NULL 가능한 컬럼도 폼에서는 필수일 수 있다. Django의 `null`·`blank`처럼 폼이 `optional`로 정한다)
- fable: 받음 — `maago.Form` 태그에서 `model=` 앞의 다른 키로 gen이 panic(1), 값 리시버 `Validate`는 사본에 오류를 써 조용히 통과(2: 생성 오류), 폼이 읽지 않는 키의 중복도 400이던 것(3: `FormValues(r, 키들…)`이 폼의 키만 본다, D-116), DELETE 라우트의 폼은 Go가 본문을 읽지 않아 모두 Required(4: 생성 오류), 계획 리뷰 절이 4번 아래에 들어가 있던 것과 낡은 6번 머리(5: 옮기고 지움), 문구 셋(7: "is not written as the key takes it", 모델 폼 체크박스의 `optional`, 포인터 폼 필드), 시험 더함(8: 모델 폼의 `ID`, optional 숫자의 빈 값, 폼 밖 키의 중복, 값 리시버, `model=` 앞의 오타), `Config.Messages`의 verb가 맞지 않으면 `Handler` 오류(10). 12번에 넘김 — 6 픽스처의 날 SQL(`?`는 SQLite만): 튜토리얼은 쿼리 파일로, 9 고치기 폼을 채우는 `Fill`(시각 칸의 `Raw`는 `Config.TimeZone`의 `2006-01-02T15:04`)


## 7. API 라우트 (D-44)

- JSON 응답, 요청 본문 JSON을 In으로(`encoding/json`, D-66 범위 밖), 오류는 RFC 9457(폼 오류는 `errors` 확장 멤버)

→ 확인: 골든, 잘못된 JSON 400 problem, 없는 글 404 problem, 검증 실패 422 problem

### 계획

있는 것(3·4번): API 라우트(`maago.API`)의 뷰는 `func(ctx, In) (P, error)`이고 P를 `maago.JSON`으로 쓴다. 오류는 RFC 9457(`type`·`title`·`status`, 4xx면 `detail`). 이 작업은 요청 본문과 422를 더한다.

모양 (결정에 없는 선택은 ★)
- ★ **본문은 폼으로**: API 라우트의 In도 `maago:"form"` 필드(D-119)로 본문을 받는다. 다른 것은 읽는 글뿐이다: `Content-Type: application/json`의 JSON 객체 하나, 값은 문자열·숫자·`true`/`false`·`null`(키 없음과 같다). 그 글을 `Raw`로 삼아 HTML 폼과 같은 `Bind*`가 해석·검증한다(숫자 `1e3`·문자열 `"12"`도 `Raw`가 같으면 같은 결과, 검증 규칙과 문구가 한 벌). 시각은 RFC 3339(오프셋 있음)도 받는다. 깨진 JSON·객체가 아님·값이 배열이나 객체·틀린 `Content-Type`·같은 키 둘은 400 problem(D-116). 모르는 키는 무시. 기각: 아무 struct를 `encoding/json`으로 받는 `maago:"json"`(검증이 따로 생기고, `int64` 칸의 빈 값·`null`·`"abc"`가 `encoding/json` 오류 하나로 뭉친다: 완료 조건이 가르는 것을 API에서 잃는다). 중첩·배열 본문은 필요해지면 그때(탈출구 D-41로는 지금도 된다)
- ★ **422는 생성 코드가**: API 라우트에서 묶인 폼이 유효하지 않으면(필드·`Validate`) 어댑터가 뷰를 부르지 않고 422 problem을 쓴다. HTML은 뷰가 200으로 다시 그려야 해서 뷰를 부르지만, API에는 다시 그릴 것이 없다. 뷰가 DB를 보고 찾은 오류(D-119, 중복)는 `return P{}, maago.Invalid(in.Form.Title.Name, "that title is taken")`(필드 이름 `""`는 폼 전체)로 돌려준다: `Status`처럼 오류 값이고 422가 된다
- ★ **problem의 `errors`**: `{"type":"about:blank","title":"Unprocessable Entity","status":422,"errors":[{"field":"count","detail":"Enter a number."},{"detail":"that period overlaps"}]}`. 필드 순서는 폼의 선언 순서, 폼 전체 오류는 뒤에, 문구는 `Config.Messages`(D-119). RFC 9457의 확장 멤버
- GET API 라우트의 폼은 HTML처럼 묶지 않는다(빈 폼, 뷰를 부른다)

파일
- 루트 `form.go`: JSON 본문을 `url.Values`로(`FormValues`가 라우트의 API 여부로 고른다: 생성 코드가 `maago.APIFormValues`를 부른다), `Invalid`, 필드 오류 목록 도우미. `http.go`의 `fail`: `errors` 멤버. `BindTime`: RFC 3339
- `internal/web/templates/views.go.tmpl`: API 라우트의 폼은 JSON으로 묶고, 유효하지 않으면 `maago.Invalid…`로 422
- 시험: 루트(JSON 해석의 400들, `null`, 숫자·문자열, `Invalid`의 problem), `cmd/maago/testdata/web`에 API 폼 라우트(골든과 종단)

하지 않기로 한 것
- 아무 struct의 JSON 본문, 중첩·배열, multipart(M7), OpenAPI 문서

단계
1. 런타임: JSON 본문 → 값, `Invalid`, problem의 `errors` → 확인: 잘못된 JSON·배열 값·틀린 Content-Type·같은 키 둘 400, `null`은 없음, `Invalid`가 422와 `errors`
2. 생성: API 폼의 어댑터 → 확인: 골든, 유효하지 않으면 뷰를 부르지 않음
3. 종단 → 확인: 잘못된 JSON 400 problem, 없는 글 404 problem, 검증 실패 422 problem(`int64` 칸의 세 결과가 `errors`에서도 다름), 뷰의 `Invalid` 422, 성공 JSON
4. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-01)**: API 본문은 폼으로만, 본문 형식은 라우트가 정함(HTML은 urlencoded, API는 JSON, 다른 것은 400), 유효하지 않은 API 폼은 생성 코드가 뷰를 부르지 않고 422, 뷰가 찾은 오류는 `maago.Invalid`, `errors`는 `field`·`detail` 배열. D-120
- **묶기를 둘로 가른다**(fable 1): 생성된 `bindXxx(r, vals, f)`는 값을 받아 필드만 채운다. 값은 어댑터가 라우트마다 `maago.FormValues`(HTML) 또는 `maago.JSONValues`(API)로 읽는다. 같은 폼을 HTML POST와 API POST가 함께 쓸 수 있다
- **JSON 읽기**(codex 1~7, fable 2·3): `json.Decoder`의 `Token`으로 객체를 열고 키마다 값을 `json.RawMessage`로 받는다. `Raw`는 쓴 그대로: 문자열은 풀린 글, 숫자는 리터럴 글(`1e3`은 `int64` 칸에 `NotNumber`, `float64`는 받는다. 큰 정수가 `float64`를 거치지 않는다), `true`/`false`, 멤버 `null`은 키 없음. 400: 빈 본문, 객체가 아님(`[]`·`"x"`·`1`·`null`), 폼의 키 값이 배열·객체, 폼의 키 둘, 객체 뒤의 글(`{}{}`), 깨진 JSON, `Content-Type`이 `application/json`(매개변수 허용)이 아님, 10MB 넘음(`ParseForm`과 같은 한도). 모르는 키는 값이 무엇이든 무시(D-116)
- **시각**(codex 8): `BindTime`은 `datetime-local` 둘에 RFC 3339(오프셋 있음)를 더한다. HTML 폼에도 같다
- **422**(codex 9·10, fable 5): `maago.Invalid(&in.Form.Title, msg)`(필드를 넘겨 이름 오타가 없다)과 `maago.InvalidForm(msg)`는 422 `Status`이고, 여럿은 `errors.Join`. `fail`이 오류 나무를 걸어 모두 모은다. 422의 `detail`은 없고 `errors`만. HTML 라우트에서 돌려주면 글 422(문서에 적는다). 대가: DB 검증의 장치가 HTML(`Field.Error`)과 API(`Invalid`) 둘이다: 뷰는 `in.Form`을 값으로 받아 어댑터가 뷰의 쓰기를 보지 못한다
- **생성 코드의 422**(fable 5): 폼마다 `invalidXxx(f) error`가 `maago.Unprocessable([]maago.FieldError{{Field: f.Title.Name, Detail: f.Title.Error}, …}, f.Errors)`(선언 순서, 빈 것은 뺀다, 폼 전체는 끝에 `field` 없이). reflect 없음(D-66)
- **GET·HEAD API 라우트의 폼은 생성 오류**(fable 4): 본문이 없다. DELETE 검사는 API에도 그대로("DELETE는 path로")
- 확인 더함: 위 400들 하나씩, `12`와 `"12"`가 같은 결과, `charset` 매개변수, 422의 `Content-Type: application/problem+json`, `errors` 순서와 폼 전체가 끝, 바꾼 `Config.Messages`가 `errors`에, `Validate`가 API에서도 돈다, 뷰를 부른 횟수(유효하지 않으면 0), `errors.Join`의 두 `Invalid`, 400·404·500에는 `errors`가 없음
- 12번에 넘김(fable 8): API로 만든 것은 200과 그 객체(201·`Location`은 지금 모양으로 못 한다), PATCH식 부분 수정은 `optional`이나 PUT
- 받지 않음: 없음

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: 변이 넷(422를 건너뜀, 숫자를 `float64`로, 폼 키 둘 허용, 객체 뒤 글 허용)이 모두 잡힌다. 첫 리뷰 둘은 주간 사용 한도로 끊겨 다시 돌렸다
- ponytail(스스로): 지울 것 없음. 알고 둔 것: 뷰가 `Invalid`와 DB 오류를 `errors.Join`하면 422가 된다(`status`가 첫 `Status`를 본다). 뷰가 그렇게 묶을 까닭이 없어 그대로
- codex(샌드박스가 쓰기를 막아 코드만 읽음): 받지 않음 — 1 `1e-999`가 `ErrRange`와 0이라 조용히 받는다: Go의 `ParseFloat`는 언더플로에 오류를 내지 않는다(돌려 봄). `0.1`처럼 반올림이고 숨는 실패가 아니다
- fable(변이 16개 중 14 잡힘): 받음 — HTML 라우트의 `Invalid`는 글 422임을 D-120에(1), `invalidXxx` 이름 겹침 시험(2: 사용자가 `invalidNoticeForm`을 선언하면 생성 오류), API 분기의 죽은 `vals != nil`(3), DELETE 문구를 방법 기준으로(4), `JSONValues` 주석(5), `{"count":0}`이 주어진 0(관찰). 12번에 넘김: 체크박스 문구가 API problem에도 나옴, API 클라이언트는 정수를 정수로(`1.0`·`1e3`은 `NotNumber`), 폼이 있는 라우트에 본문 없이 POST하면 400(`Content-Type ""`), JSON을 HTML 폼 라우트에 보내면 글 400




## 8. 표준 핸들러 탈출구와 스트리밍 (D-41)

- `func(w, r)` 뷰도 같은 생성자로 표에 올린다(2번. 이름·URL 함수는 같다, 템플릿 검사 없음). 스트리밍 응답(CSV)이 되는지

→ 확인: 탈출구 라우트의 URL 함수와 `{{url}}`. 스트리밍은 `httptest.NewServer`로 열고, 핸들러가 둘째 조각을 쓰기 전에 클라이언트가 첫 조각을 읽는다(채널로 맞춘다. `ResponseRecorder`로는 보이지 않는다)

### 계획

있는 것: 2번이 `func(w, r)` 뷰를 같은 생성자로 받고, 4번이 그 어댑터(`View(w, r); return nil`)와 URL 함수(와일드카드는 `string`, In이 없다)를 만들고, 3번의 `written`이 `http.Flusher`다(`ResponseController`도 `Unwrap`으로 닿는다). 이 작업은 새 코드가 아니라 **되는지 보이는 시험**이다.

파일
- `cmd/maago/testdata/web`: 탈출구 라우트 `maago.GET("/export/{format}", "post_export", PostExport)`. `PostExport`는 CSV 머리 줄을 쓰고 `Flush`, 시험이 준 문(패키지 변수 채널, 운영에서는 nil이라 기다리지 않는다)을 기다린 뒤 둘째 줄을 쓴다. `number_page.html`의 `{{url}}` 목록에 `{{url "post_export" "csv"}}`
- `site_test.go`: `httptest.NewServer`로 열어 GET, 첫 줄을 읽은 **뒤에** 문을 연다(둘째 조각 전에 첫 조각이 왔음), 끝까지 읽어 두 줄. `{{url}}` == `PostExportURL("csv")`

하지 않기로 한 것
- 런타임·생성기 변경(필요하면 그때 계획을 고친다), 탈출구 뷰의 CSRF·오류 보고 시험(3번에 있다)

단계
1. 픽스처 라우트와 시험 → 확인: 위 순서가 지켜지지 않으면(예: `Flush`를 지우면) 시험이 시간 안에 끝나지 않고 실패한다(변이로 본다. 읽기에 마감을 둔다)
2. 두 모듈 `go vet`·`go test`, `gofmt -l .`

구현 리뷰 생략: 런타임·생성기에 분기가 생기지 않고 시험 픽스처만 바뀐다(WORKFLOW 6의 작은 작업). 계획 리뷰와 ponytail은 한다

계획 리뷰 (codex·fable) — 위 계획을 이렇게 고친다
- **nil 채널은 영원히 기다린다**(fable 1): 뷰는 `if exportGate != nil`일 때만 기다리고, 그때도 `r.Context().Done()`과 함께(codex 2)
- **실패해도 멈추지 않게**(codex 1·2·3, fable 2): 요청 전체에 마감(`NewRequestWithContext`, 헤더를 기다리는 동안도), 문은 모든 출구에서 연다(`sync.OnceFunc`, `t.Cleanup`이 `Server.Close`보다 먼저), 시험이 끝나면 훅을 nil로
- **문은 첫 줄을 읽은 뒤에만 연다**(codex 6): `ReadString('\n')`이 돌아온 뒤. 헤더가 왔다는 것만으로는 열지 않는다
- **런타임을 잡는 변이**(fable 3, codex 4): 뷰는 `w.(http.Flusher).Flush()`를 부른다(`ResponseController`는 `Unwrap`으로 안쪽 writer에 닿아 `written.Flush`를 지워도 지나간다). 변이 둘: 뷰의 `Flush`를 지움, `http.go`의 `written.Flush`를 지움 → 둘 다 실패해야 한다. 결과는 아래 구현 리뷰 자리에 적는다(생략한 구현 리뷰를 대신한다). "`ResponseController`도 `Unwrap`으로 닿는다"는 틀린 이유였다: `written`의 `Flush`가 먼저 잡힌다
- **첫 조각 뒤의 panic**(codex 5): 상태 200 그대로, 본문은 첫 줄만, `OnError`가 한 번 듣는다
- **DB와 머리**(fable 6): 뷰가 `maago.DBFrom(r.Context())`로 행 수를 읽고, `Content-Type: text/csv`·`Content-Disposition`을 첫 `Write` 전에 정한다. `TestRequests`의 `{{url}}` 목록에 `PostExportURL("csv")`
- 12번에 넘김(fable 5): 탈출구 뷰는 오류를 돌려줄 수 없어, 중간 오류(`rows.Err()`)에 좋은 길이 없다: panic은 `OnError`가 듣지만 잘린 CSV가 정상 끝처럼 닫히고, `panic(http.ErrAbortHandler)`는 연결을 끊지만 `OnError`가 듣지 못한다. 12번이 정한다(예: 보고한 뒤 끊기, 작은 `maago.Report`). `csv.Writer.Flush`와 `http.Flusher.Flush`는 다르다
- 받지 않음: 없음

구현 리뷰 (생략, 위 사유) — 대신 변이 기록
- 뷰의 `w.(http.Flusher).Flush()`를 지움 → `TestExportStreams`가 5초 마감에 실패(멈추지 않음)
- `http.go`의 `written.Flush`가 안쪽 writer를 flush하지 않게 함 → 같은 실패. 런타임의 `Flush`를 이 시험이 지킨다
- ponytail(스스로): 런타임·생성기 변경 없음, 픽스처 뷰 하나와 시험 하나




## 9. 현재 사용자와 인증 인터페이스

- `Config.Auth`(인터페이스 하나: 요청 → 사용자 또는 없음), 뷰가 `maago.UserFrom(ctx)`로 얻음, 라우트 표에서 로그인 필수 선언(D-115), 실패 응답: HTML은 `Config`의 로그인 URL로 `next`와 함께 303, API는 401 problem. 세션·토큰은 M5. M5에 넘길 것: `next`를 쓸 때 같은 출처의 경로인지 검증(열린 이동 막기)

→ 확인: 가짜 인증으로 로그인 필수 HTML 뷰 → 로그인 URL로 303(`next` 포함), API → 401 problem, 로그인한 요청은 통과

### 계획

모양 (결정에 없는 선택은 ★)
- ★ **인증은 함수 하나**: `Config.Auth func(r *http.Request) (any, error)`. 돌려준 값이 사용자이고, `nil`이면 로그인하지 않음, `error`면 인증을 볼 수 없음(세션 저장소가 죽음 등: 500과 `OnError`). 인터페이스(메서드 하나)보다 함수가 짧고, 시험의 가짜 인증이 한 줄이다. M5의 세션·토큰이 이 함수를 준다
- ★ **사용자 타입은 코어가 모른다**(D-51 "코어는 인터페이스만"): `maago.UserFrom[U any](ctx) (U, bool)`가 ctx의 값을 U로 꺼낸다(없거나 타입이 다르면 `false`). 코어가 `User` 인터페이스를 정하면(예: `UserID() int64`) contrib User와 사용자 모델에 메서드를 강제한다(모델의 `ID` 필드와 같은 이름 `ID()`는 Go에서 못 쓴다). 기각: `UserFrom(ctx) any`(뷰마다 타입 단언), 코어의 `User` 인터페이스
- **요청마다 한 번**: `Handler`의 `serve`가 라우트 앞에서 `Auth`를 부르고 값을 ctx에 넣는다(로그인 필수가 아닌 뷰도 "로그인했다면 누구"를 안다). `Auth`가 nil이면 부르지 않는다(모두 로그인하지 않음)
- **로그인 필수**(D-115의 `maago.LoginRequired`): 생성된 `AppRoute`에 `LoginRequired bool`. 사용자가 없으면 HTML은 `Config.LoginURL`로 303, `next`에 요청의 경로와 query(`r.URL.RequestURI()`, `url.Values`로 붙임). API는 401 problem(`detail` "login required"). 뷰는 부르지 않는다
- ★ **잘못된 설정은 `Handler` 오류**(D-114): 로그인 필수 라우트가 있는데 `Config.Auth`가 nil이면 오류("어떤 요청도 들어갈 수 없다"), HTML 로그인 필수 라우트가 있는데 `LoginURL`이 비면 오류. 조용히 모두 막거나 모두 통과시키지 않는다(PHILOSOPHY 7)
- **`WWW-Authenticate`**: RFC 9110은 401에 이 머리를 요구하지만 스킴은 M5(세션·토큰)가 정한다. M4는 `Config.AuthChallenge string`(예: `Bearer`)을 두지 않고, 401에 머리 없이 낸다고 적고 M5에 넘긴다
- **CSRF와의 순서**: 교차 출처 검사(3번)가 먼저, 인증은 그 뒤(교차 출처 POST가 로그인 화면으로 가지 않고 403)

파일
- 루트 `http.go`: `Config.Auth`·`LoginURL`, `AppRoute.LoginRequired`, `serve`에서 인증과 로그인 필수, `Handler`의 설정 검사. 새 `UserFrom`, `WithUser`(뷰를 HTTP 없이 시험할 때, `WithDB`처럼)
- `internal/web/templates/urls.go.tmpl`·`gen.go`: `LoginRequired: true`
- 시험: `http_test.go`(가짜 인증으로 303·`next`·401 problem·통과·`Auth` 오류 500·설정 오류 둘·`UserFrom` 타입), `cmd/maago/testdata/web`에 로그인 필수 HTML·API 라우트 하나씩

하지 않기로 한 것
- 세션·토큰·로그인 화면(M5), `next` 검증(열린 이동, M5가 로그인 뒤 이동할 때), 권한·역할

단계
1. 런타임 → 확인: 위 시험들, `next`가 query까지 담고 이스케이프됨
2. 생성 → 확인: `urls_gen` 골든에 `LoginRequired`
3. 종단 → 확인: 픽스처의 로그인 필수 HTML 303·API 401·로그인한 요청 200과 `UserFrom`으로 본 사용자
4. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-04)**: `UserFrom[U]`(D-51 문구를 고침), `Auth`는 요청마다 한 번(리뷰어가 갈림: codex는 필요할 때만, fable은 늘), 로그인하지 않은 HTML 요청은 GET·HEAD만 로그인 URL로 303이고 나머지는 401, `WWW-Authenticate`는 M5. D-121
- **`Auth`의 오류는 `status` 규칙으로**(fable 1): `Status(401, …)`(토큰이 틀림)은 401, 그 밖은 500과 `OnError`. M5의 토큰 인증이 서명을 바꾸지 않고 들어온다
- **`UserFrom`**(fable 2, codex 3): ctx에 없으면 `(영값, false)`, 있는데 U가 아니면 panic(`DBFrom`처럼, 설정과 뷰가 어긋난 것). 비교는 타입 단언 그대로(`User` 값과 `*User`는 다르다, U가 인터페이스면 그것을 구현한 값). `Auth`가 `(*User)(nil)`을 돌려주면 로그인한 것이 된다: 문서에 적는다(reflect 없이 가를 수 없다, PHILOSOPHY 1)
- **401**(fable 10): API·HTML 모두 있는 `fail` 길(`detail` "login required"), `OnError`는 부르지 않는다
- **`next`**(codex 4): `r.URL.RequestURI()`(경로와 query). `LoginURL`에 query가 있으면 지키고 `next`를 더한다. 하위 경로(`/app`)는 11번이 `next`와 `LoginURL` 둘 다에 붙인다(11번에 넘김). `LoginURL`이 해석되지 않으면 `Handler` 오류
- **픽스처의 `Config`**(fable 4): `site_test.go`의 `handler`와 `main.go`가 `Auth`·`LoginURL`을 준다(로그인 필수 라우트가 생기면 `Handler`가 거절하므로)
- 확인 더함: 교차 출처 POST가 로그인 필수 HTML에 오면 303이 아니라 403(순서), HEAD도 막힘, preflight는 `Auth`를 부르지 않음, 공개 라우트: `Auth`가 nil이면 통과·`UserFrom` false, `Auth`가 있으면 `UserFrom`이 사용자, 공개 라우트에서 `Auth` 오류는 500, `Status(401)` 오류는 401, POST 미로그인 401, `UserFrom` 타입이 다르면 panic·포인터와 값·인터페이스 U, `LoginURL`의 query 지킴, 변이(검사를 지우면 종단 303·401이 깨짐)
- 받지 않음: codex 1의 "필요할 때만"(사람의 결정), codex 5의 머리(사람의 결정, M5)
- M5에 넘김: `next`가 같은 출처의 경로인지 검증, 401의 `WWW-Authenticate`, `func(r)`인 `Auth`는 쿠키를 쓸 수 없다(읽을 때 세션을 늘리려면 D-46 미들웨어)

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: 변이 셋(로그인 필수 검사를 지움, 생성기가 `LoginRequired`를 뺌, `Auth`의 사용자를 ctx에 넣지 않음)이 모두 잡힌다
- ponytail(스스로): 지울 것 없음(fable 6의 두 번째 루프는 아래에서 합침)
- codex(샌드박스가 쓰기를 막아 코드만 읽음): 지적 없음(`Auth` panic·오류, 401의 `OnError` 제외, 절대 `LoginURL`, `next` 이스케이프, API만 있는 앱, HEAD, CORS 머리, typed nil을 확인)
- fable(변이 다섯 모두 잡힘): 받음 — `Auth` 주석에 "없거나 끝난 세션은 nil(`sql.ErrNoRows`는 404, `Status(401)`은 로그인 화면으로 보내지 않는다)"와 `DBFrom(r.Context())`(1·3), 상대 `LoginURL`은 페이지마다 달라져 `Handler` 오류(2: `/`로 시작하거나 절대 URL. 11번의 접두어는 경로에만), 시험 더함(3·4: `Auth`가 DB를 봄, `Auth` 없는 공개 라우트, `Auth`의 panic은 500), MILESTONES의 D-51 문구(5), 두 번째 루프를 라우트 루프에 합침(6). 받지 않음 — 2의 선택(`LoginURL`이 로그인 필수 라우트면 거절): 이동 고리는 설정을 바꾸면 바로 보인다. 5의 M5 넘김을 MILESTONES에 옮기기: TASKS-M5 초안이 이 절에서 가져간다




## 10. 서비스 시험 도구 (`maagotest`, D-113)

- `maagotest.DB(t)`, 두 방언(D-113). 자체 시험은 드라이버가 필요해 `cmd/maago`에(D-90)
- 게시판 튜토리얼 2~7단계의 시험(절대 경로 `MAAGO_DATABASE_URL` 우회)을 `maagotest`로 다시 쓴다: `steps.sh`로 풀어 rebase, `examples/board/README.md`, `TestBoardTutorialCatches`, D-103의 문구(13번에서)

→ 확인: 패키지 디렉터리에서 돌린 시험이 루트의 `.env`를 찾음, 시험마다 새 DB(한 시험의 쓰기가 다른 시험에 안 보임), PG 풀의 연결 여럿이 모두 그 스키마를 봄, 끝나면 스키마가 없음, 남은 스키마를 다음 실행이 지움, 마이그레이션이 적용됨

### 계획

D-113이 모양을 정했다. 이 계획은 그것을 코드와 시험으로 옮긴다.

모양 (결정에 없는 선택은 ★)
- **패키지** `github.com/maago-web/maago/maagotest`(루트 모듈, `testing`을 import하는 유일한 비시험 패키지). `func DB(t testing.TB) *sql.DB`. 드라이버는 앱의 시험 파일이 import한다(D-90, `OpenDB`와 같은 오류 문구)
- **주소**: 작업 디렉터리에서 `go.mod`까지 올라간 루트(`project.Root`), `MAAGO_DATABASE_URL`이 이기고 없으면 루트의 `.env`(`project.Database`, D-97의 규칙과 문구). 주소에서는 방언과 서버만 쓴다
- **SQLite**: `t.TempDir()`에 빈 파일을 만들고(D-101 `mode=rw`) `maago.OpenDB(DBConfig{URL: "sqlite:" + 그 경로})`
- ★ **PG 스키마 이름**: `maagotest_` + `sha256(작업 디렉터리 + "\x00" + t.Name())`의 앞 16 hex(소문자, 63바이트 한도 안). 관리 연결 하나로 `DROP SCHEMA IF EXISTS … CASCADE; CREATE SCHEMA …`, 풀은 주소에 `search_path=<스키마>`를 더해 연다(pgx가 시작 매개변수로 보내 풀의 연결마다 같다). `t.Cleanup`이 풀을 닫고 스키마를 지운다
- **마이그레이션**: 루트의 `migrations/`를 `migrate.ReadFiles` → `practicedb.OpenDatabase`(같은 주소) → `migrate.Run`. 오류는 진단 그대로 `t.Fatal`
- **오류는 `t.Fatal`**(돌려주는 값 없음): 시험 도구라 호출 자리가 줄어든다

파일
- 새 `maagotest/maagotest.go`
- 자체 시험은 드라이버가 필요해 `cmd/maago/maagotest_test.go`(D-90): 임시 프로젝트(`go.mod`, `.env`, `migrations/0001_initial.sql`, 하위 패키지 디렉터리)에서 `t.Chdir`로 하위 디렉터리에 들어가 `DB(t)`. PG는 `MAAGO_TEST_PG_URL`(D-91)
- `noreflect_test.go`의 허용: `maagotest`도 reflect를 쓰지 않는다(바꿀 것 없음, 확인만)
- 게시판 튜토리얼: `steps.sh unpack`으로 풀어 0002의 `tx(t)`를 `maagotest.DB(t)` 위의 트랜잭션 없는 도우미로 바꾸고(시험마다 새 DB라 롤백이 필요 없다) 뒤 단계의 충돌을 고쳐 `pack`. 0002 커밋 글의 `MAAGO_DATABASE_URL=sqlite:$PWD/board.db go test` 문단을 "그냥 `go test ./...`"로, `README.md`의 같은 줄, `cmd/maago/tutorial_test.go`의 절대 경로 환경 변수를 지운다. `TestBoardTutorialCatches`의 "gofmt" 경우가 가리키는 글(`+\tt.Helper()`)이 남는지 본다

하지 않기로 한 것
- 메일 확인(M5), 트랜잭션 롤백 방식(D-113 기각), D-103의 문구(13번)

단계
1. `maagotest` → 확인(작업 그대로): 패키지 디렉터리에서 돈 시험이 루트의 `.env`를 찾음, 시험마다 새 DB(두 하위 시험 중 하나의 쓰기가 다른 쪽에 안 보임), PG 풀의 연결 여럿(동시에 잡아 둔 연결 넷)이 모두 그 스키마를 봄(`current_schema()`), 끝나면 스키마가 없음, 같은 이름으로 남겨 둔 스키마를 다음 `DB`가 지움, 마이그레이션이 적용됨, `MAAGO_DATABASE_URL`이 `.env`를 이김, 드라이버 없음·`.env` 없음의 오류
2. 튜토리얼 → 확인: `TestBoardTutorial`·`TestBoardTutorialCatches`가 절대 경로 없이 통과
3. 두 모듈 `go vet`·`go test`(PG는 `MAAGO_TEST_PG_URL`로 한 번), `gofmt -l .`

계획 리뷰 (codex·fable) — 위 계획을 이렇게 고친다
- **스키마 이름**(codex 3, fable 2·10·13): D-113의 "패키지 경로"대로 `sha256(모듈 경로 + "/" + 루트에서 작업 디렉터리까지 + "\x00" + t.Name() + "\x00" + 이 시험에서 몇 번째 DB(t)인지)`의 앞 16 hex. 작업 디렉터리의 절대 경로는 실행마다 달라(임시 디렉터리) 남은 스키마를 이름으로 지우지 못하고, 부른 함수의 패키지는 사용자가 `DB`를 도우미 패키지에서 감싸면 모두 같아진다. 한 시험에서 두 번 부르면 서로 다른 DB(번호가 다르다)
- **PG 주소**(codex 1): 주소의 query를 해석해 `search_path`를 더한다(이미 있는 매개변수를 지킨다). 마이그레이션도 그 주소로(fable 1: 날 주소면 `public`에 들어간다)
- **정리**(codex 2, fable 11): 마이그레이션 연결은 쓰자마자 닫고, `t.Cleanup`은 풀을 닫은 뒤 30초 마감과 `lock_timeout`으로 스키마를 지운다. 시험이 트랜잭션·rows를 열어 두어 못 지우면 `t.Error`("the test left a transaction or rows open")로 알리고 멈추지 않는다
- **SQLite 두 연결**(codex 5): 마이그레이션(`practicedb`)과 풀(`OpenDB`, WAL·`busy_timeout`)이 같은 파일을 다른 설정으로 연다. 마이그레이션은 풀을 열기 전에 끝나고 닫히므로 겹치지 않는다(주석에 적는다)
- **자체 시험의 자리**(fable 3·14): 오류 길(드라이버 없음, `.env` 없음)은 루트 `maagotest/maagotest_test.go`(드라이버가 없는 모듈, `testing.TB`를 감싸 `Fatal`을 잡는다). 나머지는 `cmd/maago`. `t.Chdir`을 쓰는 시험은 병렬로 하지 않는다(codex 4, Go가 막는다)
- **확인을 고침**(fable 7·8·9): 새 DB는 부모의 `DB(t)`가 살아 있는 동안 하위 시험의 `DB(t)`가 비어 있고, 하위 시험이 끝난 뒤에도 부모의 행이 남아 있음. 남은 스키마는 시험이 같은 공식으로 이름을 계산해 표를 하나 둔 스키마를 먼저 만들고, 그 이름의 하위 시험에서 `DB(t)`가 빈 스키마를 줌. "환경 변수가 이김"의 `.env`는 형식이 맞되 다른 곳을 가리킴. 한 시험의 두 `DB(t)`가 다름
- **튜토리얼**(fable 12·15·16·17, codex 6): 0002의 `tx(t)` 도우미를 지우고 시험마다 `db := maagotest.DB(t)`와 `ctx := t.Context()`를 바로 쓴다(D-113의 API를 가르치는 자리). 0003~0007의 `tx` 호출도 고친다. `TestBoardTutorialCatches`의 "gofmt" 닻은 `+\tdb := maagotest.DB(t)`로. 0002 커밋 글·README·`tutorial_test.go`의 "단계가 DB 하나를 나눈다"를 "`maago migrate`의 DB와 앱 시험의 DB는 따로"로. 시험 파일의 `_ "modernc.org/sqlite"`는 남기고 이유를 적는다(D-90: 시험은 `main`을 거치지 않는다)
- 문서: TODO M3의 "절대 경로 환경 변수" 항목을 지운다(풀렸다). ARCHITECTURE §2의 "런타임이 기대는 내부 패키지" 문장에 `maagotest`(시험 바이너리만)가 `migrate`·`practicedb`를 끈다는 것은 13번 목록에
- 받지 않음: 없음

구현 리뷰 (ponytail·codex·fable)
- 만들며 고친 것: macOS에서 `project.Root`는 심볼릭 링크를 푼 경로(`/private/var/…`), `os.Getwd`는 풀지 않은 경로(`/var/…`)라 패키지 부분이 `../..`가 되었다. 작업 디렉터리도 풀어 맞춘다. "남은 스키마" 변이(먼저 지우지 않음)가 살아남아 찾았다. 튜토리얼은 rebase 대신 커밋마다 원래 트리에 같은 변환을 걸어 다시 만들었다(충돌 없음, 글·작성자·날짜 그대로). 변이: `search_path` 없음, 끝에 지우지 않음, 한 시험의 둘째 DB가 같은 이름, SQLite 파일 공유, 남은 스키마를 지우지 않음, 이름이 절대 경로, 주소의 query를 버림 — 모두 잡힌다
- ponytail(스스로, fable 6·8과 같다): 빈 SQLite 파일 만들기(마이그레이션이 만든다)와 시험의 `MAAGO_DATABASE_URL` 거르기(`walk`가 이미 지운다)를 지웠다
- codex(샌드박스가 빌드를 막아 코드만 읽음): 받음 — `t.Fatal`은 시험의 고루틴에서만: `DB`의 문서에 적었다(1). 반환값을 오류로 바꾸는 것은 받지 않음: 시험 도구라 호출 자리가 줄어드는 것이 D-113의 모양이다
- fable(PG로 돌림): 받음 — 드라이버가 없을 때 "maago를 다시 빌드하라"던 문구를 "이 패키지의 _test.go 파일에서 import"로(1, 고친 줄까지 시험), "남은 스키마" 시험이 이름이 달라도 지나가던 것(2: 하위 시험의 `current_schema()`가 계산한 이름과 같아야 한다), 튜토리얼은 두 줄(`db := maagotest.DB(t)`, `ctx := t.Context()`, 3), 루트의 패키지는 `path.Join`(4), 주소의 query를 지키는 시험(5: `application_name`이 풀의 연결마다), `go.mod` 진단을 그대로(7)




## 11. 작은 할일

- 하위 경로(`Config.Prefix`: URL 함수·`{{url}}`·`SeeOther`·로그인 이동에 붙는다), 표시 타임존과 템플릿 시각 함수, 오류 보고 훅의 예시, 응답 뒤 작업의 문서(D-118)

→ 확인: `/app` 아래에서 `{{url}}`·`SeeOther`·로그인 이동이 접두어를 가짐, 시각 표시가 `Config.TimeZone`을 따름, `OnError`가 뷰 오류와 panic을 받음

### 계획

넷 중 둘(오류 보고 훅의 예시, D-118 문서)은 문서다. 하위 경로와 표시 타임존은 같은 물음을 공유한다: 생성된 `XxxURL`과 템플릿의 `FuncMap`은 패키지 수준의 값이라 요청 ctx가 닿지 않는다.

모양 (결정에 없는 선택은 ★)
- ★ **하위 경로의 뜻**: 앞단 프록시가 `/app`을 **떼고** 넘긴다(Django `FORCE_SCRIPT_NAME`, WSGI `SCRIPT_NAME`과 같다). 라우트는 그대로 `/posts/{id}`에 등록되고, 바깥으로 나가는 URL(`XxxURL`, `{{url}}`, `SeeOther`, 로그인 이동, `next`)에만 `Config.Prefix`가 붙는다. 기각 후보: 앱이 `/app/…`을 받는다(프록시가 떼지 않음): 라우트 등록과 URL이 모두 바뀌고, 앞단이 떼는 배치와 둘 다 지원하면 설정이 둘이 된다
- ★ **설정이 닿는 법**: `Handler`가 `Config.Prefix`와 `TimeZone`을 프로세스의 값으로 둔다(런타임 패키지 변수, `Handler`가 한 번 쓴다). `XxxURL()`은 `maago.Prefix() + "/posts/…"`. 대가: 한 프로세스에 설정이 다른 `Handler` 둘을 두면 `Handler`가 오류("Prefix가 다른 Handler가 이미 있다"), `Handler` 없이 부른 `XxxURL`(시험)은 접두어 없이. 기각 후보: 요청 ctx로 넘기기(`XxxURL(ctx, id)`: 모든 호출에 ctx, 템플릿 함수는 실행마다 다시 묶어야 한다)
- `Prefix`의 꼴: 비었거나 `/`로 시작하고 `/`로 끝나지 않는다(`/app`). 아니면 `Handler` 오류
- **표시 타임존**: 템플릿 함수 `{{date .CreatedAt "2006-01-02 15:04"}}`(값을 `Config.TimeZone`으로 옮겨 그 형식의 글로). 결과가 글이라 5번의 걷기가 타입을 안다(`date` 둘째 인자는 리터럴만, 첫째는 `time.Time`). `TimeZone`은 6번의 폼 시각 해석과 같은 값
- **오류 보고 훅의 예시**: `Config.OnError` 문서에 Sentry 같은 것을 붙이는 짧은 예(stdlib만, 외부 패키지 이름 없이), D-46 미들웨어로 요청을 감싸는 법 한 줄
- **응답 뒤 작업(D-118)**: `doc.go`에 `go f(context.WithoutCancel(ctx))`의 예와 대가(서버가 꺼질 때 기다리지 않음, panic은 앱이 받는다)

파일
- 루트 `http.go`(`Config.Prefix`, 프로세스 값, `loginURL`·`SeeOther`의 접두어), `view.go`(`URLFunc`의 접두어, `date` 함수), `doc.go`
- 생성 틀 `urls.go.tmpl`(`maago.Prefix() + …`), `views.go.tmpl`(`FuncMap`에 `date`), `internal/web/templates.go`·`walk.go`(`date`의 검사)
- 시험: 루트(접두어의 각 출구, 잘못된 접두어, 다른 접두어의 두 번째 `Handler`, `date`), `cmd/maago/testdata/web`(`/app`에서 `{{url}}`·`XxxURL`·`SeeOther`·로그인 이동, `date`)

하지 않기로 한 것
- 정적 파일 URL(M7), 프록시가 떼지 않는 배치, 요청마다 다른 타임존(사용자별)

단계
1. 접두어 → 확인: `/app`에서 `XxxURL`·`{{url}}`·`SeeOther`의 `Location`·로그인 이동과 `next`가 `/app/…`, 라우트는 `/posts/1`로 들어온다, 잘못된 꼴과 다른 접두어의 두 번째 `Handler`는 오류
2. `date` → 확인: `Config.TimeZone`(Asia/Seoul)으로 표시, 걷기가 `date`의 인자(시각 아님, 형식이 리터럴 아님)를 잡음
3. 문서 → 확인: `go doc`으로 보이는 예, `OnError`가 뷰 오류와 panic을 받는 시험(3번에 있다: 이름을 적는다)
4. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-05)**: 접두어와 표시 타임존은 `Handler`가 정하는 프로세스 값(`atomic`, 마지막 `Handler`가 이긴다, `Handler` 없이 부른 `XxxURL`은 접두어 없음·UTC). 시각 함수는 `{{date "형식" .T}}`(파이프 `{{.T | date "형식"}}`). D-122
- **접두어는 경로를 만드는 자리에서 정확히 한 번**(fable 2, codex 1·3·9): `XxxURL`, `{{url}}`(부르는 때 읽는다, init에 잡지 않는다), 요청에서 만드는 `next`. `SeeOther`·`http.Redirect`·`Config.LoginURL`은 손대지 않는다(이미 바깥에서 보이는 URL: `SeeOther[P](PostDetailURL(id))`, `LoginURL: "/app/login"`). 그래서 `/app/app/…`이 생길 자리가 없다. 확인은 `Location`을 정확한 값으로 비교한다
- **"두 번째 Handler 오류"는 버린다**(fable 1, D-114): 한 시험 프로세스가 `Handler`를 여럿 만든다. 마지막 것이 이긴다고 문서에 적는다
- **프록시가 떼지 않는 배치**(fable 3): `main`에서 `http.StripPrefix(cfg.Prefix, h)` 한 줄로 된다: `Prefix` 문서에 적는다. 교차 출처 검사는 브라우저의 `Origin`과 요청의 `Host`를 비교하므로 프록시는 바깥 `Host`를 넘겨야 한다(codex 4): 같은 문서에
- **`date`**(fable 5, codex 6·7): `date(형식 string, t)`, 형식은 문자열 리터럴(걷기가 검사, Go가 형식 글 자체는 검증하지 않음), t는 `time.Time` 또는 `*time.Time`(nil과 영값은 빈 글). 걷기의 `funcNames`·생성 `FuncMap`에 더한다. 폼의 `Field[time.Time]`을 다시 그릴 때도 `date "2006-01-02T15:04" .Form.Start.Value`
- **`OnError` 예시**(codex 8): 5xx(뷰의 오류와 panic)만 듣는다, CSRF 403·로그인 401은 아니다, 요청마다 동시에 불린다
- 확인 고침: 같은 시험 프로세스에서 접두어 없는 `Handler`와 `/app` `Handler`(마지막이 이김), `Location`·`next`의 정확한 값, `XxxURL`과 `{{url}}`이 `/app`에서도 같음(5번의 비교에 접두어). `OnError` 시험은 `TestErrorStatus`·`TestPanicIsAnError`·`TestOnErrorNilLogs`(3번)
- M7에 넘김(fable 7): `{{static}}`은 같은 `maago.Prefix()`를 부르는 때 읽고, 정적 라우트는 접두어 없이 등록
- 받지 않음: codex 5의 "init 전에 설정을 넣기"(생성 코드가 부르는 때 읽어 init 순서와 상관없다), codex 2의 프록시 통합 시험(`StripPrefix` 한 줄은 stdlib의 일)

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: 변이 넷(`URLFunc`·`next`·생성된 `XxxURL`에서 접두어를 뺌, `date`가 타임존을 무시)이 모두 잡힌다
- ponytail(스스로): 지울 것 없음. `process`는 값 둘과 `atomic.Pointer` 하나
- codex(샌드박스가 빌드를 막아 코드만 읽음): 받음 — `Prefix`의 각 조각은 비어 있지 않고 `url.PathEscape`가 바꾸지 않아야 한다(1: `/a b`·`/%2F`·`/a?b`·`/a//b`는 오류, `/app/sub`은 됨). 받지 않음 — 2 `type MyTime time.Time`(이름 붙은 다른 시각 타입)을 `date`가 받는 것: reflect가 필요하다(D-66). 거절하고 걷기가 생성 때 알린다. 4 `RequestURI`가 빈 손수 만든 요청: 서버가 준 요청은 늘 있다. 5 병렬 시험의 전역: D-122의 대가로 적혀 있다
- fable(변이 다섯 모두 잡힘): 받음 — 픽스처의 `/app` 시험을 `http.StripPrefix`로 감싸 `XxxURL`로 요청하고 `SeeOther`의 `Location`을 따라가 200(겹친 접두어면 404, 1), 숨은 상태를 사용자가 읽는 자리에(`DateFunc`의 "Handler 전에는 UTC", `Config.Prefix`의 "마지막 Handler", 2), `OnError` 문구(3), 폼 필드의 `date`(4: `{{date "2006-01-02T15:04" .Form.Start.Value}}`), M7 넘김을 MILESTONES M7에(6). 5(두 접두어 읽는 자리)는 관찰, 고칠 것 없음




## 12. 게시판 튜토리얼의 M4 단계 (D-103)

M8 대응표에서 M4가 받는 것. 각 단계는 앱의 `maagotest` 시험과 함께. ~~상태가 걸린 단계(상태 전이)는 4번의 열린 질문을 기다린다~~ → 3번이 풀었다: 규칙 밖의 전이는 `maago.Status(409, …)`.
- 게시판·글 목록(페이지)·상세 뷰와 템플릿, 일부러 틀리기(`{{.Post.Titel}}`, `Maago-Expect`)
- 글·게시판 쓰기 폼, 게시판 이름 중복(DB를 보는 검증), 공지 고정 기간 겹침·과거 시각 예약 게시 막기(폼 검증)
- 글 상태 전이(규칙 밖은 오류), 댓글, 댓글 달린 글은 숨기기
- API 라우트와 다른 출처의 호출(CORS), CSV 내보내기 스트리밍, 새 글 웹훅 알림(토큰, 실패와 재시도, 가짜 서버), 오류 보고, `/app` 하위 경로
- M5로 간 것: 작성자만 고치고 지우기(결정 절), 로그인 필수 화면

→ 확인: `TestBoardTutorial`이 새 단계까지 통과(SQLite)

### 계획

있는 것: 0001~0007(모델 셋, 마이그레이션, 쿼리 넷, 모델·쿼리 시험, 일부러 틀리기 하나). 10번부터 앱 시험은 `maagotest.DB(t)`. 이 작업은 단계(패치)를 더한다. 단계마다 커밋 글이 튜토리얼 글이고(D-103, 한국어 본문, 영어 제목), `TestBoardTutorial`이 단계마다 `gen`·`check`·`migrate`·앱 시험을 돌린다. 웹 시험은 앱 패키지에서 `maago.Handler` + `httptest`(D-113: HTTP는 `httptest` 그대로).

단계 (★ 내용의 선택)
- 0008 **Serve boards and posts**: `board/urls.go`(라우트 표), 뷰(게시판 목록, 게시판의 글 목록 `?page=`, 글 상세), 템플릿(`base.html`과 페이지 셋), `main.go`가 `maago.Handler`로 띄움(`Config.Apps`, D-114). 시험: 목록·상세 200, 없는 글 404, `?page=abc` 400
- 0009 **Try a typo in a template** (`Maago-Expect`): `{{.Post.Titel}}`과 다른 파일의 `post_detial`이 **한 번에**(D-61, M4 완료 조건 2). 0010이 되돌린다(0006→0007과 같은 꼴)
- 0010 **Fix the typos**
- 0011 **Write boards and posts**: 모델 폼 둘(`BoardForm model=Board`, `PostForm model=Post`), GET·POST 두 줄, 게시판 이름 중복은 뷰가 쿼리(`board_name_taken.sql`)로 보고 `Field.Error`(D-119), 저장 뒤 `SeeOther`. 시험: 빈 제목 200과 문구, 중복 이름 200과 그 필드의 오류, 저장 303
- ★ 0012 **Pin notices and schedule posts**: `Post`에 `PinnedFrom`·`PinnedUntil`·`PublishAt`(`*time.Time`, 마이그레이션), 폼 `Validate`가 "끝 > 시작"과 "과거 시각의 예약 금지"(DB 없는 검증), 같은 게시판의 겹치는 고정 기간은 뷰가 쿼리로(`pinned_overlap.sql`, 자기 ID 빼기). 기각 후보: 별도 `Notice` 모델(글과 공지가 갈라져 단계가 길어진다)
- ★ 0013 **Publish and archive**: 상태 전이 `draft → published → archived`, 라우트 `POST /posts/{id}/publish`·`/archive`, 규칙 밖은 `maago.Status(409, …)`. 전이 규칙은 모델 옆 Go 함수(`func (p Post) Next(to string) error`)
- ★ 0014 **Comments, and hide what has them**: 댓글 폼, 글 지우기는 댓글이 있으면 `Hidden`을 켠다(필드 `Hidden bool default=false`, 마이그레이션, 목록 쿼리가 거른다). D-84의 `NO ACTION`과 맞는다
- 0015 **An API for a frontend**: API 라우트(목록 JSON, 글 만들기 JSON 폼 D-120), `Config.Origins`로 다른 출처(CORS preflight와 실제 요청). 시험: 다른 출처의 preflight 204, 신뢰하지 않는 출처 403, 422 problem
- 0016 **Export posts as CSV**: 탈출구 뷰(D-41)가 `encoding/csv`로 흘려 보냄, `Content-Disposition`. 8번이 넘긴 것: 중간 오류는 ★ 기록하고 `panic(http.ErrAbortHandler)`로 끊는다(잘린 CSV를 정상처럼 끝내지 않는다)
- ★ 0017 **Notify a webhook of new posts**: 글을 만든 뒤 `go notify(context.WithoutCancel(ctx), …)`(D-118), 토큰은 환경 변수(`BOARD_WEBHOOK_TOKEN`, `Authorization: Bearer`), 실패는 세 번까지 다시(1초·2초 간격, 5xx·연결 오류만), 끝내 실패면 `log`. 시험: 가짜 서버(`httptest.NewServer`)가 두 번 503 뒤 200, 토큰 머리, 4xx는 다시 하지 않음. 오류 보고: `main`의 `Config.OnError` 예
- 0018 **Serve under /app**: `Config.Prefix`와 `http.StripPrefix`(D-122), `LoginURL` 없음(로그인은 M5). 시험: 링크가 `/app/…`, `SeeOther`를 따라가 200

파일
- `examples/board/steps/0008~0018`(풀어서 커밋을 쌓고 `pack`), `cmd/maago/tutorial_test.go`(새 단계의 `Maago-Expect`, 필요하면 `TestBoardTutorialCatches`에 웹 단계의 경우 하나)
- 프레임워크 코드는 바꾸지 않는 것이 목표다. 단계를 쓰다 프레임워크의 빈 곳이 나오면 멈추고 계획을 고친다(그 자리의 작업으로)

하지 않기로 한 것
- 작성자만 고치고 지우기·로그인 필수 화면(M5), 첨부 파일(M7), Tailwind(M7), PG로 바꾸기(M8)

단계
1. 단계마다 풀어 둔 저장소에서 커밋하고 `go build -C cmd/maago`로 만든 maago로 `gen`·`check`·`migrate`·`go test` → 확인: 그 단계의 시험
2. `pack` → 확인: `TestBoardTutorial`·`TestBoardTutorialCatches`(SQLite)
3. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-05)**: 예약 게시는 목록 쿼리가 미래 글을 거른다(`publish_at IS NULL OR publish_at <= :now`), M4의 지우기는 늘 숨기기(진짜 삭제는 작성자 검사와 함께 M5), 전이 규칙은 모델 옆 메서드 `func (p Post) Next(to string) error`(D-15), 앱 설정(웹훅 URL·토큰)은 `main`이 채우는 패키지 변수(튜토리얼의 관례, 프레임워크는 바꾸지 않는다)
- **0008**(fable 1·9·12): `maago.Block("rows")` 라우트 하나(게시판의 글 목록 조각, M8 표의 `{{block}}` 행)와 "본문이 그 블록만" 확인, `?page=`는 `*int64`(없으면 1), 이미 있는 `search_posts.sql`을 `?q=`(포인터 query)로, `main.go`의 `Config.OnError` 예(오류 보고는 처음부터), 앱 시험 도우미는 `maagotest.DB(t)` + `maago.Handler`
- **0011**(fable 7): 같은 게시판의 같은 제목도 뷰가 쿼리로(`post_title_taken.sql`, 아니면 unique 위반이 500), `PostForm`에 `BoardID`를 두지 않고 경로 `/boards/{id}/posts/new`에서
- **0012**(codex 1, fable 5): 모델 폼의 `*time.Time` 필드는 `Field[time.Time]` + `optional`이고 뷰가 `Given`을 보고 `*time.Time`으로 옮긴다(틀이 nullable을 지키지 않는 것은 13번 목록에), 고치기 화면이 없으므로 "자기 ID 빼기"는 지운다(고치기는 M5의 작성자와 함께)
- **0014**(codex 3, fable 6): 목록·상세 쿼리가 `NOT p.hidden`(두 방언 공통)으로 거르고, 상세는 생성된 `GetPost`가 아니라 `post_detail.sql`이라 숨긴 글은 404
- **0015**(codex 6, fable 2): 신뢰한 출처의 preflight 204, 신뢰한 출처의 실제 POST에 `Access-Control-Allow-Origin`, 신뢰하지 않은 출처의 실제 POST 403 problem(그 preflight는 405로 따로). 7번이 넘긴 것(만든 것은 200과 객체, 정수는 정수로, 본문 없는 POST 400)을 글에
- **0016**(codex 4, fable 4): 시험은 머리(`Content-Type`·`Content-Disposition`)와 CSV 행. 스트리밍 자체는 픽스처의 `TestExportStreams`가 증명한다. 중간 오류 정책(기록하고 `panic(http.ErrAbortHandler)`)과 `csv.Writer.Flush`·`http.Flusher.Flush`의 차이는 글에
- **0017**(codex 5, fable 3): 시험은 `notify`를 직접(동기) 부르고 대기는 패키지 변수(`retryWait`)로 짧게, 가짜 서버가 두 번 503 뒤 200·토큰 머리·4xx는 다시 하지 않음. 뷰 경로는 저장 303만
- **마이그레이션**(codex 7): 모델을 바꾸는 단계(0012·0014)는 쓸 때 `maago makemigrations`로 만든 SQL을 패치에 넣는다. 걷기는 만들지 않는다
- **`/app`**(codex 8): 0018 마지막, 그 시험은 다른 Handler와 섞이지 않게 처음에 만든다
- `TestBoardTutorialCatches`에 웹 경우 하나: 0009의 `Maago-Expect` 한 줄을 지우면 걷기가 실패
- 받지 않음: codex 2(모델 폼에 다른 필드)는 이 단계들이 쓰지 않는다

만들며 정한 것
- 0009는 계획의 `post_detial` 대신 첫 화면의 `board_detial`(첫 화면이 실제로 부르는 라우트)
- 0014의 지우기 뷰는 `HidePost`(`DeletePost`는 생성된 모델 함수의 이름)
- 0015의 API 뷰는 HTML 뷰와 `savePost`(DB 검사와 저장)를 함께 쓰고, 찾은 것을 `maago.Invalid`·`InvalidForm`으로 돌려준다(D-120). 생성된 `invalidPostForm`은 뷰가 부를 수 없다: `views_gen.go`는 타입 검사 뒤에 생기므로 `maago gen`이 "undefined"라 한다
- 0016의 탈출구 뷰는 경로 값이 문자열이라 템플릿이 `{{url "board_export" (print .Board.ID)}}`(런타임 `URLFunc`은 정수도 받는다)
- 0017의 `notify`는 `Hook`을 값으로 받는다(요청 때 읽은 설정, 시험이 바꾸는 패키지 변수와 겹치지 않는다)

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: 단계마다 `gen`·`check`·`go test`, `-race`·`-shuffle=on`, `TestBoardTutorial`·`TestBoardTutorialCatches`(새 경우 "템플릿 오류 둘 중 하나만 기대")
- ponytail(스스로): `TestPinAndSchedule`의 `more`를 만드는 고리를 표로(받음)
- **사람의 결정(2026-10-05)**: fable이 `ApiPostsURL`(Go 관례는 `APIPostsURL`)을 사람에게 넘겼다(codex는 13번 문서로). 지금 고친다. 범위는 에이전트가 쓰기 좋은 쪽: snake_case에서 만드는 모든 Go 이름이 staticcheck ST1003의 약어 목록을 따르고(우리 것을 더하지 않음), 사용자 코드의 `undefined: X`에 대소문자만 다른 이름이 있으면 fix가 알린다. D-110 고침. 기각: 라우트 이름에만 목록(규칙이 둘), 프로젝트가 고치는 맵핑 표(짐작할 수 없다). 바뀐 것: `mapper.FieldName`과 `TestFieldName`, `gotypes`의 `caseHint`(다른 패키지의 이름은 go/types가 이미 "but have"로 알린다), 골든·gentest(`Ids` → `IDs`), 픽스처의 `APIPostURL`
- codex(픽스처·튜토리얼 실행 없이 읽음): 받음 — CSV 쓰기 실패도 기록하고 끊는다(1, `abort`), 수식으로 시작하는 칸 앞에 `'`(2, CSV 주입, `cell`), 신뢰하지 않은 출처의 POST가 아무것도 저장하지 않음(3). 받지 않음 — 4 마이그레이션의 SQLite 꼴: PG는 M8
- fable(변이 18개, 15개 잡힘): 받음 — 첫 쪽을 보내기 전의 DB 오류는 `panic(err)`로 500과 `OnError`(1, `csv.Writer`가 첫 줄을 쥐고 있어 아직 아무것도 나가지 않았다, 글도 고침), 겹치지 않는 공지가 저장되는 경우(2, AND→OR 변이가 이제 잡힌다), 지금 이 분의 예약을 막지 않음(3, `Truncate(time.Minute)`). 4 `http.Flusher.Flush`를 지운 변이는 계획대로 픽스처의 `TestExportStreams`가 맡는다
- 13번으로: 뷰가 생성된 `invalidXxx`를 부를 수 없는 것(문서 한 줄), 탈출구 뷰의 경로 값에 템플릿 검사가 정수를 받기(런타임은 받는다)

## 13. 완료 조건 종단 테스트와 문서

- `TestM4Done`(M3의 `TestM3Done`처럼 파이프라인 수준): 템플릿 오류 둘이 한 번에 나오고(D-61) 아무것도 쓰지 않음(D-60), 나머지 조건은 두 방언에서. 튜토리얼로 D-65 시간을 다시 잰다
- 계획에서 정한 것을 DECISIONS로, TUTORIAL-FUTURE §5·7·8·9의 가정 목록을 고침, ARCHITECTURE A-3·§2·§5·§6

→ 확인: 완료 조건마다 시험 이름, CI에서 PG가 돈 실행

### 계획

완료 조건과 지금 있는 시험
1. 튜토리얼의 M4 커밋이 모두 통과: `TestBoardTutorial`(0001~0018)·`TestBoardTutorialCatches`(12번). SQLite. 그대로
2. `{{.Post.Titel}}`과 없는 URL 이름이 생성 오류: 경우별 표는 `internal/web`의 `TestTemplateErrors`(5번), 파이프라인은 튜토리얼 0009(SQLite)뿐 → **`TestM4Done`이 두 방언에서**: 픽스처를 복사해 두 파일에 오타 하나씩, `build.Gen`이 정확히 두 줄(한 번에, D-61)을 내고 파일 스냅숏이 그대로(D-60). `TestM3Done`의 꼴
3·4·5. 404·403·허용 출처, 로그인의 303·401, 폼의 세 결과: 픽스처의 `TestRequests`·`TestLogin`·`TestForm`·`TestAPIForm`(3·4·6·7·9번). 지금은 SQLite만 → **픽스처를 두 방언으로**
   - 픽스처 시험의 DB를 손으로 연 SQLite에서 `maagotest.DB(t)`로(D-113, 방언은 주소가 정한다). 드라이버 둘을 `_` import(픽스처 `go.mod`에 pgx, 버전은 `cmd/maago`의 것)
   - 뷰의 손 SQL(`SELECT EXISTS … WHERE title = ?`, 두 곳)을 쿼리 파일(`queries/title_taken.sql`)로. `?`는 PG에서 돌지 않는다. `SELECT count(*)`는 자리표시가 없어 그대로
   - `TestM4Done`이 방언마다: 복사본의 `migrations/`를 지우고 그 방언으로 `makemigrations`(`ordersProject`처럼), `gen`, 조건 2, 그다음 픽스처의 `go test`(PG는 `MAAGO_DATABASE_URL`을 PG 주소로). `TestWeb`은 SQLite 그대로 둔다(빠른 기본 경로)
- D-65 다시 재기: 튜토리얼 끝 단계(0018)의 `maago gen`과 `check` 시간을 SQLite·PG에서 재어 이 절에 적는다(시험으로 걸지 않는다: 기계마다 다르다. M3도 숫자를 적었다)

문서
- DECISIONS: M4 계획에서 정한 것은 이미 D-113~D-122와 고침으로 들어갔다. 빠진 것을 훑어 더한다(예: 0012의 모델 폼 `*time.Time`은 `Field[time.Time]` + `optional`이고 nullable을 틀이 지키지 않는다 → D-119에 한 줄. 뷰는 생성된 `bindXxx`·`invalidXxx`를 부를 수 없다 → D-120에 한 줄)
- D-103 문구: 튜토리얼 단계의 앱 시험은 `maagotest.DB(t)`와 `maago.Handler` + `httptest`(10·12번에서 바뀐 것)
- TUTORIAL-FUTURE: §5 `maago.Handler(maago.Config{DB, Apps})`와 명시적 앱(D-114, blank import 아님), §7 `/{$}`(D-115), §8 템플릿 이름은 페이지 타입(`post_list_page.html`, D-116)과 실제 오류 문구(0009의 출력), §9 폼은 `maago.Form` 임베드와 `Field[T]`(D-119). 가정 목록에서 결정이 생긴 줄을 지우거나 고친다
- ARCHITECTURE: A-3은 이미 답이 있다. §2 루트의 공개 이름 목록을 M4의 실제 이름으로(`Handler`·`Config`·`Routes`·`GET`…·`Block`·`Result`·`Show`·`SeeOther`·`Status`·`Form`·`Field`·`Invalid`·`UserFrom`·`Prefix`, 초안의 `Redirect`·`Partial`·`FormErrors`는 지운다), `internal/gotypes`를 목록에. §5 앱 찾기에 `main`의 `Config.Apps`(D-114), 생성 파일에서 `forms_gen.go`를 지움(폼은 `views_gen.go`), §6 `maagotest`가 루트의 `.env`를 찾음
- ~~12번이 넘긴 것: 탈출구 뷰의 경로 값 정수~~ → M8(사람의 결정, 아래)

하지 않기로 한 것
- 튜토리얼을 PG로(M8), CI 설정 바꾸기(이미 PG 서비스가 있다)

단계
1. 픽스처를 두 방언으로(`maagotest.DB`, 쿼리 파일, pgx) → 확인: `TestWeb`(SQLite) 그대로 통과
2. `TestM4Done` → 확인: 두 방언 통과, 변이(오타 하나만, gen이 파일을 씀, 픽스처의 404를 200으로)가 잡힘
3. ~~탈출구 경로 값 정수~~ → M8
4. 문서 → 확인: 링크·이름이 실제 코드와 같음(`git grep`으로 옛 이름이 없음)
5. 두 모듈 확인 명령, D-65 측정을 적음

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-05)**: 탈출구 뷰의 경로 값 정수(12번이 넘긴 것)는 M8로. 튜토리얼을 PG로 다시 묶을 때 검사기를 고치고 0016의 `(print …)`도 지운다. 지금은 TODO에 한 줄. 위의 "12번이 넘긴 것" 줄과 단계 3은 지운다(codex 6은 여기서, fable 14는 사람에게)
- **조건 3**(codex 1, fable 1·15): 픽스처의 `handler`에 `Origins: []string{"https://trusted.example"}`, `TestRequests`에 다른 사이트의 POST 403과 믿는 출처의 POST 통과를 더한다. 런타임의 경우별 표는 루트의 `TestCrossOrigin`. CSRF는 DB를 보지 않는다
- **조건 2**(fable 2·3): 오타를 글 그대로 — `post_page.html`의 `{{.Post.Titel}}`과 `number_page.html`의 `{{url "post_detial" …}}`, 진단은 정확히 둘. 좋은 gen과 오타 gen 사이에 `queries/title_taken.sql`을 고쳐(`queries_gen.go`가 바뀌는 변화) "아무것도 쓰지 않음"이 이빨을 갖게(`TestM3Done`의 `OFFSET 0`)
- **조건 4**(fable 4): `TestLogin`의 "API, nobody"에 `Content-Type: application/problem+json`
- **PG 마이그레이션**(codex 3): `makemigrations`가 쓴 이름(`0001_initial.sql`)과 `build.Check` 통과를 확인한다. 적용은 픽스처 시험의 `maagotest.DB`가 한다
- **두 번 돌기**(fable 8, 17): SQLite의 런타임은 `TestWeb`이 맡고, `TestM4Done`의 SQLite 쪽은 조건 2만, PG 쪽은 조건 2와 픽스처 시험
- **pgx**(codex 2, fable 7): 픽스처 `go.mod`에 `github.com/jackc/pgx/v5 v5.11.0`, `go.sum`은 `cmd/maago`의 줄들. `GOPROXY=off`는 모듈 캐시를 쓴다: `cmd/maago`가 pgx를 require하니 `go test`가 시험을 돌기 전에 받아 둔다(CI도 같다)
- **쿼리 파일**(fable 7): `queries/title_taken.sql`은 `-- column: taken bool`. 시각·스트리밍은 방언과 상관없다(상수 시각, `count(*)`), `Asia/Seoul`은 Go가 시스템 tz를 읽고 CI 이미지에 있다(codex 4)
- **전역**(codex 5): 픽스처의 `apiCalls`·`exportGate`는 시험이 병렬이 아니어서 된다. 픽스처에 그 말을 한 줄
- **D-65**(fable 6, 16): PG는 두 방언 픽스처에서 잰다(튜토리얼의 마이그레이션은 SQLite 꼴). 처음(`go list -export`가 빈 캐시)과 다시를 따로
- **문서 더함**(fable 9~12, codex 7): README의 "다음 작업", MILESTONES M4 할일 절을 한 줄로(M3처럼 D-번호), D-47에 "CSRF와 CORS는 한 목록 `Config.Origins`", ARCHITECTURE §2의 이름에 `Origins`·`OnError`·`TimeZone`·`Messages`·`DBFrom`·`WithDB`·`LoginRequired`·`API`·`App`. 옛 이름을 찾는 목록: `maago.Handler()`, blank import 등록, `forms_gen.go`, `Redirect`·`Partial`·`FormErrors`, `post_list.html`·`post_detail.html`, 폼의 `Title string` 꼴

만들며 확인한 것
- D-65 다시 잼(Apple M3 Pro, 2026-10-05): 튜토리얼 끝 단계(0018, SQLite) `gen`·`check` 0.25~0.37초. 두 방언 픽스처: SQLite `gen` 0.19초, PG `gen`·`check` 0.19~0.20초. 빈 빌드 캐시의 첫 PG `gen`은 7.4초(`go list -export`가 표준 라이브러리의 export data를 처음 만든다. 기계마다 한 번, D-65의 "캐시는 넘길 때")
- 변이: 오타를 빼면 "the failed gen changed files"(`title_taken.sql`의 변화), PG에서 픽스처의 404를 200으로 바꾸면 `TestM4Done/postgres` 실패
- TUTORIAL-FUTURE §8의 오류 두 줄은 작은 블로그 프로젝트에 실제로 돌린 출력이다

구현 리뷰 (ponytail·codex·fable)
- ponytail(스스로): 지울 것 없음. `TestM4Done`은 `webProject`·`makeMigrations`·`snapshot`·`diagnostics`를 그대로 쓴다
- 스스로 고침: §5의 `Auth: auth.User`는 타입처럼 읽혀 함수 `auth.FromSession`(🔮)으로(`Config.Auth`는 `func(*http.Request) (any, error)`)
- codex(실행 없이 읽음): 받음 — PG 쪽이 정말 PG인지 시험이 말하게(1의 뜻: 마이그레이션이 PG DDL인지 단언 한 줄. 지적 자체는 틀렸다: `webProject` 뒤에 `t.Setenv`라 PG로 돈다, 로그로 확인). 받지 않음 — 2 완료 표기: MILESTONES 체크는 "M4 끝"에서 CI 뒤(M3와 같은 순서), 3 빈 캐시의 `GOPROXY=off`: `cmd/maago`가 pgx를 import해 시험 바이너리를 만들 때 캐시에 든다, 4 §8은 게시판이 아니라 블로그 예이고 실제로 돌린 출력
- fable(변이로 확인, 막는 것 없음): 받음 — D-119에 `Config.Messages`(1), ARCHITECTURE §2의 생성 코드만 부르는 이름 보강·`DateFunc`를 그쪽으로(2), §7의 400 문장은 query 필드가 있을 때(3), D-42 `파일:줄:칸`·D-43이 D-119를 가리키게(4), D-47에 믿지 않는 출처의 preflight 405(5). 사람에게 넘긴 MILESTONES 체크 순서는 "M4 끝" 목록이 이미 정했다(CI 뒤)

---

## M4 끝

- [x] 1~13 완료 (2026-10-05)
- [x] 위 M4 완료 조건 다섯 개를 실제로 돌려 확인. PG는 로컬 서버(`MAAGO_TEST_PG_URL`)에서 돌렸다
  1. 튜토리얼의 M4 커밋(0008~0018): `cmd/maago`의 `TestBoardTutorial`, 망가진 단계를 잡는 것은 `TestBoardTutorialCatches`(웹 단계의 경우 "템플릿 오류 둘 중 하나만 기대"). SQLite
  2. `{{.Post.Titel}}`과 없는 URL 이름이 한 번에 생성 오류이고 아무것도 쓰지 않음: `TestM4Done`(두 방언). 경우별 표는 `internal/web`의 `TestTemplateErrors`(5번)
  3. path 값·`sql.ErrNoRows`의 404, 다른 사이트의 POST 403과 믿는 출처의 통과: 픽스처의 `TestRequests`(SQLite는 `TestWeb`, PG는 `TestM4Done`), 런타임의 경우별 표는 루트의 `TestCrossOrigin`
  4. 로그인 필수의 HTML 303과 API 401(`application/problem+json`): 픽스처의 `TestLogin`(두 방언), 루트의 `auth_test.go`
  5. 폼의 빈 칸·읽을 수 없는 값·범위 밖이 서로 다른 결과: 픽스처의 `TestForm`·`TestAPIForm`(두 방언), 루트의 `form_test.go`
- [ ] MILESTONES.md의 M4 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M5 초안(U-06, `fk=` 다른 앱이 선행 결정)

## 열린 질문 (계획 단계에서 정한다)

- 앱 값의 모양과 빠뜨린 앱 검사(D-114) → 2번
- 라우트 옵션의 모양(`maago.Login` 옵션인지 다른 생성자인지) → 2번
- ~~부분 응답을 요청이 고르는지 뷰가 고르는지~~ → 5번: 라우트 표의 `maago.Block("x")`(사람의 결정)
- ~~In 필드에 슬라이스(`?tag=a&tag=b`)를 받는지~~ → 4번: 받지 않는다
- ~~뷰가 404·400·500 밖의 상태(403·409)를 내는 법~~ → 3번: `maago.Status(code, err)`
- ~~게시판 이름 중복을 unique 위반으로 잡을지 검증 메서드로 잡을지~~ → 6번: 뷰가 쿼리로(사람의 결정, unique 위반을 읽는 장치는 없다)

## 초안 리뷰 (2026-09-28)

- codex: 받음 — `gc` importer는 `go list -export`로 `go` 바이너리를 부른다(1), 두 갈래 importer는 직접 짜야 한다(2), 파일 선택은 `go list`를 따르고 cgo는 지원하지 않음 오류(3), CSRF 확인 경우들(4), CORS의 메서드·헤더·자격 증명은 따로 필드, 한 목록의 보안 뜻을 적음(5), maagotest 주소 규칙과 PG `search_path`의 연결마다 확인(6), U-07을 M4·M5로 나눔(7), `maagotest`는 원칙의 예외로 명시(9), 충돌은 실제 `ServeMux` 규칙·URL 함수 인자와 이스케이프(10), 폼 흐름(12, 템플릿을 페이지 타입에), query·본문 400(13), 검사도 `html/template` 파싱을 거침·`embed`의 `..`(14), 응답 뒤 작업의 (b) 선택지(15), 작성자 검사를 사람에게(16, M5로), 확인 보강·정적 파일 URL은 M7(17), 파이프라인 순서(18). 8 명시적 등록은 초안이 받지 않았고(D-46) 사람이 골랐다(D-114). 받지 않음 — 11 `app:name`(D-115의 기각 이유)
- fable: 받음 — `embed` import(1), 레이아웃 자리(2), 생성물 두 번(3), POST의 템플릿(4), 폼 실패 상태를 사람에게(5), 해석 실패의 상태와 모르는 키(6), `WithDB`·`*sql.DB`·panic(7), contrib 라우트도 생성 때(8), unique 위반의 대가와 장치 하나(9), 오류 보고 훅 하나·`ErrAbortHandler`(10), 두 시그니처의 이유와 기각(11), 조건 1은 SQLite(12), `TestM4Done`(13), "cannot check"(14), 칸 1 기준(15), 1번 시간은 합성 픽스처·목록 둘(16), 스트리밍 시험(17), preflight 문구(18), 셋을 쌍으로(19), 결정적 스키마 이름·주소 규칙 셋·`mode=rw`·시험 자리(20), 10번의 숨은 일(21), `lookup`으로 export data(22), 웹훅 실패·재시도(23), `next` 검증을 M5로(24), 타임존 UTC(25), 12번이 4번을 기다림(26)
