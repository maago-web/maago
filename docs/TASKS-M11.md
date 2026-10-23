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
3. **권한 표**(Q5): `maagotest.Permissions`, 역할마다 로그인한 핸들러를 만드는 도우미(TODO M10의 auth 시험 도우미), 행 없는 라우트(admin 포함)의 실패, auth 복사본의 표(20 라우트) → 확인: 라우트를 더하고 행을 잊으면 `go test` 실패, 틀린 기대가 실패, 행의 URL이 다른 라우트면 실패

   **사람의 결정(2026-10-20, 추천)**: D-156의 "라우트 있는 앱에 표가 없으면 생성 오류"는 사용자를 받는 뷰(`u User`·`u *User`)가 있는 앱에만. 사용자가 없으면 역할이 하나라 표가 더하는 것이 없다. 게시판은 사용자가 처음 오는 0021부터 표를 두고 뒤 단계가 라우트를 더할 때 행도 더한다(무조건 오류면 0008~0034·짧은 튜토리얼·fixture가 모두 깨진다). 그래서 생성기의 진단은 4번으로 옮긴다: 게시판이 표를 가진 뒤에 켜야 커밋마다 걷기가 초록이다

   **계획**
   - **`maagotest.Permissions(t testing.TB, apps []maago.App, roles []Role, rows ...Row)`**(`maagotest/permissions.go` 새 파일). D-156의 `App` 하나를 앱 목록으로(fable): admin은 런타임 App이라 게시판의 표가 `[]maago.App{App, adminApp}`로 함께 덮고, auth는 제 패키지의 시험에서 `{App}`. 라우트 이름이 앱 사이에서 겹치면 실패(생성된 앱끼리는 `maago gen`이 막고, `admin_*`는 보지 않는다)
     - `Role{Name string; Handler http.Handler}`: 그 역할로 요청하는 핸들러(아무도 아님은 `h` 그대로). 역할이 없거나 이름이 겹치면 실패
     - `Row{Route, URL, Body, ContentType string; Want map[string]int}`: Route는 라우트 이름, URL은 생성된 URL 함수가 만든 것, Body는 몸통, ContentType이 비면 API 라우트는 `application/json`, 아니면 `application/x-www-form-urlencoded`. 파일 라우트는 `mime/multipart`로 만든 몸통과 그 boundary의 ContentType(Go 문서에, 둘 다). 메서드는 라우트의 것
     - **Want는 행마다 모든 역할**(fable): 빠진 역할은 그 행의 실패. 보내지 않을 역할은 `maagotest.Skip`으로 말한다(허락되는 역할이 둘인 지우기: fixture 둘에 행 둘, 서로 Skip). 행을 넘어 모으지 않으므로 "아무도 아님이 초안을 본다"를 다른 행의 게시 글이 덮지 못한다. 라우트마다 행이 하나는 있어야 한다(없으면 한 줄)
     - **차례**: 행은 적힌 차례로, 행 안의 역할은 `roles` 차례로 보낸다(요청마다 새 몸통). Go 문서: 쓰는 행은 제 fixture를 쓰고 표의 뒤에, 거절될 역할을 앞에. fixture가 정말 따로인지는 기계가 보지 않는다(codex)
     - **행이 그 라우트인지**: 라우트마다 표시 핸들러를 단 `http.ServeMux`로 요청을 실제로 돌려 어느 표시가 불렸는지 본다. `mux.Handler`의 패턴 비교는 받지 않음: 끝 `/`가 빠진 URL은 리다이렉트인데 그 뒤의 패턴을 돌려준다(codex). 어느 라우트도 아니거나(404·리다이렉트) 다른 라우트면 실패. URL에서 `maago.Prefix()`를 떼고 맞춘다(URL 함수가 붙인다), 보낼 때는 적힌 그대로(fable·codex)
     - 실패는 모두 `t.Errorf`, 줄은 `post_detail GET /posts/3 as bob: 200, want 404`, 303이면 `Location`도(로그인으로 보내기와 성공한 POST가 같은 수다, fable)
   - **`maagotest.As(h http.Handler, c *http.Cookie) http.Handler`**: 요청마다 쿠키를 붙인다(게시판의 `as`, 게이트 둘이 지어낸 것)
   - **복사되는 auth의 `SignIn(ctx, db maago.DB, u User) (*http.Cookie, error)`**(`auth.go`): 새 세션(`newSession`)과 그 쿠키. 로그인·확인·비밀번호 바꾸기의 세 뷰가 같은 두 줄을 하던 것을 이것으로(fable: 시험만 쓰는 코드가 아니라 앱의 코드). 닫혔거나 확인되지 않은 계정은 오류(`SessionUser`가 그 세션을 무시해 역할이 몰래 아무도 아님이 된다, fable). Go 문서에 확인된 계정 만들기 두 줄(`InsertUser`의 `VerifiedAt`). 바이너리에 실리는 세션 발급이지만 `newSession`과 같은 힘이고 코드만 부른다(codex의 지적, 받아들인 대가). `auth/authtest` 패키지는 받지 않음: 복사본이 모듈 경로를 import해야 해서 scaffold에 치환이 생기고, auth 자신의 시험은 import 순환으로 못 쓴다
   - **auth 복사본의 표**(`scaffold/testdata/auth/permissions_test.go`, package auth): 역할 `nobody`·`bob`·`ann`(확인된 계정 둘, `SignIn`: 둘이라야 남의 API 토큰 지우기가 404임을 보인다), 20 라우트. 상태는 지금 동작을 재서 적고 하나씩 맞는지 본다. 시도를 남기는 POST(로그인·가입·메일)는 행마다 다른 메일(제한의 수가 차례에 기대지 않게, codex), 세션을 끝내는 행(`logout`, `account_close_submit`)은 표의 끝. `auth_test.go`의 `login` 도우미는 그대로(그 시험들은 로그인 자체를 본다)
   - **문서**: 각 타입·함수의 Go 문서, `maago doc`의 주제는 8번
   - 하지 않음: API 토큰 역할의 도우미(Bearer, 쓰는 곳이 생기면), 확인된 계정 만들기 도우미(Go 문서의 두 줄), 생성기 진단(4번), 게시판(4번), HEAD·정적 파일(라우트가 아니다), "사용자 있는 앱은 아무도 아님과 로그인한 역할이 다 있어야" 검사(fable: 표는 어느 역할이 로그인했는지 모른다)
   - 계획 리뷰(codex·fable) 반영: 앱 목록과 이름 겹침, 행마다 모든 역할과 `Skip`, 표시 핸들러로 맞추기, Prefix, 303의 Location, 빈·겹친 역할, multipart의 ContentType, `SignIn`의 계정 검사와 세 뷰, auth 표의 메일·끝 행, admin 시험(아래), 4·7번 글. 받지 않음: 행의 fixture 함수(codex: 쓰는 행에 제 fixture라는 규칙과 Go 문서로 충분하고, 표가 함수의 표가 된다)
   - 구현 리뷰: ponytail(직접, 지울 것 없음), codex(코드만 읽은 리뷰, 샌드박스: 앱 사이의 같은 메서드·경로는 `ServeMux`가 panic → 오류로, Prefix는 경로의 경계에서만 뗀다(`/apple`은 `/app`의 것이 아니다, `/app?x` → `/?x`). 받지 않음: 몸통 없는 GET·HEAD·DELETE에도 기본 Content-Type(보낼 몸통이 없다)), fable(모두 Skip인 행이 빠짐없음을 채운다 → 오류, apps 밖의 라우트는 보이지 않는다는 문서, Row의 Prefix·Body 문서, auth 표 끝의 `logout`(bob)·`account_close_submit`(ann)을 Skip으로 갈라 둘 다 살아 있는 사용자로, ContentType 그대로 보내기 시험, 리다이렉트를 말하는 오류 글. 받지 않음: `TestAccess` 줄이기(표가 보지 않는 `next`·Bearer를 본다), 사용자 앱에 로그인 역할이 있는지(표는 모른다))
   - → 확인
     - `maagotest/permissions_test.go`(가짜 App: `rt.NewApp`의 라우트 셋, 역할 둘, Errorf를 모으는 가짜 `testing.TB`): 맞는 표는 오류 없음, 라우트의 행이 없으면 그 줄, 행에서 역할 하나를 빼면 그 줄(다른 행에 그 역할이 있어도), `Skip`은 보내지 않는다, 틀린 Want, 다른 라우트로 가는 URL·어느 라우트도 아닌 URL·끝 `/`가 빠진 URL, Prefix 붙은 URL, 모르는 라우트·역할, 빈·겹친 역할, 앱 사이의 이름 겹침, 몸통과 Content-Type(API·폼), 차례
     - admin: `contrib/admin`의 시험에서 `App(cfg)`의 8 라우트를 `Permissions`로(와일드카드 패턴, 아무도 아님의 303), 행 하나를 지우면 실패(fable)
     - auth 복사본: `cmd/maago`의 auth 시험(복사해 돌리는 것)이 새 표를 두 방언에서. 라우트 하나의 행을 지우면 실패함을 확인. `SignIn`의 확인 안 된 계정 오류
     - 회귀: 빠짐없음·라우트 맞추기 검사를 각각 빼면 위 시험이 실패
     - `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG 포함)
4. **게시판에 권한 표를 엮고 생성기 진단**(사람의 결정 2026-10-20): 0021(사용자가 처음 오는 단계)부터 `TestPermissions`(`auth.SignIn`·`maagotest.As`), 라우트를 더하거나 권한이 바뀌는 단계(0022·0023·0025 admin·0029·0030·0031·0032)가 행을 더하고 고친다, 단계 글도(M10 9a의 길). `TestDrafts`·`TestAuthors`의 겹치는 행은 표로. 그 뒤 생성기: 사용자를 받는 뷰가 있는 앱의 시험에 `maagotest.Permissions` 호출이 없으면 오류. 걸리는 fixture에 표 → 확인: `TestBoardTutorial`(PG), 표 없는 사용자 앱이 `maago gen` 오류·사용자 없는 앱은 그대로, Catches 둘

   **계획**
   - 두 커밋, 둘 다 걷기가 초록인 채: (a) 게시판에 표를 엮는다(생성기는 아직 보지 않는다), (b) 생성기 진단과 fixture. (b)가 먼저면 0021부터 걷기가 깨진다(걷기는 단계마다 gen·check·test)
   - **(a) 게시판**(`steps.sh unpack`, 단계마다 rebase로 고치고 걷고 `pack`. 충돌은 `board/web_test.go`의 0022·0031·0032에 몰려 있어 그 차례로, 하나씩 걸은 뒤 다음, codex)
     - **쓰는 행의 규칙은 0021부터**(fable): 0021에서는 아무도 아님과 ann이 다 쓸 수 있고 쓰기가 다음 역할이 볼 것을 바꾼다(같은 글 게시는 둘째가 409, 같은 이름의 게시판은 둘째가 Taken). 그래서 성공하는 역할마다 fixture를 따로 두고 나머지는 Skip. 0022는 누가 성공하는지만 좁힌다. 0031의 `post_change`는 `?version=`을 지니므로 고치는 역할마다 fixture
     - **0021**: 새 파일 `board/permissions_test.go`의 `TestPermissions`, 역할 `nobody`·`ann`, 앱 `[]maago.App{App}`, 라우트 15개. 단계 글의 차례는 읽는 이가 만나는 차례(fable): `Home`이 `u *auth.User`를 받자 `maago gen`이 표를 요구한다(진단과 fix를 인용, (b)가 생긴 뒤 기준. Expect 단계는 아니다) → 표. 행·역할·`Skip`·차례·쓰는 행의 규칙, `SignIn`·`As`. 0021에서 두 열이 같고 0022의 이 파일 diff가 둘이 갈리는 곳이라고 말한다
     - **0022**(작성자): 역할 `bob`·`staff`(`CreateSuperuser`+`SignIn`), 상태를 고친다. 단계 글에 메서드별로: GET 폼은 로그인으로 303, POST는 401, API는 401과 `WWW-Authenticate`. API 행은 세션 쿠키로 간다(Origin 없는 요청, Bearer 역할이 필요 없다, M10 게이트가 토큰 도우미를 찾았다). `TestAuthors`에서 상태만 보는 행은 표로
     - **0023**: `post_delete`의 행 둘(댓글 있는 글은 숨기기, 없는 글은 지우기: fixture가 다르다)
     - **0025**(admin, fable: 0024는 `Maago-Expect` 단계라 `go test`가 돌지 않고 시험의 `config`가 admin을 얻는 것도 0025): 앱 목록에 admin의 App. `config`가 만든 것과 같은 값을 쓰도록 `config`를 `admin.App`의 값도 돌려주는 도우미로(같은 설정을 다시 만들지 않는다, codex). admin 8 라우트의 행
     - **0029**(파일): `attachment_add`(게시판의 `upload`가 만드는 multipart 몸통과 ContentType), `attachment`
     - **0030**: `me`·`api_me`
     - **0031**: `post_edit`·`post_change`, `TestEdit`의 겹치는 상태 행
     - **0032**(초안): 초안 fixture와 그 행. `TestDrafts`의 `{who, method, path, code}` 표는 이 표로, 목록에 초안이 없는지(`out`)·웹훅·초안의 댓글 수(그 수는 `comment_create` 행에 기대므로 댓글 POST 하나를 `TestDrafts`에 남긴다, fable)는 그대로. 단계 글: 파일 행이 0029의 403에서 404로 바뀌는 diff가 D-147의 모양
     - 0033·0034는 패치 충돌만
     - 단계 글: 0021·0022의 `signedIn`·`login`을 `SignIn`·`As`로(`signedIn`은 `SignIn`으로 짓는다. `newUser`는 해시를 그대로: `TestSignIn`이 로그인 자체를 본다). `MAP.md` 35행의 `login` 도우미 글도
     - Catches(fable·codex): 행이 하나뿐인 라우트(`api_me`)의 행을 지우면 `go test:`(라우트에 행이 여럿이면 하나를 지워도 초록이다), 표를 쓴 뒤 그 줄로 정확히 하나만 맞는 `old`. 기존 `mayChange`·`maySee` 행은 그대로
   - **(b) 생성기 진단**: `internal/web`의 `Check`·`Views` 안이 아니라 `build.Gen`이 부르는 따로 된 함수(admin의 "no package uses board.Admin" 검사 자리, fable: `internal/web`의 사용자 뷰 fixture 14개는 시험 파일 없이 만들어진다, codex·fable)
     - 사용자를 받는 뷰가 있는 앱의 디렉터리에서 `_test.go`(바깥 시험 패키지 포함, maago가 생성한 파일 빼고)를 파싱해 `github.com/maago-web/maago/maagotest`의 import(어느 별칭이든)를 그 이름으로 부르는 `Permissions` 호출을 찾는다. 도우미 함수 안이어도 된다. dot import는 받지 않는다(호출이 그 패키지 것인지 알 수 없다: 별칭을 쓰라는 fix)
     - 진단: `var Routes`에 `app board has routes whose views take the user, but no test calls maagotest.Permissions`, fix `write board/permissions_test.go calling maagotest.Permissions(t, []maago.App{App, …}, roles, rows...), a row for each route`, see `go doc github.com/maago-web/maago/maagotest Permissions`(8번이 `maago doc` 주제로)
     - **admin이 앱 목록에 있는지의 정적 검사는 하지 않는다**(codex·fable): `admin.App`은 런타임 값이고 그 값이 표에 가는지는 생성기가 어디서도 하지 않는 데이터 흐름이다. 남는 빈틈(표가 admin을 빼면 그 라우트는 보이지 않는다)은 D-156에 한 줄, 런타임으로 막는 길(마지막 Handler가 서빙한 앱 중 apps에 없는 것)은 TODO
     - **`Permissions`의 앱 없는 표**(fable, 3번의 남은 것): 앱들에 라우트가 하나도 없으면 오류. 아니면 `Permissions(t, nil, roles)`가 진단을 채운다
     - 걸리는 fixture: `cmd/maago/testdata/web/site`(`Secret`·`Mine`·`Greet`, 22 라우트, 헤더로 사용자): 시험이 이미 HTTP를 돌므로 표를 더한다. 짧은 튜토리얼은 사용자 뷰가 없다(확인함)
     - 시험(`internal/build`의 fixture 프로젝트): 사용자 앱에 표 없음 → 오류, 별칭 import·바깥 시험 패키지·도우미 안의 호출 → 없음, dot import → 오류, 같은 이름의 다른 패키지 `Permissions` → 오류, 생성 파일 안의 호출 → 오류, 사용자 없는 앱 → 없음
     - Catches: 마지막 패치의 `maagotest.Permissions(` 호출을 지우면 `maago gen:`(fable)
   - 하지 않음: admin의 정적 검사(위), `TestAccess`류 정리
   - 계획 리뷰(codex·fable) 반영: 0025, 0021부터의 쓰는 행 규칙, Catches의 행 하나 라우트와 gen 쪽 줄, 앱 없는 표, build 자리, 별칭·dot·생성 파일·바깥 패키지, admin 검사 빼기, `config` 도우미, rebase 차례, 단계 글(진단부터, 메서드별 상태, 세션 쿠키의 API, 0032의 404), `newUser`의 모순, MAP 35행, TestDrafts의 댓글 수, 7번 글
   - 구현에서: 걸린 fixture는 계획의 `web/site` 말고 셋 더(`maago doc view`·`auth`의 예, `add_test.go`의 `staffApp`): 각자 표. `view` 주제의 예에 `TestPermissions`를 더해 진단의 `see: maago doc view`가 표를 보인다(8번이 주제 글을). 쓰는 행의 함정 둘을 겪음: 파일 행의 초안을 앞 행이 게시, Catches의 바꿈이 hunk의 줄 수를 바꾸면 패치가 깨진다(한 줄을 한 줄로)
   - 구현 리뷰: ponytail(직접), codex(코드만 읽은 리뷰, 샌드박스: 0025의 `admin_delete`가 앞 행이 지운 글을 물어 우연히 200 → 손대지 않은 fixture로. 받지 않음: 별칭을 지역 변수로 가린 `mt.Permissions`(go/types가 필요하고 일부러가 아니면 생기지 않는다)), fable(같은 0025, see 줄이 아무것도 말하지 않는다 → view 주제의 예, `as`와 `maagotest.As`의 겹침 → `as`를 지움, 파싱 안 되는 시험 파일은 그 fix로, 0032 글의 댓글 행)
   - → 확인: (a) 단계마다 걷기 초록·`TestBoardTutorial`(PG)·Catches의 행 줄, (b) 생성기 시험·fixture·Catches의 gen 줄·`Permissions`의 앱 없는 표 시험, `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG)
