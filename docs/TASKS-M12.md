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
- **Q3 폼 둘**: (a, 추천) 폼마다 생성 함수 `NewXForm(ctx) X`(이름·라벨을 채운 빈 폼, `forms_gen.go`에 생겨 뷰가 부를 수 있다, 이름은 D-112의 생성 이름 충돌 검사에). GET 페이지와 POST의 다시 그리기가 같은 함수로 다른 폼을 그린다. In은 여전히 폼 하나(둘째 폼의 fix가 `NewXForm`을 가리킨다). D-119(폼을 그리는 길)를 고친다 (b) In이 폼 여럿을 받고 GET은 모두 기술, POST는 보낸 것만 읽는다(POST의 다시 그리기 길이 따로 남는다) (c) 예와 문서만(손으로 쓴 이름이 남는다)
- **Q4 API 토큰 역할**: (a, 추천) 복사된 auth에 `SignInToken(ctx, db, u) (string, error)`(`SignIn`과 같은 검사, 토큰을 만들어 저장), `maagotest.Bearer(h, token) http.Handler`. `Role`은 그대로(이미 `http.Handler`를 받는다). 역할이 하나 늘면 모든 행에 그 칸이 필요하다(`permissions.go:135`): Bearer는 모든 라우트에서 받으므로(D-129) 페이지 행에도 진짜 답이 있어 `Skip`보다 채운다(게시판의 22행이 모두 바뀐다). D-156에 한 줄 (b) 게시판은 API 행만 토큰 역할로 보내고 페이지 행은 `Skip` (c) 도우미만, 표는 쿠키 그대로
- **Q5 타입 있는 탈출구 뷰**(리뷰가 고친 꼴): (a, 추천) `func(ctx, [u], db, in In, w http.ResponseWriter, r *http.Request) error`: 사용자·DB·In은 타입 있는 뷰와 같은 규칙으로 생성 어댑터가 채운다. `r`가 남아 `ServeMedia`(Range)·몸통 읽기가 된다. DB는 `maago.DB`만(`Tx`는 생성 오류: 스트리밍은 버퍼할 수 없어 200 뒤의 커밋 실패가 생긴다, D-150). 쓰기 전에 돌려준 오류는 타입 있는 뷰와 같은 상태(`ErrNoRows` 404, `Status`, 그 밖 500과 `OnError`), 쓴 뒤의 오류는 `OnError`와 연결 끊기(`http.ErrAbortHandler`). 감싼 `w`는 `http.Flusher`를 그대로 보인다. `ServeMedia`는 오류를 돌려준다(D-140). `func(w, r)`는 그대로 받는다(웹훅 받기). D-41·D-149(In이 마지막)를 고친다 (b) 버퍼하는 탈출구만(작은 내려받기), 스트리밍은 `func(w, r)` (c) 지금 꼴에 도우미(`maago.PathInt` 따위)만
- **Q6 decimal·UUID·JSON의 꼴**(stdlib만, 리뷰가 고친 추천): (a, 추천) 이번에는 UUID와 JSON, 돈은 정수: `maago.UUID`(`[16]byte`, `maago.NewUUID()`는 v7, PG `uuid`, SQLite 정규형 `TEXT`), `maago.JSON[T]`(PG `jsonb`, SQLite `TEXT`와 `CHECK (json_valid(…))`, 모델 폼에서 빼고 admin은 읽기만, 생성 파일에 까닭 한 줄, D-18). 돈은 `int64` 필드에 `decimal=2` 태그: DB는 두 방언 모두 정수(최소 단위)라 손 SQL의 뜻(`WHERE price > :min`, `SUM`)이 방언마다 같고, 폼·admin이 `123.45`를 읽고 쓰며 자릿수 넘침은 폼 오류. `float64`에 돈 이름(`price`·`amount`·`total`)이면 이 태그를 권하는 진단. D-77·D-16 고침 (b) `maago.Decimal`(PG `numeric(p,s)`, SQLite 곱한 `INTEGER`): 같은 태그가 손 SQL에서 방언마다 다른 뜻(`price > 100`이 100.00과 1.00)이고 `SUM`은 출처가 없어 `int64`로 조용히 읽힌다 (c) UUID·JSON만, 돈은 문서
- **Q7 outbox**: (a, 추천) `maago add outbox`가 모델·쿼리·일꾼을 복사한다(D-123의 앱 층: 표가 보통 모델 → `makemigrations`의 길을 가고 숨은 스키마가 없다). 쓰는 쪽은 요청 tx 안의 `InsertOutbox`(생성된 함수). 일꾼은 최소로(연구 §4 "outbox 표 하나까지만"): 두 방언에서 같은 잡기 `UPDATE … SET locked_until = :until WHERE id = (SELECT id … WHERE locked_until < :now … LIMIT 1) AND locked_until < :now RETURNING …`(0행이면 다시, `SKIP LOCKED` 없이 D-123 그대로), 일꾼이 죽으면 기한이 지나 다시 잡힌다, 지수 대기와 시도 수의 끝(그 뒤는 남겨 두고 admin에서 보인다), 최소 한 번이라 받는 쪽은 열쇠로 거른다. `main`이 `Serve` 곁에 띄우고 멈출 때 하던 일을 끝낸다(D-153에 한 줄). 게시판 0018의 웹훅이 그리로 (b) 런타임이 `maago_outbox`를 소유(`Migrate`가 만들고 `Config.Jobs`·`maago.Enqueue`, `Serve`가 일꾼): 쓰기 쉽지만 사용자의 마이그레이션 밖의 표 (c) 문서의 꼴만

