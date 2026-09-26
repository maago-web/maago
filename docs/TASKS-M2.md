# M2 작업 목록 — 연습용 DB와 마이그레이션

[MILESTONES.md](MILESTONES.md)의 M2를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M2 완료 조건** (MILESTONES)
- 빈 프로젝트에서 모델 추가 → 필드 추가 → 필드 이름 변경 → 필드 삭제를 차례로 했을 때,
  모든 마이그레이션을 적용한 결과가 모델과 같다 (SQLite, PG 모두)
- 초안을 손으로 고쳐 모델과 달라지게 만들면 `check`가 실패한다
- `allow-drop` 없는 `DROP`은 `check`가 실패한다

**M1에서 넘어온 것** ([TASKS-M1.md](TASKS-M1.md) 끝)
- `.env`의 `MAAGO_DATABASE_URL`(D-08)을 읽어 방언을 정하고, DDL 단계를 파이프라인에 넣는다. PG의 63바이트 검사(`postgres.DDL`)는 그때 `gen`에 들어간다
- 스키마 비교 표현(`internal/schema`)은 DB에서 읽어 오는 쪽에 맞춰 만든다. 제약은 이름이 아니라 컬럼 집합으로 맞춘다(D-87)
- SQLite 연결마다 `PRAGMA foreign_keys = ON`([TODO.md](TODO.md))
- 스모크 스크립트는 연습용 DB가 생기면 Go 테스트로 옮길지 정한다

---

## 결정 (2026-09-26)

- 드라이버: SQLite는 `modernc.org/sqlite`, PG는 `github.com/jackc/pgx/v5/stdlib`. `cmd/maago` 모듈에만(D-89)
- 루트 모듈은 `database/sql`만 쓴다. 드라이버 등록(blank import)과 실제 DB를 여는 테스트는 `cmd/maago` 모듈에 둔다. 루트 `go.mod`는 계속 `require`가 없다(D-90)
- PG 테스트는 CI의 PostgreSQL 서비스에서 돈다. 로컬은 `MAAGO_TEST_PG_URL`이 없으면 이유를 찍고 건너뛴다. CI에서는 그 변수가 없으면 실패한다(D-91)
- 스키마 비교는 CHECK까지 본다. SQLite는 CHECK가 카탈로그에 없으므로 `sqlite_schema`의 `CREATE TABLE` 원문을 토큰화해 꺼낸다(D-92)
- `allow-drop`은 바로 다음 문장 하나에, 모든 파일에 요구한다(D-37 고침)
- SQLite 재생성의 `DROP TABLE`은 파일 모양으로 알아본다. 재생성으로 사라지는 컬럼이 있으면 표시가 필요하다(D-93)
- 파일 이름은 변경에서 짓고 `--name`으로 덮는다. 파일 머리는 없다(D-94)
- 적용한 파일의 내용 해시를 기록하고, 바뀌면 오류(D-95)
- 남은 PG 연습용 스키마는 오류로 알리고 지우는 SQL을 fix에(D-96)

---

## 모양 (작업들이 함께 따르는 것)

M2 전체의 뼈대. 작업마다 계획에서 구체화하고, 여기와 달라지면 여기부터 고친다.

```
                 ┌─ 연습용 DB A: migrations/*.sql 적용 ─▶ 읽기 ─┐
 모델(M1) ──────┤                                              ├─▶ 비교 ─▶ 차이
                 └─ 연습용 DB B: 모델 DDL(M1) 적용 ────▶ 읽기 ─┘
```

