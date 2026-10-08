# 아키텍처

Maago 내부가 어떻게 생겼는지 적는다. 무엇을 왜 하는지는 [DECISIONS.md](DECISIONS.md)에 있고,
이 문서는 그 결정들이 코드의 어디에 사는지를 말한다. 괄호 속 번호는 결정이다.

---

## 1. 두 세계

Maago는 두 부분으로 나뉜다. 둘의 경계가 이 구조의 전부다.

| | 생성 시점 | 런타임 |
|---|---|---|
| 무엇 | `maago` CLI와 생성기 | 사용자 앱과 생성 코드가 import하는 라이브러리 |
| 언제 | 에이전트가 `maago gen`·`check`를 돌릴 때 | 사용자 앱이 요청을 처리할 때 |
| 할 일 | 읽고, 검사하고, 연습용 DB로 확인하고, Go 코드를 쓴다 | 생성 코드가 부르는 작은 도구들 |
| 쓸 수 있는 것 | `go/ast`, `go/types`, `text/template`, DB 드라이버 | stdlib(`net/http`, `database/sql`, `html/template`, `encoding/json`)만 |

**무거운 일은 전부 생성 시점에 한다** (D-59). 런타임은 얇게 두고, 생성 코드는 런타임의 작은 함수들과
stdlib만 부른다. 런타임에는 `reflect`가 없다 (D-66).

---

## 2. 저장소 구성

```
github.com/maago-web/maago
│
├── maago.go, …          런타임 공개 API (package maago)
│                          M3까지 있는 것: Model, DB·OpenDB·DBConfig(D-101), Opt·Set·Ptr(D-106),
│                          ErrManyRows·ErrEmptySlice(D-111).
│                          M4가 더한 것: Handler·Config(DB·Apps·Origins·OnError·Messages·TimeZone·
│                          Auth·LoginURL·Prefix)·App(D-114), Routes·GET·POST·…·API·Block·LoginRequired
│                          (D-115, D-45, D-121), Result·Show·SeeOther·Status(D-116), DBFrom·WithDB,
│                          Form·Field·Messages·Invalid·InvalidForm(D-119, D-120), UserFrom·WithUser
│                          (D-121), Prefix(D-122).
│                          생성 코드만 부르는 것: One·Many·Exec, Placeholders,
│                          SQLiteTime·SQLiteNullTime·ScanSQLiteTime·ScanNullSQLiteTime(D-105),
│                          NewApp·AppRoute·Pages·Render·Bind*·FinishForm·FormValues·JSONValues·Unprocessable·
│                          Rule·FieldError·URLFunc·PathSegment·DateFunc …
│                          런타임이 기대는 내부 패키지는 internal/diag·internal/project뿐(시험이 본다)
├── maagotest/          시험 전용 도우미 (M4, D-113). 런타임 공개 API가 루트 하나라는 원칙의 유일한 예외
├── contrib/
│   ├── auth/            인증의 보안 부품(stdlib만, M5). 앱 층은 `maago add auth`가
│   │                      프로젝트에 복사한다(D-123)                     D-50, D-51
│   └── admin/           admin 런타임 (M6)         D-48, D-49
│
├── cmd/maago/           CLI 진입점. flag 디스패치만 한다 (D-68)
│                          별도 모듈(자체 go.mod). DB 드라이버는 여기에만
│
└── internal/            생성기. 사용자는 import할 수 없다
    ├── build/           gen·check·makemigrations·migrate의 단계 조립(모든 단계를 아는 유일한 곳)  M1·M2
    ├── diag/            진단: 위치·메시지·고치는 법, 정렬·중복 제거·출력       M0
    ├── pipeline/        단계 실행, 오류 수집, 메모리 생성물, 쓰기·최신성 비교    M0
    ├── golden/          골든 파일 테스트 도구 (테스트 전용)                    M0
    ├── project/         사용자 프로젝트 찾기: go.mod 루트, `./...` 파일 목록,      M1·M2
    │                      `.env`의 `MAAGO_DATABASE_URL`과 방언 이름
    ├── model/           태그 파서, 모델 읽기·검사, 제약 모으기                  M1
    ├── schema/          DB에서 읽은 스키마(두 방언 공통 표현)                    M2
    ├── dialect/         방언 공통: 제약 이름(D-86), 인용, 리터럴. SQL 토큰화(M2)
    │   ├── sqlite/      DDL 쓰기, 스키마 읽기, 변경에서 초안 쓰기(테이블 재생성)  M1·M2
    │   └── postgres/                                                      M1·M2
    ├── practicedb/      maago가 모는 연결 하나: 연습용 DB(SQLite 메모리, PG 롤백되는  M2
    │                      스키마)와 `migrate`의 실제 DB
    ├── migrate/         파일 읽기·문장 검사(allow-drop), 비교, 진단, 초안 이름,    M2
    │                      적용·기록(`maago_migrations`)
    ├── mapper/          SQL 매퍼: 머리·본문 해석, 조합, prepare, 결과 타입, PK CRUD,   M3
    │                      생성(templates/), 이름 충돌 (D-20~D-24, D-107~D-112)
    ├── gotypes/         사용자 프로젝트의 타입 검사: go/types, export data, 메모리 생성물  M4
    │                      (D-117)
    ├── web/             라우트 표 읽기, 뷰 어댑터·폼 생성, 템플릿 검사          M4
    ├── scaffold/        `maago add`가 프로젝트에 복사하는 contrib 앱의 앱 층       M5
    │                      (embed한 testdata/<앱>/, D-123)
    └── admingen/        admin 코드 생성                                    M6
```

