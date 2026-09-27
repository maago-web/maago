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
- **출처로는 NULL 여부를 알 수 없다.** `LEFT JOIN`된 쪽의 `x.id`도 두 드라이버 모두 NOT NULL인 원래 컬럼으로 보고한다. `CASE`·`COALESCE`·집계·스칼라 서브쿼리·`UNION`도 NULL을 낼 수 있고, 이것들은 출처가 없다
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
- **모든 `:이름`은 머리에 선언되어야 하고, 선언한 파라미터는 쓰여야 한다.** 규칙 1의 해석이고 새 규칙이 아니다. 10번에서 DECISIONS에 적어 D-24 "다섯 개"가 지켜진 것이 보이게 한다. DECISIONS의 `SearchOrders` 예제는 `:limit`을 선언하지 않았다 → 예제 원문에 `-- param: limit int64`를 더했다(이 초안 커밋). 3·8번의 픽스처는 그 원문과 글자까지 같다
- **Go 쪽 모양**(TUTORIAL-FUTURE §6): `func SearchOrders(ctx context.Context, db maago.DB, p SearchOrdersParams) ([]OrderSummary, error)`. 파라미터가 없으면 `p`도 없다. 선택지는 이름 붙은 문자열 타입과 상수, 모르는 값은 실행 때 오류(조용히 기본값으로 가지 않는다). 쿼리 이름·파라미터 타입·새 결과 타입·CRUD 이름이 같은 패키지의 사용자 식별자(뷰 `PostList` 등)나 서로와 겹치면 생성 오류(M1이 이미 `go/ast`로 패키지를 읽는다)
- **조합**: 조건 라벨이 가질 수 있는 값(선택 파라미터 있음·없음, bool 참·거짓, 슬라이스 원소 있음·없음, 선택지의 각 값)의 모든 조합을 만들고, 번역한 SQL 글이 같은 것은 하나로 친다. 상한 제안: 원 조합 256개, 글 64개. 넘으면 생성 오류(fix: 쿼리를 나누거나 Go에서 bool로). 근거: M2 12번의 PG `check` 약 105ms, 글 64개의 prepare는 그 안팎(3번에서 잰다, D-65)
- **번역**: 생성기가 조합마다 고정된 SQL 조각을 Go 코드로 쓴다(D-22). 런타임은 SQL을 해석하지 않는다. PG 자리표시는 스칼라에 `$1…$n`을 생성 때 고정으로 주고, 슬라이스만 실행 때 `$n+1…`로 뒤에 붙인다(런타임이 만드는 글은 슬라이스 자리뿐). SQLite는 `?`. `::`(PG 형 변환), 문자열·주석·달러 인용 안의 `:`·`{`는 파라미터나 조건이 아니다(M2 3번의 토큰화)
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

## 7. PK CRUD의 SQL (D-21)

DB 검사가 CRUD SQL도 prepare하려면 생성 전에 SQL이 있어야 한다. 그래서 SQL을 만드는 일과 Go 코드를 쓰는 일(8번)을 나눈다.

- 모델마다 `Get`·`Insert`·`Update`·`Delete`의 방언별 SQL. `Insert`는 `m.ID`를 무시하고 `RETURNING id`로 받아 채운다(SQLite 3.35+·PG). `Update`·`Delete`가 0행이면 `sql.ErrNoRows`(M4가 404로, 조용히 성공하지 않는다). D-102의 자동 시각(M1 태그 검사에 `auto`를 더한다: `time.Time`에만, 값은 `create`·`update`)
- DB 검사 단계에서 4번과 같은 자리로 prepare한다. 실패는 maago의 버그(`dialect.Bug`)
- 함수 이름(`GetPost`·`InsertPost` …)과 파일(`models_gen.go` 또는 `crud_gen.go`)은 계획에서. 이름 충돌은 모양 절

→ 확인: 공용 예제 모델(FK, NULL, 기본값, 예약어 컬럼)의 CRUD SQL 골든(두 방언)이 연습용 DB에서 prepare된다. `RETURNING`이 modernc v1.59.0과 PG 15에서 실제로 id를 돌려준다

## 8. 코드 생성 (`queries_gen.go`, CRUD)

- `text/template`(`embed.FS`) + `go/format`(ARCHITECTURE §5). 파라미터 struct, 선택지 타입, 새 결과 타입, 쿼리 함수(조합마다 고정 조각을 잇는 코드), CRUD 함수
- 생성 코드는 방언별이다(자리표시, SQLite 시각 변환). D-31: 프로젝트는 방언 하나
- `SearchOrders` 픽스처: `Order` 모델(`table=orders`, `status`·`amount`·`created_at`)을 둔 예제 앱을 `internal/testdata/`에 둔다. 3번(글 골든)과 이 작업(생성·실행)이 같은 픽스처를 쓴다

→ 확인: 예제 앱의 생성 골든(두 방언). 생성 코드가 컴파일되고 두 방언의 실제 DB에서 돈다(`cmd/maago` 모듈): CRUD 왕복(nullable FK, 시각, 0행 `Update`·`Delete`의 `sql.ErrNoRows`), `SearchOrders`의 조합(두 정렬의 첫 행이 다름을 단언해 `sort`를 무시하는 돌연변이를 잡는다, 선택 파라미터 있음·없음의 행 수), `:one`의 `sql.ErrNoRows`, 선택지 밖의 값, 새 결과 타입의 NULL(`*T`는 `nil`, 선언되지 않은 NULL은 Scan 오류), `*sql.Tx` 안에서 부르기

## 9. 파이프라인 연결과 게이트 (`internal/build`)