- **양쪽을 모두 DB에서 읽는다.** 목표 스키마를 모델에서 직접 만들지 않고, M1의 DDL을 두 번째 연습용 DB에 적용해 같은 읽기 함수로 읽는다. 기본값·타입·CHECK의 정규화를 DB가 해 주므로(PG의 `'draft'::character varying` 같은 것) 비교는 읽은 것끼리의 같음만 보면 된다. 읽기의 빈틈은 양쪽에 똑같이 나타나 서로 지운다. M1의 DDL 골든과 스모크가 B를 이미 보증한다
- 읽기의 빈틈이 양쪽에서 지워진다는 것은 양쪽 모두 무언가를 놓쳐도 통과한다는 뜻이기도 하다. 그래서 읽기 함수는 종단 테스트와 따로, 알려진 스키마를 읽어 기대한 구조와 맞추는 자체 테스트를 가진다(4·5번)
- 차이는 두 방식으로 쓰인다. `check`·`gen`: 마이그레이션이 적용되지 않거나 차이가 있으면 진단(D-60, D-64). `makemigrations`: 마이그레이션은 적용되어야 하지만 차이는 허용하고, 그 차이가 초안이 된다. 마이그레이션이 하나도 없으면 A는 빈 스키마다
- 연습용 DB와 적용 순서는 **연결 하나**(`*sql.Conn`) 위에서 일어난다. `*sql.DB`는 풀이라 SQLite `:memory:`와 PG `search_path`가 연결마다 다르다
- 이전 상태는 기록하지 않는다(D-34). 실제 DB의 적용 기록(`maago_migrations`)은 `migrate`·`showmigrations`만 본다

---

## 1. `.env`와 DB 주소 (`internal/project`)

- 프로젝트 루트의 `.env`에서 `MAAGO_DATABASE_URL`을 읽고, 같은 이름의 환경 변수가 이긴다(D-08, ARCHITECTURE §6)
- `.env` 해석은 `KEY=VALUE`, `#` 주석, 따옴표 벗기기만. 모르는 형식의 줄은 `.env:줄` 위치의 진단
- 방언은 스킴에서: `sqlite:경로`, `postgres://`·`postgresql://`. 그 밖은 진단(fix에 두 형식)
- 주소가 없으면 진단(fix: `.env`에 적는 예). D-64에 따라 `gen`·`check`는 DB 없이 통과하지 않는다
- 환경 변수를 읽는 함수는 인자로 받아 테스트가 프로세스 환경을 건드리지 않게 한다

→ 확인: 표 테스트(주석·빈 줄·따옴표·`export` 없음·`=` 없는 줄·같은 키 두 번·모르는 스킴·환경 변수 우선·`.env` 없음). 오류 위치가 `.env:줄`

## 2. 연습용 DB (`internal/practicedb`)

- `Open(ctx, dialect, url) (*DB, []diag)`와 `Close`. 드라이버 이름(`sqlite`, `pgx`)으로 `sql.Open`하고, 드라이버는 `cmd/maago`가 등록한다(D-90). `DB`는 `*sql.Conn` 하나를 쥐고, 모든 일(PRAGMA, 스키마 생성, 적용, 읽기, 삭제)이 그 연결에서 일어난다
- `Exec(stmts)`: 3번이 나눈 문장을 차례로 실행하고, 실패하면 그 문장의 위치로 진단. 7·8·9번이 모두 이것으로 적용한다
- SQLite: `:memory:`. 연결을 쥔 직후 `PRAGMA foreign_keys = ON`을 켜고 다시 읽어 켜졌는지 확인한다(TODO에서 넘어옴). `.env`의 파일 이름은 쓰지 않는다(ARCHITECTURE §6)
- PG: 개발 DB에 `maago_practice_<무작위>` 스키마를 만들고, 같은 연결에서 `search_path`를 그 스키마로. `Close`도 같은 연결에서 `DROP SCHEMA … CASCADE`. 마이그레이션과 DDL은 스키마 이름 없이 쓰이므로 그대로 적용된다(D-35)
- 한 번의 실행에 둘(A와 B)을 연다. 여는 김에 `maago_practice_`로 시작하는 스키마가 이미 있으면(비정상 종료로 남은 것) 위치 없는 진단, fix에 `DROP SCHEMA "…" CASCADE;` 문장. 지우지 않는다(D-96). 동시에 도는 다른 실행의 스키마와 구분하는 법은 계획에서
- 연결·스키마 생성 실패는 위치 없는 진단, fix에 켜는 법(D-64): PG 서버를 켜라, 주소를 확인하라, 권한(`CREATE` on database)
- 드라이버가 등록되지 않은 경우(루트 모듈 테스트)도 같은 진단 경로로 실패한다. 조용히 건너뛰지 않는다

→ 확인
1. 루트 모듈: 드라이버 없이 `Open`이 D-64 진단으로 실패한다
2. `cmd/maago` 모듈(D-90): SQLite 연습용 DB에서 두 번 연 것이 서로 보이지 않고, FK가 강제된다. PG에서 스키마가 생기고 `Close` 뒤 사라지며, 사용자 스키마(`public`)의 테이블은 보이지도 바뀌지도 않는다. 닫힌 포트의 주소는 D-64 진단
3. PG에 `maago_practice_` 스키마를 미리 만들어 두면 진단이 나오고, 그 fix의 SQL을 돌리면 다음 실행이 통과한다. 미리 만든 스키마는 지워지지 않는다

