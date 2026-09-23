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
│                          사용자가 쓰는 이름: maago.Model, maago.Form, maago.Routes,
│                          maago.GET/POST/API, maago.Redirect, maago.Partial, maago.FormErrors …
├── contrib/
│   ├── auth/            인증 앱 (M5)             D-50, D-51
│   └── admin/           admin 런타임 (M6)         D-48, D-49
│
├── cmd/maago/           CLI 진입점. flag 디스패치만 한다 (D-68)
│                          별도 모듈(자체 go.mod). DB 드라이버는 여기에만
│
└── internal/            생성기. 사용자는 import할 수 없다
    ├── diag/            진단: 위치·메시지·고치는 법, 정렬·중복 제거·출력       M0
    ├── pipeline/        단계 실행, 오류 수집, 메모리 생성물, 쓰기·최신성 비교    M0
    ├── golden/          골든 파일 테스트 도구 (테스트 전용)                    M0
    ├── project/         사용자 프로젝트 찾기: go.mod, 앱 패키지, 파일 목록       M1
    ├── model/           태그 파서, 모델 → 스키마                            M1
    ├── schema/          방언 중립 스키마 표현                                M1
    ├── dialect/
    │   ├── sqlite/      DDL 쓰기, 스키마 읽기, 테이블 재생성                   M1·M2
    │   └── postgres/                                                      M1·M2
    ├── practicedb/      연습용 DB 열기·닫기 (SQLite 메모리, PG 임시 스키마)      M2
    ├── migrate/         비교, 초안, 적용, 검증, allow-drop                    M2
    ├── mapper/          SQL 파일 파서, 커버 조합, Go 번역, PK 기본 CRUD         M3
    ├── web/             라우트 표 읽기, 뷰 어댑터·폼 생성, 템플릿 검사          M4
    └── admingen/        admin 코드 생성                                    M6
```

원칙
- **모듈은 둘.** 루트(`github.com/maago-web/maago`)는 런타임과 생성기이고 외부 의존이 없다.
  `cmd/maago`는 별도 모듈로, 루트 모듈과 DB 드라이버(SQLite는 cgo 없는 순수 Go)에 의존한다.
  라이브러리만 `go get`하는 사용자에게 드라이버가 따라가지 않는다 (D-03, D-09).
- **런타임 공개 API는 루트 패키지 `maago` 하나.** 사용자가 import할 경로를 늘리지 않는다.
- **생성기는 모두 `internal/`.** 공개 API가 아니므로 마음대로 고칠 수 있다.
- **방언 차이는 `internal/dialect/` 안에만.** 다른 패키지는 방언을 모른다. MySQL(v1)은 디렉터리 하나를 더하는 일이 된다.
- 마일스톤마다 패키지가 하나둘씩 는다. 먼저 있는 패키지를 고치기보다 새 패키지를 더하는 모양이 되게 한다.

---

## 3. `maago gen` 파이프라인

```
 읽기 ──▶ 해석 ──▶ 검사 ──▶ DB 검사 ──▶ 생성 ──▶ 쓰기 (gen) / 비교 (check)
```

| 단계 | 입력 | 출력 | 하는 일 |
|---|---|---|---|
| 읽기 | 작업 디렉터리 | 프로젝트 정보 | go.mod, 앱 패키지, 파일 목록 (`project`) |
| 해석 | 파일들 | 구문 트리 | Go 소스(`go/parser`), 태그, SQL 파일, 템플릿, 마이그레이션 파일 |
| 검사 | 구문 트리 | 진단 | DB 없이 할 수 있는 모든 검사: 태그 목록, SQL 규칙 1~4, 라우트 표, 템플릿 필드 |
| DB 검사 | 스키마, 마이그레이션, 쿼리 | 진단 + 읽은 스키마 | 연습용 DB에 마이그레이션 적용 → 읽기 → 모델과 비교, 쿼리 커버 조합 prepare |
| 생성 | 검사를 통과한 모든 것 | 메모리 속 파일들 | `path → []byte` 표. 디스크는 아직 건드리지 않는다 |
| 쓰기 / 비교 | 메모리 속 파일들 | 디스크 / 진단 | `gen`은 쓴다. `check`는 디스크와 비교만 한다 |

규칙
- **한 단계 안의 독립 오류는 전부 모은다. 오류가 난 단계에서 멈춘다** (D-61). 해석이 실패한 파일을
  검사하면 연쇄 오류가 쏟아지기 때문이다. 단계 경계가 곧 연쇄를 끊는 자리다.
- **오류가 하나라도 있으면 쓰기 단계에 가지 않는다** (D-60).
- **연습용 DB에 연결할 수 없으면 DB 검사 단계의 오류다** (D-64). 건너뛰지 않는다.
- 쓰기는 모든 파일을 임시 파일로 쓴 뒤 하나씩 이름을 바꾼다. 여러 파일에 걸친 완전한 원자성은
  파일 시스템이 주지 않는다. 이름 바꾸기 중 실패하면 어느 파일까지 바뀌었는지 진단으로 알린다.
- 검사기는 단계에 붙는 함수다. M1부터는 새 검사기를 붙이기만 하고 파이프라인은 고치지 않는다.

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
  fix: did you mean "max"? allowed: max, min, unique, index, default, choices, fk, label, help
```

