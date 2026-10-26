# M12 작업 목록 — 게이트가 넘긴 것과 맞음

[MILESTONES.md](MILESTONES.md)의 M12를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).
차례는 사람의 결정(2026-10-23): M11 게이트의 TODO, 그다음 한 페이지의 폼 둘·API 토큰 역할, 그다음 D-148의 나머지. 결정은 D-157~D-159(M12 초안).

**M12 완료 조건** (MILESTONES)
- M11 게이트가 짚은 것이 고쳐져 있다: 규칙 6의 fix가 정책을 고르지 않는다, `go doc maago.UniqueError`가 `Fail`을, `maago doc query`가 정책 param에 무엇을 넘기는지 말한다
- 한 페이지에 폼이 둘이어도 두 폼 모두 이름·라벨이 생성 코드에서 온다: 템플릿에 손으로 쓴 `name=`이 없다
- 권한 표의 API 라우트 행을 API 토큰(Bearer) 역할로 보낸다. 토큰을 만드는 도우미가 복사된 auth와 `maagotest`에 있다
- 탈출구 뷰가 입력·사용자·DB를 서명으로 받고, 쓰기 전의 오류를 `panic` 없이 돌려준다
- UUID·JSON 필드와 정수의 돈(`decimal=`)이 두 방언에서 모델 → 마이그레이션 → 쿼리 → 폼·admin을 지나고, 손 SQL의 돈이 두 방언에서 같은 값이다
- 커밋된 일(웹훅)이 프로세스가 죽어도 최소 한 번 간다(outbox)
- 게시판 튜토리얼이 이것들로 걷고(돈은 gentest `orders`), 새 판을 `go install`한 에이전트 게이트가 새 단계 글만 보고 쓴다

**앞에서 넘어온 것**
- TODO M11의 게이트 지적 둘(규칙 6의 fix, 문서 두 줄). TODO는 "v0.2.1에서 글과 함께"였으나 v0.2.1은 내지 않는다(사람의 결정): 0035 글은 저장소의 코드에 맞춰 고치고, 걷기는 `go.work`로 저장소를 보므로 초록이다. 게시판의 `go.mod`는 릴리스 작업까지 v0.2.0
- TODO M10의 둘: 한 페이지의 폼 둘, auth 시험 도우미의 남은 것(API 토큰 역할)
- D-148의 나머지 중 셋: decimal·UUID·JSON, outbox, 타입 있는 탈출구 뷰. 외부 미디어 저장소와 단계를 넘어 계속하는 검사는 M13(Q1)

**이번에 하지 않는 것**(TODO 그대로): 모델 폼의 버전 칸, 권한 표가 admin을 빼는 빈틈의 런타임 검사, `InsertX`·`GetX` 시각의 Local·UTC, `Maago-Expect`의 줄:열, admin의 거절된 삭제 200·잘못된 POST 400, `maago doc`의 주제 없음 exit 1, API 뷰의 unique 위반 422, 앱을 넘는 정책, auth 모델의 정책, dot import한 사용자 타입

---

## 먼저 확인한 사실 (2026-10-23)

- **규칙 6의 fix는 첫 정책을 고른다**: `internal/mapper/policycheck.go:229`가 `g.ps[0].Name`으로 `declare -- policy: out p and write out(p) …`를 쓰고, 괄호에 모든 정책을 늘어놓는다(`policies of Post: out, visible`). 게시판 0035 글(줄 54~58)은 그 fix를 인용하고 "the fix names the model's first policy; a post's page is visible"라고 경고한다. `Maago-Expect`(줄 60)는 진단의 글만이라 fix를 바꿔도 그대로다. 게이트 1번: 메시지 없이는 `out`을 골라 작성자에게서 초안을 숨겼을 것
- **둘째 폼은 생성 오류**: `internal/web/views.go:260`의 `BoardIn reads a second form, More` / `fix: keep one form field in In; a page with two forms posts each to its own route`. 폼의 이름·라벨은 생성된 `bindX`(소문자, `views.go.tmpl`의 `views_gen.go`)만 채우고, GET에는 값 없이 "그리기만 위해 칸을 기술"하는 길이 이미 있다. 그 함수를 앱이 부를 길이 없다(`views_gen.go`는 타입 검사 뒤에 생기므로 뷰가 부를 함수는 `forms_gen.go`에, `internal/web/gen.go:131`). CSRF 토큰은 없다(교차 출처 POST는 403, `http.go:170`). M10 게이트 둘 다 글 페이지(댓글·첨부)에서 `name="file"`·`name="text"`와 라벨을 템플릿에 손으로 썼다. POST로 다시 그릴 때 다른 폼은 `CommentForm{}`이라 이름이 빈다
- **권한 표의 역할은 쿠키뿐**: `maagotest.As(h, *http.Cookie)`, 쿠키는 auth 복사본의 `SignIn(ctx, db, u)`. 게시판의 `api_*` 행(0021~)은 쿠키 역할로 보내고 200을 받는다(세션도 API를 연다). Bearer는 모든 라우트에서 받고 쿠키보다 이긴다(D-129, `contrib/auth/session.go:57`). Bearer 경로는 `TestAPI`(0022)·`TestMe`(0030)가 손으로: `authlib.NewToken()` + `auth.InsertAPIToken(…)` + `Authorization` 머리. 권한 표는 API 토큰으로 오는 진짜 클라이언트의 길을 보지 않는다
- **탈출구 뷰는 `func(w, r)`**(D-41): 게시판 0017 `ExportBoard`는 경로 값을 `strconv.ParseInt`로 손 파싱하고, 쓰기 전 오류를 `panic(err)`로 500에 보낸다(쓴 뒤는 `abort`). `DownloadAttachment`와 `ServeMedia`(반환 없음, 안에서 panic, D-140)도 같다. `ServeMedia`는 `http.ServeContent(w, r, …)`로 Range를 다룬다(`r`가 필요하다). 0017은 묶음마다 `Flush`로 스트리밍한다. 사용자·DB는 `maago.UserFrom`·`DBFrom`(D-149가 탈출구에만 남긴 것). M11 게이트 2번: `func(w, r)` 뷰가 정책 쿼리에 넘길 사용자를 어떻게 얻는지 문서에 없다
- **필드 타입은 닫힌 목록 일곱**(D-77, `internal/model/check.go:32`): `int64`·`int32`·`bool`·`float64`·`string`·`time.Time`·`[]byte`와 포인터. 그 밖은 D-18. 런타임은 stdlib만(D-09): stdlib에 decimal·UUID 타입이 없다(`math/big`은 있다). JSON은 런타임이 이미 `encoding/json`을 쓴다(`http.go`, `rt/form.go`, D-66의 reflect 금지는 import만 본다). SQLite에는 decimal도 UUID도 없다: `NUMERIC` 친화성은 정수가 아닌 값을 REAL로 저장한다(SQLite 문서의 친화성 규칙, 이 저장소에서 잰 것은 아니다). 결과 열의 타입은 드라이버가 주는 출처뿐이라 `SUM`·식·UNION에서는 출처가 없다(D-110)
- **커밋 뒤의 일은 메모리**: `maago.AfterCommit(ctx, f)`(`tx.go:110`)는 커밋 뒤 같은 프로세스에서 돌고, `Serve`가 멈출 때 기다린다(D-153). 프로세스가 죽거나 웹훅이 실패하면 잃는다(게시판 0018의 재시도는 메모리 안). 테이블 이름 `maago_`는 런타임의 것(`maago_migrations`, TODO M2). 복사되는 SQL은 두 방언에서 돈다(D-123): `FOR UPDATE SKIP LOCKED`는 PG뿐
- **미디어는 로컬 디스크**: `SaveMedia`·`OpenMedia`·`DeleteMedia`·`ServeMedia`(`media.go`)가 `Config.MediaDir` 아래 무작위 열쇠(D-140). 인스턴스가 둘이면 깨진다
- **게시판 걷기는 `Maago-Expect:` 줄(진단의 첫 줄)만 맞춘다**(`tutorial_test.go`): 단계 글이 인용한 `fix:` 줄은 시험이 보지 않는다. 새 단계는 `examples/board/MAP.md`의 둘째 표에 행이 있어야 `TestBoardMap`이 통과한다
- **단계에서 멈춘다**(D-61, ARCHITECTURE §3, `pipeline.Generate`): "단계 경계가 곧 연쇄를 끊는 자리". 연구 10번의 왕복(첫 오류 `maxlen` 뒤에야 `auth.App`)을 줄이려면 그 경계를 다시 그어야 한다: 코드보다 결정의 일이다