## 3. SQL 토큰화와 문장 나누기 (`internal/dialect`)

- 두 곳에 쓴다: 마이그레이션 파일을 문장으로 나누기(2·8번, DB 오류를 문장의 `파일:줄`로 짚고 PG의 여러 문장 제한을 피한다), SQLite `CREATE TABLE` 원문의 구조 읽기(4번, D-92)
- 문자열(`'…'`, `''`), 인용 식별자(`"…"`), 주석(`--`, `/* */`), PG의 `$tag$…$tag$`(여는 태그와 같은 태그에서만 닫힌다)를 안다. 줄·칸 위치를 가진다
- `-- maago: allow-drop`(D-37)은 **바로 다음 문장 하나**에 붙는다. 한 파일의 여러 `DROP`은 하나씩 허락한다. 모르는 `-- maago:` 표시와 문장이 따라오지 않는 표시는 진단(닫힌 목록, D-17과 같은 원칙)
- 정규화: 공백을 하나로, 키워드는 대문자, 인용 식별자와 문자열은 그대로. 비교에만 쓴다

→ 확인: 표 테스트. 문장마다 내용과 시작 줄·칸을 본다(세미콜론이 든 문자열·주석, `$$`·`$fn$` 본문, 서로 다른 태그가 겹친 본문, 닫히지 않은 문자열·주석·달러 인용의 위치 있는 진단, 빈 문장, 마지막 세미콜론 없음, CRLF). 표시가 다음 문장에만 붙는지

## 4. 스키마 표현과 SQLite 읽기 (`internal/schema`, `internal/dialect/sqlite`)

- `schema.Schema`: 테이블마다 STRICT 여부, 컬럼(이름, 타입, NOT NULL, 기본값, 순서), PK(컬럼과 자동 생성 방식: SQLite `AUTOINCREMENT`, PG IDENTITY `BY DEFAULT`), FK(컬럼 목록, 대상 테이블·컬럼, ON DELETE), unique(컬럼 집합), index(컬럼 목록), CHECK(붙은 컬럼 또는 테이블, 정규화된 식). 이름이 아니라 컬럼으로 맞춘다(D-87)
- SQLite 읽기: `sqlite_schema`, `pragma_table_xinfo`(`pk` 순서, `hidden`), `pragma_index_list`(`origin`, `partial`)·`index_xinfo`(`cid = -2`는 식), `pragma_foreign_key_list`(여러 컬럼 FK는 `seq`로 묶는다). `AUTOINCREMENT`·STRICT·CHECK는 원문을 3번의 토큰으로 읽어 컬럼 정의와 테이블 제약을 최상위 쉼표로 나눈 뒤 꺼낸다(D-92). `sqlite_sequence`와 `maago_migrations`는 뺀다
- 표현에 없는 것은 조용히 넘기지 않고 진단(PHILOSOPHY 7): 부분 인덱스, 식 인덱스, generated 컬럼, `WITHOUT ROWID`, 트리거, 뷰. v0은 마이그레이션에서 이것들을 막는다(필요해지면 표현을 넓힌다 → TODO)

→ 확인 (`cmd/maago` 모듈)
1. M1 공용 예제의 SQLite DDL을 적용해 읽은 결과가 기대한 구조와 같다(골든, 모델을 거치지 않는 자체 테스트)
2. 공백·대소문자만 다르게 손으로 쓴 같은 스키마는 같게 읽힌다
3. 컬럼 CHECK와 테이블 CHECK, 한 컬럼의 CHECK 둘, 문자열 안의 괄호·쉼표, 여러 컬럼 FK, 복합 PK가 제자리에 읽힌다
4. 위의 막는 것들이 각각 진단

## 5. PG 읽기 (`internal/dialect/postgres`)