원칙
- **모듈은 둘.** 루트(`github.com/maago-web/maago`)는 런타임과 생성기이고 외부 의존이 없다.
  `cmd/maago`는 별도 모듈로, 루트 모듈과 DB 드라이버(SQLite는 cgo 없는 순수 Go)에 의존한다.
  라이브러리만 `go get`하는 사용자에게 드라이버가 따라가지 않는다 (D-03, D-09).
- **런타임 공개 API는 루트 패키지 `maago` 하나.** 사용자가 import할 경로를 늘리지 않는다.
- **생성기는 모두 `internal/`.** 공개 API가 아니므로 마음대로 고칠 수 있다.
- **방언의 SQL은 `internal/dialect/` 안에만.** DDL 쓰기, 스키마 읽기, 초안 쓰기는 방언 디렉터리에 있다. MySQL(v1)은
  디렉터리 하나를 더하는 일이 된다. 연결과 트랜잭션을 모는 곳(`practicedb`, `migrate`의 적용·기록, `build`의 방언 표)은
  `project.Dialect` 이름으로 갈린다: SQLite의 `PRAGMA foreign_keys`와 `foreign_key_check`, PG의 스키마와 `SET LOCAL` 같은 것(M2).
- 마일스톤마다 패키지가 하나둘씩 는다. 먼저 있는 패키지를 고치기보다 새 패키지를 더하는 모양이 되게 한다.

---

## 3. `maago gen` 파이프라인

```
 읽기 ──▶ 해석 ──▶ 검사 ──▶ DB 검사 ──▶ 생성 ──▶ 쓰기 (gen) / 비교 (check)
```

| 단계 | 입력 | 출력 | 하는 일 |
|---|---|---|---|
| 읽기 | 작업 디렉터리 | 프로젝트 정보 | go.mod, 앱 패키지, 파일 목록 (`project`) |
| 해석 | 파일들 | 구문 트리 | Go 소스(`go/parser`), 태그, SQL 파일(머리·본문, 모델 검사·`.env`·마이그레이션 파일과 같은 단계라 서로를 가리지 않는다), 템플릿 |
| 검사 | 구문 트리 | 진단 | DB 없이 할 수 있는 모든 검사: 태그 목록, SQL 규칙 1~4와 조합(D-109), 생성 이름의 충돌과 쿼리 앱의 package(D-112), 라우트 표, 템플릿 필드. 모델 검사·`.env`·마이그레이션 파일 읽기 → 이름·문장 검사 → 방언 DDL의 작은 단계들이다(M2 8번) |
| DB 검사 | 스키마, 마이그레이션, 쿼리 | 진단 + 읽은 스키마 | 연습용 DB에 마이그레이션 적용 → 읽기 → 모델과 비교. 같으면 같은 DB에서 쿼리의 모든 조합을 prepare하고(PG는 prepare마다 `SAVEPOINT`, 파라미터 타입도 맞춰 본다. SQLite는 파라미터 타입을 모른다) 결과 타입을 정한다(D-23, D-104, D-110). CRUD의 SQL도 prepare한다(실패는 maago의 버그) |
| 생성 | 검사를 통과한 모든 것 | 메모리 속 파일들 | `path → []byte` 표. 디스크는 아직 건드리지 않는다 |
| 쓰기 / 비교 | 메모리 속 파일들 | 디스크 / 진단 | `gen`은 쓰고 옛 생성물을 지운다. `check`는 디스크와 비교만 한다 |

규칙
- **한 단계 안의 독립 오류는 전부 모은다. 오류가 난 단계에서 멈춘다** (D-61). 해석이 실패한 파일을
  검사하면 연쇄 오류가 쏟아지기 때문이다. 단계 경계가 곧 연쇄를 끊는 자리다.
