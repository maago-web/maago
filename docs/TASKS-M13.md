# M13 작업 목록 — 미디어 저장소와 조용한 맞음

[MILESTONES.md](MILESTONES.md)의 M13을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).
범위는 사람의 결정(2026-11-22, M12 끝): 외부 미디어 저장소, 단계를 넘어 계속하는 검사, TODO의 남은 것, 실제 이전 사례(readmate: Django에서 옮기는 서비스)를 모아 질문으로 묶는다.

차례와 범위는 사람의 결정(2026-11-22, Q1): M13은 미디어와 조용한 맞음, Google 로그인·진단 묶음·단계 글은 M14. 결정은 D-160~D-163.

**M13 완료 조건** (MILESTONES)
- 앱이 만든 바이트(백그라운드 일꾼의 것도)를 미디어로 저장한다. 인스턴스가 둘이어도 미디어가 깨지지 않는 저장소(S3 호환)가 있고, 고아 파일을 치우는 길이 있다
- 마이그레이션이 모델과 달라도 쿼리의 오류가 같은 실행에 나온다
- 조용히 틀리는 런타임 넷(시각의 Location, API의 unique 위반, admin의 거절된 삭제, 폼의 `default=''`)이 고쳐져 있다
- 게시판 튜토리얼이 이것들로 걷고, 새 판을 `go install`한 에이전트 게이트가 새 단계 글만 보고 쓴다

**앞에서 넘어온 것**
- MILESTONES "그다음(M13)": 외부 미디어 저장소, 단계를 넘어 계속하는 검사(D-61)
- readmate의 이전에서: OCR 일꾼이 만든 이미지 바이트를 미디어로. Google 로그인(allauth의 `socialaccount`)은 M14(D-163)
- TODO(아래 "먼저 확인한 사실"의 훑기): M12 게이트 셋(import 빠진 `outbox.Admin` 진단, 단계 글의 남은 빈틈, 0040 시험의 `adminApp()`), M10의 시각 Location·admin 상태·`Maago-Expect` 줄:열·버전 칸 fix, M11의 API unique 위반, M4의 `default=''`, M3의 결과 타입 fix 예 이름, M2의 `maago_` 테이블 이름, M12의 걷기 시험 시간

**이번에 하지 않는 것**(TODO 그대로): `Normalize`의 따옴표 이름, 연습 PG의 `search_path`, FK 고리의 DROP 순서, 테이블 `--rename`, 태그 값의 `"`, 빌드 제약 파일, 권한 표의 런타임 검사, 정책 셋(앱을 넘는 정책, auth 모델의 정책, dot import), UUID·JSON의 남은 것(채우는 태그, default, JSON param, `int32` decimal, 옛 행 채우기), `maago doc main` 예의 멈춤, 롤백, T3, JSON 필드의 `default`(앱이 Insert에서 채운다고 문서에). 다른 소셜 제공자와 MFA는 v1(D-50)

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
- 단계는 9개: `stages`(`internal/build/build.go:262`)의 바깥 단계 6과 생성·타입 검사·생성의 3(`build.go:155`), `pipeline.Generate`는 단계의 오류가 하나라도 있으면 거기서 돌려준다(`pipeline.go:46`). 단계 안에서는 이미 여러 오류를 모은다(`TestEnvTagsAndMigrations`, 게이트의 "9 in one run")
- 연구 10번의 `maxlen` → `auth.App` 왕복은 연쇄가 아니라 단계 순서다: `maxlen`은 ③ `model.Check`, `auth.App`은 ④ `web.Check`이고 뒤는 모델을 읽지 않는다(`internal/web/routes.go:339`)
- 진짜 연쇄: `model.Check`는 태그가 틀려도 모델을 돌려주고(`check.go:101`), DDL·DB·쿼리·admin·폼·정책이 그 위에서 돈다. ⑧ 타입 검사는 ⑦의 생성물에 기댄다
- DB 단계 안의 끊기: 마이그레이션이 모델과 다르면 쿼리를 prepare하지 않는다(`build.go:326`). 모델 DDL의 연습 DB는 이미 따로 만들어 읽고 닫는다(`build.go:449`)
- 게이트 기록 12개에서 단계 경계 때문인 왕복: 약 7~8번, 튜토리얼이 일부러 낸 4번(`maxlen` → `auth.App`)을 빼면 3~4번. 그중 "run makemigrations" → 쿼리 열 불일치가 2번(board-2 0012, m12-board-1 0039). 한 번에 `maago gen` 몇 초
- 바꾸면 같이 고칠 곳: `TUTORIAL-FUTURE.md` §3(두 실패를 일부러 나눔, `TestShortTutorial`), `TestMaxlen`의 `len(ds) != 1`