## 질문과 답 (사람의 결정, 2026-10-23, 모두 추천)

초안 리뷰(아래)로 고친 추천. 모두 에이전트 편의로 골랐다. Q1·Q5·Q6·Q7은 사람이 추천대로 답했고(D-157~D-159), Q2·Q3·Q4는 리뷰 둘이 추천과 같은 쪽이라 정했다.

- **Q1 범위**: D-148의 나머지 다섯과 게이트 넷은 M11의 두 배쯤이다(게시판 단계도 넷쯤 는다). (a, 추천) M12는 게이트 TODO, 폼 둘, API 토큰 역할, 타입 있는 탈출구 뷰, decimal·UUID·JSON, outbox. 에이전트가 만드는 서비스의 맞음(돈을 `float64`에, `panic`의 500, 잃는 웹훅)에 닿는 것들. 외부 미디어 저장소와 계속하는 검사는 M13(앞은 배포의 모양이고, 뒤는 D-61을 다시 정하는 일이라 따로 합의가 필요하다) (b) 다섯 모두 M12(작업 약 19, 게이트까지 길다) (c) M12는 게이트 TODO·폼 둘·API 토큰만 작게, D-148의 나머지는 M13부터
- **Q2 끝의 릴리스**: (a, 추천) M12의 끝에 v0.3.0(새 필드 타입·탈출구 뷰의 서명이 API를 더하고 바꾼다) 태그, 그 판을 `go install`한 에이전트 게이트를 새 단계 글만으로(M10·M11의 길). 단계 글이 생성기의 출력을 인용하므로 릴리스가 글을 참으로 만든다 (b) 태그 없이 저장소에서 지은 `maago`로 게이트(M9의 길) (c) 게이트 없이
- **Q3 폼 둘**: (a, 추천) 폼마다 생성 함수 `NewX() X`(폼 타입 X, `NewCommentForm()`)(이름·라벨을 채운 빈 폼, 2번 계획에서 `ctx`를 뺐다, `forms_gen.go`에 생겨 뷰가 부를 수 있다, 이름은 D-112의 생성 이름 충돌 검사에). GET 페이지와 POST의 다시 그리기가 같은 함수로 다른 폼을 그린다. In은 여전히 폼 하나(둘째 폼의 fix가 `NewXForm`을 가리킨다). D-119(폼을 그리는 길)를 고친다 (b) In이 폼 여럿을 받고 GET은 모두 기술, POST는 보낸 것만 읽는다(POST의 다시 그리기 길이 따로 남는다) (c) 예와 문서만(손으로 쓴 이름이 남는다)
- **Q4 API 토큰 역할**: (a, 추천) 복사된 auth에 `SignInToken(ctx, db, u) (string, error)`(`SignIn`과 같은 검사, 토큰을 만들어 저장), `maagotest.Bearer(h, token) http.Handler`. `Role`은 그대로(이미 `http.Handler`를 받는다). 역할이 하나 늘면 모든 행에 그 칸이 필요하다(`permissions.go:135`): Bearer는 모든 라우트에서 받으므로(D-129) 페이지 행에도 진짜 답이 있어 `Skip`보다 채운다(게시판의 22행이 모두 바뀐다). D-156에 한 줄 (b) 게시판은 API 행만 토큰 역할로 보내고 페이지 행은 `Skip` (c) 도우미만, 표는 쿠키 그대로
- **Q5 타입 있는 탈출구 뷰**(리뷰가 고친 꼴): (a, 추천) `func(ctx, [u], db, in In, w http.ResponseWriter, r *http.Request) error`: 사용자·DB·In은 타입 있는 뷰와 같은 규칙으로 생성 어댑터가 채운다. `r`가 남아 `ServeMedia`(Range)·몸통 읽기가 된다. DB는 `maago.DB`만(`Tx`는 생성 오류: 스트리밍은 버퍼할 수 없어 200 뒤의 커밋 실패가 생긴다, D-150). 쓰기 전에 돌려준 오류는 타입 있는 뷰와 같은 상태(`ErrNoRows` 404, `Status`, 그 밖 500과 `OnError`), 쓴 뒤의 오류는 `OnError`와 연결 끊기(`http.ErrAbortHandler`). 감싼 `w`는 `http.Flusher`를 그대로 보인다. `ServeMedia`는 오류를 돌려준다(D-140). `func(w, r)`는 그대로 받는다(웹훅 받기). D-41·D-149(In이 마지막)를 고친다 (b) 버퍼하는 탈출구만(작은 내려받기), 스트리밍은 `func(w, r)` (c) 지금 꼴에 도우미(`maago.PathInt` 따위)만
- **Q6 decimal·UUID·JSON의 꼴**(stdlib만, 리뷰가 고친 추천): (a, 추천) 이번에는 UUID와 JSON, 돈은 정수: `maago.UUID`(`[16]byte`, `maago.NewUUID()`는 v7, PG `uuid`, SQLite 정규형 `TEXT`), `maago.JSON[T]`(PG `jsonb`, SQLite `TEXT`와 `CHECK (json_valid(…))`, 모델 폼에서 빼고 admin은 읽기만, 생성 파일에 까닭 한 줄, D-18). 돈은 `int64` 필드에 `decimal=2` 태그: DB는 두 방언 모두 정수(최소 단위)라 손 SQL의 뜻(`WHERE price > :min`, `SUM`)이 방언마다 같고, 폼·admin이 `123.45`를 읽고 쓰며 자릿수 넘침은 폼 오류. `float64`에 돈 이름(`price`·`amount`·`total`)이면 이 태그를 권하는 진단. D-77·D-16 고침 (b) `maago.Decimal`(PG `numeric(p,s)`, SQLite 곱한 `INTEGER`): 같은 태그가 손 SQL에서 방언마다 다른 뜻(`price > 100`이 100.00과 1.00)이고 `SUM`은 출처가 없어 `int64`로 조용히 읽힌다 (c) UUID·JSON만, 돈은 문서
- **Q7 outbox**: (a, 추천) `maago add outbox`가 모델·쿼리·일꾼을 복사한다(D-123의 앱 층: 표가 보통 모델 → `makemigrations`의 길을 가고 숨은 스키마가 없다). 쓰는 쪽은 요청 tx 안의 `InsertOutbox`(생성된 함수). 일꾼은 최소로(연구 §4 "outbox 표 하나까지만"): 두 방언에서 같은 잡기 `UPDATE … SET locked_until = :until WHERE id = (SELECT id … WHERE locked_until < :now … LIMIT 1) AND locked_until < :now RETURNING …`(0행이면 다시, `SKIP LOCKED` 없이 D-123 그대로), 일꾼이 죽으면 기한이 지나 다시 잡힌다, 지수 대기와 시도 수의 끝(그 뒤는 남겨 두고 admin에서 보인다), 최소 한 번이라 받는 쪽은 열쇠로 거른다. `main`이 `Serve` 곁에 띄우고 멈출 때 하던 일을 끝낸다(D-153에 한 줄). 게시판 0018의 웹훅이 그리로 (b) 런타임이 `maago_outbox`를 소유(`Migrate`가 만들고 `Config.Jobs`·`maago.Enqueue`, `Serve`가 일꾼): 쓰기 쉽지만 사용자의 마이그레이션 밖의 표 (c) 문서의 꼴만

