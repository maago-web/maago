# M8 작업 목록 — 튜토리얼 검증: 게시판

[MILESTONES.md](MILESTONES.md)의 M8을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).

**M8 완료 조건** (MILESTONES)
- 원본 89개 커밋이 모두 대응표에 있고(대응하지 않는 것은 이유와 함께), 대응한 것은 튜토리얼의 커밋으로 있다 → 6번(표), 3~5번(빈 곳)
- 커밋을 하나씩 걷는 스크립트로 첫 커밋부터 마지막까지 `maago check`와 테스트가 통과한다(PostgreSQL로 바꾼 뒤는 PG에서) → 2·3번
- 일부러 틀리는 커밋마다 기대한 오류 메시지가 나온다 → 이미 `Maago-Expect`가 걷기에서 강제(D-103), README에 찾는 법 한 줄(7번)
- 에이전트가 튜토리얼을 처음부터 따라 하는 확인은 M9 게이트의 셋째 항목과 함께 한다 → M9

**앞에서 넘어온 것**
- TODO M4: 탈출구 뷰의 경로 값은 URL 함수에서 `string`이라 템플릿 검사가 정수를 거절한다. "M8에서 튜토리얼을 PG로 다시 묶을 때 검사기를 고치고 0016의 `print`를 지운다" → 1번(0028의 `(print .ID)`도)
- TODO 첫 릴리스 전: 튜토리얼도 `replace` 대신 태그한 버전 → M8 밖(릴리스, 태그는 사람의 일)
- MILESTONES M8 할일: 배포(단일 바이너리, 설정, 컨테이너 이미지 예시) → 5번

---

## 먼저 확인한 사실 (2026-10-13, 리뷰가 원본과 코드로 다시 봄)

- **원본**(`puddingcamp/release-your-product-with-fastapi`)은 커밋 89개다. 게시판은 지금 0001~0028
- **대략의 대응**(6번에서 한 줄씩): 모델 셋(4~6) → 0002, Alembic(8·9) → `makemigrations`(설정 없음), pytest·conftest(10·16) → `maagotest`, 회원가입·로그인·로그아웃·탈퇴·해싱(20~26·28·29) → 0019·0020(복사한 auth가 준다), JWT(23) → 세션과 API 토큰(D-129), 캘린더 CRUD·중복(30~34·42) → 0008·0011, 목록·상세(43·45) → 0008, 타임슬롯·겹침·과거 막기(36~38·41) → 0011·0012, 상태 필드와 변경(47·48) → 0003·0013, 취소(49) → 0022, 파일 업로드(50~53·68) → 0028, SQLAdmin(55~58·60·63) → 0023·0024, 정적 파일(61) → 0025, CORS·API·프런트엔드(62·64·66·67·69) → 0015, 헤더 토큰·토큰 없음(65·70) → 0021, 구글 캘린더·BackgroundTasks(71~76) → 0017, Streaming(77·78) → 0016, Sentry(79) → `Config.OnError`(0008), `/app`(80·81) → 0018. 대응 없음: 59(이미지 경로)·73(스타일 리팩터)·82~89(pytest 설정·ORM의 lazy·unique()·프런트엔드 URL)와 리팩터·테스트 고침
- **원본 39는 바꾸기가 아니다**: 코드의 방언 분기(`is_sqlite`면 다른 SQL)다. 원본은 끝까지 SQLite로 시험한다
- **빈 곳**: (a) PostgreSQL로 바꾸기(39의 자리), (b) 내 것: 게스트의 자기 목록(44)·`@me`(24), (c) 글 고치기(35 캘린더 변경·46 부킹 변경), (d) 배포
- **걷기는 SQLite뿐이다**(`walk`가 `.env`를 `sqlite:board.db`로 쓴다). `.env`는 패치 밖(D-103). `expected()`는 `Maago-Expect`·`Maago-Run`만 알고 모르는 `Maago-` 트레일러를 거절한다. `steps.sh unpack`도 `Maago-Run`만
- **`maagotest`·`maago migrate`는 `MAAGO_DATABASE_URL`(`.env`)만 읽는다.** `MAAGO_TEST_PG_URL`은 이 저장소의 PG 시험만 읽고 CI가 그것만 준다. `build.Migrate`는 URL로 바로 연다: 같은 서버의 `public`을 다른 시험과 함께 쓰지 않으려면 걷기가 스키마를 따로 만든다(`maagotest`의 `search_path` 꼴)
- **0012 뒤의 마이그레이션은 넷뿐**(0014의 hide_posts, 0019의 accounts, 0021의 post_author, 0028의 attachments). 게시판 쿼리에 방언에만 있는 SQL은 없다(`LIKE '%' || :q || '%'`는 둘 다 된다)
- **복사한 auth는 패치 밖**(D-103, `steps.sh`의 `info/exclude`, README "단계는 `auth/`를 손으로 고치지 않는다"): 단계가 `User`에 필드를 더할 수 없다
- **환경 변수**: 패치에 있는 것은 `MAAGO_DATABASE_URL`·`BOARD_WEBHOOK_URL`·`BOARD_WEBHOOK_TOKEN`·`MAAGO_SUPERUSER_*`·`BOARD_SITE_URL`(0020). 포트는 0018의 `:8000` 고정