- `pg_catalog`에서, 모든 조회에 연습용 스키마의 `nspname` 조건. 타입은 `format_type`, 기본값은 `pg_get_expr`, CHECK 식은 `pg_get_constraintdef`에서 식만. FK는 문자열이 아니라 `pg_constraint`의 `conkey`·`confrelid`·`confkey`·`confdeltype`으로, 제약 이름은 버린다. IDENTITY는 `attidentity`
- 4번과 같은 표현, 같은 "표현에 없는 것은 진단" 규칙

→ 확인 (`cmd/maago` 모듈, D-91): 4번과 같은 경우들을 PG로. `public`에 같은 이름의 테이블을 만들어 두어도 읽히지 않는다

## 6. 비교 (`internal/migrate`)

- `Diff(from, to schema.Schema) []Change`. 테이블·컬럼 추가·삭제, 컬럼 타입·NULL·기본값·CHECK 변경, PK·FK·unique·index 추가·삭제
- 변경은 결정적인 순서로(테이블은 모델 순, 삭제는 의존의 역순). 초안과 진단이 같은 순서를 쓴다
- 진단 문구: `migrations do not match models: column "body" of "blog_post" is missing`처럼 테이블·컬럼을 DB 이름으로. 위치는 모델 필드(모델에서 온 쪽이면) 또는 없음. fix는 `run maago makemigrations`
- 이름 변경은 여기서 판정하지 않는다. 비교는 삭제+추가를 낸다(D-37: 결과 스키마만으로는 구분되지 않는다)

→ 확인: 표 테스트(두 `Schema` → 변경 목록), unique의 `(a,b)`와 `(b,a)`는 같음, index는 다름(M1 5번)

## 7. 초안 DDL (`internal/dialect/sqlite`, `internal/dialect/postgres`)

- 변경 목록 → 방언별 SQL. 새 테이블은 M1의 `DDL`을 그 테이블만큼 재사용한다
- PG: `ALTER TABLE … ADD/DROP COLUMN`, `ALTER COLUMN … TYPE/SET/DROP NOT NULL/SET/DROP DEFAULT`, 제약·인덱스 `ADD`/`DROP`
- SQLite: `ADD COLUMN`으로 되는 것은 그것으로, 나머지는 **테이블 재생성**(SQLite 문서의 12단계: 새 테이블 생성 → 복사 → 옛 테이블 삭제 → 이름 바꾸기 → 인덱스 다시 만들기)
- 재생성의 `DROP TABLE t`는 데이터를 버리지 않는다. 앞에 `INSERT INTO x … FROM t`, 뒤에 `ALTER TABLE x RENAME TO t`가 있으면 allow-drop 대상이 아니다(D-93, 8번이 검사)
- 단, 모양만으로는 옮기는 `SELECT`가 컬럼을 빠뜨린 것을 알 수 없다. 재생성 앞뒤의 스키마를 연습용 DB에서 읽어 사라진 컬럼이 있으면 그 `DROP TABLE`에 표시를 요구한다(D-93). 필드 삭제는 되도록 `ALTER TABLE … DROP COLUMN`(SQLite 3.35+)으로 쓰고, 그럴 수 없을 때(인덱스·FK·CHECK에 걸린 컬럼)만 재생성한다. 구체적인 방법은 7번 계획
- `NOT NULL`이고 기본값 없는 컬럼 추가는 기존 행이 있으면 실패한다. 초안에 ⚠ 주석으로 알리고 그대로 둔다(추측해서 값을 채우지 않는다, PHILOSOPHY 6)
- `DROP`(테이블·컬럼)은 초안에 쓰되 `-- maago: allow-drop`은 **붙이지 않는다**. 대신 ⚠ 주석으로 무엇을 지우는지와 표시를 다는 법을 적는다. 기계가 표시까지 붙이면 D-37이 아무것도 막지 못한다

→ 확인: 변경 종류마다 초안 골든(두 방언). 초안을 2번의 `Exec`로 연습용 DB에 적용하면 `Diff`가 비는 것을 경우마다 확인(`cmd/maago` 모듈). SQLite 재생성은 행과 FK가 있는 테이블로

## 8. 마이그레이션 파일과 DB 검사 단계 (`internal/migrate`, `internal/build`)

