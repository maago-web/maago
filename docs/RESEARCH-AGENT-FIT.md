# 연구: Maago는 코딩 에이전트에게 정말 편하고 안전한가 (M9 9번)

2026-10-15. 물음은 사람의 것(M9 차례를 바꾼 날): 지금까지 알게 된 것으로 (1) Claude·Codex 같은 코딩 에이전트가 정말 쓰기
편한 프레임워크인지, (2) 구조적으로 바꿀 것, (3) 없는 것, (4) 이것을 모두 알고 처음부터 다시 쓴다면 어떻게 할지.
여기의 제안은 아직 결정이 아니다. 끝의 [사람이 정할 것](#사람이-정할-것)에 답을 받은 뒤 DECISIONS·MILESTONES에 반영한다(M9 10번).

## 근거

| 근거 | 무엇 | 어디 |
|---|---|---|
| 에이전트 게이트: 짧은 튜토리얼 | 새 서브에이전트 둘, 코드는 못 보고 문서와 `maago`만, §2~5 | [gate-logs/short-1.md](gate-logs/short-1.md)·[short-2.md](gate-logs/short-2.md), TASKS-M9 6번 |
| 에이전트 게이트: 게시판 | 새 서브에이전트 둘, 단계 0001~0012의 **메시지만**(코드는 에이전트가), API는 `go doc`만 | [gate-logs/board-1.md](gate-logs/board-1.md)·[board-2.md](gate-logs/board-2.md), TASKS-M9 7번 |
| 만든 기록 | M0~M8의 계획·리뷰 지적 약 600건·결정, 게시판 33단계 | TASKS-M0~M8, DECISIONS, `examples/board` |
| 처음부터 다시 쓴다면 | 같은 증거를 받은 codex와 Opus 서브에이전트가 따로 낸 안(서로와 이 문서의 의견을 모른 채) | 아래 본문에 요약, codex 전문은 [research/codex-redesign.md](research/codex-redesign.md) |
| 실측 | 아래 본문의 "실측" 표시 | 이 문서를 쓰며 돌린 것 |

## 1. 판정

**쉽다. 그러나 안전하지는 않다.** 그리고 MISSION의 "에이전트가 틀릴 수 있는 자리를 **모두** 생성 시점으로 당겨와 막는다"는 지금 사실이 아니다.
기계가 막는 것은 *모양*의 틀림(태그, SQL, 타입, 템플릿의 이름과 필드)이다. 서비스를 실제로 망가뜨린 것은 *의미*의 틀림(권한,
동시성, 데이터 보존)이었고, 그것은 리뷰어와 앱의 시험이 잡았다. 세 의견(나, codex, Opus)이 이 판정에서 같다.

### 1.1 쉬운 까닭: 증거가 확인한 내기

- 게시판 게이트의 두 에이전트가 코드 없이 메시지만으로 12단계를 각 약 8분에 끝냈고, 계획에 없던 Maago 오류의 11/12·9/10을
  **메시지만 보고** 고쳤다. 짧은 튜토리얼은 두 실행 모두 뜻밖의 실패 0, 139초·132초(D-69 통과)
- 닫힌 목록과 `fix:` 줄(D-17·D-62), 한 단계의 독립 오류를 한 번에(D-61), 오류가 있으면 아무것도 쓰지 않음(D-60)
- **생성 시점에 실제 방언의 연습용 DB로 SQL을 prepare**(D-35): 가장 강한 자산. 마이그레이션을 적용한 결과를 스키마로 쓰는 것(D-34)은 sqlc보다 정확하다
- 템플릿의 이름·필드 검사(`{{define}}` 안까지), 라우트 이름 오타, 뷰의 Go 타입 오류를 생성이 잡는다
- 속도는 문제가 아니다: 합성 예제(앱 5, 모델 30, 쿼리 100)에서 SQLite 145ms, PG 368ms(D-65 한도의 1/7·1/8)

**그러나 쉬움의 일부는 운이다.** 게시판 에이전트의 기록에는 오류 없이 받아들여진 짐작 14개가 있다(`fk=Board`, `unique=(BoardID,Title)`,
`-- name: X :many T`, `maago:"path=id"`, `Validate`의 서명, `base.html`의 `{{block "content" .}}` …). 짐작이 맞은 것은 Django·sqlc를 닮았기
때문이고, **문법은 `go doc`에 없다**(루트 패키지 문서 36줄, 핵심 개념 하나도 없음, 진짜 명세는 한국어 DECISIONS 303줄의 표). 틀린 짐작이
오류 없이 통과하는 자리에서 운은 사고가 된다(1.2 (c)).

### 1.2 안전하지 않은 까닭: 원인이 셋

**(a) 권한: 무엇이 사용자의 범위인지 기계가 모른다.**
- M6~M8의 보안 버그 8개 중 정적으로 잡힌 것 0. 초안이 누구에게나 보이는 누출이 튜토리얼 31단계를 살아 있다가 리뷰어가 찾았다(M8 4번 → 0032)
- `TestBoardTutorialCatches`의 권한 돌연변이(작성자 검사 빼기, 초안 첨부, 로그인 없는 `/me`, 아무나 고치기, 아무나 초안 보기)는 모두
  `go test`만 잡는다. 그 시험은 **같은 에이전트가 쓴다**: 기계의 검증이 아니라 셀프 채점이다
- 완성본에서 "초안은 작성자만"은 **네 곳에 손으로 복사**되어 있다(SQL 셋 `post_list`·`search_posts`·`export_rows`와 Go의 `maySee`). 다섯째 쿼리에서 빠뜨려도 아무것도 실패하지 않는다. 31단계의 누출이 정확히 이 모양이었다
- 숨은 결합: `u, _ := maago.UserFrom[auth.User](ctx) // the routes require login`(셋). 라우트 표에서 `LoginRequired`가 빠지면 `u`는 영값이고, `Me`는 조용히 "사용자 0의 글"을 보인다

**(b) 동시성·데이터 보존: 설계가 경쟁을 받아들였다.**
- 앱 코드의 트랜잭션 0(복사된 auth는 4번 쓴다: 규칙이 아니라 작성자의 감각). `DBFrom`은 `*sql.DB`
- 잃어버린 갱신: 생성된 `UpdatePost`는 열 11개를 통째로 쓴다. `EditPost`가 `Publish`와 겹치면 published가 draft로 되돌아간다
- 데이터 손실 경로: `deletePost`가 트랜잭션 없이 첨부 행부터 지우고 글을 지운다. 그 사이 댓글이 달리면 글은 FK로 남고 첨부 행은 사라지고 파일은 고아
- 같은 제목 검사 뒤 INSERT: 경쟁에서 지면 500(D-119가 받아들인 대가)
- 응답 뒤 고루틴은 서버가 꺼지면 사라진다(D-118), 서버에 타임아웃이 없다(`http.ListenAndServe`)

**(c) 틀린 코드가 맞아 보이는 API.**
- `f.Title.Error = "..."`를 뷰가 적어도 `f.Valid()`는 참이다(`form.go`: `bound && ok && len(Errors)==0`). 다음 줄의 INSERT가 unique에 걸려 500. 게시판 게이트의 에이전트가 찾았다. `AddError`는 세고 `Field.Error`는 세지 않는다: 한 기호 한 뜻(PHILOSOPHY 4)을 런타임 API가 어긴다
- 세션 쿠키가 http://localhost에서도 `Secure`: curl의 쿠키 저장소가 버려 로그인이 조용히 돈다(짧은 튜토리얼 게이트, Safari는 0021에서)
- 이름 바꾸기의 ⚠는 마이그레이션 파일 안에만 있다. 파일을 읽지 않는 에이전트는 DROP+ADD를 적용한다

### 1.3 원칙을 시험함: "장황함은 싸고, 모호함과 숨은 규칙은 비싸다"

**대체로 맞다.** `maago.Result[X]{}` 90번, `-- param:`, `SeeOther[P]`에 불평한 에이전트는 없다(`-- param:`은 튜토리얼에 없었는데 오류 하나로
배웠다). 비용이 든 자리는 모두 숨은 규칙이었다: `choices`의 구분자 `/`(오류가 틀린 `draft|published`를 맞는 값처럼 되돌려 보였다), `:one`의
스칼라 불가, 모델 폼의 포인터 필드가 optional이 아님, `Secure` 쿠키, `Valid()`의 뜻, 같은 경로의 GET·POST에 이름 둘, 성공한 `gen`·
`-createsuperuser`가 아무것도 찍지 않음, 오류의 `(D-114)`.

**다듬을 점(Opus의 제안을 받는다): 장황함은 *검사될 때만* 싸다.** 기계가 원천과 대조하는 장황함(`-- param:`, `-- column:`, `Result[P]`)은
공짜지만, 대조하지 않는 *복사*는 버그 농장이다(초안 조건 4벌, 새 필드마다 `:many Post`의 열 목록을 다시 고친 일: 0012·0015·0022).
제안: **"장황함은 싸다. 단, 반복된 사실마다 기계가 대조할 원천이 하나 있어야 한다."**

**문서도 같은 원칙에 걸린다.** 문서 낡음이 마일스톤마다 가장 꾸준한 버그였다(약 57건). 시험이 읽는 문서만 낡지 않았다: 튜토리얼 §2의
드라이버 줄은 M6의 손 실측을 지나 M9에서 시험이 문서를 읽고서야 드러났고, DECISIONS의 첫 줄은 열흘 동안 깨져 있었다(4c0d8f5에서 고침).

## 2. 지금의 Maago에 할 구조 변경 (에이전트에게 미치는 영향 순)

세 의견이 모두 낸 것은 ●, 둘은 ◐. "깨짐"은 API·튜토리얼·복사본을 깨는지.

1. ● **참조 문서를 검사기의 닫힌 목록에서 만든다.** `maago doc <주제>`(태그, 쿼리 파일, 조건 줄, In 태그, 뷰·폼의 약속, 템플릿, admin URL·필드)를
   검사기가 쓰는 같은 표에서 찍고, `go doc maago`의 패키지 문서도 그 표로 생성한다. 문서의 모든 코드 블록은 시험이 빈 프로젝트에서 돌린다
   (`TestShortTutorial`의 꼴). 진단의 `(D-xxx)`를 `see: maago doc tags#choices`로. 사용자 문서는 영어(오류가 이미 영어, D-06), 설계 기록은
   한국어로 둔다. 생성 코드만 쓰는 export 약 40개(91개 중)는 하위 패키지(예: `maago/rt`)로 옮겨 `go doc maago`에 사용자 API만 남긴다.
   비용: 중간(닫힌 목록은 이미 코드에 있다). 깨짐: 생성물의 import 경로(재생성으로 끝)
2. ● **트랜잭션을 기본 경로로.** 쓰기 뷰는 요청 트랜잭션 안에서 돌고(Django의 `ATOMIC_REQUESTS`), 커밋 뒤의 일은 `maago.AfterCommit(ctx, f)`(서버가
   꺼질 때 기다린다). 생성된 Update는 `auto=update` 열을 전제 조건으로(`WHERE id=… AND updated_at=…`, 0행이면 409 `ErrConflict`, Rails의
   `lock_version`), 고친 열만 쓰는 `UpdateXFields`도 생성. unique 위반은 생성 코드가 필드 오류로(pgx `SQLState()`·modernc `Code()`가 있다: 실측,
   어느 제약인지는 D-86의 이름). 깨짐: 뷰의 `go notify`, auth 복사본의 명시 tx, admin의 "나중 저장이 이긴다", D-118·D-119
3. ◐ **사용자와 DB를 뷰 서명에.** codex는 라우트에 정책 이름(`.Require("post.create")`), Opus는 서명(`u auth.User`는 로그인 필수, `u *auth.User`는
   선택, `maago.DB`는 읽기, `maago.Tx`는 쓰기, GET 라우트에 `Tx`는 생성 오류). **서명 쪽을 추천한다**: 컴파일러가 잡고, 표와 뷰가 어긋날 길이
   없어지고, `LoginRequired`·`UserFrom`·`DBFrom`·`WithDB`가 사라진다(FastAPI의 `Depends(current_user)`, Encore의 `auth` 선언과 같은 수).
   대가: GET에 그리고 POST에 저장하는 한 함수(0011의 폼 뷰, `ShowPost`의 댓글)를 둘로 나눈다(같은 페이지 타입이면 템플릿을 함께 쓴다).
   장황해지지만 검사된다(1.3). 깨짐: 뷰 전부, 튜토리얼 0008 이후, auth 복사본
4. ● **읽기 정책(행 범위)을 생성기가 강제.** 모델 태그 `policy=visible`과 정책 SQL 하나(`-- policy: visible Post`, 조건식 한 조각), 그 표를 읽는
   쿼리는 `-- policy: visible p`(또는 `-- policy: none <까닭>`) 없이는 생성 오류, 정책을 쓰는 쿼리와 `GetX`는 `maago.Viewer`를 받아야 컴파일된다
   (보이지 않는 행은 `ErrNoRows` = 404, 시스템 작업은 `maago.System`을 일부러 넘긴다). 생성기는 쿼리가 읽는 표를 이미 안다(D-104·D-110의 출처).
   Postgres RLS는 SQLite에 없고(D-31이 두 방언), Ent의 privacy는 런타임 훅이라 손 SQL을 못 본다: 생성기가 맞는 자리다. 효과: 초안 조건 4벌이 1벌,
   31단계의 누출은 생성 시점에 막혔을 것. 대가: SQL 규칙 하나(D-24의 "다섯"이 여섯), 정책 없는 표는 그대로. 쓰기 권한은 여전히 Go 코드:
   빠짐없음은 7번의 권한 표가 본다
5. ● **모듈 하나.** `cmd/maago`를 루트 모듈에 넣고 드라이버를 그 모듈의 `require`로. 실측: 런타임 패키지만 import하는 사용자 모듈의 `go.mod`·`go.sum`에
   드라이버가 들어오지 않는다(Go의 모듈 그래프 가지치기, `replace`로 한 실험이라 프록시 경로는 릴리스 때 다시 본다). "런타임은 stdlib만"(D-09)은
   모듈 경계 대신 import 허용 목록 시험으로(D-66이 이미 이 방식). `replace`와 "두 커밋에 태그 둘"(TASKS-M9 2번의 계획)이 사라진다: 태그 하나,
   `go install …/cmd/maago@v0.1.0` 하나. 생성물에 생성기 판을 찍고 런타임이 기동 때 어긋나면 "vX로 다시 gen"
6. ● **운영의 최소선.** `maago.Serve(ctx, addr, h)`(타임아웃, 신호에 graceful shutdown, `AfterCommit` 기다리기), 바이너리에 실리는 마이그레이션
   (`//go:embed migrations` + `maago.Migrate(ctx, db, fs)`, `main`의 `-migrate`). 지금 0033의 "Ship it"은 타임아웃 없는 서버와 소스에서 돌리는
   `migrate`라, 공개 문서로는 위험하다
7. ◐ **권한 시험 표.** 라우트 × 역할(익명·본인·남·staff)의 기대 상태를 표로 적고, 표에 없는 라우트가 있으면 `maago check`가 실패(Pundit의
   `verify_authorized`). 맞는지는 시험이, 빠짐없는지는 기계가 본다. 4번이 읽기를 덮고 이것이 쓰기를 덮는다
8. ● **작은 함정 정리**(각각 작다, 공개 전에): `Valid()`가 필드의 `Error`도 센다(또는 `Field.Error`를 숨기고 `f.Fail(err)`), localhost의 TLS 아닌
   요청에는 `Secure`를 빼거나 적어도 문서에, `choices`의 구분자를 말하는 오류, 모델 폼의 포인터 필드는 기본 optional(Django의 `null=True` →
   `required=False`), `:one bool`·`:one int64` 스칼라, 이름 바꾸기·DROP의 ⚠를 stderr에, 성공한 명령의 한 줄(`generated 7 files`,
   `created superuser me@example.com (id 1)`), `go list`의 오류는 여러 줄 그대로, In의 fix에 `maago:"form"`, 모델 결과의 `SELECT *`를 문서와
   fix에(**이미 된다**: 실측, `:many Post`가 새 필드마다 깨지던 일의 답), 튜토리얼 머리말의 내부 번호
9. **타입 있는 탈출구 뷰** `func(ctx, In, w) error`: CSV·내려받기의 손 파싱과 `panic`으로 내는 500을 없앤다
10. **독립인 다음 단계까지 검사를 계속**: 단계별로 오류가 나뉘어 오는 왕복(첫 오류 `maxlen` 뒤에야 `auth.App`)을 줄인다. 연쇄를 끊는 경계를 다시 그어야 한다

## 3. 실제 서비스를 만들 에이전트에게 없는 것

트랜잭션 도우미와 커밋 뒤의 일(2번), 운영 서버와 바이너리 마이그레이션(6번), unique·FK 위반을 오류 값으로(2번), **decimal·UUID·JSON 필드**
(decimal이 없으면 에이전트는 돈을 `float64`에 넣는다, D-16·D-77), 권한 시험 표(7번), **기동 때 실패하는 설정**(지금은 `auth.SiteURL`·
`board.Webhook.URL` 같은 패키지 변수: 빠뜨려도 아무것도 실패하지 않는다), 관측(요청 로그·요청 ID·`/healthz`, 지금은 `OnError`뿐), 커밋 뒤
최소 한 번을 보장하는 outbox(지금은 프로세스가 죽으면 웹훅을 잃는다), 로컬 디스크가 아닌 미디어 저장소(인스턴스가 둘이면 깨진다: v0에서는
"인스턴스 하나"라고 크게 적는다), 운영 DB에 개발 기계가 닿지 않는 곳의 마이그레이션(TODO M8, 6번이 푼다).

## 4. 처음부터 다시 쓴다면

### 남길 것 (증거가 확인한 내기)
struct + 닫힌 태그 목록과 did-you-mean, **SQL은 파일에 손으로 쓰고 생성 시점에 실제 방언의 연습용 DB로 prepare**, 모든 진단의 `fix:`와 경고 없음,
오류면 아무것도 쓰지 않음, 생성물 커밋, 라우트 표 + URL 함수 + 템플릿 검사(templ로 바꿀 까닭 없음), 런타임 reflect 0과 stdlib, 보안 부품은
라이브러리·앱 층은 복사(D-123), 장황한 타입(`Result[P]`, `Opt[T]`).

### 바꿀 것: 개념을 넷의 층으로, 약 20묶음·90규칙에서 10개로
1. **Schema**: 모델(태그), 마이그레이션 파일
2. **Query**: 쿼리 파일(머리 + 조건 줄 + 정책 줄)
3. **Endpoint**: 라우트 표, 뷰 서명(의존을 다 말한다: `ctx`, `maago.DB`/`maago.Tx`, `U`/`*U`, In), 페이지 타입 = 템플릿, 폼(`Field[T]`·`Validate`·`Fail`)
4. **Policy**: 읽기 정책(SQL 조각)과 권한 시험 표

폼과 API는 한 입력 스키마의 두 직렬화(지금도 D-120이 그렇다). 뷰 서명이 의존을 말하면 `DBFrom`·`WithDB`·`UserFrom`·`WithUser`·
`LoginRequired`가 사라진다:

```go
func ShowPost(ctx context.Context, db maago.DB, u *auth.User, in PostIn) (maago.Result[PostPage], error)       // GET, 로그인 선택
func AddComment(ctx context.Context, tx maago.Tx, u auth.User, in AddCommentIn) (maago.Result[PostPage], error) // POST, 로그인 필수, 요청 tx

p, err := InsertPost(ctx, tx, NewPostInput{…})
if f.Fail(err) { return maago.Show(PostFormPage{b, f}), nil } // unique 위반 → 그 필드에 문구
```

### 권한과 데이터 무결성은 기계가
읽기는 정책 줄(2의 4번), 쓰기의 빠짐없음은 권한 표(2의 7번), 트랜잭션·전제 조건 있는 갱신·unique → 필드 오류(2의 2번). "기계가 비즈니스 권한을
증명한다"고 말하지는 않는다: 강제하는 것은 *빠짐없음*(정책 없는 쿼리, 표에 없는 라우트, Viewer 없는 조회가 생성 오류)이고, *맞음*은 시험이 본다.

### 참조가 에이전트에게 닿는 길: 낡을 수 없게
원천은 검사기의 표 하나. `maago doc`과 `go doc`이 그 표에서 나오고, 모든 예는 시험이 돌리고, 진단은 그 주제를 가리킨다. `maago new`(지금 v1)가
`AGENTS.md`에 "문법은 `maago doc`, maago vX"를 쓴다. 에이전트 게이트(TestShortTutorial의 꼴, 게시판 메시지만 주는 실행)를 CI의 일부로.

### 모듈·릴리스·도구
모듈 하나, 태그 하나(2의 5번). `go list -export`(D-117)는 남기되 go의 오류는 그대로 보인다. PG 프로젝트의 연습용 DB는 서버가 필요하다는 비용은
남는다(내장 PG는 새 외부 의존이라 하지 않는다).

### 일부러 만들지 않을 것
ORM과 숨은 관계 로딩(D-71), 런타임 권한 엔진(Casbin·OPA 류), 모든 SQL을 안전하다고 추정하는 분석기, 자동 마이그레이션 실행, 내장 작업 큐
(outbox 테이블 하나까지만), MySQL(방언마다 버그 약 29·시각 9가 나왔고 셋은 CI에서만 보였다: 방언 하나가 그 비용을 1.5배로), `--json` 진단,
템플릿 컴파일, down 마이그레이션, Django 전체의 복제, 자체 JS 프레임워크.

## 5. v0.1.0 전과 뒤 (추천)

**공개 전**: 2의 1번(참조 문서·`maago doc`·D-번호 빼기), 8번(작은 함정 전부, 특히 `Valid()`와 `Secure`), 5번(모듈 하나), 6번(`Serve`, 바이너리
마이그레이션), 그리고 **API를 깨는 결정 셋을 지금**: 3번(사용자·DB를 뷰 서명에, GET·POST 뷰 나누기), 2번(요청 tx·`AfterCommit`·전제 조건 있는 갱신).
지금은 깨질 외부 사용자가 0명이다. 공개 뒤에는 튜토리얼·복사본·사용자 코드를 한꺼번에 깬다. 튜토리얼 33단계는 다시 쓴다(걷기와 Catches가
그대로 지킨다).

**뒤로(v0.2~)**: unique → 필드 오류 자동화(D-119가 받아들인 대가, 데이터 손실이 아니라 500), 4번 읽기 정책(설계 합의는 공개 전에), 7번 권한 표,
decimal·UUID·JSON, outbox, 외부 미디어 저장소, 9번 탈출구 뷰, 10번 계속하는 검사, admin의 FK 고르기. MySQL·소셜 로그인·MFA·i18n은 v1 그대로.

## 사람이 정할 것

- **R1. MISSION의 "모두 막는다"**: (a) 지키는 범위로 고친다("모양의 틀림을 생성 시점에, 권한과 무결성의 *빠짐*은 생성 시점에, *맞음*은 시험으로"),
  (b) 그대로 두고 4·7번으로 범위를 넓힌다. 추천 (a) 지금, 4·7번이 들어오면 문장을 넓힌다
- **R2. 공개 전에 할 범위**: (a) 작은 함정·문서·모듈·운영(1·5·6·8번)만, API를 깨는 것은 뒤로, (b) (a) + API를 깨는 결정(2·3번)과 튜토리얼 다시 쓰기,
  (c) (b) + 읽기 정책(4번)과 권한 표(7번). 추천 (b): 깨는 값이 지금 가장 싸다. (c)는 설계가 크고 더하기에 가까워 v0.2로
- **R3. 뷰 서명(3번)**: Opus의 서명 안(추천) 또는 codex의 라우트 정책 이름(`.Require("...")`)
- **R4. 사용자 문서의 언어**: 영어(추천: 오류와 같은 언어, 공개 저장소의 독자) 또는 한국어. 설계 기록(DECISIONS·TASKS)은 한국어 그대로
- **R5. 원칙 문구**: PHILOSOPHY에 "장황함은 싸다. 단, 반복된 사실마다 기계가 대조할 원천이 하나"를 더하는지
