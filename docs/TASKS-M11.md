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
2. **게시판 0034: unique**: 먼저 읽는 검사(`BoardNameTaken`·`PostTitleTaken`, 0031의 `skip_id`)를 지우고 `Fail` 하나로 → 확인: `TestBoardTutorial`, 같은 제목의 시험이 결정적
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