## 결정 (2026-10-13, 초안 리뷰 뒤 "에이전트가 편한가"로)

초안의 Q1~Q5를 codex·fable 리뷰 뒤 정했다. 자세한 것은 DECISIONS.

- **D-141 PostgreSQL로 바꾸기**: 원본 39의 자리(0012 뒤, MILESTONES가 정한 대로). 그 단계는 SQLite의 마이그레이션을 지우고 `makemigrations`로 PG의 첫 초안을 새로 쓴다(데이터는 옮기지 않는다: 새 DB에서 시작하는 연습, D-31). 트레일러 `Maago-Database: postgres`가 걷기와 `steps.sh unpack`의 `.env`를 그 단계부터 PG로(모르는 값·두 번은 오류). 걷기는 `MAAGO_TEST_PG_URL`의 서버에 걷기마다 스키마를 만들어(`search_path`, 끝에 지움) `.env`의 `MAAGO_DATABASE_URL`로 쓴다. 로컬에 PG가 없으면 그 단계 앞에서 건너뛰고(이유), `CI`면 실패(D-91). 뒤 단계의 마이그레이션 넷은 PG로 다시
- **D-142 더할 단계 셋**: `Me`(`/me` 내 글 목록, `GET /api/me` 토큰으로. 복사한 auth는 손대지 않는다, 커스텀 User 필드는 메시지의 한 문단으로), `Edit a post`(작성자·staff), `Ship it`(마지막: 바이너리 하나, 환경 변수, `PORT`(기본 8000), `Dockerfile` 예시: 다단계·`CGO_ENABLED=0`·`gcr.io/distroless/static`·`media/` 볼륨. 걷기의 확인은 `CGO_ENABLED=0 GOOS=linux go build`와 이미 있는 `-createsuperuser`의 기동. Docker는 돌리지 않는다)
- 대응표는 `examples/board/MAP.md`, 89줄 한 줄씩(원본 커밋 → 게시판 커밋 → Maago 기능 → 마일스톤, 대응 없음은 이유). "복사한 auth가 준다"와 "게시판이 시험한다"를 가른다. 원본 13·14·18(공개 프로필)은 대응 없음(작성자는 글에), 27(계정 변경)은 복사본의 비밀번호 바꾸기(이메일·이름 바꾸기는 v0에 없음), 39는 "같은 자리에 D-31의 답(분기 없이 하나로)"

## 작업