**TODO 훑기**(열린 33개, 이미 고쳐진 하나 반)
- 이미 고쳐짐: `maago doc` 주제 없음의 exit 1(`cmd/maago/doc.go:28`이 목록과 exit 0) → 체크, 같은 줄의 `go doc contrib/admin`은 Go 도구의 동작이라 지움. M12 게이트 빈틈 중 셋은 69d9ddb에서
- 게이트에서 실제로 걸린 것: 시각의 Location(보고됨, 먼저 재현: TODO는 "확인하지 않음", `InsertX`는 `time.Now()` Local, SQLite `GetX`는 UTC, `m10-board-1`), `Maago-Expect` 줄:열(`m10-board-1`), import 빠진 `outbox.Admin` 진단(`routes.go:303`의 `Uses`는 import된 이름만 센다, `m12-board-2`), 단계 글의 빈틈(m12 둘)
- 조용히 틀림: API 뷰의 `*UniqueError`가 500(`internal/rtcore/rtcore.go:38`의 `Code`가 모른다), admin의 거절된 삭제가 200(`contrib/admin/site.go:494`), 사용자 폼이 `default=''`를 보지 않음(`internal/web/forms.go:443`, admin은 받는다), `table=maago_…`를 막지 않음(`internal/model/check.go:517`)
- fix가 엉뚱한 예: 모델 결과 fix의 `PostSummary`(`internal/mapper/result.go:176`, 게시판에 이미 있음), `has no field Version`의 fix가 `query=version`을 가리키지 않음(`forms.go:424`)
- 걷기 시험: 로컬 `cmd/maago` 약 29분, CI 60분 제한(M12 12번). M13이 단계를 더하면 넘을 수 있다
- 지움 권함: Django 대응표(에이전트 편의와 무관)

## 질문과 답 (초안 리뷰 뒤, 2026-11-22)

초안 리뷰(아래)로 고친 추천이다. 모두 에이전트 편의로 골랐다. Q1(범위)과 Q6(계정 연결)은 사람이 답했고(Q1은 추천대로, Q6은 (b)), 나머지는 리뷰 둘이 막지 않아 추천대로 정했다(에이전트 편의로 결정). 결정은 D-160~D-163.

- **Q1 범위**(사람의 결정: (a)): 리뷰가 고친 작업은 미디어 다섯, Google 셋, 그 밖 다섯과 게시판·릴리스·게이트로 약 18개. 5~10번이 모두 새 API·보안·파일 지우기라 리뷰어 둘을 부르는 작업이다(WORKFLOW 3).
  (a, 추천) **둘로 자른다.** M13 = 걷기 시험 시간, 조용한 런타임, DB 단계의 쿼리 계속(Q8), 미디어(경계·바이트·S3·청소), 게시판 단계, v0.4.0, 게이트. M14 = Google 로그인, 진단 묶음, 단계 글의 빈틈. 한 게이트가 보는 새 주제를 줄이고, readmate의 첫 막힘(일꾼의 바이트)을 먼저 푼다. 대가: readmate의 Google 로그인은 M14까지 기다리거나 그동안 이메일 로그인으로
  (b) 모두 M13(Google을 미디어와 함께). 게시판 단계를 미디어 뒤·Google 뒤 둘로 쪼갠다. 대가: 마일스톤이 길고 게이트 한 번이 새 주제 다섯을 본다
  (c) M13 = Google과 조용한 런타임(readmate의 두 막힘 중 로그인을 먼저), 미디어는 M14
