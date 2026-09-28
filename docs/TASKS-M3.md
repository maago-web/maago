# M3 작업 목록 — SQL 매퍼

[MILESTONES.md](MILESTONES.md)의 M3를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M3 완료 조건** (MILESTONES)
- 게시판 튜토리얼(M8)의 모델·마이그레이션·매퍼 커밋이 통과한다 → 10번
- DECISIONS의 `SearchOrders` 예제가 생성되고, 조합별로 기대한 SQL이 나온다(골든) → 3번(글), 8번(생성·실행)
- 빈 WHERE의 DELETE, 라벨이 보장하지 않는 파라미터, 컬럼이 빠진 모델 결과 타입이 각각 생성 오류다 → 3번(앞의 둘), 5번(셋째)
- 생성 코드에 `text/template`도 `reflect`도 없다 → 9번

**M2에서 넘어온 것** ([TASKS-M2.md](TASKS-M2.md) 끝)
- 쿼리 여럿을 연습용 DB에서 prepare하며 실패를 모두 모으려면 PG는 쿼리마다 `SAVEPOINT`가 필요하다(D-96: 트랜잭션 안의 스키마) → 4번
- 연습용 DB는 연결 하나(`*sql.Conn`)다. 매퍼 검증도 그 연결에서 한다 → 모양 절
- 런타임 연결에서 SQLite `PRAGMA foreign_keys = ON`(TODO) → D-101, 6번
- PG 연습용 DB의 `search_path`에 `public`이 없다. `public`의 확장 함수를 이름만으로 쓰는 쿼리도 연습용 DB에서 실패한다(TODO에 남긴다, 필요해지면)

---

## 먼저 확인한 사실 (2026-09-27, 모듈 캐시의 드라이버와 작은 실험)

계획이 드라이버가 못 하는 일을 가정하지 않게 먼저 쟀다. codex·fable이 소스에서 다시 확인했다.

- **PG는 실행 없이 묘사할 수 있다.** pgx v5.11.0의 `pgconn.StatementDescription`이 결과 컬럼마다 `TableOID`·`TableAttributeNumber`·`DataTypeOID`를, 파라미터마다 `ParamOIDs`를 준다. `sql.Conn.Raw` → `*stdlib.Conn` → `Conn()`(`*pgx.Conn`) → `PgConn().Prepare(ctx, "", …)`로 닿는다. 이름이 빈 문장은 서버에 남지 않는다(이름 있는 것은 `Deallocate`가 필요하다. 4번 계획에서 고침)
- **SQLite는 컬럼의 출처만 안다.** modernc v1.59.0의 `ColumnInfo(query)`(비공개 `*conn`의 메서드라 `Raw`의 driverConn을 `interface{ ColumnInfo(string) ([]sqlite.ColumnInfo, error) }`로 단언해 부른다. modernc 자체 테스트가 그렇게 한다)가 결과 컬럼마다 이름, 선언 타입, 출처 테이블·컬럼을 준다. 식(`count(*)`)은 모두 비어 있고, 파라미터의 타입은 없다
- **출처로는 NULL 여부를 알 수 없다.** `LEFT JOIN`된 쪽의 `x.id`도 두 드라이버 모두 NOT NULL인 원래 컬럼으로 보고한다. `CASE`·`COALESCE`·집계·스칼라 서브쿼리도 NULL을 낼 수 있고, 이것들은 출처가 없다. `UNION`은 SQLite가 첫 `SELECT`의 출처를 주고 PG는 주지 않는다(5번에서 잼, 그래서 D-110은 출처를 모두 버린다)
- **SQLite의 `time.Time`은 그대로 되지 않는다.** STRICT `TEXT` 컬럼에 `time.Time`을 넘기면 modernc는 Go의 `String()` 모양(`2026-09-27 12:00:00 +0900 KST`)으로 쓰고, 읽기의 자동 변환은 선언 타입이 `DATE`·`DATETIME`·`TIMESTAMP`일 때뿐이라 `*time.Time`으로 읽으면 Scan 오류다. → D-105
- **`database/sql`의 Scan**: NULL을 `int64`·`string`·`time.Time`에 넣으면 오류, 포인터에 넣으면 `nil`. 선언되지 않은 NULL은 조용히 0이 되지 않는다
- 드라이버를 부르는 일(묘사, 출처)은 루트 모듈이 할 수 없다(D-90). `practicedb.ErrorPosition`처럼 `cmd/maago`가 함수를 꽂는다

## 결정 (2026-09-27, 초안 리뷰 뒤 사람의 결정)

- **D-101 런타임이 DB를 여는 법**: `maago.OpenDB(ctx, maago.DBConfig{}) (*sql.DB, error)`. 빈 URL은 환경 변수 `MAAGO_DATABASE_URL`, 없으면 작업 디렉터리의 `.env`(D-97 형식, 환경 변수가 이긴다). 스킴에서 드라이버 이름(`sqlite`·`pgx`), 드라이버 import는 사용자 `main`(D-90). SQLite는 maago가 드라이버에 넘기는 주소에 `_pragma=foreign_keys(1)`·`busy_timeout(5000)`·`journal_mode(WAL)`을 붙인다(사용자 주소의 `?`는 여전히 오류, TODO의 PRAGMA 닫힘). 풀 기본값: PG 최대 20·유휴 5·수명 30분·유휴 시간 5분, SQLite 최대 4·유휴 4·수명 없음. 필드로 덮는다. 생성된 매퍼는 `maago.DB`(`ExecContext`·`QueryContext`·`QueryRowContext`)를 받는다. 추천은 환경 변수만이었고, 사람이 `.env`까지 골랐다(`go run .`이 CLI와 같은 설정으로 돈다). 대가와 6번 계획이 정할 것: 런타임이 `.env`를 읽는 코드(`internal/project`를 들일지, 작게 다시 쓸지), SQLite 상대 경로는 작업 디렉터리 기준(CLI는 루트 기준)임을 문서에
- **D-102 자동 시각**: `time.Time` 필드의 태그 `auto=create`·`auto=update`. 생성된 CRUD의 Insert가 둘을, Update가 `update`를 채운다. D-83의 닫힌 목록에 더한다(M1의 태그 검사와 골든이 는다, 7번). admin·폼은 읽기 전용으로 본다. 손으로 쓴 `UPDATE`는 스스로 채운다. 기각: 모델 메서드(D-15, admin이 모른다), DB 기본값(수정 시각에 트리거가 필요), 두지 않음(잊으면 `0001-01-01`)
- **D-103 게시판 튜토리얼**: 이 저장소 `examples/board/`의 패치 묶음(`steps/NNN-이름.patch`, `git format-patch` 모양, 생성물 제외)과 단계마다의 기대 상태 목록(통과, 또는 일부러 틀린 단계의 기대 문구). 걷는 것은 `cmd/maago` 모듈의 Go 테스트: 단계마다 패치 적용 → `maago gen` → `check` → 임시 SQLite 파일에 `.env`와 `maago migrate` → 앱의 `go test`(`go mod edit -replace`로 이 저장소를 가리킨다). M3의 앱 테스트는 생성 함수를 직접 부르는 Go 테스트다(U-07은 M4). 가운데 단계를 고치는 스크립트(임시 git 저장소에 풀고 rebase하고 다시 뽑기)도 10번에서. 기각: `git bundle`(리뷰에 diff가 안 보인다), 별도 저장소(이 저장소 CI가 모른다), 단계마다 완성 디렉터리(복제)
- **D-104 새 결과 타입의 컬럼 타입과 NULL**: 모델 테이블의 컬럼을 그대로 고른 결과 컬럼(드라이버가 출처를 주고, 그 테이블이 바깥 조인의 NULL 쪽이 아닌 것)은 그 필드의 Go 타입을 쓰고 선언하면 오류. 그 밖(출처가 없는 식, 바깥 조인의 NULL 쪽)은 머리에 반드시 선언: `-- column: n int64`, `-- column: author_name? string`. 방언과 상관없다. PG는 선언을 OID와 맞춰 본다. 바깥 조인은 토큰으로(`LEFT [OUTER] JOIN 테이블 [AS] 별칭`의 테이블, `RIGHT`·`FULL`이면 모든 테이블), 분류할 수 없는 모양(`FROM`의 서브쿼리, `NATURAL`, 괄호 조인)은 오류. 판정이 틀리면 실행 때 Scan 오류가 된다(조용하지는 않다). 규칙 1의 "결과"를 넓힌 것이라 규칙 수는 그대로(D-24). 기호 표의 `?`는 "없을 수 있다 → 포인터"로 고쳤다. 기각: 모델 컬럼만(`count(*)`를 못 쓴다), 사용자가 struct를 씀(D-23과 부딪힌다)
- **D-105 SQLite의 시각**: UTC `YYYY-MM-DD HH:MM:SS.ffffff`(공백, 마이크로초 6자리 고정, 표시 없음). 읽기는 이 모양과 RFC 3339. 변환은 런타임 도우미 하나를 생성 코드·CRUD·admin·auth가 함께 쓴다. SQLAlchemy와 같은 모양이고(Django는 소수부가 0이면 생략), 글자 순서가 시간 순서, `datetime('now')`와 비교가 맞고, PG와 정밀도가 같다. 처음 추천한 `T…Z` 고정 폭은 손으로 쓴 `datetime('now')` 비교가 조용히 틀려 버렸다(사람이 Django·FastAPI를 물어 다시 봄). M1 표의 "RFC 3339"를 고쳤다. `maago_migrations.applied_at`(RFC 3339, maago 자신의 표)은 그대로 둔다

---

## 모양 (작업들이 함께 따르는 것)

M3 전체의 뼈대. 작업마다 계획에서 구체화하고, 여기와 달라지면 여기부터 고친다.

```
 앱/queries/*.sql ─▶ 해석(머리, 본문 토큰) ─▶ 검사(규칙 1~4, 조합·번역, 규칙 5의 정적 오류)
 모델 ─▶ PK CRUD의 SQL(7번) ──────────────────────────┐
                                                      ▼
 마이그레이션을 적용한 연습용 DB A (M2 비교가 통과한 뒤, 같은 연결)
   조합마다 prepare + 묘사 ─▶ 결과 타입 검사(D-23) ─▶ 생성: 앱/queries_gen.go, CRUD
```

- **파일**: 앱 디렉터리의 `queries/*.sql`, 파일 하나에 쿼리 하나(규칙 1). `queries/`가 있으면 모델이 없어도 앱이다(ARCHITECTURE §5). 그런 앱의 결과는 새 이름만 될 수 있다
- **머리**: `-- name: 이름 :종류 [결과]`, `-- param: 이름[?] 타입`, `-- column: 이름[?] 타입`(D-104). 머리는 본문 앞의 주석 줄이다. `--` 뒤가 `단어:` 꼴(`-- name : X`, `-- parm:`)이면 머리 줄로 보고 모르는 키는 did you mean(D-17). 그 밖의 주석은 설명이다(동작을 바꾸지 않는다)
- **종류**는 닫힌 목록 `:one`·`:many`·`:exec`. `:one`은 행이 없으면 `sql.ErrNoRows`를 그대로(M4가 404로), **두 행 이상이면 오류**(`QueryRowContext`처럼 첫 행을 조용히 쓰지 않는다). `:exec`는 결과 타입이 없고 영향받은 행 수를 돌려준다
- **파라미터 타입**: D-77의 타입, 그 슬라이스(규칙 2), 선택지(`latest/amount`). `?`는 포인터 필드(없음 = `nil`, D-75와 같은 표기). `bool`에는 `?`를 붙일 수 없다(`{flag}`가 "있음"인지 "참"인지 두 뜻이 된다. 세 값이 필요하면 선택지). 슬라이스에도 `?`를 붙이지 않는다: 슬라이스는 `nil`·빈 것이 "없음"이고 `{ids}`는 원소가 있을 때 참이다. 라벨이 보장하지 않는 자리의 빈 슬라이스는 실행 때 오류(`IN ()`은 SQL이 아니다)
- **모든 `:이름`은 머리에 선언되어야 하고, 선언한 파라미터는 쓰여야 한다.** 규칙 1의 해석이고 새 규칙이 아니다. 11번에서 DECISIONS에 적어 D-24 "다섯 개"가 지켜진 것이 보이게 한다. DECISIONS의 `SearchOrders` 예제는 `:limit`을 선언하지 않았다 → 예제 원문에 `-- param: limit int64`를 더했다(이 초안 커밋). 3·8번의 픽스처는 그 원문과 글자까지 같다
- **Go 쪽 모양**(TUTORIAL-FUTURE §6): `func SearchOrders(ctx context.Context, db maago.DB, p SearchOrdersParams) ([]OrderSummary, error)`. 파라미터가 없으면 `p`도 없다. 선택지는 이름 붙은 문자열 타입과 상수, 모르는 값은 실행 때 오류(조용히 기본값으로 가지 않는다). 쿼리 이름·파라미터 타입·새 결과 타입·CRUD 이름이 같은 패키지의 사용자 식별자(뷰 `PostList` 등)나 서로와 겹치면 생성 오류(M1이 이미 `go/ast`로 패키지를 읽는다)
- **조합**: 조건 라벨이 가질 수 있는 값(선택 파라미터 있음·없음, bool 참·거짓, 슬라이스 원소 있음·없음, 선택지의 각 값)의 모든 조합을 만들고, 번역한 SQL 글이 같은 것은 하나로 친다. 상한 제안: 원 조합 256개, 글 64개. 넘으면 생성 오류(fix: 쿼리를 나누거나 Go에서 bool로). 근거: M2 12번의 PG `check` 약 105ms, 글 64개의 prepare는 그 안팎(3번에서 잰다, D-65)
- **번역**: 생성기가 조합마다 고정된 SQL 조각을 Go 코드로 쓴다(D-22). 런타임은 SQL을 해석하지 않는다. 자리표시는 조합의 글마다 스칼라에 `1…n`을 생성 때 고정으로 주고(PG `$n`, SQLite `?n`), 슬라이스만 실행 때 `n+1…`로 뒤에 붙인다(런타임이 만드는 글은 슬라이스 자리뿐, D-108). `::`(PG 형 변환), 문자열·주석·달러 인용 안의 `:`·`{`는 파라미터나 조건이 아니다(M2 3번의 토큰화)
- **prepare 자리**: M2의 DB 검사 단계에서 비교가 통과한 뒤, 같은 연습용 DB A의 연결에서. PG는 쿼리마다 `SAVEPOINT s` → prepare·묘사 → 실패면 `ROLLBACK TO SAVEPOINT s` → 어느 쪽이든 `RELEASE SAVEPOINT s`로 실패를 모두 모은다(D-61). 슬라이스는 원소 1개와 2개로 두 번 prepare한다(자리표시 개수·순서)
- **`makemigrations`는 쿼리 파일을 읽지 않는다**(`migrate`가 모델을 읽지 않는 것과 같다). 잘못된 `.sql` 하나나 아직 없는 컬럼을 쓰는 쿼리가 마이그레이션 초안을 막으면 안 된다
- **런타임**: 루트 패키지 `maago`에 작은 것만(`DB` 인터페이스, 슬라이스 펼치기, `:one`의 두 행 검사, D-105의 시각 변환, D-101). stdlib만(D-09), reflect 없음(D-66)

---

## 1. 매퍼 파일 읽기와 머리 (`internal/mapper`)

- 앱 디렉터리의 `queries/*.sql`을 읽는다(모양 절: `queries/`만 있어도 앱)
- 머리 줄 해석: `name`(Go 식별자, 대문자로 시작, 앱 안에서 겹치지 않음), 종류, 결과 이름, `param`(이름·`?`·타입), D-104의 `column`. 머리 줄 판정은 모양 절대로
- 오류 위치는 `파일:줄:칸`(D-62). 독립 오류는 모두(D-61)

→ 확인: 표 테스트. 정상 머리(세 종류, 결과 있음·없음, 선택 파라미터, 슬라이스, 선택지, 설명 주석 줄), 잘못된 것 각각(이름 없음, 소문자 이름, 모르는 종류, `:exec` + 결과, `:many` + 결과 없음, 모르는 타입, `bool?`, `[]int64?`, 같은 파라미터 두 번, `-- parm:`의 did you mean, `-- name : X`, 같은 이름의 쿼리 두 파일)이 정확한 위치와 fix로

### 계획

파일
- `internal/project/project.go`: `GoFiles`의 걷기를 `walk(root, keep)`로 빼고 `QueryFiles(root) ([]string, []diag.Diagnostic)`를 더한다. 루트 링크 해석, 읽기 오류 진단 뒤 계속, 정렬, 제외 규칙(`./...`: `.`·`_`·`testdata`·`vendor`·하위 모듈, 링크 파일)이 `GoFiles`와 같다. 고르는 것: 부모 디렉터리 이름이 정확히 `queries`인 `*.sql`(`.`·`_`로 시작하는 것 빼고). 바로 아래만: `queries/a/x.sql`은 아니다
- `internal/diag/diag.go`: `model`의 `suggest`·`distance`를 `diag.Suggest`로 옮긴다(동작 그대로). 전용 테스트가 없었으므로 `diag`에 `TestSuggest` 표를 새로 쓴다(접두사 우선, 길이 1/3 올림 거리, 동률은 앞 이름)
- 새 `internal/mapper/header.go`: `Read(root string, files []string) ([]*App, []diag.Diagnostic)`. 앱 = `queries/`의 부모 디렉터리(루트면 `.`). 머리에 오류가 있는 파일은 `Query`를 내지 않는다(2번이 연쇄 오류를 쏟지 않게, D-61). 앱 안에서 쿼리 이름이 겹치면 경로 순으로 첫 파일 뒤의 파일마다 오류(메시지에 첫 파일)
- 새 `internal/mapper/header_test.go`: 표 테스트(글 → `diag.Write` 출력 전체, 골든 `testdata/errors.golden`). `QueryFiles` 표는 `internal/project/project_test.go`

모양
```go
type App struct { Dir string; Queries []*Query }
type Query struct {
	File      string         // 루트 기준 슬래시 경로
	Src       string         // 파일 전체. 2번은 이것을 통째로 dialect.Split에 넘긴다(머리는 -- 주석이라 토큰이 없다)
	HeaderEnd int            // 머리 뒤 첫 바이트의 오프셋(원본 기준)
	Name      string; NamePos token.Position
	Kind      Kind           // One·Many·Exec
	Result    string         // :exec면 ""
	Params    []Param        // 선언 순서(파라미터 struct의 필드 순서가 된다)
	Columns   []Column       // D-104
}
type Param  struct { Name string; Pos token.Position; Optional bool; Type Type }
type Column struct { Name string; Pos token.Position; Optional bool; Type model.Type }
type Type   struct { Base model.Type; Slice bool; Choices []string } // 선택지면 Base 0
```
위치는 `dialect`처럼 `token.Position`, 칸은 바이트(Go와 같다). 글을 다시 만들지 않으므로 오프셋은 원본과 같다(codex·fable 1).

머리 규칙 (결정에 없는 선택은 ★)
- 줄은 `\n`으로 나누고 줄 끝 `\r`은 무시한다(`\r`만 쓰는 파일은 한 줄이다. `Split`과 같다). 공백은 ASCII 공백·탭
- 머리 = 파일 처음부터, 빈 줄이거나 (앞 공백 뒤) `--`로 시작하는 줄들. 다른 줄이 오면 거기서부터 본문. `/* */`는 본문의 시작
- `--` 뒤(공백 무시)가 `단어` + (공백) + `:`이면 머리 줄. `단어`는 `[A-Za-z_][A-Za-z0-9_]*`. 키는 `name`·`param`·`column`, 모르면 did you mean과 `allowed: …; start a comment with something other than "word:"`. ★ `-- Note: …`와 `-- https://…`도 머리 줄로 보고 오류다(모양 절 그대로). `-- maago: …`도 모르는 키(마이그레이션 표시는 쿼리에 없다). `-- name : X`처럼 키와 `:` 사이 공백은 오류(fix: `-- name: X`)
- ★ 같은 꼴(`-- 단어:`)의 주석이 본문에 있으면 2번에서 오류다(fix: 머리로 옮긴다). 주석의 뜻이 위치에 따라 갈리지 않게(PHILOSOPHY 4). 결정은 여기서, 검사는 2번
- `name`: `이름 :종류 [결과]`. 이름·결과는 ★ `^[A-Z][A-Za-z0-9_]*$`(ASCII 내보내는 Go 식별자. 생성 이름 `SearchOrdersParams`·상수로 이어 붙고 한 가지 꼴만 둔다). 종류는 `:one`·`:many`·`:exec`, 모르면 did you mean, `:` 빠지면(`many`) 그 fix. 종류 없음, `:exec` + 결과, `:one`·`:many` + 결과 없음, 남는 단어, 두 번째 `name` 줄은 오류. `name` 줄이 없으면 `파일:1:1: query has no -- name: line`(fix는 SQL 앞 첫 줄에 쓰라고, `/* */` 뒤의 `--`는 머리가 아니라고)
- `param`: `이름[?] 타입`. 이름은 ★ `^[a-z][a-z0-9_]*$`(SQL 쪽 이름. Go 필드 이름은 D-76의 거꾸로, 5·8번). `?`는 이름에 붙여 쓴다(`status ?`는 오류). 타입은 D-77의 7개(`[]byte`는 기본 타입), `[]` + 그것(슬라이스, `[][]byte`도), 또는 선택지 `a/b[/…]`. 타입 없음, 남는 단어는 오류. ★ 선택지는 2개 이상, 값은 param 이름과 같은 꼴(Go 상수 이름이 되므로 모델 `choices`보다 좁다), 겹치면 오류. `*int64`는 오류(fix: `name? int64`). 모르는 타입은 did you mean(`int` → `int64`)
- `?` 금지: `bool`(모양 절), 슬라이스(모양 절, `[]byte`는 슬라이스가 아니므로 `data? []byte`는 된다), ★ 선택지(없음이 필요하면 값 하나를 더한다: `none/latest/amount`. 조건 값의 종류를 늘리지 않는다)
- `column`: `이름[?] 타입`, 타입은 D-77의 7개만(슬라이스·선택지는 오류). 같은 이름 두 번은 오류. 판정(선언이 필요한지)은 5번
- 한 줄에 오류가 여럿이면 첫 오류만(태그 읽기와 같다). 줄끼리는 독립이라 모두 모은다(D-61)

