# M13 작업 목록 — 미디어 저장소, Google 로그인, 맞음의 나머지

[MILESTONES.md](MILESTONES.md)의 M13을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).
범위는 사람의 결정(2026-11-22, M12 끝): 외부 미디어 저장소, 단계를 넘어 계속하는 검사, TODO의 남은 것, 실제 이전 사례(readmate: Django에서 옮기는 서비스)를 모아 질문으로 묶는다.

**초안이다.** 아래 질문(Q1~Q10)에 사람이 답하기 전에는 작업 목록이 확정되지 않는다.

**M13 완료 조건**(초안, Q의 추천대로라면)
- 앱이 만든 바이트(백그라운드 일꾼의 것도)를 미디어로 저장한다. 인스턴스가 둘이어도 미디어가 깨지지 않는 저장소(S3 호환)가 있고, 고아 파일을 치우는 길이 있다
- Google 계정으로 가입·로그인하고, 같은 이메일의 계정에 연결된다. 외부 의존 없이(D-09)
- 마이그레이션이 모델과 달라도 쿼리의 오류가 같은 실행에 나온다
- 게이트가 짚은 진단·단계 글의 빈틈과, 조용히 틀리는 런타임 넷(시각의 Location, API의 unique 위반, admin의 거절된 삭제, 폼의 `default=''`)이 고쳐져 있다
- 게시판 튜토리얼이 이것들로 걷고, 새 판을 `go install`한 에이전트 게이트가 새 단계 글만 보고 쓴다

**앞에서 넘어온 것**
- MILESTONES "그다음(M13)": 외부 미디어 저장소, 단계를 넘어 계속하는 검사(D-61)
- readmate의 이전에서: OCR 일꾼이 만든 이미지 바이트를 미디어로, Google 로그인(allauth의 `socialaccount`)
- TODO(아래 "먼저 확인한 사실"의 훑기): M12 게이트 셋(import 빠진 `outbox.Admin` 진단, 단계 글의 남은 빈틈, 0040 시험의 `adminApp()`), M10의 시각 Location·admin 상태·`Maago-Expect` 줄:열·버전 칸 fix, M11의 API unique 위반, M4의 `default=''`, M3의 결과 타입 fix 예 이름, M2의 `maago_` 테이블 이름, M12의 걷기 시험 시간

**이번에 하지 않는 것**(TODO 그대로): `Normalize`의 따옴표 이름, 연습 PG의 `search_path`, FK 고리의 DROP 순서, 테이블 `--rename`, 태그 값의 `"`, 빌드 제약 파일, 권한 표의 런타임 검사, 정책 셋(앱을 넘는 정책, auth 모델의 정책, dot import), UUID·JSON의 남은 것(채우는 태그, default, JSON param, `int32` decimal, 옛 행 채우기), `maago doc main` 예의 멈춤, outbox Kind 여럿, 롤백, T3. 다른 소셜 제공자와 MFA는 v1(D-50)

---

## 먼저 확인한 사실 (2026-11-22, 조사 넷)

**미디어**
- 앱은 바이트를 저장할 길이 없다: `SaveMedia(ctx, f File)`는 `f.Open()`으로만 읽고(`media.go:63`), `File`은 내보내지 않는 `*multipart.FileHeader` 하나라(`file.go:15`) 앱이 만들 수 없다
- 일꾼의 ctx에는 미디어 디렉터리가 없다: 디렉터리를 ctx에 넣는 곳은 `Handler`의 요청 경로뿐(`http.go:255`). outbox 일꾼은 `main`의 ctx를 받으므로(`outbox.Run(work, …)`) 그 안의 `SaveMedia`는 "no media directory"(`media.go:40`). 우회 `maago.WithMediaDir`는 문서가 "시험에서"라고만 한다
- 저장소가 공개 API 꼴에 박혀 있다: `OpenMedia`는 `*os.File`을 돌려주고(`media.go:100`), `ServeMedia`는 `Stat().ModTime()`과 seek하는 `http.ServeContent`에 기댄다(`media.go:145`·`162`). 외부 저장소는 `OpenMedia`의 반환을 깬다
- 고아는 문서로만: 행을 지울 때 앱이 `DeleteMedia`, admin은 지우지 않음, Insert 뒤 커밋 실패와 죽은 프로세스의 `.tmp-<key>`는 덮지 않음(`internal/ref/topics.go:410`, `media.go:77`)
- 런타임·contrib는 stdlib만(D-09, `TestRuntimeImports`): S3 SDK는 못 들인다. SigV4는 stdlib(`crypto/hmac`·`sha256`·`net/http`)로 된다
- readmate: OCR이 페이지마다 이미지 여럿과 markdown을 `media/marked/<id>/`에 쓰고, OCR 제공자는 PDF를 공개 URL로 받는다. Maago로 오면 외부 서비스에 줄 서명된 URL이 필요하다