## 작업 (초안 리뷰 뒤)

순서: 게이트 TODO, 폼 둘·토큰(사람의 차례), 그다음 API를 바꾸는 탈출구 뷰를 타입보다 먼저(뒤의 게시판 단계가 새 꼴로 쓴다), 타입, outbox, 문서, 릴리스, 게이트.

1. **게이트 TODO**: 규칙 6의 fix가 정책을 고르지 않는다(꼴은 계획에서), 0035 글을 저장소의 출력으로. 글이 인용한 fix 줄은 걷기가 보지 않으므로 트레일러를 fix까지 넓힐지 계획에서 정한다. `go doc maago.UniqueError`가 `Fail`이 생성되는 폼의 메서드라고, `maago doc query`가 `GetX`의 정책 param(`now`는 `time.Now()`)을, `maago doc view`가 `func(w, r)` 뷰의 사용자는 `maago.UserFrom`이라고(게이트 2번) → 확인: `TestPolicyCheckFix`(정책 하나·둘의 fix 전문), `TestBoardTutorial`(0035의 `Maago-Expect` 그대로), `TestDocExamples`, `TestPackageDoc`

   계획
   - `internal/mapper/policycheck.go:229`: 모델의 정책이 하나면 fix 그대로(그 이름). 여럿이면 이름을 고르지 않는다: `declare -- policy: name p and write name(p) in the WHERE of the SELECT that reads it, name the policy of Post whose rows the query may show (out or visible, as policies.sql says); or -- policy: none p why, if it may read every row`. 정책의 주석은 fix에 싣지 않는다(정책마다 여러 줄이고 `policies.sql`에 있다) → 확인: `policy_test.go`에 fix 전문을 보는 시험 하나, Post(둘)와 Comment(`shown` 하나)
   - 0035 글(`examples/board/steps/0035-*.patch`의 메시지)의 fix 인용을 새 출력으로, "The fix names the model's first policy…" 문단을 "a post's page is visible"로. 메시지만 고치고 hunk는 그대로
   - 걷기가 인용을 본다(fable의 지적): `expected`가 `Maago-Expect:`가 있는 단계의 메시지에서 들여 쓴 `fix: ` 줄을 모으고, `walkStep`이 그 줄마다 gen 출력에 있는지 본다. 지금 그런 단계는 여섯(0006·0009·0020·0024·0027·0035), 줄은 일곱. 실패를 기대하지 않는 단계(0021)의 인용은 보지 않는다(그 단계의 gen은 통과한다) → 확인: `TestBoardTutorial`, fix 줄 하나를 일부러 바꾸면 실패
   - 문서 셋: `UniqueError`의 Go 문서에 "Fail은 `maago gen`이 폼마다 쓰는 메서드"(`query.go:28`, 이미 `Fail`을 가리키고 빠진 것은 "생성된다"), `maago doc query`의 `GetX` 예 뒤에 정책 param은 쿼리의 param처럼 그 값(`now`는 보통 `time.Now()`), `maago doc view`의 `func(w, r)` 문단에 정책을 쓰는 쿼리에는 `UserFrom`이 ok면 `&u`, 아니면 nil(게이트 둘 다 `userFrom(r)`을 지어냈다) → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
   - 하지 않는 것: 정책 주석을 fix에 넣기, 트레일러 새로 만들기(`fix:` 인용은 이미 메시지에 있다)
   - 계획 리뷰는 fable 하나: 정해진 틀 안의 작은 작업(진단 글 하나, 문서 세 줄, 시험의 몇 줄)
   - 계획 리뷰(fable): `name`은 자리표시자인지 모른다 → `<name>`(`:viewer_<column>`의 꼴), post_detail을 가르는 사실은 `-- get:` → fix에 "GetPost reads under visible"(모델마다 `-- get:` 정책은 하나, 고르기가 아니라 사실), 경로는 `board/policies.sql`(게이트 2번이 어느 디렉터리인지 헤맸다), 인용 확인은 줄 전체 일치(부분 일치는 짧은 인용이 묻힌다), 문서 줄은 정책 쿼리 문단 곁. 모두 받음
   - 구현 리뷰는 ponytail(지울 것 없음)과 fable 하나(계획 리뷰를 하나로 줄인 작업): 걷기의 새 확인을 지키는 Catches 행이 없다 → "a misquoted fix", `UserFrom` 문장이 "either"의 앞을 끊었다 → 그 문단 끝으로, `now`의 글. 받음. fable의 PG 걷기가 같은 스키마 이름으로 내 전체 시험과 겹쳐 0017에서 `no schema has been selected`: 겹치지 않게 다시 돌림

   기록(2026-10-24): `missingPolicyFix`(`internal/mapper/policycheck.go`), `TestPolicyCheckFix`(옛 꼴로 되돌리면 실패), 0035 메시지, 걷기의 `fix:` 인용 확인과 Catches "a misquoted fix"(인용을 한 글자 바꾸면 `the message quotes`로 실패), 문서 셋(`query.go`의 `UniqueError`, `maago doc query`·`view`). 여섯 단계의 일곱 인용은 지금 출력과 글자 그대로 같았다