- **Q2 앱이 만든 바이트의 저장**(정함): `maago.SaveMediaFrom(ctx, name string, r io.Reader) (Media, error)`. 실제로 복사한 바이트 수를 `Size`로, 형식은 읽은 첫 512바이트의 `http.DetectContentType`(업로드와 같은 뜻), 상한은 앱의 몫(`io.LimitReader`를 문서에). `SaveMedia(ctx, File)`은 이것을 부르는 얇은 함수. 일꾼의 ctx에 저장소를 넣는 공개 함수는 경계(Q3) 뒤에 한 번만 이름 짓는다. 기각: `NewFile([]byte)`(폼 값의 뜻이 흐려지고 통째로 메모리), `SaveMedia`의 서명 바꾸기(폼 경로마다 `Open`·`Close`)
- **Q3 외부 저장소의 경계**(정함): 런타임에 작은 `maago.MediaStore`: Put(이름·형식·Reader → 크기), Open(크기·시각·Range 읽기를 주는 `io.ReadSeeker`꼴), Delete, List(페이지 나눔), URL(Media, ttl). 로컬 디스크가 기본이고 **단일 인스턴스용**이라고 문서에, `Config.MediaDir`은 그 줄임이며 저장소와 함께 주면 `Handler` 오류(PHILOSOPHY 6). S3 호환은 `contrib/s3`(stdlib SigV4): 크기를 모르는 Reader는 임시 파일에 받아 센 뒤 PUT. **서명된 URL**은 기한이 필수이고 `response-content-disposition=attachment`를 강제하며, 권한을 본 뷰나 일꾼만 부른다(D-140 고침: 기한부 공개 하나). 로컬 저장소도 같은 `URL`을 HMAC으로 서명한 기한부 라우트로 준다(로컬 개발에서 같은 코드가 돈다, readmate의 OCR이 PDF를 URL로 받는다). `OpenMedia`의 `*os.File` 반환이 깨진다: 릴리스 글에 적는다. 기각: 인터페이스만 두고 SDK 어댑터는 앱이(프로젝트마다 이음새), 공유 파일시스템 문서만(서명된 URL 없음)
- **Q4 고아 청소**(정함, 리뷰가 바꾼 추천): 열쇠 열에 필드 태그 `media`를 붙이면 생성기가 모든 앱의 그 열을 읽는 "쓰는 중" 쿼리를 만들고, `maago.SweepMedia(ctx, olderThan)`가 저장소의 오래된 열쇠(기본 여유 24시간) 중 쓰이지 않는 것과 남은 `.tmp-*`를 지운다. 태그 없는 프로젝트에서 `SweepMedia`는 오류. 손으로 쓴 `inUse`는 기각: 열쇠 열을 가진 둘째 모델을 나중에 더하면 쓰는 파일을 조용히 지운다(MISSION의 데이터 보존, PHILOSOPHY 2). 타입 있는 `maago.Media` 필드는 기각: 모델·폼·admin을 걸치고 게시판 `Attachment`를 다시 쓴다
- **Q5 Google 로그인의 꼴**(M14, 정함): `maago add google`이 이미 있는 `auth/`에 `google.go`(뷰 둘, `OAuthAccount`)·`google_test.go`·`queries/oauth_*.sql`을 덧쓴다(`auth/`가 없거나 그 파일이 있으면 거절). 복사된 `google_test.go`는 두 라우트와 권한 표 행이 없으면 실패하고, 넣을 줄은 출력이 짚는다. 보안 부품(PKCE, state·nonce, 코드 교환, claims 검사)은 stdlib로 `contrib/auth/oidc.go`. **서명 검증 생략은 고정된 Google 토큰 엔드포인트에서 HTTPS로 직접 받은 id_token에만**: `contrib/auth`의 공개 API에 id_token 글을 받는 함수가 없고(코드 교환만 claims를 준다), iss·aud·azp·nonce·exp를 본다, `http://`와 다른 엔드포인트는 거절. state·nonce·verifier는 HttpOnly·Secure·SameSite=Lax의 짧은(10분) 쿠키 하나에, 콜백이 성공이든 실패든 지워 재사용을 거절한다. 비밀번호를 다시 묻는 넷(`recheck`·`VerifySubmit`)은 새 `add auth` 원본에서 고치고, 이미 복사한 프로젝트에는 출력이 짚는다. 기각: `add auth`에 처음부터(이미 복사한 프로젝트가 손으로 합침), `x/oauth2`(의존과 fixture, 아끼는 것 약 30줄)
- **Q6 Google 계정의 연결**(M14, 사람의 결정: (b). 리뷰가 고친 추천은 (a)였다):
  (a, 추천) **자동 연결·넘겨받기 없음.** (google, sub)가 있으면 로그인(`!IsActive`면 거절). 없고 `email_verified`가 false면 거절. 정규화한 이메일(`normalizeEmail`)의 User가 있으면 연결하지 않고 "이 이메일의 계정이 있다: 비밀번호로 로그인한 뒤 연결하거나 비밀번호를 재설정하라"로 돌려보낸다(재설정 메일은 이메일 주인에게 가므로 남의 이메일로 먼저 가입한 사람이 남지 못한다). 로그인한 사용자의 "Google 연결" 뷰가 연결한다. 없으면 비밀번호 없는 User를 만든다. 쓰기는 `WithTx` 하나에서, unique (Provider, Subject). readmate의 `socialaccount(provider, uid, user_id)`는 uid가 sub라 그대로 옮기므로 이미 연결된 사용자는 막히지 않는다. 대가: 처음 연결하는 사람은 한 번 더(로그인 뒤 연결), 뷰와 권한 표 행 하나
  **(b, 사람의 결정)** 같은 정규화 이메일(`normalizeEmail`)의 인증된 User면 자동 연결(allauth의 email authentication), 인증 안 된 User는 넘겨받기(`VerifiedAt`을 지금, 비밀번호·세션·토큰을 지움), 탈퇴(`!IsActive`)는 어느 갈래에서도 거절, `email_verified`가 false면 거절. 쓰기는 `WithTx` 하나에서(넘겨받기 중간 실패는 아무것도 바꾸지 않는다), unique (Provider, Subject). 대가: Google의 `email_verified`를 믿고, 비밀번호 계정이 다시 인증 없이 Google로 열린다(codex가 막자고 했다: M14 계획에서 대가를 문서에)
  (c) `OAuthAccount` 없이 이메일로만. 대가: Google 이메일이 바뀌면 계정을 잃는다