**Google 로그인**
- `contrib/auth`는 `golang.org/x/oauth2`를 쓸 수 없다(D-09). 복사본이 쓰면 사용자 `go.mod`와 `TestAddAuth`의 고정 fixture(`GOPROXY=off`, `add_test.go:66`)에 의존이 생기는데, x/oauth2가 덜어 주는 것은 PKCE와 코드 교환 약 30줄이다(id_token 검증은 하지 않는다)
- stdlib만의 가늠: PKCE·인가 URL·코드 교환·claims 검사 약 100줄. id_token을 토큰 엔드포인트에서 TLS로 직접 받으면 서명 검증을 생략할 수 있다(OIDC Core 3.1.3.7의 6항). JWKS·RS256까지 약 170~200줄
- D-123: 앱 층은 복사, 보안 모양의 부품은 `contrib/auth`(stdlib). 복사본은 이미 `authlib.NewToken`·`SessionCookie`·`SafeNext`를 쓴다
- 비밀번호 없는 계정은 안전하게 거절된다: 빈 해시와 Django의 `!` 해시에 `Check`가 false(`contrib/auth/password.go:61`). 다만 비밀번호를 다시 묻는 넷(`recheck`의 토큰 만들기·탈퇴·비밀번호 변경, `VerifySubmit`)이 Google로만 들어온 사용자를 막는다
- 뷰는 쿠키를 읽지 못한다(In에 쿠키 태그 없음): state를 브라우저에 묶는 콜백은 `r`가 필요하고, D-157의 쓰는 뷰 `func(ctx, db, in, w, r) error`로 된다. 런타임은 그대로
- 게시판 대응표는 OAuthAccount를 "대응 없음"으로 적었다(`MILESTONES.md:183`, `examples/board/MAP.md:12`)

**단계를 넘어 계속하는 검사**
- 단계는 9개(`internal/build/build.go:262`의 `stages`), `pipeline.Generate`는 단계의 오류가 하나라도 있으면 거기서 돌려준다(`pipeline.go:46`). 단계 안에서는 이미 여러 오류를 모은다(`TestEnvTagsAndMigrations`, 게이트의 "9 in one run")
- 연구 10번의 `maxlen` → `auth.App` 왕복은 연쇄가 아니라 단계 순서다: `maxlen`은 ③ `model.Check`, `auth.App`은 ④ `web.Check`이고 뒤는 모델을 읽지 않는다(`internal/web/routes.go:339`)
- 진짜 연쇄: `model.Check`는 태그가 틀려도 모델을 돌려주고(`check.go:101`), DDL·DB·쿼리·admin·폼·정책이 그 위에서 돈다. ⑧ 타입 검사는 ⑦의 생성물에 기댄다
- DB 단계 안의 끊기: 마이그레이션이 모델과 다르면 쿼리를 prepare하지 않는다(`build.go:326`). 모델 DDL의 연습 DB는 이미 따로 만들어 읽고 닫는다(`build.go:449`)
- 게이트 기록 12개에서 단계 경계 때문인 왕복: 약 7~8번, 튜토리얼이 일부러 낸 4번(`maxlen` → `auth.App`)을 빼면 3~4번. 그중 "run makemigrations" → 쿼리 열 불일치가 2번(board-2 0012, m12-board-1 0039). 한 번에 `maago gen` 몇 초
- 바꾸면 같이 고칠 곳: `TUTORIAL-FUTURE.md` §3(두 실패를 일부러 나눔, `TestShortTutorial`), `TestMaxlen`의 `len(ds) != 1`