- 해석 단계에 매퍼 파일 읽기, 검사 단계에 1~3번과 7번의 SQL, DB 검사 단계에서 M2 비교가 통과한 뒤 4·5번과 CRUD prepare, 생성 단계에 8번. `makemigrations`는 쿼리 파일을 읽지 않는다(모양 절)
- 게이트(완료 조건 4): 기존 `TestNoReflect`(저장소 전체, 생성기 포함)와 **범위가 다른 둘째 검사**. 생성기는 `text/template`을 정당하게 쓰므로, `text/template`·`reflect` 금지는 생성물(첫 줄에 표시가 있는 파일, 생성 골든, `.tmpl`)에만. 루트를 인자로 받는 함수로 두어 일부러 틀린 생성물은 임시 트리에서 시험한다(틀린 골든은 커밋할 수 없다)

→ 확인: `gen` → `check` 0, 쿼리 파일을 바꾸면 `check` 1(생성물이 낡음). 모델 필드를 지우고 `makemigrations`하면 그 필드를 쓰는 쿼리가 있어도 초안이 나오고, `check`는 그 쿼리의 오류. 잘못된 `.sql`이 있어도 `makemigrations`는 돈다. 게이트는 임시 트리에 `text/template`을 import한 표시 있는 파일을 넣어 실패하고, 표시 없는 파일(생성기 코드)은 통과(WORKFLOW 5)

## 10. 게시판 튜토리얼의 모델·마이그레이션·매퍼 단계 (D-103)

- D-103의 자리, 기대 상태 목록, 걷는 스크립트, 가운데 단계를 고치는 스크립트를 만든다
- 단계: 한 앱 `board`의 Board·Post·Comment 모델(같은 앱 안의 `fk=`, D-78. `fk=auth.User`는 M5), 복합 unique, `makemigrations`·`migrate`, 필드 추가·이름 변경, 글 목록(페이지)·상세·게시판별 검색 쿼리, 일부러 틀리는 단계 하나 이상(컬럼이 빠진 모델 결과 타입 등)과 그 기대 문구. User·첨부는 M5·M7의 몫
- 단계마다 기대 상태대로(통과 단계는 `maago check`와 테스트가 통과, SQLite. PG로 바꾸는 단계는 M8의 표 자리)

→ 확인: 걷는 스크립트가 처음부터 끝까지 기대 상태대로 돈다. 통과 단계의 쿼리 하나를 깨면 그 단계에서 멈추고, 일부러 틀린 단계를 고치면(기대한 오류가 나지 않으면) 그것도 실패한다

## 11. 완료 조건 종단 테스트와 문서

- 완료 조건 네 개를 증명하는 테스트 이름을 이 문서 끝에 적는다
- DECISIONS: 계획에서 생긴 결정(모양 절의 선택들, "선언한 파라미터는 쓰여야 한다"). ARCHITECTURE(§2 `mapper/`와 런타임 공개 이름, §3 매퍼 단계와 `makemigrations`, §5 CRUD 자리, §6 런타임도 `.env`를 읽음(D-101)), TUTORIAL-FUTURE §6(실제 문구와 시그니처), TODO, README, MILESTONES 체크

→ 확인: 두 방언, 두 모듈 `go vet`·`go test`, `gofmt -l`, `go mod tidy -diff`. CI에서 PG가 실제로 돈 것

---

## M3 끝

- [ ] 1~11 완료
- [ ] 위 M3 완료 조건 네 개를 실제로 돌려 확인(PG는 로컬 서버나 CI. 건너뛴 실행은 확인이 아니다)
- [ ] MILESTONES.md의 M3 체크박스 갱신
- [ ] TASKS-M4 초안(다음 커밋)

## 열린 질문 (계획 단계에서 정한다)

- 조합 상한의 숫자(모양 절의 제안, 3번에서 재 보고 고친다)
- CRUD 함수 이름과 파일(7번)

## 초안 리뷰 (2026-09-27)

codex·fable 모두 반영. 그 뒤 사람의 결정 D-101~D-105(위 결정 절). codex: CRUD SQL을 생성 전에 만들어 prepare(7번을 나눔), `SAVEPOINT`의 `ROLLBACK TO`·`RELEASE`와 그 확인, PG prepared statement 정리, OID 대응표, 슬라이스는 원소 1·2개로 prepare, 시각의 NULL·오프셋·잘못된 글, NULL은 바깥 조인만이 아님(P4), P4는 판정할 수 있는 규칙으로, `SearchOrders`의 `limit`은 원문을 고침, 조합 상한·빈 SET·빈 슬라이스를 미루지 않음, P2가 D-83을 넓힘(대가에), 런타임의 `.env`(P1 a를 환경 변수만으로, a′로 대안), SQLite 주소의 `?`는 maago가 붙임, `RETURNING` 실제 시험, `text/template` 게이트 범위, `makemigrations`의 쿼리, 튜토리얼의 기대 상태 목록, 같은 앱의 `fk=`, 확인 보강, P3에 bundle 선택지. fable: P2에 D-15의 모델 메서드 선택지, P1 숫자를 제안에, 시각 형식을 P5로, P4의 `?` 기호 표, 분류 못 하는 조인은 오류와 그 실패 모드, P4 (c) 기각 이유, 조합 수를 숫자로(fable의 12개는 틀림: 8개), 4·5번 확인 보강, `SearchOrders` 픽스처 자리, P3의 생성물 비용과 테스트가 DB를 얻는 법, `:one`의 두 행, `bool?` 금지, 슬라이스는 빈 것이 없음, PG 자리표시 고정, 빈 SET은 정적 오류, 0행은 `sql.ErrNoRows`, 게이트는 범위가 다른 둘째 검사, `makemigrations`는 쿼리를 읽지 않음, 이름 충돌, 머리 줄 판정, 7번 돌연변이, `Equal`, `queries/`만 있는 앱은 ARCHITECTURE가 답함, D-24 기록