2. **폼 둘**(Q3): `forms_gen.go`의 `NewX()`(폼 타입 X), 생성 이름 충돌 검사(앱이 같은 이름을 선언하면 오류), 둘째 폼 진단의 fix, `maago doc form`의 두 폼 예 → 확인: 한 페이지 두 폼의 GET과 POST 다시 그리기에서 다른 폼의 이름·라벨이 차 있다(웹 시험), 뷰가 `NewXForm`을 불러도 `maago gen`이 통과(타입 검사 앞), 골든, `TestDocExamples`

   계획
   - **`ctx`를 받지 않는다**(Q3에서 고침): 빈 폼의 기술은 `rt.Rule`의 `Name`·`Label`·`Help`·`Choices`뿐이고(`rt/form.go`의 `bind`가 `vals == nil`이면 그것만 쓴다), `ctx`는 `FinishForm`이 `Fail`의 문구를 위해 담는 설정에만 쓰인다. 보내지 않은 폼에는 `Fail`이 할 일이 없다(`Valid()`도 false). 그래서 `func NewCommentForm() CommentForm`. 이름은 `New` + 폼 타입 이름 그대로(`NewPostForm`이면 `NewNewPostForm`: 어색하지만 짐작할 수 있다)
   - `rt`: `Describe[T any](f *maago.Field[T], rule Rule)`가 네 칸을 쓴다. `bind`와 `BindFile`이 그것을 부른다(같은 네 칸이 두 곳에 있다)
   - `internal/web/templates/forms.go.tmpl`: 폼마다(쓰이지 않는 폼도, `FormsFile`이 이미 모든 폼을 쓴다) `NewXForm`: `var f X`, 칸마다 `rt.Describe(&f.Field, <ruleLit>)`, `return f`. `FormsFile`이 칸마다 `ruleLit(ff)`를 넘긴다. 문서 주석: GET이 그리는 빈 폼과 같고, 한 페이지의 다른 폼을 그린다
   - 이름 충돌: `More`가 앱의 모든 폼에 `NewX`를 더한다(`bindX`와 같은 길, D-112). 폼의 칸 이름과는 부딪히지 않는다(패키지 수준 함수)
   - `internal/web/views.go:260`의 fix: `keep one form field in In; draw the other form with NewX(), and post each form to its own route`(X는 둘째 폼의 타입)
   - 문서: `maago doc form`의 "A GET view and a POST view take the same In" 문단 뒤에 한 페이지 두 폼: In은 폼 하나를 읽고, 다른 폼은 `NewCommentForm()`으로 그린다, 각 폼은 자기 라우트로 보낸다. 예 프로젝트(`cmd/maago/docs/form.txt`)에 그 쓰임 하나
   - 시험: 웹 골든(`forms_forms_gen.go.golden`), 충돌 시험 한 줄(`forms_test.go`의 표: 앱이 `func NewBoardForm()`을 선언하면 오류), site의 업로드 페이지가 `NewNewPostForm()`도 그려 GET과 실패한 POST 둘 다 `name="title"`과 라벨 `제목`(`TestUpload`), `TestDocExamples`
   - 하지 않는 것: In이 폼 여럿을 받기(Q3 b), 보낸 폼을 다른 폼과 함께 다시 그리는 도우미(뷰가 `NewX()`를 부르면 된다), 게시판(4번)
   - 계획 리뷰는 codex·fable 둘(생성물의 새 모양)
   - 계획 리뷰(codex·fable): 충돌 검사는 `mapper.Names`(M3 8번)이지 D-112가 아니다 → 고침. `More`가 지금은 쓰이는 폼의 `bindX`만 → 앱의 모든 폼에 `NewX`, fix는 "rename the form, or what else declares NewX; maago generates NewX to draw form X empty"(가장 흔한 충돌은 같은 이름의 GET 뷰, fable: 진단이 그 선언의 자리를 함께 말한다). `FormsFile`은 `formsData`(쓰이는 폼만)가 아니라 모든 폼(codex). 쓰이지 않는 폼의 `NewX`를 뷰가 불러도 gen이 통과: `TestFormValidUnread`를 넓힌다(fable). `TestUpload`에 GET이 없다 → GET 하나, 그리고 "no file chosen"에서 required 문구가 한 번(다른 폼이 요청에 묶이지 않는다, fable). 생성된 주석과 `maago.Field` 문서에 "보내지 않은 폼: Valid는 false, Fail의 문구는 기본"(둘 다). D-119에 한 줄. 문서와 예는 같은 폼 이름. 받지 않음: `Describe`에 네 칸만 담은 짧은 Rule(fable 선택 사항): `ruleLit` 하나를 다시 쓰는 편이 코드가 적다
   - 구현 리뷰(ponytail·codex·fable): ponytail 지울 것 없음(`Describe`는 두 곳의 같은 네 줄을 하나로). codex 결함 없음(샌드박스가 쓰기를 막아 시험은 돌리지 못했다: 읽은 리뷰). fable: 보내지 않은 폼의 `Fail`은 할 일이 없지 않고 기본 문구를 쓴다(`rt/write.go:92`) → 생성 주석·`maago.Field`·`maago doc form`에 같은 문장, 두 폼 사례가 둘 다 `BoardForm`이라 fix가 어느 폼을 말하는지 못 가린다 → 둘째를 `NoticeForm`으로, `form.go`의 글. 받음

   기록(2026-10-24): `rt.Describe`, 폼마다 `NewX()`(`forms_gen.go`, 쓰이지 않는 폼도), `More`의 `NewX` 이름 검사, 둘째 폼의 fix가 `NewX()`를 가리킨다, `maago doc form`의 문단과 예(리뷰 페이지가 `NewPostForm()`), `maago.Field` 문서, D-119 고침. 시험: 골든 둘, 충돌 사례, `TestFormValidUnread`(쓰이지 않는 폼의 `NewX`), site `TestUpload`(GET과 실패한 POST가 다른 폼의 `name="title"`·`제목`, 그 폼에 오류 없음, `NewNewPostForm()`을 빼면 실패를 확인), 예의 시험. PG 전체 시험 초록