**TODO 훑기**(열린 33개, 이미 고쳐진 하나 반)
- 이미 고쳐짐: `maago doc` 주제 없음의 exit 1(`cmd/maago/doc.go:28`이 목록과 exit 0) → 체크, 같은 줄의 `go doc contrib/admin`은 Go 도구의 동작이라 지움. M12 게이트 빈틈 중 셋은 69d9ddb에서
- 게이트에서 실제로 걸린 것: 시각의 Location(`InsertX`는 `time.Now()` Local, SQLite `GetX`는 UTC, `m10-board-1`), `Maago-Expect` 줄:열(`m10-board-1`), import 빠진 `outbox.Admin` 진단(`routes.go:303`의 `Uses`는 import된 이름만 센다, `m12-board-2`), 단계 글의 빈틈(m12 둘)
- 조용히 틀림: API 뷰의 `*UniqueError`가 500(`internal/rtcore/rtcore.go:38`의 `Code`가 모른다), admin의 거절된 삭제가 200(`contrib/admin/site.go:494`), 사용자 폼이 `default=''`를 보지 않음(`internal/web/forms.go:443`, admin은 받는다), `table=maago_…`를 막지 않음(`internal/model/check.go:517`)
- fix가 엉뚱한 예: 모델 결과 fix의 `PostSummary`(`internal/mapper/result.go:176`, 게시판에 이미 있음), `has no field Version`의 fix가 `query=version`을 가리키지 않음(`forms.go:424`)
- 걷기 시험: 로컬 `cmd/maago` 약 29분, CI 60분 제한(M12 12번). M13이 단계를 더하면 넘을 수 있다
- 지움 권함: Django 대응표(에이전트 편의와 무관)

## 질문 (초안, 사람의 결정을 기다림)

모두 에이전트 편의로 추천을 골랐다.

- **Q1 범위**: 아래 Q2~Q9를 모두 하면 작업이 약 18개(M12의 14보다 크다).
  (a, 추천) M13 = 걷기 시험 시간을 먼저, 미디어(바이트 API·저장소 경계·S3·청소), Google, TODO 묶음 셋(진단, 조용한 런타임, 단계 글), DB 단계의 쿼리 계속(Q8). 갈래 나누기(Q7)는 하지 않는다. readmate의 이전을 막는 둘과 조용히 틀리는 것을 앞에 둔다
  (b) 넷 모두(갈래 나누기까지)
  (c) M13 = 미디어와 계속하는 검사만(원래의 M13), Google과 TODO는 M14
- **Q2 앱이 만든 바이트의 저장**:
  (a, 추천) `maago.SaveMediaFrom(ctx, name string, r io.Reader) (Media, error)`. 형식은 첫 512바이트로 `http.DetectContentType`(업로드와 같은 뜻), 크기는 센 값, `SaveMedia(ctx, File)`은 이것을 부르는 얇은 함수. 일꾼의 ctx에 저장소를 넣는 공개 함수를 `main`의 꼴로 문서화하고 `maago doc main` 예에. 대가: 상한은 앱의 몫, 공개 이름 하나
  (b) `maago.NewFile(name, []byte) File`로 `SaveMedia` 하나를 둔다. 대가: `File`이 "요청 동안의 폼 값"이라는 뜻이 흐려지고 큰 바이트를 통째로 메모리에
  (c) `SaveMedia`의 서명을 `(ctx, name, io.Reader)`로 바꾼다(API를 깸). 대가: 폼 경로마다 `Open`·`Close`, 게시판 0029·0039를 다시 씀