- **Q7 단계를 넘어 계속하는 검사(갈래)**(정함): 나누지 않고 D-61을 그대로 둔다. 실측 효과가 작고(12 실행에 3~4번, 한 번에 몇 초), 갈래를 나누면 D-61의 뒤 절(연쇄 오류는 뺀다)에서 새는 자리가 는다. 가장 잦은 것(makemigrations → 쿼리 열)은 Q8이 덮는다. D-148의 목록에서 지우고 까닭을 적는다
- **Q8 DB 단계 안의 쿼리 계속**(정함): 마이그레이션이 모델과 다를 때 쿼리를 모델 DDL의 연습 DB에 prepare해 "run makemigrations"와 쿼리 오류를 한 실행에, 그 진단은 "against the models"라고 말한다. 손 마이그레이션에만 있는 것(인덱스·뷰·확장)을 쓰는 쿼리가 거짓 오류를 받지 않게: 모델 DB에서 실패하고 그 실패가 마이그레이션의 차이와 닿지 않으면 내지 않는다(계획에서 꼴). CRUD·admin의 prepare는 지금처럼 같을 때만. D-23·D-34·D-32 고침
- **Q9 API 뷰의 unique 위반**(정함): 422 problem+json. `UniqueError.Fields`가 있으면 생성된 `invalidX`의 꼴로 `errors`에, nil이면 필드 없는 422. 409는 이미 `ErrConflict`(버전 충돌)의 것. HTML 뷰는 `Fail`을 안 부르면 지금처럼 500(`query.go`의 주석을 고친다). 같은 묶음: 사용자 모델 폼도 `default=''`인 문자열 칸의 빈 입력을 받는다(admin과 같게, D-132), admin의 잘못된 POST에 이유를 보인다
- **Q10 끝의 릴리스**(정함): M13의 끝에 v0.4.0 태그와 그 판을 `go install`한 에이전트 게이트(M11·M12의 길). `OpenMedia`의 반환이 바뀌므로 태그의 글에 깨지는 API를 적는다

## 작업 (초안 리뷰 뒤)

병렬로 할 수 있는 묶음(각자의 worktree, 루프마다 계획 → 계획 리뷰 → 구현 → 구현 리뷰): **가** 1번, **나** 2번, **다** 3번, **라** 4~7번(차례대로). 8번은 모두 뒤, 9~12번은 차례대로.