3. **API 토큰 역할**(Q4): 복사된 auth의 `SignInToken`, `maagotest.Bearer`, auth 복사본의 권한 표에 토큰 역할 → 확인: `TestAddAuth`(두 방언), 닫히거나 확인 안 된 계정은 오류(`TestSignInRefuses`), 지운 토큰·쿠키와 함께인 토큰은 이미 `TestAPITokens`·`contrib/auth`의 시험, `TestDocExamples`, `TestNoDecisionsInUserText`
   계획
   - 복사된 auth(`internal/scaffold/testdata/auth/auth.go`, `SignIn` 곁): `SignInToken(ctx, db, u User) (string, error)`: `SignIn`과 같은 거절(닫히거나 확인 안 된 계정은 오류: 그 토큰은 `APITokenUser`가 아무도 아님으로 읽는다), `authlib.NewToken()`과 `InsertAPIToken(… Name: "test" …)`, 평문 토큰을 돌려준다. 문서: `maagotest.Bearer(h, token)`과 함께, 토큰은 그 사용자의 토큰 목록(/tokens)에 보인다. 비밀번호를 다시 묻지 않는다(D-129의 재확인은 사람이 토큰을 만드는 화면의 것, 시험은 DB에 직접 쓴다: `SignIn`이 세션을 그렇게 만드는 것과 같다)
   - `maagotest.Bearer(h http.Handler, token string) http.Handler`: 요청마다 `Authorization: Bearer token`(`As` 곁, 같은 꼴). 쿠키가 함께여도 토큰이 정하는 것은 auth의 일(D-129, `TestAPITokens`가 본다)
   - auth 복사본의 권한 표(`permissions_test.go`): 역할 `cat_token`(셋째 사용자 cat의 `SignInToken`, 쿠키는 없다)을 더하고 모든 행에 그 칸을 채운다(리뷰가 고침: ann의 토큰이면 세션을 끝내는 `logout`·`account_close_submit`과 ann의 토큰을 지우는 `api_token_delete`가 ann의 쿠키 행을 먹는다. cat이면 셋 다 진짜 답: 로그아웃 303, 남의 토큰 404, 자기 계정 닫기 303). Bearer는 모든 라우트에서 받으므로(D-129) 나머지 칸은 로그인한 사용자의 것과 같다. 표의 주석에 토큰 역할의 까닭(API 클라이언트의 길)과 시도 수의 문장 고침(로그인한 행의 틀린 비밀번호는 그 사용자의 시도로 센다)
   - `TestSignInRefuses`가 `SignInToken`도 본다
   - `maagotest`: `Bearer`의 시험 하나(머리가 실린다, `As`와 함께 감싸도 머리가 실린다), `As`의 문서와 패키지 문서의 짝
   - 문서: `maago doc view`의 권한 표 문단(`maagotest.As`와 `auth.SignIn`을 말하는 곳, `internal/ref/topics.go:298`)에 `SignInToken`·`Bearer`, 예 `cmd/maago/docs/auth.txt`의 권한 표에 토큰 역할(`api_me` 행의 진짜 클라이언트, `TestDocExamples`가 이름을 컴파일한다). `SignInToken`의 Go 문서는 "for a test"로 시작하고, 사람에게 토큰을 만들어 주는 길은 비밀번호를 다시 묻는 `APITokensSubmit`이라고. `TestAddAuth`(두 방언)가 복사본의 시험을 돌린다
   - 하지 않는 것: `Role`을 바꾸기(Q4 b), 게시판(4번)
   - 계획 리뷰는 codex·fable 둘(auth, DB 쓰기)
   - 계획 리뷰(codex·fable): 토큰 역할이 ann이면 소비하는 행 셋이 서로를 먹는다(둘 다, 고친 길은 갈렸다: codex는 진짜 행, fable은 Skip과 둘째 fixture) → 셋째 사용자 cat으로 둘 다의 뜻을 살린다. "쿠키와 Bearer가 함께면 토큰"·"지운 토큰은 아무도 아님"은 이미 `contrib/auth`의 `session_test.go`와 auth 복사본 `TestAPITokens`가 본다(fable) → 확인에서 그 이름으로. 문서 이름을 컴파일하는 예(fable) → `auth.txt`. 받지 않음: `SignInToken`을 `_test.go`로(codex): 게시판의 시험은 다른 패키지라 `_test.go`의 이름을 못 본다, `SignIn`도 같은 까닭으로 `auth.go`에 있다(M11 3번). 대신 문서가 "for a test"로 시작하고 사람의 길을 가리킨다. 토큰에 만료가 없다(codex)는 이번 일 밖
   - 구현 리뷰(ponytail·codex·fable): ponytail 지울 것 없음(`Bearer`는 `As`의 꼴, `SignInToken`은 여섯 줄). codex 결함 없음(시험은 샌드박스가 막았다: 읽은 리뷰). codex의 "`logout`의 303은 토큰을 받았는지 가리지 못한다"는 받지 않음: 로그아웃은 누구에게나 303이고 표는 누가 물을 수 있는지를 본다, 토큰을 받았다는 증거는 `password_change` 200(아무도 303)과 `api_token_delete` 404(아무도 401). fable: 글 고침(takes → accepts, `Role` 문서에 Bearer, 표 주석, 예의 겹친 append). 받음

   기록(2026-10-24): 복사된 auth의 `SignInToken`, `maagotest.Bearer`와 `TestBearer`, auth 복사본의 권한 표에 `cat_token`(모든 행, 토큰 칸 하나를 틀리게 하면 실패를 확인), `TestSignInRefuses`, `maago doc view`의 권한 문단, `auth.txt` 예의 토큰 역할. `TestAddAuth`(두 방언)·`TestDocExamples`