- **오류가 하나라도 있으면 쓰기 단계에 가지 않는다** (D-60).
- **연습용 DB에 연결할 수 없으면 DB 검사 단계의 오류다** (D-64). 건너뛰지 않는다.
- 쓰기는 모든 파일을 임시 파일로 쓴 뒤 하나씩 이름을 바꾼다. 여러 파일에 걸친 완전한 원자성은
  파일 시스템이 주지 않는다. 이름 바꾸기 중 실패하면 실패한 파일을 알리고 다시 돌리라고 한다.
  디스크와 같은 파일은 건너뛰므로, 다시 돌리면 남은 파일만 쓴다.
- 검사기는 단계에 붙는 함수다. M1부터는 새 검사기를 붙이기만 하고 파이프라인은 고치지 않는다.
- 다른 명령은 같은 단계를 쓰고 끝만 바꾼다(`add`는 단계를 쓰지 않고 루트만 찾아 파일을 복사한다, D-123). `makemigrations`는 DB 검사 자리에서 초안을 쓰고 생성이 없다. 쿼리 파일은 읽지 않는다(D-112).
  `migrate`·`showmigrations`는 모델을 읽지 않는다: `.env`와 마이그레이션 파일을 읽고 문장 검사 뒤 실제 DB에 간다(M2 9·10번).

---

## 4. 진단

```go
type Diagnostic struct {
    pos token.Position // 파일:줄:칸. 파일 이름이 없으면 위치 없음
    msg string         // 무엇이 틀렸나 (영어, 한 줄)
    fix string         // 고치는 법 (영어, 한 줄, 필수)
}

func New(pos token.Position, msg, fix string) Diagnostic // 빈 msg·fix는 패닉
func Write(w io.Writer, ds []Diagnostic) error          // 정렬, 중복 제거, 출력
```

```
blog/models.go:12:18: unknown tag key "maxlen" in Post.Title
  fix: did you mean "max"? allowed: max, unique, index, default, choices, label, help, fk, auto
```

- 고치는 법이 빈 진단은 만들 수 없게 생성자에서 막는다 (D-62). 필드가 비공개라 다른 패키지는 생성자로만 채울 수 있다.
- 수준(경고/오류)은 없다. 진단은 모두 오류다 (D-63).
- 출력은 파일·줄·칸 순으로 정렬하고, 같은 진단은 한 번만 낸다.
- 위치가 파일 전체나 프로젝트 전체인 진단(예: DB 연결 실패)은 위치 없이 낸다.
- 파일 경로는 프로젝트 루트 기준이다(D-88).

---

## 5. 사용자 프로젝트와 생성 코드

사용자 프로젝트의 모양 (M1~M4에 걸쳐 채워진다):

```
myapp/
├── go.mod
├── main.go
├── blog/                  앱 = Go 패키지
│   ├── models.go          struct + maago 태그           사람
│   ├── queries/*.sql      매퍼 파일                      사람
│   ├── urls.go            라우트 표                      사람
│   ├── views.go           뷰                           사람
│   ├── templates/*.html   html/template                 사람
│   └── *_gen.go           생성 코드                      maago
└── migrations/
    └── 0001_initial.sql   초안은 maago, 고치는 건 사람
```

- **앱 찾기**: 등록 파일은 없다. 생성기는 모듈 안의 패키지를 훑어 `maago.Model`을 임베드한 struct, `queries/`
  디렉터리, `maago.Routes` 값이 있는 패키지를 앱으로 본다. 서버에 올리는 것은 `main`이 `Config.Apps`에
  적는다(D-114). 라우트가 있는 앱의 `App`을 아무 패키지도 쓰지 않으면 생성 오류다.
- **생성 파일 이름**: 관심사마다 하나. `models_gen.go`, `queries_gen.go`, `urls_gen.go`(URL 함수와 `App`),
  `views_gen.go`(뷰 어댑터와 폼 읽기), `admin_gen.go`. 파일 하나가 끝없이 커지지 않고, diff가 어느 기능의
  변화인지 보인다.
- **생성 파일 표시**: 첫 줄은 Go 표준 형식 `// Code generated by maago. DO NOT EDIT.`. 첫 줄에 이 표시가
  있는 파일만 maago 소유다(D-67). `check`는 새로 생겨야 할 파일·사라져야 할 파일까지 비교하고, `gen`은
  사라져야 할 파일을 지운다(D-60). 표시 없는 파일은 이름이 `_gen.go`여도 덮어쓰거나 지우지 않는다.
  훑는 범위는 `go` 도구의 `./...`와 같다(`testdata`, `vendor`, `.`·`_`로 시작하는 것, 하위 모듈 제외).
- 생성 코드는 커밋한다 (D-67). 생성기 없이도 빌드되고, 변화가 리뷰에 보인다.
- `migrations/`의 파일은 생성 파일이 아니다. 초안은 maago가 쓰지만 주인은 사람이고, 검증은
  "적용 결과가 모델과 같은가"로 한다 (D-32, D-34).