1. **탈출구 뷰의 정수 경로 값**(TODO M4): 템플릿 검사가 탈출구 라우트의 `{{url}}`에 정수를 받는다(런타임 `URLFunc`은 이미 받는다). → 확인: `internal/web`의 시험, 0016·0028의 `(print …)`를 지운 뒤 `TestBoardTutorial`

   계획
   - `internal/web/typecheck.go`: `{{url}}`에 넘기는 라우트 표에서 탈출구 뷰의 와일드카드 타입을 `string or int`로(뷰는 `r.PathValue`로 글을 읽고, `URLFunc`은 글과 `int`·`int32`·`int64`를 받는다). 생성되는 Go의 `XxxURL(id string)`은 그대로(`view.wildcards`는 손대지 않는다) → 확인: 아래 시험
   - `internal/web/walk.go`의 `fits`: 타입마다 받는 기본 종류의 표 하나로, 상수도 그 표로(`string or int`는 글·정수 상수 둘 다). 오류는 지금 꼴 그대로 `{tag} of route feed is string or int, not time.Time`. 정수는 `int`·`int32`·`int64`만(`URLFunc`과 같다, 이름 붙은 타입·`uint`·포인터는 둘 다 거절)
   - `internal/web/templates_test.go`: 시험의 `feed` 라우트를 `/feed/{tag}`로, `TestTemplatesPass`에 `{{url "feed" 3}}{{url "feed" "go"}}{{url "feed" .Count}}{{.Count | url "feed"}}`, `TestTemplateErrors`의 url 줄에 `{{url "feed" .At}}`의 오류 → 확인: 고치기 전에 Pass가 실패하는 것부터
   - 게시판: `steps.sh unpack` → 0016·0028의 `(print …)`를 지우는 rebase(0016의 메시지 "In이 없으니 URL 함수의 인자도 문자열이다 … `(print .Board.ID)`"도 고친다) → `pack`. 뒤 패치의 맥락이 바뀌면 함께 지우는 것은 템플릿의 `(print …)` 둘뿐, 시험의 `fmt.Sprint(a.ID)`는 Go `XxxURL(string)`을 부르니 그대로 → 확인: `TestBoardTutorial`, `grep -c '(print' steps/*.patch`가 0, 바뀐 패치는 0016·0028뿐
   - TODO의 M4 줄 체크
   - 하지 않는 것: 탈출구 뷰의 Go `XxxURL`을 정수로(경로 값의 타입은 뷰가 `PathValue`로 정하니 생성기가 모른다)
   - 계획 리뷰는 fable 하나: 정해진 틀 안의 검사 몇 줄(WORKFLOW 3). 받음: 오류 글의 순서, 파이프 경우의 시험, `print` 지우기의 범위, 정수 종류를 계획에. 결정 변경 없음
   - 구현 리뷰는 fable 하나(계획 리뷰를 하나로 줄인 작업, WORKFLOW 6): 막는 지적 없음, `typecheck.go`를 고치기 전으로 되돌리면 시험이 실패하는 것을 실측. 받음: `slices.Clone`으로 줄이기. 받지 않음: `kinds` 표를 패키지 `var`로(세 줄, 차이 없음), 0016 메시지의 문구("Go 함수는 문자열, 템플릿은 정수도"를 이미 한 문장에)