4. **게시판 0037: 폼 둘과 토큰**: 글 페이지가 `NewCommentForm`·`NewAttachmentForm`으로, 손으로 쓴 폼 칸의 `name=`·라벨을 지운다. `TestPermissions`에 토큰 역할(모든 행), `TestAPI`·`TestMe`의 손 토큰을 `SignInToken`으로, MAP.md에 행 → 확인: `TestBoardTutorial`(PG), `TestBoardMap`, 폼 struct 칸의 `name=`이 템플릿에 손으로 없다(0008의 검색 `q`는 폼이 아니다)
   계획
   - 단계 0037 `Draw both forms and test as an API client`(이름은 짧게 고칠 수 있다): `postPage`가 `AttachForm: NewAttachmentForm()`을 기본으로, `Attach`의 다시 그리기가 `CommentForm{}` 대신 `NewCommentForm()`, `post_page.html`의 파일 칸이 `{{.AttachForm.File.Name}}`·라벨 `{{.AttachForm.File.Label}}`(지금 손으로 쓴 `name="file"`)
   - `TestPermissions`: 역할 `cat_token`(셋째 사용자 cat, `auth.SignInToken` + `maagotest.Bearer`, 쿠키 없음). 작성자도 staff도 아니므로 칸은 bob과 같은 까닭의 답(남의 글 403·초안 404·admin 403), 만드는 행 셋(`board_create`·`post_create`·`api_post_create`)은 cat의 제목으로 행을 하나씩 더하고 기존 행은 `skip`. 3번처럼 ann의 토큰이 아닌 까닭: 작성자의 행(게시·보관·지우기)은 소비하는 행이라 ann의 쿠키 행을 먹는다. 표 주석의 "The API takes the session's cookie too"에 토큰을 더한다
   - `TestAPI`(0022~)·`TestMe`(0030~)의 손 토큰(`authlib.NewToken()` + `auth.InsertAPIToken`)을 `auth.SignInToken`으로. 그 요청은 `Origin`과 머리를 직접 싣는 그대로(평문 토큰만 필요)
   - 시험이 손으로 쓴 이름을 보던 곳: 첨부 시험이 `name="file"`을 찾는지 본다(있으면 그대로 통과해야 한다: 이름은 같다)
   - 단계 글: 0029·0031이 손으로 쓴 이름을 고친다는 것, `NewX`는 폼마다 생성된다, 토큰 역할의 까닭. `examples/board/MAP.md` 둘째 표에 0037 행
   - 확인: `TestBoardTutorial`(PG), `TestBoardMap`, `TestBoardTutorialCatches`(새 단계가 마지막이 되어 바뀌는 행이 있는지), 템플릿에 폼 칸의 손 `name=`이 없다(grep: `name="file"`·`name="text"`)
   - 계획 리뷰는 fable 하나: 정해진 틀 안의 게시판 단계(2·3번의 API를 쓴다)
   - 계획 리뷰(fable): `want`는 역할의 차례대로라 cat이 ann 뒤면 ann·staff의 게시·지우기 행이 먼저 먹는다 → 차례 `nobody, bob, cat_token, ann, staff`. 손 이름 grep은 헛것(댓글은 이미 `{{.Name}}`) → 진짜 결함(첨부 실패 뒤 댓글 칸 `name=""`)을 `TestAttachments`가 보고, GET의 파일 칸 이름·라벨도, Catches 두 행(`NewCommentForm()` → `CommentForm{}`, 토큰 칸 하나). 글은 0029만(0031 아님), 서명을 인용, ann의 토큰이 아닌 까닭. 마지막 단계를 이름으로 부르는 다른 행은 없다(확인). 모두 받음
   - 구현 리뷰는 ponytail(지울 것 없음)과 fable 하나: 패치 머리의 `From:`이 개인 주소(0034도) → `Maago <maago@example.com>`, 글 두 구절, 주석 줄 바꿈. 받음

   기록(2026-10-24): 단계 0037 `Draw both forms, and ask as an API client`(`postPage`의 `NewAttachmentForm()`, `Attach`의 `NewCommentForm()`, 파일 칸의 라벨·이름, 권한 표의 `cat_token`과 cat의 만드는 행 셋, `TestAPI`·`TestMe`의 `SignInToken`, `TestAttachments`의 두 폼), MAP의 0037 행, Catches "the comment form, unnamed"·"a token's cell". `TestBoardTutorial`·`TestBoardTutorialCatches`·`TestBoardMap`(PG)

