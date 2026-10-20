# M11 작업 목록 — 안전: 읽기 정책과 권한

[MILESTONES.md](MILESTONES.md)의 M11을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).
근거는 [RESEARCH-AGENT-FIT.md](RESEARCH-AGENT-FIT.md)의 2·4·7번, 결정은 D-147·D-148과 D-154~D-156(2026-10-19).

**M11 완료 조건** (MILESTONES)
- unique 위반이 500이 아니라 그 폼의 필드 오류다: 동시 요청 경쟁에서 진 쪽도(D-119의 대가를 푼다)
- 정책을 선언한 모델의 표를 읽는 쿼리와 `GetX`는 정책을 쓰거나 쓰지 않는 까닭을 말해야 생성된다. 게시판의 "초안은 작성자·staff만"이 네 곳의 조건이 아니라 정책 둘이다
- 라우트마다(admin 포함) 역할별 기대 상태가 시험의 표에 있고, 표에 없는 라우트가 있으면 `go test`가 실패한다
- 게시판 튜토리얼이 이 셋으로 걷는다. v0.2.0을 `go install`한 에이전트 게이트가 단계 글만 보고 셋을 쓴다

**앞에서 넘어온 것**
- D-148의 뒤로 미룬 것 중 셋(사람의 결정, 2026-10-19: 안전부터)
- TODO M10의 게이트 지적 중 auth 시험 도우미(권한 표가 역할마다 로그인한 핸들러를 만든다: 3번과 함께)

---

## 먼저 확인한 사실 (2026-10-19)

- **생성기는 쿼리가 읽는 표를 다 모른다.** 연구(4번)의 "D-104·D-110의 출처로 이미 안다"는 틀렸다. 출처는 드라이버가 *결과 열*에 주는 것뿐(`practicedb.Column.Table`, 별칭이 아니라 실제 표 이름)이라, WHERE·`EXISTS`·`count(*)` 부분 쿼리에서만 읽는 표(`post_list.sql`의 `board_comment c`)는 보이지 않고, `UNION`이면 출처를 모두 버린다(D-110). `joins()`(`internal/mapper/result.go`)는 토큰 훑기로 `LEFT JOIN`의 표만 모은다. 정책에는 FROM·JOIN의 표(와 별칭)를 모으는 새 훑기가 필요하다
- **쿼리 머리 키는 셋**(`name`·`param`·`column`, `ref.QueryKeys`). 키 하나 더하기는 `ref`의 목록, `header.read`의 switch, `Query`의 필드, 검사 자리
- **생성된 읽기는 `GetX` 하나**(`SELECT 모든 열 FROM t WHERE id = ?`), `ListX`는 없다. admin은 따로 생성된 `Rows`·`Count`(`admin.go.tmpl`)로 읽는다
- **게시판의 "초안은 작성자·staff만"은 네 곳**: `post_list.sql`·`search_posts.sql`·`export_rows.sql`의 조건과 Go의 `maySee`. `post_detail.sql`은 `NOT hidden`만 보고 `postPage`가 먼저 `GetPost`+`maySee`를 부르는 것에 기댄다(빠뜨린 쿼리가 31단계를 산 누출과 같은 모양, D-147)
- **unique 위반은 지금 500**: `rtcore.Code`는 `Status`·`ErrNoRows`(404)·`ErrConflict`(409)만 안다. admin은 문서에 "500"이라 적는다. 게시판(`BoardNameTaken`·`PostTitleTaken`)과 auth 가입(`UserByEmail`)은 먼저 읽고 쓰는 경쟁 있는 검사. 드라이버 오류를 제약·필드로 옮기는 코드는 없다
- **두 드라이버의 unique 위반**: pgx는 `*pgconn.PgError`의 `Code`(`23505`)와 `ConstraintName`(D-86의 이름, `board_post_board_id_title_key`) 필드, modernc는 `Code()`(2067 `SQLITE_CONSTRAINT_UNIQUE`)와 글 `UNIQUE constraint failed: board_post.board_id, board_post.title`(열 이름, 제약 이름이 아니다). 런타임은 stdlib만(D-09)이라 런타임이 드라이버 타입을 볼 수 없다. 생성 코드는 사용자 모듈에 있고 그 모듈은 드라이버를 이미 require한다(`main`의 import)
- **라우트**: 생성기는 프로젝트 패키지의 `var Routes`를 AST로 읽는다(복사된 auth 20개 포함). admin의 라우트 8개는 `admin.App`이 런타임에 만든다(`maago check`가 정적으로 보지 못한다). 게시판 마지막 단계 22개
- **역할별 시험은 이미 표 꼴**: 게시판 `TestDrafts`(0032)는 `{who, method, path, code}` 행과 `nobody`·`bob`·`ann`·`staff` 핸들러. 빠짐없는지는 아무도 보지 않는다. 로그인한 핸들러를 만드는 도우미는 게시판이 손으로 쓴다(`signedIn`, 게이트 둘 다 지어냈다)