- `migrations/NNNN_이름.sql`, 프로젝트 루트에 하나(D-38, D-94). 번호는 네 자리, 1부터 빈틈·겹침 없이. 어긋나면 파일 위치의 진단. 파일 머리는 없다(D-94)
- 해석 단계에 마이그레이션 파일 읽기(문장 나누기, 표시 줄)를 더한다(ARCHITECTURE §3)
- **DB 검사 단계**(ARCHITECTURE §3의 자리): 연습용 DB A에 모든 파일을 순서대로 적용, B에 모델 DDL, 둘을 읽어 비교. 차이는 진단
  - 적용 오류는 그 문장의 `파일:줄`(PG가 주는 위치가 있으면 칸까지)과 DB의 메시지
  - 바로 앞에 `-- maago: allow-drop`이 없는 `DROP`(테이블·컬럼) 문장은 그 위치의 진단(D-37, 3번). SQLite 재생성은 7번의 규칙대로
  - 모든 파일에 요구한다. `check`는 실제 DB를 보지 않으므로 적용 여부를 모르고, 표시는 파일에 남는 기록이다(D-37)
- `makemigrations`(9번)는 같은 단계를 비교의 차이를 진단으로 바꾸지 않는 모드로 쓴다(모양 절). 단계 목록을 두 벌 만들지, 비교 단계만 바꿀지는 계획에서
- 방언을 알게 되었으므로 PG 프로젝트는 `postgres.DDL`의 63바이트 검사가 이 단계 전에 돈다(M1에서 넘어옴)
- `gen`·`check`는 같은 단계를 돈다. 모델을 바꾸고 `makemigrations`를 잊으면 `gen`도 실패하고 fix가 `maago makemigrations`를 말한다
- M1의 `internal/build` 종단 테스트(gen → check 통과)는 DB가 필요해지므로 `cmd/maago` 모듈로 옮긴다. 루트에는 DB 전 단계의 오류와 D-64 진단만 남긴다

→ 확인
1. 루트 모듈: 드라이버 없는 `Check`가 D-64 진단(위치 없음, fix 있음)으로 실패하고 아무것도 쓰지 않는다
2. `cmd/maago` 모듈, 두 방언: 모델과 맞는 마이그레이션 → `check` 0. 그 뒤 초안을 한 곳씩 손으로 바꾼 표(완료 조건 2): 컬럼 삭제, 타입, NOT NULL, 기본값, CHECK 완화(`max` 삭제, 선택지 하나 더), FK 삭제·`ON DELETE CASCADE`, unique 삭제, index 컬럼 순서, `AUTOINCREMENT`·IDENTITY 삭제. 각각 `check` 1이고 문구가 바뀐 곳을 말한다
3. 표시(완료 조건 3): 표시 없는 `DROP` → 그 문장 위치의 진단. 표시가 다른 문장에 붙은 경우, `DROP` 둘 중 하나만 표시한 경우. SQLite 재생성의 `DROP TABLE`은 표시 없이 통과하지만, 옮기는 `SELECT`에서 컬럼 하나를 빼면 진단(D-93)
4. 문법 오류 문장은 그 줄을 짚는다. 번호 빈틈은 파일 위치의 진단
5. 두 브랜치가 각자 만든 `0003`이 함께 있으면 두 파일 위치의 진단, fix는 나중에 합친 쪽을 다음 번호로 바꾸고 `maago check`를 다시 돌리라는 것. 바꾼 뒤 두 파일을 차례로 적용한 결과는 기존 비교가 확인한다(Alembic의 head가 둘인 경우. merge 리비전 같은 개념은 두지 않는다)

## 9. `makemigrations`

- 8번의 단계를 돌고, 차이가 있으면 다음 번호의 초안을 쓴다. 차이가 없으면 `no changes`, 종료 0
- 이름(D-94): 첫 파일은 `0001_initial`. 그 뒤는 변경에서 짓는다: 변경 하나면 `add_blog_post_body`·`drop_blog_post_summary`·`create_blog_comment`처럼, 여럿이면 한 테이블이면 그 테이블 이름, 여러 테이블이면 계획에서. `--name`이 이긴다(`[a-z0-9_]+`만). 기존 마이그레이션이 적용되지 않으면(8번의 적용 오류) 초안을 쓰지 않고 1
- 이름 변경(D-36): 같은 테이블에서 컬럼 하나가 사라지고 하나가 생기면 초안에 ⚠와 대안 `ALTER TABLE … RENAME COLUMN …`을 주석으로. `--rename 테이블.옛이름:새이름`(DB 이름, TUTORIAL §4)을 주면 삭제+추가 대신 `RENAME COLUMN`. `--rename`이 가리키는 컬럼이 차이에 없으면 진단
- 쓰기 직전 초안을 연습용 DB에 한 번 더 적용해 `Diff`가 비는지 본다. 비지 않으면 maago의 버그이므로 쓰지 않고 실패한다
- `migrations/`는 생성 파일이 아니다(ARCHITECTURE §5). 표시(`pipeline.Marker`)를 달지 않고, `pipeline.Write`의 지우기 대상도 아니다. 기존 파일은 덮어쓰지 않는다
- 하지 않는 것: 테이블(모델) 이름 변경의 `--rename`(v0은 컬럼만, 테이블은 삭제+추가로 보이고 allow-drop이 막는다 → TODO), 대화형 질문(D-36)