2. **걷기의 PG**(D-141): 트레일러 `Maago-Database`(`expected`, `steps.sh unpack`), 걷기마다 스키마, 로컬은 건너뛰고 CI는 실패. → 확인: `TestBoardTutorialPostgresTrailer`(모르는 값, 두 번, PG 없이 `CI`면 실패)

   계획
   - `cmd/maago/tutorial_test.go`의 `expected`: 반환을 `trailers{want, apps []string; postgres int}`로. 정확한 줄 `Maago-Database: postgres`만 안다(`Maago-Run`처럼 닫힌 목록: 공백·대소문자·뒤 글자가 다르면 모르는 줄). 모르는 `Maago-` 줄의 오류 글에 `Maago-Database: postgres`를 더한다
   - `walk`: 단계마다 패치를 적용하기 **전에**(독자도 `.env`를 먼저 바꾼다) 이 순서로: 트레일러 세기 → 이미 PG거나 한 메시지에 둘이면 오류 `Maago-Database: postgres twice` → `MAAGO_TEST_PG_URL`(없으면 `CI`에서는 걷기의 오류, 아니면 `t.Skip`, 글은 `pgURL`의 것) → 스키마 `maagotest.SchemaName("github.com/maago-web/maago/cmd/maago board walk", t.Name(), 0)`를 `DROP SCHEMA IF EXISTS … CASCADE`·`CREATE SCHEMA`, `t.Cleanup`에서 `SET lock_timeout = '5s'` 뒤 지운다(`maagotest.schema`의 꼴) → `.env`를 `MAAGO_DATABASE_URL=<url.Parse 뒤 q.Set("search_path", 스키마)>`로 덮는다(서버 URL의 다른 쿼리는 남는다). `maago migrate`·끝의 `-createsuperuser`는 그 스키마에, `gen`·`check`의 연습용 DB는 자기 스키마(트랜잭션 안), 앱의 시험은 `maagotest`가 `search_path`를 자기 것으로 덮는다(`q.Set`)
   - 걷기 끝: PG면 `-createsuperuser`의 계정이 걷기 스키마의 표에 있는지 그 스키마를 이름으로 적어 센다(`search_path`가 끝까지 닿았는지의 확인) → 확인: 3번의 `TestBoardTutorial`(PG)
   - `steps.sh unpack`: 그 줄이 있으면 그 패치 앞에서 `.env`를 `MAAGO_DATABASE_URL=postgres://localhost:5432/board`로(unpack은 DB에 닿지 않는다. 시험 서버를 기본으로 두면 풀어 둔 곳의 `migrate`가 그 `public`에 쓴다). 두 번·모르는 값은 걷기가 잡는다
   - 시험: `TestBoardTutorialPostgresTrailer`, 0001에 트레일러를 넣은 표 셋(트레일러는 적용 전에 읽으니 앞 단계를 걷지 않고, 3번의 번호 밀림에도 그대로): `unknown database`(`Maago-Database: mysql` → `unknown trailer "Maago-Database: mysql"`), `twice`(두 줄), `no server in CI`(한 줄 → `MAAGO_TEST_PG_URL is not set`). 모두 `CI=true`·`MAAGO_TEST_PG_URL=`이고 건너뛰면 실패. 계획은 앞 둘을 `TestBoardTutorialCatches`의 줄로 했으나, 검사를 일부러 고장 내면 걷기가 PG 단계에서 `t.Skip`으로 빠져 SKIP이 되어 회귀를 못 잡았다(셋 다 돌연변이로 확인: 고치기 전엔 통과, 표로는 FAIL)
   - README(게시판): 트레일러 한 줄, "PG 없이는 걷기가 바꾸는 단계 앞까지만 본다(그 뒤의 Catches 줄과 CSS 비교도 건너뛴다)", 풀어 둔 곳의 `.env`는 마지막 단계의 것(PG)이라 앞 단계에서 `gen`을 돌리면 PG로 검사된다
   - 하지 않는 것: 지금 단계에 트레일러를 넣는 것(3번). PG 연결의 미리 확인(스키마 만들기의 오류가 그 글). 이 2번만으로는 PG 경로(스키마·`.env`·PG의 migrate)를 걸어 보지 못한다: 3번의 `TestBoardTutorial`이 처음 잰다
   - 계획 리뷰 codex·fable(뒤 작업이 기대는 약속). 받음: 검사 순서, `q.Set`, 끝의 스키마 확인, 정확한 줄, `lock_timeout`, unpack의 고정 주소와 README, Catches 줄을 0001에·`by`를 값까지. 받지 않음: 같은 서버에서 두 걷기가 동시에 돌 때의 스키마 충돌(codex. 이름을 고정해야 죽은 실행의 것을 다음 실행이 지운다: `maagotest`와 같은 맞바꿈), `maagotest`가 `search_path`를 덮는지의 시험(`q.Set`이 덮고 `maagotest`의 몫), 걷기마다 데이터베이스(fable 선택, D-141이 스키마)
   - 구현 리뷰 codex·fable: 막는 지적 없음(codex는 샌드박스로 시험을 못 돌려 읽은 리뷰). fable이 PG로 0001을 걸어 스키마·`.env`·지우기를 실측. 받음: unpack에서 `.env`를 `maago add` 앞으로(시험 드라이버를 `.env`로 고른다, D-123), 확인 줄, 표 시험의 앵커 확인. 받지 않음: `pgURL`과의 중복(CI 경우를 걷기의 오류로 돌려야 표가 된다), `maagotest.schema`와의 중복(그쪽은 `maago.OpenDB`·모듈 경로에 묶임)