1. **걷기 시험 시간**: `TestBoardTutorialCatches`의 줄을 병렬로, 또는 같은 단계까지의 걷기를 한 번 만들어 나눠 쓴다(단계를 더하기 전에) → 확인: Catches의 줄 수와 줄마다의 기대 메시지가 고치기 전과 같다(시험이 센다), 한 줄을 헛것으로 만들면 실패, 로컬 `cmd/maago`와 CI의 시간이 기준(계획에서 N분) 아래

   계획
   - 잰 것(2026-11-22, 로컬 M-시리즈 12코어, PG 55432): 걷기 한 번(`TestBoardTutorial`, 0001~0040과 ship) 82초, `cmd/maago`에서 Catches를 뺀 것 142초, Catches는 약 27분(TODO의 29분에서 뺀 값). CI의 `cmd/maago`는 2492초(약 41.5분, 실행 44934297161). 까닭은 TODO 그대로: Catches의 36줄이 저마다 0001부터 걸어 줄 수 × 단계 수로 자란다
   - **같은 앞부분을 한 번 걷고 나눠 쓴다**(병렬이 아니라): Catches가 고치지 않은 단계를 0001부터 한 번 걷고, 단계 k를 적용하기 **전에** 그 단계를 고치는 줄마다 지금의 프로젝트를 복사해 서브시험에서 고친 k, 그 뒤 원래 단계들, 끝의 ship을 이어 걷는다. 걷기가 실패하는 자리와 메시지(`at+": "+c.by`)는 그대로이고, 줄마다 앞부분이 고치기 전과 같은 상태라 뜻이 같다. 가늠: 걷기 한 번 + 36줄 × 한 단계(몇 초) + ship까지 가는 넷의 꼬리(0033에서 둘, 0040에서 둘)로 약 5분. 단계를 더해도 단계 수 + 줄 수로 자란다
   - `cmd/maago/tutorial_test.go`만 바꾼다(새 파일 없음):
     - `walk`를 셋으로 가른다: 프로젝트를 만드는 부분(`git init`·`go.work`·`.env`·`noEnv`), 단계 하나(트레일러 읽기·`toPostgres`·`walkStep`, 오류에 패치 이름), 끝(`ship`, 오류에 마지막 패치 이름). 루트와 스키마를 담는 작은 구조체 `walker{root, schema string}`. `walk(t, dir)`은 그 셋을 차례로 부르는 지금의 함수로 남아 `TestBoardTutorial`·`TestBoardTutorialPostgresTrailer`는 그대로
     - `clone(t)`: 새 `t.TempDir()`(go.work 때문에 `EvalSymlinks`)에 `os.CopyFS`로 루트를 통째로(`.git`, SQLite의 `board.db`, `.env`). 스키마가 있으면(0013 뒤) 서브시험의 이름으로 `toPostgres(t, root, "", 1)`가 새 스키마와 `.env`를 만들고 `build.Migrate`가 그 단계까지의 마이그레이션을 적용한다(PG에는 스키마 복사가 없다. 단계가 공유하는 DB에는 `maago migrate`가 만든 것뿐이라 같은 상태). 부모의 스키마는 건드리지 않는다
     - `TestBoardTutorialCatches`: 줄의 표(리터럴)는 한 글자도 바꾸지 않는다. 고친 패치는 서브시험의 TempDir에 같은 이름으로 쓴다(오류의 패치 이름과 `git apply`). 부모가 단계마다 그 단계를 고치는 줄을 `t.Run`으로 돌린 뒤 원래 단계를 적용한다. 돈 줄을 세어 끝에 `ran != len(rows)`면 실패(없는 패치 이름을 적은 줄이 조용히 빠지지 않게: 지금은 `readFile`이 실패시키던 것)
     - 주석: Catches 위에 "앞부분은 한 번 걷고 줄마다 복사한다"
   - 단계와 확인
     1. 고치기 전 줄 이름 36개를 적어 둔다(`grep`으로 표에서) → 확인: 고친 뒤 `MAAGO_TEST_PG_URL=… go test -run TestBoardTutorialCatches -v ./cmd/maago`의 `--- PASS` 서브시험이 같은 36개(이름이 같다), `git diff`에 표의 줄이 없다
     2. 헛것 넷 → 확인: 각각 실패하고 되돌린다. (a) 0013 앞 줄 하나("query")의 `new`를 `old`로 (b) 0013 뒤 줄 하나("author check") (c) 꼬리 줄 하나("no worker") (d) 한 줄의 패치 이름을 없는 이름으로(`ran != len(rows)`). 복사를 단계 **뒤**에 하는 실수는 고친 패치의 `git apply`가 실패해 모든 줄의 접두가 틀리므로 따로 잡을 것이 없다
     3. PG 없이 로컬 → 확인: 0012까지의 줄은 돌고 그 뒤는 부모가 0013에서 skip(지금도 그 줄들은 skip), `CI=1`이면 0013에서 실패(D-91, `TestBoardTutorialPostgresTrailer` 그대로)
     4. 시간 → 확인: 로컬 Catches **8분** 아래, 로컬 `cmd/maago` **12분** 아래(PG와 함께, 로그의 `go test exit=` 줄). CI의 `cmd/maago` **20분** 아래는 push한 뒤 GitHub Actions에서(push는 묻고 하므로 M13 끝의 "push 뒤 GitHub Actions"에서 재고 TODO에 적는다)
     5. 전체 → 확인: `go mod tidy -diff && go vet ./... && go test -timeout 60m ./...`(PG와 함께)와 `gofmt -l .` 비어 있음, TODO 81줄을 체크하고 잰 시간을 적는다
   - ★ → 에이전트 편의로 결정: **병렬 대신 나눠 쓰기.** 에이전트가 단계를 더할 때마다 Catches가 줄 × 단계로 자라는 것을 막는 쪽은 나눠 쓰기뿐이다(병렬은 코어 수로 나눌 뿐이고, 걷기의 `go test ./...`가 이미 코어를 쓴다: 걷기 한 번 192% CPU). 병렬에는 걸림도 둘: `walk`의 `noEnv`가 `t.Setenv`라 병렬 서브시험에서 panic, 그리고 0013 뒤 걷기들의 앱 시험이 같은 `maagotest_*` 스키마(모듈 경로·디렉터리·시험 이름의 해시라 걷기마다 같다)를 서로 지운다
   - ★ → 에이전트 편의로 결정: 줄 수는 고정 숫자(36)가 아니라 돈 줄 = 표의 줄로 센다. 8번이 줄을 더할 때 숫자를 따라 고칠 일이 없고, 줄이 조용히 빠지는 길(없는 패치 이름)은 막힌다. 표가 그대로인 것은 이 작업의 `git diff`가 보인다
   - ★ → 에이전트 편의로 결정: PG 줄의 복사는 새 스키마 + `build.Migrate`(위). `pg_dump` 따위 새 외부 도구를 걷기에 들이지 않는다
   - 결정 기록 없음: 시험 안의 구조이고 D-103(패치를 하나씩 적용하며 gen·check·migrate·앱 시험)의 뜻은 그대로
   - 하지 않는 것: 서브시험 병렬(위), `TestBoardTutorial`의 걷기를 Catches와 나눠 쓰기(82초를 아끼지만 두 시험의 실패가 엉킨다), CI의 `-timeout 60m`을 되돌리기(M12 12번 사람의 결정, CI 시간을 재기 전에는 근거가 없다), `TestShortTutorial`(이 작업의 범위 밖)