→ 확인 (`cmd/maago` 모듈, 두 방언): 빈 프로젝트에 모델 → `0001_initial`, 필드 추가 → `0002`, 이름 변경은 ⚠ 초안이 나오고 `--rename`으로 다시 만들면 `RENAME COLUMN`, 필드 삭제는 표시 없는 `DROP`과 ⚠. 각 단계 뒤 `check`는 (표시를 달아야 하는 삭제만 빼고) 0. 초안 골든

## 10. `showmigrations`·`migrate`

- 실제 DB(`MAAGO_DATABASE_URL`, SQLite 경로는 프로젝트 루트 기준)에 `maago_migrations`(이름, 적용 시각) 표. 없으면 `migrate`가 만든다
- `migrate`: 적용되지 않은 파일을 번호 순으로, 파일마다 트랜잭션 하나. 그 파일의 문장과 `maago_migrations` 기록 삽입이 같은 트랜잭션이다. 실패하면 그 파일(기록 포함)을 되돌리고 멈춘다(앞 파일은 남는다), 종료 1과 `파일:줄`
- SQLite의 `PRAGMA foreign_keys`는 트랜잭션 안에서 바뀌지 않는다. 재생성이 있는 파일은 같은 연결에서 `foreign_keys = OFF` → `BEGIN` → 문장들 → `PRAGMA foreign_key_check`(행이 나오면 롤백) → `COMMIT` → `foreign_keys = ON`. 실패해도 ON으로 돌린다. 연습용 DB의 적용(8번)도 같은 순서
- 적용 기록에 있는데 파일이 없거나, 기록보다 앞 번호의 파일이 적용되지 않았으면 진단. 기록의 이름과 다르지만 해시가 같은 파일이 있으면 번호만 바뀐 것(브랜치를 합치며 다시 매긴 경우)으로 보고, fix에 기록의 이름을 고치는 `UPDATE maago_migrations …` 문장을 적는다
- `maago_migrations`에 파일 내용의 SHA-256을 함께 기록한다. 적용 뒤 바뀐 파일은 `migrate`·`showmigrations`의 진단(파일 위치, fix: 되돌리거나 새 마이그레이션으로)이고, `migrate`는 아무것도 적용하지 않는다(D-95)
- `showmigrations`: 파일마다 `[x]`/`[ ]`, 마지막 줄에 적용된 마지막 번호와 파일의 마지막 번호. DB는 읽기만
- 하지 않는 것: 롤백(D-39), `--fake`, 특정 번호까지만 적용

→ 확인 (`cmd/maago` 모듈, 두 방언): 적용한 파일의 주석 한 글자를 바꾸면 두 명령 모두 1. 적용한 `0003_add_y`를 `0004_add_y`로 바꾸면 진단의 fix에 기록을 고치는 문장이 있고, 그것을 돌리면 통과. 빈 DB에 `migrate` → 모두 적용, `showmigrations` 전부 `[x]`, 다시 `migrate`는 아무것도 하지 않고 0. 중간 파일의 마지막 문장이 실패하면 그 파일의 앞 문장(DDL·데이터)과 기록이 함께 되돌려지고 앞 파일만 남는다. 기존 행이 있는 SQLite 테이블의 재생성 뒤 행과 FK가 그대로이고, FK를 깨는 재생성은 롤백되며, 어느 경우든 끝난 뒤 `foreign_keys`가 ON

## 11. CLI와 CI