## 질문과 답 (사람의 결정, 2026-10-19, 모두 추천)

초안 리뷰(아래)로 고친 추천. 모두 에이전트 편의로 골랐다. Q1·Q2·Q4·Q5는 사람이 추천대로 답했고(D-154~D-156), Q3·Q6은 리뷰 둘이 같은 쪽이라 정했다.

- **Q1 unique 위반의 길**: (a, 추천) 판정은 `rt` 한 곳에서 인터페이스로(`SQLState() string`이 `23505`, `Code() int`가 2067): 생성 파일도 런타임도 드라이버를 import하지 않는다. 어느 제약인지는 모델에 unique가 하나면 그것, 여럿이면 DB가 내는 글에서(PG 서버 글의 D-86 이름, SQLite 코어 글의 열 목록: 드라이버의 글이 아니다), 못 찾으면 폼 오류. 생성된 `InsertX`·`UpdateX`는 `maago.Tx` 안에서 `SAVEPOINT`로 감싸, 실패 뒤에도 같은 트랜잭션으로 다시 읽고 커밋한다(PG는 실패한 문장이 트랜잭션을 abort한다). 폼의 `f.Fail(err) bool`이 그 제약의 필드 중 폼에 있는 것이 정확히 하나면 그 칸에, 아니면 폼 오류에 `Config.Messages`의 새 문구를 쓴다 (b) 판정을 생성 코드가 드라이버 타입으로(방언마다 생성물이 다르고 생성 패키지가 드라이버를 import한다) (c) 그대로 두고 문서에. D-119 고침: "오류 글 읽기"의 기각을 "드라이버의 글"로 좁히고, DB가 내는 글을 읽는 두 자리를 적는다
- **Q2 정책을 쓰는 꼴**: (a, 추천) 앱의 `policies.sql`에 정책마다 이름·모델·별칭과 조건식 한 조각, 쓰는 매개변수는 `-- param:`으로 선언. 모델 하나에 정책이 여럿일 수 있다(게시판: 목록의 `out`, 상세·파일의 `visible`). 쿼리는 머리에 `-- policy: visible p`를 선언하고 본문에 `visible(p)`를 쓴다: 선언·사용의 쌍이라(`:param`처럼) 선언된 이름만 펼쳐지고 SQL 함수와 섞이지 않는다. 정책 모델의 표가 FROM·JOIN에 나오면 그 별칭마다 정책 하나나 `-- policy: none <까닭>`이 있어야 하고, 별칭이 그 모델의 표인지 본다. 펼치기는 조건 줄의 조합보다 먼저 (b) 머리만 쓰고 생성기가 WHERE에 끼워 넣는다(어느 WHERE인지 모호하다) (c) 식을 손으로 쓰고 이름만 확인(식이 여러 벌로 남는다)
- **Q3 정책의 매개변수**(리뷰가 고친 꼴, 묻지 않고 정함): 사용자의 열은 `:viewer_<필드>`(D-110의 이름 규칙), 생성 함수가 뷰와 같은 사용자 타입의 포인터에서 채운다(nil은 아무도 아님: 열은 NULL). 다른 매개변수(`:now`)는 그 정책을 쓰는 쿼리의 `Params`에 합쳐진다(쿼리가 다시 선언하면 오류). 정책을 쓰는 함수와 정책 모델의 `GetX`는 뷰와 같은 차례 `(ctx, u, db, …)`. 정책 없는 길은 이름에 드러나는 생성 함수 하나(이름은 계획에서), admin 생성 코드가 그것을 부른다. 사용자 타입을 추론할 뷰가 없으면 생성 오류
- **Q4 규칙의 수**: (a, 추천) D-24를 고쳐 여섯째 규칙("정책 모델의 표를 읽는 쿼리는 정책이나 `none`을 말한다"), PHILOSOPHY 3의 "다섯"도 (b) 머리 선언이라 규칙 1의 해석으로 두고 다섯 그대로(D-104·D-107의 길). 검사가 새로 하나 생기므로 (a)가 솔직하다
- **Q5 권한 표의 꼴**: (a, 추천) 표는 시험 안의 Go: `maagotest.Permissions(t, App, roles, rows...)`, 행은 라우트 이름·요청(fixture로 만든 URL과 몸통)·역할별 상태. 빠짐없음은 런타임이 본다: 앱의 라우트를 모두 꺼내(`rtcore.Parts`) 행 없는 라우트에 실패. 그래서 admin의 런타임 라우트도 표 안이다. 생성기는 라우트 있는 앱의 시험에 그 호출이 있는지만 본다 (b) 앱마다 `permissions.txt`를 `maago check`가 대조(fixture를 적을 수 없다: 같은 라우트가 초안·게시 글에 다른 상태, 리뷰 둘 다) (c) 생성된 빈 표. 완료 조건의 "`maago check`가 실패"는 "`go test`가 실패"로
- **Q6 게시판**(리뷰 둘 다 추천대로, 묻지 않고 정함): 새 단계를 더한다. 실제 업그레이드의 모양이고 앞 단계가 초록으로 남는다. 정책 단계에는 정책을 빠뜨린 쿼리의 `Maago-Expect` 단계를 하나 둔다(31단계를 산 누출이 생성 시점에 막히는 것을 튜토리얼이 보인다)