3. **PostgreSQL로 바꾸는 단계**(D-141): 0012 뒤 새 단계(트레일러만이 아니라 앱 시험의 드라이버 import·`go.mod`/`go.sum`도 pgx로, 버전은 이 저장소가 받은 것: 걷기는 `GOPROXY=off`), 마이그레이션 넷 다시, 패치 번호 밀림(`TestBoardTutorialCatches`의 패치 이름, README·MILESTONES의 번호). → 확인: `TestBoardTutorial`(PG), 0001~0012는 바이트 그대로

   계획 (풀어 둔 곳에서 먼저 해 봄: 새 단계와 뒤 16단계의 cherry-pick이 충돌 없이, 새 단계에서 gen·check·migrate·시험이 PG로 통과)
   - 새 단계 `0013-Move-to-PostgreSQL.patch`: `main.go`·`board/board_test.go`의 드라이버 import를 `github.com/jackc/pgx/v5/stdlib`로, `go.mod`(직접 require 둘: pgx v5.11.0·maago, `modernc.org/*`는 빠짐)·`go.sum`은 `go mod tidy`의 것, `migrations/0001~0004`를 지우고 `maago makemigrations`가 쓴 PG의 `0001_initial.sql`. 메시지: `.env`의 새 주소(패치 밖), 드라이버, 마이그레이션은 방언의 것이라 새로 쓰고 데이터는 옮기지 않는다(D-31), 모델·쿼리·시험은 그대로, 트레일러 `Maago-Database: postgres` → 확인: 걷기가 이 단계를 PG로 통과
   - 뒤 단계의 마이그레이션 넷을 PG로 다시(`makemigrations --name` 같은 이름): `0005_hide_posts`→`0002`, `0006_accounts`→`0003`, `0007_post_author`→`0004`(SQLite의 표 다시 만들기가 `ADD COLUMN`·FK 하나로), `0008_attachments`→`0005`
   - 메시지 고침: 마이그레이션 번호(옛 0014·0019·0021의 "0005를"·"0006에"·"(0007)"), 옛 0019의 "여기는 SQLite라 modernc.org/sqlite" → PG·pgx, 옛 0021의 표 다시 만들기 문장, 옛 0024의 "(SQLite는 ASCII 글자만 무시한다)"를 지움, 다른 단계를 부르는 번호 +1(옛 0021의 "0014의", 0022의 "0014에서"·"(0021)", 0024의 "0020의"·"(0022)")
   - `steps.sh pack`: 0013~0028이 0014~0029로 이름이 바뀐다 → 확인: `git diff --stat`에서 0001~0012가 바뀌지 않음
   - `cmd/maago/tutorial_test.go`의 `TestBoardTutorialCatches` 패치 이름 넷 +1. README(게시판)의 `board.db` 줄을 방언 없이, MILESTONES의 `0019~0022` → `0020~0023`
   - → 확인: PG로 `TestBoardTutorial`·`TestBoardTutorialCatches`·`TestBoardTutorialPostgresTrailer`, PG 없이 로컬은 0013 앞에서 SKIP, `CI=1`이고 PG 없으면 `0013-Move-to-PostgreSQL.patch: MAAGO_TEST_PG_URL is not set`으로 FAIL, `grep -il sqlite steps/00{13..29}*.patch`는 0013과 0020("SQLite 프로젝트라면")뿐
   - 하지 않는 것: 데이터 옮기기(D-141), 0001~0012의 메시지(그때는 SQLite가 맞다), TASKS-M8 "먼저 확인한 사실"의 옛 번호(그날의 기록, 대응표는 6번이 새 번호로), TODO의 체크된 "0016의"(그날의 기록)
   - 계획 리뷰는 fable 하나: 정해진 결정(D-141) 안의 튜토리얼 고침(WORKFLOW 3). 받음: 확인의 grep을 `-il`로(대문자 `SQLite` 문장을 놓친다. 남는 것은 0013과 0020의 "SQLite 프로젝트라면"), DECISIONS D-129의 "게시판 0015" → 0016, 제목에서 "(draft)" 떼기, 메시지에 D-번호 대신 말로(공개 저장소에는 DECISIONS가 없다), `go get` 없이 `go mod tidy` 하나, 0022 문구(열·인덱스·FK), `TestBoardTutorialCatches`에 0013 줄(0001_initial의 PG 열을 SQLite의 것으로 → `maago gen:`의 PG 구문 오류). 선택을 받음: pack 전에 PG로 걸어 보기(`TestBoardTutorial`로 함)
   - 구현 리뷰는 fable 하나(계획 리뷰를 하나로 줄인 작업, WORKFLOW 6): 막는 지적 없음, 스테이지된 것을 PG로 걸어 실측. 받음: 새 Catches 줄의 주석을 그 줄이 증명하는 것으로(방언은 가르지 못한다, PG로 가는 증명은 `TestBoardTutorial`의 끝 확인), 확인의 grep 사실, 0013 메시지에 `createdb board`