5. **타입 있는 탈출구 뷰**(Q5): 서명 검사, 생성 어댑터, 쓰기 전·뒤의 오류, `ServeMedia`의 오류, 라우트 표·URL 함수·권한 표가 그대로 → 확인: 쓰기 전 오류의 상태(404·`Status`·500과 `OnError`), 쓴 뒤 오류의 끊긴 연결, 감싼 `w`의 `Flush`가 스트리밍한다, `Tx`는 생성 오류, `ServeMedia`의 Range 응답(206), 골든, 두 방언
   계획
   - **이미 있는 것**: 런타임은 라우트마다 `w`를 `written`으로 감싸고(`http.go`), 뷰가 돌려준 오류는 `report`가 상태로(아직 쓰지 않았으면) 바꾸고 5xx면 `OnError`에 알린다. `written`은 `Flush`와 `Unwrap`을 이미 보인다(0017의 스트리밍). 그래서 생성 어댑터는 뷰의 오류를 그대로 돌려주면 되고, 새 런타임 코드는 "쓴 뒤의 오류"뿐이다
   - 꼴(D-157): `func(ctx context.Context, [u U | *U,] [db maago.DB,] in In, w http.ResponseWriter, r *http.Request) error`. 이 문서와 진단에서는 "스트리밍 뷰"가 아니라 "a view that writes the response itself"(`func(w, r)`도 그렇다)
   - `internal/web/views.go` `readView`: 결과가 하나이고 마지막 두 매개변수가 `http.ResponseWriter`·`*http.Request`(구문의 이름, 타입 검사가 확인)면 `v.writes = true`, 그 둘을 떼고 타입 있는 뷰와 같은 길(사용자·DB·In의 차례, `DBFrom`·`UserFrom` 금지, In의 필드와 경로 값). `maago.Tx`는 생성 오류: "a view that writes the response itself takes db maago.DB: its response is sent as it is written, before any commit". `shapes`에 셋째 꼴
   - `internal/web/typecheck.go` `signature`: `writes`면 첫째 `context.Context`, 끝 둘의 타입, 결과 `error`. 사용자 타입 검사는 그대로(1번 자리). 페이지·템플릿이 없다: `escape`처럼 건너뛴다(`Block`은 오류). 템플릿 검사에서 그 라우트의 경로 값은 In의 타입(지금 `func(w, r)`만 `string or int`)
   - `views.go.tmpl`: `writes`면 In·사용자 읽기는 타입 있는 뷰 그대로, 그다음 `return View(r.Context(), [u,] [maago.DBFrom(r.Context()),] in, w, r)`
   - `http.go` `report`: 이미 썼으면(`w.wrote`) `OnError`(5xx) 뒤 `panic(http.ErrAbortHandler)`: 받는 쪽이 잘린 200을 끝난 응답으로 받지 않게. 패닉 경로(`recover`의 `report`)도 같은 길을 탄다: 쓴 뒤의 패닉도 연결을 끊는다(지금은 끊지 않고 남는다)
   - `media.go` `ServeMedia(w, r, m) error`: 없는 열쇠는 `maago.Status(404, err)`, 그 밖의 여는 오류는 그대로(쓰기 전이라 500과 `OnError`). `func(w, r)`에서 부르는 곳은 오류를 스스로 다룬다(문서). D-140 고침
   - 시험(`cmd/maago/testdata/web/site`): `MediaDownload`를 새 꼴로(경로 `key`는 In, `return maago.ServeMedia(w, r, …)`), 새 라우트 하나가 쓰기 전 오류(`sql.ErrNoRows` → 404, `maago.Status(403)` → 403, 그 밖 → 500과 `OnError`)와 쓴 뒤 오류(연결 끊김: 클라이언트가 `io.ErrUnexpectedEOF`, `OnError`가 들음)와 `Flush`(묶음 사이에 플러시가 닿는다), 사용자를 받는 꼴(로그인 없으면 401·303). `ServeMedia`의 Range(206)와 없는 열쇠 404. 생성기 골든(`views.golden`의 진단: `Tx`, 결과 둘, 끝 둘의 순서), `signature_views_gen.go.golden`류의 어댑터
   - 문서: `maago doc view`의 탈출구 문단(`DBFrom`·`UserFrom`을 쓰는 `func(w, r)` 대신 새 꼴이 먼저, `func(w, r)`은 몸통을 그대로 읽는 웹훅 받기 따위), 예 `cmd/maago/docs/view.txt`에 새 꼴 하나, D-41·D-149·D-140 고침
   - 하지 않는 것: `func(w, r)` 없애기(Q5 b), `maago.Tx`(D-157), 게시판의 새 꼴(6번)
   - 계획 리뷰는 codex·fable 둘(새 API의 모양, 런타임)
   - 계획 리뷰(codex·fable)로 고친 것(모두 받음, 결정에 없는 선택은 ★)
     - 꼴 판별은 결과 수로: 0이면 `func(w, r)`, 1이면 쓰는 뷰, 2이면 타입 있는 뷰. `readView`의 매개변수 상한은 6, 끝 둘(w·r)을 먼저 떼고 그 앞을 지금의 사용자·DB·In 검사에 넘긴다. 결과 둘에 w·r가 붙은 뷰는 "takes no In"이 아니라 "a view that writes the response itself returns only error"
     - `writes`를 `escape`가 있는 자리 모두에 더한다: `gen.go:99,181`의 `html`(쓰는 뷰만 있는 앱에 `templates` embed가 생기지 않게), 템플릿 데이터, `typecheck.go:92`의 건너뛰기(`Block`은 오류), `loginOptions`의 fix, `viewSignature`(새 꼴도 찍는다), `shapes` 상수 둘과 `views.golden`
     - ★ POST 라우트의 쓰는 뷰: 트랜잭션이 없다. `noTxOptions`에서 `func(w, r)`처럼 다룬다: `maago.NoTx`는 "writes the response itself, which has no transaction" 오류, "takes maago.DB, which runs outside a transaction"에서 빠진다. 까닭: `Tx`가 생성 오류인 뷰에 `NoTx`를 요구하면 고를 것이 없는 의식이다
     - ★ 쓴 뒤의 오류는 상태와 관계없이 끊고, 5xx만이 아니라 모두 `OnError`에 알린다: 첫 바이트 뒤에는 403도 답할 수 없어 뷰의 잘못이고, 조용히 버리지 않는다(PHILOSOPHY 7). 쓴 뒤의 패닉도 끊는다(D-157은 돌려준 오류만 말한다). 타입 있는 뷰는 `rt.Render`·`rt.JSON`이 버퍼하므로 쓴 뒤 오류는 `Write` 실패(클라이언트가 떠남)뿐이다
     - 쓰는 뷰의 `AfterCommit`은 트랜잭션이 없어 곧 돈다(`tx.go`): 문서에 한 줄
     - 시험: 쓴 뒤 오류는 `httptest.NewServer`로, 뷰가 오류 전에 `Flush`하고 `io.ReadAll`의 err가 `io.ErrUnexpectedEOF`임을 단언(몸통이 아니라). `TestExportStreams`의 "panic after a row"도 그 err를 단언하게 고친다(지금은 err를 버려 abort 없이도 통과). `Flush`는 gate로 "첫 묶음이 둘째가 쓰이기 전에 닿는다"(`TestExportStreams`의 꼴). `WriteHeader(200)`만 한 뒤의 오류도 끊긴다. `ServeMedia`의 Range(206), HEAD(몸통 없음), 없는 열쇠 404. 쓰기 전 오류와 쿠키(쓰기 전 오류면 `SetCookie`가 버려진다)는 Recorder로. 사용자를 받는 꼴은 GET이라 로그인 없이 303. 새 라우트마다 `permissions_test.go`에 행(쓰기 전 상태만)
     - 문서의 자리: `cmd/maago/docs/view.txt`(새 꼴 예), 패키지 주석 `http.go`의 `func(w, r)` 문단들, `media.go`의 `ServeMedia` 주석, fix 글 `views.go`의 `func(w, r)`를 말하는 곳. In에 폼이 든 쓰는 뷰는 어댑터가 몸통을 읽으므로 `r.Body`를 다시 읽을 수 없다는 한 줄
     - 게시판 0029·0032(사람의 결정 2026-10-27, fable이 짚음): `ServeMedia`를 문장으로 부르면 반환이 버려져 파일 없는 내려받기가 빈 200이 된다. 두 패치를 `switch err := maago.ServeMedia(w, r, a.media()); { case errors.Is(err, fs.ErrNotExist): http.NotFound(w, r); case err != nil: panic(err) }`로(0038이 이 손 처리를 지운다), 0029 글의 "calls maago.ServeMedia"에 그 처리
     - 확인에 `TestBoardTutorial`과 골든 `urls_gen.go.golden`·`routes.golden`(라우트 표·URL 함수가 그대로)