- 고치는 법이 빈 진단은 만들 수 없게 생성자에서 막는다 (D-62). 필드가 비공개라 다른 패키지는 생성자로만 채울 수 있다.
- 수준(경고/오류)은 없다. 진단은 모두 오류다 (D-63).
- 출력은 파일·줄·칸 순으로 정렬하고, 같은 진단은 한 번만 낸다.
- 위치가 파일 전체나 프로젝트 전체인 진단(예: DB 연결 실패)은 위치 없이 낸다.

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

- **앱 찾기**: 등록 파일은 없다. 모듈 안의 패키지를 훑어 `maago.Model`을 임베드한 struct, `queries/`
  디렉터리, `maago.Routes` 값이 있는 패키지를 앱으로 본다.
- **생성 파일 이름**: 관심사마다 하나. `models_gen.go`, `queries_gen.go`, `urls_gen.go`, `views_gen.go`,
  `forms_gen.go`, `admin_gen.go`. 파일 하나가 끝없이 커지지 않고, diff가 어느 기능의 변화인지 보인다.
- **생성 파일 표시**: 첫 줄은 Go 표준 형식 `// Code generated by maago. DO NOT EDIT.`. `check`는 이 표시가
  있는 파일을 maago 소유로 보고, 새로 생겨야 할 파일·사라져야 할 파일까지 비교한다.
- 생성 코드는 커밋한다 (D-67). 생성기 없이도 빌드되고, 변화가 리뷰에 보인다.
- `migrations/`의 파일은 생성 파일이 아니다. 초안은 maago가 쓰지만 주인은 사람이고, 검증은
  "적용 결과가 모델과 같은가"로 한다 (D-32, D-34).

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
- SQLite 프로젝트의 연습용 DB는 메모리라 주소의 파일 이름은 쓰지 않는다. 방언만 읽는다.
- `.env`는 커밋하지 않는다. `maago new`(v1)가 `.gitignore`에 넣는다.

---

## 7. reflect 금지 게이트 (D-66)

- 검사 대상: 저장소의 모든 비테스트 `.go` 파일, 그리고 생성기 템플릿(생성 코드가 될 글).
- 생성기 자신은 런타임이 아니지만, 예외를 두지 않는 편이 규칙이 단순하다. 필요해지면 그때 좁힌다.
- 생성기 템플릿의 확장자는 `.tmpl`이다. Go로 해석되지 않으므로 `"reflect"` 문자열로 검사한다.
- `testdata/`도 검사한다. 생성 코드의 골든 파일은 `.go` 확장자를 유지하고, 일부러 깨진 Go 입력 픽스처는
  `.go`가 아닌 확장자를 쓴다.
- `go test ./...`의 테스트 하나로 돌고, CI가 매번 돈다.

---

## 정해진 질문

초안에서 열어 두었던 질문과 그 답이다(2026-09-24).

| # | 질문 | 답 |
|---|---|---|
| A-1 | CLI의 DB 드라이버 의존 | `cmd/maago`를 별도 모듈로. 런타임 라이브러리는 의존 0 (D-03) |
| A-2 | 프로젝트의 DB 선언 위치 | `.env`의 `MAAGO_DATABASE_URL`, 환경 변수가 우선 (D-08) |
| A-3 | 타입 정보 | M1~M3은 `go/ast`만. M4에서 `golang.org/x/tools/go/packages`를 쓸지 정한다. 쓰면 루트 `go.mod`에 `require`가 생기므로(D-09) 그 코드를 `cmd/maago` 모듈에 둘지도 함께 정한다 |
| A-4 | 외부 의존 원칙 | 런타임은 stdlib만. 생성기는 필요한 것을 쓰되 마이그레이션 로직은 자체 구현 (D-09) |