4. **`Me`와 `Edit a post`**(D-142): 단계 둘. → 확인: `TestBoardTutorial`, `TestBoardTutorialCatches`에 단계마다 한 줄(권한 검사 빼기)

   계획 (풀어 둔 곳에서 먼저 해 봄: 두 단계 모두 PG로 gen·check·`go test`가 통과)
   - 자리: 0029 뒤에 `0030-Me.patch`, `0031-Edit-a-post.patch`(앞 단계의 번호가 밀리지 않는다. `Ship it`은 5번에서 0032)
   - **0030 `Me`**(원본 44·24): `board/queries/my_posts.sql`(`MyPosts :many MyPost`, `author_id`의 글, 초안·예약도, 숨긴 글은 빼고, 게시판 이름과, 새것부터. 페이지 없음). 뷰 `Me(ctx, struct{}) (MePage, error)` 하나를 라우트 둘이 쓴다: `GET /me`(`me`, 템플릿 `me_page.html`)와 `GET /api/me`(`api_me`, `maago.API`), 둘 다 `maago.LoginRequired`. `MePage{Email, Posts}`: `auth.User`를 통째로 JSON에 넣지 않는다(비밀번호 해시). `base.html` 머리에 `내 글` 링크(클래스 없음: Tailwind 출력은 그대로)
     - 시험 `TestMe`: 앤의 초안·게시 글은 나오고 숨긴 글·밥의 글은 안 나온다(페이지와 JSON 둘), 로그인 없는 `/me`는 303, 토큰 없는 `/api/me`는 401과 `WWW-Authenticate: Bearer`, 토큰이면 200 JSON(`"Email":"ann@example.com"`)
     - 메시지: 한 뷰가 페이지와 JSON을 함께 하는 법, 사용자는 `maago.UserFrom`이 준다. 커스텀 User 필드 한 문단: `auth/models.go`의 `User`에 필드를 적고 `makemigrations`(복사한 앱은 내 코드다). 이 튜토리얼은 `auth/`를 패치에 넣지 않아 하지 않는다
     - Catches: `/me`의 `maago.LoginRequired`를 빼면 → `go test: `(로그인 없이 빈 목록의 200)
   - **0031 `Edit a post`**(원본 35·46): `GET`·`POST /posts/{id}/edit`(`post_edit`·`post_update`, `maago.LoginRequired`), 뷰 `EditPost`, 폼 `PostEditForm{Title, Body}`(`model=Post`). 고정·예약 시각은 고치지 않는다: 쓸 때 정한 것이고, 같이 고치면 겹침 검사가 제 글을 빼야 하고 지난 예약 시각이 `Validate`에 걸린다(메시지에 한 줄). 숨긴 글은 404, `mayChange`(작성자·staff, 아니면 403)는 GET에도. GET이면(`!f.Bound()`) 폼의 `Raw`를 글의 값으로. 같은 제목 검사는 `post_title_taken.sql`에 선택 파라미터 `skip_id? int64`와 조건 줄 `{skip_id} AND id <> :skip_id`(새 글은 nil 그대로: `savePost`는 바뀌지 않는다). 저장은 생성된 `UpdatePost`(`UpdatedAt`이 바뀐다). 글 화면에 `고치기` 링크(클래스 없음)
     - 시험 `TestEdit`: 앤 GET은 200과 채운 제목, 앤 POST는 303이고 글이 바뀜(`UpdatedAt`도), 제목 그대로 POST도 303(`skip_id`), 같은 게시판의 다른 글 제목은 200과 오류, 밥은 GET·POST 403, staff는 303, 숨긴 글 404, 로그인 없는 POST 401
     - Catches: `EditPost`의 `mayChange`를 빼면 → `go test: `
   - TASKS·README의 다음 작업 줄. 대응표(6번)는 이 번호를 쓴다
   - 하지 않는 것: `/me`의 페이지 나누기, 글 고치기의 API, 고친 글의 웹훅 알림, 고치기의 기록(누가 언제)
   - 계획 리뷰 codex·fable(권한 검사): 막는 지적 없음. 받음: 비로그인 GET edit의 303, 고치기 뒤 고정·예약 시각이 그대로인지, `/me`에 예약 글, 다른 게시판의 같은 제목은 303, Catches를 컴파일되는 돌연변이로 고정하고 실측, `UpdatedAt`은 `!Equal`, 시험은 `TestAuthors`의 표 꼴, 편집 화면의 제목과 `Form.Errors`. 받지 않음(TODO로, 사람에게 묻는다): 초안·예약 글의 상세(`ShowPost`)가 누구에게나 보인다(codex). `/me`는 작성자에게 제 글의 링크를 보일 뿐 접근을 넓히지 않고, 고치면 단계가 하나 더 생겨 D-142가 바뀐다(fable)
   - 구현 리뷰는 fable 하나: codex는 이 기기에서 Codex CLI가 없다며 돌지 않았다(계획 리뷰 때는 돌았다). 막는 지적 없음, 복사본을 PG로 걸어 돌연변이 8개 중 7개를 잡는 것을 실측(Catches 두 줄도). 받음: 안 잡힌 하나(`my_posts.sql`의 순서)를 `TestMe`가 순서로 보게(뒤집으면 실패를 확인), 패키지 `auth`를 가리는 매개변수 이름, `PostEditPage`의 키 있는 리터럴. 알아 둘 것: 같은 서버·같은 데이터베이스에서 걷기 둘이 동시에 돌면 스키마가 겹친다(2번에서 받지 않은 그 경우). 리뷰어에게는 다른 데이터베이스를 준다