- **Q3 외부 저장소의 경계**(런타임은 stdlib만):
  (a, 추천) 런타임에 작은 `maago.MediaStore` 인터페이스(Put·Open·Delete·List), 로컬 디스크가 기본(`Config.MediaDir`은 그 줄임). S3 호환 구현은 `contrib/s3`(stdlib SigV4로 PUT·Range GET·DELETE·LIST·서명된 GET URL). 시험은 AWS의 SigV4 시험 벡터와 httptest의 가짜, 실제 서버(MinIO·R2)는 `MAAGO_TEST_S3_URL`이 있을 때만(PG 시험의 꼴). `ServeMedia`는 저장소의 Range 읽기 위로, 지금의 머리 그대로. 대가: SigV4와 S3 변종(R2, 경로·가상 호스트 방식)을 Maago가 유지, `OpenMedia`의 `*os.File`이 깨짐, D-123의 "런타임 공개 API는 루트 하나"에 예외 하나
  (b) 인터페이스와 적합성 시험(`maagotest.MediaStore`)만, 어댑터(aws-sdk-go-v2 위 30줄)는 앱이. 대가: 프로젝트마다 이음새를 다시 쓰고 앱에 SDK, 서명된 URL도 앱의 몫
  (c) 코드 없이 공유 파일시스템(EFS·볼륨)을 문서에. 대가: Fly·Cloud Run에서 안 되고 서명된 URL 없음
- **Q4 고아 청소**:
  (a, 추천) `maago.SweepMedia(ctx, olderThan, inUse func(ctx, keys []string) (map[string]bool, error)) (int, error)`: 저장소의 오래된 열쇠를 묶음으로 넘기고 앱이 쿼리 하나로 "쓰는 중"을 답한다, 남은 `.tmp-*`도. `inUse`가 오류면 아무것도 지우지 않는다. 일꾼이나 `main`의 주기 고루틴이 부르고 시간을 주입해 시험한다. 대가: 어느 열이 열쇠인지 생성기가 모르므로 `inUse`를 빠뜨려도 검사되지 않는다
  (b) 타입 있는 `maago.Media` 모델 필드로 생성기가 열쇠 열을 알게 한다. 대가: 모델·마이그레이션·폼·admin을 걸치는 큰 작업, 게시판 `Attachment`를 다시 씀
  (c) 문서만. 대가: S3에서 보이지 않게 비용이 쌓인다
- **Q5 Google 로그인의 꼴**:
  (a, 추천) M13에 Google 하나. `maago add google`이 이미 있는 `auth/`에 `google.go`(뷰 둘, `OAuthAccount` 모델)·`google_test.go`·`queries/oauth_*.sql`을 덧쓴다(`auth/`가 없거나 그 파일이 있으면 거절, `urls.go`에 넣을 두 줄과 로그인 버튼은 출력으로). 보안 부품(PKCE, state·nonce, 코드 교환, claims 검사)은 stdlib로 `contrib/auth/oidc.go`. 서명 검증은 하지 않는다(TLS 직접 수신, OIDC Core 3.1.3.7). `ClientID`·`ClientSecret`은 복사본의 패키지 변수로 `main`이 채운다. 시험은 httptest의 가짜 제공자. 대가: 앱 디렉터리에 덧쓰는 첫 `add`(D-123에 규칙 하나), 브라우저를 거쳐 오는 id_token이 생기면 그때 JWKS
  (b) `maago add auth`에 처음부터. 대가: 쓰지 않는 프로젝트에도 표·라우트, 이미 auth를 복사한 프로젝트는 손으로 합친다
  (c) 복사본이 `golang.org/x/oauth2`를 쓴다. 대가: 사용자 `go.mod`와 fixture에 의존, 아끼는 것은 약 30줄
- **Q6 Google 계정의 연결**:
  (a, 추천) `OAuthAccount{UserID, Provider, Subject, CreatedAt}`, unique (Provider, Subject). 콜백: (google, sub)가 있으면 로그인(`!IsActive`면 거절) → 없고 `email_verified`가 false면 거절 → 같은 이메일의 인증된 User면 연결 → 인증 안 된 User면 넘겨받기(`VerifiedAt`을 지금, 비밀번호·세션·토큰을 지우고 연결: 남의 이메일로 먼저 가입한 사람이 남지 못하게) → 없으면 비밀번호 없는 User를 만든다. 비밀번호 없는 사용자의 토큰·탈퇴·비밀번호 변경은 먼저 재설정 메일로. allauth의 `socialaccount(provider, uid, user_id)`는 uid가 sub라 그대로 옮긴다. 대가: Google의 `email_verified`를 믿는다
  (b) 이메일로 자동 연결하지 않고, 로그인한 뒤 설정에서 연결. 대가: 화면·해제·권한 표 행이 더 생긴다
  (c) `OAuthAccount` 없이 이메일로만. 대가: Google 이메일이 바뀌면 계정을 잃는다