- M3가 쓰는 것: 모델이 있는 앱마다 `models_gen.go`(D-18 주석, 모델마다 입력 타입과 CRUD, D-106·D-111),
  `queries/`가 있는 앱마다 `queries_gen.go`(쿼리마다 파라미터·결과 타입과 함수). 둘이 선언하는 이름이
  서로나 그 패키지의 사람이 쓴 이름(같은 패키지의 테스트 파일 포함)과 겹치면 생성 오류다.

생성기는 Go 코드를 `text/template`(`embed.FS`에 내장)로 쓰고 `go/format`으로 정리한다.
생성기 안에서 stdlib가 reflect를 쓰는 것은 괜찮다. 생성 시점이기 때문이다.

---

## 6. CLI 설정 (D-08)

CLI는 프로젝트의 DB를 `MAAGO_DATABASE_URL` 하나로 안다. 방언은 스킴에서 읽는다.

```
# myapp/.env
MAAGO_DATABASE_URL=postgres://localhost:5432/myapp_dev
# 또는
MAAGO_DATABASE_URL=sqlite:app.db
```

- 프로젝트 루트(`go.mod`가 있는 곳)의 `.env`에서 읽는다. 같은 이름의 환경 변수가 있으면 그것이 이긴다
  (CI에서 덮어쓰기 위해).
- `.env` 해석은 `KEY=VALUE` 줄, `#` 주석, 따옴표 벗기기만 한다. 외부 라이브러리를 쓰지 않는다.
  모르는 형식의 줄은 조용히 넘기지 않고 `.env:줄` 위치와 함께 오류로 낸다.
- SQLite 연습용 DB는 메모리라 `check`는 방언만 읽는다. `migrate`·`showmigrations`는 `sqlite:` 뒤의 경로를
  프로젝트 루트 기준으로 연다. `?` 옵션이나 `file:` 주소는 오류다(M2 10번).
- `.env`는 커밋하지 않는다. `maago new`(v1)가 `.gitignore`에 넣는다.
- 런타임의 `maago.OpenDB`도 같은 주소를 읽는다: 환경 변수, 없으면 작업 디렉터리의 `.env`. 런타임의 SQLite 상대 경로는 작업 디렉터리 기준이다(D-101, M3 6번에서 만든다).
- 앱의 시험은 `maagotest.DB(t)`: `go test`가 도는 패키지 디렉터리에서 위로 루트를 찾아 그 `.env`(또는 환경
  변수)의 방언과 서버를 쓰고, 시험마다 새 DB(SQLite는 `t.TempDir()`의 파일, PG는 시험의 스키마)에 마이그레이션을
  적용한다(D-113).

---

## 7. reflect 금지 게이트 (D-66)

- 검사 대상: 저장소의 모든 비테스트 `.go` 파일, 그리고 생성기 템플릿(생성 코드가 될 글).
- 생성기 자신은 런타임이 아니지만, 예외를 두지 않는 편이 규칙이 단순하다. 필요해지면 그때 좁힌다.
- 생성기 템플릿의 확장자는 `.tmpl`이다. Go로 해석되지 않으므로 `"reflect"` 문자열로 검사한다.
- `testdata/`도 검사한다. 생성 코드의 골든 파일은 `.go` 확장자를 유지하고, 일부러 깨진 Go 입력 픽스처는
  `.go`가 아닌 확장자를 쓴다.
- `go test ./...`의 테스트 하나로 돌고, CI가 매번 돈다.
- 둘째 검사(`TestGeneratedImports`, M3): 생성기는 `text/template`을 쓰므로 위 검사로는 막을 수 없다. 첫 줄이
  생성 표시인 `.go` 파일(생성 골든 포함)의 import는 `mapper.Imports` 허용 목록으로 보고(D-66), 템플릿에
  `"text/template"`·`"reflect"` 글이 있으면 실패한다.

---

## 정해진 질문

초안에서 열어 두었던 질문과 그 답이다(2026-09-24).

| # | 질문 | 답 |
|---|---|---|
| A-1 | CLI의 DB 드라이버 의존 | `cmd/maago`를 별도 모듈로. 런타임 라이브러리는 의존 0 (D-03) |
| A-2 | 프로젝트의 DB 선언 위치 | `.env`의 `MAAGO_DATABASE_URL`, 환경 변수가 우선 (D-08) |
| A-3 | 타입 정보 | M1~M3은 `go/ast`만. M4부터 `go/types`와 직접 짠 importer(모듈 밖은 `go list -export`의 export data, 프로젝트 안은 소스와 메모리 생성물). `x/tools` 없음 (D-117) |
| A-4 | 외부 의존 원칙 | 런타임은 stdlib만. 생성기는 필요한 것을 쓰되 마이그레이션 로직은 자체 구현 (D-09) |