5. **`Ship it`**(D-142): 마지막 단계. → 확인: 걷기의 교차 빌드
6. **대응표** `examples/board/MAP.md`(번호가 정해진 뒤). → 확인: 시험이 표의 게시판 커밋 열을 `steps/*.patch`와 맞춤(모든 패치가 한 번은 나오고, 표의 단계가 다 있음), 원본 줄은 89
7. **완료 조건과 문서**: "M8 끝"에 조건마다 시험 이름, README(일부러 틀리는 단계를 찾는 법 `grep -l Maago-Expect steps/*.patch`), MILESTONES의 M8 표·할일, TUTORIAL-FUTURE

---

## M8 끝

- [ ] 1~7 완료
- [ ] 위 M8 완료 조건을 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로), 조건마다 시험 이름
- [ ] MILESTONES.md의 M8 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M9 초안

## 초안 리뷰 (2026-10-13)

- codex: 받음 — `maagotest`·`migrate`는 `MAAGO_DATABASE_URL`만(걷기가 `MAAGO_TEST_PG_URL`을 `.env`로 옮긴다), 트레일러는 `expected`와 `steps.sh` 둘에, 걷기마다 PG 스키마, 건너뛰는 자리는 바꾸는 단계 앞, 대응표의 "따위"를 커밋마다, Q4에 권한 시험, 배포 시험이 약함(→ 교차 빌드와 `PORT`). 받지 않음: Q3의 이메일·이름 바꾸기(복사본을 고쳐야 한다: D-103), Docker 빌드 시험(새 외부 의존)
- fable(원본 89 커밋을 실측): 받음 — ★Q3의 `Nickname`은 복사본이 패치 밖이라 안 된다 → `Me`(원본 44·24), 원본 39는 바꾸기가 아니라 방언 분기, 비용은 "16단계 rebase"가 아니라 마이그레이션 넷과 번호, 걷기의 PG 스키마, 미배정 커밋 15개(42·43·45·59·62·66·67·70·73·82~89)와 79는 0008, Q4·Q5의 확인(Catches 한 줄, 교차 빌드), 베이스 이미지 하나(`distroless/static`), `BOARD_SITE_URL`은 0020에 있다, 대응표의 확인이 동어반복 → 패치와 맞춤, 대응표는 번호가 정해진 뒤, 2번을 셋으로(검사기·걷기·단계), 일부러 틀리는 목록은 README에 grep 한 줄. 받지 않음(선택): 일부러 틀리는 "SQLite 마이그레이션을 PG에서" 단계(PG 오류 글이 길다)
