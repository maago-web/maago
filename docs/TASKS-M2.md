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
- PG 연습용 스키마는 트랜잭션 안에서 만들고 롤백해 남지 않게 한다(D-96, 2026-09-27 고침)

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

**계획**
- 새 파일: `internal/project/database.go`, `database_test.go`. 다른 패키지는 건드리지 않는다. 결정에 없는 선택은 완료 때 D-97로
- `type Dialect string`, 상수 `SQLite = "sqlite"`, `Postgres = "postgres"`. `project`에 둔다: `dialect`는 `model`을 import하고 `model`의 테스트(`package model`)가 `project`를 import하므로 `project → dialect`는 테스트 빌드에서 순환이다. ARCHITECTURE §2의 "다른 패키지는 방언을 모른다"는 SQL의 차이를 말한다. `project`는 스킴으로 방언의 **이름**만 알고 차이는 모른다(12번 ARCHITECTURE 갱신에 `project/` 줄을 더한다). 결정에 없는 선택
- `Database(root string, lookupEnv func(string) (string, bool)) (Dialect, string, []diag.Diagnostic)`. 진단이 하나라도 있으면 `"", ""`를 돌려준다(반쪽 값을 쓰지 않게, 10번은 파이프라인 밖에서도 부른다). `lookupEnv`는 `os.LookupEnv` 모양: 빈 환경 변수와 없는 것을 가른다
  1. `root/.env`를 읽는다. 없으면 빈 파일과 같다. 그 밖의 읽기 오류는 위치 없는 진단(`Root`와 같은 모양)
  2. 줄마다(`\r`은 벗긴다) 앞뒤 공백을 벗기고: 빈 줄과 `#`로 시작하는 줄은 넘긴다. 나머지는 첫 `=`로 나눠 키와 값의 앞뒤 공백을 벗긴다(`KEY = v`도 받는다)
  3. 형식 오류는 모두 `.env:줄`의 진단이고 fix는 경우마다: `=` 없음(`write KEY=VALUE`), `export `로 시작(`remove "export"`), 키가 `[A-Za-z_][A-Za-z0-9_]*`가 아님, 짝 없는 따옴표(`close the quote`), 따옴표 없는 값에 공백(`quote the value or remove the trailing comment`: `sqlite:app.db # dev`가 조용히 경로가 되지 않게. URL에는 공백이 없다)
  4. 값이 `"`나 `'`로 시작하면 같은 따옴표로 끝나야 하고(길이 2 이상), 그 한 쌍만 벗긴다. 이스케이프는 해석하지 않는다. 따옴표 없는 값의 `#`은 값의 일부다(PG 비밀번호에 쓰일 수 있다, 줄 뒤 주석은 없다)
  5. 같은 키가 두 번이면 두 번째 줄의 진단, 키가 무엇이든. §6의 "모르는 형식"보다 엄격하지만 어느 쪽이 이기는지 추측하지 않는다(PHILOSOPHY 6). 결정에 없는 선택
  6. `.env`의 줄 진단은 환경 변수가 이길 때도 모두 낸다(D-61, 조용히 넘기지 않는다)
  7. 환경 변수가 **있으면**(빈 값이어도) 그것, 아니면 `.env`의 값. 빈 값은 그 출처의 진단(`.env:줄: MAAGO_DATABASE_URL is empty` 또는 위치 없는 `environment variable MAAGO_DATABASE_URL is empty`). 둘 다 없으면 위치 없는 `MAAGO_DATABASE_URL is not set`, fix `add MAAGO_DATABASE_URL=sqlite:app.db or MAAGO_DATABASE_URL=postgres://localhost:5432/myapp to .env`. 단 깨진 줄이 그 키였을 수 있으면(키를 읽지 못했거나 `MAAGO_DATABASE_URL`) "not set"은 내지 않는다(D-61 연쇄). 관계없는 키의 깨진 줄은 막지 않는다(한 번에 둘 다 보인다)
  8. 방언: `sqlite:` 뒤에 무언가 있으면 SQLite(경로는 해석하지 않는다. 10번이 루트 기준으로 쓴다), `postgres://`·`postgresql://`는 Postgres. 그 밖(`sqlite:`만, `sqlite://`, 모르는 스킴, 스킴 없음)은 진단, fix에 두 형식. 스킴은 소문자만 받는다(결정에 없는 선택, 규칙 하나)
- 진단의 위치: 값이 `.env`에서 왔으면 `.env:줄`, 환경 변수에서 왔으면 위치 없음이고 메시지에 `environment variable MAAGO_DATABASE_URL`
- **진단에 주소도 줄 내용도 싣지 않는다.** PG 주소에 비밀번호가 있다. 모르는 스킴은 첫 `:` 앞이 스킴 모양(RFC 3986)일 때만 보이고, 아니면 `no scheme`이라고만 한다. 잘못된 키는 싣지 않는다(키를 빼먹고 붙여 넣은 주소일 수 있다). 올바른 키 이름은 싣는다
- 하지 않는 것: 파이프라인 연결(8번), `export`·줄 뒤 주석·이스케이프·여러 줄 값·변수 치환, 경로 해석
- 리뷰: codex(같은 키일 때 값, 연쇄 "not set", `Dialect` 자리와 §2, 비밀 노출 테스트, 환경 변수 행 나누기, 8번의 통합 확인), fable(연쇄 "not set", 빈 값의 위치와 빈 환경 변수, 따옴표 없는 값의 ` # 주석`, 빠진 행, 표 전체의 비밀 단언, 반환값, 결정에 없는 선택 표시, fix를 경우마다) 모두 반영. `Dialect`를 별도 잎 패키지로(codex)는 받지 않는다: 상수 둘에 패키지 하나는 과하고, fable의 "이름만 안다"로 §2와 맞춘다. 구현 리뷰: ponytail(키 검사는 `regexp`, 표와 겹치는 연쇄 테스트 삭제), codex(BOM → 벗긴다. 샌드박스로 테스트는 못 돌림), fable(잘못된 키를 인용해 비밀번호가 샘 → 싣지 않음, 닫힌 따옴표 뒤 주석을 "unclosed quote"로 내어 fix대로 고치면 통과 → 따로 진단, 스킴 모양이 아닌 `:` 앞도 찍힘 → `no scheme`, fix 문구를 글자로 고정·진단 개수 단언, `sqlite://`가 통과 → 진단, 빈 환경 변수의 fix) 모두 반영. 새 분기마다 돌연변이를 넣어 테스트가 잡는 것을 확인. 커밋 뒤 advisor: 모든 줄 오류가 "not set"을 막아 왕복이 하나 는다 → 그 키였을 수 있는 줄만

→ 확인 (`go test -count=1 ./internal/project`): 표 테스트 한 벌, 행마다 `.env` 내용(또는 없음), 환경 변수(없음·빈 값·값), 기대한 방언·주소, 또는 기대한 진단 전문(`.env:줄: …`과 fix). 행: 주석·빈 줄·CRLF, 두 따옴표, 짝 없는 따옴표, `#`이 든 값, ` # 주석`이 붙은 값, `KEY = v`, `export`, `=` 없는 줄, 잘못된 키, 같은 키 두 번(다른 키도), `export MAAGO_DATABASE_URL=…` 한 줄만(진단 정확히 하나), 모르는 스킴·스킴 없음·`sqlite:`만, `postgresql://`, 환경 변수 정상 + `.env` 모르는 스킴(환경 변수가 이긴다), 환경 변수가 모르는 스킴 + `.env` 정상(위치 없음, `environment variable` 문구), 환경 변수 정상이어도 `.env`의 줄 오류는 나온다, `.env` 빈 값(`.env:줄`), 빈 환경 변수 + `.env` 정상(빈 값 진단, `.env`로 떨어지지 않는다), `.env` 없음 ± 환경 변수. 표 전체에 걸쳐: 비밀번호 `s3cret`를 넣은 주소가 어느 행의 출력에도 없다, 진단이 있는 행은 `"", ""`. 읽기 오류는 `.env`를 디렉터리로 만들어 확인

## 2. 연습용 DB (`internal/practicedb`)

- `Open(ctx, dialect, url) (*DB, []diag)`와 `Close`. 드라이버 이름(`sqlite`, `pgx`)으로 `sql.Open`하고, 드라이버는 `cmd/maago`가 등록한다(D-90). `DB`는 `*sql.Conn` 하나를 쥐고, 모든 일(PRAGMA, 스키마 생성, 적용, 읽기)이 그 연결에서 일어난다
- `Exec(ctx, pos, sql)`: 문장 하나를 실행하고, 실패하면 그 문장의 위치로 진단. 7·8·9번이 3번이 나눈 문장마다 이것으로 적용한다
- SQLite: `:memory:`. 연결을 쥔 직후 `PRAGMA foreign_keys = ON`을 켜고 다시 읽어 켜졌는지 확인한다(TODO에서 넘어옴). `.env`의 파일 이름은 쓰지 않는다(ARCHITECTURE §6)
- PG: 같은 연결에서 `BEGIN` → `CREATE SCHEMA maago_practice_<무작위>` → `SET LOCAL search_path`를 그 스키마로. `Close`는 `ROLLBACK`. 스키마는 커밋되지 않으므로 다른 세션에 보이지 않고, 비정상 종료·연결 끊김도 롤백이라 남는 것이 없다(D-96). 마이그레이션과 DDL은 스키마 이름 없이 쓰이므로 그대로 적용된다(D-35). 이름이 무작위인 것은 같은 이름의 `CREATE SCHEMA`가 다른 트랜잭션의 커밋을 기다리며 막히기 때문(한 실행이 A와 B를 함께 연다)
- 연결·스키마 생성 실패는 위치 없는 진단, fix에 켜는 법(D-64): PG 서버를 켜라, 주소를 확인하라, 권한(`CREATE` on database)
- 드라이버가 등록되지 않은 경우(루트 모듈 테스트)도 위치 없는 진단으로 실패한다. 조용히 건너뛰지 않는다

→ 확인
1. 루트 모듈: 드라이버 없이 `Open`이 진단으로 실패한다
2. `cmd/maago` 모듈(D-90): SQLite 연습용 DB에서 두 번 연 것이 서로 보이지 않고, FK가 강제된다. PG에서 연습용 스키마가 다른 세션에 보이지 않고 `Close` 뒤에도 남지 않으며, 사용자 스키마(`public`)의 테이블은 보이지도 바뀌지도 않는다. 닫힌 포트의 주소는 D-64 진단

**계획**
- 새 파일: `internal/practicedb/practicedb.go`, `practicedb_test.go`(루트, 드라이버 없음), `cmd/maago/practicedb_test.go`(실제 DB). 바꿀 파일: `cmd/maago/main.go`(드라이버 blank import, 11번에서 당김: `cmd/maago`의 테스트가 드라이버를 필요로 한다), `cmd/maago/go.mod`·`go.sum`(D-89의 두 드라이버, 최신 `modernc.org/sqlite` v1.59.0·`pgx/v5` v5.11.0), `.github/workflows/ci.yml`(PostgreSQL 18 서비스, health check, `MAAGO_TEST_PG_URL=postgres://postgres:postgres@localhost:5432/postgres`. 11번에서 당김: 아래 테스트가 CI에서 변수가 없으면 실패하므로). 새 외부 의존은 D-89(사람의 결정, 2026-09-26)가 정한 둘뿐. 작업 본문(2번 `Exec`, 11번)도 이에 맞춰 고쳤다
- `Open(ctx, d project.Dialect, url string) (*DB, []diag.Diagnostic)`: 드라이버가 `sql.Drivers()`에 없으면 먼저 진단(아래). `sql.Open` → `db.Conn(ctx)`로 연결 하나를 쥔다. `DB`는 `*sql.DB`와 `*sql.Conn`. 실패하면 연 것을 모두 닫고 `nil`
  - SQLite: `sql.Open("sqlite", ":memory:")`, url은 쓰지 않는다. `PRAGMA foreign_keys = ON` 뒤 `PRAGMA foreign_keys`를 읽어 1이 아니면 진단
  - PG: `sql.Open("pgx", url)`. 같은 연결에서 `BEGIN`, `CREATE SCHEMA "maago_practice_" + 소문자 rand.Text()`(base32 26자), `SET LOCAL search_path TO "이름"`(`public`을 넣지 않는다: 넣으면 마이그레이션이 만들지 않은 사용자 테이블이 보여 틀린 마이그레이션이 통과한다. 대가: `public`에 설치된 확장의 타입·함수(`citext`, `uuid_generate_v4()`)를 이름만으로 쓰는 마이그레이션은 연습용 DB에서 실패한다 → TODO), `SELECT current_schema()`가 그 이름인지 다시 읽어 확인(SQLite의 PRAGMA 다시 읽기와 같은 이유). `BEGIN`은 `database/sql`의 `Tx`가 아니라 문장으로: `Exec`·`Close`가 방언과 상관없이 `*sql.Conn` 하나만 쓴다
  - 알 수 없는 방언은 호출자의 버그이므로 panic(`diag.New`와 같은 관례. 결정에 없는 선택)
- `Exec(ctx, pos token.Position, sql string) []diag.Diagnostic`: 문장 **하나**를 쥔 연결에서 실행, 실패하면 `pos`에 DB 메시지(`diag.OneLine`), fix `fix the SQL statement`. 작업 본문의 처음 모양 `Exec(stmts)`와 다른 선택: 3번의 문장 타입을 기다리지 않고, 문장을 나누는 일과 실행을 섞지 않는다. 차례로 돌며 첫 실패에서 멈추는 것은 부르는 쪽의 반복문. PG에서는 실패한 문장 뒤 트랜잭션이 aborted가 되어 다음 문장도 실패하므로 첫 실패에서 멈추는 것이 맞다. PG가 주는 칸 위치는 8번
- `Close()`: 연결과 `*sql.DB`를 닫는다. PG의 커밋되지 않은 트랜잭션은 연결과 함께 롤백되므로 `ROLLBACK`을 따로 보내지 않는다(멈춘 서버에 막히지 않게, fable). 돌려주는 것이 없다
- 진단(모두 위치 없음)
  - 드라이버 없음(두 방언 공통): `no "pgx" database driver in this maago build`, fix `build maago from its cmd/maago module, which registers the drivers`. `sql.Open` 전에 가른다(서버 fix가 붙지 않게, fable)
  - PG 연결·`BEGIN`·스키마 만들기·`search_path` 실패(D-64): `cannot open the practice database: <오류>`, fix `start the PostgreSQL server, check MAAGO_DATABASE_URL, and make sure its user may create schemas (GRANT CREATE ON DATABASE)`. pgx의 연결 오류는 host·user·database만 싣고 비밀번호는 싣지 않는다(테스트가 확인)
  - SQLite 연결 실패, `foreign_keys`가 켜지지 않음: 드라이버의 문제이므로 `…`, fix `report this as a maago bug`