2. **조용한 런타임 묶음**(Q9): `InsertX`·`UpdateX`·`GetX`의 시각을 두 방언 모두 UTC로(보고된 것, 먼저 재현), API 뷰의 unique 위반 422, admin의 거절된 삭제를 4xx와 이유로·잘못된 POST의 이유, 사용자 폼의 `default=''` → 확인: 고치기 전 SQLite에서 빨강인 시각 시험과 PG의 `Location() == time.UTC`, API unique 위반의 422(필드 있음·없음), `contrib/admin`의 거절 상태, `internal/web/forms_test.go`의 빈 입력. D-105·D-111·D-154·D-120·D-132 고침
3. **DB 단계의 쿼리 계속**(Q8) → 확인: 두 방언에서 모델에 열을 더하고 makemigrations 없이 gen하면 "run makemigrations"와 쿼리의 "missing" 진단이 모두 위치와 함께 한 실행에(모든 쿼리를 본다), 새 열을 맞게 쓴 쿼리는 "run makemigrations" 하나뿐, 손 마이그레이션에만 있는 것을 쓰는 쿼리는 이 길에서 오류가 없다. 게시판 0012·0039 글의 "먼저 makemigrations"를 다시 봄
4. **저장소 경계**(Q3): `MediaStore`와 로컬 구현(서명된 로컬 URL 포함), `OpenMedia`·`ServeMedia`를 그 위로, `WithMediaDir`의 처지(남기거나 지움, 계획에서 하나), `MediaDir`과 저장소를 함께 주면 `Handler` 오류 → 확인: 미디어 시험을 저장소를 받는 함수 하나로 만들어 로컬과 메모리 가짜 둘에(7번의 S3도 같은 함수), Range(206)·404·nosniff·attachment, 서명된 URL의 기한 지남·바뀐 서명 거절, 게시판 0029·0038·0039가 고치지 않고 걷는다
5. **바이트로 미디어 저장**(Q2): `SaveMediaFrom`, 일꾼의 ctx에 저장소를 넣는 공개 함수 → 확인: 저장소 시험 함수에 바이트·빈 입력·형식·센 크기, 복사된 outbox 일꾼의 handle 안에서 저장이 성공(`TestAddOutbox`, 두 방언), `maago doc main` 예(`TestDocExamples`)
6. **`contrib/s3`**: stdlib SigV4(머리 서명과 서명된 URL), Range GET, LIST(페이지 나눔), 크기 모르는 Reader → 확인: S3 문서의 예 벡터(머리 서명과 서명된 URL 둘), 4번의 저장소 시험 함수를 가짜 S3에(가짜는 서명을 다시 계산해 틀리면 403), 길이 모르는 Reader의 `Content-Length`, CI에 MinIO 서비스와 `MAAGO_TEST_S3_URL`(D-91의 꼴, 없으면 CI에서 실패), `TestRuntimeImports`
7. **고아 청소**(Q4): 태그 `media`, 생성된 "쓰는 중" 쿼리, `SweepMedia` → 확인: 오래된 고아만 지움, 여유 안의 새 것과 `.tmp-`, 태그 없는 프로젝트의 오류, 열쇠 열이 둘인 모델·두 앱에서 둘 다 쓰는 중으로 봄, 로컬과 가짜 S3 둘(LIST 페이지 둘 이상)
8. **게시판 0041~**: 일꾼이 만든 파일 여럿(내보내기 따위, 이미지 처리 아님: D-72)을 이름 → 열쇠 행으로 저장하고 뷰가 이름으로 찾아 준다(readmate의 `media/marked/<id>/` 꼴), 저장소를 환경으로 고름, 서명된 URL로 밖에 건넴, 청소(태그), outbox의 Kind 둘(웹훅과 이 일: `main`의 일꾼이 Kind로 가름), lease를 넘어 다시 도는 일이 남긴 파일은 청소가 치운다 → 확인: `TestBoardTutorial`·`TestBoardTutorialCatches`·`TestBoardMap`, Catches(일꾼 ctx에 저장소 없음, 태그 빠뜨림 따위)
9. **문서**: `maago doc`의 미디어(단일 인스턴스, 서명된 URL, 청소)·main(일꾼의 저장소, Kind 둘)·model(태그 `media`), 결정 고침: D-140·D-153·D-159·D-123(루트 밖 공개 패키지의 셋째 `contrib/s3`)·D-09·D-72(앱이 만든 바이트는 이미지 처리가 아니다)·D-61·D-148·D-23·D-34·D-32·D-91·D-105·D-111·D-154·D-120·D-132, MILESTONES v1에서 옮김 → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
10. **v0.4.0**(Q10): 태그(깨지는 API를 태그 글에), 게시판의 `go.mod`(0001·0013) → 확인: 빈 GOPATH에서 `go install …@v0.4.0`, 공개 판으로 걷기의 끝(`GOWORK=off`)
11. **에이전트 게이트**: `go install …@v0.4.0`부터, 0040의 끝에서 새 단계 글만 보고 서로 모르는 실행 둘 → 확인: 기록, 메시지와 `maago doc`만으로 씀
12. **M13 끝**: MILESTONES의 M13 체크, README, 다음 마일스톤(M14: Google 로그인, 진단 묶음, 단계 글의 빈틈)의 차례