하지 않기로 한 것
- 본문(`:이름` 사용·미사용, 본문의 `-- 단어:` 주석, `name` 줄 뒤 본문이 빔)은 2번
- 쿼리 이름과 모델·사용자 식별자의 충돌, 파라미터 이름의 Go 필드 충돌은 5·8번(모양 절)
- 파이프라인 연결은 9번. 열린 질문(9번): `.go` 파일 없이 `queries/`만 있는 디렉터리의 package 이름

단계
1. `diag.Suggest` 옮기기 → 확인: `TestSuggest`, `go test ./internal/model ./internal/diag`(골든 바뀌지 않음)
2. `project.QueryFiles` → 확인: 임시 트리(`a/queries/x.sql`, `queries/y.sql`, `a/queries/sub/z.sql`, `testdata/queries/t.sql`, `vendor/queries/v.sql`, `a/queries/_skip.sql`, `a/queries/.dot.sql`, `a/queries/x.txt`, 하위 모듈의 `queries/`, 링크 파일)에서 앞의 둘만, 루트가 링크여도 같음. 없는 루트는 오류 하나
3. `mapper.Read` → 확인: 위 표 테스트의 경우 전부 + (`-- Note:`, `-- https://x`, `sort? a/b`, `*int64`, `a//b`, 선택지 하나, 겹친 선택지, 두 `name` 줄, `column` 두 번, `column`의 슬라이스·선택지, 타입 없는 `param`, 종류 없는 `name`, 남는 단어, `/* */` 뒤의 `-- name:`, 탭·공백으로 들여 쓴 머리 줄의 칸, `data? []byte`와 `[][]byte`, 같은 이름 세 파일, 한 파일의 잘못된 줄 둘은 둘 다 나오고 `Query`는 없음)이 정확한 `파일:줄:칸`과 fix로. 정상 경우는 `Query` 값 비교, `\r\n` 파일의 `HeaderEnd`가 원본 바이트 오프셋
4. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: `Body`/`BodyPos` 대신 `Src`·`HeaderEnd`(둘 다), `QueryFiles`의 오류·제외 계약과 표(codex), 공백·줄바꿈 정의(codex), 오류 난 파일은 `Query` 없음·잘못된 줄 둘(fable), `[]byte` 규칙(fable), 본문의 `-- 단어:`를 결정으로(fable), 확인 보강(fable), `TestSuggest` 새로(fable), 선택지 값 꼴의 이유·ASCII의 이유(둘 다), `1:1`(fable), package 이름은 9번 열린 질문(fable)
- 받지 않음: codex 5 "`suggest` 주석과 구현이 다르다" — 주석은 "접두사가 맞는 이름이 이기고 동률은 앞 이름"이고 구현과 같다

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `file` 필드, 겹침 검사용 map(앱 안 `IndexFunc`로), 공백 건너뛰기를 `TrimLeft`로. 받지 않음 — did you mean fix를 만드는 도우미(`unknownType`의 `[]` 붙이기 때문에 넷째 호출에서도 줄지 않는다)
- codex: 받음 — 쿼리가 없는 디렉터리는 `App`을 만들지 않는다. 받지 않음 — "선택지 하나(`x latest`)가 통과한다": `/`가 없으면 선택지가 아니라 `unknown type`이다(표 줄을 더해 보였다). 샌드박스가 `/tmp` 쓰기를 막아 테스트를 돌리지 못했다(코드만 읽은 리뷰)
- fable(변이 실험으로 겹친 이름·`?` 금지·오류 파일의 `Query` 없음 등이 잡히는 것을 확인): 받음 — fix 문구 전체를 골든으로, 메시지에 실제 이름(`write x? int64`, `{x}`), 빈 파일·`column: flag? bool`·읽기 실패 뒤 계속·앱 수 단언, `? int64`는 "no name". 받지 않음 — `x a/`의 문구(값이 빈 것이 곧 원인이다), `-- name:Q`(콜론 뒤 공백 없음)도 받음: 키 뒤는 공백으로 나누므로 뜻이 하나다. `-- Note:`가 `"name"`을 제안하는 것은 `diag.Suggest`의 규칙대로 둔다

## 2. 본문: 바인딩, 조건 줄, `{ }` 구역 (규칙 2·3·4)

- M2의 토큰화(`dialect`)로 본문을 읽어 `:이름`, 줄 머리의 `{조건}`, `WHERE {`·`SET {` 구역을 찾는다
- 조건 문법(규칙 3): `이름`, `이름=값`, `!이름`을 `&` 또는 `|` 한 종류로. 섞기·괄호·모르는 이름·선택지에 없는 값은 오류. `{이름}`은 선택 파라미터면 "있음", bool이면 "참", 슬라이스면 "원소가 있음". 그 밖의 타입(필수 `int64` 등)은 오류(늘 참이라 뜻이 없다)
- 구역(규칙 4): 구역 안의 줄은 조건 줄이거나 늘 들어가는 줄. 앞의 `AND`/`OR`(WHERE)와 끝의 쉼표(SET)를 정리할 자리를 표시해 둔다. 구역 밖의 `{ }`는 줄 머리 조건뿐
- 선언되지 않은 `:이름`, 쓰이지 않은 파라미터는 오류

→ 확인: 표 테스트. `::text`·문자열 속 `:x`·`'{1,2}'`·주석 속 `{a}`·`$$…:x…$$`는 무시된다. `{a & b | c}`, `{(a)}`, `{sort=oldest}`(선택지 밖), `{limit}`(필수 int64), 선언되지 않은 `:x`, 쓰이지 않은 파라미터 각각 위치 있는 오류. 닫히지 않은 구역

### 계획

파일
- `internal/dialect/split.go`: splitter에 플래그 둘(`markers`: `-- maago:` 표시 해석, `comments`: `--` 주석을 따로 모음). `Split`은 지금과 같이 표시 켬·주석 끔, 새 `SplitComments(filename, src) ([]Stmt, []Token, []diag.Diagnostic)`는 표시 끔·주석 켬(새 종류 `Comment`, 글은 `--`부터 줄 끝까지). 진단은 하나도 버리지 않는다(codex·fable 2). 본문의 `-- 단어:` 주석(1번의 ★)을 찾으려면 문자열·`/* */` 안이 아닌 진짜 주석의 위치가 필요하고, 그것을 아는 것은 토큰화뿐이다. 머리 줄도 주석이므로 `Offset < HeaderEnd`인 주석은 건너뛴다
- 새 `internal/mapper/body.go`: 머리를 읽은 쿼리의 본문을 읽어 `Query.Lines`를 채운다. `Read`가 머리 다음에 부르고, 본문에 오류가 있는 파일도 `Query`를 내지 않는다(1번과 같은 이유)
- 새 `internal/mapper/body_test.go`: 오류 표(골든 `testdata/body.golden`, 글 → `diag.Write` 전체)와 `Lines`를 한 줄씩 적은 덤프 비교(`SearchOrders` 원문 포함)

모양 (3번이 조합마다 이것을 걸러 SQL 글을 만든다)
```go
type Line struct {
	Pos     token.Position // 조건 뒤 첫 토큰
	Cond    *Cond          // nil이면 늘 들어간다
	Kind    LineKind       // Text, Open(WHERE {·SET {), Close(})
	Keyword string         // Open: 쓴 그대로의 WHERE·SET
	Lead    string         // WHERE 구역의 줄: 맨 앞 AND·OR(쓴 그대로). Parts에 없다
	Trail   bool           // SET 구역의 줄: 끝 쉼표. Parts에 없다
	Parts   []Part         // SQL 조각과 바인딩이 번갈아
}
type Part struct { SQL, Param string; Pos token.Position } // 둘 중 하나
type Cond struct { Pos token.Position; Or bool; Terms []Term }
type Term struct { Pos token.Position; Not bool; Name, Value string }
```
- 줄 = 시작 줄이 같은 토큰들(토큰은 시작 위치로 줄에 묶인다. `/* */`는 어느 줄에도 토큰을 더하지 않는다. 여러 줄 문자열 토큰은 시작한 줄에 속한다). `Parts`의 SQL은 원문을 바이트 그대로 자른 것이다: 첫 토큰부터 끝 토큰까지(토큰 끝 = `Pos.Offset+len(Text)`), 바인딩 `:이름` 자리만 `Param`으로. 그래서 `Parts`를 이으면(바인딩은 `:이름`으로) 원문과 바이트까지 같다. 3번의 골든은 원문에서 자리표시를 바꾼 것이 된다
- `DELETE FROM t WHERE {`는 Text 줄 `DELETE FROM t`와 Open 줄로 나눈다
- 정리(규칙 4)는 3번의 몫: 구역에서 살아남은 첫 줄의 `Lead`와 마지막 줄의 `Trail`을 뺀다. `Lead`·`Trail`은 있으면 떼어 둘 뿐, 모든 줄에 요구하지 않는다(가운데 줄에 `AND`를 빠뜨린 SQL은 4번의 prepare가 그 조합과 함께 잡는다)

규칙 (결정에 없는 선택은 ★)
- 문장은 하나(규칙 1): 둘째 문장은 오류, 끝의 `;`는 된다. 문장이 없으면 오류
- 바인딩: ★ `:` 바로 뒤에 붙은 모든 단어이고 앞에 붙은 `:`가 없을 때. `::text`, `: x`는 바인딩이 아니다. `a[1:2]`는 선언되지 않은 파라미터 `2`로 오류다(fix: `a[1: 2]`): SQLite는 `:2`·`:한글`도 제 파라미터로 읽는다(codex 구현 리뷰 3, 처음 계획은 글자로 시작하는 단어만이었다). 선언되지 않은 이름은 did you mean과 fix 끝에 `if it is not a parameter, put a space after the colon`
- ★ 네이티브 자리표시는 방언과 상관없이 오류(fix: `:name`): `$`·`@` 바로 뒤에 붙은 단어(`$1`, `$name`, `@x`. 닫힌 `$tag$…$tag$`는 토큰화가 문자열로 읽고, `foo$bar`는 한 단어라 아니다), 그리고 맨 `?`(`?1` 포함). PG에서 `$1`은 생성기가 매기는 번호와 겹쳐 값이 조용히 바뀌어 들어가고, SQLite의 `?`·`@x`는 드라이버가 파라미터 수를 알려 주지 않아(modernc `NumInput`은 -1) 실행 때에야 드러난다(fable 1, 처음 계획은 4번에 미뤘다). 대가: PG jsonb의 `?`·`?|`·`?&` 연산자를 쓸 수 없다 → fix에 `jsonb_exists`·`jsonb_exists_any`·`jsonb_exists_all`. `?`는 머리에서 "없을 수 있다"는 한 뜻이다(PHILOSOPHY 4)
- 조건: 줄의 첫 토큰이 `{`이고 같은 줄에 `}`. 항은 `이름`, `!이름`, `이름=값`, 잇는 것은 `&` 또는 `|` 한 종류. 오류: 섞기, 괄호, 빈 조건, 닫히지 않음, `!이름=값`, 모르는 이름(did you mean), 조건 뒤에 SQL이 없음
- 항의 뜻(모양 절): `이름`·`!이름`은 선택 파라미터(있음), `bool`(참), 슬라이스(원소 있음)에만. 필수 스칼라·선택지는 오류(늘 참. 선택지는 fix에 `{sort=latest}`). `이름=값`은 선택지에만, 값은 그 선택지 안(did you mean)
- 구역: `WHERE`·`SET`(대소문자 무시) 바로 뒤의 `{`가 그 줄의 마지막 토큰이면 여는 것, 줄에 `}` 하나뿐이면 닫는 것. ★ 구역은 겹치지 않고 괄호 깊이 0에서만 연다(서브쿼리의 `WHERE {`는 오류. 3번이 "WHERE가 사라지는 UPDATE·DELETE"를 볼 때 구역이 최상위인지 따로 묻지 않는다, fable 5). 빈 구역, 닫히지 않은 구역, 짝 없는 `}`, 여는·닫는 줄의 조건, 그 밖 자리의 `{`·`}`는 오류. `WHERE` 다음 줄에 `{` 홀로면 fix는 `WHERE {`를 한 줄에
- ★ 본문의 `-- 단어:` 주석은 오류(1번의 결정, fix: 머리로 옮기거나 다른 말로 시작). 판정은 머리와 같은 함수(fable 3). `-- maago:`도 이것으로 잡힌다(`SplitComments`는 표시를 해석하지 않는다)
- 쓰이지 않은 파라미터(바인딩에도 조건에도 없음)는 그 `param` 줄에 오류
- 오류는 줄마다 첫 것만, 줄끼리는 모두(D-61)

하지 않기로 한 것
- 조합·번역·빈 WHERE의 UPDATE·DELETE·라벨이 보장하지 않는 파라미터는 3번. 3번에 넘기는 확인: PG가 `$1`·`$3`만 쓰고 `$2`가 없는 글을 prepare하는지(못 하면 조합마다 번호를 매긴다. 이 모양은 그대로, fable 5)

단계
1. `SplitComments` → 확인: `dialect` 표에 주석 토큰(문자열·`/* */` 안의 `--`는 아님), 기존 테스트 그대로
2. `body.go` → 확인: 위 원래 확인 전부 + 둘째 문장, 빈 본문, `$1`·`$name`·`@x`·`?`·`?1`(오류)과 `foo$bar`·`$tag$…$tag$`(통과), `{a=b}`(선택지 아님), `{!sort=x}`, `{a b}`, `{}`, `{a`, `{a}`만 있는 줄, `{sort}`, 중첩 구역, 괄호 안 구역, 빈 구역, 짝 없는 `}`, `x = {`, `{a} WHERE {`, `WHERE` 다음 줄의 `{`, 본문의 `-- param:`·`-- maago: allow-drop`, 한 파일의 독립 오류 둘. 통과: `a[1: 2]`·`: x`(바인딩 아님), `:from::date`(바인딩 뒤 형 변환), `:xfoo`는 파라미터 `xfoo`, `data? []byte` + `{data}`, 조건에서만 쓰인 `bool`. 덤프(골든): `SearchOrders`(DECISIONS 원문 그대로), `SET {` + `Trail`, `DELETE … WHERE {` 한 줄, CRLF 파일, `a::text`. 덤프는 줄마다 Kind·Keyword·Lead·Trail·조건(쓴 대로)·Part마다 종류와 위치를 찍는다(무엇이 떨어져도 골든이 바뀐다, fable 4). `dialect`: 주석 토큰 표, `Split`의 표시 진단이 그대로
3. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: 표시·주석 플래그로 나누고 진단을 버리지 않음(둘 다), 줄 묶기 계약(codex 5), `$`·`?`·`@` 경우의 표(둘 다), `?`·`@x`도 2번에서 막음(fable 1: modernc는 파라미터 수를 알려 주지 않는다), 괄호 깊이 0의 구역(fable 5), `Parts`가 원문과 바이트까지 같음(fable 5), 머리와 같은 `단어:` 판정(fable 3), 확인 보강과 덤프 형식(fable 4)
- 받지 않음: codex 1·4 "토큰에 끝 위치가 없다" — `Text`가 원문 `src[i:j]`라 끝은 `Pos.Offset+len(Text)`다. 바인딩마다 `Part.Pos`가 있다

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `typeString`의 닿지 않는 선택지 갈래, 이름 첫 글자 판정을 `isNameStart` 하나로. 받지 않음 — `TestSplitComments`의 "주석이 토큰에 섞이지 않음" 루프(지금 늘 참인 것이 회귀 시험이다), `prev` 인자(읽기 쉽다)
- codex(샌드박스가 임시 디렉터리를 막아 코드만 읽음): 받음 — 오류 난 줄이 있으면 "쓰이지 않은 파라미터"를 모두 숨기던 것을, 그 줄에 나온 이름만 숨기게. `:` 뒤의 모든 단어를 바인딩으로(위 ★). 독립 오류 여럿이 함께 나오는 표 줄
- fable(변이 실험): 받음 — 조건 속 모르는 이름에 "콜론 뒤 공백" fix가 나가던 것, `AND`·`,`만 남은 구역 줄은 오류, 계획 문구, 괄호가 앞 줄에서 열린 `WHERE {`·`SELECT\n{`·구역 밖 정리 없음의 표와 덤프, `Parts`가 원문과 바이트까지 같음을 단언, `!name=value` 문구, `}`의 fix, `Line.Pos` 주석, 덤프에 연산자를 늘 찍기. 받지 않음 — 본문 `-- maago:`의 fix(드물다)

## 3. 조합과 번역 (규칙 3·4·5의 정적 오류)

- 조합을 모두 만들고(모양 절), 조합마다 방언의 SQL 글을 만든다: 조건이 거짓인 줄을 빼고, 구역을 정리하고, 빈 구역은 절째 없애고, `:이름`을 자리표시로. 같은 글은 하나로
- 규칙 5의 정적 오류(DB 없이):
  - 어떤 조합에서 WHERE가 사라지는 `UPDATE`·`DELETE`
  - 어떤 조합에서 비는 `SET { }`(`UPDATE t WHERE …`는 SQL이 아니다. 실행 때 아무것도 하지 않는 숨은 분기를 두지 않는다)
  - 선택 파라미터를, 그 파라미터가 있음을 보장하지 않는 줄에서 씀(조건 줄이 아닌 곳, `|`로 이은 조건, `!이름`)
  - 조합 수가 상한을 넘음
- 오류는 어느 조합인지(라벨의 값)를 메시지에 싣는다

→ 확인
1. `SearchOrders`(DECISIONS 원문, `limit` 선언을 더한 것): 원 조합 16개(status 2 × from·to 4 × sort 2)가 SQL 글 **8개**로 접힌다(from·to는 "둘 다"와 "그 밖"만 글이 다르다). 두 방언의 골든은 **손으로 쓴다**(구현에서 뽑으면 열거·중복 제거가 깨져도 통과한다)
2. 빈 WHERE의 DELETE, 빈 SET의 UPDATE, 라벨이 보장하지 않는 파라미터(세 모양 각각)가 위치 있는 오류(완료 조건 3의 앞 둘). 상한을 넘는 쿼리
3. PG 자리표시: 슬라이스가 가운데 있는 쿼리에서 스칼라의 번호가 고정이다

### 계획