- **D-96 고침**(사람의 결정, 2026-09-27, fable 제안): 남은 스키마를 찾아 알리던 것을, 트랜잭션으로 남지 않게 한다. advisory lock·남은 스키마 찾기·그 진단이 필요 없다. 대가: 마이그레이션 파일 안의 트랜잭션 제어문(`BEGIN`·`COMMIT`·`ROLLBACK`)과 `CREATE INDEX CONCURRENTLY`는 연습용 DB에서 쓸 수 없다. 10번의 "파일마다 트랜잭션 하나"와도 어차피 맞지 않으므로 8번이 진단한다(8번 본문에 더함). `COMMIT`이 새면 연습용 스키마가 커밋되어 남는다. M3에서 쿼리 여럿을 prepare하며 실패를 모두 모으려면 쿼리마다 `SAVEPOINT`가 필요하다(M3 계획에서)
- `cmd/maago` 테스트의 PG 쪽: `MAAGO_TEST_PG_URL`이 없으면 `CI`가 있을 때 `t.Fatal`, 아니면 이유를 찍고 `t.Skip`(D-91). 로컬 확인은 임시 클러스터(`127.0.0.1`의 빈 포트)를 띄워 변수를 주고 돌리고, 한 번은 `CI=1`에 변수 없이 돌려 실패하는 것까지 본다
- 하지 않는 것: 문장 나누기(3번), 스키마 읽기(4·5번), 읽기용 조회 API(4번에서 필요한 모양으로), `foreign_keys` OFF/ON 순서(10번), PG 오류의 칸 위치(8번), 연결 시간 제한(ctx로 부르는 쪽이), SQLSTATE별 fix(메시지에 DB 오류를 그대로 실으므로 v0은 fix 하나)
- 리뷰: codex(`hashtext` 충돌, `Close` 뒤 잠금 해제 확인, `Open` 중간 실패의 스키마, `Close`와 열린 트랜잭션, `LIKE`의 `_`, 찾기→잠그기의 원자성, 사용자 SQL의 `search_path`, fix 속 이름 인용, CI 서비스 모양, 연결 하나 계약) — 대부분은 트랜잭션 방식으로 문제 자체가 사라졌다. 남는 것: 사용자 SQL의 `SET search_path`·트랜잭션 제어문은 8번으로, CI 모양은 위에 적음. fable(트랜잭션으로 감싸기 → 사람에게 물어 채택, 드라이버 없음에 서버 fix → `sql.Drivers()`로 먼저 가름, "fix 있음"은 늘 참 → fix 전문 단언, PRAGMA 실패의 fix, 한 프로세스에서 A·B 동시 열기, 스키마 이름이 붙은 문장이 개발 DB를 바꿈 → 롤백으로 무해, 확인에 넣음, `hashtext` 비공개 함수 → 없어짐, §2 문장은 12번) 모두 반영. 구현 리뷰: ponytail(지울 것 없음), codex(비밀번호 숨김이 pgx에만 기댐 → 잘못된 주소 6가지로 pgx가 가리는 것을 확인하고, 주소의 비밀번호를 메시지에서 한 번 더 지움, 잘못된 주소 행 추가. 샌드박스로 테스트는 못 돌림), fable(테스트가 개발 DB에 `public` 테이블을 남김 → Cleanup 순서, `public` 없는 `search_path`의 대가 → 계획·TODO, 파일 이름 단언 → 빈 디렉터리, `ROLLBACK`은 가르지 못하고 멈출 수 있음 → 빼고 연결 종료에 맡김, 스키마 붙은 이름의 잠금·첫 실패에서 멈춤 → 8번, 문서 잔재) 모두 반영. 돌연변이 8개 중 다시 읽기 검사만 살아남는다(정상 드라이버로는 재현 불가, 무시된 설정을 잡는 방어)

→ 확인
1. 루트 `go test ./internal/practicedb`: 두 방언 모두 드라이버 없이 `Open`이 위치 없는 진단 하나(메시지·fix 전문 단언)와 `nil`. 비밀번호가 든 주소를 줘도 출력에 없다
2. `cmd/maago` SQLite: 둘을 열어 한쪽에 만든 테이블이 다른 쪽의 `Exec`에서 없다고 실패한다. FK를 어기는 `INSERT`가 `Exec`의 진단이고 위치가 준 `pos` 그대로
3. `cmd/maago` PG("`Close` 뒤 스키마 없음"은 트랜잭션 방식에서 늘 참인 문서 성격의 단언이고, 실제로 잡는 것은 "다른 연결에 안 보임"과 `public`의 흔적): A와 B를 함께 열어 둘 다 `t`를 만들 수 있다(서로 따로, 막히지 않는다). 열려 있는 동안 다른 연결의 `pg_namespace`에 `maago_practice_`가 없다. 미리 만든 `public.t`(행 하나)는 연습용 DB에서 `SELECT * FROM t`로 보이지 않고, 연습용 DB에서 `CREATE TABLE public.t2`·`DELETE FROM public.t`를 해도 `Close` 뒤 `public.t2`는 없고 `public.t`의 행은 그대로. `Close` 뒤 `maago_practice_` 스키마가 없다. 닫힌 포트(`net.Listen("tcp", "127.0.0.1:0")`으로 번호를 얻고 닫는다)의 주소는 D-64 진단(메시지 앞부분·fix 전문)이고 비밀번호가 없다

## 3. SQL 토큰화와 문장 나누기 (`internal/dialect`)

- 두 곳에 쓴다: 마이그레이션 파일을 문장으로 나누기(2·8번, DB 오류를 문장의 `파일:줄`로 짚고, 8번이 문장마다 `DROP`·트랜잭션 제어문을 본다), SQLite `CREATE TABLE` 원문의 구조 읽기(4번, D-92)
- 문자열(`'…'`, `''`), 인용 식별자(`"…"`), 주석(`--`, `/* */`), PG의 `$tag$…$tag$`(여는 태그와 같은 태그에서만 닫힌다)를 안다. 줄·칸 위치를 가진다
- `-- maago: allow-drop`(D-37)은 **바로 다음 문장 하나**에 붙는다. 한 파일의 여러 `DROP`은 하나씩 허락한다. 모르는 `-- maago:` 표시와 문장이 따라오지 않는 표시는 진단(닫힌 목록, D-17과 같은 원칙)
- 정규화: 공백을 하나로, 키워드는 대문자, 인용 식별자와 문자열은 그대로. 비교에만 쓴다

→ 확인: 표 테스트. 문장마다 내용과 시작 줄·칸을 본다(세미콜론이 든 문자열·주석, `$$`·`$fn$` 본문, 서로 다른 태그가 겹친 본문, 닫히지 않은 문자열·주석·달러 인용의 위치 있는 진단, 빈 문장, 마지막 세미콜론 없음, CRLF). 표시가 다음 문장에만 붙는지