M14로 간 셋(그 마일스톤의 초안에서 다시 본다): `contrib/auth/oidc.go`(확인: `TestOIDC*`의 틀린 nonce·aud·azp·iss·지난 exp·`email_verified` false·`http://` 엔드포인트 거절), `maago add google`(확인: `TestAddGoogle` 두 방언·User에 필드를 더한 `auth/` 위에서도, 복사본의 `TestGoogle*`가 가짜 제공자와 진짜 쿠키를 왕복해 state·nonce·verifier 재사용·쿠키 속성·탈퇴 계정의 이메일로 온 Google 거절·Q6 (b)의 갈래마다·넘겨받기 중간 실패의 원자성·같은 sub의 동시 콜백 둘), 진단 묶음(import 빠진 `outbox.Admin`·`.App`, 결과 타입 fix의 `<Query>Row`, 버전 칸 fix, `table=maago_…`), 단계 글의 빈틈(0038~0040의 남은 것, 0040 시험의 `adminApp()`, `Maago-Expect` 줄:열).

## 초안 리뷰 (2026-11-22, codex와 fable)

- **범위가 크다**(fable: 5~10번이 모두 리뷰어 둘의 작업, 11번에 단계 다섯) → Q1 추천을 둘로 자르기로
- **손으로 쓴 `inUse`는 살아 있는 파일을 지운다**(fable) → Q4를 태그 `media`와 생성된 쿼리로. 경쟁과 LIST 페이지(codex) → 여유 시간과 시험
- **계정 넘겨받기**(codex: 비밀번호 계정은 명시적 재인증이나 설정의 연결만, 한 트랜잭션, 정규화 이메일 / fable: 탈퇴 계정이 Google로 살아남, 콜백은 쓰는 뷰라 Tx 없음 → `WithTx`) → Q6 추천을 "자동 연결 없음"으로 바꾸고 사람에게 묻는다
- **서명 검증 생략의 조건**(둘 다), state·nonce·verifier의 쿠키와 재사용(codex), 통합 시험(codex) → Q5와 Q1 (b)의 확인
- **서명된 URL이 D-140과 부딪히고 로컬에 없다**(fable), `MediaStore`에 크기·시각·Range·URL이 없다(둘 다), S3 PUT의 길이(fable) → Q3. 로컬은 단일 인스턴스(codex)
- **5번과 6번의 차례**(fable: 공개 이름을 두 번 바꾼다) → 경계를 먼저
- **썸네일은 D-72**(fable) → 이름 → 열쇠 행의 내보내기 파일. outbox Kind 여럿(fable) → 8번에. 긴 일과 lease(fable) → 8번
- **헛것이 되는 확인**(fable): 같은 저자의 가짜 S3(서명을 다시 계산, CI의 MinIO), PG에서 이미 초록일 시각 시험(고치기 전 빨강, `Location()`), 로컬 하나로만 도는 미디어 시험(저장소 함수 하나를 둘에), 양성 하나뿐인 Q8(음성 둘), 기록뿐인 시험 시간(줄 수를 센다), 읽지 않는 allauth SQL(문서에서 뺐다: Google이 M13에 들면 `TestDocExamples`가 돌린다). Q8의 진단 누락(codex) → 위치까지
- **사실 고침**: "9단계"는 `stages`의 6과 생성·타입 검사 3(codex·fable), 시각의 Location은 "보고됨, 먼저 재현"(fable), 결정 번호 더함(fable: D-154·D-120·D-105·D-111·D-132·D-157·D-34·D-32·D-91·D-09·D-23, D-123의 셋째)
- **readmate에 비춰**(fable): JSON 필드의 `default`(`JSONField(default=dict)`) → "하지 않는 것"에 두고 `maago doc model`에 "앱이 Insert에서 채운다"(9번), 프로필 이미지·pbkdf2·이메일 인증은 이미 덮임
- 병렬 묶음(codex) → 작업 머리의 가~라
- 받지 않음: 없음

## M13 끝

- [ ] 1~12 완료
- [ ] M13 완료 조건을 실제로 돌려 확인, 조건마다 시험 이름
- [ ] MILESTONES.md의 M13 체크박스
- [ ] push 뒤 GitHub Actions