6. **게시판 0038: 내보내기와 내려받기**: `ExportBoard`·`DownloadAttachment`를 새 꼴로, 손 파싱·`panic`·`UserFrom`이 사라진다, MAP.md에 행 → 확인: `TestBoardTutorial`, `TestBoardMap`, 게시판에 `panic(`이 없다
7. **UUID·JSON·돈의 정수**(Q6): 태그·DDL·스키마 읽기·비교, 쿼리 결과·매개변수 타입, 폼 바인딩(UUID, `decimal=`), admin, `float64` 돈 이름의 진단, `maago doc` → 확인: 두 방언에서 모델 → `makemigrations` → `migrate` → 쿼리 → 폼·admin(gentest, `orders`의 `Total`이 `decimal=2`), 손 SQL `WHERE total > :min`·`SUM(total)`이 두 방언에서 같은 행·같은 값, 소수 자리 넘침·자릿수 넘침이 폼 오류, 스키마 비교의 차이 없음, `TestDocExamples`
8. **게시판 0039: UUID와 JSON**: 게시판에 돈은 없다(돈은 gentest `orders`가 본다). 글의 공개 열쇠 UUID(정책을 쓰는 손 쿼리로 읽기)와 첨부의 JSON 메타 따위, 쓰임은 계획에서, MAP.md에 행 → 확인: `TestBoardTutorial`, `TestBoardMap`
9. **outbox**(Q7): `maago add outbox`, 복사되는 모델·쿼리·일꾼과 그 시험 → 확인: 쓰기 tx가 롤백되면 일이 없다, 커밋 뒤 일꾼이 일 도중 죽었다 다시 떠도 기한 뒤 한 번 이상 간다, 두 일꾼이 같은 일을 동시에 잡지 않는다(두 방언), 시도 수의 끝, 멈출 때 하던 일을 끝낸다, `TestAddAuth`의 꼴로 복사본 시험
10. **게시판 0040: 웹훅을 outbox로**: 0018의 `AfterCommit`과 메모리 재시도를 outbox로, `main`이 일꾼을 띄운다, MAP.md에 행 → 확인: `TestBoardTutorial`, `TestBoardMap`, 웹훅 서버가 실패하다 살아나면 받는다
11. **문서**: `maago doc`의 view(탈출구)·form(`NewXForm`)·model(타입)·main(일꾼), 결정 고침: D-41·D-149(탈출구의 차례)·D-150(탈출구의 DB)·D-140(`ServeMedia`)·D-77·D-16·D-18(JSON을 폼에서 뺀다)·D-119(`NewXForm`)·D-123(outbox의 복사)·D-153(일꾼)·D-156(토큰 역할) → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
12. **v0.3.0**(Q2): 태그, 게시판의 `go.mod`를 v0.3.0으로(0001·0013) → 확인: 빈 GOPATH에서 `go install …@v0.3.0`, 공개 판으로 걷기의 끝(`GOWORK=off`)
13. **에이전트 게이트**: `go install …@v0.3.0`부터, 0036의 끝에서 새 단계(0037~0040) 글만 보고 서로 모르는 실행 둘. 게이트가 고칠 것을 찾으면 v0.3.1(M11과 같은 길) → 확인: 기록, 메시지와 `maago doc`만으로 씀
14. **M12 끝**: MILESTONES의 M12 체크, README, 다음 마일스톤의 차례

## 초안 리뷰 (2026-10-23, codex와 fable)

- **CSRF 토큰은 없다**(둘 다): `FinishForm`은 `Bound·OK·Settings`만 → 사실·Q3·2번에서 지움
- **decimal의 곱한 INTEGER**(fable, codex는 "의미가 정해지지 않았다"): 같은 태그가 손 SQL에서 방언마다 다른 뜻, `SUM`은 출처가 없어 `int64`로 조용히 → Q6 추천을 두 방언 모두 정수 + `decimal=` 태그로, 확인에 손 SQL의 같은 값
- **탈출구 뷰에 `r`가 없다**(fable: `ServeMedia`의 Range), 버퍼·스트리밍(둘 다), `Tx`와 D-150(fable), 감싼 `w`의 `Flusher`(fable) → Q5를 `…, w, r) error`·`DB`만으로, 확인에 206과 `Flush`
- **outbox의 `SKIP LOCKED`는 D-123과 부딪힌다**(fable), 기한·재시도 상태·멈춤(둘 다) → 두 방언 같은 잡기, 기한, 시도 수의 끝, D-153 한 줄
- **역할 하나는 모든 행의 칸**(fable): Bearer는 모든 라우트에서 받는다(D-129) → Q4에 비용을 적고 페이지 행도 채운다. 쿠키와 Bearer가 함께, 지운 토큰(codex) → 3번 확인
- **`NewXForm`은 `forms_gen.go`에**(fable, `gen.go:131`), 충돌 검사(둘 다) → Q3·2번
- **0035 글의 fix 인용은 시험이 보지 않는다**(fable) → 1번에서 트레일러를 넓힐지 정한다. `UniqueError`의 문서는 이미 `Fail`을 말한다: 빠진 것은 "생성된다" 한 마디(fable). `func(w, r)`의 사용자(게이트 2번) → 1번
- **손 `name=` 확인이 0008의 `q`에 걸린다**(fable) → 폼 struct 칸으로 좁힘. MAP.md의 행(fable) → 4·6·8·10번
- **돈이 없는 게시판**(fable) → 돈은 gentest `orders`, 0039는 UUID·JSON
- **결정 고침의 목록**(fable) → 11번
- **게이트 뒤의 v0.3.1**(fable) → 13번에 적음. 깨끗한 모듈의 확인(codex)은 M11의 `GOWORK=off`로 이미 한다
- 받지 않음: "UUID·JSON부터, 돈은 나중"(codex의 더 단순한 길)은 Q6 (c)로 남김. 돈을 `float64`에 넣는 것이 연구의 첫 근거라 정수 태그는 이번에

---

## M12 끝

- [ ] 1~14 완료
- [ ] M12 완료 조건을 실제로 돌려 확인, 조건마다 시험 이름
- [ ] MILESTONES.md의 M12 체크박스
- [ ] push 뒤 GitHub Actions