- `makemigrations`(`--name`, `--rename` 반복 가능), `showmigrations`, `migrate`를 명령 표에 더한다. `flag` 디스패치, 종료 코드 D-68
- `cmd/maago`에 드라이버 blank import(D-89), `go mod tidy`
- CI: PostgreSQL 서비스와 `MAAGO_TEST_PG_URL`(D-91). 로컬에서 건너뛴 PG 테스트는 `-v`에 이유가 보인다
- M1의 스모크 스크립트(`internal/dialect/*/testdata/smoke.sh`)의 경우들을 `cmd/maago`의 Go 테스트로 옮기고 스크립트를 지울지 계획에서 정한다(M1에서 넘어옴)

→ 확인: `cmd/maago`의 `go mod tidy -diff`, 두 모듈 `go vet`·`go test`, CI에서 PG 테스트가 실제로 돈다(건너뜀 0, CI에서 변수를 빼면 실패). M2의 완료 확인은 PG를 건너뛴 로컬 결과로 대신하지 않는다

## 12. 완료 조건 종단 테스트와 문서

- 완료 조건 세 개를 그대로 `cmd/maago` 모듈의 테스트로: 빈 임시 프로젝트에서 모델 추가 → 필드 추가 → 필드 이름 변경(`--rename`) → 필드 삭제(표시를 단다), 단계마다 `makemigrations` → `check` 0, 끝에 새 DB에 `migrate`한 결과를 읽어 모델 DDL을 적용해 읽은 것과 같다. 두 방언
- ARCHITECTURE(§2 `schema`·`practicedb`·`migrate`의 실제 모양, §3 DB 검사 단계), TUTORIAL-FUTURE §4(초안 모양, 가정 목록 4 지우기), TODO(`PRAGMA` 항목 닫기, 미룬 것 더하기) 갱신

→ 확인: 위 테스트가 두 방언에서 통과하고, 단계 하나를 빼거나(`--rename` 없이, 표시 없이, 초안 한 줄을 손으로 바꿔) 돌리면 해당 완료 조건이 실패한다

---

## M2 끝

- [ ] 1~12 완료
- [ ] 위 M2 완료 조건 세 개를 실제로 돌려 확인
- [ ] MILESTONES.md의 M2 체크박스 갱신

## 열린 질문 (계획 단계에서 정하거나 사람에게 묻는다)

- 동시에 도는 두 실행의 연습용 스키마를 D-96의 "남은 것"과 구분하는 법(2번)
- 재생성으로 사라지는 컬럼을 알아내는 구체적인 방법(7·8번, D-93)
- 속도(D-65): 매번 연습용 DB 둘에 전체 마이그레이션을 적용한다. 보통 규모에서 PG 3초를 넘기는지 M2 끝에 한 번 잰다

## 초안 리뷰 (2026-09-26)

codex: 16건 모두 반영. 연결 하나(`*sql.Conn`)에 고정, `PRAGMA foreign_keys`의 트랜잭션 경계, 재생성 `DROP`의 판정 규칙 제안(7번), PK의 자동 생성 방식을 표현에, SQLite 카탈로그의 `origin`·`hidden`·식 인덱스·여러 컬럼 FK, `CREATE TABLE`을 최상위 쉼표로 나눠 CHECK를 제자리에, 달러 인용 태그, PG 조회의 스키마 조건과 FK를 카탈로그 열로, `makemigrations`는 차이를 허용하는 모드, 적용 함수를 2번으로 당겨 7번이 8번을 기다리지 않게, 표시는 다음 문장 하나, 기록 삽입도 같은 트랜잭션, 읽기 자체 테스트, 남은 PG 스키마(열린 질문), 로컬 skip을 완료 증거로 쓰지 않음, 손으로 바꾼 초안의 돌연변이 표(8번)

사람의 결정(2026-09-26, 초안 리뷰 뒤): allow-drop은 다음 문장 하나·모든 파일(D-37 고침), 재생성 `DROP`은 파일 모양으로(D-93), 이름은 변경에서 자동·`--name`(D-94), 내용 해시(D-95), 남은 PG 스키마는 오류로 알림(D-96)
Alembic의 head·base(2026-09-27): 번호가 한 줄로 이어져 head는 마지막 파일, base는 빈 DB라 따로 두지 않는다. head가 둘인 경우(브랜치마다 만든 같은 번호)는 8번의 번호 겹침 진단과 10번의 번호 바뀜 fix로, 마지막 번호는 `showmigrations`로 본다