## 작업 (초안 리뷰 뒤)

순서: 게이트 TODO, 폼 둘·토큰(사람의 차례), 그다음 API를 바꾸는 탈출구 뷰를 타입보다 먼저(뒤의 게시판 단계가 새 꼴로 쓴다), 타입, outbox, 문서, 릴리스, 게이트.

1. **게이트 TODO**: 규칙 6의 fix가 정책을 고르지 않는다(꼴은 계획에서), 0035 글을 저장소의 출력으로. 글이 인용한 fix 줄은 걷기가 보지 않으므로 트레일러를 fix까지 넓힐지 계획에서 정한다. `go doc maago.UniqueError`가 `Fail`이 생성되는 폼의 메서드라고, `maago doc query`가 `GetX`의 정책 param(`now`는 `time.Now()`)을, `maago doc view`가 `func(w, r)` 뷰의 사용자는 `maago.UserFrom`이라고(게이트 2번) → 확인: 규칙 6 골든(정책 하나·둘), `TestBoardTutorial`(0035의 `Maago-Expect` 그대로), `TestDocExamples`, `TestPackageDoc`
2. **폼 둘**(Q3): `forms_gen.go`의 `NewXForm(ctx)`, 생성 이름 충돌 검사(앱이 같은 이름을 선언하면 오류), 둘째 폼 진단의 fix, `maago doc form`의 두 폼 예 → 확인: 한 페이지 두 폼의 GET과 POST 다시 그리기에서 다른 폼의 이름·라벨이 차 있다(웹 시험), 뷰가 `NewXForm`을 불러도 `maago gen`이 통과(타입 검사 앞), 골든, `TestDocExamples`
3. **API 토큰 역할**(Q4): 복사된 auth의 `SignInToken`, `maagotest.Bearer`, auth 복사본의 권한 표에 토큰 역할 → 확인: `TestAddAuth`(두 방언), 닫히거나 확인 안 된 계정은 오류, 지운 토큰은 사용자 없음, 쿠키와 Bearer가 함께면 토큰이 정한다
4. **게시판 0037: 폼 둘과 토큰**: 글 페이지가 `NewCommentForm`·`NewAttachmentForm`으로, 손으로 쓴 폼 칸의 `name=`·라벨을 지운다. `TestPermissions`에 토큰 역할(모든 행), `TestAPI`·`TestMe`의 손 토큰을 `SignInToken`으로, MAP.md에 행 → 확인: `TestBoardTutorial`(PG), `TestBoardMap`, 폼 struct 칸의 `name=`이 템플릿에 손으로 없다(0008의 검색 `q`는 폼이 아니다)
5. **타입 있는 탈출구 뷰**(Q5): 서명 검사, 생성 어댑터, 쓰기 전·뒤의 오류, `ServeMedia`의 오류, 라우트 표·URL 함수·권한 표가 그대로 → 확인: 쓰기 전 오류의 상태(404·`Status`·500과 `OnError`), 쓴 뒤 오류의 끊긴 연결, 감싼 `w`의 `Flush`가 스트리밍한다, `Tx`는 생성 오류, `ServeMedia`의 Range 응답(206), 골든, 두 방언
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