**계획**
- 새 파일: `internal/dialect/split.go`, `split_test.go`, `testdata/split.golden`. 다른 코드는 건드리지 않는다
- **원칙**: 토큰화가 DB와 달라도 DB보다 **적게 보지는 않는다**. maago가 문자열·주석으로 보는 곳을 DB가 SQL로 읽으면 그 안의 `DROP`·`COMMIT`이 8번의 검사를 피한다. 반대 방향(maago가 SQL로 보는 곳이 DB에는 문자열)은 문장이 잘못 나뉘어 DB 오류나 과한 진단이 될 뿐 조용히 통과하지 않는다. 그래서 문자열을 여는 규칙은 두 방언의 합집합이되 한쪽에만 있는 것(`` ` ``·`E'`·`$$`)은 다른 쪽 DB에서 경계 조각이 오류가 나는 것만, 닫는 규칙은 DB와 같게. PG의 `standard_conforming_strings = off`(보통 `'…'` 안의 `\`가 이스케이프)는 연습용 DB가 `SET LOCAL … = on`으로 막는다(2번 코드, 3번 구현 리뷰 codex)
- `Split(filename, src string) ([]Stmt, []diag.Diagnostic)`. `Stmt{Pos token.Position; SQL string; Tokens []Token; AllowDrop token.Position}`. `SQL`은 첫 토큰부터 마지막 토큰까지의 원문(안의 주석 포함, 끝 `;`와 그 앞 주석 제외): DB에 그대로 보낸다. `Pos`는 첫 토큰의 줄·칸(바이트 칸, Go와 같이)과 `Offset`(8번이 PG 오류의 글자 위치를 파일 위치로 옮길 때). `AllowDrop`은 붙은 표시의 `--` 위치, 없으면 영값(8번이 표시 줄을 짚을 수 있게). 4번은 `CREATE TABLE` 원문을 `Split("", 원문)`해 한 문장의 `Tokens`를 쓰고, 8번은 `Tokens`로 `DROP`·트랜잭션 제어문을 알아본다
- `Token{Kind; Text string; Pos token.Position}`. 종류 넷: `Word`(따옴표 없는 이름·키워드·숫자 조각: 글자(`unicode.IsLetter`)·숫자·`_`, 둘째 글자부터 `$`), `Quoted`(`"…"`와 `` `…` ``, 안의 두 겹 따옴표), `Literal`(`'…'`와 안의 `''`, `E'…'`, 달러 인용), `Punct`(그 밖의 글자 하나씩: `::`는 `:` 둘. SQLite의 `[이름]`도 여기: PG에서는 배열 첨자이고, `Punct`로 보는 쪽이 위 원칙의 안전한 방향). 주석은 토큰이 아니다. `Text`는 원문 그대로(따옴표 포함)
- `E'…'`(PG, 앞이 낱말이 아닌 `E`·`e` 바로 뒤 `'`): 안의 `\`가 다음 글자를 가린다. 이것을 모르면 `E'\''` 뒤를 통째로 문자열로 읽어 위 원칙을 어긴다(codex·fable). SQLite에서는 문법 오류이므로 DB가 낸다
- 달러 인용: 낱말 안이 아닌 `$` 뒤에 태그(글자·`_`로 시작, 뒤는 글자·숫자·`_`, 없어도 됨)와 `$`가 오면 여는 태그. 같은 태그가 나올 때까지가 본문. `$1`(숫자)은 `Punct` `$`와 `Word` `1`
- 주석: `--`는 줄 끝까지(`\r\n`의 `\r`은 공백), `/* */`는 겹치지 않는다. PG의 겹친 주석은 잘못 토큰화되지만 maago가 주석을 일찍 닫는 쪽이라 위 원칙의 안전한 방향(DB 오류나 과한 진단)
- 닫히지 않은 문자열·인용 식별자·주석·달러 인용: 여는 곳의 위치에 진단(`unterminated string`, fix `close it with '`처럼 닫는 글자). 진단이 있으면 문장을 돌려주지 않는다(뒤가 모두 그 안이다). 표시 진단만 있으면 문장은 돌려준다
- 문장: 최상위 `;`로 나눈다. 토큰이 없는 문장(`;;`, 주석만)은 버린다. 마지막 `;`가 없어도 된다
- 표시(D-37): `--` 뒤 공백을 벗긴 글이 `maago:`(소문자)로 시작하는 줄 주석. `maago:` 뒤 공백을 벗긴 글이 `allow-drop`이어야 한다(`--maago:allow-drop`도 같다. 결정에 없는 선택: 정본 하나를 공백만 무시하고 맞춘다. fix에는 정본만). `/* maago: … */`는 표시가 아니라 주석이다(표시는 한 줄 주석 하나로만, 규칙을 늘리지 않는다). 위치는 `--`. 한 표시에 진단은 하나, 아래 순서로 처음 걸린 것
  1. 모르는 표시: `unknown maago marker "…"`, fix `the only marker is -- maago: allow-drop`(닫힌 목록, D-17)
  2. 같은 줄 앞에 토큰이 있다(`DROP TABLE x; -- maago: allow-drop`): `-- maago: allow-drop must be on its own line`, fix `move it to its own line right before the DROP statement`. 앞 문장에 붙는 것처럼 읽히지만 다음 문장에 붙는 자리를 막는다(PHILOSOPHY 5)
  3. 앞 줄들에서 연 문장이 `;`로 닫히지 않았다(대개 앞 문장의 `;`를 빠뜨린 것): `-- maago: allow-drop is inside the statement starting at line N`, fix `end the statement starting at line N with ;`(fable: 이때 "혼자 있어야" 문구는 고칠 수 없는 조언)
  4. 다음 줄이 문장의 첫 토큰으로 시작하지 않는다(빈 줄, 주석, 다른 표시, 파일 끝): `-- maago: allow-drop is not on the line right before a statement`, fix `put it on the line right before the DROP statement`. D-37의 "바로 앞"을 글자 그대로: 표시와 문장 사이에 아무것도 두지 않는다. 겹친 표시도 여기로(규칙 하나 적게, fable)
  - 붙은 문장이 `DROP`인지는 보지 않는다. 8번이 `Tokens`로 보고, `DROP`이 아닌 문장에 붙은 표시도 8번이 진단한다(8번 본문에 더함)
- `Normalize(toks []Token) string`: `Word`는 대문자, 나머지는 그대로, 토큰 사이 공백 하나. `a(b)`와 `a ( b )`가 같다("공백을 하나로"보다 강하다: 4번이 CHECK 식을 비교할 때 괄호 둘레 공백이 달라도 같게). 비교에만 쓴다. 한계 둘: `"title"`과 `title`은 다르다(SQLite에서는 같은 이름 → TODO), 여러 글자 연산자(`<=`)와 띄어 쓴 것(`< =`)이 같아진다(띄어 쓴 쪽은 DB가 받지 않으므로 비교할 두 식에 함께 나오지 않는다)
- 방언 인자는 두지 않는다: 위 원칙 아래서 두 방언의 합집합으로 충분하다
- 하지 않는 것: PG의 겹친 주석·SQLite의 `[…]` 인용(위), 트리거 본문(`BEGIN … END;` 안의 `;`, 트리거는 4번이 막는다), 키워드 목록(모든 `Word`를 대문자로), 숫자 해석(`1.5e3`은 여러 토큰이지만 양쪽에 같게)
- 계획 리뷰: codex(`E'…'` → 다룸, SQLite `[…]`·`` ` `` → `` ` ``는 인용, `[`는 원칙에 따라 `Punct`, 유니코드 식별자·태그 → 다룸, `ALTER … DROP COLUMN`과 `END`·`ABORT`·`START TRANSACTION`·`RELEASE`·`RESET` → 8번 본문에, 겹친 주석 → 원칙의 안전한 방향, 연산자 정규화 → 한계로 적음, `DROP`이 아닌 문장의 표시 → 8번, `public.x` → 8번 계획에서, 숫자 → 양쪽에 같게, 실제 DB prepare 확인 → 8번의 종단 테스트). fable(표시는 "바로 앞 줄"로 엄격하게, 문장 중간 표시는 `;` 빠짐으로 진단, 빠진 fix·위치, 8번의 의무와 `AllowDrop` 위치, 표시 진단 때의 반환, `E'\''`, 골든에 토큰 열과 빠진 경우들, `Offset`, `"title"` TODO, 공백 무시 표시는 결정에 없는 선택) 모두 반영. 구현 리뷰: ponytail(지울 것 없음), codex(PG `standard_conforming_strings = off`면 `'a\';…`의 `DROP`이 숨음 → 연습용 DB가 켠다, 테스트는 연결 인자로 서버 기본값을 끈 채 확인. 표시 다음 줄의 `/* */` 뒤 문장에 표시가 붙음 → 사이에 줄바꿈만 허용. 샌드박스로 테스트는 못 돌림), fable(규칙 2·3의 순서를 가르는 경우, `PREPARE TRANSACTION`·트리거·PG 글자 위치 → 8번, `[이름]` → 4번, 원칙 문장과 "여러 문장 제한" 문구 고침, 패키지 주석) 모두 반영. 분기마다 돌연변이를 넣어 모두 잡는 것을 확인.

→ 확인 (`go test -count=1 ./internal/dialect`): 골든 파일 하나에 입력마다 문장(시작 `줄:칸`, 표시 위치, `SQL`, 토큰 열 `종류:글`)과 진단. 입력: 세미콜론이 든 문자열·인용 식별자·`` ` ``·`--`·`/* */`, `'it''s; x'`·`"a""b;"`(두 겹 따옴표가 나누기에 영향), `E'\'; x'`, `$$`·`$fn$` 본문(안에 `;`와 `$$`), `$a$` 안의 `$b$`, `$태그$`, `$1`, `a$b`, 여러 줄 문자열·`$$` 본문 **뒤** 문장의 시작 줄, 문자열·달러 본문 안의 `-- maago: allow-drop`(표시 아님), `/* maago: allow-drop */`(표시 아님), 닫히지 않은 `'`·`"`·`/*`·`$x$`·`E'\'`(위치), 빈 문장·주석만 있는 문장, 마지막 `;` 없음, `;` 앞 주석이 `SQL`에서 빠짐, CRLF(줄·칸), 한글 뒤 칸(바이트). 표시: 다음 문장에만 붙고 그다음은 아님, `DROP` 둘 중 하나만, 사이에 빈 줄, 사이에 주석, 같은 줄 뒤, 앞 문장의 `;`가 빠진 경우(표시는 줄에 혼자), 둘 겹침, 파일 끝, 모르는 표시(`allow-dorp`), 모르는 표시가 같은 줄 뒤에도 있을 때 진단 하나, `--maago:allow-drop`. `Normalize` 표 테스트: 공백·줄바꿈·주석·대소문자가 달라도 같고, 문자열·인용 식별자의 대소문자는 가른다

## 4. 스키마 표현과 SQLite 읽기 (`internal/schema`, `internal/dialect/sqlite`)

- `schema.Schema`: 테이블마다 STRICT 여부, 컬럼(이름, 타입, NOT NULL, 기본값, 순서), PK(컬럼과 자동 생성 방식: SQLite `AUTOINCREMENT`, PG IDENTITY `BY DEFAULT`), FK(컬럼 목록, 대상 테이블·컬럼, ON DELETE), unique(컬럼 집합), index(컬럼 목록), CHECK(붙은 컬럼 또는 테이블, 정규화된 식). 이름이 아니라 컬럼으로 맞춘다(D-87)
- SQLite 읽기: `sqlite_schema`, `pragma_table_xinfo`(`pk` 순서, `hidden`), `pragma_index_list`(`origin`, `partial`)·`index_xinfo`(`cid = -2`는 식), `pragma_foreign_key_list`(여러 컬럼 FK는 `seq`로 묶는다). `AUTOINCREMENT`·STRICT·CHECK는 원문을 3번의 토큰으로 읽어 컬럼 정의와 테이블 제약을 최상위 쉼표로 나눈 뒤 꺼낸다(D-92). `sqlite_sequence`와 `maago_migrations`는 뺀다
- SQLite의 `[이름]` 인용은 3번에서 `Punct`다(안전한 방향). 손으로 쓴 `[order] TEXT`의 컬럼 머리는 `[`를 다루거나 진단한다(3번 구현 리뷰 fable)
- 표현에 없는 것은 조용히 넘기지 않고 진단(PHILOSOPHY 7): 부분 인덱스, 식 인덱스, generated 컬럼, `WITHOUT ROWID`, 트리거, 뷰. v0은 마이그레이션에서 이것들을 막는다(필요해지면 표현을 넓힌다 → TODO)

→ 확인 (`cmd/maago` 모듈)
1. M1 공용 예제의 SQLite DDL을 적용해 읽은 결과가 기대한 구조와 같다(골든, 모델을 거치지 않는 자체 테스트)
2. 공백·대소문자만 다르게 손으로 쓴 같은 스키마는 같게 읽힌다
3. 컬럼 CHECK와 테이블 CHECK, 한 컬럼의 CHECK 둘, 문자열 안의 괄호·쉼표, 여러 컬럼 FK, 복합 PK가 제자리에 읽힌다
4. 위의 막는 것들이 각각 진단

**계획**
- 새 파일: `internal/schema/schema.go`(타입만, 테스트 없음), `internal/dialect/sqlite/read.go`, `cmd/maago/sqliteread_test.go`, `cmd/maago/testdata/sqlite_*.golden`. 바꿀 파일: `internal/practicedb/practicedb.go`(`QueryContext` 하나)
- `schema` 표현(5번 PG와 같이 쓴다. 비교는 같은 방언끼리만 하므로 방언의 글을 그대로 둔다)
  ```go
  type Schema struct{ Tables []*Table }  // 이름 순
  type Table struct {
      Name    string
      Strict  bool       // SQLite
      Columns []Column   // 정의 순(7번의 재생성이 쓴다). 6번은 순서를 비교하지 않는다
      PK      []string   // PK 순서
      PKAuto  string     // "AUTOINCREMENT", PG "BY DEFAULT"/"ALWAYS", 없으면 ""
      FKs     []FK       // (컬럼, 대상) 순
      Indexes []Index    // unique와 보통 인덱스 모두, (Unique, 컬럼) 순
      Checks  []Check    // (식, 컬럼) 순
  }
  type Column struct{ Name, Type string; NotNull bool; Default string } // Type 대문자, Default는 Normalize, 없으면 ""
  type Index struct{ Name string; Columns []string; Unique, Constraint bool } // Constraint: SQLite origin "u", PG 제약
  type FK struct{ Name string; Columns []string; RefTable string; RefColumns []string; OnDelete, OnUpdate string }
  type Check struct{ Name, Column, Expr string } // Column ""은 테이블 CHECK, Expr는 Normalize한 괄호 안
  ```
  - **이름은 담되 맞추는 데 쓰지 않는다**(D-87은 이름으로 맞추지 않는 것이지 버리는 것이 아니다). 7번의 `DROP INDEX "…"`·`DROP CONSTRAINT "…"`가 실제 이름을 쓴다. `Constraint`는 7번이 제약을 지우는 법(SQLite `u`는 재생성, PG는 `DROP CONSTRAINT`)을 고르는 데. 6번이 맞추는 기준: unique는 정렬한 컬럼 집합, 보통 인덱스는 컬럼 순서, FK는 컬럼·대상·동작, CHECK는 `Expr`(`Column`은 진단 문구용: PG 카탈로그는 컬럼 CHECK와 테이블 CHECK를 가르지 않는다). 6번 본문에 이것과 `Strict`·`PKAuto` 변경을 더한다
  - 컬럼 순서를 비교하지 않는 이유: `ADD COLUMN`은 두 방언 모두 끝에 붙고 모델 DDL은 필드 자리에 둔다. 비교하면 모델 중간에 필드를 넣을 때 재생성 말고는 못 고치는 차이가 영구히 남는다
  - `OnUpdate`는 작업 본문에 없다: 손으로 쓴 `ON UPDATE CASCADE`가 모델의 FK와 같게 읽히면 조용히 통과한다(PHILOSOPHY 7). 결정에 없는 선택
- `sqlite.Read(ctx, q Querier) (*schema.Schema, []diag.Diagnostic)`, `Querier`는 `QueryContext`만 가진 인터페이스. `practicedb.DB`가 쥔 연결로 `QueryContext`를 넘기는 메서드를 더해 이것을 만족한다(연결 하나 원칙 그대로). **진단이 있으면 8번은 비교하지 않는다**(부분 스키마의 빠진 것이 차이로 또 나오는 연쇄, D-61). 그래도 `Read`는 모든 테이블을 읽어 진단을 한 번에 낸다
  1. `sqlite_schema`: `type`이 `view`·`trigger`면 진단. 가상 테이블의 shadow 테이블(`pragma_table_list`의 `type = 'shadow'`)은 뺀다(원인 하나에 진단 하나). `table` 중 `sqlite_`로 시작하는 것(`sqlite_sequence` 포함)과 `maago_migrations`는 뺀다. `CREATE VIRTUAL TABLE`은 진단
  2. 테이블마다 `CREATE TABLE` 원문(SQLite가 `ALTER` 뒤 고쳐 쓴 것일 수 있다)을 `dialect.Split("", sql)`해 한 문장의 토큰으로. `Split`의 진단은 버린다(저장된 문장은 닫혀 있고, 원문에 남은 `-- maago:` 주석의 진단은 파일 단계에서 이미 났다). (`CREATE TABLE … AS SELECT`도 SQLite가 컬럼 목록으로 고쳐 저장하므로 진단할 것이 없다. 확인함) 첫 `(`와 짝 `)` 사이를 최상위 쉼표로 정의들로 나눈다. `)` 뒤 테이블 옵션의 `Word`: `STRICT` → `Strict`, `WITHOUT`(ROWID) → 진단
     - 정의의 첫 토큰이 `CONSTRAINT`·`PRIMARY`·`UNIQUE`·`CHECK`·`FOREIGN`(대소문자 무시)이면 테이블 제약, 아니면 컬럼 정의이고 첫 토큰이 컬럼 이름(`Word` 그대로, `Quoted`는 따옴표 벗김, `[`는 진단: fix `quote the name with "…"`)
     - 정의 안 **깊이 0**의 `CHECK` 뒤 `( … )`의 안쪽 토큰 → `Normalize` → `Check`. 바로 앞이 `CONSTRAINT 이름`이면 `Name`. 한 컬럼의 CHECK 둘은 둘
     - 컬럼 정의의 깊이 0 `AUTOINCREMENT` → `PKAuto`
     - 정의 안 깊이 0의 `COLLATE`·`CONFLICT`(`ON CONFLICT`)·`DEFERRABLE`·`MATCH`는 진단: 카탈로그에 없거나(`pragma_foreign_key_list.match`는 늘 `NONE`) 비교에서 빠져 조용히 같아진다(PHILOSOPHY 7). 깊이 0만: CHECK 식 안의 `COLLATE`는 비교되는 `Expr`의 일부. 작업 본문의 목록에 더한 것, 결정에 없는 선택
  3. `pragma_table_xinfo`: 이름·타입(대문자)·`notnull`·`dflt_value`(있으면 `Split`+`Normalize`, DB가 `DEFAULT (0)`의 괄호를 벗겨 준다)·`pk`(0보다 크면 그 순서로 `PK`). `hidden`이 0이 아니면 진단(generated 컬럼)
  4. `pragma_index_list`: `origin`이 `pk`면 건너뜀(PK가 이미 나타낸다). `partial`이면 진단. `pragma_index_xinfo`의 `key = 1` 행만(0은 rowid): `cid = -2`(식)면 진단, `desc = 1`이면 진단, `coll`이 `BINARY`가 아니면 진단. 단 그 컬럼이 2에서 `COLLATE`로 이미 진단됐으면 인덱스 쪽은 내지 않는다(원인 하나에 진단 하나, D-61). `Index{이름, 컬럼, unique, origin == "u"}`
  5. `pragma_foreign_key_list`: `id`로 묶고 `seq` 순으로 컬럼. 이름은 원문의 `CONSTRAINT 이름 … REFERENCES 대상`에서(없으면 ""), (컬럼, 대상 테이블)로 맞춘다(`pragma_foreign_key_list`의 `id`는 선언의 역순이라 순서로는 맞출 수 없다). `to`가 NULL(`REFERENCES people`)이면 대상 테이블의 `PK`로 채운다. 대상 테이블이 읽은 테이블에 없으면 진단 `foreign key of "t" references unknown table "x"`(SQLite는 없는 테이블을 참조하는 `CREATE TABLE`을 받는다. 대소문자가 다른 이름도 여기로)
  - 진단은 모두 위치 없음(DB에서 읽은 것), 메시지에 테이블(과 인덱스·컬럼) 이름과 무엇이 표현에 없는지, fix `maago does not support …; remove it from the migrations`. 8번이 마이그레이션 파일의 자리로 옮길지는 8번 계획에서
  - 조회 오류(연결이 끊김 등)는 위치 없는 진단, fix `report this as a maago bug`
- 하지 않는 것: 테이블 이름의 대소문자 무시 비교, `DEFAULT` 식의 의미 비교(`0`과 `0.0`), SQLite 3.37보다 낮은 버전(D-87의 STRICT가 이미 요구), `sqlite_schema`의 `sql`이 NULL인 테이블(내부 테이블뿐, 1에서 빠짐). `Normalize`가 `1.5`를 `1 . 5`로 만드는 것은 6번 진단 문구의 일(6번 계획에서). 타입의 affinity 비교(`INT`와 `INTEGER`는 다르게 읽힌다. 조용히 통과하지 않고 차이로 나온다)
- 확인 테스트는 `cmd/maago`(드라이버가 거기). 스키마를 사람이 읽는 글로 늘어놓는 함수는 테스트 파일에 두고 5번 PG 테스트도 같은 것을 쓴다
- 리뷰: fable(컬럼 순서 비교 → 하지 않음, 이름을 버림 → 담되 맞추지 않음, `Check.Column`의 뜻, `MATCH`는 늘 `NONE` → 원문에서, 따옴표 유무와 `Normalize`, `ALTER` 뒤 고쳐 쓴 원문, 없는 대상 테이블, 진단 뒤 비교 연쇄, 깊이 0, `Split` 진단 반복) 모두 반영. codex(첫 시도는 멈춰 취소, 다시: `ATTACH`·`TEMP` → 8번, `AS SELECT` → SQLite가 컬럼 목록으로 고쳐 저장함을 확인, 할 일 없음, affinity → 하지 않는 것에) 반영, `word$tag$`는 받지 않음(낱말 안의 `$`는 낱말로 읽힌다, 3번 골든의 `a$b`). 구현 리뷰: ponytail(`rows`의 인자, 테스트의 `extra` 줄임), codex(같은 컬럼에서 다른 테이블을 가리키는 FK 둘의 이름 → (컬럼, 대상)으로 맞춤. "CONSTRAINT 이름이 지워진다"는 받지 않음: 골든에 이름이 남는다. 샌드박스로 테스트는 못 돌림), fable(PK 인덱스의 `DESC`·`COLLATE`가 조용히 통과 → PK도 검사, `PRIMARY KEY (id AUTOINCREMENT)` → 읽음, FK 이름을 맞추는 법의 문구, `maago_migrations` 제외와 진단 뒤에도 읽는 것을 테스트로, `'이름'` 컬럼, fix 문구, 빈 `(`의 panic 경로) 반영. `Column.Type`의 대문자는 5번이 PG `format_type`을 대문자로 올린다. 분기마다 돌연변이를 넣어 모두 잡는 것을 확인
- 12번에 적을 것: 완료 조건 테스트가 필드를 끝에만 붙이면 컬럼 순서 문제가 숨으므로 중간에 넣는 필드를 하나 둔다

→ 확인 (`cmd/maago`에서 `go test -count=1 -run SQLiteRead ./...`)
1. `internal/dialect/sqlite/testdata/example.sql`(M1 골든)을 연습용 DB에 적용해 읽은 것이 `testdata/sqlite_example.golden`과 같다. 골든은 손으로 한 줄씩 대조한다
2. 같은 스키마를 공백·줄바꿈·대소문자·**정의의 이름**의 따옴표 유무(CHECK 식 안은 그대로: `Normalize`가 `"title"`과 `title`을 가른다, TODO)·`DEFAULT (0)`·`REFERENCES people`(컬럼 생략)·주석·인덱스 이름만 다르게 손으로 쓴 것이 1과 같은 글로 읽힌다(이름은 골든에서 뺀 글로 비교). `UNIQUE` 제약(인덱스 대신)은 `Constraint`만 다르다
3. `testdata/sqlite_cases.golden`: 컬럼 CHECK와 테이블 CHECK, 한 컬럼의 CHECK 둘, 문자열 안의 괄호·쉼표(`CHECK (a IN ('x,(', 'y'))`), `CONSTRAINT 이름 CHECK`, 여러 컬럼 FK, 복합 PK, `ON UPDATE CASCADE`, 비-STRICT. SQLite가 고쳐 쓴 원문: `ALTER TABLE … ADD COLUMN c TEXT CHECK (…)`, `RENAME COLUMN`(CHECK 식 안의 이름도 바뀐다), `DROP COLUMN` 뒤 각각
4. 막는 것 각각 진단 하나(골든): 부분 인덱스, 식 인덱스, `DESC` 인덱스, `COLLATE` 인덱스, `COLLATE` 컬럼(+ 그 컬럼의 인덱스: 진단 하나), generated 컬럼, `WITHOUT ROWID`, 트리거, 뷰, 가상 테이블, `[이름]`, `ON CONFLICT`, `DEFERRABLE`, `MATCH FULL`, 없는 대상 테이블. 진단이 있어도 나머지 테이블은 읽는다(D-61)

## 5. PG 읽기 (`internal/dialect/postgres`)

- `pg_catalog`에서, 모든 조회에 연습용 스키마의 `nspname` 조건. 타입은 `format_type`, 기본값은 `pg_get_expr`, CHECK 식은 `pg_get_constraintdef`에서 식만. FK는 문자열이 아니라 `pg_constraint`의 `conkey`·`confrelid`·`confkey`·`confdeltype`으로. 제약 이름은 담되 맞추지 않는다(4번 계획). IDENTITY는 `attidentity`
- 4번과 같은 표현, 같은 "표현에 없는 것은 진단" 규칙

→ 확인 (`cmd/maago` 모듈, D-91): 4번과 같은 경우들을 PG로. `public`에 같은 이름의 테이블을 만들어 두어도 읽히지 않는다

**계획**
- 새 파일: `internal/dialect/postgres/read.go`, `cmd/maago/pgread_test.go`, `cmd/maago/testdata/pg_*.golden`. 바꿀 파일: 두 방언이 같이 쓰는 것을 `internal/dialect`로 옮긴다: `Querier`, 행 읽기(`Rows`), 진단 문구(`Unsupported`, `Bug`)와 `schema.Sort`(4번 코드에서 옮김). 진단 문구는 두 방언이 글자까지 같다(`table "t" has …, which maago does not support` / `remove it from the migrations`)
- 작업 본문의 "제약 이름은 버린다"는 4번 계획(이름은 담되 맞추지 않는다, 7번의 `DROP CONSTRAINT "…"`)에 맞춰 고친다: `Name = conname`
- `postgres.Read(ctx, q dialect.Querier) (*schema.Schema, []diag.Diagnostic)`. 4번과 같은 계약: 진단이 있어도 끝까지 읽고, 진단이 있으면 8번은 비교하지 않는다. 조회 오류는 `report this as a maago bug`
- 모든 조회의 **읽는 쪽**은 `n.nspname = current_schema()`(연습용 DB의 `SET LOCAL search_path`, 2번). FK의 대상(`confrelid`)은 스키마 조건 없이 조인해 다른 스키마면 진단(codex)
- 번호 배열(`conkey`, `confkey`, `indkey`)은 `array_to_string(…, ',')`로 받아 2의 `attnum → 이름` 표로 바꾼다(fable: 숫자에는 쉼표가 없고, `indkey`의 0(식)도 자연히 걸린다). 인덱스의 키별 옵션은 `generate_subscripts(indkey, 1)` 한 번으로 돌며 `indnkeyatts`까지만 `indoption`·`indcollation`을 본다(codex: 따로 `unnest`하면 `INCLUDE`에서 어긋난다)
- 확장이 만든 객체(`pg_depend`의 `deptype = 'e'`)는 1의 진단에서 뺀다. 대신 연습용 스키마의 확장(`pg_extension`) 하나마다 진단 하나(원인 하나에 진단 하나, D-61). 8번이 `CREATE EXTENSION`을 적용 전에 진단한다(8번 본문에 더함)
  1. 관계(`pg_class`): `relkind` `r` 중 `maago_migrations`는 뺀다(4번과 같이, 12번이 실제 DB를 읽는다). `relispartition`인 자식은 뺀다(부모의 `p` 진단 하나로). `v`·`m`(뷰), `p`(파티션), `f`(외부 테이블)는 진단. 테이블의 `relpersistence = 'u'`, `pg_inherits`의 상속(파티션이 아닌 것), `relrowsecurity`, 규칙(`pg_rewrite`의 `_RETURN`이 아닌 것)도 진단. 시퀀스(`S`)는 이 스키마의 테이블 컬럼이 가진 것(`pg_depend` `deptype` `i`·`a`, `refobjid`가 그 테이블)만 넘기고 나머지는 진단. 함수(`pg_proc`), 타입(`pg_type`의 `e`·`d`·`r`, 복합 타입은 `typrelid`의 `relkind = 'c'`만. 배열 타입(`typelem`이 있는 것)과 multirange(`m`)는 뺀다: 각각 따라 생기는 것이라 원인 하나에 진단 둘)도 진단
  2. 컬럼(`pg_attribute`, `attnum > 0`, 지워진 것 제외): 타입 `upper(format_type(…))`, `attnotnull`, 기본값 `pg_get_expr`을 `Normalize`, `attidentity` `d`·`a` → PK 컬럼이면 `PKAuto` `BY DEFAULT`·`ALWAYS`, PK가 아닌 컬럼의 IDENTITY는 진단. `attgenerated`는 진단. `attcollation`이 타입의 기본과 다르면 COLLATE 진단
  3. 제약(`pg_constraint`): `p` → `PK`. `u` → `Index{Name, Unique, Constraint: true}`. `f` → FK(이름, 컬럼, 대상 테이블·컬럼, `confdeltype`·`confupdtype`을 글로: `a` NO ACTION, `r` RESTRICT, `c` CASCADE, `n` SET NULL, `d` SET DEFAULT), `confmatchtype`이 `s`가 아니면 진단, `confdelsetcols`(PG 15의 `SET NULL (cols)`)가 있으면 진단. `c` → CHECK: 정의의 토큰에서 `CHECK` 뒤 짝 괄호 안만 `Normalize`(뒤의 `NO INHERIT` 때문에 글자로 자르지 않는다), `connoinherit`면 진단, 컬럼이 하나면 그 컬럼(PG는 컬럼 CHECK와 테이블 CHECK를 가르지 않는다: 테이블 CHECK `(c > 0)`의 `Column`이 SQLite는 `""`, PG는 `c`. 같은 방언끼리만 비교하니 무해하고, 6번 진단 문구가 방언마다 조금 다를 수 있다). `x`는 진단. `n`(PG 18의 NOT NULL 제약)은 넘긴다. 모든 제약의 `condeferrable`, `convalidated = false`(NOT VALID)는 종류와 상관없이 진단
  4. 인덱스(`pg_index`): **모두 검사**하고, 같은 테이블의 `p`·`u`·`x` 제약이 쓰는 것(`conindid`, `conrelid`가 그 테이블)만 `Indexes`에 넣지 않는다(4번에서 PK 인덱스도 검사한 것과 같다: `UNIQUE NULLS NOT DISTINCT`, `PRIMARY KEY … INCLUDE`가 조용히 통과하지 않게). 진단: `indpred`(부분), `indexprs`(식), `btree`가 아닌 접근 방법, `INCLUDE`(`indnatts > indnkeyatts`), `indnullsnotdistinct`, 키마다 `indoption`의 DESC(1)·NULLS FIRST(2) 비트, 기본이 아닌 opclass(`pg_opclass.opcdefault = false`), `indcollation`이 그 컬럼의 `attcollation`과 다름(컬럼 COLLATE가 이미 진단됐으면 인덱스 쪽은 내지 않는다)
  5. 트리거(`pg_trigger`, `NOT tgisinternal`, 제약 트리거 포함)는 진단
- 하지 않는 것: 권한·소유자·코멘트·통계·테이블 공간, 파티션 읽기, 복합 PK의 한 컬럼만 IDENTITY인 경우(`PKAuto`가 어느 컬럼인지 잃지만 모델이 만들 수 없고 양쪽이 같게 읽힌다)
- 리뷰: codex(`conindid` 문구는 이미 같은 테이블의 `p`·`u`·`x`로 좁혀 두었다: 받지 않음, 키별 옵션을 `generate_subscripts`로, FK 대상의 스키마 조건, NULLS FIRST, NOT VALID는 종류와 상관없이, 시퀀스의 의존 대상, `Check.Column`의 뜻) 반영. fable(제약 인덱스도 검사, `maago_migrations`, 이름을 담기, `NO INHERIT`, 확장, 확인에 빠진 분기, 파티션 자식·multirange·배열·행 타입의 중복 진단, opclass, 공용 진단 함수, 번호 배열은 글로, `confdelsetcols`·규칙, 복합 PK의 IDENTITY) 모두 반영. 구현 리뷰: ponytail(괄호 짝 찾기를 `dialect.Closing`으로 두 방언이 같이), codex(인덱스 하나에 진단 여럿 → 하나, 확장이 만든 규칙·트리거도 빼기. 테스트는 돌리지 않음), fable(테스트가 개발 DB의 `public.people`을 지우는 `DROP`을 커밋함 → 연습용 트랜잭션 안에서 만들어 함께 롤백, `EXCLUDE USING gist`·DEFERRABLE 제약 트리거의 진단 둘 → 하나, 최소 PG 버전 → 15(D-98)와 연습용 DB를 열 때 확인, PG 18의 `NOT NULL … NO INHERIT`, CHECK 식의 바깥 괄호·문구의 방언 차이는 6번의 일) 반영. 돌연변이: 살아남은 둘(`typcategory`, 컬럼 COLLATE의 인덱스 억제)은 쓸데없는 조건이라 지웠고, `s < indnkeyatts`는 카탈로그의 바른 경계라 주석과 함께 둔다

→ 확인 (`cmd/maago`에서 `MAAGO_TEST_PG_URL`을 주고 `go test -count=1 -run PGRead ./...`)
1. `internal/dialect/postgres/testdata/example.sql`(M1 골든)을 읽은 것이 `testdata/pg_example.golden`과 같다. 손으로 한 줄씩 대조
2. 같은 스키마를 손으로 다르게 쓴 것(공백·대소문자·이름의 따옴표·인라인 `REFERENCES`·`UNIQUE` 컬럼 제약·`CHECK`를 컬럼 대신 테이블 제약으로·인덱스 이름)이 1과 이름 없이 같은 글
3. `testdata/pg_cases.golden`: 4번의 경우들(CHECK 여럿·문자열 안의 괄호와 쉼표·이름 붙은 CHECK, 여러 컬럼 FK·복합 PK·`ON UPDATE CASCADE`·`ON DELETE SET NULL`·`SET DEFAULT`·`RESTRICT`, `ADD`·`RENAME`·`DROP COLUMN` 뒤, 같은 컬럼의 FK 둘, 키워드 이름), `maago_migrations`는 읽히지 않음
4. 막는 것 각각 진단 하나(골든): 부분·식·DESC·NULLS FIRST·COLLATE·GIN·INCLUDE·NULLS NOT DISTINCT·opclass 인덱스, `UNIQUE NULLS NOT DISTINCT` 제약, `PRIMARY KEY … INCLUDE`, COLLATE 컬럼(+그 인덱스: 하나), generated 컬럼, PK가 아닌 IDENTITY, 뷰, 머티리얼라이즈드 뷰, 파티션 테이블(자식 둘: 진단 하나), 외부 테이블은 서버가 필요하므로 빼고, 트리거, 제약 트리거, 규칙, 함수, enum·도메인·범위·복합 타입(각각 하나), 떨어진 시퀀스, UNLOGGED, 상속, RLS, EXCLUDE, DEFERRABLE, `MATCH FULL`, `NOT VALID`(CHECK와 FK), `NO INHERIT`, `SET NULL (cols)`, 다른 스키마를 가리키는 FK, 확장(`CREATE EXTENSION citext`: 진단 하나). 진단이 있어도 나머지 테이블은 읽는다
5. `public`에 같은 이름의 테이블(다른 컬럼)을 만들어 두어도 읽은 결과가 같다

## 6. 비교 (`internal/migrate`)

- `Diff(from, to schema.Schema) []Change`. 테이블·컬럼 추가·삭제, 컬럼 타입·NULL·기본값·CHECK 변경, PK·FK·unique·index 추가·삭제, `Strict`·`PKAuto` 변경. 맞추는 기준과 컬럼 순서를 비교하지 않는 것은 4번 계획
- 변경은 결정적인 순서로(테이블은 모델 순, 삭제는 의존의 역순). 초안과 진단이 같은 순서를 쓴다
- 진단 문구: `migrations do not match models: column "body" of "blog_post" is missing`처럼 테이블·컬럼을 DB 이름으로. 위치는 모델 필드(모델에서 온 쪽이면) 또는 없음. fix는 `run maago makemigrations`
- 이름 변경은 여기서 판정하지 않는다. 비교는 삭제+추가를 낸다(D-37: 결과 스키마만으로는 구분되지 않는다)

→ 확인: 표 테스트(두 `Schema` → 변경 목록), unique의 `(a,b)`와 `(b,a)`는 같음, index는 다름(M1 5번)

**계획**
- 새 파일: `internal/migrate/diff.go`, `internal/migrate/diff_test.go`. 바꿀 파일: `internal/schema/schema.go`(`Sort`의 FK 키). 루트 모듈, DB 없음(읽은 `Schema` 둘만 받는다)
- `Diff(from, to *schema.Schema) []Change`. `from`은 마이그레이션(A), `to`는 모델(B). 처음에 양쪽을 `Sort`한다(7·9·12번이 손으로 만든 `Schema`를 넘겨도 같은 결과)
  ```go
  type Kind int // 값의 순서가 곧 변경의 순서
  const (
      DropFK Kind = iota; DropIndex; DropCheck; DropPK; DropTable; DropColumn
      CreateTable; AddColumn; AlterType; AlterNotNull; AlterDefault; AlterStrict
      AddPK; AddIndex; AddCheck; AddFK
  )
  type Change struct {
      Kind     Kind
      From, To *schema.Table  // 양쪽의 테이블, 없는 쪽은 nil
      Old, New *schema.Column // 컬럼 종류만, 없는 쪽은 nil
      Index    *schema.Index  // AddIndex·DropIndex: Drop은 From의 것, Add는 To의 것
      FK       *schema.FK     // 같은 규칙
      Check    *schema.Check  // 같은 규칙
  }
  func (c Change) Message() string // "migrations do not match models: …"
  ```
  - 테이블을 통째로 담는 이유: 7번의 SQLite 재생성은 옛 테이블의 컬럼(옮길 것)과 새 테이블이 모두 필요하고, PG의 `DROP INDEX`·`DROP CONSTRAINT`는 `From`의 이름을 쓴다. PK는 `From.PK`·`To.PK`로 충분해 따로 담지 않는다
- 맞추기(4번 계획의 기준): 테이블·컬럼은 이름. 컬럼은 `Type`·`NotNull`·`Default`를 글자 그대로 비교해 다른 것마다 변경 하나(타입과 기본값이 같이 다르면 둘: 서로 독립이다, D-61). 컬럼 순서는 보지 않는다. PK는 (`PK` 순서 있음, `PKAuto`)가 다르면 `DropPK`+`AddPK`(인덱스와 같이 삭제+추가. 한쪽에 PK가 없으면 그쪽 것만). 둘로 나눈 이유: PG에서 PK 컬럼을 지우기 전에 PK가 먼저 풀리고, 새 PK는 새 컬럼이 생긴 뒤에 걸린다(codex). unique 인덱스는 정렬한 컬럼 집합, 보통 인덱스는 컬럼 순서, 둘은 `Unique`로 갈린다(`Name`·`Constraint`는 보지 않는다). FK는 컬럼·대상 테이블·대상 컬럼·`OnDelete`·`OnUpdate`. CHECK는 `Expr`. 인덱스·FK·CHECK는 여럿 겹칠 수 있어 **짝 짓기**로 맞춘다(같은 CHECK 둘과 하나면 하나 삭제). 바뀐 인덱스·FK·CHECK는 삭제+추가
- 새 테이블·지운 테이블은 변경 하나만 낸다. 그 테이블의 컬럼·인덱스·FK·CHECK는 따로 내지 않는다(초안은 테이블을 통째로 쓰고, 진단은 원인 하나에 하나, D-61). 남은 테이블에서 컬럼이 바뀌어 생긴 인덱스·CHECK 차이는 따로 낸다: 초안(7번)에 각각 필요하고, 손으로 고친 경우(`max`만 지움)에는 독립된 차이다
- 순서: `Kind` 순 → 테이블 이름 순 → 테이블 안에서는 컬럼은 정의 순(삭제는 `From`의, 나머지는 `To`의), 인덱스·FK·CHECK는 `Sort`의 순서. M1 PG DDL(테이블 → FK → 인덱스)과 같은 생각으로, 삭제를 먼저(FK부터 풀어 다른 삭제를 막지 않게), 추가를 나중에(FK는 맨 끝, 가리킬 테이블·컬럼이 다 생긴 뒤). `DropTable`만 이름 순이 아니라 **의존의 역순**: 지우는 테이블끼리 FK가 있으면 가리키는 쪽을 먼저(PG는 참조되는 테이블을 먼저 지우지 못한다). 남은 테이블에서 지우는 테이블을 가리키던 FK는 `DropFK`로 먼저 나온다
- 작업 본문과 다른 곳: "테이블은 모델 순" → **이름 순**. `Schema`에 모델 순서가 없고, `Diff`가 모델을 받게 하면 DB 없는 비교에 모델이 끼어든다. "초안과 진단이 같은 순서"는 초안에만 해당한다: 진단은 `diag.Write`가 위치·문구로 정렬한다
- 7번에 넘길 것: `CreateTable`은 이름 순이라 PG의 새 테이블끼리 FK는 그 테이블의 DDL에 넣으면 아직 없는 테이블을 가리킬 수 있다. 새 테이블의 FK `ALTER`는 모든 `CreateTable` 뒤, `AddFK` 자리에 쓴다(M1 PG DDL과 같다, codex·fable). 또 `DropPK`는 남은 테이블의 **바뀌지 않은** FK가 그 PK를 가리키면 PG가 거부한다(FK가 같아 `DropFK`·`AddFK`는 나오지 않는다. `PKAuto`만 다른 경우). 7번이 그 FK를 함께 풀고 다시 걸지 정한다(구현 리뷰 fable)
- 8번에 넘길 것: 위치는 8번이 (테이블, 컬럼) DB 이름 → 모델 필드 표로 붙인다(`Change`에는 위치가 없다). 인덱스·FK·CHECK는 첫 컬럼의 필드, 테이블 종류는 모델. 모델에서 필드 하나를 지우면 `DropColumn`에 딸린 `DropIndex`·`DropCheck`가 함께 나온다(초안에는 모두 필요). 진단 하나로 접을지 8번 계획에서 정한다(fable)
- `Sort`의 FK 키에 대상 컬럼·`OnDelete`·`OnUpdate`·`Name`을 더한다: 같은 컬럼이 같은 테이블을 동작만 다르게 가리키는 FK 둘의 순서가 정해지지 않는다(codex). 읽기 골든은 바뀌지 않는다
- 문구(`Message`): 접두 `migrations do not match models: `, 이름은 읽기의 진단처럼 `%q`. "마이그레이션에 없다"는 `is missing`, "모델에 없다"는 `is not in the models`
  - `table "t" is missing` / `table "t" is not in the models`
  - `column "c" of "t" is missing` / `… is not in the models`
  - `column "c" of "t" is TEXT, the models have INTEGER`, `… is NOT NULL, the models have NULL`, `… has DEFAULT 0, the models have no default`
  - `primary key ("id") AUTOINCREMENT of "t" is not in the models`, `table "t" is not STRICT, the models have STRICT`
  - `unique index ("a", "b") of "t" is missing`, `foreign key ("a") -> "p" ("id") ON DELETE CASCADE of "t" is not in the models`(`ON UPDATE`는 `NO ACTION`이 아닐 때만), `check (LENGTH ("title") <= 200) of "t" is missing`
  - 기본값·CHECK 식은 `Normalize`의 글(단어는 대문자)을 `dialect.Split`으로 다시 나눠 붙인다. 규칙: 기본은 공백 하나. `(`·`[` 뒤, `)`·`]`·`,` 앞, `.` 앞뒤, `[` 앞은 공백 없음. `:` 앞뒤도 공백 없음(`: :` → `::`). 이웃한 `Punct` 둘이 `<>=!|` 중이면 붙인다(`< =` → `<=`). 토큰 전체를 감싼 괄호는 벗긴다(PG의 CHECK는 `((a <> ''::TEXT))`로 읽힌다, 5번 계획이 넘긴 것). `- 1`은 `- 1`로 둔다(`a < - 1`과 가를 수 없다). 4번 계획이 넘긴 `1 . 5` 때문이다(`DEFAULT 1 . 5`는 다른 값으로 읽힌다). 문구에만 쓰고 비교는 `Normalize`의 글 그대로. 받지 않은 것: 문구의 식을 SQL로 다시 쓸 수 있게 만들기(codex: 표시 전용이다)
- `diag.Diagnostic`은 만들지 않는다: fix `run maago makemigrations`와 위치는 8번이 붙인다. 6번은 `Message`까지
- 하지 않는 것: 이름 변경 판정(작업 본문), FK 컬럼 짝의 순서 무시(`(a,b)->(x,y)`와 `(b,a)->(y,x)`는 다르게 본다. 모델이 여러 컬럼 FK를 만들지 않는다), 지우는 테이블끼리 FK가 고리를 이루는 경우(서로 가리키는 두 모델을 한 번에 지울 때. codex는 지우는 테이블의 FK를 `DropFK`로 먼저 풀자고 했으나 진단이 원인 하나에 여럿이 된다. PG 초안이 실패하고 9번이 쓰기 전 다시 적용하므로 조용히 통과하지는 않는다 → TODO, 7번이 한 문장 `DROP TABLE a, b`로 풀 수도 있다), `Column.Type`의 뜻 비교(4번 계획)
- 리뷰: codex(PK를 삭제·추가로 나눔, 새 테이블 FK는 7번 `AddFK` 자리, 위치는 8번의 표, `Sort` FK 키) 반영. 고리는 위 이유로 TODO. 이름의 SQL식 이스케이프는 받지 않음(읽기 진단과 같은 `%q`). fable(컬럼 순서만 다름, 의존 순서가 이름 순서와 어긋나는 확인, Drop은 From의 것, 한 컬럼의 차이 둘, `Diff`가 `Sort`, 문구 예와 포매터 규칙, `unique(a)`+`index(a)`, 지운 필드의 딸린 변경은 8번, 7번에 넘길 FK) 모두 반영. 포매터를 빼자는 대안(ponytail)은 `1 . 5`가 다른 값으로 읽혀 받지 않음. 구현 리뷰: ponytail(`Sort` 테스트의 반복, 빈 `Name`) 반영, `reflect.DeepEqual`은 D-66 게이트 때문에 받지 않음. codex(`Diff`가 입력을 정렬하고 `Change`가 입력을 가리킴 → 주석) 반영. 새 테이블의 `AddFK`가 없다는 지적은 계획대로(7번), 고리는 TODO, 컬럼 순서는 읽기가 정의 순으로 결정적이라 받지 않음. 테스트는 샌드박스로 못 돌림. fable(FK 키의 `OnUpdate`·`RefColumns`·`RefTable`이 검증되지 않음, 16종 모두 나오는 경우, 정렬하지 않은 입력, 자기를 가리키는 지우는 테이블, PG CHECK의 바깥 괄호, `DropPK`와 바뀌지 않은 FK → 7번, 순서 문구, `%q` 키, 평평한 `if`) 모두 반영. 돌연변이: 살아남은 것은 `SortStableFunc` → `SortFunc` 하나(변경이 적어 우연히 안정적. 코드는 맞다)

→ 확인 (루트에서 `go test -count=1 ./internal/migrate/`)
1. 같은 스키마 → 변경 없음. 컬럼 순서만 다른 테이블 → 없음. 이름·`Constraint`만 다른 인덱스, `(a,b)`·`(b,a)` unique → 없음. `(a,b)`·`(b,a)` index → 삭제+추가(M1 5번)
2. 종류마다 한 경우씩: 기대한 `Message` 목록과 같다(문구까지). Drop 쪽은 From과 To의 `Name`·`Constraint`를 다르게 두고 `Change`의 것이 From의 것임을 본다. 한 컬럼에 타입과 기본값이 함께 다르면 변경 둘. `unique(a)`+`index(a)` → `unique(a)`는 `DropIndex` 하나
3. 순서: 여러 종류가 섞인 경우 `Kind` 순, 지우는 테이블 셋(`a → c → b` FK, 이름 순과도 역순과도 다름)은 `a, c, b`, 남은 테이블의 FK가 지우는 테이블을 가리키면 `DropFK`가 `DropTable`보다 앞. 입력을 정렬하지 않고 넘겨도 같다
4. 새 테이블·지운 테이블은 변경 하나. 같은 CHECK 둘 → 하나면 `DropCheck` 하나
5. 문구의 식: `1 . 5`, `< =`, PG의 `'draft' : : TEXT`, `ARRAY [ … ]`, `IN ( 0 , 1 )`이 붙은 모양으로
6. `schema.Sort`: 동작만 다른 같은 컬럼의 FK 둘이 입력 순서와 상관없이 같은 순서(`cmd/maago`의 읽기 골든도 그대로)

## 7. 초안 DDL (`internal/dialect/sqlite`, `internal/dialect/postgres`)

- 변경 목록 → 방언별 SQL. 새 테이블은 M1의 `DDL`을 그 테이블만큼 재사용한다
- PG: `ALTER TABLE … ADD/DROP COLUMN`, `ALTER COLUMN … TYPE/SET/DROP NOT NULL/SET/DROP DEFAULT`, 제약·인덱스 `ADD`/`DROP`
- SQLite: `ADD COLUMN`으로 되는 것은 그것으로, 나머지는 **테이블 재생성**(SQLite 문서의 12단계: 새 테이블 생성 → 복사 → 옛 테이블 삭제 → 이름 바꾸기 → 인덱스 다시 만들기)
- 재생성의 `DROP TABLE t`는 데이터를 버리지 않는다. 앞에 `INSERT INTO x … FROM t`, 뒤에 `ALTER TABLE x RENAME TO t`가 있으면 allow-drop 대상이 아니다(D-93, 8번이 검사)
- 단, 모양만으로는 옮기는 `SELECT`가 컬럼을 빠뜨린 것을 알 수 없다. 재생성 앞뒤의 스키마를 연습용 DB에서 읽어 사라진 컬럼이 있으면 그 `DROP TABLE`에 표시를 요구한다(D-93). 필드 삭제는 되도록 `ALTER TABLE … DROP COLUMN`(SQLite 3.35+)으로 쓰고, 그럴 수 없을 때(인덱스·FK·CHECK에 걸린 컬럼)만 재생성한다. 구체적인 방법은 7번 계획
- `NOT NULL`이고 기본값 없는 컬럼 추가는 기존 행이 있으면 실패한다. 초안에 ⚠ 주석으로 알리고 그대로 둔다(추측해서 값을 채우지 않는다, PHILOSOPHY 6)
- `DROP`(테이블·컬럼)은 초안에 쓰되 `-- maago: allow-drop`은 **붙이지 않는다**. 대신 ⚠ 주석으로 무엇을 지우는지와 표시를 다는 법을 적는다. 기계가 표시까지 붙이면 D-37이 아무것도 막지 못한다

→ 확인: 변경 종류마다 초안 골든(두 방언). 초안을 2번의 `Exec`로 연습용 DB에 적용하면 `Diff`가 비는 것을 경우마다 확인(`cmd/maago` 모듈). SQLite 재생성은 행과 FK가 있는 테이블로

**계획**
- 새 파일: `internal/dialect/sqlite/draft.go`, `internal/dialect/postgres/draft.go`, `cmd/maago/draft_test.go`, `cmd/maago/testdata/draft_sqlite.golden`·`draft_pg.golden`. 바꿀 파일: M1 `DDL`을 모델·필드 단위로 나눈다(sqlite `table`에 이름 인자, 두 방언의 인덱스·PG FK 문장, PG 타입·CHECK를 함수로. `DDL`의 출력은 그대로, M1 골든이 확인). `schema.Table`에 `PKName`(PG `conname`, SQLite는 `""`)
- `sqlite.Draft(apps []*model.App, cs []migrate.Change) string`, `postgres.Draft`도 같다. 문장을 `"\n\n"`으로 잇고 끝에 줄바꿈(`DDL`과 같다). `cs`가 비면 `""`. 모든 이름은 `dialect.Quote`로(M1과 같이. 아래의 `t`·`c`·`n`은 줄인 표기)
- **글의 출처**(결정에 없는 선택): 모델 쪽 글은 모두 **모델의 M1 DDL 조각**에서: 새 테이블, SQLite 재생성의 새 테이블, 추가하는 컬럼 정의(타입·NOT NULL·기본값·CHECK), PG의 바꾸는 타입·기본값, 더하는 CHECK. `Change`의 (테이블, 컬럼) DB 이름으로 모델 필드를 찾는다(B는 모델 DDL을 적용해 읽은 것이라 늘 있다). 인덱스·제약의 이름과 컬럼, FK의 대상은 `Change`의 것(B의 이름은 모델의 이름과 같다). 지우는 문장은 `From`의 이름. `migrate`의 `display`는 문구 전용으로 남는다(6번 계획의 결정, fable: `Normalize`가 `1e-07`을 `1e - 07`로 나눠 SQL로 다시 쓰면 두 방언 모두 거부한다). 초안이 TUTORIAL §4 모양이고, PG 역파싱 글(`CHARACTER VARYING(200)`, `STATUS = ANY (ARRAY[…])`)이 섞이지 않는다
- **PG**: `Change`마다 순서대로
  - `DropFK`·`DropCheck`, `Constraint`인 `DropIndex` → `ALTER TABLE t DROP CONSTRAINT n`, 아닌 `DropIndex` → `DROP INDEX n`
  - `DropTable` → ⚠ 주석 + `DROP TABLE t`, `DropColumn` → ⚠ 주석 + `ALTER TABLE t DROP COLUMN c`. ⚠은 무엇의 행·값이 사라지는지와 "의도했다면 ⚠ 줄 아래, `DROP` 바로 앞 줄에 `-- maago: allow-drop`"(표시는 붙이지 않는다. ⚠ 줄은 `-- maago:`로 시작하지 않는다: 3번의 표시 규칙)
  - `CreateTable` → 모델의 `CREATE TABLE`과 그 인덱스. 그 테이블의 FK `ALTER`는 맨 끝에 모은다(6번이 넘긴 것: 이름 순이라 아직 없는 테이블을 가리킬 수 있다)
  - `AddColumn` → `ADD COLUMN` + 모델의 컬럼 정의(인라인 CHECK 포함). 그 컬럼의 `AddCheck`(`Column`이 그 컬럼)는 여기에 들어간다. NOT NULL이고 기본값이 없으면 ⚠(행이 있으면 실패한다, 기본값을 주거나 NULL로 더하고 채운 뒤 `SET NOT NULL`)
  - `AlterType` → `ALTER COLUMN c TYPE T USING c::B`(T는 모델 타입, B는 그 길이 없는 기본 타입. `USING`이 없으면 `TEXT` → `BIGINT`가 빈 테이블에서도 거부된다. `::VARCHAR(100)` 캐스트는 긴 값을 조용히 자르고, `USING` 뒤의 대입만 길이를 검사해 거부한다, 구현 리뷰 fable). 옛 컬럼에 기본값이 있으면 앞에 `DROP DEFAULT`, 뒤에 모델의 기본값 `SET DEFAULT`(옛 기본값이 새 타입으로 캐스트되지 않으면 `TYPE`이 거부된다, codex·fable). 그 컬럼의 `AlterDefault`는 이때 함께 처리된다
  - `AlterNotNull` → `SET`/`DROP NOT NULL`, `AlterDefault` → 모델 기본값으로 `SET DEFAULT d`/`DROP DEFAULT`. `AlterStrict`는 PG에 나오지 않는다
  - PK: 컬럼이 같고 `PKAuto`만 다르면 제약은 두고 IDENTITY만, `AddPK` 자리에서: `""` → `ADD GENERATED BY DEFAULT AS IDENTITY`, 있던 것 → `SET GENERATED BY DEFAULT`(모델은 늘 `BY DEFAULT`라 모델 쪽이 없는 경우는 없다). 바뀌지 않은 FK가 가리키는 PK를 풀지 않는다(6번이 넘긴 것의 흔한 경우). 옛 컬럼의 기본값(`nextval`)은 `AlterDefault`가 먼저 지운다(Kind 순). 컬럼이 다르면 `DropPK` → (IDENTITY가 있으면 `DROP IDENTITY`) + `DROP CONSTRAINT From.PKName`, `AddPK` → `ADD CONSTRAINT To.PKName PRIMARY KEY (…)` + (모델이면 `ADD GENERATED`). 그 PK를 다른 테이블의 바뀌지 않은 FK가 가리키면 PG가 거부한다: 모델은 PK 컬럼을 바꾸지 못하므로 손으로 고친 마이그레이션에서만 생기고, 9번이 쓰기 전 적용에서 잡는다 → 하지 않는 것
  - `AddIndex` → `Constraint`면 `ADD CONSTRAINT n UNIQUE (…)`, 아니면 `CREATE [UNIQUE] INDEX n ON t (…)`. `AddCheck`(있던 컬럼) → `ADD CONSTRAINT n CHECK (모델의 식)`: PG 모델의 CHECK는 필드마다 많아야 하나(`choices`)다. `AddFK` → `ADD CONSTRAINT n FOREIGN KEY (…) REFERENCES r (…) ON DELETE x`(모델의 FK는 늘 `ON UPDATE NO ACTION`)
- **SQLite**: 테이블마다 바로 쓸 수 있는 변경만 있으면 그대로, 하나라도 아니면 **재생성**. 바로 쓰는 것:
  - `Constraint`가 아닌 `AddIndex`·`DropIndex` → `CREATE [UNIQUE] INDEX`·`DROP INDEX`
  - `AddColumn`: NULL을 받거나 기본값이 있고, PK도 FK도 아닌 컬럼 → `ADD COLUMN` + 모델의 컬럼 정의(CHECK 포함), 그 컬럼의 `AddCheck`는 여기로. SQLite는 NOT NULL에 기본값 없는 `ADD COLUMN`을 빈 테이블에서도 거부하고, FK 컬럼의 기본값은 NULL이어야 한다. FK 컬럼 추가는 NULL을 받아도 재생성(분기를 줄인다)
  - `DropColumn`: PK·FK·`Constraint` unique에 없고, 다른 컬럼의 CHECK·테이블 CHECK의 식에 이름이 나오지 않는 컬럼(SQLite `DROP COLUMN`의 조건. 보통 인덱스는 `DropIndex`가 먼저 지운다) → ⚠ + `ALTER TABLE t DROP COLUMN c`. 그 컬럼의 `DropCheck`는 함께 사라진다(작업 본문: 되도록 `DROP COLUMN`)
  - 재생성(SQLite 문서의 순서, D-93의 모양): `CREATE TABLE "maago_new_t" (…모델…) STRICT` → `INSERT INTO "maago_new_t" (공통 컬럼) SELECT 공통 컬럼 FROM "t"` → `sqlite_sequence`의 `t` 값을 `maago_new_t`로 옮김(`DELETE`+`INSERT … SELECT`, 아래) → `DROP TABLE "t"` → `ALTER TABLE "maago_new_t" RENAME TO "t"` → 모델의 인덱스. 공통 컬럼은 양쪽에 같은 이름으로 있는 것, 모델 순. 그 테이블의 다른 변경은 모두 여기에 들어간다. 재생성 블록은 그 테이블의 첫 변경 자리에 쓴다(`Diff` 순서를 따른다)
  - `sqlite_sequence`: `DROP TABLE`이 `t`의 행을 지우고 새 테이블은 max(id)+1부터 세므로, 끝에서 지운 id가 다시 쓰인다(D-87의 근거와 어긋남, fable). `sqlite_sequence`에는 unique가 없어 `INSERT OR REPLACE`가 겹친 행을 만든다: 복사가 만든 행을 `DELETE`하고 옛 값을 `INSERT … SELECT`. `RENAME`이 이름을 고쳐 준다
  - 재생성의 ⚠: 옮기지 않는 컬럼(마이그레이션에만 있는 것)이 있으면 `DROP TABLE` 앞에 그 컬럼들과 표시를 다는 법(D-93). NOT NULL이고 기본값 없는 새 컬럼이 있으면 `INSERT` 앞에(행이 있으면 실패)
  - 임시 이름 `maago_new_t`: `maago_`는 maago의 것(`maago_migrations`)이다. 같은 이름의 테이블이 이미 있으면 `CREATE TABLE`이 실패하고 9번의 쓰기 전 적용이 잡는다(codex의 충돌 검사는 받지 않음: 조용히 통과하지 않고, 쓸 일이 없는 이름이다). 모델의 자기 참조 FK는 `"t"`를 가리킨 채 만들어져 이름을 바꾼 뒤 맞는다(SQLite 문서의 방법, 확인 3)
  - `PRAGMA foreign_keys`를 끄고 켜는 것과 `foreign_key_check`는 초안에 쓰지 않는다: 트랜잭션 안에서는 바뀌지 않으므로 적용하는 쪽(8번 연습용 적용, 10번 `migrate`)이 파일 둘레에서 한다(작업 10 본문)
- 8번에 넘길 것(열린 질문 "재생성으로 사라지는 컬럼"의 답, fable): 연습용 DB A에 적용할 때, 재생성 모양의 `DROP TABLE t` 직전에 `t`의 컬럼을 읽어, 앞 `INSERT`의 `SELECT` 목록에 나오지 않는 옛 컬럼이 있으면 그 `DROP TABLE`에 표시를 요구한다(8번 계획에서 컬럼 목록 대신 `SELECT` 목록으로 고침). 스키마 전후 비교는 양쪽에 있는데 목록에서만 빠진 컬럼을 놓친다(연습용 DB는 비어 있어 값이 사라지는 것이 보이지 않는다). 컬럼 목록 없는 `INSERT`는 표시를 요구한다. 한계: `SELECT a, NULL FROM t`는 통과한다. `INSERT`와 `DROP` 사이의 `sqlite_sequence` 문장은 모양을 깨지 않는다. 8번 확인 3의 문구를 이 뜻으로 맞춘다
- 두 방언이 같이 쓰는 것: 모델 찾기(`dialect.Model`·`Field`)와 ⚠ 문구(`dialect.Dropping`·`NoDefault`)는 `internal/dialect/draft.go`, 변경의 테이블 이름은 `migrate.Change.Table()`
- 9번에 넘길 것: `--rename`은 `RENAME COLUMN`을 연습용 DB A에 먼저 적용하고 다시 읽어 `Diff` → `Draft`를 부른다(`Draft`는 이름 변경을 모른다, fable)
- 하지 않는 것: 이름 변경(9번), 값의 변환(`USING`은 캐스트만, SQLite 재생성은 값을 그대로 옮긴다), NOT NULL로 바꿀 때의 ⚠(작업 본문은 추가만), PK 컬럼 바꾸기를 가리키는 FK(위), 지우는 테이블끼리의 FK 고리(6번 TODO. codex가 다시 짚었다), 기존 행의 값이 새 CHECK에 걸리는 경우(모델은 `choices` 밖의 기본값을 거부한다. 행의 값은 적용이 알려 준다, codex)
- 리뷰: codex(`TYPE` 앞 기본값, 이름 인용, IDENTITY 전환의 글과 옛 기본값) 반영. 고리는 TODO, 임시 이름 충돌·기본값과 CHECK는 위 이유로 받지 않음. fable(D-93 검사는 `INSERT` 목록으로, `Display`를 SQL로 쓰지 않음 → 모델 글, `TYPE` 앞 기본값, 행 있는 경우와 없는 경우, `PKName` 경로를 실제로 지나는 확인, `sqlite_sequence`, `Split`으로 적용·⚠ 줄·자기 참조 재생성, `--rename`) 모두 반영 구현 리뷰: ponytail(두 방언이 같은 `DROP`·인덱스 문장과 `dialect.Field`, 잃는 컬럼 문구 한 벌, 테스트 도우미) 반영, `rowsText` 줄이기는 받지 않음. codex(`sqlite_sequence`가 없을 수 있다 → 재생성의 새 테이블이 `AUTOINCREMENT`라 늘 있다, 따옴표 겹친 이름 → 틀리면 적용이 실패한다, SQLite 바로 쓰는 CHECK → `lang` 경우가 덮는다) 받지 않음, 테스트는 샌드박스로 못 돌림. fable(`USING`의 길이 캐스트가 값을 자름 → 기본 타입, FK·`choices` 필드 추가, 복합 PK로 `DROP IDENTITY` 경로, `UNIQUE` 제약만 다른 경우, 닿지 않는 분기 둘 삭제, `alter`가 글 하나, SQLite `type` 경우가 보이게) 반영. 손 SQL 전용 가드(`droppable`)의 미검증은 받아들인다: 틀리면 SQLite `DROP COLUMN`이 9번의 쓰기 전 적용에서 실패한다

→ 확인 (`cmd/maago`에서 `go test -count=1 -run Draft ./...`, PG는 `MAAGO_TEST_PG_URL`)
1. 경우마다 모델 둘(전·후, 임시 프로젝트): A에 전 모델의 DDL(+ 방언별 손 SQL), B에 후 모델의 DDL, `Diff` → `Draft` → 골든(두 방언). 초안을 `dialect.Split`으로 나눠 A에 적용(SQLite는 `foreign_keys` 끄고, `foreign_key_check`가 빈 것까지 보고 켠다)하고 다시 읽으면 B와 `Diff`가 빈다. A는 비어 있다(NOT NULL 기본값 없는 추가는 행이 있으면 실패하는 것이 맞다), 행은 확인 3에서만
2. 경우: 빈 → 모델 셋(이름 순보다 앞 테이블이 뒤 테이블을 가리키는 FK: PG가 FK를 끝에), NULL 필드 추가, NOT NULL 기본값 없는 필드 추가(PG ⚠, SQLite 재생성 ⚠), `max`·기본값 있는 필드 추가(SQLite `ADD COLUMN … CHECK`), 인덱스 없는 필드 삭제(`DROP COLUMN` ⚠), FK 필드 삭제(PG `DropFK`·`DROP INDEX`·`DROP COLUMN`, SQLite 재생성과 `DROP TABLE` ⚠), 타입(`int32` → `int64`), 타입과 기본값 함께(`string` `'0'` → `int64` `0`), `max` 바꾸기, `*T`로(NOT NULL), 기본값, 작은 `float64` 기본값(`0.0000001`)과 음수 기본값, `unique` 더하기·빼기, `choices` 더하기(CHECK), 모델 삭제(`DROP TABLE` ⚠). 손 SQL: SQLite 비-STRICT·`UNIQUE` 제약·`AUTOINCREMENT` 없음·이름 붙은 테이블 CHECK, PG IDENTITY `ALWAYS`·없음(옛 `serial` 기본값 포함), PK를 다른 컬럼에(아무도 가리키지 않는 테이블: `DROP CONSTRAINT` 이름 경로), `ON DELETE CASCADE` FK
3. SQLite 재생성은 행이 있는 테이블, 그것을 가리키는 행, 자기 참조 FK로: 적용 뒤 행이 그대로이고 FK 검사가 비고, 끝 id를 지운 뒤 재생성하고 새 행을 넣으면 그 id를 다시 쓰지 않는다
4. M1 `DDL` 골든(`internal/dialect/*/testdata/example.sql`)이 그대로다

## 8. 마이그레이션 파일과 DB 검사 단계 (`internal/migrate`, `internal/build`)

- `migrations/NNNN_이름.sql`, 프로젝트 루트에 하나(D-38, D-94). 번호는 네 자리, 1부터 빈틈·겹침 없이. 어긋나면 파일 위치의 진단. 파일 머리는 없다(D-94)
- 해석 단계에 마이그레이션 파일 읽기(문장 나누기, 표시 줄)를 더한다(ARCHITECTURE §3)
- **DB 검사 단계**(ARCHITECTURE §3의 자리): 연습용 DB A에 모든 파일을 순서대로 적용, B에 모델 DDL, 둘을 읽어 비교. 차이는 진단
  - 적용 오류는 그 문장의 `파일:줄`(PG가 주는 위치가 있으면 칸까지)과 DB의 메시지
  - 바로 앞에 `-- maago: allow-drop`이 없는 `DROP`(테이블·컬럼) 문장은 그 위치의 진단(D-37, 3번). SQLite 재생성은 7번의 규칙대로
  - 모든 파일에 요구한다. `check`는 실제 DB를 보지 않으므로 적용 여부를 모르고, 표시는 파일에 남는 기록이다(D-37)
  - 트랜잭션 제어문(`BEGIN`·`START TRANSACTION`·`COMMIT`·`END`·`ROLLBACK`·`ABORT`·`SAVEPOINT`·`RELEASE`), `CREATE INDEX CONCURRENTLY`, `search_path`를 바꾸는 `SET`(`LOCAL`·`SESSION` 포함)·`RESET`은 그 위치의 진단. `DROP` 판정은 `DROP TABLE`만이 아니라 `ALTER TABLE … DROP COLUMN`도. `-- maago: allow-drop`이 `DROP`이 아닌 문장에 붙었으면 표시 위치의 진단(3번의 `AllowDrop`). 파일은 `migrate`가 트랜잭션 하나로 감싸고(10번), PG 연습용 DB는 트랜잭션 안에서 돈다(D-96)
  - 스키마 이름이 붙은 이름(`public.x`)은 롤백되어 남지 않지만, 연습용 DB가 닫힐 때까지 그 실제 테이블의 잠금을 쥐어 다른 세션을 막을 수 있다. 진단할지 계획에서(2번 구현 리뷰, fable)
  - 트랜잭션 제어문 목록에 `PREPARE TRANSACTION`·`COMMIT PREPARED`·`ROLLBACK PREPARED`도: 연습용 트랜잭션을 연결이 끊겨도 남는 객체로 바꾼다(D-96)
  - SQLite `CREATE TRIGGER … BEGIN …; END;`는 3번이 본문의 `;`에서 나누므로 DB 문법 오류가 먼저 난다. 첫 두 `Word`가 `CREATE TRIGGER`면 적용 전에 `triggers are not supported`로 진단(4번이 막는 것과 같은 이유)
  - PG 오류의 `Position`은 문장 안의 1부터 세는 **글자** 번호다. `Stmt.Pos.Offset`(바이트)에 더하기 전에 글자를 바이트로 바꾼다
  - `CREATE EXTENSION`은 진단(PG 연습용 스키마에 확장의 객체가 깔린다, 5번 계획)
  - `ATTACH`, `CREATE TEMP`(`TEMPORARY`) 문장은 진단: 읽기(4·5번)는 연습용 스키마만 보므로 그 밖의 객체는 비교에서 빠진다
  - 적용은 두 방언 모두 첫 실패에서 멈춘다(PG는 실패 뒤 트랜잭션이 aborted라 더 갈 수 없다. 방언마다 오류 개수가 달라지지 않게, D-61)
- `makemigrations`(9번)는 같은 단계를 비교의 차이를 진단으로 바꾸지 않는 모드로 쓴다(모양 절). 단계 목록을 두 벌 만들지, 비교 단계만 바꿀지는 계획에서
- 방언을 알게 되었으므로 PG 프로젝트는 `postgres.DDL`의 63바이트 검사가 이 단계 전에 돈다(M1에서 넘어옴)
- `gen`·`check`는 같은 단계를 돈다. 모델을 바꾸고 `makemigrations`를 잊으면 `gen`도 실패하고 fix가 `maago makemigrations`를 말한다
- M1의 `internal/build` 종단 테스트(gen → check 통과)는 DB가 필요해지므로 `cmd/maago` 모듈로 옮긴다. 루트에는 DB 전 단계의 오류와 D-64 진단만 남긴다

→ 확인
1. 루트 모듈: 드라이버 없는 `Check`가 D-64 진단(위치 없음, fix 있음)으로 실패하고 아무것도 쓰지 않는다
2. `cmd/maago` 모듈, 두 방언: 모델과 맞는 마이그레이션 → `check` 0. 그 뒤 초안을 한 곳씩 손으로 바꾼 표(완료 조건 2): 컬럼 삭제, 타입, NOT NULL, 기본값, CHECK 완화(`max` 삭제, 선택지 하나 더), FK 삭제·`ON DELETE CASCADE`, unique 삭제, index 컬럼 순서, `AUTOINCREMENT`·IDENTITY 삭제. 각각 `check` 1이고 문구가 바뀐 곳을 말한다
3. 표시(완료 조건 3): 표시 없는 `DROP` → 그 문장 위치의 진단. 표시가 다른 문장에 붙은 경우, `DROP` 둘 중 하나만 표시한 경우. SQLite 재생성의 `DROP TABLE`은 표시 없이 통과하지만, 옮기는 `SELECT`에서 컬럼 하나를 빼면 진단(D-93)
4. 문법 오류 문장은 그 줄을 짚는다. 번호 빈틈은 파일 위치의 진단. 하위 디렉터리에서 돌려도 루트의 `.env`를 읽고, `.env` 오류면 아무것도 쓰지 않는다(1번)
5. 두 브랜치가 각자 만든 `0003`이 함께 있으면 두 파일 위치의 진단, fix는 나중에 합친 쪽을 다음 번호로 바꾸고 `maago check`를 다시 돌리라는 것. 바꾼 뒤 두 파일을 차례로 적용한 결과는 기존 비교가 확인한다(Alembic의 head가 둘인 경우. merge 리비전 같은 개념은 두지 않는다)

**계획**
- 새 파일: `internal/migrate/files.go`(파일 읽기와 DB 없는 문장 검사), `internal/migrate/apply.go`(연습용 DB에 적용, 재생성 `DROP` 검사), `internal/migrate/diagnose.go`(변경 → 진단), 각각의 `_test.go`(루트, DB 없음), `cmd/maago/check_test.go`. 바꿀 파일: `internal/build/build.go`(단계), `internal/build/build_test.go`(DB가 필요한 것은 `cmd/maago`로), `cmd/maago/main_test.go`(`.env`·마이그레이션), `internal/practicedb/practicedb.go`(PG 오류 위치), `cmd/maago/main.go`(그 위치를 꺼내는 함수를 넣는다)
- **파일**(`migrate.ReadFiles(root) ([]File, []diag.Diagnostic)`): `migrations/*.sql`. 디렉터리가 없으면 파일 없음(A는 빈 스키마). 이름은 `^(\d{4})_([a-z0-9_]+)\.sql$`(D-94, 9번의 `--name` 규칙과 같다). 맞지 않는 `.sql`은 그 파일 위치의 진단, `.sql`이 아닌 파일과 하위 디렉터리는 보지 않는다. 번호는 1부터: 겹치면 겹친 파일 **모두**의 진단(fix: 나중에 합친 쪽을 다음 빈 번호로 바꾸고 `maago check`를 다시), 빠지면 빈틈 뒤 첫 파일의 진단(`migration 0003 is missing before 0004_x.sql`). 파일마다 `dialect.Split(경로, 내용)`, 그 진단도 이 단계에서. `File{Path, Number, Stmts}`
- **DB 없는 문장 검사**(`migrate.CheckFiles(files) []diag.Diagnostic`, 문장의 첫 낱말들로. 대소문자 무시, 모두 그 문장 위치):
  - 트랜잭션 제어: `BEGIN`·`START TRANSACTION`·`COMMIT`(`COMMIT PREPARED` 포함)·`END`·`ROLLBACK`(`ROLLBACK PREPARED` 포함)·`ABORT`·`SAVEPOINT`·`RELEASE`·`PREPARE TRANSACTION`. fix: `remove it; maago runs each migration in one transaction`
  - **모든** `SET`·`RESET`·`PRAGMA`와 `set_config` 호출(낱말 `set_config` 뒤 `(`), `DO`: 연습용 DB와 `migrate`가 정한 설정(`search_path`, `standard_conforming_strings`, `foreign_keys`)을 되돌리거나 검사할 수 없는 SQL을 돈다(D-96, codex·fable). 목록 대신 전부 막는 쪽이 규칙이 적다(PHILOSOPHY 3). 대가: PG `SET CONSTRAINTS`(DEFERRABLE은 5번이 이미 막는다), SQLite의 무해한 `PRAGMA`
  - `CONCURRENTLY`: `CREATE [UNIQUE] INDEX`·`DROP INDEX`·`REINDEX` 문장의 앞 다섯 토큰 안의 따옴표 없는 낱말(식 안의 컬럼 `concurrently`를 잡지 않게, codex)
  - `CREATE EXTENSION`, `CREATE SCHEMA`, `ATTACH`·`DETACH`, `CREATE [OR REPLACE] [GLOBAL|LOCAL] TEMP|TEMPORARY …`, `CREATE [OR REPLACE] [MATERIALIZED] VIEW`, `CREATE [OR REPLACE] TRIGGER`(4·5번이 위치 없이 막는 것을 문장 위치로. SQLite 트리거 본문이 `;`에서 나뉘어 뒤따르는 `END` 한 낱말 문장은 같은 원인이라 다시 내지 않는다)
  - **스키마가 붙은 대상 이름**(fable·codex): `CREATE TABLE`, `CREATE [UNIQUE] INDEX 이름 ON 테이블`(둘 다), `ALTER TABLE`, `DROP TABLE`, `INSERT INTO`의 대상 이름(`IF [NOT] EXISTS`·`ONLY`는 건너뜀) 바로 뒤가 `.`이면 진단. 읽기(4·5번)는 연습용 스키마(`main`, `current_schema()`)만 보므로 `public.x`·`temp.x`는 적용되고 비교에서 빠진다(PHILOSOPHY 7). 식 안의 `pg_catalog.lower(…)`·`t.col`은 대상 자리가 아니라 건드리지 않는다
  - **allow-drop**(D-37): 데이터를 지우는 문장은 `DROP TABLE`과 `ALTER TABLE t` 뒤 최상위 쉼표로 나눈 동작 중 `DROP`으로 시작하고 다음 낱말이 `CONSTRAINT`가 아닌 것(PG는 `DROP [COLUMN] [IF EXISTS] c`, `ALTER COLUMN c DROP DEFAULT`는 `ALTER`로 시작해 해당 없음). 표시 없는 것은 진단, 단 **재생성 모양**(D-93)의 `DROP TABLE t`는 여기서 통과시키고 적용 때 검사한다(아래). 재생성 모양은 7번 초안의 모양만: 같은 파일 앞에 `FROM t`로 **끝나는** `INSERT INTO x … SELECT … FROM t`(`WHERE`·`JOIN`이 있으면 행을 버릴 수 있어 표시가 필요하다, codex), 뒤에 `ALTER TABLE x RENAME TO t`. 표시(`Stmt.AllowDrop`)가 데이터를 지우지 않는 문장에 붙었으면 표시 위치의 진단. 이름 비교는 따옴표를 벗기고 대소문자 무시
- **적용**(`migrate.Apply(ctx, db, d, files) []diag.Diagnostic`): 파일 순서대로, 문장마다 `db.Exec`. 첫 실패에서 멈춘다(두 방언 같게, D-61). SQLite는 파일마다 `PRAGMA foreign_keys = OFF` → 문장들 → `PRAGMA foreign_key_check`(행이 나오면 그 파일 위치의 진단) → `ON`, 실패해도 `ON`. 연습용 DB는 트랜잭션을 열지 않는다(PG는 연습용 트랜잭션 안, SQLite는 자동 커밋). 10번 `migrate`는 `OFF`를 반드시 `BEGIN` 앞에 둔다(트랜잭션 안의 `PRAGMA foreign_keys`는 무시된다, codex가 확인). 모든 파일에 하므로 자식 행이 있는 `DROP TABLE parent`의 오류가 문장이 아니라 파일 위치의 `foreign_key_check`로 옮겨 간다(조용하지는 않다, fable). 10번 본문의 "재생성이 있는 파일만"을 이것에 맞춰 고친다. SQLite만의 분기는 이 한 곳이다(ARCHITECTURE §2의 예외, 10번이 실제 DB에서 같은 순서를 쓸 때 `dialect/sqlite`로 옮길지 정한다)
  - 재생성 모양의 `DROP TABLE t`를 실행하기 직전(7번이 넘긴 것을 fable이 고침, D-93): `SELECT * FROM t LIMIT 0`의 컬럼(두 방언에 같은 한 줄)과 앞 `INSERT`의 `SELECT`와 `FROM` 사이 토큰에 나오는 이름(따옴표 벗김, 대소문자 무시, `*`면 전부)을 비교해 나오지 않는 옛 컬럼이 있으면 `DROP` 위치의 진단(`DROP TABLE "t" loses column "c", which the copy above does not select`, fix: 복사하거나 표시). `INSERT` 컬럼 목록으로 보면 이름 바꾼 재생성(`SELECT id, text`)을 거짓으로 잡는다. 표시가 있으면 검사하지 않는다. 한계: `SELECT a, NULL AS b`처럼 이름은 나오지만 값을 버리는 것
  - 적용 오류는 `practicedb.Exec`의 진단(문장 위치 + DB 메시지). PG는 오류의 `Position`(문장 안 1부터 세는 **글자** 번호)으로 칸까지: 루트 모듈은 pgx를 모르므로(D-90) `practicedb.ErrorPosition func(error) int`를 두고 `cmd/maago/main.go`가 `pgconn.PgError`로 채운다(pgx 모듈 안, 새 의존 없음). 글자를 바이트로 바꾸고 `Stmt.SQL`을 걸어 줄·칸을 센다. 루트에서는 시험하지 못하고 확인 4(`cmd/maago`)가 덮는다
- **단계**(`internal/build`, ARCHITECTURE §3의 자리에 맞춘다, fable): [읽기: `GoFiles`] → [해석: `model.Read`] → [검사: `model.Check`, `project.Database`, `ReadFiles`] → [검사: `CheckNames`, `CheckFiles`] → [방언 `DDL`: PG의 63바이트 검사, M1에서 넘어옴] → [DB 검사] → [생성]. `.env`·마이그레이션 `Split`·태그 오류가 서로 가리지 않는다(구현하며 고침: `.env`를 첫 단계에 두면 `.env`가 없는 것이 태그 오류를 가리고, `ReadFiles`를 해석 단계에 두면 닫히지 않은 문자열이 태그 오류를 가린다. 셋 다 서로 필요 없어 한 단계에). Go 문법 오류는 여전히 먼저다(모델을 그것에서 읽는다). 방언 표(`DDL`·`Read`·`Draft`를 방언마다, `cmd/maago/draft_test.go`의 표를 여기로)
- **DB 검사**: 9번이 쓸 조각을 지금 나눈다(fable): `applied(ctx, d, url, files) (*practicedb.DB, []diag)` = A를 열고 `Apply`(닫기는 호출자. 9번의 `--rename`·쓰기 전 재적용이 같은 A에), `modelSchema(ctx, d, url, ddl) (*schema.Schema, []diag)` = B를 열어 DDL을 적용하고 읽고 닫는다(실패는 maago 버그), 그리고 A 읽기 → `Diff` → `Diagnose`. 앞 것에 진단이 나면 뒤는 하지 않는다(연쇄, D-61). 10번은 실제 DB에서 트랜잭션·기록과 함께 따로 돈다(`Apply`는 연습용 DB 전용)
- **변경 → 진단**(`migrate.Diagnose(cs, pos func(table, column string) token.Position)`): 문구는 6번의 `Message`, fix `run maago makemigrations`. 위치는 `build`가 모델에서 주는 함수로: 컬럼 종류와 인덱스·FK·CHECK는 그 (첫) 컬럼의 필드, 모델에 그 필드가 없으면(지운 필드) 모델, 테이블 종류와 PK·STRICT는 모델, `DropTable`은 위치 없음. **딸린 변경은 접는다**(6번이 넘긴 것): 같은 테이블의 `DropColumn`이 지운 컬럼을 포함하는 `DropIndex`·`DropFK`·`DropCheck`, `AddColumn`이 더한 컬럼을 포함하는 `AddIndex`·`AddFK`·`AddCheck`는 진단을 내지 않는다(연쇄, D-61). 초안(7번)은 `Diff`를 그대로 쓰므로 영향이 없다
- 테스트 옮기기: 루트 `internal/build`에는 DB 전에 멈추는 것(`TestMaxlen`·`TestStagesStop`·`TestNoModule`·`TestNameClash`)과 확인 1, `TestGenThenCheck`·`TestRunsAreIndependent`는 `cmd/maago`로(`.env`와 `0001_initial.sql`=모델 DDL을 더한 예제, 두 번째 프로젝트에도 `.env`). `cmd/maago`의 `TestRun`에서 빈 모듈의 `gen`도 `.env`가 필요하다(D-64). M1 문구 "모델이 없는 프로젝트의 `gen`은 조용히 성공한다"에 `.env` 전제를 더한다. 테스트는 `t.Setenv("MAAGO_DATABASE_URL", …)`로 방언을 고르고, `.env`를 보는 확인 4는 환경 변수를 비운다
- 하지 않는 것: 적용 오류 뒤 계속 적용하기, 파일 머리(D-94), 읽기 진단을 마이그레이션 파일 위치로 옮기기(DB에서 읽은 것은 문장을 모른다), `lock_timeout`(대상 이름의 스키마를 막아 다른 세션의 테이블을 잠글 길이 거의 없다), 모델의 `maago_` 테이블 이름 거부(→ TODO: 읽기가 `maago_migrations`를 건너뛰고 초안이 `maago_new_*`를 쓴다, fable)
- 리뷰: codex(`set_config`·`DO`, 10번과 `PRAGMA` 순서, 스키마 이름, `CONCURRENTLY` 자리, 재생성은 초안 모양만) 반영. fable(대상 자리의 스키마 이름, D-93은 `SELECT` 목록으로, 단계 자리, 9·10번 조각, 방언 표, `.env`가 필요한 테스트, `SET` 전부, `OR REPLACE`·뷰, 진단 개수 단언) 반영. `PRAGMA` 한 벌을 `dialect/sqlite`로 옮기는 것은 10번에서(위) 구현 리뷰: ponytail(`dialect.Name` 하나로, 패키지 밖에서 안 쓰는 이름 소문자로, `word`를 다시 쓰기, 테스트 도우미 줄이기) 반영. codex(`foreign_key_check`의 오류, `AS` 뒤 별칭은 복사로 치지 않음, FK 검사와 한글 뒤 위치 테스트) 반영. 받지 않은 것: PG의 재생성 모양 → 잃는 컬럼 검사가 두 방언 모두에서 돌아 데이터가 조용히 사라지지 않는다, `OFF` 실패 때 `ON` 복구 → 바뀐 것이 없다, 겹침 fix의 번호 → 마지막+1이 맞다(예시는 빈틈 진단이 따로 난다). 테스트는 샌드박스로 못 돌림. fable(두 문장 트리거 본문의 `END`, `RENAME` 없는 모양·줄 넘긴 PG 위치·FK 검사가 테스트되지 않음, 초안이 검사를 통과하는 테스트, 겹침과 빈틈을 따로, DDL 실패는 maago 버그, `len(ds) == 1`) 반영. D-93은 문구가 SQLite 재생성이지만 예외와 검사는 두 방언에 같다 → 12번 문서 갱신에서 D-93을 "두 방언"으로

→ 확인
1. 루트 모듈(`go test ./internal/build/ ./internal/migrate/`): 드라이버 없는 `Check`(`.env`에 SQLite)가 D-64 진단(위치 없음, fix 있음)으로 실패하고 아무것도 쓰지 않는다. PG 주소에 63바이트가 넘는 테이블 이름은 연결 전에 진단. `.env` 오류와 마이그레이션 문법 오류(닫히지 않은 문자열)가 태그 오류와 한 번에 나온다. 문장 검사 골든: 막는 것 각각 진단 하나(트리거 뒤 `END`는 하나로), 스키마가 붙은 대상 각 자리, `pg_catalog.lower(x)` 식과 따옴표 친 `"concurrently"` 컬럼은 통과, 표시 없는 `DROP TABLE`·`DROP COLUMN`·PG `DROP c`, `DROP CONSTRAINT`와 `ALTER COLUMN … DROP DEFAULT`는 통과, 재생성 모양은 통과하고 `WHERE`가 붙으면 진단, 다른 문장의 표시. 파일 이름·번호 골든(빈틈, 겹침 둘 다, 잘못된 이름). `Diagnose` 표 테스트: 위치와 접기
2. `cmd/maago` 모듈, 두 방언: 모델 DDL을 `0001_initial.sql`로 → `check` 0. 그 초안을 한 곳씩 손으로 바꾼 표(완료 조건 2): 컬럼 삭제, 타입, NOT NULL, 기본값, CHECK 완화(`max` 삭제, 선택지 하나 더), FK 삭제·`ON DELETE CASCADE`, unique 삭제, index 컬럼 순서, `AUTOINCREMENT`·IDENTITY 삭제. 각각 종료 1이고 진단의 **개수**와 문구·모델 필드 위치가 기대와 같다
3. 표시(완료 조건 3): 표시 없는 `DROP` → 그 문장 위치. 표시가 다른 문장에 붙은 경우(진단 둘), `DROP` 둘 중 하나만 표시한 경우. SQLite 재생성(7번 초안)의 `DROP TABLE`은 표시 없이 통과하고, `SELECT` 목록에서 컬럼 하나를 빼면 `DROP` 위치의 진단, 이름 바꾼 재생성(`SELECT id, text` → `body`)은 통과(D-93)
4. 문법 오류 문장은 그 줄을 짚고 PG는 칸까지. 번호 빈틈은 파일 위치의 진단. 하위 디렉터리에서 돌려도 루트의 `.env`를 읽고, `.env` 오류면 아무것도 쓰지 않는다
5. 두 `0003`은 두 파일 위치의 진단, fix는 다음 번호로 바꾸고 `maago check`
6. 모델을 바꾸고 마이그레이션을 두면 `gen`이 1이고 fix가 `run maago makemigrations`, 아무것도 쓰지 않는다

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
- `migrate`: 적용되지 않은 파일을 번호 순으로, 파일마다 트랜잭션 하나. PG는 그 트랜잭션에서 `SET LOCAL standard_conforming_strings = on`(연습용 DB와 같게, 3번이 문자열을 그렇게 읽는다). 그 파일의 문장과 `maago_migrations` 기록 삽입이 같은 트랜잭션이다. 실패하면 그 파일(기록 포함)을 되돌리고 멈춘다(앞 파일은 남는다), 종료 1과 `파일:줄`
- SQLite의 `PRAGMA foreign_keys`는 트랜잭션 안에서 바뀌지 않는다. 모든 파일을(8번 계획, 연습용 적용과 같게) 같은 연결에서 `foreign_keys = OFF` → `BEGIN` → 문장들 → `PRAGMA foreign_key_check`(행이 나오면 롤백) → `COMMIT` → `foreign_keys = ON`. 실패해도 ON으로 돌린다. 연습용 DB의 적용(8번)도 같은 순서
- 적용 기록에 있는데 파일이 없거나, 기록보다 앞 번호의 파일이 적용되지 않았으면 진단. 기록의 이름과 다르지만 해시가 같은 파일이 있으면 번호만 바뀐 것(브랜치를 합치며 다시 매긴 경우)으로 보고, fix에 기록의 이름을 고치는 `UPDATE maago_migrations …` 문장을 적는다
- `maago_migrations`에 파일 내용의 SHA-256을 함께 기록한다. 적용 뒤 바뀐 파일은 `migrate`·`showmigrations`의 진단(파일 위치, fix: 되돌리거나 새 마이그레이션으로)이고, `migrate`는 아무것도 적용하지 않는다(D-95)
- `showmigrations`: 파일마다 `[x]`/`[ ]`, 마지막 줄에 적용된 마지막 번호와 파일의 마지막 번호. DB는 읽기만
- 하지 않는 것: 롤백(D-39), `--fake`, 특정 번호까지만 적용

→ 확인 (`cmd/maago` 모듈, 두 방언): 적용한 파일의 주석 한 글자를 바꾸면 두 명령 모두 1. 적용한 `0003_add_y`를 `0004_add_y`로 바꾸면 진단의 fix에 기록을 고치는 문장이 있고, 그것을 돌리면 통과. 빈 DB에 `migrate` → 모두 적용, `showmigrations` 전부 `[x]`, 다시 `migrate`는 아무것도 하지 않고 0. 중간 파일의 마지막 문장이 실패하면 그 파일의 앞 문장(DDL·데이터)과 기록이 함께 되돌려지고 앞 파일만 남는다. 기존 행이 있는 SQLite 테이블의 재생성 뒤 행과 FK가 그대로이고, FK를 깨는 재생성은 롤백되며, 어느 경우든 끝난 뒤 `foreign_keys`가 ON

## 11. CLI와 CI

- `makemigrations`(`--name`, `--rename` 반복 가능), `showmigrations`, `migrate`를 명령 표에 더한다. `flag` 디스패치, 종료 코드 D-68
- 드라이버 blank import(D-89)와 CI의 PostgreSQL 서비스·`MAAGO_TEST_PG_URL`(D-91)은 2번이 이미 했다. 로컬에서 건너뛴 PG 테스트는 `-v`에 이유가 보인다
- M1의 스모크 스크립트(`internal/dialect/*/testdata/smoke.sh`)의 경우들을 `cmd/maago`의 Go 테스트로 옮기고 스크립트를 지울지 계획에서 정한다(M1에서 넘어옴)

→ 확인: `cmd/maago`의 `go mod tidy -diff`, 두 모듈 `go vet`·`go test`, CI에서 PG 테스트가 실제로 돈다(건너뜀 0, CI에서 변수를 빼면 실패). M2의 완료 확인은 PG를 건너뛴 로컬 결과로 대신하지 않는다

## 12. 완료 조건 종단 테스트와 문서

- 완료 조건 세 개를 그대로 `cmd/maago` 모듈의 테스트로: 빈 임시 프로젝트에서 모델 추가 → 필드 추가 → 필드 이름 변경(`--rename`) → 필드 삭제(표시를 단다), 단계마다 `makemigrations` → `check` 0, 끝에 새 DB에 `migrate`한 결과를 읽어 모델 DDL을 적용해 읽은 것과 같다. 두 방언
- ARCHITECTURE(§2 `schema`·`practicedb`·`migrate`의 실제 모양과 `project`의 `.env`·방언 이름, §3 DB 검사 단계), TUTORIAL-FUTURE §4(초안 모양, 가정 목록 4 지우기), TODO(`PRAGMA` 항목 닫기, 미룬 것 더하기) 갱신

→ 확인: 위 테스트가 두 방언에서 통과하고, 단계 하나를 빼거나(`--rename` 없이, 표시 없이, 초안 한 줄을 손으로 바꿔) 돌리면 해당 완료 조건이 실패한다

---

## M2 끝

- [ ] 1~12 완료
- [ ] 위 M2 완료 조건 세 개를 실제로 돌려 확인
- [ ] MILESTONES.md의 M2 체크박스 갱신

## 열린 질문 (계획 단계에서 정하거나 사람에게 묻는다)

- 재생성으로 사라지는 컬럼을 알아내는 구체적인 방법(7·8번, D-93)
- 속도(D-65): 매번 연습용 DB 둘에 전체 마이그레이션을 적용한다. 보통 규모에서 PG 3초를 넘기는지 M2 끝에 한 번 잰다

## 초안 리뷰 (2026-09-26)

codex: 16건 모두 반영. 연결 하나(`*sql.Conn`)에 고정, `PRAGMA foreign_keys`의 트랜잭션 경계, 재생성 `DROP`의 판정 규칙 제안(7번), PK의 자동 생성 방식을 표현에, SQLite 카탈로그의 `origin`·`hidden`·식 인덱스·여러 컬럼 FK, `CREATE TABLE`을 최상위 쉼표로 나눠 CHECK를 제자리에, 달러 인용 태그, PG 조회의 스키마 조건과 FK를 카탈로그 열로, `makemigrations`는 차이를 허용하는 모드, 적용 함수를 2번으로 당겨 7번이 8번을 기다리지 않게, 표시는 다음 문장 하나, 기록 삽입도 같은 트랜잭션, 읽기 자체 테스트, 남은 PG 스키마(열린 질문), 로컬 skip을 완료 증거로 쓰지 않음, 손으로 바꾼 초안의 돌연변이 표(8번)

사람의 결정(2026-09-26, 초안 리뷰 뒤): allow-drop은 다음 문장 하나·모든 파일(D-37 고침), 재생성 `DROP`은 파일 모양으로(D-93), 이름은 변경에서 자동·`--name`(D-94), 내용 해시(D-95), 남은 PG 스키마는 오류로 알림(D-96, 2026-09-27 트랜잭션 안에서 롤백으로 고침)
Alembic의 head·base(2026-09-27): 번호가 한 줄로 이어져 head는 마지막 파일, base는 빈 DB라 따로 두지 않는다. head가 둘인 경우(브랜치마다 만든 같은 번호)는 8번의 번호 겹침 진단과 10번의 번호 바뀜 fix로, 마지막 번호는 `showmigrations`로 본다