5. **표를 읽는 쿼리 훑기**: FROM·JOIN(부분 쿼리·CTE·쉼표 조인·UNION의 각 SELECT)의 표 이름과 별칭. 정책 표와 같은 이름의 CTE(가림), SELECT 목록 안의 부분 쿼리. 읽는 표를 모를 모양은 정책 모델이 있을 때만 오류 → 확인: 게시판·auth·gentest 모든 쿼리의 골든, 숨긴 표(EXISTS·CTE·SELECT 목록 안)를 찾는다

   **계획**
   - **훑기**(`internal/mapper/tables.go` 새 파일): `func (q *Query) tables() ([]tableRef, []diag.Diagnostic)`, `tableRef{Table, Alias string; Pos token.Position}`. `q.toks`(본문 문장의 토큰, 조건 줄의 `{q}` 표시 포함)를 `joins()`처럼 평평하게 한 번 돈다: 부분 쿼리·`EXISTS`·SELECT 목록 안·UNION의 각 SELECT·조건 줄의 표가 따로 갈래 없이 잡힌다. Table은 `joins()`처럼 소문자, `schema.` 뗌(D-104의 길). Alias는 따옴표 없는 이름이면 소문자, 따옴표 이름이면 그대로(SQL의 대소문자 규칙), 없으면 빈 값(맨 이름이 별칭인지는 6번이 정한다). Pos는 표 이름 토큰. 맵이 아니라 나온 자리마다 하나: 6번은 별칭 이름이 아니라 나온 자리마다 본다(같은 별칭 `p`의 `EXISTS` 둘 중 하나만 정책을 쓰면 누출, fable)
   - **표 항목이 시작하는 자리**: `FROM`, 모든 `JOIN`, FROM 목록의 `,`, `UPDATE … FROM`·PG `DELETE … USING`(뒤가 `(`가 아닌 `USING`). FROM 목록은 같은 깊이의 절 낱말(`WHERE`·`GROUP`·`HAVING`·`WINDOW`·`ORDER`·`LIMIT`·`OFFSET`·`FETCH`·`FOR`·`UNION`·`INTERSECT`·`EXCEPT`·`RETURNING`·`SET`)이나 그 깊이를 닫는 `)`에서 끝난다(fable: `SELECT (SELECT 1 FROM a) AS x, y FROM t`의 y는 표가 아니다). `ON`은 끝내지 않는다: `ON` 식의 쉼표는 괄호 안뿐이고 `JOIN b ON …, c`의 c는 다시 항목이다. 쿼리의 `FROM`이 아닌 것: `IS [NOT] DISTINCT FROM`, 그 깊이에 `SELECT`·`DELETE`·`UPDATE`가 앞서지 않는 `FROM`(`EXTRACT(year FROM …)`·`SUBSTRING(x FROM 2)`·`TRIM(… FROM x)`). 깊이마다의 상태(동사를 봤는지, FROM 목록 안인지)는 `(`에서 쌓고 `)`에서 버린다
   - **쓰기 대상은 나오지 않는다**(fable: DELETE만 나오면 규칙을 예측할 수 없다): `INSERT INTO t`·`UPDATE t`·`DELETE FROM t`의 t. D-155의 "읽는", 쓰기 권한은 정책 밖. `DELETE … USING u`·`UPDATE … FROM u`의 u와 그 안의 부분 쿼리는 나온다. 대가: 쓰기의 `RETURNING`은 정책 없이 쓴 행을 돌려준다(쓸 수 있는 행이다)
   - **항목 하나**: `LATERAL`·`ONLY`를 건너뛰고(`joins()`와 같이) ① 이름(Word·Quoted, `a.b`) 뒤가 `(`면 표 함수(`json_each`·`generate_series`: 표가 아니다), 아니면 표. 바로 다음 토큰이 `AS`면 그 뒤 이름이 별칭, Word·Quoted이고 멈춤 낱말이 아니면 별칭(fable: 바로 다음만, `{q}`의 `q`는 `{` 뒤라 아니다). 별칭(이나 이름) 뒤의 토큰은 멈춤 낱말·`,`·`)`·`{`·`}`·끝이어야 하고 아니면 모를 모양: 그래서 `t *`·`TABLESAMPLE`·`INDEXED BY`·`NOT INDEXED`는 받지 않고 모를 모양이다(fable: 말뭉치에 없고 진단은 정책 모델이 있을 때만 난다). 멈춤 낱말: 위 절 낱말과 `ON`·`USING`·`JOIN`·`INNER`·`LEFT`·`RIGHT`·`FULL`·`OUTER`·`CROSS`·`NATURAL` ② `(` 뒤가 `SELECT`·`WITH`·`VALUES`면 파생 표(그 별칭은 표가 아니다, 안의 표는 평평한 훑기가 잡는다) ③ 그 밖(괄호 친 조인 `FROM (a JOIN b)`, 이름이 아닌 토큰)은 **읽는 표를 모를 모양**의 진단. PG의 `TABLE t`도 이 진단(`TABLE` 낱말이 문장·괄호의 처음)
   - **CTE의 가림**: CTE 이름은 그 정의가 보이는 곳에서만 표를 가린다. 보이는 곳은 정의의 몸통 끝(`RECURSIVE`면 그 `WITH`부터: PG는 형제끼리 앞도 본다)부터 `WITH`를 담은 괄호(없으면 문장)의 끝까지. 그래서 `WITH board_post AS (SELECT * FROM board_post)`의 몸통은 진짜 표를 내고, 형제 `WITH a AS (SELECT * FROM board_post), board_post AS (…)`의 a 몸통도 진짜 표다(평평한 이름 모음이면 가려져 누출, 그래서 fable의 더 단순한 안은 받지 않음), 안쪽 `(WITH …)` 괄호 밖에서는 가리지 않는다(codex). SQLite는 `RECURSIVE` 없이도 제 이름을 부르면 재귀로 읽는데 이 훑기는 진짜 표로 낸다: 많이 내는 쪽이라 받아들임. `joins()`의 WITH 읽기를 둘이 쓰는 도우미 `ctes(toks)`(이름과 몸통의 괄호 자리)로 뽑는다: `joins()`는 이름만 쓰고 동작은 그대로(`TestJoins`)
   - **진단은 돌려줄 뿐 내지 않는다**: 5번에서 `tables()`를 부르는 것은 시험뿐이다. "정책 모델이 있을 때만 오류"의 문은 6번(정책 표를 아는 곳)이 단다. 진단 글은 `cannot tell which tables the query reads: <모양>`, fix `name the table in FROM or JOIN, or split the query`
   - **정해 둔 것**(6번이 물려받는다): 조건 줄 위의 표는 몇 변형에서만 읽혀도 나온다(보수적인 쪽: 빠뜨림이 D-147의 누출). 파생 표의 별칭이 표 이름과 같아도(`FROM (SELECT …) board_post`) 가리지 않는다: 안의 진짜 표가 이미 나온다. 표 함수가 안에서 표를 읽는지는 모른다(마이그레이션에 함수를 쓰는 앱만). 따옴표 이름의 대소문자와 스키마는 `joins()`처럼 접는다(codex: 다른 표를 같은 표로 볼 수 있지만 많이 내는 쪽이고, 모델의 표 이름은 소문자에 스키마가 없다). 7번에 넘김: 게시판의 `board_post`를 읽는 쿼리는 일곱이고 `pinned_overlap.sql`(별칭 없음)도 정책이나 `none`이 필요하다(fable)
   - **확인**
     - `TestTables`(`TestJoins` 꼴): 말뭉치에 있는 모양(SELECT 목록의 `(SELECT count(*) FROM board_comment c …)`, `EXISTS (SELECT 1 FROM orders …)`, `JOIN auth_token t`, 별칭 없는 `FROM t`, `AS`, 따옴표 이름·별칭, 별칭의 대소문자)과 없는 모양: 쓰기 대상 셋은 안 나오고 `DELETE … USING u`·`UPDATE … FROM u`·`INSERT … SELECT … FROM u`·`WHERE id IN (SELECT …)`는 나옴, CTE가 같은 이름의 표를 가림·형제 몸통·`RECURSIVE`·`JOIN cte`·중첩 `WITH`(codex의 예)·괄호 밖에서는 안 가림, 쉼표 조인, `JOIN … ON …, c`, `SELECT (SELECT 1 FROM a) AS x, y FROM t`·`SELECT (SELECT 1 FROM a), EXTRACT(year FROM b) FROM t`(fable), 파생 표 `(SELECT…) s`, `LATERAL`, `ONLY`, `FROM json_each(…)`, `FROM t {c} JOIN u`·`FROM t\n{q} JOIN u`, 조건 줄의 `{x} AND EXISTS (SELECT 1 FROM t2)`, 글·주석 안의 `FROM x`, `EXTRACT(… FROM …)`·`IS DISTINCT FROM`, UNION의 둘째 SELECT, 모를 모양(괄호 친 조인, `TABLE t`, 이름 아닌 토큰, `t *`, `TABLESAMPLE`, `INDEXED BY`)
     - 말뭉치 골든(`internal/mapper/testdata/tables.golden`, DB 없이: `Read`가 `body()`까지 돌려 `toks`가 있다): auth 복사본·gentest(orders·plain, blog는 쿼리 없음)·`web/site`·게시판. 게시판 쿼리는 패치 안에만 있으므로 시험이 임시 git 저장소에서 `steps/*.patch`를 차례로 `git apply --include='*/queries/*.sql'`(빌드·PG 없이 마지막 단계의 10개, 리뷰 둘 다 확인). 파일마다 표마다 한 줄 `file:line:col table alias`. 숨긴 표(`post_list.sql`·`search_posts.sql`·`export_rows.sql`의 `board_comment c`, `order_exists.sql`의 `orders`, `title_taken.sql`의 `site_post`)가 골든에 있다
     - 회귀: 숨긴 표를 잡는 것은 깊이마다의 동사 규칙이다. 그것을 깨면(동사를 깊이 0에서만 보기) 골든의 숨긴 표 다섯이 빠지고, `)`에서 FROM 목록을 끝내지 않으면 위 `y` 사례가, 쉼표·CTE 가림을 빼면 그 사례가 실패한다(fable). 말뭉치에 없는 모양(CTE·쉼표·UNION·USING·파생 표·LATERAL)은 `TestTables`만이 지킨다
     - `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG 없이 돈다)
   - 하지 않음: 머리 키·`policies.sql`·진단을 내는 문(6번), `joins()`를 이 훑기 위로 다시 쓰기(NULL 쪽을 보는 다른 질문), 표 함수 안 보기, 생성 이전 단계의 게시판 쿼리(지워진 `board_name_taken.sql` 등: 마지막 단계만)
   - 계획 리뷰(codex·fable) 반영: 쓰기 대상 셋을 함께 빼기, `)`에서 끝나는 FROM 목록과 깊이마다의 상태, 나온 자리마다의 검사, 별칭의 대소문자·Pos·바로 다음 토큰·`ON`, 받지 않는 모양을 모를 모양으로(`t *`·`TABLESAMPLE`·`INDEXED`), 중첩 `WITH`·형제 몸통 사례, 회귀의 실제 기제, 7번의 `pinned_overlap`. 받지 않음: 재귀로 내려가는 훑기(codex: 보이는 곳의 규칙이 중첩을 이미 가린다), 평평한 CTE 이름 모음과 중첩 WITH 오류(fable: 형제 몸통이 가려진다), 따옴표 이름·스키마를 따로(codex: 위)
   - 구현에서: 말뭉치 골든이 계획에 없던 둘을 찾았다. 매개변수 `:from`(`pinned_overlap.sql`)과 줄 처음의 조건 `{from & to}`(`search_orders.sql`)의 낱말을 `FROM`으로 읽었다: 둘 다 건너뛴다(조건은 `body.go`처럼 줄 처음의 `{`부터 `}`까지). Pos는 이름의 첫 토큰(스키마가 있으면 스키마). `joins()`도 CTE를 보이는 곳의 규칙으로 본다(아래 fable): 중첩 `(WITH …)` 앞의 `LEFT JOIN`이 CTE로 읽히던 것이 표로, 제 몸통에서 제 이름을 `LEFT JOIN`하는 비재귀 CTE는 오류가 아니라 진짜 표. 나머지는 그대로(`TestJoins`에 한 줄)
   - 구현 리뷰: ponytail(직접, 지울 것 없음), codex(코드만 읽은 리뷰, 샌드박스: 따옴표 CTE `"Posts"`가 PG에서 다른 이름인 `posts`를 가려 누출 → CTE 이름과 비교는 따옴표면 대소문자 그대로, SQLite에서는 덜 가리는 쪽), fable(`joins()`가 이름을 모두 먼저 모아 동작이 바뀜 → 보이는 곳의 규칙과 `TestJoins` 한 줄, 시험이 못 잡던 멈춤 낱말(`CROSS`·`INNER`·`NATURAL`·`RIGHT`·`FULL`)과 절 낱말(`RETURNING`·`SET`·`HAVING`·`WINDOW`·`LIMIT`·`OFFSET`·`FOR`)의 줄, `VALUES` 파생 표, 괄호 안 `TABLE`, Pos 문서, 주석, 끝의 `TABLE`. 받지 않음: 조건 건너뛰기를 달리는 bool 하나로(같은 길이), `WITH ORDINALITY`·`WITH TIES`가 `ctes`에 주는 가짜 이름(전부터, 해가 없다))
6. **읽기 정책**(Q2·Q3·Q4): `policies.sql`, 선언·사용의 쌍, 별칭 검사, 펼치기 → 조합 → prepare, 정책 매개변수, 정책 모델의 `GetX`와 정책 없는 길, admin은 정책 없는 길 → 확인: 정책을 빠뜨린 쿼리(EXISTS 안 포함)·까닭 없는 `none`·모르는 `:viewer_x`·다른 모델의 별칭이 생성 오류, 아무도·남·작성자·staff가 같은 쿼리에서 다른 행, nil 사용자, 조건 줄과 `WHERE { }` 안의 정책 골든, 두 방언

   **계획**
   - **정책 파일**(`<앱>/policies.sql`, `queries/`의 옆: `queries/` 안이면 쿼리로 읽혀 "no -- name:"). `project.PolicyFiles`(`QueryFiles`의 이웃, 같은 walk). 파일 하나에 정책 여럿, 정책마다 블록 하나: `-- name: 이름 모델 별칭` 줄이 블록을 열고 다음 `-- name:` 줄까지가 그 블록(쿼리 파일의 `-- name:`과 같은 뜻 "이 단위의 이름과 모양". `-- policy:`는 쿼리가 쓴다는 한 뜻으로 남긴다, PHILOSOPHY 4, fable. 쿼리 머리에 붙여 넣은 3필드 `-- policy: visible Post p`의 fix는 `-- policy: visible p`). 블록의 머리 줄은 `-- param: 이름 타입`(필수 스칼라만: `?`·슬라이스·선택지는 오류. 정책은 조건 줄이 없어 셋 다 뜻이 없다)과 `-- get: GetX`(아래), 그 밖의 `word:` 키는 오류(`ref.PolicyKeys`: name·param·get), 다른 주석은 설명. 머리 줄 뒤의 SQL이 조각(식 하나, 괄호 없이 써도 펼칠 때 괄호로 싼다)
     - 이름·별칭은 `^[a-z][a-z0-9_]*$`(파라미터 이름 꼴, `visible(p)`가 SQL에 쓰인다), 앱 안에서 이름이 겹치면 오류, 모델은 그 앱의 모델 이름(did you mean). 조각이 비었거나 `{`·`}`·`;`가 있으면 오류. `:이름`은 블록의 `-- param:`이거나 `:viewer_<열>`이어야 하고, 선언한 것은 쓰여야 한다. `-- param: viewer_x`는 오류(사용자에서 온다). 네이티브 자리표시는 쿼리처럼 오류. 파일 전체를 한 번 토큰으로 자르고(`dialect.SplitComments`) 블록에 오프셋으로 나눈다
     - 오류가 있는 정책도 이름은 남긴다: 그것을 쓰는 쿼리는 조용히 빠진다(머리 오류의 쿼리처럼, 따라 나오는 오류가 없게)
   - **★ `GetX`가 쓰는 정책**(결정에 없는 선택, 리뷰에 묻는다): 게시판의 `Post`는 정책이 둘(`out`·`visible`)이고 `GetPost`는 `visible`이어야 한다(7번: 남의 초안 403 → 404). (a, 추천) 그 모델의 정책 중 하나가 `-- get: GetPost`를 가진다. 정책 있는 모델에 그 줄이 없거나 둘이면 오류(fix가 그 모델의 정책 이름을 늘어놓는다), 값은 꼭 `Get<모델>`(grep으로 `GetPost`를 찾으면 정책이 나온다, 틀린 값의 fix는 기대 이름 그대로). 정책이 하나뿐인 모델에도 요구한다(예외 없는 규칙). 그 정책의 `-- param:`은 `GetX`의 id 뒤 인자로 선언 차례대로: `GetPost(ctx, u, db, id, now)`(게시판 `visible`의 `:now`. 매개변수 구조체를 만들면 정책 없는 모델의 `GetX`와 모양이 갈린다. 같은 타입의 param이 둘이면 차례를 틀릴 수 있다: 서명에 이름이 남고 문서에 한 줄) (b) 첫 정책: 숨은 규칙(PHILOSOPHY 5) (c) 정책 모델에는 `GetX`를 만들지 않고 정책 없는 길만, 읽기는 손으로 쓴 쿼리: D-155("정책 모델의 `GetX`는 사용자 포인터를 받고")와 부딪히고, 미리 생긴 함수가 정책 없는 쪽뿐이라 게으른 에이전트가 그것을 부른다. 리뷰 둘 다 (a)
   - **쿼리 머리**(`ref.QueryKeys`에 `policy`, `header.read`의 switch): 한 꼴 `-- policy: <이름|none> 별칭 [까닭…]`: `-- policy: visible p`이나 `-- policy: none c 까닭…`(까닭은 한 낱말 이상). 별칭마다 하나(같은 별칭 둘은 오류), `none`은 예약 이름. D-155의 글 그대로 **별칭마다**(fable): 검사가 나온 자리마다 그 별칭의 정책 사용이나 none을 요구하므로 섞인 머리(`visible p` + `none c`)도 빠뜨림은 생성 오류이고, 쿼리 전체 none이면 게시판 목록의 상관 집계 `(SELECT count(*) FROM board_comment c …)`가 Comment에 정책이 생기는 날 Post의 정책을 내려놓게 한다(나누면 N+1). codex는 쿼리 전체를 권했지만 까닭(별칭 하나가 보호 없이 남는다)은 자리마다의 검사가 막는다. 별칭 없는 `FROM board_post`의 별칭은 표 이름(5번이 넘긴 것: `out(board_post)`)
   - **펼치기**(본문 읽기, 조건 줄의 조합보다 먼저: `bodyReader.text`): 머리에 선언된 이름의 Word 뒤가 `( 별칭 )`이면 `(` + 조각 + `)`의 Part들로 바꾼다. 조각의 정책 별칭 `p.`은 쿼리의 별칭으로(그 토큰만 따로 된 Part, Pos는 정책 파일의 자리), `:now`·`:viewer_id`는 `Part{Param}`. 조각 Part의 Pos는 `policies.sql`을 가리켜 prepare 오류가 정책 파일의 줄을 말한다(`Variant.Position`이 그대로 나른다). 선언된 이름 뒤가 `( 별칭 )`이 아니거나 별칭이 머리와 다르면 오류, 선언했는데 본문에 없으면 오류(파라미터처럼). 선언하지 않은 `visible(p)`는 SQL 함수로 남는다(그 표는 아래 검사가 잡는다)
     - **매개변수 합치기**(`Read`가 머리 뒤 본문 앞에): 정책의 `-- param:`은 쿼리의 `Params`에 들어간다(쿼리나 다른 정책이 같은 이름을 선언하면 타입이 같을 때 하나, 다르면 오류: 게시판의 `out`·`visible`이 둘 다 `:now`). Q3의 "쿼리가 다시 선언하면 오류"는 고친다(리뷰가 정한 것, fable): `post_list`는 고정 공지 `ORDER BY`에도 `:now`를 쓰는데 선언을 지우면 파일이 제 매개변수를 말하지 못한다(D-107의 "모든 `:이름`은 선언되어야"). `:viewer_<열>`은 `Param{Viewer: true}`로 들어가지만 `Params` 구조체·조건 줄·쿼리 본문에서는 보이지 않는다(쿼리 본문의 `:viewer_id`는 선언 안 된 파라미터, fix가 정책 파일을 말한다). 타입은 아래에서 사용자 모델로 채운다
     - **조건 줄 위의 사용은 오류**(`{x} AND visible(p)`는 `!x` 변형에서 정책이 빠진다): "a policy is never conditional". 그 줄에 그 사용과 같은 블록의 정책 표가 나오면(조건 줄의 `EXISTS (SELECT … FROM board_post p2 WHERE visible(p2))`) 받는다: 둘이 함께 들고 난다(fable)
   - **검사**(`mapper.CheckPolicies`, `build`의 early, 정책이 하나라도 있는 프로젝트에서): 정책 모델의 표 → 모델·정책. 모든 앱의 모든 쿼리에서
     - `tables()`의 진단(5번의 모를 모양)을 낸다. 프로젝트에 정책이 없으면 내지 않는다
     - **나온 자리마다**: 정책 모델의 표가 나온 자리(`tableRef`)는 **같은 SELECT 블록**에 그 별칭(없으면 표 이름)의 사용이 있어야 한다. 블록은 동사(SELECT·UPDATE·DELETE)를 본 괄호 깊이(없으면 문장), 같은 깊이의 `UNION`·`INTERSECT`·`EXCEPT`가 새 블록을 연다. 동사 없는 괄호(`(a OR b)`)는 바깥 블록. `tables()`가 `tableRef.Block`을 채우고 사용도 같은 블록 번호를 받는다(한 번 훑기의 깊이 상태에 블록 번호만 더함). 그래서 같은 별칭 `p`의 `EXISTS` 둘 중 하나만 쓰면 다른 쪽이 오류(fable, 5번), UNION의 둘째 SELECT도. 오류 글: `query reads board_post p without a policy`, fix `declare -- policy: visible p (or out p) and write visible(p) in its WHERE; or -- policy: none <why> if the query may read every row`(그 모델의 정책 이름을 늘어놓는다)
     - **사용의 별칭**: 사용 `visible(x)`의 블록에 별칭 x로 나온 자리가 없거나 그것이 정책의 모델 표가 아니면 오류(`x reads board_board, but policy visible is for Post`). 상관 부분 쿼리에서 바깥 별칭을 쓰는 것도 이 오류: 사용은 표가 나온 블록에(규칙이 하나)
     - **사용은 좁힌다**(있는데 효과 없는 정책이 곧 누출): 사용은 블록의 `WHERE`나 그 별칭을 들여온 JOIN의 `ON`의 **AND 항**이다. 앞 토큰은 `WHERE`·`ON`·`AND`·`{`·묶는 `(`(앞이 낱말이 아닌 괄호: `COALESCE(`·`CASE … WHEN`은 아니다), 뒤 토큰은 `AND`·`)`·`}`·절 낱말·끝(`IS NOT FALSE`·`::bool`은 아니다). 사용에서 절의 뿌리까지 어느 괄호 깊이에도 `OR`가 없고 묶는 괄호 앞이 `NOT`이 아니다(`AND (visible(p) AND y)`는 받는다, fable). `WHERE { }`의 OR 줄머리도 토큰이라 같이 걸린다(codex의 CASE·COALESCE, fable의 괄호). `ON`은 그 JOIN이 들여온 별칭에만, `RIGHT`·`FULL` JOIN이 아닐 때만: `FROM board_post p LEFT JOIN c ON visible(p)`는 p를 거르지 못한다(codex·fable). `HAVING`은 거절. fix `write visible(p) in WHERE, joined by AND`
     - `none c`: 별칭 c로 나온 자리가 없거나 정책 모델의 표가 아니면 오류(선언한 것은 쓰인다), 있으면 c의 자리는 검사를 건너뛴다. 모를 모양이면(tables의 진단) 그 진단만 내고 none의 미사용은 건너뛴다(fable). 다른 앱의 쿼리가 정책 모델을 읽으면 오류(정책은 쿼리 앱의 `policies.sql`에서만 찾고, none을 fix로 주면 설계된 누출 길이다, fable): fix는 "move the query into app board". 앱을 넘는 사용은 TODO. `policies.sql` 없는 앱의 `-- policy:`는 그 정책이 없다는 오류. 파생 표의 별칭에 쓴 정책(`FROM (SELECT … FROM board_post p) s WHERE visible(s)`)은 fix가 "write visible(p) inside"
     - 정책의 `get` 줄(위), 모델 이름, 쓰이지 않는 정책(어느 쿼리도 쓰지 않고 `get`도 아닌 것: 오류, 선언한 것은 쓰인다)
   - **사용자 타입**(Q3): `web.User(apps)`가 구문에서 처음으로 사용자를 받는 뷰의 타입(import 경로, 패키지 이름, 타입 이름, 자리: `u *auth.User`의 `auth`를 그 파일의 import로 푼다. 푸는 일은 `internal/web`에, mapper에 복사하지 않는다, codex)을 돌려준다. 생성 파일의 import와 서명에만 쓰는 잠정 값이다. `web.Types`는 queries_gen.go를 쓴 뒤에 돌아서 쓸 수 없다. 뷰들이 모두 같은 타입인지는 지금처럼 `Types`가 본다. `mapper.Viewer{Path, Name, Model}`: 그 경로·이름의 모델. 정책이 있는데 사용자를 받는 뷰가 없으면 오류(첫 정책 자리, Q3), `:viewer_x`가 있는데 사용자 타입이 모델이 아니면 오류(열 타입을 모른다), `x`가 사용자 모델의 열 이름이 아니면 오류(did you mean). 정책 앱이 사용자 패키지와 다르면 생성 파일이 그 패키지를 import한다(`maagouser`, views_gen.go의 이름). 사용자 패키지가 정책 앱을 import하면 순환: 문서에 한 줄(auth는 아니다)
   - **생성**(`gen.go`, 템플릿)
     - 정책을 쓰는 쿼리(none 아닌 것): `func PostList(ctx context.Context, u *maagouser.User, db maago.DB, p PostListParams)`. 본문 앞에 `var viewerID, viewerIsStaff any` / `if u != nil { viewerID, viewerIsStaff = u.ID, u.IsStaff }`: nil 사용자는 모든 viewer 열이 NULL(WHERE가 버리는 SQL의 모름). 포인터 필드는 `u.X != nil`일 때 `*u.X`, SQLite 시각은 `rt.SQLiteTime`(지금의 `g.arg`). `any`라 드라이버가 포인터를 다루는지에 기대지 않는다
     - 정책 모델의 `GetX(ctx, u *U, db, id[, 정책 param…])`: `SELECT 열 FROM t p WHERE "id" = $1 AND (조각)`(정책의 별칭을 FROM에, 열은 지금처럼 맨 이름). `GetXIgnoringPolicies(ctx, db, id)`가 지금의 `GetX`(이름에 드러나는 정책 없는 길, Q3. 문서 주석: admin·시험·마이그레이션 도구처럼 모두를 읽을 곳). `mapper.Names`에 그 이름. `UpdateX`·`DeleteX`·버전의 `Exists`는 그대로(쓰기와 쓰기의 확인은 정책 밖, D-155의 "읽는")
     - admin(`admingen/form.go`): 정책 모델의 행 읽기는 `GetXIgnoringPolicies`. admin의 `Rows`·`Count`는 제 SQL이라 그대로. `admingen.File`이 정책 모델을 안다(인자 하나)
   - **prepare**(`checkQueries`): 쿼리는 펼친 글로 지금처럼(정책 param 타입도 PG가 본다). 정책마다 따로 `SELECT 1 FROM t p WHERE (조각)`을 Describe의 길로 prepare해 오류가 정책 파일의 자리로 난다. 그래서 정책 있는 `GetX`의 SQL은 지금처럼 maago의 버그 안전띠(`dialect.Bug`)로 남는다
   - **규칙**: 여섯째 SQL 규칙을 `maago doc query`의 규칙 목록(`ref`, 진단의 "rule N")과 DECISIONS의 "SQL 규칙" 목록에: "6. 정책이 있는 모델의 표를 읽으면 그 자리마다 정책을 쓰거나(`-- policy: 이름 별칭`과 `이름(별칭)`, WHERE의 AND 항) `-- policy: none 별칭 까닭`". 같은 커밋에서 "five"를 "six"로(`topics.go`와 그 수를 보는 시험: 진단의 "rule 6"과 문서가 한 커밋도 어긋나지 않게, fable). 표 함수가 읽는 표는 보지 않는다는 한 줄(codex). D-24의 "5개"·PHILOSOPHY 3·주제의 긴 글은 8번(Q4)
   - **하지 않음**: 앱을 넘는 정책 사용(오류로 막고 TODO), 표 함수 안의 읽기(규칙 글에 한 줄), 쓰기 대상의 정책(D-155), 쿼리 본문의 `:viewer_x`, 모델이 아닌 사용자 타입의 viewer 열, 선택·슬라이스 정책 param, 정책 조각 안의 표 검사(정책이 원천이다), 복사된 auth 모델에 정책을 거는 프로젝트(auth의 쿼리가 none이 없어 실패: TODO), 게시판(7번), 문서(8번)
   - → 확인
     - `TestPolicyFile`(골든, DB 없이): 블록 나누기, 이름·별칭·모델 꼴, 겹친 이름, 모르는 키, 빈 조각, `{`·`;`, 선언 안 된·쓰이지 않은 param, `-- param: viewer_x`, 선택·`?`·슬라이스 param
     - `TestPolicyCheck`(골든, DB 없이, 작은 모델 둘 `Post`·`Board`와 사용자 모델): 빠뜨림(맨 위·`EXISTS` 안·SELECT 목록 안·UNION의 둘째·CTE 몸통·별칭 없는 표), 같은 별칭 `p`의 `EXISTS` 둘 중 하나만, 까닭 없는 `none`, 같은 별칭의 정책과 `none`, 쓰이지 않는 `none`, 선언하고 안 쓴 정책, 머리와 다른 별칭, 다른 모델의 별칭, 조건 줄 위의 사용, `OR`·`NOT`·괄호 안의 사용, `WHERE { }`의 OR 줄, 모르는 `:viewer_x`, 쿼리가 다시 선언한 `now`, 타입이 다른 같은 param, 다른 앱의 쿼리, `get` 없음·둘·틀린 이름, 쓰이지 않는 정책, 사용자 뷰 없음, 정책 없는 프로젝트의 모를 모양은 오류가 아님
     - 펼친 글의 골든(`body.golden` 꼴): 정책이 `WHERE { }` 안과 조건 줄 옆(조건 없는 줄)에 있는 쿼리의 변형마다 SQL, 번호(정책 param과 쿼리 param이 섞인 차례), 조각 Part의 Pos가 `policies.sql`
     - gentest 새 fixture `policy`(두 방언, 실제 DB, 사용자 모델은 같은 앱이라 import 없음): `User{ID, Name, IsStaff, TeamID, JoinedAt}`, `Post{…, Status, Hidden, AuthorID *int64, PublishAt *time.Time}`(사용자에 포인터·시각 필드 `TeamID *int64`·`JoinedAt time.Time`도: `u.X != nil`·`rt.SQLiteTime` 갈래, fable), 정책 `out`·`visible`(`:now`·`:viewer_id`·`:viewer_is_staff`, `visible`이 get), 쿼리: 목록(out), JOIN 상세(visible), `EXISTS`, 내 글(visible + author), none. 아무도·남·작성자·staff가 같은 쿼리와 `GetPost`에서 다른 행, nil 사용자, `GetPostIgnoringPolicies`는 모두, admin의 고치기 폼이 남의 초안을 연다. 생성 파일이 골든(`TestGenerated`)
     - 정책 조각의 오타가 두 방언에서 `policies.sql`의 줄로 나는 Describe 시험(`describe_errors` 골든 꼴). 정책을 따로 prepare할 때 `:viewer_x`도 `paramType`을 지난다(viewer는 `Params`에 없다, fable). PG가 타입을 정하지 못하는 viewer 자리(`:viewer_id IS NULL`만)는 그 정책의 위치 있는 오류(codex)
     - build 시험(`internal/build`): 정책이 있고 사용자 뷰가 없으면 gen 오류, 사용자 타입이 다른 패키지면 생성 파일이 import하고 타입 검사를 지난다
     - `TestPolicyCheck`에 더함(리뷰): 별칭마다 none(정책 표가 아닌 별칭의 none은 오류, `visible p` + `none c` 한 쿼리는 깨끗), 쿼리가 같은 타입으로 다시 선언한 `now`는 깨끗, 3필드 `-- policy:`, 모를 모양의 none, 파생 표 별칭, CTE 몸통에 정책을 쓰고 바깥이 CTE를 읽는 긍정, `LEFT JOIN … ON visible(c)`(들여온 쪽)은 받고 `ON visible(p)`(남는 쪽)·`RIGHT JOIN`·`HAVING`은 거절, `CASE WHEN visible(p)`·`COALESCE(visible(p), true)`·`visible(p) IS NOT FALSE`·`NOT visible(p)`·`NOT (visible(p))` 거절, `AND (visible(p) AND y)` 받음, 조건 줄 위 같은 줄의 EXISTS 받음, `policies.sql` 없는 앱의 `-- policy:`, 생성 이름 `maagouser`가 views_gen과 같은 패키지에서 겹치지 않음
     - 회귀: 나온 자리마다를 별칭 이름의 수로 바꾸면 `EXISTS` 둘 사례가, UNION의 새 블록을 빼면 UNION 사례가, 조건 줄 검사를 빼면 그 사례가, 앞뒤 토큰 검사를 빼면 CASE·COALESCE 사례가 실패. 펼치기의 골든은 `WHERE { }` 첫 줄에 정책이 있어 Lead 정리가 조각 앞에서 일어나는 것을 본다(fable)
     - `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG 포함)
   - 계획 리뷰(codex·fable) 반영: `-- get:`(둘 다 (a)), none은 별칭마다(fable, D-155의 글. codex의 쿼리 전체는 받지 않음: 위), 바깥 조인의 남는 쪽·`HAVING`(둘 다)과 CASE·COALESCE·`IS NOT FALSE`(codex)를 막는 AND 항 검사, 괄호 안의 AND 항은 받음(fable), 같은 줄 EXISTS는 받음(fable), 정책 블록은 `-- name:`(fable, PHILOSOPHY 4), 같은 타입의 param 다시 선언은 하나로(fable, Q3 고침), 앱을 넘는 쿼리는 none이 아니라 오류(fable), "five"→"six"를 이 커밋에(fable), 모를 모양이 none 미사용을 이김, 사용자 타입은 web의 잠정 값(codex), PG가 타입을 모르는 viewer 자리와 포인터·시각 필드의 시험(둘 다), 표 함수 한 줄(codex)
   - 구현에서: 정책 조각은 토큰마다 Part이고 토큰 사이의 줄바꿈·주석은 빈칸 하나(펼친 정책이 줄 가운데 들어가므로 `--` 주석이 남으면 쿼리의 나머지를 지운다). 사용 뒤에 조건 줄의 `{`가 와도 받는다(`WHERE { out(p) / {q} AND …}`: 그 줄의 OR는 앞으로 훑기가 잡는다, gentest가 찾음). `Viewer`는 모델이 아닌 사용자 타입이면 `Model` nil, 표현은 `auth.User`. 사용자 타입이 다른 패키지인 경우(`maagouser`)는 `internal/build` 시험에 드라이버가 없어 7번 게시판이 처음 컴파일한다. `TestDocExamplesUseEveryKey`가 머리 키마다 예를 요구해 `maago doc query`의 예(`cmd/maago/docs/query.txt`)에 정책·`maago add auth`·API 뷰·권한 표를 이 작업에서 넣었다(주제의 글은 8번). `TestSearchOrdersIsTheDecisionsExample`이 "SQL 규칙 5개" 제목을 찾는다: "6개"로. PG가 viewer 열의 타입을 다르게 읽으면 fix가 "선언하라"가 아니라 그 열과 비교하라(viewer는 선언하지 않는다). 전체 시험 한 번에서 `TestBoardTutorial`의 0014(정책 없는 단계)가 maagotest의 "no schema has been selected to create in"으로 한 번 실패했다: 단독 실행과 전체 재실행에서 통과, 재현되지 않음(간헐 실패로 적어 둔다)
   - 구현 리뷰: ponytail(직접: 파서의 블록 셈 단순화), codex(코드만 읽은 리뷰, 샌드박스: 한 별칭이 UNION 양쪽에서 다른 표면 none 하나가 둘 다 덮는다(둘 다 정책 표여도, 리드가 짚음) → none의 별칭이 서로 다른 표를 가리키면 오류(같은 표를 여러 블록에서 읽는 것은 받는다). 받지 않음: `Probe`의 nil 모델(부르는 곳이 모두 `CheckPolicies` 뒤, 문서 주석에), 표 이름만으로 된 `byTable`(표 이름은 프로젝트에서 하나다: DDL이 겹치면 실패한다)), fable(이미 고친 "SQL 규칙 5개" 시험, 정책 이름과 같은 열 `p.out`·`AS out`이 "used without its alias" → 뒤가 `(`가 아니면 사용이 아니다, GetX의 정책 param을 선언 차례로(자리표시는 쓰인 차례), `id`·`u`·`db`·`ctx` param 거절, 정책 없는 프로젝트의 모를 모양 시험, PG viewer 타입 시험, `;`의 자리, 토큰 오류의 정책을 broken으로 남김, 죽은 `find`, viewer 열 반복의 차례. 받지 않음: dot import한 사용자 타입이 "no view takes the user"가 되는 것(드물고 views는 이미 dot import를 받지 않는 쪽: TODO), `TestPolicyCheck`가 첫 진단 줄만 비교(첫 줄이 그 사례의 진단이고, 연쇄는 세지 않는다))
7. **게시판 0035~0036: 정책**: 0035는 정책을 빠뜨린 쿼리(`Maago-Expect`), 0036은 `out`·`visible` 두 정책으로 `maySee`와 쿼리 넷의 조건을 접는다(`MyPosts` 포함). `RecentPosts`가 `none`의 본보기 → 확인: `TestBoardTutorial`, `TestPermissions`의 네 행(남의 초안에 bob의 `post_publish`·`post_edit`·`post_change`·`post_delete`)이 403 → 404로 바뀌고 0036 글이 그 넷을 이름으로(계획에서 고침: 바뀌는 넷은 `post_publish` 둘과 `api_post_publish` 둘), `TestDrafts`에 남은 out·웹훅은 그대로, Catches의 "아무나 초안 보기"가 `go test`가 아니라 생성 시점에 잡힌다

   **계획**
   - 새 단계 둘(`steps.sh unpack`, 커밋 둘 더, `pack`). 0013부터 PG
   - **0035 `Write-who-may-read-a-post`(Maago-Expect)**: `board/policies.sql`과 쿼리의 머리·사용만(Go는 0036, Expect 단계는 컴파일하지 않는다). 정책 둘:
     - `out Post p`(`-- param: now time.Time`): `NOT p.hidden AND p.status <> 'draft' AND (p.publish_at IS NULL OR p.publish_at <= :now)`, 지금 목록 셋에 복사된 조건
     - `visible Post p`(`-- get: GetPost`, `-- param: now`): `NOT p.hidden AND (p.status <> 'draft' AND (p.publish_at IS NULL OR p.publish_at <= :now) OR p.author_id = :viewer_id OR :viewer_is_staff)`, `maySee`를 SQL로. 보관한 글은 지금처럼 모두에게
     - 쿼리: `post_list`·`search_posts`·`export_rows`는 `-- policy: out p`와 `out(p)`(손으로 쓴 조건을 지운다. `post_list`는 `-- param: now`를 그대로 둔다: `ORDER BY`가 쓰고 같은 이름·타입은 하나. `search_posts`·`export_rows`는 조건에만 썼으므로 지운다, fable), `my_posts`는 `visible(p)`(`NOT p.hidden`을 지운다), `recent_posts`는 `-- policy: none board_post` 까닭 "the newest posts for the tests, drafts too"(호출자는 시험뿐, fable), `pinned_overlap`은 `-- policy: none board_post` 까닭 "a count of the notices pinned at once, drafts too, which shows no post"(5번이 넘긴 것, 별칭 없는 표). **`post_detail`을 빠뜨린다**: 31단계를 산 누출과 같은 자리(`postPage`의 `maySee`에 기댄 쿼리). `Maago-Expect: board/queries/post_detail.sql:<줄>:<열>: query reads board_post p without a policy (rule 6)` 한 줄(다른 오류 없음: 정책은 Go를 보지 않는다)
     - 단계 글: 초안 조건이 네 곳(쿼리 셋과 `maySee`)에 복사된 것, `policies.sql` 전문과 쿼리 머리 줄의 예(fable: 게이트가 글만 본다), fix가 그 모델의 첫 정책(`out`)을 먼저 늘어놓으니 상세에는 `visible`이라는 한 줄, 정책 파일의 모양(`-- name:`·`-- param:`·`-- get:`·`:viewer_<열>`), 쿼리의 선언·사용, `none`과 그 까닭, 빠뜨린 `post_detail`이 생성 시점에 멈춘다는 것(진단 인용)
   - **0036 `Read-posts-under-the-policies`**: `post_detail`에 `-- policy: visible p`와 `AND visible(p)`(`NOT p.hidden`을 지운다). Go:
     - 정책을 쓰는 쿼리와 `GetPost`가 사용자를 받는다: `ShowBoard`·`APIPosts`가 `u *auth.User`를 받고(포인터: 누구나 그대로), `ExportBoard`(func(w, r))는 `DownloadAttachment`처럼 `maago.UserFrom`에서 읽는다, `Me`의 `MyPosts(ctx, &u, db, …{AuthorID, Now})`(codex). `GetPost`의 호출자 모두(`Attach`·`DownloadAttachment`·`RemovePost`·`moveTo`·`editable`, `postPage`는 `PostDetail`로)가 `GetPost(ctx, u, db, id, time.Now())`(codex)
     - `maySee`를 지운다: `postPage`는 `GetPost`+`maySee` 대신 `PostDetail`(visible) 하나, `DownloadAttachment`는 `GetPost`가 404. `Attach`·`editable`의 `p.Hidden` 검사도 지운다(정책이 숨긴 글을 읽지 않는다). `mayChange`는 그대로(쓰기는 정책 밖)
     - 시험: 다 읽어야 하는 시험의 `GetPost`(쓴 것을 다시 읽기)는 `GetPostIgnoringPolicies`, `queries_test`의 쿼리 호출은 `nil`과 `Now`. `TestPermissions`: 정책이 바꾸는 행은 초안의 행이다. `post_publish`(ann의 초안: bob 403 → 404, 작성자 없는 초안: bob·ann 403 → 404)와 `api_post_publish`(같은 둘). **고침**: 확인 조건의 "`post_edit`·`post_change`·`post_delete`"는 그 행들의 글이 게시된 글이라(지금 표에서) 바뀌지 않는다. 바뀌는 넷은 `post_publish` 둘과 `api_post_publish` 둘이고 0036 글이 그 넷을 이름으로 말한다(남의 초안은 고칠 수 없는 글이 아니라 없는 글: D-147의 모양). `TestDrafts`의 `out`·웹훅·댓글 수는 그대로
     - 바뀌지 않는 것(codex, 표와 시험이 그대로 지킨다): 보관한 글은 모두에게, 예약 글은 그때까지 작성자·staff만, 초안의 파일은 작성자·staff만, 초안의 댓글은 작성자·staff만(`comment_create`의 404), CSV는 out만, admin은 모두
     - 단계 글: 새 서명(`GetPost(ctx, u, db, id, now)`, `PostList(ctx, u, db, p)`), 정책 있는 `GetPost`가 `visible`을 쓰고 `GetPostIgnoringPolicies`는 admin·시험의 길, 표의 diff가 403 → 404의 넷, `maySee`가 지워진 자리
   - **Catches**: 0036의 `+-- policy: visible p` 머리 줄을 지운다 → `maago gen:`(아무나 초안 보기가 생성 시점에, 규칙 6의 진단. 본문의 사용을 지우면 "policy visible is not used"가 나서 규칙 6이 아니다, fable). 0032의 "a draft to anybody"(`maySee`)는 그대로(그 단계의 시험이 잡는다)
   - `MAP.md`: 0035·0036은 "원본에 없는 단계" 표에 두 행(`TestBoardMap`이 모든 단계의 이름을 요구한다, fable), 원본의 초안 행 칸에 "0032·0036"(0034의 꼴)
   - 하지 않음: 댓글·파일의 정책(그 표는 글을 통해서만 보인다: 파일은 `GetPost`가 거른다), admin의 정책(정책 없는 길), 0037 이후
   - 계획 리뷰(codex·fable) 반영: `Me`, `GetPost` 호출자 목록, 확인 조건의 고침을 항목 글에도, 바뀌지 않는 것, MAP의 표, Catches의 머리 줄, 단계 글의 정책 전문·서명·fix 순서, 쓰지 않는 `-- param: now`. Expect의 자리는 리뷰 둘의 짐작(5:6·5:17)이 갈려 돌려서 정한다. 받지 않음: 초안 고치기 행 더하기(fable의 선택: 쓰는 행의 fixture가 늘고 `post_publish`가 같은 모양을 이미 보인다)
   - 구현에서: Expect의 자리는 돌려서 5:6(진단은 그 하나). 0036 뒤 `TestPermissions`가 실패한 행은 계획대로 정확히 넷, 나머지 시험은 그대로 초록. MAP의 "원본의 초안 행"은 없다(0032는 원본에 없는 단계의 표에만): 새 두 행만. Catches는 줄을 지우지 않고 주석 줄로 바꾼다(hunk의 줄 수, 4번의 함정). 다른 패키지의 사용자 타입(`maagouser`)을 처음 컴파일한 것이 이 단계다. 5번의 말뭉치 골든(`TestTablesOfQueries`)이 게시판 쿼리를 읽으므로 `policies.sql`도 `git apply`하고 정책을 읽는다(골든은 줄 번호만 바뀜)
   - 구현 리뷰: ponytail(직접, 지울 것 없음), codex(코드만 읽은 리뷰, 샌드박스: 미래 `publish_at`의 보관 글은 보이지 않는다 → 전의 `maySee`도 그랬다, 0036 글을 "out by its time"으로. 받지 않음: `PostDetailParams`의 `Now` 없는 시험 호출이 컴파일되지 않는다(Go의 struct 리터럴은 빠진 필드를 0으로 채운다, 시험이 돈다)), fable(0036 `post_detail`의 군더더기 `-- param: now` 탓에 Catches가 규칙 6이 아니라 "parameter now is not used"로 통과 → 줄을 지우고 Catches의 `by`를 규칙 6의 진단 줄까지로, 0035 글의 "copies only the hidden half", 숨긴 글은 작성자·staff도 게시·보관·지우지 못한다는 반 줄을 0036 글에)
   - → 확인: 단계마다 걷기 초록·`TestBoardTutorial`(PG)·Catches의 새 줄, 0035의 Expect 한 줄, `TestPermissions`의 diff가 그 넷, `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG)
8. **문서**: `maago doc`의 query(정책, 규칙)·form(`Fail`)·권한 표(maagotest 또는 view 주제), MISSION의 범위 문장(D-147), PHILOSOPHY 3, D-24·D-119·D-147 고침 → 확인: `TestDocExamples`(새 예), `TestPackageDoc`, `TestNoDecisionsInUserText`

   **계획**
   - 이미 된 것(앞 작업): form 주제의 `Fail`(1번), D-119의 대가·기각 고침(1번), query 주제의 규칙 목록 여섯째와 "six"·`PolicyKeys` 목록·예의 정책과 권한 표(6번), D-155 고침(6번). 여기서는 다시 쓰지 않는다
   - **query 주제의 글**(`internal/ref/topics.go`, 규칙 목록 뒤): "Read policies" 단락. 게이트(10번)가 `maago doc query`만 보고 쓴다고 보고, 6·7번이 정한 것을 빠짐없이 짧게
     - `policies.sql`은 앱의 `queries/` 옆, 블록 `-- name: 이름 모델 별칭`·`-- param:`(필수 한 타입)·`-- get: GetX`(정책 있는 모델마다 하나), 조건 한 조각, `:viewer_<열>`은 뷰가 받는 사용자 모델의 열(사용자 없음은 NULL: WHERE가 버린다), 예는 게시판의 `visible`
     - 쿼리: `-- policy: 이름 별칭`과 그 표를 읽는 SELECT의 WHERE에 AND 항 `이름(별칭)`(또는 그 별칭을 들여온 JOIN의 ON, RIGHT·FULL은 아님), 별칭 없는 표는 표 이름, `{조건}` 줄에는 쓰지 않는다, `-- policy: none 별칭 까닭`, 다른 앱의 쿼리는 그 표를 읽지 못한다, 정책 param은 쿼리의 Params에 합쳐지고 같은 이름·타입으로 다시 선언해도 하나
     - 생성되는 것: 정책을 쓰는 함수는 `(ctx, u *User, db, p)`, `GetX(ctx, u, db, id[, 정책 param…])`(선언 차례), `GetXIgnoringPolicies(ctx, db, id)`(admin·모두 읽는 시험), 쓰기는 정책 밖, 정책이 있으면 모르는 FROM 모양(괄호 친 조인, `TABLE t`)이 오류
   - **view 주제의 글**: 권한 표 단락. `maagotest.Permissions(t, apps, roles, rows...)`, `Role`·`Row`·`maagotest.Skip`·`maagotest.As`, 로그인한 역할은 복사된 auth의 `auth.SignIn`, 라우트마다 행·행마다 모든 역할, 쓰는 행은 성공하는 역할마다 fixture, 사용자를 받는 뷰가 있는 앱은 `maago gen`이 그 호출을 요구한다, admin의 앱은 목록에 넣어야 보인다. 예는 이미 있는 view 주제 예의 `TestPermissions`(4번)
   - **설계 문서**: MISSION의 둘째 문장(기계가 이제 막는 빠짐 둘: 정책을 빠뜨린 쿼리는 생성 시점, 행 없는 라우트는 `go test`. 권한·동시성·보존의 맞음은 여전히 시험), PHILOSOPHY 3의 "다섯"을 "여섯"으로(M11에서 하나 늘었고 그 까닭 한 줄), D-24 "아래 규칙 6개"와 고침 줄(Q4, M11 6번), D-147에 M11에서 넓힌 것과 MISSION 고침, DECISIONS의 "이력"에 2026-10-23 줄(D-154~D-156의 고침), ARCHITECTURE의 mapper 줄에 정책(policy.go·policycheck.go)
   - 하지 않음: 예를 더 늘리기(query의 예가 이미 정책·none·GetX를 쓴다), 영어 단계 글 고치기(7번), TUTORIAL-FUTURE
   - 계획 리뷰는 fable 하나: 문서 작업(WORKFLOW 3)
   - 계획 리뷰(fable) 반영: 정책이 가린 행은 그 사용자에게 `sql.ErrNoRows`(404, 403이 아니다)를 query 글에, model 주제의 `GetX` 서명 옆에 정책 모델의 반 줄, policies.sql 단락은 `PolicyKeys` 목록과 규칙 6이 말하지 않는 것만(`queries/` 옆, `{조건}` 없음, `:viewer_<열>`과 nil의 NULL, param 합치기), view 단락은 예의 주석이 말하지 않는 것만(`maagotest.As`와 `auth.SignIn`, admin의 App은 목록에 넣어야 보인다, URL은 그 라우트의 것, `Row`의 자세한 것은 `go doc maagotest`), ARCHITECTURE는 mapper 줄에 "읽기 정책(policies.sql), D-155"와 web 줄에 "권한 표 호출 검사, D-156"(파일 이름이 아니라 그 줄의 꼴), DECISIONS 이력은 작업마다 한 줄(1·3·4·6번과 8번), query 예의 `Show`에 `GetPostIgnoringPolicies` 호출(이름이 바뀌면 `TestDocExamples`가 실패), 진단 둘("used by no query", "no view takes the user")과 admin의 `GetXIgnoringPolicies`에 한 마디. `maagotest.Skip`은 예에 자연스러운 행이 없어 손으로 본다. 주제 글의 D-번호는 `TestNoDecisionsInStrings`가 본다
   - 구현에서: query 주제의 정책 예는 예 파일(`query.txt`)의 `visible`과 글자까지 같게(두 벌이 어긋나지 않게). `TestPackageDoc`은 주제의 글을 보지 않아 `doc.go`는 그대로
   - 구현 리뷰는 fable 하나(문서 작업): "쓰이지 않는 정책"은 `-- get:`이 이름 붙인 것을 빼고 → "neither a query nor -- get: uses", 이력 줄의 차례(1번을 초안 뒤로), ARCHITECTURE의 maagotest 줄에 `Permissions`·`As`, admin은 목록·수를 제 SELECT로 읽고 폼만 `GetXIgnoringPolicies`로. 받지 않음: "never on a {condition} line"을 코드만큼 정확히(같은 줄의 조건부 JOIN은 받는다: 에이전트에게는 "never"가 안전한 읽기), D-24의 출처(초안의 사람의 결정이 맞다: 계획의 "M11 6번"이 틀렸다)
   - → 확인: `TestDocExamples`·`TestDocKeysAreRead`·`TestDocExamplesUseEveryKey`, `TestPackageDoc`(-update 뒤 diff를 읽는다), `TestNoDecisionsInUserText`, 글에 쓴 이름이 코드와 같은지(`GetXIgnoringPolicies`, `maagotest.Skip`, `auth.SignIn`, 진단 글), `gofmt -l .`, `go mod tidy -diff && go vet ./... && go test ./...`(PG)
9. **v0.2.0**: 태그, 게시판의 `go.mod`를 v0.2.0으로(0001·0013, 12번의 길) → 확인: 빈 GOPATH에서 `go install …@v0.2.0`, 공개 판으로 걷기의 끝

   **사람의 결정(2026-10-23)**: 진행 승인, 이번에 한해 `v0.2.0` 태그를 만들어 push(다른 태그 없음, force-push 없음). 태그는 PG 전체 시험이 통과한 커밋에. GetX의 정책 param은 위치 인자 그대로(D-155에 적음)

   **계획**(M10 12번의 길. 계획 리뷰 없음: 사람의 결정과 정해진 확인만인 작업, M10 12번과 같은 까닭)
   - 차례: 이 계획 커밋 → PG 전체 시험(`go test`의 종료 코드를 로그에서) → 그 커밋에 `v0.2.0` 태그·push → 확인 셋: `git ls-remote --tags origin v0.2.0`, 빈 `GOPATH`·`GOMODCACHE`·`GOWORK=off`에서 `GOPROXY=direct`와 기본 프록시 둘 다 `go install github.com/maago-web/maago/cmd/maago@v0.2.0`과 `maago help`, 바이너리의 `go version -m`이 `github.com/maago-web/maago v0.2.0`
   - 게시판: `steps.sh unpack`, 0001의 `go.mod`·`go.sum`(require와 두 줄)과 0013의 문맥 줄을 v0.2.0으로(체크섬은 기본 프록시와 sum.golang.org가 확인한 `go mod download -json`의 것), 그 사이 커밋은 rebase가 나르고 `pack`. 태그는 게시판 커밋보다 앞이다: 걷기는 `go.work`로 로컬 모듈을 보므로 게시판 커밋이 태그를 기다리지 않는다
   - 공개 판으로 걷기의 끝: 마지막 단계의 사본에서 `go.work` 없이(`GOWORK=off`), 설치한 v0.2.0 `maago`로 PG에서 `migrate`·`gen`·`check`, `go mod tidy -diff`·`go mod verify`·`go vet ./...`·`go test ./...`
   - 그 뒤 `TestBoardTutorial`과 전체 시험(게시판 패치가 바뀌었으므로), 커밋·push. 공개 뒤 고칠 것이 나오면 태그를 옮기지 않고 v0.2.1(12번의 규칙)
   - 기록(2026-10-23): 909069c에서 PG 전체 시험(`go test exit=0`) 뒤 `v0.2.0`(주석 태그, v0.1.0과 같은 꼴)을 그 커밋에 달아 push, `git ls-remote --tags origin v0.2.0`. 빈 `GOPATH`·`GOMODCACHE`·`GOCACHE`·`GOWORK=off`에서 `GOPROXY=direct`와 기본 프록시 둘 다 `go install github.com/maago-web/maago/cmd/maago@v0.2.0`, `maago help`, `go version -m`이 `github.com/maago-web/maago v0.2.0 h1:vy2v09FW…`. 게시판: 0001·0013의 `go.mod`·`go.sum`을 v0.2.0으로(체크섬은 기본 프록시·sum.golang.org의 `go mod download -json`, `go.mod`의 것은 v0.1.0과 같다), `unpack`·`pack`. 공개 판으로 걷기의 끝: 마지막 단계의 사본(`git archive`)에서 `GOWORK=off`, 설치한 v0.2.0 `maago`로 새 PG 데이터베이스에 `add auth`·`migrate`·`gen`·`check`, `go mod tidy -diff`·`go mod verify`·`go vet ./...`·`go test ./...` 초록
10. **에이전트 게이트**: `go install …@v0.2.0`부터, 0033의 끝에서 시작해 새 단계(0034~0036) 글만 보고 서로 모르는 실행 둘. 0033의 끝에 표가 이미 있으므로(4번) 게이트는 표를 처음부터가 아니라 정책이 바꾼 행으로 쓴다 → 확인: 기록, 셋을 메시지와 `maago doc`만으로 씀
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