- **Q7 단계를 넘어 계속하는 검사(갈래)**:
  (a) 모델 갈래(태그 → DDL → DB → 쿼리)와 웹 갈래(`web.Read` → `Check`·`Views`·`Permissions`)로 나눠 한쪽이 실패해도 다른 쪽 오류를 낸다. 합류(폼·admin·정책·타입 검사)는 둘 다 통과해야. 대가: `stages`가 갈래와 합류로 바뀌고 튜토리얼 §3을 다시 씀. 줄이는 왕복은 실측 3~4번(튜토리얼의 4번 빼고)
  (b) 실패한 선언만 빼고 끝까지. 대가: 모든 검사기가 "독" 집합을 알아야 하고 빠뜨린 자리에서 연쇄 오류가 샌다(D-61의 뒤 절)
  (c, 추천) 갈래는 나누지 않고 D-61을 그대로 둔다. 실측 효과가 작고(12 실행에 3~4번, 한 번에 몇 초), 줄일 수 있는 것 중 가장 잦은 것(makemigrations → 쿼리 열)은 Q8이 덮는다. D-148의 목록에서 지우고 까닭을 적는다
- **Q8 DB 단계 안의 쿼리 계속**:
  (a, 추천) 마이그레이션이 모델과 달라도 쿼리를 모델 DDL의 연습 DB에 prepare해 "run makemigrations"와 쿼리 오류를 한 실행에. CRUD·admin의 prepare는 지금처럼 같을 때만. 대가: 둘째 연습 DB를 쿼리 끝까지 열어 둔다, 손으로 쓴 마이그레이션이 모델 DDL과 다른 모양이면 결과가 다를 수 있다(다음 gen이 다시 본다)
  (b) 그대로. 대가: makemigrations를 먼저 하지 않은 에이전트는 한 번 더 왕복
- **Q9 API 뷰의 unique 위반**:
  (a, 추천) `rtcore.Code`가 `*UniqueError`를 422 problem+json으로, 필드 이름을 `errors`에(생성된 `invalidX`와 같은 꼴). 폼 검증 실패와 같은 상태라 클라이언트가 한 길로 다룬다
  (b) 409 Conflict. 대가: 클라이언트가 422와 409를 따로 다룬다
- **Q10 끝의 릴리스**:
  (a, 추천) M13의 끝에 v0.4.0(API가 는다: `SaveMediaFrom`, `MediaStore`, `contrib/s3`, `maago add google`) 태그와 그 판을 `go install`한 에이전트 게이트(M11·M12의 길)
  (b) 태그 없이 저장소에서 지은 `maago`로 게이트

## 작업 (초안, Q의 추천대로)