## 작업 (초안 리뷰 뒤)

순서: 정책과 독립인 안전 둘(unique, 권한 표)을 먼저 끝내고 정책을 뒤에 둔다. 정책이 미끄러져도 둘은 남는다. 권한 표가 먼저 있으면 정책이 바꾸는 상태(남의 초안 403 → 404)를 표가 드러낸다.

1. **unique 위반 → 필드 오류**(Q1): `rt`의 판정, `maago.UniqueError`, 생성된 `InsertX`·`UpdateX`의 savepoint, 폼의 `Fail`, 새 문구, admin이 500 대신 그 칸의 오류, auth 가입 → 확인: 두 방언에서 tx A가 제목을 쓰고 열어 둔 채 B가 같은 제목을 쓰면(타이밍 없이 unique 인덱스가 B를 막는다) A 커밋 뒤 B는 200과 그 칸의 문구이고 B의 트랜잭션이 다시 읽고 커밋까지 간다, 제약이 여럿인 모델과 폼에 없는 필드의 폼 오류, admin의 같은 시험, 런타임 import 허용 목록 그대로

   **계획**
   - **오류 타입**(`db.go` 옆, 루트): `type UniqueError struct { Fields []string; Err error }`, `Error`·`Unwrap`. Fields는 모델의 Go 필드 이름, 어느 제약인지 모르면 nil. 폼의 `Fail`이 받지 않으면 지금처럼 500(`rtcore.Code` 그대로)
   - **판정**(`rt/write.go` 새 파일): `type UniqueKey struct { Fields []string; Key string }`(Key는 DB가 오류에 쓰는 이름: PG는 D-86의 제약 이름, SQLite는 `table.col, table.col`)와 `Write(ctx, db, keys []UniqueKey, write func() error) error`. `write`를 savepoint 안에서 돌리고(아래), 오류가 `SQLState() string`이 `23505`이거나 `Code() int`가 2067이면 `*maago.UniqueError`로 싼다. 제약은 keys가 하나면 그것, 여럿이면 PG는 `Error()`의 글에 `"Key"`가 있는 것(제약 이름은 `PgError`의 필드뿐이고 메서드가 없다. 서버 글이 번역되어도 이름은 따옴표 안에 남는다), SQLite는 `UNIQUE constraint failed: ` 뒤에서 드라이버의 ` (2067)` 접미를 뗀 것이 Key와 같은 것, 못 찾으면 Fields nil. 판정은 `errors.As`로 인터페이스만 보므로 드라이버 import가 없다(`TestRuntimeImports` 그대로)
   - **savepoint**(`rt.Write`): db가 `maago.Tx`면 그 `ExecContext`로 `SAVEPOINT maago_write` → write → 실패면 `ROLLBACK TO`·`RELEASE`, 성공이면 `RELEASE`. Tx가 아니면(NoTx 뷰, admin, 시험의 `*sql.Conn`) write만. 되감기가 실패하면 원래 오류를 `%v`로만 담은 오류(UniqueError가 아니다: `Fail`이 삼키면 커밋의 500 까닭이 숨는다). 두 방언 같은 길: SQLite는 실패한 문장이 트랜잭션을 깨지 않지만 방언마다 다른 생성물보다 하나가 단순하다. 대가: Tx 안의 생성된 쓰기마다 문장 둘. 모든 오류에 되감는다(FK 위반 뒤에도 트랜잭션이 산다). 커밋 오류의 힌트에서 "check with a query before writing (a title taken, say)"를 "생성된 Insert·Update는 제 실패를 되감지만 손으로 쓴 쿼리는 아니다"로. `txState.failed`는 되돌리지 않는다(fable: 되돌림을 잡는 시험이 없고 그 때문에만 rtcore 훅이 필요하다. 그 뒤 커밋이 실패해도 "앞의 쿼리가 실패했다"는 사실이다)
   - **생성**(`internal/mapper/crud.go`, `models.go.tmpl`): 모델마다 `var uniqueX = []rt.UniqueKey{…}`(unique 없으면 `nil`: 손으로 만든 unique 인덱스는 Fields nil로 폼 오류. unique가 하나인 모델에서는 그것으로 친다, fable). 순서는 `m.Uniques`(DDL과 같은 열 순서, SQLite 글의 순서), 이름은 `dialect.UniqueName`. `InsertX`의 `Scan`과 `UpdateX`의 `rt.Exec`를 `rt.Write`로 감싼다. 버전 비교의 `Exists` 읽기는 밖. `DeleteX`는 그대로. gentest 골든이 바뀐다
   - **폼**(`internal/web`, `forms.go.tmpl`): 폼마다 `func (f *X) Fail(err error) bool { return rt.Fail(&f.Form, err, map[string]*string{"Title": &f.Title.Error, …}) }`. `rt.Fail`: `*maago.UniqueError`가 아니면 false. 그 Fields 중 폼에 있는 것이 정확히 하나면 그 칸의 `Error`, 아니면 `f.Errors`에 문구를 쓰고 true. 이름으로 맞추므로 모델 폼과 평범한 폼이 같다(폼 필드 이름이 모델과 다르면 폼 오류: 문서에 한 줄, fable). 폼이 `Fail` 메서드나 필드를 선언하면 `Valid`와 같은 생성 오류. 문구는 `rt.FinishForm(ctx, …)`가 `FormState`에 새로 남기는 요청의 설정에서(묶지 않은 폼은 기본값): `Fail(err)`에 ctx가 없어서. `FinishForm`은 생성 코드만 부른다
   - **문구**: `Messages.Taken`(기본 `"This value is already taken."`, 동사 없음), `rtcore.Messages`의 끝, `NewSettings`, `check`. 받지 않음: `%s`에 필드 라벨(fable): 폼 오류로 떨어지는 길의 하나는 제약을 모르는 것(Fields nil)이라 채울 라벨이 없다
   - **admin**(`admingen/form.go`, `contrib/admin`): 생성된 폼 struct에 `maago.Form`을 임베드하고 `FinishForm`, 저장(`InsertX`, update의 `rt.Exec`를 `rt.Write(…, uniqueX, …)`로)이 실패하면 `rt.Fail`이 받으면 오류 없이 다시 그린다. 그래서 입력(`in`)을 저장 뒤에 만든다(지금은 앞). `admin.Form.Errors`와 `form.html`의 폼 오류, `site.form`은 그대로(받은 오류는 nil로 온다). 문서의 "unique 값 둘은 500"을 지운다(지운 행을 가리키는 FK는 그대로 500)
   - **auth 가입**(`scaffold/testdata/auth/views.go`): 먼저 읽기를 지나 `InsertUser`가 `*maago.UniqueError`면 이미 있는 계정처럼 `Sent: true`로(메일 없음: 이긴 쪽이 제 메일을 받는다). `Fail`을 쓰면 그 메일의 계정이 있다고 알려 준다(가입은 계정이 있는지 숨긴다): 그래서 필드 오류가 아니다(fable 동의). 시험은 `INSERT INTO "auth_user"` 앞에 같은 메일을 먼저 넣는 `maago.DB` 감싸개로 경쟁을 만든다. 게시판 단계 중 auth의 views·forms를 고치는 patch는 없다
   - **문서**: D-119의 대가·기각을 D-154로 고침, `doc.go`·`maago doc form`의 `Fail` 한 단락(자세한 것은 8번)
   - 하지 않기로 한 것: API 뷰가 `Fail` 뒤 422를 돌려주는 길(TODO에 지금 쓸 길: `errors.As(err, &ue)` 뒤 `maago.Invalid`·`InvalidForm`), `UniqueError`의 409, 손으로 쓴 쿼리의 savepoint, raw `*sql.Tx`의 savepoint(`maago.Tx`만), 게시판(2번)
   - 계획 리뷰(codex·fable) 반영: SQLite의 접미, SQLite의 B는 `BEGIN IMMEDIATE`에서 기다린다(둘 다), 겹치는 제약 fixture(fable), admin fixture(fable), 되감기 실패(codex), `failed` 되돌림 빼기(fable), 이름 `UniqueKey`(fable). 받지 않음: PG 제약 이름을 구조로(codex: 필드뿐이라 reflect가 필요하다, D-09의 noreflect), `Fail`·`rt.Write`의 이름 바꾸기(fable: `Fail`은 D-154의 이름, `rt`는 생성 코드만 부른다)
   - 구현 리뷰: ponytail(`cmp.Or`, 겹친 if 접기를 받음. 받지 않음: unique 없는 모델에도 빈 `uniqueX`를 생성해 `Keys`를 없애기: 모든 생성 파일에 빈 변수가 생긴다), codex(결함 없음, 샌드박스 때문에 코드만 읽은 리뷰: PG의 abort 뒤 되감기는 `TestWriteRace`·`TestFormTaken`이 PG에서 돈다), fable(빠진 되감기 실패 시험을 더함, unique 하나인 모델의 뜻을 D-154·`maago doc form`에, API 뷰의 한 줄과 예의 `_`, 제약을 모를 때의 오류 글. 받지 않음: admin의 두 칸 폼 오류 끝까지 시험: 같은 `rt.Fail`이고 `TestFail`이 그 갈래를 본다)
   - → 확인
     - `tx_test.go`(두 방언, 실제 `OpenDB`): A가 `x=1`을 쓰고 열어 둔 채 B(다른 고루틴, `WithTx`)가 `rt.Write`로 같은 값을 쓴다. 주 고루틴은 B를 기다리지 않고 A를 커밋한다(SQLite의 B는 `BEGIN IMMEDIATE`에서, PG의 B는 INSERT에서 기다린다). B는 `*maago.UniqueError`, 같은 트랜잭션에서 읽고 다른 행을 쓰고 커밋, 두 행이 남는다
     - `rt` 단위 시험: 가짜 오류(`SQLState`·`Code`와 두 드라이버의 실제 글 꼴)로 제약 고르기, 접미, 앞이 같은 열 목록(`t.a`와 `t.a, t.b`), 못 찾으면 nil, 되감기 실패는 UniqueError가 아니다
     - gentest blog(두 방언, 실제 드라이버): `Author`의 서로 독립인 unique 둘(`EditorID`, `Code`)이 각각 그 Fields, `InsertX`·`UpdateX` 둘 다, `WithTx` 안의 실패 뒤 읽기와 커밋. `Post`의 둘은 겹쳐서(같은 slug는 두 제약을 다 어기고 어느 것을 내는지는 DB의 차례) 쓰지 않는다. admin: `Author`의 `EditorID`를 남의 것으로 바꾸면 500이 아니라 그 칸의 문구, 입력이 다시 그려진다
     - contrib/admin: `Form.Errors`가 그려진다
     - web fixture(HTTP, `TestWeb`의 SQLite와 `TestM4Done`의 PG): 새 모델 `Tag`(`Name` unique, `unique=(Group,Code)`)와 그 폼(`Name`·`Group`·`Code`)의 Tx 뷰. 같은 Name은 200과 Name 칸의 `Taken`, 같은 Group+Code는 둘 다 폼에 있어 폼 오류, 실패 뒤 같은 트랜잭션에서 읽기, `Config.Messages.Taken`, `Fail(nil)`·다른 오류는 false. `Fail` 선언 폼의 생성 오류. 폼에 필드 하나인 복합 제약은 rt 단위 시험과 게시판(2번, 두 방언). PG에서 savepoint를 빼면 이 시험과 `TestWriteRace`가 실패한다(확인함)
     - auth 시험: 경쟁에서 진 가입이 200 `Sent`, 메일 없음
     - 되감기 실패(`TestWriteSavepointLost`, 두 방언: 요청이 끊겨 `ROLLBACK TO`가 실패하면 UniqueError가 아니고 커밋하지 않는다. 검사를 빼면 실패함을 확인)
     - 회귀가 잡히는지: savepoint를 빼면 PG의 B가 커밋에서 실패, 판정 글을 바꾸면 Author 시험이 실패
     - `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG 포함)
2. **게시판 0034: unique**: 먼저 읽는 검사(`BoardNameTaken`·`PostTitleTaken`, 0031의 `skip_id`)를 지우고 `Fail` 하나로 → 확인: `TestBoardTutorial`, 같은 이름·제목의 시험이 Taken 문구

   **계획**
   - 새 단계 `0034-Let-the-database-refuse-what-is-taken.patch`(`steps.sh unpack`, 커밋 하나 더, `pack`). 0013부터 PG라 이 단계는 PG에서만 걷는다
   - 지운다: `board_name_taken.sql`, `post_title_taken.sql`(`skip_id?`와 함께)
   - `CreateBoard`: `InsertBoard` 뒤 `if f.Fail(err) { return maago.Show(…) }`. `savePost`: 고정 기간 겹침(제약이 없어 읽기로 남는다) 뒤 `InsertPost`, `if f.Fail(err)`이면 false. `Board.Name`은 unique 하나라 `Name` 칸, `Post`의 `(BoardID,Title)`은 폼에 `Title`만 있어 `Title` 칸. `ChangePost`: `UpdatePost`의 switch에 `case f.Fail(err):`(`ErrConflict` 다음). `APICreatePost`는 그대로: `savePost`가 쓴 `f.Title.Error`를 `maago.Invalid`로
   - 주석: `CreateBoard`의 "A taken name needs the database, so the view checks it", `savePost`·`ChangePost`의 "unique … as a 500"을 새 길로
   - 문구: 손으로 쓴 "이미 있는 이름입니다."·"이 게시판에 같은 제목의 글이 있습니다."가 기본 `Messages.Taken`("This value is already taken.")이 된다. 틀의 다른 문구(필수·길이)도 영어 기본이고 시험이 이미 그 글을 본다. 손으로 쓴 `Validate`·고정 겹침·충돌 문구는 한국어로 남는다. 시험 넷(`TestWrite` 둘, `TestAPI`의 422, `TestEdit`)이 Taken 글을 본다
   - **고침(사람의 결정, 2026-10-20)**: 경쟁 시험을 더한다. 게시판 `TestWriteRace`: 1번 틀의 `TestWriteRace` 꼴로, A(`WithTx`)가 이름을 쓰고 열어 둔 채 B(HTTP 요청, 다른 고루틴)가 같은 이름을 쓰고, A는 B를 기다리지 않고 커밋한다. B가 먼저 닿았든 나중이든 200과 Taken, 게시판은 하나. `pg_stat_activity`는 쓰지 않는다(아래의 기각 까닭 둘째는 그대로). savepoint를 끄면(`rt.Write`의 `inTx = false`) B가 500(확인함)
   - 시험은 순차 시험 그대로: 새 코드에서 B의 결과는 차례와 무관하다(A가 먼저 커밋했으면 바로, B가 잠금을 기다렸으면 깨어나며 제약이 거절 → `Fail`). 그래서 같은 이름·제목의 순차 시험이 곧 이 길의 시험이고, PG에서 savepoint가 빠지면 `Show`의 커밋이 실패해 500이다. 잠금 대기 경로는 틀의 `TestWriteRace`(두 방언)가 본다
   - `TestBoardTutorialCatches`에 한 줄: 0034의 `savePost`에서 `f.Fail(err)`를 `false`로 → `go test:`. `f.Fail(err)`는 패치에 셋이고 `Replace`는 첫째(CreateBoard)를 바꾸므로 `old`는 savePost에만 있는 줄(`if f.Fail(err) {`와 `return Post{}, false, nil`)
   - `MAP.md`: 0034 행(M11), 원본 34·42·46의 단계 칸은 바꾸지 않고 "0011·0034"·"0031·0034"처럼 둘 다(57행의 "0024·0025" 꼴)
   - 커밋 글: 0011이 적은 먼저 읽기의 경쟁(둘이 동시에 읽고 둘 다 쓴다 → 500)을 받아 이제 제약이 거절하고 `Fail`이 그 칸에, 트랜잭션이 이어진다(savepoint). 0011의 질문과 0031의 `skip_id`가 간다고 이름으로 지목하고(앞 단계 글은 고치지 않는다, 0027→0028 꼴), `skip_id`가 필요 없는 까닭: 제 제목을 그대로 두는 UPDATE는 같은 행이라 제약이 울리지 않는다. 고정 기간은 제약이 없어 읽기로 남고, 그래서 제목과 기간이 둘 다 겹치면 겹침이 먼저 보인다(받아들임). 틀의 문구는 영어 기본, 바꾸려면 `Config.Messages.Taken`
   - 걷기는 `go.work`로 로컬 모듈을 본다(`go.mod`는 v0.1.0, `Fail` 없음). 9번이 v0.2.0으로 올리고 10번 게이트가 `@v0.2.0`을 설치한다
   - 하지 않음: 고정 기간 겹침을 PG의 exclusion 제약으로(모델 태그에 없다), 문구를 한국어로 바꾸기(`main.go`와 `web_test.go`의 `config` 두 곳), `pg_stat_activity`로 B의 잠금 대기를 본 뒤 A를 커밋하는 경쟁 시험(순차 시험과 다른 것을 보이지 못하고, 게이트 에이전트에게 PG 내부를 가르친다)
   - 계획 리뷰(fable, codex는 결과 없이 멈춤) 반영: 경쟁 시험 빼기(사람의 결정), Catches의 `old`, 문구의 정확한 범위, 낡은 주석, 0011·0031 지목과 `skip_id`, 검사 순서, MAP의 두 단계 칸, v0.2.0 한 줄
3. **권한 표**(Q5): `maagotest.Permissions`, 역할마다 로그인한 핸들러를 만드는 도우미(TODO M10의 auth 시험 도우미), 행 없는 라우트(admin 포함)의 실패, 생성기의 "라우트 있는 앱에 표가 없다" 진단, auth 복사본의 표(20 라우트) → 확인: 라우트를 더하고 행을 잊으면 `go test` 실패, 틀린 기대가 실패, 표 없는 앱이 `maago gen` 오류
4. **게시판 0035: 권한 표**: 22 라우트와 admin, `TestDrafts`·`TestAuthors`의 겹치는 행을 표로 → 확인: `TestBoardTutorial`
5. **표를 읽는 쿼리 훑기**: FROM·JOIN(부분 쿼리·CTE·쉼표 조인·UNION의 각 SELECT)의 표 이름과 별칭. 정책 표와 같은 이름의 CTE(가림), SELECT 목록 안의 부분 쿼리. 읽는 표를 모를 모양은 정책 모델이 있을 때만 오류 → 확인: 게시판·auth·gentest 모든 쿼리의 골든, 숨긴 표(EXISTS·CTE·SELECT 목록 안)를 찾는다
6. **읽기 정책**(Q2·Q3·Q4): `policies.sql`, 선언·사용의 쌍, 별칭 검사, 펼치기 → 조합 → prepare, 정책 매개변수, 정책 모델의 `GetX`와 정책 없는 길, admin은 정책 없는 길 → 확인: 정책을 빠뜨린 쿼리(EXISTS 안 포함)·까닭 없는 `none`·모르는 `:viewer_x`·다른 모델의 별칭이 생성 오류, 아무도·남·작성자·staff가 같은 쿼리에서 다른 행, nil 사용자, 조건 줄과 `WHERE { }` 안의 정책 골든, 두 방언
7. **게시판 0036~0037: 정책**: 0036은 정책을 빠뜨린 쿼리(`Maago-Expect`), 0037은 `out`·`visible` 두 정책으로 `maySee`와 쿼리 넷의 조건을 접는다(`MyPosts` 포함). `RecentPosts`가 `none`의 본보기 → 확인: `TestBoardTutorial`, `TestDrafts`가 그대로 초록, 0035의 표가 바뀐 상태를 행으로 보인다, Catches의 "아무나 초안 보기"가 `go test`가 아니라 생성 시점에 잡힌다
8. **문서**: `maago doc`의 query(정책, 규칙)·form(`Fail`)·권한 표(maagotest 또는 view 주제), MISSION의 범위 문장(D-147), PHILOSOPHY 3, D-24·D-119·D-147 고침 → 확인: `TestDocExamples`(새 예), `TestPackageDoc`, `TestNoDecisionsInUserText`
9. **v0.2.0**: 태그, 게시판의 `go.mod`를 v0.2.0으로(0001·0013, 12번의 길) → 확인: 빈 GOPATH에서 `go install …@v0.2.0`, 공개 판으로 걷기의 끝
10. **에이전트 게이트**: `go install …@v0.2.0`부터, 0033의 끝에서 시작해 새 단계(0034~0037) 글만 보고 서로 모르는 실행 둘 → 확인: 기록, 셋을 메시지와 `maago doc`만으로 씀
11. **M11 끝**: MILESTONES의 M11 체크, README, 다음 마일스톤의 순서

## 초안 리뷰 (2026-10-19, codex와 fable)

- **PG의 abort**(둘 다): 실패한 INSERT 뒤 요청 트랜잭션이 abort라 `Fail` 뒤의 읽기와 커밋이 실패한다 → 생성 함수의 savepoint, 확인에 "다시 읽고 커밋까지"
- **판정의 자리**(둘 다): 생성 파일은 드라이버를 import하지 않는다(codex), 판정을 `rt`의 인터페이스로(fable) → Q1. SQLite의 열 목록은 글에서만 온다: D-119의 기각과 부딪힌다(둘 다) → 드라이버 글이 아니라 DB가 내는 글, 제약이 하나면 글을 읽지 않는다
- **복합 unique**: 마지막 필드는 위치 짐작(fable), 모든 필드에(codex) → 폼에 있는 필드가 정확히 하나면 그 칸, 아니면 폼 오류
- **`visible(p)`가 SQL 함수와 섞인다**(둘 다) → 머리 선언과 본문 사용의 쌍. **별칭 검사를 미루지 않는다**(둘 다). **펼치기의 차례**(codex) → 조합보다 먼저, 골든
- **정책 하나는 게시판과 안 맞는다**(fable): 목록은 작성자에게도 초안을 숨기고 상세는 보인다 → 모델당 정책 여럿. **`:now`**(둘 다) → 정책의 `-- param:`이 쿼리의 Params에
- **`GetX`의 서명과 우회**(둘 다): 차례는 뷰와 같이, 정책 없는 길은 이름에 드러나게, admin의 `Rows`·`Count`도 그 길 → Q3
- **권한 표에 fixture가 없다**(둘 다): 같은 라우트가 행의 상태마다 다른 기대 → Go 표, 빠짐없음은 런타임이 앱의 라우트로(fable) → Q5. admin이 표 밖(codex) → 런타임 대조가 덮는다
- **먼저 읽기와 `Fail`의 두 길**(fable) → 게시판에서 먼저 읽기를 지운다. 경쟁 시험은 열어 둔 트랜잭션으로(타이밍 없이)
- **게이트의 판**(fable): 새 단계는 v0.1.0에 없다 → 9번 v0.2.0
- **차례**: 정책을 뒤로(fable), 권한 표를 정책 뒤로(codex: 표가 다시 쓰인다). 받은 것은 fable: 표가 먼저 있어야 정책이 바꾸는 상태가 드러나고, 고쳐지는 행은 그 증거다
- **Q4**: 여섯째 규칙은 DSL의 결정(codex), 머리 선언이면 다섯 그대로도 된다(fable) → 사람에게
- 받지 않음: 정책 매개변수를 viewer 밖에서 금지(codex의 둘째 안: 게시판의 `:now`가 필요하다)

---

## M11 끝

- [ ] 1~11 완료
- [ ] M11 완료 조건을 실제로 돌려 확인, 조건마다 시험 이름
- [ ] MILESTONES.md의 M11 체크박스
- [ ] push 뒤 GitHub Actions