파일
- 새 `internal/mapper/variant.go`: `(q *Query) Variants() ([]Axis, []Variant, []diag.Diagnostic)`와 `(v Variant) SQL(d project.Dialect, n int) string`(슬라이스마다 원소 n개의 자리표시를 넣은 글. 4번은 n=1·2로 prepare, 테스트도 이것을 쓴다. 8번의 생성 코드는 같은 일을 Go로 한다. n<1은 부른 쪽의 버그라 panic: 빈 슬라이스는 8번의 생성 코드가 글을 고르기 전에 실행 오류로 막는다)
- `internal/mapper/body.go`: 여는 줄 `WHERE {`에 그 구역의 문장 `Verb`(아래)를 적는다. 토큰의 괄호 깊이를 아는 곳이 여기다
- 새 `internal/mapper/variant_test.go`, 손으로 쓴 `testdata/*.{sqlite,postgres}.sql`(`golden.Check`가 아니라 파일과 직접 비교: `-update`가 손으로 쓴 것을 덮지 못하게): `search_orders`, `delete_posts`(슬라이스·bool 축, `or` Lead, 슬라이스 자리), `update_post`(SET의 쉼표, 마지막 줄)
- `internal/testdata/orders/queries/search_orders.sql`(커밋한다): DECISIONS의 원문. 8번이 `Order` 모델을 더해 예제 앱으로 만든다(모양 절). DECISIONS의 ```sql 덩어리와 글자까지 같은지 테스트가 본다
- `cmd/maago`: modernc가 `?2, ?1, ?1`을 순번으로 묶는 작은 시험(★ `?n`이 기대는 드라이버 동작을 못 박는다)

모양
```go
type Axis struct {
	Param  string
	Values []string // 선택지면 그 값들, 아니면 "absent","present" / "false","true" / "empty","non-empty"
}
type Variant struct {
	Pieces []Piece  // SQL 조각과 슬라이스 자리
	Args   []string // 스칼라 파라미터, 자리표시 번호 순(Args[i]가 $i+1)
	Slices []string // 슬라이스 파라미터, 번호를 이어 받는 순. 인자는 Args의 값들 뒤에 Slices마다 원소들
	Combos [][]int  // 이 글을 내는 조합들. 조합은 축마다 Values의 번호
}
type Piece struct {
	SQL   string
	Arg   int    // 스칼라 자리표시 번호(1부터). SQL·Arg·Slice 중 하나
	Slice string
	Pos   token.Position // 원문 위치(Part.Pos). 줄까지만 정확하다(조각이 여러 줄을 담고 줄바꿈·들여쓰기·Lead로 시작할 수 있다). 4번이 DB 오류를 원문으로 옮긴다(fable 1)
}
```
- 축 = 조건에 나오는 파라미터, 선언 순. 조합은 축 값의 곱, 앞 축이 느리게 돈다(축이 없으면 빈 조합 하나). 글이 같은 조합은 한 `Variant`로, 순서는 처음 나온 조합 순. 8번은 실행 때 값 → 축마다 번호 → 표 조회
- 자리표시 ★: 조합의 글마다 스칼라에 처음 나온 순으로 1…n(같은 파라미터는 같은 번호), PG `$n`, SQLite `?n`. 슬라이스는 실행 때 n+1부터 `Slices` 순으로 이어서(같은 슬라이스의 두 번째 자리는 같은 번호). 그래서 번호에 빈자리가 없고(PG는 쓰이지 않은 `$2`의 타입을 정하지 못한다, modernc도 빈자리 없는 순번을 요구한다), 스칼라 번호는 슬라이스 길이와 상관없다. 모양 절의 "스칼라에 `$1…$n` 고정"을 "조합마다"로 좁히고 SQLite `?`를 `?n`으로 바꾼다(modernc v1.59.0의 `bind`가 `?NNN`을 순번으로 맞춘다. `?NNN`은 SQLite 문법이고 순번 맞춤은 modernc의 동작이다. 드라이버를 바꾸면 다시 본다, D-89)
- 글 모양: 살아남은 줄마다 원문 그 줄의 앞 공백(줄 시작부터 첫 글자까지, `Src`에서) + `Lead`(있고, 구역의 첫 살아남은 줄이 아니면) + `Parts` + `,`(`Trail`이고 구역의 마지막 살아남은 줄이 아니면). 없는 `AND`·쉼표를 생성기가 채우지 않는다(PHILOSOPHY 5, prepare가 잡는다). 원문의 같은 줄에서 온 것(`DELETE FROM t` + `WHERE`)은 공백 하나로 잇고, 다른 줄은 줄바꿈. 구역 키워드는 그 줄에, 빈 구역은 키워드째 빠진다. `}`는 아무것도 내지 않는다

규칙 5의 정적 오류 (위치 + 어느 조합인지)
- 선택 스칼라 `:x`가 있음을 보장하지 않는 줄에서 쓰임: 줄의 조건이 `&`(항 하나 포함)이고 `x`(`!`·`=` 없이)를 항으로 가져야 한다. 바인딩마다 따로 본다. 위치는 그 바인딩. 조합과 상관없는 판정이라 조합은 싣지 않는다(fix: 줄을 `{x}`로 가두거나 필수로)
- ★ `WHERE { }`가 비는 조합이 있으면, 그 구역의 문장이 `SELECT`가 아닌 한 오류(위치는 `WHERE {`, 처음 비는 조합). 구역의 문장 = 구역 앞에서 괄호 깊이 0의 마지막 `SELECT`·`INSERT`·`UPDATE`·`DELETE`(2번이 구역을 깊이 0에서만 열게 했으므로 구역은 늘 최상위 문장의 것이다). `WITH … SELECT`·`INSERT … SELECT … WHERE { }`는 SELECT라 비어도 되고, `WITH … DELETE`·`UPDATE … FROM (SELECT …) s WHERE { }`는 오류, 그 밖(`MERGE` 등, 못 찾음)은 오류(fable 5). 규칙 5의 "빈 WHERE의 UPDATE/DELETE"를 넓힌 것이라 11번에 DECISIONS로 적는다(규칙 수는 그대로)
- `SET { }`가 비는 조합(문장과 상관없이)
- 조합 256개·글 64개 초과(위치는 `-- name:`, 숫자와 fix: 쿼리를 나누거나 조건을 Go의 bool로). 모양 절의 제안 그대로. prepare 시간은 4번에서 잰다
- 조합의 적기: `status absent, from present, to absent, sort=latest`(값 이름 그대로)

하지 않기로 한 것
- 가운데 줄의 빠진 `AND`·쉼표, 문법 오류는 4번의 prepare
- 코드 생성(조합을 고르는 Go, 빈 슬라이스 오류)은 8번
- 11번에 넘길 것: 규칙 2의 "`?, ?, ?`"와 모양 절의 "SQLite는 `?`"를 `?n`으로, 위 ★ 둘을 DECISIONS에

단계
1. `Variants`·`SQL` → 확인: `SearchOrders` 16조합이 글 8개, 두 방언의 손 골든(조합·`Args`·글)과 같다. DECISIONS 원문과 픽스처가 같다. `delete_posts`·`update_post` 손 골든(가운데 줄 쉼표를 빠뜨린 줄이 그대로 나감 포함). 축 없는 쿼리는 조합 하나·글 하나
2. 정적 오류 → 확인: 빈 WHERE의 `DELETE`(조합 적힘), `WITH … DELETE`, `UPDATE … FROM (SELECT …) s WHERE { }`, 빈 SET의 `UPDATE`는 오류. 늘 비지 않는 WHERE의 `DELETE`, `SELECT`·`WITH … SELECT`·`INSERT … SELECT`의 빈 WHERE는 통과. 보장하지 않는 파라미터 세 모양(조건 없는 줄, `{status | flag}`, `{!status}`)과 `{x & y}`에서 `:z`는 오류, `{x & y}`에서 `:x`·`:y`는 통과. 조합 257개, 글 65개
3. 자리표시: `a = :a AND id IN (:ids) AND b = :b AND c = :a AND d IN (:ids2) AND e IN (:ids)`가 `Args` `[a b]`, `Slices` `[ids ids2]`, 원소 2개로 그리면 `$1`, `($3, $4)`, `$2`, `$1`, `($5, $6)`, `($3, $4)`. SQLite는 `?n`. modernc 시험(`cmd/maago`)
4. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: `Slices`로 인자 순서를 드러냄(codex 1), `Axes`와 번호 조합(둘 다), 빈 슬라이스는 8번이 막고 `SQL`은 panic(codex 4), 구역의 문장을 깊이 0의 마지막 동사로(fable 5, codex 5), `{x & y}`의 `:z`(codex 6), modernc 시험(codex 2), `Piece.Pos`(fable 1), `Trail`·`Lead`가 있을 때만(fable 2), 손 골든 둘 더(fable 3), `Variants`는 방언 없이·`SQL`은 n 하나(fable 6), 들여쓰기는 `Src`에서(fable 7), 문서 어긋남(fable 8)
- 받지 않음: codex 7(모양을 더 나누자) — `Variant`가 이미 그 중간 표현이다

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — 구역의 동사를 한 번만 셈, 동사 비교를 `ToUpper` 한 줄로, `verbName`을 `cmp.Or`로. 받지 않음 — `Piece.Pos`(4번이 바로 쓴다)
- codex(샌드박스 때문에 테스트는 못 돌림): 발견 없음. 손 골든을 규칙으로 따로 검산했다
- fable(손 골든 검산, 변이 24개 중 19개를 잡음): 받음 — 빈 구역이 키워드째 빠짐, 같은 글·다른 인자의 두 변형, 상한 경계(256·64 통과), 구역마다 한 번·처음 비는 조합과 fix까지 비교, `Piece.Pos`는 줄까지만 정확하다고 적음, 모양 블록의 `Arg`, `Variants`의 전제(오류 없이 읽힌 쿼리). 받지 않음 — 선택지 값이 `absent`·`true` 같은 이름일 때 적기의 `=`(메시지 표기뿐)

## 4. 연습용 DB에서 prepare와 묘사 (`internal/mapper`, `cmd/maago`의 드라이버 함수)

- 3번의 조합마다(슬라이스는 원소 1·2개로) 연습용 DB A에서 prepare. 실패는 그 쿼리 파일의 위치와 DB 메시지, 어느 조합인지. PG는 모양 절의 `SAVEPOINT` 절차로 다음 쿼리를 이어 간다
- 묘사(`practicedb`에 함수 자리를 두고 `cmd/maago`가 꽂는다, `ErrorPosition`과 같은 모양, 먼저 확인한 사실의 경로): 결과 컬럼마다 이름·출처 테이블·출처 컬럼·(PG) 타입 OID, (PG) 파라미터 타입 OID. PG의 테이블 OID는 연습용 스키마의 `pg_class`로 이름을 찾는다
- PG는 파라미터 타입을 선언과 맞춰 본다. 받는 OID는 모델 필드가 만드는 타입(M1 표)과 같게: `int64`↔`int8`, `int32`↔`int4`, `string`↔`text`·`varchar`·`bpchar`, `float64`↔`float8`, `bool`↔`bool`, `time.Time`↔`timestamptz`, `[]byte`↔`bytea`. `unknown`·도메인·배열 등 표 밖은 오류(fix: `::타입`으로 형 변환). SQLite는 파라미터 타입을 모른다(맞춰 보지 않는다고 문서에 적는다)
- 한 쿼리의 조합들이 서로 다른 결과 컬럼을 내면 오류(`{sort=…}`가 SELECT 목록을 바꾸는 경우)

→ 확인 (`cmd/maago` 모듈, 두 방언)
1. 없는 컬럼, 문법 오류, 조합 하나에서만 나는 오류가 각각 그 조합을 짚는다. 한 실행에서 여러 쿼리의 오류가 모두 나온다: PG에서 첫 쿼리가 실패해도 뒤 쿼리의 오류가 나오고, 성공한 쿼리 뒤의 쿼리도 묘사된다(`ROLLBACK TO`·`RELEASE`를 하나씩 빼면 실패)
2. PG 파라미터 타입 불일치(`status int64`인데 컬럼이 `varchar`)가 오류. `int32` 컬럼에 `int64` 선언도 오류
3. 조합마다 결과 컬럼이 다른 쿼리가 오류
4. 묘사 표 테스트(직접 컬럼, 별칭, JOIN, 식, 바깥 조인)가 "먼저 확인한 사실"과 같다. 연습용 DB를 닫은 뒤 PG에 prepared statement가 남지 않는다

### 계획

파일
- `internal/practicedb/practicedb.go`: `Describe` 자리와 메서드.
  ```go
  type Column struct {
  	Name          string // 결과 컬럼 이름
  	Table, Origin string // 출처 테이블·컬럼(연습용 스키마의 것). 식이면 ""
  	Type          uint32 // PG 타입 OID, SQLite는 0
  }
  type Description struct { Columns []Column; Params []uint32 } // Params: PG 파라미터 OID, 번호 순. SQLite는 nil
  var DescribeQuery = func(ctx context.Context, conn *sql.Conn, query string) (Description, error) // cmd/maago가 꽂는다(ErrorPosition과 같은 모양)
  func (p *DB) Describe(ctx context.Context, query string) (Description, error, []diag.Diagnostic) // error: 쿼리의 잘못, 진단: 연결·SAVEPOINT의 실패(쿼리 탓이 아니다)
  func (p *DB) Dialect() project.Dialect
  func (p *DB) TypeName(ctx context.Context, oid uint32) string // PG format_type, 불일치 메시지에만
  ```
  `Describe`는 PG에서 `SAVEPOINT maago_describe` → `DescribeQuery` → 실패면 `ROLLBACK TO SAVEPOINT maago_describe` → 어느 쪽이든 `RELEASE SAVEPOINT maago_describe`(모양 절). 드라이버 없는 빌드의 기본값은 오류
- `cmd/maago/main.go`: `DescribeQuery`를 꽂는다. PG: `conn.Raw` → `*stdlib.Conn` → `Conn()` → `PgConn().Prepare(ctx, "", query, nil)`(이름 없는 문장: 서버에 남지 않고 다음 문장이 덮는다. 먼저 확인한 사실의 `Deallocate`가 필요 없다), 출처는 `TableOID`·`TableAttributeNumber`를 묘사마다 한 번의 조회(`pg_attribute JOIN pg_class`, `attrelid = ANY($1)`, `relnamespace = current_schema()::regnamespace`)로 이름에(연습용 스키마 밖·카탈로그는 출처 없음, fable 5). SQLite: `Raw`의 driverConn을 `interface{ ColumnInfo(string) ([]sqlite.ColumnInfo, error) }`로 단언, `TableName`·`OriginName`(`DeclType`은 버린다: 출처가 있는 컬럼은 모델 필드의 타입을 쓰고, 출처 없는 식은 선언이 필요하다, D-104. 뷰는 D-100이 막아 연습용 DB에 없다)
- 새 `internal/mapper/describe.go`: `Describe(ctx, db *practicedb.DB, q *Query, axes []Axis, vs []Variant) ([]practicedb.Column, []diag.Diagnostic)`. 변형마다(슬라이스가 있으면 원소 1·2개로 두 번) `db.Describe`. 9번이 쿼리마다 부르고, 5번이 돌려받은 컬럼으로 결과 타입을 본다
- `internal/mapper/variant.go`: `renderer`가 `text` 부를 때마다 조각을 따로 둔다(합치지 않는다). 그러면 `Parts`에서 온 조각의 `Pos`는 칸까지 정확하고, 줄바꿈·들여쓰기·Lead 조각만 그 줄의 위치다(3번 fable 5). 새 `(v Variant) Position(d, n, char int) token.Position`: 글의 `char`번째 글자(1부터, PG `Position`이 세는 것)가 원문의 어디인지
- 새 `cmd/maago/describe_test.go`(두 방언, 연습용 DB에 표를 만들고)

오류 (위치 + 어느 조합인지, D-61·D-62)
- prepare 실패: 위치는 PG면 `ErrorPosition`을 `Position`으로 옮긴 곳, 없으면(SQLite) `-- name:` 줄. 메시지는 DB 메시지 + ` (when status absent, …)`, 조합이 하나뿐이면(축 없음) 붙이지 않는다. fix는 `fix the SQL`. 한 쿼리 안에서 위치·DB 메시지가 같은 오류는 처음 조합의 것 하나만(변형 여럿이 같은 줄에서 같은 이유로 실패한다)
- 결과 컬럼이 변형마다 다름(이름·출처·PG 타입 중 하나라도): `-- name:`에, 두 조합과 각각의 컬럼 이름 목록. fix: 결과 컬럼은 `{조건}`으로 바꾸지 말 것
- PG 파라미터 타입: `Params[i]`가 선언의 받는 OID 안에 있어야 한다. 표(M1이 만드는 타입과 같게): `int64` 20, `int32` 23, `string` 25·1043·1042, `float64` 701, `bool` 16, `time.Time` 1184, `[]byte` 17, 선택지는 `string`. 슬라이스는 원소의 표로 원소 자리마다. 위치는 그 파라미터가 처음 쓰인 자리(변형의 그 번호 조각). 메시지 `parameter status is string, but PostgreSQL reads :status here as integer`(사용자가 쓰지 않은 `$1`을 보이지 않는다, fable 4. 타입 이름은 `TypeName`), fix `declare it as int32, or write :status::text if the SQL needs a text there`(선언을 먼저: 컬럼과 비교하는 자리의 캐스트는 새 오류를 낸다, 4번 구현 리뷰 fable 1. 선택지·슬라이스는 `compare it with a column of type text`). 변형·원소 수를 가로질러 같은 것은 하나만
- SQLite는 파라미터 타입을 모른다: 맞춰 보지 않는다(11번에 ARCHITECTURE·TUTORIAL-FUTURE로)
- `:exec`인데 결과 컬럼이 있음(`RETURNING`): `-- name:`에, 컬럼 이름들. fix `use :one or :many, or remove RETURNING`(결과를 조용히 버리지 않는다, PHILOSOPHY 7, fable 7)
- 연결·`SAVEPOINT`의 실패는 쿼리 오류가 아니라 그 진단 그대로(연습용 DB를 더 쓸 수 없으니 거기서 멈춘다)
- prepare 실패가 있는 쿼리는 결과 컬럼 비교를 하지 않는다(연쇄)

하지 않기로 한 것
- 결과 타입(D-23·D-104)은 5번. 이 작업은 컬럼 묘사를 넘길 뿐
- 파이프라인 연결은 9번. 테스트는 연습용 DB를 직접 연다
- `Describe`가 쓰는 시간(조합 상한의 근거, 모양 절)은 테스트에서 `SearchOrders` 16조합 × 두 방언을 재어 계획에 숫자로 적는다. 잰 값(2026-09-27, 로컬 PG 18): `SearchOrders`(글 8개)와 쿼리 둘, 글 10개의 묘사가 SQLite 0.9ms, PG 5.5ms. 글 64개인 쿼리 하나는 PG 40ms 안팎이라 D-65(PG 3초)에 비해 작다. 상한은 그대로 둔다
- 5번에 넘길 것(fable 7·8): `Column.Table`은 별칭이 아니라 테이블 이름이다(바깥 조인 판정은 테이블 단위가 한계, 한쪽만 바깥인 셀프 조인은 그 테이블 전부가 NULL 쪽). 두 드라이버 모두 CTE를 꿰뚫어 출처를 준다: CTE 안의 `LEFT JOIN`을 보려면 토큰 스캔이 글 전체를 보거나 `WITH`를 "분류할 수 없는 모양"으로. 출처가 모델이 아닌 테이블(손으로 만든 표, PG의 스키마 밖)은 한 규칙("선언하라")
- 8번에 넘길 것: 조각을 `text`마다 나눴으므로 생성 코드는 이웃한 SQL 조각을 이어 쓴다
- modernc `ColumnInfo`는 첫 문장만 prepare한다. 규칙 1이 둘째 문장을 막으므로 충분하다

단계
1. `practicedb.Describe`와 `cmd/maago`의 두 드라이버 함수 → 확인 4: 실패 두 종류(문법 오류: Parse에서, 없는 컬럼: 분석에서)가 두 방언에서 오류로 오고, 묘사 표(직접 컬럼, 별칭 `AS x`, JOIN의 두 테이블, `count(*)`, `LEFT JOIN`의 오른쪽, 파라미터 OID)가 먼저 확인한 사실과 같다. PG에서 묘사 뒤 `pg_prepared_statements`에 `stmtcache_%`(pgx가 자기 조회에 쓰는 것) 밖의 문장이 없다(fable 2)
2. `SAVEPOINT` → 확인 1의 뒤 절: PG에서 실패한 묘사 뒤 다음 묘사가 된다(`ROLLBACK TO`를 지우면 여기서 실패). 끝에 `RELEASE SAVEPOINT maago_describe`를 한 번 더 보내 SQLSTATE `3B001`(`RELEASE`를 지우면 여기서 실패: 같은 이름의 SAVEPOINT는 다시 세워지므로 다른 것은 깨지지 않는다, fable 1). 둘을 하나씩 지워 실패를 보고 되돌린다(WORKFLOW 5)
3. `Position`과 조각 나눔 → 확인: 3번의 손 골든 그대로, 여러 줄 글에서 둘째 줄 바인딩 뒤 글자의 위치가 원문 칸과 같다
4. `mapper.Describe` → 확인 1·2·3: 없는 컬럼(PG는 그 칸, SQLite는 `-- name:`), 문법 오류, 조합 하나에서만 나는 오류(`{flag} AND nope = 1`이 `flag true`를 짚는다), 쿼리 둘의 오류가 한 실행에 모두, `status int64`인데 컬럼이 `varchar`·`int32` 컬럼에 `int64`(PG), 슬라이스 원소 타입, 조합마다 결과 컬럼이 다른 쿼리, 여러 변형이 같은 줄에서 같은 이유로 실패하면 진단 하나(처음 조합), 잘못된 컬럼 앞의 `'한글'`(PG의 글자 수 → 칸), `:exec` + `RETURNING`. 통과: `SearchOrders`(두 방언), `:exec`의 `UPDATE`, `:one`의 `INSERT … RETURNING id`
5. 두 모듈 `go vet`·`go test`(PG는 로컬 서버로), `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: pgx 경로와 "먼저 확인한 사실"의 `Deallocate`(codex 1·2), 실패 두 종류의 표(codex 3의 앞), `DeclType`을 버리는 이유(codex 4), `Position`의 줄바꿈(codex 5: 줄바꿈과 들여쓰기를 따로 조각으로), `RELEASE`의 3B001 확인(fable 1), `stmtcache_%`를 뺀 확인(fable 2), 확인 보강(fable 3), `$1` 대신 `:status`(fable 4), 카탈로그 조회 한 번과 `format_type`(fable 5), `d` 인자 없음·연결 실패는 따로(fable 6), `:exec` + `RETURNING`(fable 7), 5·8번에 넘길 것(fable 7·8·9), 첫 문장만(fable 10)
- 받지 않음: codex 3의 뒤(취소·연결 끊김의 시험) — 둘 다 연습용 DB를 끝내는 경로이고 진단은 그대로 나간다

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `slices.Clone`, 시간 로그 삭제(잰 값은 위에), `MkdirAll` 한 번, `RuneCountInString`, 카탈로그 결과의 문자열 키
- codex(PG 없이 코드만 읽음): 받음 — 준비에 실패한 쿼리는 결과 컬럼을 비교하지 않음(`differ_and_fail`), 파라미터 오류의 중복 키에 OID
- fable(PG로 돌리고 변이 실험, 잡힌 변이 10개): 받음 — 캐스트보다 선언을 먼저 권하는 fix, 살아남은 변이 셋의 사례(슬라이스를 스칼라 자리에: 원소 2개에서만 실패, 카탈로그 표는 출처 없음, 이름이 같고 출처가 다른 컬럼), 여러 줄 문자열 뒤의 위치. 받지 않음 — 슬라이스 오용의 DB 메시지에 보이는 `,`(위치가 이미 `:ids`를 짚는다)
- 5·9번에 넘길 것(fable): 식 컬럼의 이름이 방언마다 다르다(PG `count`, SQLite `count(*)`: `AS`를 요구하는 5번의 규칙이 덮는다). `Describe`는 진단이 있으면 컬럼을 주지 않는다(파라미터 타입만 틀려도): 9번에서 결과 타입 검사를 같은 실행에 돌리려면 파라미터 진단과 컬럼을 함께 주도록 고칠지 정한다

## 5. 결과 타입 (D-23)

- 결과 이름이 모델이면: 결과 컬럼 집합이 그 모델 테이블의 컬럼과 딱 맞아야 한다(출처 테이블까지). 빠진 것·남는 것·다른 테이블의 같은 이름 컬럼은 오류(TUTORIAL-FUTURE §6의 문구). 바깥 조인의 NULL 쪽에서 모델을 고르는 것도 오류
- 새 이름이면: 컬럼마다 필드(이름은 D-76의 거꾸로, `author_id` → `AuthorID`), 타입은 출처 필드 또는 D-104. 같은 이름을 여러 쿼리가 쓰면 모양이 같을 때만(다르면 두 파일 위치의 오류). 모양 절의 이름 충돌
- 컬럼 이름이 Go 식별자가 되지 않거나 두 컬럼이 같은 필드 이름이 되면 오류(fix: `AS`)

→ 확인: 컬럼이 빠진 모델 결과 타입이 오류(완료 조건 3의 셋째). 남는 컬럼, `FROM boards`를 골라 `:many Post`(다른 테이블의 같은 이름), 식별자가 안 되는 컬럼(`count(*)`에 별칭 없음), 두 컬럼이 같은 필드 이름이 각각 오류. 순서만 다른 모델 컬럼은 통과. 두 쿼리가 같은 새 이름을 같은 모양·다른 모양으로. D-104의 경우들(선언이 필요한데 없음, 필요 없는데 있음, 분류할 수 없는 조인, PG에서 선언과 OID가 다름)

### 계획

파일
- 새 `internal/mapper/result.go`:
  ```go
  type Result struct {
  	Name   string
  	Pos    token.Position // 처음 이 이름을 쓴 쿼리의 -- name: (8번의 이름 충돌 오류가 짚는다)
  	Model  *model.Model   // 모델이면 그 모델, 새 타입이면 nil
  	Fields []ResultField  // 결과 컬럼마다 하나, 결과 컬럼 순(Scan 순서). 모델이면 그 컬럼이 들어갈 필드
  }
  type ResultField struct {
  	Name, Column string // Go 필드 이름, 결과 컬럼 이름
  	Type         model.Type
  	Pointer      bool // NULL일 수 있다(D-75)
  }
  func Results(apps []*model.App, dir string, qs []*Query, cols [][]practicedb.Column, typeName func(uint32) string) ([]*Result, []diag.Diagnostic)
  ```
  `typeName`은 PG 타입 OID의 이름(메시지에만, 9번이 `practicedb.DB.TypeName`을 넘긴다). 한 앱(`dir`)의 쿼리들과 4번이 돌려준 컬럼(쿼리마다). 돌려주는 것은 쿼리마다(`:exec`면 nil). 같은 이름의 새 타입은 같은 `*Result`. `cols[i]`가 nil인 쿼리(4번이 진단을 냈다)는 건너뛴다(D-61, 연쇄 "missing"을 쏟지 않는다). 자기 검사에 실패한 쿼리는 같은 새 이름의 모양을 정하지 않는다. DB가 필요 없다: 테스트는 컬럼을 손으로 만든 표(드라이버의 답은 4번이 시험했다). `Result`만으로 8번의 struct와 `Scan` 인자가 정해진다
- `internal/mapper/body.go`: 문장의 토큰을 `Query`에 남긴다(비공개). 바깥 조인 판정이 쓴다
- 새 `internal/mapper/result_test.go`, 골든 `testdata/model_result_errors.golden`·`new_result_errors.golden`. 모델은 `modeltest.Example`로 읽는 작은 예제 앱(`internal/mapper/testdata/example/blog`: Board·Post, 포인터 필드 둘)

모델 결과 (D-23)
- 결과 이름이 같은 앱의 모델이면: 결과 컬럼마다 출처가 그 모델의 테이블·필드 컬럼이고, 바깥 조인의 NULL 쪽이 아니고, 겹치지 않고, 모델의 컬럼을 모두 덮어야 한다. 순서는 상관없다(필드는 결과 컬럼 순으로 대응)
- 어긋남은 한 진단에 모아서(TUTORIAL-FUTURE §6의 문구): `result columns do not match model Post: missing "published_at"; extra "n"; "id" comes from boards; "title" is an expression, not posts.title; "title" is selected twice; "title" is on the outer side of a join`. 위치는 본문의 첫 줄(§6의 `3:1`). fix `select every column of Post, or declare a new result type name (e.g. ":many PostSummary")`
- 모델 결과의 쿼리에 `-- column:` 줄은 오류(선언은 새 타입의 것)

새 타입 (D-23·D-104)
- 결과 컬럼 이름은 `^[a-z][a-z0-9_]*$`이고 서로 달라야 한다. 먼저 본다: 어기면 그것만(fix: `AS snake_name`. SQLite `count(*)`, PG의 대문자 따옴표 이름, `a.id, b.id`). 선언은 결과 컬럼 이름으로 맞춘다
- 필드 이름: 컬럼 이름이 출처 컬럼 이름과 같고 출처가 모델 필드면 그 필드의 이름(`AuthorID`), 아니면(별칭·식·모델 아닌 표) 컬럼 이름의 거꾸로 snake: `_`로 나눠 첫 글자를 대문자로, 조각이 `id`면 `ID`(`board_id` → `BoardID`, `parent_id` → `ParentID`). ★ 약어는 `id` 하나: D-76의 거꾸로가 모델의 `…ID`와 맞으려면 이것이 필요하고, 그 밖의 약어는 이름 휴리스틱이다(PHILOSOPHY 6. 필요하면 필드 이름은 모델에서 오게 쓴다)
- 타입(D-104): 출처가 모델 필드이고 NULL 쪽이 아니면 그 필드의 타입·포인터, 이때 `-- column:` 선언은 오류(선언 줄에, fix `remove it; author_id is posts.author_id (int64)`). 그 밖(출처 없는 식, NULL 쪽, 모델이 아닌 표)은 선언이 반드시 있어야 하고 선언이 타입·`?`를 정한다. 선언이 없으면 `-- name:`에 `result column author_name needs a declaration: posts.author_name is on the outer side of a join`(식이면 `…: it is an expression`), fix는 넣을 줄 그대로 `add -- column: author_name? string`(NULL 쪽 모델 컬럼은 타입을 안다), 식이면 `add -- column: n TYPE`. 쓰이지 않은 선언(결과 컬럼에 없는 이름)도 오류. PG는 선언을 컬럼 OID와 맞춰 본다(4번의 표, 틀리면 선언 줄에, fix `declare it as X`, 표 밖 OID(`numeric`·`date` 등)면 `cast the expression, e.g. sum(amount)::bigint`). SQLite는 맞춰 보지 않는다
- 같은 앱에서 같은 새 이름을 쓰는 쿼리들은 필드(이름·타입·포인터·순서)가 같아야 한다. 다르면 뒤 쿼리의 `-- name:`에, 앞 파일과 다른 첫 필드
- 이름 충돌(쿼리 이름·파라미터 타입·결과 타입·CRUD 이름끼리, 사용자 식별자와)은 8번: 생성할 이름이 모두 정해지는 곳이다(모양 절)

바깥 조인 판정 (D-104, 토큰으로, 문장 전체: CTE·서브쿼리 안의 것도. 드라이버가 CTE·서브쿼리를 꿰뚫어 출처를 주므로 안쪽 조인도 봐야 한다)
- `LEFT [OUTER] JOIN [ONLY] 이름[.이름]`의 끝 이름(따옴표는 `dialect.Name`, 대소문자 무시)이 NULL 쪽 테이블. `RIGHT`·`FULL`이 하나라도 있으면 출처 있는 모든 컬럼이 NULL 쪽(D-104 그대로. 오른쪽의 보존 쪽까지 선언을 요구하지만 안전한 쪽이다)
- ★ `UNION`·`INTERSECT`·`EXCEPT`가 어디든 있으면 모든 컬럼의 출처를 버린다(모두 선언). SQLite는 첫 `SELECT`의 출처를 주고 PG는 주지 않아(fable, 두 드라이버로 잼), 같은 파일이 방언마다 달라지면 안 된다(D-104 "방언과 상관없다"). "먼저 확인한 사실"의 `UNION` 줄은 11번에서 고친다
- 분류할 수 없는 모양은 오류(위치는 그 토큰): `NATURAL`, `LEFT JOIN (`·`LEFT JOIN LATERAL`(안쪽 `JOIN (`은 NULL을 만들지 않고, 그 안의 `LEFT JOIN`은 문장 전체를 훑는 판정이 본다), 조인 대상이 CTE 이름(`WITH [RECURSIVE] 이름 … AS`로 모은 것: 드라이버는 CTE 안의 테이블을 출처로 주므로 이름으로는 NULL 쪽을 알 수 없다, fable 1), 괄호로 묶은 조인(`FROM (` 뒤가 `SELECT`·`WITH`·`VALUES`가 아닌 것), `JOIN` 뒤가 위 모양이 아닌 것. 결과 타입이 있는 쿼리(`:one`·`:many`)에서만. fix: 테이블을 직접 조인하거나 쿼리를 나눈다
- ★ D-104는 "`FROM`의 서브쿼리"를 분류할 수 없는 모양에 넣었지만, 문장 전체를 훑으면 서브쿼리 안의 조인도 보이고 드라이버가 출처를 꿰뚫어 주므로 받는다(`FROM (SELECT …) s`는 통과). 11번에서 D-104를 고친다
- 한계(안전한 쪽으로만 틀린다: 선언을 더 요구할 뿐 조용히 틀리지 않는다): 테이블 이름 단위라 한쪽만 바깥인 셀프 조인, 다른 스코프(CTE·`WHERE`의 서브쿼리)의 `LEFT JOIN t`가 바깥 쿼리의 `t`도 NULL 쪽으로 만든다

하지 않기로 한 것
- 코드 생성(struct와 Scan)은 8번. 이름 충돌도 8번. 8번에 넘길 것: 모델 결과의 필드 대응은 결과 컬럼 순이다
- 모델이 아닌 테이블을 모델 결과로 고르는 것: 모델 결과는 모델 테이블만

단계
1. 바깥 조인 판정 → 확인: 표 테스트(`LEFT JOIN b`, `LEFT OUTER JOIN ONLY s."B" AS x`, `RIGHT JOIN`, `FULL JOIN`, CTE 안의 `LEFT JOIN`, 문자열·주석 속 `LEFT JOIN`은 무시, `UNION`, `NATURAL JOIN`·`LEFT JOIN (SELECT …)`·`JOIN LATERAL`·`LEFT JOIN <CTE 이름>`·`FROM (a JOIN b)`는 오류, `FROM (SELECT …) s`·안쪽 `JOIN`은 통과)
2. 모델 결과 → 확인: 완료 조건 3의 셋째(빠진 컬럼, §6 문구 그대로), 남는 컬럼, `FROM boards`의 `:many Post`, 식(`COALESCE(title,'') AS title`), 두 번 고름, NULL 쪽의 모델(`RIGHT JOIN` 포함), 네 절이 한꺼번에 나오는 골든 하나, 모델 결과의 `-- column:`, 순서만 다른 모델 컬럼은 통과(필드 대응이 컬럼 순)
3. 새 타입 → 확인: 모델 필드 이름을 그대로(`AuthorName` 같은 것), 별칭(`b.name AS board_name` → `BoardName`), 거꾸로 snake(`parent_id` → `ParentID`), `count(*)`(식별자 아님), `a.id, b.id`(이름 겹침), 선언 필요한데 없음(식·NULL 쪽·모델 아닌 표·`UNION`), 필요 없는데 있음, 쓰이지 않은 선언, PG에서 선언과 OID가 다름(표 안·표 밖의 fix, SQLite는 통과), 같은 새 이름 두 쿼리가 같은 모양(한 `*Result`)·다른 모양(오류), 4번이 진단을 낸 쿼리(`cols` nil)는 건너뜀
4. 두 모듈 `go vet`·`go test`, `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: 스코프를 잃는 한계를 적음(codex 1, fable 11), `ONLY`·따옴표·`LATERAL`과 그 밖은 오류(codex 3), 거꾸로 snake의 규칙을 고정(codex 4), 이름 겹침을 먼저·선언은 이름으로(codex 5), 필드 대응은 결과 컬럼 순(codex 6), `LEFT JOIN <CTE>`는 오류(fable 1), 별칭이면 별칭의 이름(fable 2), `UNION`은 출처를 버림(fable 3), `FROM` 서브쿼리를 받는 것을 ★로(fable 4), `cols` nil 건너뜀(fable 5), 표 밖 OID의 캐스트 fix(fable 6), 선언이 필요한 오류의 위치·문구(fable 7), 약어는 `id`만(fable 8), 모델 불일치 문구 둘(fable 9), `Result.Pos`(fable 10), 확인 보강(fable 12)
- 받지 않음: codex 2(`RIGHT`의 보존 쪽을 가려 내자) — D-104가 `RIGHT`·`FULL`을 모든 테이블로 정했고 안전한 쪽이다. codex 7(바깥 조인이 있으면 모든 출처를 선언) — D-104가 테이블 단위를 정했다

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `fieldString`이 `fieldType`을 씀, 손으로 든 OID 이름 표 대신 `typeName` 인자(`format_type`), 테스트의 선언 줄을 사례에 바로. 받지 않음 — OID→타입 되찾기 루프 둘(두 곳뿐)
- codex(샌드박스 때문에 테스트는 못 돌림): 발견 없음
- fable(돌연변이 36개 중 31개를 잡음): 받음 — 두 컬럼이 같은 필드 이름(`a_1_b`·`a1_b` → `A1B`, 생성 코드가 컴파일되지 않는다), 모델 필드 이름 규칙의 시험(`URL`), 출처 타입을 아는 선언은 방언과 상관없이 맞춰 봄, `word(-1)`, 모델 결과 + `UNION`의 문구, 선언 타입에 맞춘 캐스트 fix, `WITH RECURSIVE`·CTE 컬럼 목록·모델 없는 앱의 시험, 계획 문구 셋. 기록만: PG는 `count(*)`를 `count`로·미인용 대문자를 소문자로 주어 SQLite에서만 "snake_case 아님"이 나는 파일이 있다(안전한 쪽, D-31)

## 6. 런타임 (`maago`, 루트 패키지)

- `maago.DB` 인터페이스, 슬라이스 펼치기, `:one`의 두 행 검사, D-105의 시각 변환, D-101의 여는 함수와 풀 기본값, `.env` 읽기
- stdlib만, reflect 없음. 드라이버를 쓰는 시험은 `cmd/maago` 모듈에

→ 확인: 루트 `go.mod`에 `require`가 없다. `cmd/maago`에서 두 방언: 환경 변수가 `.env`를 이기고, SQLite FK가 풀의 모든 연결에서 켜져 있다(연결 둘을 잡아 확인, TODO 닫힘). 시각: 소수 자릿수가 다른 두 시각의 SQLite `ORDER BY`가 시간 순서, 다른 오프셋의 입력이 UTC `…ffffff` 글로 저장되고 `datetime('now')`와의 비교가 맞다, NULL(`*time.Time`), 손으로 넣은 RFC 3339 일반형 읽기, 잘못된 글은 Scan 오류. PG `TIMESTAMPTZ` 왕복은 `==`가 아니라 `Equal`로 비교한다(위치·모노토닉이 사라진다). 빈 슬라이스는 오류. `:one`의 두 행은 오류

### 계획

파일 (루트 패키지 `maago`, stdlib와 이 모듈의 `internal`만)
- 새 `db.go`:
  ```go
  type DB interface { // *sql.DB·*sql.Tx·*sql.Conn이 만족
  	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
  	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
  	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
  }
  type DBConfig struct {
  	URL                              string        // ""면 MAAGO_DATABASE_URL, 없으면 작업 디렉터리의 .env
  	MaxOpenConns, MaxIdleConns       int           // 0이면 기본값, 음수면 제한 없음(database/sql과 같은 뜻)
  	ConnMaxLifetime, ConnMaxIdleTime time.Duration // 같다
  }
  func OpenDB(ctx context.Context, cfg DBConfig) (*sql.DB, error)
  ```
  주소는 `internal/project.Database`로 읽는다(★ D-101이 남긴 선택: 다시 쓰지 않는다. CLI와 같은 규칙·같은 오류 문구이고 D-97의 닫힌 형식이 한 곳에 있다). `cfg.URL`은 환경 변수처럼 이긴다. 진단은 `diag.Write`의 글 그대로 오류로(주소를 싣지 않는 규칙도 그대로). `.env`는 작업 디렉터리의 것(D-101). 드라이버 이름 `sqlite`·`pgx`가 등록되지 않았으면 오류(fix: `main`에 `_ "modernc.org/sqlite"`·`_ "github.com/jackc/pgx/v5/stdlib"`). SQLite 주소: `sqlite:` 뒤의 파일(작업 디렉터리 기준) + `?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`(modernc가 연결마다 실행한다). 풀 기본값은 D-101. 연 뒤 `PingContext`로 한 번 닿아 본다(주소가 틀리면 첫 쿼리가 아니라 여기서 실패)
- 새 `query.go`: 생성 코드가 쓰는 작은 것들
  ```go
  func One[T any](ctx context.Context, db DB, scan func(*sql.Rows) (T, error), query string, args ...any) (T, error) // 없으면 sql.ErrNoRows, 둘 이상이면 ErrManyRows
  func Many[T any](ctx context.Context, db DB, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error)
  func Placeholders(mark string, first, n int) string // "$3, $4". n<1은 부른 쪽의 버그(panic): 빈 슬라이스는 생성 코드가 먼저 ErrEmptySlice로
  var ErrManyRows = errors.New("maago: query returned more than one row")
  var ErrEmptySlice = errors.New("maago: empty slice") // 생성 코드가 쿼리·파라미터 이름을 붙여 감싼다
  ```
  쿼리를 스스로 돌려 생성 코드가 한 줄이다(`db.QueryContext(…)`의 두 값을 다른 인자와 함께 넘길 수는 없다, codex 1). 성공 전에 `rows.Err()`와 `rows.Close()`의 오류를 돌려준다. 제네릭이라 reflect가 없다
- 새 `sqlitetime.go` (D-105):
  ```go
  func SQLiteTime(t time.Time) string            // t.UTC()의 "2006-01-02 15:04:05.000000"
  func ScanSQLiteTime(dst *time.Time) sql.Scanner // NULL은 오류
  func ScanNullSQLiteTime(dst **time.Time) sql.Scanner // NULL은 nil
  ```
  읽기는 `YYYY-MM-DD HH:MM:SS[.소수]`(UTC로 읽는다)와 RFC 3339(오프셋 그대로 읽고 UTC로), 글은 `string`·`[]byte`. 그 밖의 글은 Scan 오류(글을 싣는다). 돌려주는 시각은 UTC
- 새 `query_test.go`·`sqlitetime_test.go`(루트, 드라이버 없이): `Placeholders`, 시각의 쓰기·읽기 표(소수 자릿수, 오프셋, RFC 3339 일반형, 잘못된 글, NULL)
- 새 `cmd/maago/runtime_test.go`(두 방언): 아래 확인의 DB 쪽

하지 않기로 한 것
- 선택지의 모르는 값, 파라미터 struct, 조합 고르기는 생성 코드(8번). 런타임은 그것이 부를 것만
- PG의 시각은 pgx가 `time.Time`을 그대로 다룬다: 도우미가 필요 없다(확인만)

단계
1. `DB`·`OpenDB` → 확인: 루트 `go.mod`에 `require`가 없다(`go mod tidy -diff`). `cmd/maago`에서 두 방언: 빈 `URL`에 환경 변수가 `.env`를 이긴다(`t.Setenv`, `t.Chdir`), `.env`만 있어도 열린다, `cfg.URL`이 둘 다를 이긴다, 드라이버 이름이 없는 빌드의 오류는 루트 테스트(드라이버가 없다)에서, 잘못된 주소는 `.env:줄`의 오류. SQLite FK가 풀의 연결 둘(`db.Conn` 둘을 잡아)에서 모두 켜져 있다(TODO 닫음). 풀 기본값(`db.Stats().MaxOpenConnections`)과 덮기
2. `One`·`Many`·`Placeholders` → 확인: `:one`의 0행은 `sql.ErrNoRows`, 2행은 `ErrManyRows`, 쿼리 오류는 그대로, `Placeholders("$",3,2)`·`("?",1,1)`, `n=0`은 panic
3. SQLite 시각 → 확인(`cmd/maago`, STRICT `TEXT` 컬럼): 소수 자릿수가 다른 두 시각(`…:05.5`와 `…:05.25`를 넣고)의 `ORDER BY`가 시간 순서, `+09:00` 입력이 UTC `…ffffff` 글로 저장되고 `datetime('now')`와의 `<`가 맞다, NULL은 `*time.Time` nil·`time.Time`에는 오류, 손으로 넣은 `2026-09-27T12:00:00+09:00` 읽기, 잘못된 글은 Scan 오류. PG `TIMESTAMPTZ` 왕복은 `Equal`
4. 두 모듈 `go vet`·`go test`(PG 포함), `gofmt -l .`, `go mod tidy -diff`

계획 리뷰 (codex·fable) — 구현이 계획과 달라진 곳은 여기가 정본이다
- 받음: `One`·`Many`가 쿼리를 스스로 돌린다(`One(ctx, db, scan, query, args...)`: `db.QueryContext(…)`의 두 값은 다른 인자와 함께 넘길 수 없다, codex 1). 성공 전에 `rows.Err()`·`Close` 오류(codex 2). PG 왕복은 마이크로초로 자른 뒤 `Equal`(codex 4). 음수 설정의 뜻을 바로(유휴는 "남기지 않음"), 유휴 기본값의 확인(fable 1). ★ SQLite 파일은 `file:…?mode=rw`로 열어 없는 파일은 새 빈 DB가 아니라 파일 이름이 든 오류(fix: `maago migrate` 또는 프로젝트 루트에서 실행). 파일은 CLI가 만든다(fable 2, PHILOSOPHY 7). `DBConfig.URL`이 있으면 `.env`를 읽지 않고 오류의 출처도 `DBConfig.URL`(`project.URLDialect`를 뽑아 냄, fable 3a). 런타임이 기대는 이 모듈의 패키지가 `internal/diag`·`internal/project`뿐임을 시험으로(fable 3b, 11번에서 ARCHITECTURE §2에 적는다). PRAGMA 셋을 연결 둘에서(fable 4). 그대로 넘긴 `time.Time`은 읽히지 않음·소수부 0도 여섯 자리(fable 5). `ErrManyRows`의 고치는 법, `Ping` 실패의 한 줄 문구, "not set"의 확인(fable 6). `Many`의 0행은 빈 슬라이스(JSON `[]`, fable 7)
- 8번에 넘길 것: SQLite에서 `time.Time` 파라미터·CRUD 값은 `maago.SQLiteTime`을, `*time.Time`은 nil이면 nil을, `[]time.Time`은 원소마다(codex 3, fable 8). 결과의 NULL(값 타입·포인터, `:one`·`:many`)을 실제 DB로 시험(codex 7). PG는 `time.Time`을 로컬 위치로 돌려주고 SQLite 도우미는 UTC다(D-31이라 섞이지 않는다, 11번에 한 줄)
- 받지 않음: codex 5의 "연결을 놓고 새로 만든 뒤" 시험(연결 둘을 동시에 잡는 것이 이미 연결마다를 증명한다), codex 6의 취소 시험(`database/sql`을 그대로 통과한다), fable 4의 검증되는 단축 키(`_foreign_keys` 등. `_pragma`는 연결마다 읽어서 확인했다)

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — `pick` 대신 `cmp.Or`, `URLDialect`의 끝을 `return d, ds`로. 받지 않음 — `DB`의 `QueryRowContext`(D-101이 세 메서드를 정했다), `Placeholders`의 panic(생성기 버그를 크게 알린다, 매퍼와 같다)
- codex(샌드박스 때문에 테스트는 못 돌림): 받음 — `DBConfig.URL`이 있으면 `.env`를 아예 읽지 않게(먼저 갈라서), PG 오류의 비밀번호 가리기(`practicedb`의 것을 `project.HidePassword`로 옮겨 함께 쓴다). 받지 않음 — `One`의 이른 반환에서 `Close` 오류(이미 오류를 돌려주는 길이고 `rows.Err()`를 먼저 본다), `ErrManyRows` 문구를 처음 계획대로(계획 리뷰에서 고치는 법을 넣기로 했다)
- fable(돌연변이 26개 중 21개를 잡음): 받음 — `sqlite::memory:`는 파일이 아니라는 오류(풀의 연결마다 빈 DB가 되고, CLI는 `:memory:`라는 파일을 만든다), SQLite 열기 실패의 문구를 없는 파일(작업 디렉터리를 싣는다)과 쓸 수 없는 파일(WAL이 쓴다)로 나눔, `ConnMaxLifetime` 덮기와 `MaxOpenConns: -1`의 시험, `#`·`%`가 든 파일 이름, `HidePassword`가 짧은 비밀번호에서 글을 망가뜨리던 것(주소가 담는 자리만 바꾼다), `DB`를 만족하는 세 타입의 컴파일 단언, 문구와 시험 위생(`pgURL`은 PG 사례에서만, `TestSQLiteTimes`의 오류). 시험하지 않음: `ConnMaxIdleTime`(`database/sql`의 청소가 1초 이상 간격으로 한다)과 수명·유휴 시간의 기본값 자체(`Stats`로 보이지 않는다). 11번으로: CI의 reflect import 검사(D-66, 지금은 `noreflect_test.go`가 한다)

## 7. PK CRUD의 SQL (D-21)

DB 검사가 CRUD SQL도 prepare하려면 생성 전에 SQL이 있어야 한다. 그래서 SQL을 만드는 일과 Go 코드를 쓰는 일(8번)을 나눈다.

- 모델마다 `Get`·`Insert`·`Update`·`Delete`의 방언별 SQL. `Insert`는 `m.ID`를 무시하고 `RETURNING id`로 받아 채운다(SQLite 3.37+(D-87)·PG 15+(D-98)). `Update`·`Delete`가 0행이면 `sql.ErrNoRows`(M4가 404로, 조용히 성공하지 않는다). D-102의 자동 시각(M1 태그 검사에 `auto`를 더한다: `time.Time`에만, 값은 `create`·`update`)
- DB 검사 단계에서 4번과 같은 자리로 prepare한다. 실패는 maago의 버그(`dialect.Bug`)
- 함수 이름(`GetPost`·`InsertPost` …)과 파일(`models_gen.go` 또는 `crud_gen.go`)은 계획에서. 이름 충돌은 모양 절

→ 확인: 공용 예제 모델(FK, NULL, 기본값, 예약어 컬럼)의 CRUD SQL 골든(두 방언)이 연습용 DB에서 prepare된다. `RETURNING`이 modernc v1.59.0과 PG 15에서 실제로 id를 돌려준다

### 계획

파일
- `internal/model/check.go`: 필드 태그 `auto`(D-102). `Field.Auto`(`""`·`"create"`·`"update"`). 규칙: 값은 `create`·`update` 하나(모르면 did you mean), 필드는 포인터 아닌 `time.Time`(늘 채우므로 NULL일 일이 없다. `*time.Time`이면 오류, fix `declare it as time.Time`), `ID`에는 이미 막힌다(ID는 `label`·`help`만). `time.Time`의 `default`는 원래 오류라 `auto`와 부딪히지 않는다. 스키마(DDL)는 바뀌지 않는다
- `internal/testdata/example/blog/models.go`(공용 예제): `Author`에 `CreatedAt t.Time maago:"auto=create"`, `UpdatedAt t.Time maago:"auto=update"`. `Author`는 FK·NULL·예약어 컬럼(`order`)을 이미 가졌다. 공용 예제를 쓰는 골든(M1 DDL·dump, M2 읽기·빌드)은 컬럼 둘이 늘어난 것으로 다시 뽑는다(그 밖의 차이가 없는지 diff로 본다)
- 새 `internal/mapper/crud.go`:
  ```go
  type CRUD struct {
  	Model               *model.Model
  	Get, Insert, Delete Statement
  	Update              *Statement // ID 말고 바꿀 필드가 없으면 nil: 그 모델에는 UpdateX를 만들지 않는다
  }
  type Statement struct {
  	SQL  string
  	Args []string // 자리표시 번호 순의 필드 이름(Args[i]가 $i+1·?i+1)
  }
  func CRUDs(app *model.App, d project.Dialect) []CRUD // 앱 하나, 모델 선언 순(8번이 앱마다 파일을 쓴다)
  ```
  실행 계약(8번이 따른다, 필드마다 고정): Get은 행 하나를 필드 순으로 Scan(없으면 `sql.ErrNoRows`), Insert는 `QueryRow`로 id 하나를 받아 `m.ID`에(`Exec`가 아니다), Update·Delete는 `Exec`하고 영향 행이 0이면 `sql.ErrNoRows
  ```
  SQL(식별자는 모두 `dialect.Quote`, 자리표시는 3번과 같게 PG `$n`·SQLite `?n`):
  - Get: `SELECT 모든 컬럼(필드 순) FROM t WHERE "id" = $1`, Args `[ID]`
  - Insert: `INSERT INTO t (ID 밖의 컬럼) VALUES ($1, …) RETURNING "id"`. ID 밖의 필드가 없으면 `INSERT INTO t DEFAULT VALUES RETURNING "id"`. `m.ID`는 쓰지 않는다. auto 필드도 Args에 있다(8번이 `time.Now()`로 채운다)
  - Update: `UPDATE t SET "c" = $1, … WHERE "id" = $n`. SET은 ID와 `auto=create` 밖의 필드(`auto=update`는 있다, 8번이 채운다)
  - Delete: `DELETE FROM t WHERE "id" = $1`
  - 0행이면 `sql.ErrNoRows`는 8번의 Go 코드(`RowsAffected`)
- 새 `internal/mapper/crud_test.go`: 공용 예제의 두 방언 골든(`testdata/crud.sqlite.golden`·`crud.postgres.golden`, 모델마다 넷, Args 포함). 필드가 ID뿐인 모델(Insert의 `DEFAULT VALUES`, Update 없음)은 작은 인라인 모델
- 새 `cmd/maago/crud_test.go`(두 방언): 공용 예제의 DDL을 연습용 DB에 적용하고 CRUD의 SQL을 모두 `practicedb.Describe`로 prepare(실패 없음). PG는 `Params[i]`가 `Args[i]` 필드의 타입과 맞는지(4번의 `pgTypes`), Get의 결과 컬럼이 필드 순인지 본다(자리표시와 `Args`가 어긋나면 잡힌다). 그리고 실제로 Insert를 실행해(인자는 필드 이름 → 값 표에서 `Args` 순으로. SQLite 시각은 `maago.SQLiteTime`, D-105) `RETURNING`의 id를 받고(두 번 넣어 1, 2), 그 id로 Get·Update·Delete가 도는 것까지. nullable FK(`ParentID`)는 nil과 값 둘 다 넣고 읽는다. 영값 `Post`의 Insert는 choices CHECK로 실패한다(위 ★의 대가를 못 박는다)

결정 (계획에서 정한 것, 11번에서 DECISIONS로)
- ★ 함수 이름 `GetPost`·`InsertPost`·`UpdatePost`·`DeletePost`(동사 + 모델), 파일은 `models_gen.go`(모델의 생성 코드가 한 파일. ARCHITECTURE §5의 "관심사마다 하나"에서 CRUD는 모델의 관심사다). 코드는 8번이 쓴다. 이름 충돌 검사도 8번(모양 절)
- ★ `auto`는 `time.Time`(포인터 아님)에만
- CRUD는 모델의 값을 모두 쓴다: Insert에서 DB 기본값(`default`, D-83)은 쓰이지 않는다(Go 영값은 "주지 않음"과 가를 수 없다). 기본값은 **D-106**: Insert는 입력 struct(`NewPostInput`)를 받고, 기본값이 있는 필드는 `maago.Opt[T]`라 "주지 않음"이면 생성 코드가 기본값을 넣는다(사람의 결정: 영값이 조용히 들어가면 안 된다. 생성자 안은 구조체 리터럴이 건너뛰어 버렸다). 코드는 8번(codex 1, fable 2)
- ★ `models_gen.go`는 8번이 통째로 쓴다(D-18 주석과 CRUD). 지금 `model.Files`가 `fmt`로 쓰는 것은 8번으로 접히고, 모델이 있는 앱은 늘 이 파일을 갖는다(fable 1. ARCHITECTURE §5의 "M1의 `models_gen.go`는 `fmt`로"를 11번에서 고친다)
- CRUD SQL의 prepare 실패는 maago의 버그(`dialect.Bug`)이고 파이프라인 연결은 9번. 이 작업의 확인은 테스트가 한다

하지 않기로 한 것
- Go 코드 생성(Scan, 0행 오류, auto 채우기, SQLite 시각 변환)은 8번
- 부분 갱신(바뀐 필드만)은 하지 않는다: Update는 모든 필드를 쓴다(D-21 "PK 기준 기본 CRUD", 나머지는 손으로)

단계
1. `auto` 태그 → 확인: 태그 표에 `auto=create`·`auto=update` 통과, `auto=crate`(did you mean), `auto` 값 없음, `*time.Time`·`string`에 `auto`는 각각 위치 있는 오류. 공용 예제 골든(dump·DDL·읽기·빌드)을 다시 뽑고 diff가 두 컬럼뿐인지 본다
2. `CRUDs` → 확인: 두 방언 골든(손으로 읽어 따옴표·자리표시·`RETURNING`·auto의 자리를 확인), ID뿐인 모델
3. `cmd/maago` → 확인: 모든 CRUD SQL이 두 방언의 연습용 DB에서 prepare되고, Insert가 1, 2를 돌려주고, Get·Update·Delete가 돈다(Update의 영향 행 1, 없는 id의 Delete 영향 행 0)
4. 두 모듈 `go vet`·`go test`(PG 포함), `gofmt -l .`

계획 리뷰 (codex·fable)
- 받음: 기본값의 뜻(둘 다, 위 ★), nullable FK 왕복(codex 2), ID뿐인 모델은 `UpdateX` 없음과 `*Statement`(codex 3, fable 8), 필드마다의 실행 계약(codex 4, fable 6), 1단계는 이미 구현(codex 5), SQLite 3.37(codex 6), `models_gen.go`의 주인(fable 1), `Args` 순서를 시험(fable 4), SQLite 시각은 `maago.SQLiteTime`(fable 5), 앱 단위 `CRUDs`(fable 7). M6에 D-21이 주는 것은 Get·Insert·Update·Delete와 `Field.Auto`(읽기 전용)까지: 목록·필터·개수는 D-48의 생성 코드 몫이다(fable 9). 모양 절의 "SQLite는 `?`"는 11번에서 `?n`으로(fable 3)

구현 리뷰 (ponytail·codex·fable)
- ponytail: 받음 — Get의 묘사를 다시 하지 않음, 한 번 쓰는 타입 이름 도우미를 풀어 씀. 받지 않음 — `TestCRUDPrepares`가 `openPractice`를 쓰게(실행 시험에 날 연결이 필요해 `exampleConn`이 남는다), `WHERE "id"` 조각 나눔(줄지 않는다)
- codex(샌드박스 때문에 테스트는 못 돌림): 받음 — 실행 시험이 `Args`에 기대지 않게 넣은 값을 되읽어 비교(같은 타입 필드가 바뀌어도 잡힌다), ID뿐인 모델의 Insert를 두 방언에서 실행
- fable(돌연변이 15종, 같은 타입 필드 맞바꿈까지 잡힘): 받음 — 시험의 PG 타입 표가 `mapper`의 것을 옮겨 적은 이유를 주석으로, 자리표시 수와 `Args` 수가 다르면 panic 대신 오류. 8번에 넘길 것: `Statement.Args`가 이름이라 템플릿이 필드를 찾아야 한다(시끄러우면 `[]*model.Field`로). 11번에 넘길 것: ARCHITECTURE·TUTORIAL-FUTURE의 "allowed:" 예시에 `auto`, D-102의 대가에 "기존 테이블에 `auto` 필드를 더하면 초안이 `NoDefault` 경고를 내고 사람이 마이그레이션을 손본다"

## 8. 코드 생성 (`queries_gen.go`, CRUD)

- `text/template`(`embed.FS`) + `go/format`(ARCHITECTURE §5). 파라미터 struct, 선택지 타입, 새 결과 타입, 쿼리 함수(조합마다 고정 조각을 잇는 코드), CRUD 함수
- 생성 코드는 방언별이다(자리표시, SQLite 시각 변환). D-31: 프로젝트는 방언 하나
- `SearchOrders` 픽스처: `Order` 모델(`table=orders`, `status`·`amount`·`created_at`)을 둔 예제 앱을 `internal/testdata/`에 둔다. 3번(글 골든)과 이 작업(생성·실행)이 같은 픽스처를 쓴다
- D-106: 모델마다 입력 struct `NewPostInput`(ID와 `auto` 필드는 없다)과 `InsertPost(ctx, db, NewPostInput) (Post, error)`. 기본값이 있는 필드는 `maago.Opt[T]`(런타임에 `Opt`·`Set`을 더한다), 설정하지 않았으면 생성 코드가 D-83의 기본값을 넣는다(포인터 필드는 그 값을 가리키는 포인터). 이름과 돌려주는 모양은 이 작업의 계획에서

→ 확인: 예제 앱의 생성 골든(두 방언). 생성 코드가 컴파일되고 두 방언의 실제 DB에서 돈다(`cmd/maago` 모듈): CRUD 왕복(nullable FK, 시각, 0행 `Update`·`Delete`의 `sql.ErrNoRows`), `SearchOrders`의 조합(두 정렬의 첫 행이 다름을 단언해 `sort`를 무시하는 돌연변이를 잡는다, 선택 파라미터 있음·없음의 행 수), `:one`의 `sql.ErrNoRows`, 선택지 밖의 값, 새 결과 타입의 NULL(`*T`는 `nil`, 선언되지 않은 NULL은 Scan 오류), `*sql.Tx` 안에서 부르기

### 계획

생성되는 Go의 모양 (예: 앱 `orders`, SQLite. PG는 자리표시와 시각 변환만 다르다)

`models_gen.go` — 모델이 있는 앱마다 늘(D-18 주석 + CRUD. 지금 `model.Files`가 쓰는 주석은 이 파일로 접힌다)
```go
// NewOrderInput is what InsertOrder takes (D-106): no ID, no auto field.
// A field with a default is a maago.Opt: left unset, it takes the default.
type NewOrderInput struct {
	CustomerID *int64
	Status     maago.Opt[string] // default 'new'
	Amount     int64
	Priority   maago.Opt[*int32] // default 1
	CreatedAt  time.Time
}

func GetOrder(ctx context.Context, db maago.DB, id int64) (Order, error)       // 없으면 sql.ErrNoRows
func InsertOrder(ctx context.Context, db maago.DB, in NewOrderInput) (Order, error) // 기본값·auto를 채우고 RETURNING id까지 채운 모델
func UpdateOrder(ctx context.Context, db maago.DB, o *Order) error            // auto=update를 o에 채운다. 0행이면 sql.ErrNoRows. ID뿐인 모델은 없다
func DeleteOrder(ctx context.Context, db maago.DB, id int64) error            // 0행이면 sql.ErrNoRows
```
`queries_gen.go` — `queries/`가 있는 앱마다
```go
type SearchOrdersSort string // 선택지 타입과 상수
const (
	SearchOrdersSortLatest SearchOrdersSort = "latest"
	SearchOrdersSortAmount SearchOrdersSort = "amount"
)
type SearchOrdersParams struct { // 파라미터 선언 순, 필드 이름은 D-76의 거꾸로(5번의 fieldName)
	Status *string
	From   *time.Time
	To     *time.Time
	Sort   SearchOrdersSort
	Limit  int64
}
type OrderSummary struct{ … } // 새 결과 타입, 앱에 한 번(5번의 공유 *Result)

func SearchOrders(ctx context.Context, db maago.DB, p SearchOrdersParams) ([]OrderSummary, error)
```
- 종류: `:one` → `(T, error)`(`maago.One`), `:many` → `([]T, error)`(`maago.Many`), `:exec` → `(int64, error)`(영향 행). 파라미터가 없으면 `p` 인자가 없다
- 몸통: 축마다 값의 번호를 구해 혼합 진법 열쇠 하나로 모으고(3번의 `Combos`와 같은 순), `switch key { case 0, 2, 4: … }`로 변형의 고정 SQL과 인자를 고른다(D-22: 런타임은 SQL을 해석하지 않는다). 선택지의 모르는 값은 `fmt.Errorf("SearchOrders: sort %q is not latest or amount")`. 슬라이스가 있는 변형은 먼저 빈 슬라이스를 `fmt.Errorf("DeleteOrders: ids: %w", maago.ErrEmptySlice)`로 막고, 고정 조각 사이에 `maago.Placeholders(mark, first, len)`을 잇는다(`first`는 스칼라 수 + 앞 슬라이스들의 길이 + 1, 3번의 번호 규칙). 이웃한 SQL 조각은 하나의 문자열 상수로 잇는다(4번에서 넘긴 것)
- 인자: 스칼라는 `Args` 순, 그 뒤 `Slices` 순의 원소들. 선택 파라미터는 그 줄이 있음을 보장하므로 `*p.Status`, 선택지는 `string(p.Sort)`
- Scan: 쿼리마다 비공개 `scanSearchOrders(*sql.Rows) (T, error)`(결과 컬럼 순, 모델 결과도 5번의 필드 대응대로). CRUD의 Get은 `scanOrder`(필드 순)
- SQLite 시각(D-105, 6번에서 넘긴 것): `time.Time` 인자는 `maago.SQLiteTime(v)`, `*time.Time`은 `maago.SQLiteNullTime(v)`(nil이면 nil, 런타임에 더한다), `[]time.Time`은 원소마다. Scan은 `maago.ScanSQLiteTime(&v.X)`·`maago.ScanNullSQLiteTime(&v.X)`. PG는 그대로
- D-106: `maago.Opt[T]`(영값이 "주지 않음"), `maago.Set(v) Opt[T]`, `(Opt[T]) Or(def T) T`, 포인터 기본값을 위한 `maago.Ptr(v) *T`를 런타임에 더한다. 기본값의 Go 글자: 문자열은 `strconv.Quote`, 수는 `dialect.Number`(DDL과 같은 정규형: `010`을 그대로 쓰면 Go는 8로 읽는다, fable 1), bool은 그대로. `InsertX`는 `time.Now().Truncate(time.Microsecond)` 하나로 `auto` 필드를 모두 채운다(D-102. DB의 정밀도와 같게 잘라, Insert가 돌려준 값과 Get이 읽은 값이 같다, fable 8)
- `InsertX`는 입력·기본값·auto·`RETURNING`의 id로 모델을 모두 채워 돌려준다(다시 읽지 않는다, codex 1). `UpdateX`는 영향 행이 1인 것을 확인한 뒤에야 `o`의 `auto=update` 필드를 바꾼다(실패하면 호출자의 모델은 그대로, fable 8). 주석: "pass what GetX returned: every column is written"(fable 9). ★ `InsertX`가 모델을 돌려주는 것과 `UpdateX`가 포인터를 받는 것은 11번에서 DECISIONS로
- import는 쓰는 것만(`go/format`은 쓰지 않는 import를 지우지 않는다, fable 2)
- 실행 오류: Scan 오류는 `fmt.Errorf("CustomerOrdersStrict: %w", err)`로 쿼리 이름을 붙인다(D-104의 "판정이 틀리면 실행 때 Scan 오류"가 어느 `.sql`인지 보이게, fable 11). 선택지는 `sort %q is not one of latest, amount`, `ErrEmptySlice`의 글에 고치는 법(fable 12)

파일
- 새 `internal/mapper/gen.go`와 `internal/mapper/templates/{models,queries}.go.tmpl`(`embed.FS`, `text/template` + `go/format`, ARCHITECTURE §5):
  ```go
  type Checked struct { // 한 쿼리의 검사 결과: 3·4·5번이 만든 것
  	Query    *Query
  	Axes     []Axis
  	Variants []Variant
  	Result   *Result // :exec면 nil
  }
  func ModelsFile(d project.Dialect, app *model.App) []byte                 // 모델이 있는 앱
  func QueriesFile(d project.Dialect, pkg string, qs []Checked) []byte      // queries/가 있는 앱. pkg는 9번이 정한다(열린 질문)
  func Names(root, dir string, app *model.App, qs []Checked) []diag.Diagnostic // 이름 충돌(아래)
  ```
  `go/format`이 실패하면 maago의 버그(panic): 생성물이 Go가 아니면 조용히 쓰지 않는다
- `internal/build/build.go`: 생성 단계가 `model.Files` 대신 `mapper.ModelsFile`을 쓴다(한 파일의 주인이 둘이 되지 않게, 7번의 결정). `model.Files`와 그 골든은 지운다. 쿼리 쪽 연결은 9번
- 루트 `maago`: `Opt`·`Set`·`Or`·`Ptr`(`opt.go`), `SQLiteNullTime`(`sqlitetime.go`)
- 새 픽스처 앱 `internal/testdata/orders/`(이미 `queries/search_orders.sql`이 있다): `models.go`(`Customer`, `Order`: `CustomerID *int64 fk=Customer`, `Status string default='new'`, `Amount int64`, `Priority *int32 default=010`(정규형 10), `CreatedAt time.Time`, `UpdatedAt time.Time auto=update`)와 쿼리 몇 개 더: `customer_orders.sql`(`LEFT JOIN`의 NULL 쪽을 `-- column: amount? int64`로), `customer_orders_strict.sql`(같은 것을 `?` 없이: NULL이면 Scan 오류), `order_by_amount.sql`(`:one Order`, SELECT 순서를 필드 순과 다르게: 결과 컬럼 순 대응을 실행으로 본다, fable 5), `delete_orders.sql`(`:exec`, 슬라이스 `ids`), `orders_matching.sql`(슬라이스가 스칼라보다 먼저, 슬라이스 둘, `[]time.Time`, 슬라이스 뒤의 스칼라: 인자 조립이 어긋나면 행이 틀린다, codex 3·fable 4)
- 새 `cmd/maago/internal/gentest/{sqlite,postgres}/{orders,blog,plain}/`: 생성물이 실제로 컴파일되고 도는 자리. 이 패키지들의 `models_gen.go`·`queries_gen.go`가 곧 생성 골든이다(`TestGenerated`가 픽스처에서 다시 만들어 `golden.Check`로 비교, `-update`로 쓴다). `models.go`는 픽스처의 복사본(같은지 시험이 본다). 손으로 쓴 `*_test.go`가 두 방언의 실제 DB에서 부른다. cmd/maago 모듈 안이라 드라이버가 이미 있고 따로 모듈을 만들지 않는다(사용자 모듈 모양으로 걷는 시험은 10번의 튜토리얼 스크립트, codex 5)
  - `orders`: 위 픽스처
  - `blog`: 공용 예제(`internal/testdata/example/blog`). 예약어 `order`, `[]byte`, bool 기본값, `auto=create`, D-18 주석이 컴파일되고 CRUD가 한 번 왕복한다(fable 3). `cmd/maago`의 `models_gen.golden`을 보던 빌드 시험은 이 파일로
  - `plain`: 시각·선택지·슬라이스가 없는 앱(모델 하나, 쿼리 하나). 쓰지 않는 import가 없음을 컴파일로 본다(fable 2)
  - PG 골든은 결과 타입이 실제 PG의 묘사에서 오므로 `MAAGO_TEST_PG_URL`이 있어야 비교·갱신된다. 정본은 CI(D-91, fable 6)

이름 충돌 (모양 절, 생성 오류)
- 앱의 생성 이름: CRUD 넷과 `NewXInput`·`scanX`(모델마다), 쿼리 함수·`QParams`·선택지 타입과 상수·새 결과 타입·`scanQ`(쿼리마다). 이것끼리 겹치거나, 그 패키지의 생성되지 않은 `.go` 파일(표시 없는 파일. 같은 패키지 이름의 `_test.go`도: 테스트 바이너리에서 함께 컴파일된다, codex 2·fable 13)의 최상위 이름과 겹치면 오류. 위치는 쿼리의 `-- name:`(모델 쪽 이름이면 모델), 메시지에 겹치는 상대의 위치. fix: 쿼리나 결과 타입의 이름을 바꾼다
- 사용자 이름은 `go/parser`로 그 디렉터리의 표시 없는 파일을 읽어 모은다(`model.Read`는 모델만 남긴다)

하지 않기로 한 것
- 파이프라인에 쿼리를 잇는 일(해석·검사·DB 검사·생성 단계), `queries/`만 있는 앱의 패키지 이름, 생성물의 `text/template`·`reflect` 게이트는 9번
- 선택지의 모르는 값·빈 슬라이스를 가를 오류 타입(M4가 400으로)은 필요해질 때. 지금은 `fmt.Errorf`와 `ErrEmptySlice`
- 생성물의 문서 주석은 짧게: 함수마다 한 줄(어느 파일에서 왔는지)

단계
1. 런타임 `Opt`·`Set`·`Or`·`Ptr`·`SQLiteNullTime` → 확인: 루트 표 시험
2. `ModelsFile` → 확인: 공용 예제의 골든(두 방언, gentest `blog`), `build`가 이것을 쓰고 빌드 시험이 새 골든으로 통과. `TestGenThenCheck`의 끝 단언("모든 필드가 컬럼이면 지워진다")은 "앱의 모델이 모두 사라지면 지워진다"로 고친다(fable 7)
3. `QueriesFile` → 확인: `orders` 픽스처의 골든(두 방언, 위 gentest 자리)
4. gentest 실행 → 확인(두 방언의 실제 DB): CRUD 왕복(nullable FK nil·값, 시각은 마이크로초로 자른 `Equal`, `Opt` 안 줌 → 기본값(`Priority` 10)·`Set` → 그 값, `auto=update`가 바뀜·실패한 `Update`는 모델을 바꾸지 않음, 없는 id의 `Update`·`Delete`는 `sql.ErrNoRows`), `SearchOrders`의 16조합을 모두 실제로 돌려 결과를 Go로 거른 기대값과 비교(열쇠 계산과 `Combos`의 대응을 실행으로 본다, codex 4: 두 정렬, 선택 파라미터 있음·없음, `from`만 있으면 기간 조건이 빠짐), `orders_matching`의 행(인자 조립), `:one`의 `sql.ErrNoRows`·`ErrManyRows`, 선택지 밖의 값, 빈 슬라이스의 `ErrEmptySlice`, 슬라이스 원소 2개의 삭제, 새 결과 타입의 NULL(`*int64`는 nil, `?` 없는 선언은 Scan 오류), `*sql.Tx` 안에서 부르기와 롤백
5. 이름 충돌 → 확인: 쿼리 `GetOrder`(CRUD와), 결과 타입이 쿼리 함수 이름과, 두 쿼리의 선택지 타입, 사용자 파일의 같은 이름, 같은 패키지 `_test.go`의 `scanOrder`·`NewOrderInput`이 각각 위치 있는 오류. 생성 파일과 `package x_test` 파일의 이름은 세지 않는다
6. 두 모듈 `go vet`·`go test`(PG 포함), `gofmt -l .`(생성물 포함), `go mod tidy -diff`

계획 리뷰 (codex·fable)
- 받음: `InsertX`는 다시 읽지 않고 채운다(codex 1), 같은 패키지 `_test.go`의 이름(codex 2, fable 13), 인자 조립을 실행으로 보는 쿼리(codex 3, fable 4), `SearchOrders` 16조합을 모두 실행해 기대값과 비교(codex 4: 열쇠를 단위로 시험하면 같은 계산을 다시 적는 것이라 실행으로), 기본값은 `dialect.Number`로(fable 1), 쓰는 import만과 `plain`(fable 2), gentest `blog`(fable 3), 모델 결과의 컬럼 순(fable 5), PG 골든의 정본은 CI(fable 6), `TestGenThenCheck`(fable 7), `UpdateX`의 쓰는 때와 마이크로초(fable 8), 주석(fable 9), Scan 오류에 쿼리 이름(fable 11), 문구(fable 12)
- 11번에 넘길 것: `:exec`는 0행이어도 오류가 아니고 CRUD의 `Update`·`Delete`는 `sql.ErrNoRows`(TUTORIAL-FUTURE §6에 한 줄, fable 10), 선택 파라미터는 `maago.Ptr("paid")`로 채운다(fable 14)
- 받지 않음: codex 5(따로 사용자 모듈을 만드는 스모크) — CI가 `cmd/maago` 모듈을 돌리고, 사용자 모듈 모양은 10번의 튜토리얼 스크립트가 걷는다

구현에서 달라진 것
- 런타임에 `maago.Exec(ctx, db, query, args...) (int64, error)`(영향 행)를 더했다: `:exec` 쿼리가 그대로 돌려주고, CRUD의 `Update`·`Delete`는 0이면 `sql.ErrNoRows`
- 이름 충돌은 생성 파일이 import하는 이름(`context`·`sql`·`time`·`fmt`·`maago`)을 사용자가 최상위에 선언한 것도 잡는다(Go는 파일 블록과 패키지 블록에 같은 이름을 금한다). 두 생성자가 여러 이름(선택지 상수들)에서 겹치면 한 번만 알린다. 생성 파일을 가리려고 `pipeline.IsGenerated`를 내보냈다
- `build`는 모델 앱마다 `Names`를 검사 단계에서 돌린다: 이제 `models_gen.go`에 CRUD가 있어 사용자 이름과 겹치면 컴파일이 깨진다. 모델 앱이면 늘 `models_gen.go`가 있으므로 `cmd/maago`의 `expect`는 check 앞에 gen을 돈다
- gentest의 손 시험은 두 방언에 같은 파일이다(`gentest.Open`이 디렉터리로 방언을 안다). `TestGenerated`가 PG 쪽이 SQLite 쪽의 복사본인지 본다. 표는 DDL 생성기(`sqlite.DDL`·`postgres.DDL`)로 만든다: `default=010`이 DDL과 Go에서 같은 10임을 실행으로 본다
- 11번에 넘길 것: 파라미터 `ids`의 필드는 `Ids`다(D-76이 되돌리는 약어는 `id`뿐)

구현 리뷰 (ponytail·codex·fable)
- 받음: 시험의 포인터 출력을 제네릭 하나로(ponytail 1), nullable 시각에 값이 있는 왕복(codex 2), `InsertX`·`UpdateX` 주석을 사실대로(fable 1·2: 입력 시각은 그대로 담고 DB는 마이크로초로 쓴다, `auto=create` 필드는 Update가 쓰지 않는다), 생성물끼리 겹칠 때 상대의 위치(fable 3), Go가 무시하는 `.`·`_`로 시작하는 파일은 세지 않는다(fable 4), `Names`를 `makemigrations`가 아니라 gen·check의 검사로(fable 6), `plain`에 bool 축 쿼리, 쿼리 없는 앱에 남은 `queries_gen.go`를 `TestGenerated`가 잡는다(fable 8)
- 받지 않음: `IsOwned`라는 이름(ponytail 2: 바뀌는 자리 수는 같고 `IsGenerated`가 뜻을 말한다), import 이름 충돌을 실제 import만으로(codex 1, fable 5): 쿼리를 더할 때 import가 늘어 그때 가서 오류가 나지 않게 고정 목록을 두고, 메시지만 "may import"로. 최상위 `time`·`fmt`를 둔 패키지는 스스로도 그 패키지를 import할 수 없다
- 11번에 넘길 것: `InsertX`가 돌려준 모델은 입력의 포인터를 그대로 담는다(fable 7)

## 9. 파이프라인 연결과 게이트 (`internal/build`)

- 해석 단계에 매퍼 파일 읽기, 검사 단계에 1~3번과 7번의 SQL, DB 검사 단계에서 M2 비교가 통과한 뒤 4·5번과 CRUD prepare, 생성 단계에 8번. `makemigrations`는 쿼리 파일을 읽지 않는다(모양 절)
- 게이트(완료 조건 4): 기존 `TestNoReflect`(저장소 전체, 생성기 포함)와 **범위가 다른 둘째 검사**. 생성기는 `text/template`을 정당하게 쓰므로, `text/template`·`reflect` 금지는 생성물(첫 줄에 표시가 있는 파일, 생성 골든, `.tmpl`)에만. 루트를 인자로 받는 함수로 두어 일부러 틀린 생성물은 임시 트리에서 시험한다(틀린 골든은 커밋할 수 없다)

→ 확인: `gen` → `check` 0, 쿼리 파일을 바꾸면 `check` 1(생성물이 낡음). 모델 필드를 지우고 `makemigrations`하면 그 필드를 쓰는 쿼리가 있어도 초안이 나오고, `check`는 그 쿼리의 오류. 잘못된 `.sql`이 있어도 `makemigrations`는 돈다. 게이트는 임시 트리에 `text/template`을 import한 표시 있는 파일을 넣어 실패하고, 표시 없는 파일(생성기 코드)은 통과(WORKFLOW 5)

### 계획

단계 (ARCHITECTURE §3의 자리. `stages`에는 해석 단계의 훅 `parse` 하나만 더하고, 나머지는 `checking`의 `early`·`final`·생성 단계가 는다. `makemigrations`는 `parse`에 nil을 넘기고 자기 `early`·`final`을 쓰므로 쿼리를 읽지 않는다)
- 1단계(읽기): 그대로. `project.QueryFiles`는 `parse` 안에서 부른다(목록 실패와 읽기 실패를 한 단계에)
- 3단계(해석, 모델 검사·`.env`·마이그레이션 파일 옆): `parse` = `QueryFiles` → `mapper.Read`. `.sql` 읽기는 모델이 필요 없으므로 태그 오류가 `.sql` 오류를 숨기지 않는다(fable 1)
- 4단계(DB 없는 검사, `early`): 읽힌 쿼리마다 `Variants()`, 앱 디렉터리마다(모델 앱 ∪ 쿼리 앱) `Names`, 쿼리 앱마다 패키지 이름. `Variants`·`Names`는 읽힌 쿼리만 본다(`Read`는 오류 난 파일의 `Query`를 주지 않는다)
- 6단계(DB 검사, `final`): 지금의 `checkDatabase` 안에서, A를 닫기 전에(`defer a.Close()`가 함수 끝에서 닫는다, codex 1의 (가)). M2 비교(`Diagnose`)에 진단이 있으면 쿼리로 가지 않는다(모델과 다른 DB에 prepare하면 연쇄 오류). 통과하면 `checkQueries(ctx, s, a)`:
  1. 쿼리 앱마다 쿼리마다 `mapper.Describe`. 쿼리 진단은 모아 두고 그 쿼리의 컬럼은 nil로 다음 쿼리로(쿼리끼리 독립, D-61). 연결·`SAVEPOINT`의 실패(치명)면 그 진단 하나로 곧바로 돌아간다(fable 2: 죽은 연결에 계속 보내지 않는다, 4번의 결정). 앱마다 `mapper.Results`(`typeName`은 `a.TypeName`)의 진단도 모은다. 앱들의 진단을 모두 모아 돌려준다
  2. 모델 앱마다 `mapper.CRUDs`의 문장을 `a.Describe`로 prepare. 쿼리 오류면 `dialect.Bug`(7번). **실행으로 실패시킬 수 없는 안전띠다**: M2 비교를 통과한 DB는 모델과 같으므로 생성기의 버그로만 실패하고, 그 생성기는 `TestCRUDPrepares`가 방언마다 고정한다. 작업 글(모양 절 그림)에 있으므로 남기고, 이 한 줄로 이유를 적는다(fable 3)
- 생성: 모델 앱마다 `models_gen.go`(그대로), 쿼리 앱마다 `queries_gen.go` = `mapper.QueriesFile(d, 패키지 이름, checked)`

`Describe`가 치명 실패를 가르고, 파라미터 타입 오류에도 컬럼을 준다 (4번의 넘긴 질문, fable 2·6)
- 시그니처 `Describe(…) (cols []practicedb.Column, ds []diag.Diagnostic, fatal bool)`. `db.Describe`의 셋째 값(치명)이면 `nil, 그것, true`
- 파라미터 타입 검사는 prepare가 성공한 뒤라 컬럼은 유효하다. prepare 실패·조합마다 다른 컬럼·`:exec`+컬럼이 아니면 파라미터 진단과 함께 `cols`를 준다. 그러면 파라미터 타입 오류와 결과 타입 오류가 한 실행에 나온다(독립 오류를 두 왕복으로 나누지 않는다). 처음 계획의 "고치지 않는다"는 D-61을 잘못 읽은 것이었다

`Names`를 DB 앞으로 (8번과 달라지는 것)
- `Names(…, qs []*Query)`. 새 결과 타입은 `result()`와 같은 규칙으로 정적으로: `:exec`가 아니고 결과 이름이 그 앱의 모델 이름이 아니면 새 타입. 두 곳이 `modelNamed(app, name) *model.Model` 하나를 쓴다. 새 결과 타입은 **이름으로** 한 번만 더한다(지금은 `*Result` 포인터로: 같은 `R`을 쓰는 두 쿼리가 거짓 충돌이 되지 않게, fable 5). `gen_test.go`의 호출도 바꾼다
- 이득: DB에 닿지 않아도 이름 충돌이 나온다

`queries/`만 있는 디렉터리의 패키지 이름 (열린 질문, 1번에서 넘어옴)
- **package 절을 요구한다**(fable 4): 그 디렉터리의 생성되지 않은 비테스트 `.go` 파일의 package 절. 없으면 오류(위치 `dir/queries`, fix: "add a .go file with a package clause to dir, such as doc.go with package <name>"). 디렉터리 이름으로 짓는 것은 추측이다: 루트에서는 import 경로 끝과 다르고, 나중에 `main.go`를 더하면 빌드가 깨진다(PHILOSOPHY 6). 식별자·키워드 검사도 필요 없어진다(codex 5는 fallback이 없어져 해당 없음)
- 첫 파일의 것을 쓴다. 두 package 절은 Go 빌드가 막는다. 구문 오류는 `model.Read`가 앞 단계에서 모든 `.go`에 대해 먼저 내므로 `PackageClauseOnly`가 깨진 파일을 볼 일은 없다
- 모델 앱은 `app.Name`을 그대로 쓴다
- 새 `mapper.PackageName(root, dir string) (string, []diag.Diagnostic)`. 파일 거르기는 `declared`와 같다(`.`·`_`, 생성물, `_test.go`)

게이트 (완료 조건 4)
- 생성기가 import할 수 있는 경로를 `mapper.Imports`로 내보낸다(`context`, `database/sql`, `fmt`, `time`, `github.com/maago-web/maago`). `names.go`의 `imported`(이름)는 이것의 `path.Base`로 만든다
- 루트 `noreflect_test.go` 옆에 둘째 검사 `generatedImports(root string) []string`: 트리를 걸으며(`.git`만 건너뜀, `cmd/maago`·`testdata` 포함) `pipeline.IsGenerated`인 `.go`(CRLF 표시 포함, codex 6)의 import가 `mapper.Imports` 밖이면(`text/template`·`reflect`를 포함한다), 읽기·해석에 실패하면, `.tmpl`에 `"text/template"`·`"reflect"` 글이 있으면 문제 한 줄. 허용 목록이 금지 목록보다 세다: 템플릿이 `strconv`를 더하면 `Names`가 그 충돌을 놓치는 어긋남까지 잡는다(fable 7)
- `TestGeneratedImports`: `"."`에서 빈 것. 뜻이 있는 것은 gentest의 생성 골든이 표시를 달고 이 트리에 있기 때문(fable 7c). `TestGeneratedImportsGate`: 임시 트리에서 표시 있는 파일의 `text/template`, 표시 있는 파일의 `reflect`, CRLF 표시 파일의 `text/template`, 표시 있는 깨진 Go, `.tmpl`의 `"text/template"`가 각각 걸리고, 표시 없는 파일의 `text/template`와 표시 있는 파일의 허용 import는 통과

파일
- `internal/build/build.go`: `stages`의 `parse` 훅, `state`의 쿼리 필드, `checking`, `checkQueries`. `makemigrations.go`: `parse`에 nil
- `internal/mapper/describe.go`: 셋째 반환값과 파라미터 진단 + 컬럼. `names.go`: `[]*Query`, `Imports`, `PackageName`. `result.go`: `modelNamed`
- `cmd/maago/gen_test.go`·`mapper_test.go`: `Names`·`Describe` 호출
- `noreflect_test.go`: 게이트
- 새 `cmd/maago/queries_build_test.go`

하지 않기로 한 것
- `TestGenerated`의 `checkedQueries`를 `build`로 바꾸기: 골든이 gentest 자리에 가고 PG를 골라 돌리므로 그대로. `build`가 같은 것을 만드는지는 아래 시험이 골든과 비교해 본다
- 쿼리 파일만 바뀐 `check`를 빠르게 하기: D-65는 M9에서 잰다
- 생성 코드를 임시 모듈에서 `go vet`로 컴파일하기(처음 계획의 `reports/`): 새로 증명할 것은 package 절뿐이고, 컴파일은 gentest가, 사용자 모듈 모양은 10번이 본다(fable 11)

단계
1. `Describe`의 반환 → 확인: `TestDescribeQueryErrors`·`TestDescribeQueries`(두 방언) 그대로, 더해 파라미터 타입만 틀린 PG 쿼리가 진단과 컬럼을 함께 준다. 닫힌 연습용 DB에서 `fatal`이 참
2. `Names`의 `[]*Query`와 `modelNamed` → 확인: `TestNames`(DB 없음, codex 4)의 골든 그대로(같은 `R`의 두 쿼리가 거짓 충돌이 되지 않는 것이 판별, fable 5), 표에 `:exec` 쿼리(결과 없음)와 모델 이름을 결과로 쓴 쿼리(새 타입으로 세지 않음)를 더한다. `TestGenerated` 골든 그대로
3. `PackageName` → 확인: 표 시험(package 절 있는 파일 → 그것, 생성물만 → 오류, `_test.go`만 → 오류, 파일 없음 → 오류와 위치, `.`·`_`로 시작하는 파일만 → 오류)
4. `build` 연결 → 확인(`cmd/maago`, SQLite. `orders` 픽스처를 임시 프로젝트로 복사하고 `go.mod`, `makemigrations`로 `0001`):
   - `gen` → `check` 0. 생성된 `models_gen.go`·`queries_gen.go`가 gentest `sqlite/orders`의 골든과 같다. 쿼리만 있는 디렉터리 `reports/`(`doc.go`의 `package reports`, 모델 표를 읽는 쿼리 하나)도 `queries_gen.go`를 받고 `go/parser`로 읽은 package 절이 `reports`(codex 7, fable 11). `reports/`에서 `doc.go`를 지우면 `reports/queries` 위치의 오류
   - 쿼리 파일 하나를 고치면(`SearchOrders`의 `LIMIT`) `check`가 `queries_gen.go: generated file is out of date`. `queries/`를 모두 지우면 `gen`이 `queries_gen.go`를 지운다
   - 순서(fable 9): 모델에 필드 `Note string`을 더하고 마이그레이션 없이 `note`를 쓰는 쿼리를 두면 `check`는 마이그레이션 불일치만 내고 prepare 오류는 내지 않는다. 그 상태에서 `-- nmae: X`인 `.sql`을 넣어도 `makemigrations`는 초안을 쓴다(차이가 있을 때라 "초안을 쓴다"를 단언할 수 있다, fable 8)
   - 모델에서 `Priority`를 지우고 `makemigrations` → 초안이 써진다(`order_by_amount.sql`이 `priority`를 쓰는데도). `allow-drop` 표시 뒤 `check`는 `queries/order_by_amount.sql:줄:칸`의 오류 **하나**. 하나인 것이 판별이다: 생성물 비교나 다른 쿼리의 연쇄가 섞이면 줄이 는다(fable 12)
   - 여러 앱·여러 쿼리의 오류가 한 실행에(codex 2, fable 10): `orders`와 `reports`에 깨진 쿼리 하나씩(없는 컬럼) → `check`에 둘 다
   - 이름 충돌(쿼리 `GetOrder`)은 DB 없이: `MAAGO_DATABASE_URL`을 닿지 않는 PG 주소로 두면 이름 충돌만 나오고 DB 연결 오류는 없다(`early`에서 멈춤)
   - 태그 오류와 `.sql` 머리 오류가 한 실행에 함께(fable 1의 판별: `parse`가 3단계에 있다)
   - PG(`MAAGO_TEST_PG_URL`이 있을 때): `gen` → `check` 0, `queries_gen.go`가 gentest `postgres/orders` 골든과 같다. 앞 쿼리 하나가 prepare에 실패해도 뒤 쿼리와 CRUD prepare에서 "transaction is aborted"가 나오지 않고 그 쿼리의 오류만(codex 3: `SAVEPOINT`가 연결을 지킨다)
5. 게이트 → 확인: 위 두 시험, 그리고 일부러 템플릿에 `"strconv"` import를 넣으면 `TestGeneratedImports`(골든 갱신 뒤)가 실패(되돌림)
6. 두 모듈 `go vet`·`go test`(PG 포함), `gofmt -l .`, `go mod tidy -diff`

계획 리뷰 (codex·fable)
- 받음: A의 수명(codex 1), `checkQueries`의 모으는 법과 앱 둘의 시험(codex 2, fable 10), PG에서 실패한 쿼리 뒤의 prepare(codex 3), DB 없는 `TestNames` 표(codex 4), `IsGenerated`·CRLF·깨진 Go(codex 6), 파일·package 절 단언(codex 7), `parse`를 3단계로(fable 1), 치명 실패를 가름(fable 2), CRUD prepare는 안전띠로 이유를 적고 남김(fable 3), package 절 요구(fable 4), 이름으로 한 번(fable 5), `Describe`가 컬럼을 줌(fable 6), 허용 목록 게이트(fable 7), 확인 순서들(fable 8·9·12), `go vet` 대신 package 절(fable 11)
- 해당 없음: codex 5(디렉터리 이름 fallback을 뺐다)

구현에서 달라진 것
- SQLite는 오류 위치를 주지 않아 prepare 오류는 쿼리의 `-- name:` 자리다(4번 그대로). 시험의 위치를 그에 맞췄다. PG는 SQL 안의 자리
- `Names`는 모델 앱마다(그 디렉터리의 쿼리와 함께), 그다음 모델 없는 쿼리 앱마다 부른다
- 게이트의 `.tmpl` 검사는 파일 단위(줄·칸 없이). 템플릿은 둘뿐이다

구현 리뷰 (ponytail·codex·fable)
- 받음: 한 번 쓰는 도우미 `queryApp`·`dirs`를 `early` 안으로(ponytail 1), 시험은 `expect`를 쓰고 `expect`는 `diagnostics`를 쓴다(ponytail 2), `withColumns` 상수(ponytail 3), `.tmpl`은 `Contains`(ponytail 5), 게이트 시험에 `strconv`: 허용 목록을 금지 목록으로 바꾸는 변이가 살아남았다(fable 1), 쿼리만 있는 디렉터리의 `Names`: 빼는 변이가 살아남았다 → `reports/doc.go`의 `OrderCount`와 충돌(fable 2), `t.TempDir()`을 `Clean`(fable 4: `GOTMPDIR`에 `..`), 주석을 PG로 좁힘(fable 5). fable의 변이 13개 중 10개가 잡혔고 살아남은 셋 중 둘을 위에서 막았다
- 받지 않음: CRUD prepare 지우기(ponytail 4, 계획의 fable 3대로 안전띠로 남김), 앱을 정렬해 돌기(codex 1: `diag.Write`가 위치 순으로 정렬하므로 출력은 이미 결정적이다), `checkQueries`의 `fatal`을 빌드 수준에서 시험(fable 3: prepare로는 연결을 죽일 수 없다. `TestDescribeFatal`이 `Describe`를 덮고 호출자는 한 줄), `//go:build ignore`인 파일의 package 절(fable 6: `declared`와 같은 기존 맹점, TODO로)
- codex는 샌드박스가 임시 디렉터리를 막아 시험을 돌리지 못했다(코드만 읽은 리뷰). fable은 복사본에서 PG까지 돌렸다

## 10. 게시판 튜토리얼의 모델·마이그레이션·매퍼 단계 (D-103)

- D-103의 자리, 기대 상태 목록, 걷는 스크립트, 가운데 단계를 고치는 스크립트를 만든다
- 단계: 한 앱 `board`의 Board·Post·Comment 모델(같은 앱 안의 `fk=`, D-78. `fk=auth.User`는 M5), 복합 unique, `makemigrations`·`migrate`, 필드 추가·이름 변경, 글 목록(페이지)·상세·게시판별 검색 쿼리, 일부러 틀리는 단계 하나 이상(컬럼이 빠진 모델 결과 타입 등)과 그 기대 문구. User·첨부는 M5·M7의 몫
- 단계마다 기대 상태대로(통과 단계는 `maago check`와 테스트가 통과, SQLite. PG로 바꾸는 단계는 M8의 표 자리)

→ 확인: 걷는 스크립트가 처음부터 끝까지 기대 상태대로 돈다. 통과 단계의 쿼리 하나를 깨면 그 단계에서 멈추고, 일부러 틀린 단계를 고치면(기대한 오류가 나지 않으면) 그것도 실패한다

### 계획

사람의 결정 (2026-09-28, 계획 리뷰 뒤)
- 앱 시험의 DB: `go test`는 `board/`에서 돌므로 루트 `.env`를 보지 못한다(`maago.OpenDB`는 작업 디렉터리의 `.env`, D-101). 튜토리얼은 "`go test`는 `MAAGO_DATABASE_URL=sqlite:$PWD/board.db go test ./...`"로 한 줄 적고, 걷기도 똑같이 한다. 제대로 된 서비스 시험 방법은 U-07(M4)에서. 이 문제를 그 입력으로 TODO에(fable 1)
- 기대 상태는 커밋 메시지의 트레일러 `Maago-Expect:`(아래). 따로 목록 파일은 두지 않는다. 대가: 문구가 바뀌면 패치 머리를 고친다(`git apply`는 머리를 보지 않으므로 손으로 고쳐도 되고, unpack → reword → pack도 된다)
- Post의 `Status`는 필드만 지금(`choices`, 기본값 `'draft'`: D-106의 `Opt`가 한 번 더 보인다). 전이 규칙은 M4

자리와 모양 (D-103)
- `examples/board/steps/NNNN-제목.patch`: `git format-patch --root -N --zero-commit --no-signature --no-stat`의 출력 그대로. 파일 이름은 커밋 제목에서 나오고 `format-patch`는 비ASCII를 버리므로(fable 3) **제목은 영어 한 줄, 본문은 한국어**(튜토리얼 글과 짝). `-N`이라 단계를 끼워 넣어도 다른 파일의 Subject가 바뀌지 않는다. 생성물(`*_gen.go`), `.env`, DB 파일, `go.work`는 패치에 없다
- 트레일러: 커밋 메시지(패치의 머리 줄들 다음, 첫 `---` 줄 앞)의 `Maago-Expect: <진단 첫 줄 전체>`, 기대하는 진단마다 한 줄. 없으면 통과 단계. 메시지 안의 다른 `Maago-` 키는 걷기의 오류(오타가 조용히 "통과 단계"가 되지 않게, codex 4)
- `examples/board/steps.sh`(POSIX sh): `unpack DIR` = `git init` → `git am steps/*.patch` → `.git/info/exclude`에 `*_gen.go`·`.env`·`*.db`·`go.work*` → 이 저장소를 가리키는 `go.work`(`use .` + `replace`). `pack DIR` = `steps/*.patch`를 지우고 다시 뽑는다. 사람은 그 사이에 `git rebase -i`로 고친다. 고치지 않은 왕복은 같은 바이트다(저자·날짜가 패치에 있고 `am`이 지킨다). 고친 커밋은 날짜가 바뀔 수 있고 그것은 괜찮다(codex 3)
- `examples/board/README.md`: 쓰는 법 몇 줄. maago는 `go build -o …/maago ./cmd/maago`로 만들어 풀어 둔 디렉터리에서 부른다. 공개 저장소를 만들 때는 커밋마다 `maago gen`을 돌려 생성물도 커밋한다(D-67. 패치가 생성물을 빼는 것은 D-103, fable 5)
- 튜토리얼 모듈: `module example.com/board`, `require github.com/maago-web/maago v0.0.0-00010101000000-000000000000`과 `modernc.org/sqlite v1.59.0`(cmd/maago와 같은 판), `go.sum` 포함. `replace`는 패치에 없다(릴리스 전에는 태그한 판으로, TODO)

걷기 (`cmd/maago/tutorial_test.go`)
- `walk(stepsDir string) error`: 임시 디렉터리에서, 먼저 `go.work`(`use .`, `replace github.com/maago-web/maago => <저장소 루트>`)와 `.env`(`MAAGO_DATABASE_URL=sqlite:board.db`, 독자와 같은 상대 경로). 그리고 패치마다
  1. `git apply`(작업 디렉터리는 git 저장소가 아니어도 된다) → 트레일러 읽기
  2. `build.Gen` → 진단 첫 줄들이 기대와 **정확히** 같아야 한다(기대마다 같은 줄 하나, 남는 진단 없음, codex 5). 기대가 있는 단계는 여기서 다음 패치로
  3. 단계의 `.go` 파일이 `gofmt`(`go/format`)된 그대로인지
  4. `build.Check` 진단 없음 → `build.Migrate`(`.env`의 상대 경로 = 루트 기준 `board.db`. 앞 단계의 마이그레이션은 이미 적용되어 있다: 독자처럼 한 DB를 이어 쓴다)
  5. `go test ./...`: `MAAGO_DATABASE_URL=sqlite:<임시 절대 경로>/board.db`(위 결정, 같은 파일), `GOPROXY=off`(모듈 캐시만: cmd/maago 모듈을 시험하면 같은 판이 이미 캐시에 있다. 네트워크가 튜토리얼을 흔들지 않는다, codex 1), `GOFLAGS=-mod=readonly`(커밋한 `go.sum`이 낡으면 덮지 않고 실패), 작업 공간은 위 `go.work`(장치 하나, codex 2·fable 4). 실패하면 그 출력
  - 실패는 `NNNN-제목.patch: …`(어느 단계인지)
- `TestBoardTutorial`: `walk("../../examples/board/steps")`
- `TestBoardTutorialCatches`: 스텝을 임시로 복사해 고친 셋, 각각 그 단계 이름으로 실패하는지. (a) 통과 단계의 쿼리 `+` 줄을 없는 컬럼으로(gen 다리), (b) 일부러 틀린 단계의 틀린 줄을 바른 줄로(기대 다리: "expected … got none"), (c) 통과 단계 `board_test.go`의 기대값 한 줄을 뒤집음(`go test` 다리: 그 다리를 지우거나 DB 주소를 넘기지 않는 변이를 잡는다, fable 6)
- 앱 시험은 생성 함수를 직접 부른다(U-07은 M4). `_ "modernc.org/sqlite"`를 import(D-90: 시험은 `main`을 거치지 않는다, fable 5). 행이 단계마다 쌓이지 않게 시험 하나를 트랜잭션 안에서 돌리고 되돌린다. 이것은 M3 확인을 위한 것이지 튜토리얼이 가르치는 시험 패턴이 아니다(U-07 전, fable 1). 되돌리기를 잊으면 다음 단계의 같은 이름 게시판이 unique 위반으로 실패해 드러난다(codex 7은 따로 시험하지 않는다)

단계 (한 앱 `board`, SQLite. M8의 표에서 M3가 받는 줄)
1. `Start the project`: `go.mod`·`go.sum`, `main.go`(드라이버 import, `maago.OpenDB`로 열고 `Ping`: D-101을 처음 보여 준다). 모델 없음 → 통과
2. `Add Board, Post and Comment`: Board(`Name max=100 unique`), Post(`BoardID fk=Board`, `Title max=200`, `Body`, `Status choices=draft/published/archived default='draft'`, `CreatedAt auto=create`, `UpdatedAt auto=update`, 모델 태그 `unique=(board_id,title)`), Comment(`PostID fk=Post`, `Body`, `CreatedAt auto=create`) + `makemigrations`의 `migrations/0001_initial.sql` + `board/board_test.go`(Insert·Get·Update·Delete 왕복, `Status` 기본값, 같은 게시판의 같은 제목은 오류, 없는 게시판 FK는 오류)
3. `Add Post.Views`: `Views int64 default=0` + `0002`, 시험에 기본값
4. `Rename Comment.Body to Text`: + `makemigrations --rename board_comment.body:text`의 `0003`
5. `Add post queries`: `post_list.sql`(게시판의 글 목록, `limit`·`offset` 페이지, 최신 순, 새 타입 `PostSummary`, 댓글 수 `-- column: comments int64`), `post_detail.sql`(`:one`, 게시판 이름을 JOIN한 새 타입), `search_posts.sql`(게시판별, `q?`로 제목 검색: `{q}` 조건 줄) + 시험(페이지 경계, `q` 있음·없음, 없는 글의 `sql.ErrNoRows`)
6. `Try a model result that misses a column`(일부러 틀림): `recent_posts.sql :many Post`가 `body`를 빼고 고른다 → `Maago-Expect: board/queries/recent_posts.sql:…: result columns do not match model Post: missing "body"`
7. `Select every column of Post`: 모든 컬럼 + 시험

하지 않기로 한 것
- `maago` 바이너리를 만들어 CLI로 걷기: 같은 함수를 프로세스 안에서 부른다. CLI 배선은 `TestRun`·`TestGenCheck`가 본다
- PG로 바꾸는 단계(M8의 표 자리), User·첨부(M5·M7), 뷰·템플릿(M4), `Status` 전이(M4)
- 패치를 Go로 적용하기: `git`은 CI(checkout)와 개발 환경에 있다. 없으면 `walk`가 그 이유로 실패한다(건너뛰지 않는다)
- `migrate`와 앱 시험이 같은 파일을 여는지 따로 시험(codex 6): 둘 다 같은 절대 파일을 가리키고, 어긋나면 앱 시험이 빈 DB에서 "no such table"로 실패한다

계획 리뷰 (codex·fable)
- 받음: 모듈 캐시만으로 `go test`(codex 1), 작업 공간 하나(codex 2, fable 4), 바이트 같음은 고치지 않은 왕복만(codex 3), 엄격한 트레일러(codex 4), 첫 줄 전체 비교(codex 5), `.env`와 시험의 DB(fable 1 → 사람의 결정), 트레일러는 사람에게 보임(fable 2 → 사람의 결정), 영어 제목·`-N`·`--committer-date-is-author-date` 뺌(fable 3), 드라이버 import·`Status`·D-67 한 줄(fable 5), `go test` 다리의 변이와 비교 함수 공유(fable 6)
- 받지 않음: codex 6(위), codex 7(되돌리기를 잊으면 다음 단계의 unique 위반이 드러낸다)
- 모양 절의 "10번에서 DECISIONS에"(D-24 해석)는 11번으로 고친다(fable 5). D-103의 `NNN` → `NNNN`과 트레일러는 11번에서 D-103에

단계 (작업)
1. 스크래치에 튜토리얼 저장소를 만들며 단계 1~7을 커밋(각 단계에서 실제 `maago gen`·`makemigrations`·`check`·`go test`) → `steps.sh pack` → 확인: `unpack` 뒤 `pack`하면 `git diff --exit-code examples/`
2. `walk`와 두 시험 → 확인: `TestBoardTutorial` 통과, `TestBoardTutorialCatches`의 세 경우가 기대한 단계에서 잡힘. 걷기 시간을 잰다
3. 두 모듈 `go vet`·`go test`(PG 포함), `gofmt -l .`, `go mod tidy -diff`

구현에서 달라진 것
- `--no-stat`이면 메시지와 diff 사이에 `---` 줄이 없다. 메시지는 첫 `diff --git ` 줄 앞까지
- `go.work`의 디렉터리는 실제 경로여야 한다(macOS의 `/var` → `/private/var`): 임시 루트와 저장소 루트 모두 `EvalSymlinks`
- 첫 줄 비교는 `expect`(앞부분 비교)와 함수를 나누지 않았다. 걷기는 정확히 같아야 하고 오류를 돌려줘야 한다. `diagnostics`만 함께 쓴다
- 걷기는 약 4초, 망가뜨린 다섯 경우를 합쳐 약 16초(로컬, 캐시가 찬 뒤)

구현 리뷰 (ponytail·codex·fable)
- 받음: `gofmt -l`을 그대로 부름, 메시지 읽기를 `strings.Cut`으로, `steps.sh`의 usage 하나(ponytail 1·2·5). `pack`은 새 패치를 다 쓴 뒤에 옛것을 지운다, 저장소 경로도 `EvalSymlinks`, `unpack`이 실패하면 반쯤 만든 DIR을 지운다(codex 2·4·6). 망가뜨린 경우에 앞부분만 적은 기대와 gofmt 안 된 파일: fable의 변이 11개 중 살아남은 둘(fable 1·2). `unpack`이 `.env`를 쓴다(fable 3), README의 빌드 명령(fable 4: `cmd/maago`는 따로 모듈), 계획 글의 `expect` 문장(fable 6), 7단계는 `body`를 SELECT 끝에 붙여 "컬럼 순서는 달라도 된다"를 보인다(fable 7), `PostPage` → `PostWithBoard`(fable 8: M4의 `*Page` 뷰 타입과 헷갈린다), 2단계 메시지에 `Opt`(fable 9)
- 받지 않음: `GOWORK`·`GOFLAGS`를 뺌(ponytail 3·4: 바깥 환경의 값을 이기려는 것, 주석으로), README의 `go test` 줄 뺌(ponytail 6: 독자의 입구), 환경 변수 중복을 지우는 도우미(codex 1: `exec.Cmd`는 같은 키의 마지막 값을 쓴다), 임시 디렉터리 정리(codex 3: `t.TempDir`가 한다), 메시지의 트레일러 블록만 읽기(codex 5: 줄 머리의 `Maago-`만 보고, 본문에 그렇게 시작하는 줄은 쓰지 않는다), 걷기의 `build.Check` 뺌(fable 5: Gen 직후라 중복이지만 D-103이 단계마다 `check`를 적었고 독자가 돌리는 명령이다), 모르는 트레일러의 망가뜨린 경우(fable 11)
- 가르침 범위 메모(fable 10): 규칙 4의 `WHERE { }`는 이 단계들에 없다. M4 이후 검색 화면을 더할 때
- codex는 샌드박스가 `/tmp`를 막아 시험을 돌리지 못했다(코드만 읽은 리뷰). fable은 복사본에서 돌렸다

## 11. 완료 조건 종단 테스트와 문서

- 완료 조건 네 개를 증명하는 테스트 이름을 이 문서 끝에 적는다
- DECISIONS: 계획에서 생긴 결정(모양 절의 선택들, "선언한 파라미터는 쓰여야 한다"). ARCHITECTURE(§2 `mapper/`와 런타임 공개 이름, §3 매퍼 단계와 `makemigrations`, §5 CRUD 자리, §6 런타임도 `.env`를 읽음(D-101)), TUTORIAL-FUTURE §6(실제 문구와 시그니처), TODO, README, MILESTONES 체크

→ 확인: 두 방언, 두 모듈 `go vet`·`go test`, `gofmt -l`, `go mod tidy -diff`. CI에서 PG가 실제로 돈 것

### 계획

종단 테스트와 결정은 아래 "계획 리뷰"대로 고쳤다: `TestM3Done`은 조건 3만, 새 결정 D-107~D-112와 고침 D-66·D-101~D-104(DECISIONS C·D·G절, 변경 이력)

ARCHITECTURE
- §2: `mapper/` 줄을 실제 파일로(머리·본문·조합·묘사·결과·CRUD·생성·이름). 런타임 공개 이름(`DB`·`OpenDB`·`DBConfig`·`One`·`Many`·`Exec`·`ErrManyRows`·`ErrEmptySlice`·`Placeholders`·`Opt`·`Set`·`Ptr`·SQLite 시각 도우미)과 런타임이 기대는 내부 패키지는 `internal/diag`·`internal/project`뿐(6번 fable 3b)
- §3: 표의 해석·검사·DB 검사 줄에 매퍼 단계(D-112), `makemigrations`는 쿼리를 읽지 않음
- §5: 생성 파일에 `models_gen.go`(D-18 주석 + CRUD, 템플릿)와 `queries_gen.go`. "M1의 `models_gen.go`처럼 주석 몇 줄인 파일은 `fmt`로"를 지운다(8번). "allowed:" 예시에 `auto`
- §7: 둘째 검사(생성물의 import 허용 목록, `TestGeneratedImports`)
- §6은 이미 D-101을 적었다(확인만)

TUTORIAL-FUTURE §6
- 실제 시그니처와 문구로: `SearchOrders(ctx, db, SearchOrdersParams{…}) ([]OrderSummary, error)`, 선택 파라미터는 `maago.Ptr("paid")`, 슬라이스 파라미터 `ids`의 필드는 `Ids`, `:exec`는 영향 행 수(0행도 오류 아님)와 CRUD의 `sql.ErrNoRows`, `InsertX`가 돌려준 모델은 입력의 포인터를 그대로 담음, SQLite는 파라미터 타입을 맞춰 보지 않음, PG는 `time.Time`을 로컬 위치로 돌려주고 SQLite 도우미는 UTC. "일부러 틀려 보기"의 오류 문구를 실제 출력으로(게시판 6단계의 것)

TASKS-M3
- "먼저 확인한 사실"의 `UNION` 줄(5번), 모양 절의 "SQLite는 `?`"·"스칼라에 `$1…$n` 고정"(3번), 열린 질문 둘에 답(조합 상한 = 그대로 256·64, CRUD 이름·파일 = 7번)
- "M3 끝" 체크

나머지
- README: "M3(SQL 매퍼)를 하고 있다" → 끝남, M4 차례. MILESTONES M3 체크 넷
- TODO: M3 절에서 이미 끝난 것이 있으면 지우고, 남는 것 그대로
- push 뒤 GitHub Actions 로그에서 PG 시험이 실제로 돈 것(건너뜀 0)을 확인하고 "M3 끝"에 실행 번호

하지 않기로 한 것
- TASKS-M4 초안: "M3 끝"의 넷째 항목이고 다음 커밋(M2 끝과 같은 순서)

단계
1. `TestM3Done` → 확인: 두 방언 통과. 조건 3의 세 경우에서 오류 문구 한 글자를 바꾸면 실패(기대가 정확히 비교되는지), 스냅숏 비교를 빼는 대신 한 경우에 파일을 하나 쓰게 한 변이가 잡힘(손으로 한 번)
2. 문서 → 확인: 새 D-번호가 서로·기존과 겹치지 않음(`grep -o 'D-1[01][0-9]'`), 문서 안의 링크·번호가 가리키는 곳이 있음, TUTORIAL-FUTURE의 시그니처가 gentest 골든의 것과 같음(눈으로, 목록으로 적음)
3. 두 모듈 `go vet`·`go test`(PG 포함), `gofmt -l .`, `go mod tidy -diff`. push 뒤 CI 로그

계획 리뷰 (codex·fable) — 위 계획을 이렇게 고친다
- `TestM3Done`은 **조건 3만**(fable 12). 조건 2는 이미 이름 붙은 시험이 끝까지 증명한다: `TestSearchOrdersIsTheDecisionsExample`(픽스처 == DECISIONS 원문), `TestVariants`(조합별 글 골든), `TestGenerated`·`TestQueriesGenCheck`·`TestQueriesPG`(파이프라인이 만든 `queries_gen.go` == 골든), gentest `TestSearchOrders`(16조합 실행). 조건 1·4와 같은 방식
- 스냅숏이 무는 모양(fable 13): 먼저 `gen`을 성공시켜 생성물을 만든 뒤, **제대로 재생성되면 달라질 변경**(`SearchOrders`의 `LIMIT :limit OFFSET 0`)과 오류 파일을 함께 넣고 `gen` → 트리가 그대로. 실패한 `gen`이 무엇이든 쓰면 드러난다. 비교는 진단 첫 줄 **전체**가 같음(`expect`의 앞부분 비교를 쓰지 않는다, fable 14). 두 방언으로 돌리되 (a)(b)는 DB 앞에서 멈추고 (c)만 DB 단계임을 주석에(fable 15)
- 결정의 알갱이(fable 6~10, codex): D-107 머리와 바인딩(근거 칸에 네이티브 자리표시 금지의 대가: PG jsonb `?`·`?|`·`?&` → `jsonb_exists*`). D-108 자리표시(규칙 2의 문구). D-109 조합과 구역: 구역은 겹치지 않고 괄호 깊이 0에서만(fable 1), 상한 256·64(근거: 4번이 잰 글 64개 PG 약 40ms, fable 11), 빈 `WHERE { }`는 `SELECT`만, 빈 `SET { }`는 오류(규칙 4·5의 문구). D-110 결과 컬럼: `UNION`류는 출처를 버림, 필드 이름은 컬럼 이름이 출처 컬럼 이름과 같고 출처가 모델 필드일 때만 그 필드의 이름이고 별칭·식이면 거꾸로 snake(codex). D-111 실행 계약: CRUD 넷(`UpdateX`는 `auto=create` 컬럼을 쓰지 않는다, codex), `:one`·`:exec`, 빈 슬라이스와 선택지 밖의 값은 실행 오류(fable 5). D-112는 사용자에게 보이는 둘만: `queries/`만 있는 디렉터리의 package 절, `makemigrations`는 쿼리를 읽지 않음(단계 배치는 ARCHITECTURE §3, fable 9)
- 고침: D-66(생성물 import 허용 목록, fable 9), D-101(SQLite 파일은 `mode=rw`: 없으면 오류, 파일은 CLI가 만든다, fable 2), D-102(`auto`는 포인터 아닌 `time.Time`만, 대가: 있는 테이블에 `auto` 필드를 더하면 초안이 기본값 없는 NOT NULL 경고를 내고 사람이 손본다, fable 3·7, codex), D-103, D-104(`FROM` 서브쿼리는 여기서만, fable 10). 변경 이력 한 줄에 "규칙 2·4·5의 문구를 고쳤고 개수는 그대로"(fable 6)
- 문서: "allowed:" 목록은 `internal/model/check.go`의 실제 문구를 ARCHITECTURE와 TUTORIAL-FUTURE 두 곳에(fable 3a). SQLite는 파라미터 타입을 맞춰 보지 않음을 ARCHITECTURE §3에도(fable 4). PHILOSOPHY의 "SQL 규칙은 다섯 개"가 그대로인지 확인(fable D). TODO는 지울 것이 없으면 그대로

구현 리뷰 (ponytail·codex·fable)
- 받음: 시험 주석의 시험 이름 목록은 "M3 끝"에만, 계획의 뒤집힌 줄들을 한 줄로, import 허용 목록은 D-66에만(ARCHITECTURE §7은 가리키기만), §2 `mapper/`는 책임만, README·MILESTONES의 겹친 문장, TUTORIAL-FUTURE의 `Ids` 근거는 D-110에만(ponytail 1·3~9). PG의 `SAVEPOINT`는 prepare마다, 변경 이력의 순서, D-107의 "빈 슬라이스가 곧 없음"(fable 1~3)
- 받지 않음: 앞 두 경우를 SQLite만(ponytail 2: 두 방언에서 같은 문구가 나오는 것도 조건이고 PG 준비는 0.1초 안팎), D-103에서 앱 `go test`의 주소를 뺌(ponytail 10: 사람의 결정 2026-09-28을 적은 것)
- codex는 고칠 것 없음(샌드박스가 임시 디렉터리를 막아 코드만 읽음). fable은 PG까지 돌려 변이 9개: 정확 비교와 스냅숏이 각각 하중을 받는 것(빼면 살아남음)을 보였다. 알려진 천장: `fix:` 줄은 비교하지 않는다(첫 줄 전체만)

---

## M3 끝

- [x] 1~11 완료 (2026-09-28)
- [x] 위 M3 완료 조건 네 개를 실제로 돌려 확인. PG는 로컬 서버(`MAAGO_TEST_PG_URL`)에서 돌렸다. 로컬에서 URL 없이 건너뛴 실행은 확인이 아니다
  1. 게시판 튜토리얼의 모델·마이그레이션·매퍼 커밋: `cmd/maago`의 `TestBoardTutorial`(패치 7개, 일부러 틀린 단계 하나), 걷기가 망가진 단계를 잡는 것은 `TestBoardTutorialCatches`(10번)
  2. `SearchOrders`: 픽스처가 DECISIONS 원문과 같음 `TestSearchOrdersIsTheDecisionsExample`, 조합별 글 골든 `TestVariants`(`internal/mapper`, 3번), 파이프라인이 만든 `queries_gen.go`가 골든 `TestQueriesGenCheck`·`TestQueriesPG`(9번)·`TestGenerated`(8번), 16조합 실행 gentest `TestSearchOrders`(두 방언, 8번)
  3. 빈 WHERE의 DELETE, 라벨이 보장하지 않는 파라미터, 컬럼이 빠진 모델 결과 타입이 각각 생성 오류이고 아무것도 쓰지 않음: `cmd/maago`의 `TestM3Done`(두 방언, 11번). 경우별 표는 `TestVariantErrors`(3번)·`TestModelResult`(5번)
  4. 생성 코드에 `text/template`도 `reflect`도 없음: 루트의 `TestGeneratedImports`·`TestGeneratedImportsGate`(9번), `TestNoReflect`(M0)
- [x] MILESTONES.md의 M3 체크박스 갱신
- [ ] push 뒤 GitHub Actions 로그에서 PG 테스트가 실제로 돈 것(건너뜀 0)을 확인
- [ ] TASKS-M4 초안(다음 커밋)

## 열린 질문 (계획 단계에서 정한다)

- ~~조합 상한의 숫자~~ → 제안 그대로 256·64(4번이 잼: 글 64개 PG 약 40ms, D-109)
- ~~CRUD 함수 이름과 파일~~ → `GetX`·`InsertX`·`UpdateX`·`DeleteX`, `models_gen.go`(7번, D-111)

## 초안 리뷰 (2026-09-27)

codex·fable 모두 반영. 그 뒤 사람의 결정 D-101~D-105(위 결정 절). codex: CRUD SQL을 생성 전에 만들어 prepare(7번을 나눔), `SAVEPOINT`의 `ROLLBACK TO`·`RELEASE`와 그 확인, PG prepared statement 정리, OID 대응표, 슬라이스는 원소 1·2개로 prepare, 시각의 NULL·오프셋·잘못된 글, NULL은 바깥 조인만이 아님(P4), P4는 판정할 수 있는 규칙으로, `SearchOrders`의 `limit`은 원문을 고침, 조합 상한·빈 SET·빈 슬라이스를 미루지 않음, P2가 D-83을 넓힘(대가에), 런타임의 `.env`(P1 a를 환경 변수만으로, a′로 대안), SQLite 주소의 `?`는 maago가 붙임, `RETURNING` 실제 시험, `text/template` 게이트 범위, `makemigrations`의 쿼리, 튜토리얼의 기대 상태 목록, 같은 앱의 `fk=`, 확인 보강, P3에 bundle 선택지. fable: P2에 D-15의 모델 메서드 선택지, P1 숫자를 제안에, 시각 형식을 P5로, P4의 `?` 기호 표, 분류 못 하는 조인은 오류와 그 실패 모드, P4 (c) 기각 이유, 조합 수를 숫자로(fable의 12개는 틀림: 8개), 4·5번 확인 보강, `SearchOrders` 픽스처 자리, P3의 생성물 비용과 테스트가 DB를 얻는 법, `:one`의 두 행, `bool?` 금지, 슬라이스는 빈 것이 없음, PG 자리표시 고정, 빈 SET은 정적 오류, 0행은 `sql.ErrNoRows`, 게이트는 범위가 다른 둘째 검사, `makemigrations`는 쿼리를 읽지 않음, 이름 충돌, 머리 줄 판정, 7번 돌연변이, `Equal`, `queries/`만 있는 앱은 ARCHITECTURE가 답함, D-24 기록