1. **걷기 시험 시간**: `TestBoardTutorialCatches`의 줄을 병렬로, 또는 같은 단계까지의 걷기를 한 번 만들어 나눠 쓴다(단계를 더하기 전에) → 확인: 고치기 전후 로컬 `cmd/maago` 시간과 CI 시간을 기록, Catches 줄마다 여전히 실패를 잡는다(한 줄을 헛것으로 만들면 실패)
2. **진단 묶음**: import 없이 `<app>.Admin`·`<app>.App`을 쓴 파일을 짚는 진단, 모델 결과 fix의 예 이름을 `<Query>Row`로, `has no field Version`의 fix가 `query=version`을, `table=maago_…` 거절 → 확인: `internal/web`·`internal/admingen` 시험에 import 빠진 main(`.Admin`·`.App`), `internal/mapper` 시험의 fix 전문, `internal/model` 시험의 거절
3. **조용한 런타임 묶음**(Q9): `InsertX`가 돌려주는 시각을 `GetX`와 같은 Location으로, API 뷰의 unique 위반 422, admin의 거절된 삭제를 4xx와 이유로, 사용자 폼의 `default=''` → 확인: 두 방언에서 `InsertX`와 `GetX`의 결과가 `==`, API unique 위반의 422 problem+json, `contrib/admin`의 거절 상태, `internal/web/forms_test.go`의 빈 입력
4. **DB 단계의 쿼리 계속**(Q8): → 확인: 모델에 열을 더하고 makemigrations 없이 gen하면 "run makemigrations"와 "result columns do not match … missing"이 한 실행에(두 방언), 게시판 0012·0039 글의 "먼저 makemigrations"를 다시 봄, D-61·D-23 고침, D-148에서 "계속하는 검사"를 지우고 까닭(Q7)
5. **바이트로 미디어 저장**(Q2): `SaveMediaFrom`, 일꾼의 ctx에 저장소를 넣는 공개 길 → 확인: `media_test.go`의 바이트·빈 입력·형식, 복사된 outbox 일꾼의 handle 안에서 저장이 성공(`TestAddOutbox`, 두 방언), `maago doc main` 예(`TestDocExamples`)
6. **저장소 경계**(Q3): `MediaStore`와 로컬 구현, `OpenMedia`·`ServeMedia`를 그 위로 → 확인: `TestMedia`·`TestServeMedia`(Range·404·nosniff) 그대로, 게시판 0029·0038·0039가 고치지 않고 걷는다(`TestBoardTutorial`)
7. **`contrib/s3`**: stdlib SigV4, Range GET, LIST, 서명된 URL → 확인: AWS SigV4 시험 벡터, 가짜 S3에서 같은 `ServeMedia` 시험, `MAAGO_TEST_S3_URL`(MinIO)이 있을 때의 실제 시험, `TestRuntimeImports`
8. **고아 청소**(Q4): `SweepMedia` → 확인: 오래된 고아만 지움, 새 것과 `.tmp-`, `inUse`가 오류면 0개, 로컬과 가짜 S3 둘
9. **`contrib/auth/oidc.go`**(Q5): PKCE, 인가 URL, 코드 교환, claims 검사 → 확인: `TestOIDC*`(httptest: 틀린 nonce·aud·iss, 지난 exp, `email_verified` false, 엔드포인트 오류를 거절), `TestRuntimeImports`
10. **`maago add google`**(Q5·Q6): `auth/`에 덧쓰기, 거절 둘, 뷰 둘(콜백은 쓰는 뷰), `OAuthAccount`와 연결 차례 → 확인: `TestAddGoogle`(두 방언, `GOPROXY=off`로 복사본 시험 초록), 복사본의 `TestGoogle*`(새 사용자, sub로 다시, 인증된 이메일 연결, 인증 안 된 계정 넘겨받기, state 없음·다름 400, 비활성 거절, `next`의 `SafeNext`), 권한 표의 행
11. **게시판 0041~**: 일꾼이 바이트를 미디어로(그림의 썸네일 따위, 쓰임은 계획에서), 저장소를 환경으로 고름, 청소, Google 로그인(가짜 제공자 시험), 단계 글의 빈틈 묶음(0038~0040의 남은 것, 0040 시험의 `adminApp()`, `Maago-Expect` 줄:열을 글이 보이게) → 확인: `TestBoardTutorial`·`TestBoardTutorialCatches`·`TestBoardMap`(MAP의 OAuthAccount 행), Catches(일꾼 ctx에 저장소 없음, `inUse`를 빠뜨림 따위)
12. **문서**: `maago doc`의 미디어·main·auth(Google 콘솔의 redirect URI, 환경 변수, allauth 표에서 옮기는 SQL), 결정 고침: D-140·D-153·D-159·D-123·D-72(앱이 만든 바이트는 이미지 처리가 아니다)·D-50·D-130·D-129·D-33·D-127·D-61·D-148, MILESTONES v1에서 옮김 → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
13. **v0.4.0**(Q10): 태그, 게시판의 `go.mod`(0001·0013) → 확인: 빈 GOPATH에서 `go install …@v0.4.0`, 공개 판으로 걷기의 끝(`GOWORK=off`)
14. **에이전트 게이트**: `go install …@v0.4.0`부터, 0040의 끝에서 새 단계 글만 보고 서로 모르는 실행 둘 → 확인: 기록, 메시지와 `maago doc`만으로 씀
15. **M13 끝**: MILESTONES의 M13 체크, README, 다음 마일스톤의 차례

## M13 끝

- [ ] 1~15 완료
- [ ] M13 완료 조건을 실제로 돌려 확인, 조건마다 시험 이름
- [ ] MILESTONES.md의 M13 체크박스
- [ ] push 뒤 GitHub Actions
