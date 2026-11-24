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
   - 잰 것(2026-11-22, 로컬 M-시리즈 12코어, PG 55432): 걷기 한 번(`TestBoardTutorial`, 0001~0040과 ship) 82초, `cmd/maago`에서 Catches를 뺀 것 142초, Catches는 약 27분(이 문서 사실 절의 `cmd/maago` 약 29분에서 142초를 뺀 값. TODO 81줄의 약 27분은 M12 때 `cmd/maago` 전체). CI의 `cmd/maago`는 2492초(약 41.5분, 실행 44934297161). 까닭은 TODO 그대로: Catches의 34줄이 저마다 0001부터 걸어 줄 수 × 단계 수로 자란다
   - **같은 앞부분을 한 번 걷고 나눠 쓴다**(병렬이 아니라): Catches가 고치지 않은 단계를 0001부터 한 번 걷고, 단계 k를 적용하기 **전에** 그 단계를 고치는 줄마다 지금의 프로젝트를 복사해 서브시험에서 고친 k를 걷는다. 각 복사본은 고친 k의 오류를 보고 끝내며(`walkStep`은 오류에서 돌아온다, 지금도 k에서 실패하지 않으면 그 줄은 실패), ship에서 실패하는 꼬리 줄만 k 뒤 원래 단계와 ship까지 이어 걷는다. 걷기가 실패하는 자리와 메시지(`at+": "+c.by`)는 그대로이고, 줄마다 앞부분이 고치기 전과 같은 상태라 뜻이 같다. 가늠: 걷기 한 번 + 34줄 × 한 단계(몇 초) + ship까지 가는 넷의 꼬리(0033에서 둘, 0040에서 둘)로 약 5분(잰 값: Catches 229초, `cmd/maago` 446초). 단계를 더해도 단계 수 + 줄 수로 자란다
   - `cmd/maago/tutorial_test.go`만 바꾼다(새 파일 없음):
     - `walk`를 셋으로 가른다: 프로젝트를 만드는 부분(`git init`·`go.work`·`.env`·`noEnv`), 단계 하나(트레일러 읽기·`toPostgres`·`walkStep`, 오류에 패치 이름), 끝(`ship`, 오류에 마지막 패치 이름). 루트와 스키마를 담는 작은 구조체 `walker{root, schema string}`. `walk(t, dir)`은 그 셋을 차례로 부르는 지금의 함수로 남아 `TestBoardTutorial`·`TestBoardTutorialPostgresTrailer`는 그대로
     - `clone(t)`: 새 `t.TempDir()`(go.work 때문에 `EvalSymlinks`)에 `os.CopyFS`로 루트를 통째로(`.git`, SQLite의 `board.db`, `.env`). 스키마가 있으면(0013 뒤) 서브시험의 이름으로 `toPostgres(t, root, "", 1)`가 빈 새 스키마와 `.env`를 만든다(PG에는 스키마 복사가 없다. 단계가 공유하는 DB에는 `maago migrate`가 만든 것뿐이라 줄의 단계가 스스로 Migrate하면 같은 상태가 된다: gen·check는 연습 DB만 쓰고, 통과 단계는 `walkStep`이 앱 시험 앞에서 Migrate하고, Expect 단계는 DB를 보기 전에 돌아오고, 꼬리의 ship은 스키마를 비운다. 구현 리뷰 fable 1로 `build.Migrate`를 뺐다). `clone`은 서브시험의 `t`만 받는다(스키마 이름이 `t.Name()`에서 나와, 부모의 `t`면 부모의 스키마를 지운다). 부모의 스키마는 건드리지 않는다
     - `TestBoardTutorialCatches`: 줄의 표(리터럴)는 한 글자도 바꾸지 않는다. 고친 패치는 서브시험의 TempDir에 같은 이름으로 쓴다(오류의 패치 이름과 `git apply`). 부모가 단계마다 그 단계를 고치는 줄을 `t.Run`으로 돌린 뒤 원래 단계를 적용한다. 걷기 전에 줄마다 패치 이름이 `steps/`에 있는지 보고 없으면 실패(없는 패치 이름을 적은 줄이 조용히 빠지지 않게: 지금은 `readFile`이 실패시키던 것. PG 없는 로컬에서도 돈다, 구현 리뷰 fable 2)
     - 주석: Catches 위에 "앞부분은 한 번 걷고 줄마다 복사한다"
   - 단계와 확인
     1. 고치기 전 줄 이름 34개를 표에서 `grep`해 정렬해 둔다 → 확인: 고친 뒤 `MAAGO_TEST_PG_URL=… go test -run TestBoardTutorialCatches -v ./cmd/maago`의 `--- PASS` 서브시험 이름을 정렬해 그것과 `diff`가 비어 있다, `git diff`에 표의 줄이 없다
     2. 헛것 여섯 → 확인: 각각 실패하고 되돌린다. (a) 0013 앞 줄 하나("query")의 `new`를 `old`로 (b) 0013 뒤, 복사본이 부모보다 마이그레이션 하나를 먼저 가진 줄("no permission table", 0021) (c) 꼬리 줄 하나("no worker"): (a)(b)(c)는 실패 메시지가 `walk: <nil>`인 것을 본다(고치지 않은 복사본이 k를, 꼬리는 끝의 ship까지 걷는다는 뜻. 다른 오류면 복사가 틀린 것) (d) 한 줄의 패치 이름을 없는 이름으로(걷기 전의 검사) (e) 한 줄의 `by`를 다른 접두로("author check"의 `go test: ` → `maago gen:\n`) (f) 꼬리의 `at`이 마지막 패치인지: `by`를 바꾸면 꼬리가 아니게 되어(접두가 꼬리를 정한다) `walk: <nil>`로 끝나므로, 대신 시험 코드의 `at = last`를 빼면 "no migrations at the start"(0033)가 실패. 복사를 단계 **뒤**에 하는 실수는 고친 패치의 `git apply`가 실패해 모든 줄의 접두가 틀리므로 따로 잡을 것이 없다
     3. PG 없이 로컬 → 확인: 0012까지의 줄은 돌고 그 뒤는 부모가 0013에서 skip(지금도 그 줄들은 skip), `CI=1`이면 0013에서 실패(D-91, `TestBoardTutorialPostgresTrailer` 그대로)
     4. 시간 → 확인: 로컬 Catches **8분** 아래, 로컬 `cmd/maago` **12분** 아래(PG와 함께, 로그의 `go test exit=` 줄). CI의 `cmd/maago` **20분** 아래는 push한 뒤 GitHub Actions에서(push는 묻고 하므로 M13 끝의 "push 뒤 GitHub Actions" 항목에 적어 두고 거기서 잰다). 기준은 가늠(5분)에 넉넉한 위 한계라 먼저 재지 않고, 잰 값을 TODO에 적는다
     5. 전체 → 확인: `go mod tidy -diff && go vet ./... && go test -timeout 60m ./...`(PG와 함께)와 `gofmt -l .` 비어 있음, TODO 81줄에 잰 로컬 시간을 적는다(체크는 CI를 잰 뒤에)
   - ★ → 에이전트 편의로 결정: **병렬 대신 나눠 쓰기.** 에이전트가 단계를 더할 때마다 Catches가 줄 × 단계로 자라는 것을 막는 쪽은 나눠 쓰기뿐이다(병렬은 코어 수로 나눌 뿐이고, 걷기의 `go test ./...`가 이미 코어를 쓴다: 걷기 한 번 192% CPU). 병렬에는 걸림도 둘: `walk`의 `noEnv`가 `t.Setenv`라 병렬 서브시험에서 panic, 그리고 0013 뒤 걷기들의 앱 시험이 같은 `maagotest_*` 스키마(패키지 경로·시험 이름·차례의 해시라 걷기마다 같다)를 서로 지운다
   - ★ → 에이전트 편의로 결정: 줄 수는 고정 숫자(34)가 아니라 줄마다 패치가 있는지로 본다(줄은 제 패치에서 반드시 돈다). 8번이 줄을 더할 때 숫자를 따라 고칠 일이 없고, 줄이 조용히 빠지는 길(없는 패치 이름)은 막힌다. 표가 그대로인 것은 이 작업의 `git diff`가 보인다
   - ★ → 에이전트 편의로 결정: PG 줄의 복사는 빈 새 스키마(위). `pg_dump` 따위 새 외부 도구를 걷기에 들이지 않는다
   - 결정 기록 없음: 시험 안의 구조이고 D-103(패치를 하나씩 적용하며 gen·check·migrate·앱 시험)의 뜻은 그대로
   - 하지 않는 것: 서브시험 병렬(위), `TestBoardTutorial`의 걷기를 Catches와 나눠 쓰기(82초를 아끼지만 두 시험의 실패가 엉킨다), CI의 `-timeout 60m`을 되돌리기(M12 12번 사람의 결정, CI 시간을 재기 전에는 근거가 없다), `TestShortTutorial`(이 작업의 범위 밖)
   - 받지 않음: 줄 이름의 골든 상수(codex): 표를 한 번 더 적는 것이라 8번이 줄을 더할 때 두 곳을 고친다. 이름이 같은지는 단계 1이 이 작업에서 한 번 `diff`로 보고, 그 뒤에는 표가 이름의 출처이며 돈 줄 = 표의 줄을 시험이 센다. 기준을 먼저 재기(codex): 8·12·20분은 가늠의 두 배 넘는 위 한계라 잰 뒤 정해도 바뀌지 않는다, 잰 값은 단계 4에서 적는다
   - 계획 리뷰 codex·fable 반영: 복사본은 고친 k에서 끝내고 꼬리 줄만 ship까지(codex 1), 이름은 정렬해 `diff`(codex 2의 반), 헛것 (a)(b)(c)는 `walk: <nil>`을 보고 (b)를 0021로, `by`를 바꾸는 헛것과 `at`을 지키는 헛것(fable 1·4·5, 꼬리의 `by`를 바꾸는 꼴은 꼬리가 아니게 되어 `at = last`를 빼는 꼴로), CI 20분을 M13 끝 항목에 걸고 TODO 체크는 그 뒤(fable 2), 36 → 34(fable 3), 29분의 출처와 해시의 재료(fable 6), `clone`은 서브시험의 `t`만(fable 7)
   - 받지 않음(구현 리뷰): `(*walker).ship`을 지우고 두 곳에서 직접 감싸기(ponytail nit 2: nil 가드가 하나 더 들어 지금 꼴이 짧다)
   기록(2026-11-22): `cmd/maago/tutorial_test.go`의 `walker`(`newWalker`·`step`·`ship`·`clone`)와 `walk`, Catches는 앞부분을 한 번 걷고 단계 k 앞에서 그 단계를 고치는 줄마다 복사해 고친 k를 걷는다(꼬리 줄만 뒤 단계와 ship까지), 줄의 표는 그대로(`for … range` → `rows :=` 한 줄). 시험: 서브시험 이름 34개가 고치기 전과 같다(정렬 `diff`), 헛것 여섯이 모두 실패(fable이 따로 다시 돌려 확인, `at = last`를 빼면 0033의 꼬리 둘), PG 없이 0012까지 돌고 0013에서 skip·`CI=1`이면 실패, `GOTOOLCHAIN=go1.25.14`로도 통과(fable). 로컬 Catches 229초(기준 8분), `cmd/maago` 369초(기준 12분, 리뷰 반영 뒤 PG 전체 시험 `go test exit=0`, 따로 만든 DB). CI의 `cmd/maago` 20분 아래는 M13 끝의 push 뒤에 잰다. 구현 리뷰는 ponytail·codex·fable: 막는 것 없음. `clone`의 `build.Migrate`는 지키는 줄이 없어 뺐다(fable 1, 줄의 단계가 스스로 Migrate), 패치 이름 검사를 걷기 전으로 옮겨 PG 없는 로컬에서도 돌게 하고 `ran` 세기를 지웠다(fable 2, 없는 이름이 PG 없이 실패함을 확인), 꼬리 루프를 `if tail`로 묶었다(ponytail nit 1), CI 시간은 M13 끝에서(fable 3). codex는 지적 없음(쓰기 금지 환경이라 실행은 못 함). 두 리뷰어의 첫 실행이 같은 PG에서 겹쳐 실패한 것은 TODO의 스키마 겹침 항목 그대로
2. **조용한 런타임 묶음**(Q9): `InsertX`·`UpdateX`·`GetX`의 시각을 두 방언 모두 UTC로(보고된 것, 먼저 재현), API 뷰의 unique 위반 422, admin의 거절된 삭제를 4xx와 이유로·잘못된 POST의 이유, 사용자 폼의 `default=''` → 확인: 고치기 전 SQLite에서 빨강인 시각 시험과 PG의 `Location() == time.UTC`, API unique 위반의 422(필드 있음·없음), `contrib/admin`의 거절 상태, `internal/web/forms_test.go`의 빈 입력. D-105·D-111·D-154·D-120·D-132 고침
   계획
   - **따라가 본 것**: (시각) SQLite는 생성된 scan이 `rt.ScanSQLiteTime`으로 `time.Parse` 뒤 `.UTC()`(`rt/sqlitetime.go`), PG는 `g.dest`가 맨 `&x`라 pgx가 준 값 그대로인데 그것이 Local이다(:55432에서 `SELECT now()`를 읽어 `Location()`이 `Local`, `== time.UTC`가 거짓을 실측). `InsertX`·`UpdateX`의 auto 시각은 `time.Now().Truncate(µs)`라 Local(`models.go.tmpl`). 모든 scan(모델 `scanX`, 쿼리 행, 스칼라 결과)은 `internal/mapper/gen.go`의 `dest` 한 함수를 지나고(199·340·353줄), admin도 생성된 `scanX`로 읽는다. 런타임은 드라이버를 import하지 않으므로(D-09) pgx의 코덱을 바꾸는 길은 없다. (API unique) API 라우트의 어댑터는 `WithTx` 뒤(또는 Tx 없는 길의 뷰 뒤) 뷰의 오류를 그대로 돌려주고, `rtcore.Code`가 `*UniqueError`를 몰라 500. 어댑터는 In의 폼(`in.F`, 뷰가 받은 것은 값 복사)과 생성된 `Fail`·`invalidX`를 이미 갖고 있다. (admin) `delete`의 POST가 막히면 같은 `delete.html`을 200으로(`site.go:494`), 이유(참조·없는 행)는 이미 그 페이지에 있다. 잘못된 POST의 `maago.Status(400, …)`는 런타임의 `fail`이 HTML 라우트에 `http.Error(StatusText)`만 써서 이유가 사라진다(`http.go` `fail`). 409를 쓰는 `conflicted` 쓰개가 이미 있다. (`default=''`) admin은 `optional(f)`가 `Pointer || String && Default == ""`(`admingen/form.go:293`), `web`의 `modelRules`는 포인터만 본다(`forms.go:443`)
   - **1. 시각을 UTC로**(먼저 재현)
     - 시험 먼저: `cmd/maago/internal/gentest/{sqlite,postgres}/orders/orders_test.go`(두 사본이 같은 글)에 `TestTimesUTC`: `InsertOrder`(입력 `CreatedAt`은 `t0`를 KST 위치로 바꾼 것) → `GetOrder` → `UpdateOrder` → `GetOrder` → 시각을 돌려주는 쿼리 하나(행과 스칼라). 확인은 시각 필드마다 `Location() == time.UTC`와 `==`(Insert의 `UpdatedAt` == Get의 것, Update 뒤의 `o.UpdatedAt` == 다시 읽은 것). 입력 시각은 준 그대로라 Insert가 돌려준 `CreatedAt`은 입력과 `==`, Get의 것은 같은 순간을 `Equal`과 `Location() == time.UTC`로(이 짝만 `Equal`). 그 밖에 `Equal`·`show()`(형식을 낸 글)는 쓰지 않는다: 버그를 가린다. 포인터 시각은 policy의 `PublishAt`(`policy_test.go`의 `TestNullTimeUTC`, 두 사본): nil과 KST 값을 Insert → Get해 nil은 nil, 값은 `Location() == time.UTC`(PG의 `ScanNullTime` 갈래). 구조체 `==`는 포인터 필드(`CustomerID`·`Priority`) 때문에 늘 거짓이라 시각 필드만 → 확인: 고치기 전 SQLite에서 빨강(Insert의 `now`가 Local), PG에서 빨강(Get이 Local)
     - `rt/sqlitetime.go`: `ScanSQLiteTime`·`ScanNullSQLiteTime`을 `ScanTime`·`ScanNullTime`으로 이름을 바꾸고 `time.Time`(PG 드라이버가 주는 것)도 받아 `.UTC()`. `internal/mapper/gen.go` `dest`가 시각이면 두 방언 모두 그것을 쓴다. 쓰는 쪽(`arg`, `SQLiteTime`)은 그대로(PG는 순간을 보낸다)
     - `models.go.tmpl`: `now := time.Now().UTC().Truncate(time.Microsecond)`(Insert·Update 둘 다), Insert의 Go 문서에 "the auto times, and every time GetX or a query reads, are UTC"
     - `cmd/maago/runtime_test.go` `TestSQLiteTimes`의 이름을 따라 고치고, PG 드라이버의 `time.Time`을 받는 줄을 `TestPostgresTime`에(`rt.ScanTime`·`rt.ScanNullTime`으로 값과 NULL을 읽어 `Location() == time.UTC`). `rt/runtime_test.go`의 `TestScanSQLiteTime`도 `TestScanTime`으로, src가 Local·KST인 `time.Time`인 줄(`ScanTime`·`ScanNullTime`, PG 없이 돈다)을 더한다. `int64(5)`의 거절은 그대로. `docs/ARCHITECTURE.md`의 `rt` 줄도 새 이름으로. 골든(`gentest`의 두 방언, `internal/mapper`·`internal/web`의 것)은 `MAAGO_TEST_PG_URL`과 함께 `-update`
     - → 확인: `TestTimesUTC`·`TestNullTimeUTC` 두 방언 초록, `TestSQLiteTimes`·`TestPostgresTime`, `TestGenerated`
   - **2. API 뷰의 unique 위반 422**
     - `internal/web/templates/views.go.tmpl`: API 라우트(쓰는 뷰·탈출구 뷰 밖)의 뷰 오류를 Tx 길(`txErr`)과 Tx 없는 길 둘 다에서 돌린다. In이 폼 F(타입 X)를 읽으면 `if rt.Code(err) == http.StatusInternalServerError && in.F.Fail(err) { return invalidX(&in.F) }`, 그다음 `return rt.Taken(r.Context(), err)`(두 출구가 같은 템플릿 조각). 뷰가 준 상태가 이긴다: 상태가 500일 때만 바꾼다(`maago.Status(409, err)`는 UniqueError를 감싸도 409). `invalidX`는 이미 API 폼마다 생성된다(`forms.go`의 `api`). 어댑터의 `in.F`는 뷰 앞에서 `Valid()`였으므로 422에는 unique 오류만 실린다
     - `rt`: `Code(err) int`(`rtcore.Code` 그대로, admin도 쓴다)와 `Taken(ctx, err) error`: `Code(err)`가 500이고 나무에 `*maago.UniqueError`가 있으면 `rtcore.Invalid(FieldError{Detail: SettingsFrom(ctx).Msgs.Taken})`, 아니면 err 그대로
     - 422의 본문: `Fail`이 한 칸이면 `{"field":"name","detail":Taken}`, 칸이 둘 이상이거나 `Fields`가 nil이면 `{"detail":Taken}`. `fail`의 `p.Detail = err.Error()`에 빠지지 않는다(422는 `FieldErrors`가 있어 `errors`만): 드라이버 글과 "show it on the form with its Fail"이 클라이언트에 가지 않는다
     - 시험: site fixture(`cmd/maago/testdata/web/site`)에 API 라우트 둘: `POST /api/tags` `api_tag_save`(`TagIn`을 JSON으로, `maago.Tx`: Tx 길, 뷰는 `InsertTag`의 오류를 그대로 돌려준다)와 `POST /api/tags/{name}` `api_tag_name`(경로 값만, 폼 없음, `maago.NoTx`와 `maago.DB`: Tx 없는 길). 두 뷰 모두 이름이 `409`이면 오류를 `maago.Status(409, err)`로 감싸 돌려준다. `TestFormTaken`(`Messages.Taken`이 "이미 있습니다.")에 줄: 이름이 겹침 → 422 `"errors":[{"field":"name","detail":"이미 있습니다."}]`, kind·code가 겹침 → 422 `"errors":[{"detail":"이미 있습니다."}]`, 폼 없는 라우트의 겹침 → 같은 꼴, 두 라우트에서 `409`의 겹침 → 409(뷰가 준 상태), 모두 `application/problem+json`이고 본문에 `UNIQUE`·`duplicate`·`Fail`이 없다. `rt/write_test.go` `TestFail`의 `Fields`가 nil인 줄(폼 오류로)은 이미 있다(`no constraint known`), 곁에 `TestTaken`(UniqueError, 감싼 UniqueError, `Status(409)` 안의 것, 다른 오류, nil). `permissions_test.go`에 두 행
     - 문서: `query.go`의 `UniqueError` 주석("else it answers 500" → "On an API route maago answers it with a 422 itself; on another route a view that does not call Fail answers 500"), `internal/ref/topics.go` 폼 절의 끝 문장(지금 "errors.As로 받아 Invalid를") → "An API view returns the error: maago answers a 422, on the field as Fail would put it", 뷰 절(309줄)의 상태 목록에 "a generated Insert's or Update's unique refusal on an API route a 422"(손 `:exec`의 위반은 드라이버 오류라 500 그대로)
     - → 확인: `TestWeb`(site의 `TestFormTaken`·`TestAPIForm`·`TestPermissions`), `rt` 시험, HTML 라우트 `tag_save`는 그대로(`TestFormTaken`의 기존 줄)
   - **3. admin의 거절된 삭제와 잘못된 POST**
     - `contrib/admin/site.go` `delete`: POST가 막히면(`!ok`) `w = &conflicted{ResponseWriter: w}`로 같은 페이지를 409. GET의 확인 페이지는 200 그대로
     - `App`의 `route` 도우미가 serve를 감싼다: 오류의 상태가 400이면 `http.Error(w, "Bad Request: "+err.Error(), 400)`로 이유를 쓰고 nil. 403·404·409·413·5xx는 런타임 그대로(404의 맨 `sql.ErrNoRows` 글을 보이지 않는다, TODO가 바라는 것도 잘못된 POST의 400). 상태는 `rt.Code(err)`(새 한 줄, `rtcore.Code`를 그대로: contrib는 `maago`와 `rt`만 쓰는 관례). admin의 400 글은 admin이 만든 것(`admin: id "x": …`, `_version`)과 런타임의 본문 해석(`form key "title" is given 2 times`)이다
     - 시험: `contrib/admin/delete_test.go` 90·93줄을 409로(본문에 "refers to it"·"no such row"), 116·117·138줄의 400 본문에 이유(`no row to delete`·`id "x"`·`rows to delete`), `form_test.go` 166줄의 400 본문에 `given 2 times`, 119줄의 500(`?id=9`) 본문에 오류 글이 없다(감싸기가 5xx 글을 흘리는 회귀)
     - 게시판: 0025의 `board/admin_test.go`(211줄 `code != 200`)를 409로, 0036이 그 줄을 문맥으로 가지므로 함께. `steps.sh unpack` → rebase → `pack`. 0025 글의 "the two cases of deleting"에 "the refused one is a 409" 한 마디
     - `internal/ref/topics.go` admin 절에 "a delete that cannot be is a 409, and a bad request says why"
     - → 확인: `contrib/admin` 시험, `TestBoardTutorial`(PG)·`TestBoardMap`·`TestBoardTutorialCatches`
   - **4. 사용자 모델 폼의 `default=''`**
     - `internal/web/forms.go` `modelRules`: `ff.optional = ff.optional || mf.Pointer && ff.typ != "bool" || mf.Type == model.String && mf.Default != nil && *mf.Default == ""`(admin의 `optional`과 같은 뜻, D-132)
     - 시험: `internal/web/forms_test.go` `TestModelFormDefaultEmpty`: 모델에 `Note string \`maago:"max=200,default=''"\``, 폼에 태그 없는 `Note` → 생성된 규칙에 `Optional: true`. 음성: `Kind`(`default='free'`)를 태그 없이 두면 `Optional`이 없다(빈 입력은 여전히 필수)
     - `maago doc form`의 모델 폼 문단에 "a string field whose default is '' takes an empty input"
     - → 확인: `go test ./internal/web/`
   - **5. 결정·TODO·문서**: 결정 고침은 이 작업에서(9번은 미디어·검사의 것): D-105(읽는 시각은 두 방언 모두 UTC), D-111(auto 시각은 UTC, 입력 시각은 준 그대로), D-120·D-154(API 라우트의 unique 위반은 생성 어댑터가 422), D-132(사용자 모델 폼도 `default=''`, 막힌 삭제 409, 400의 이유, admin의 409는 버전 충돌과 막힌 삭제 둘이고 페이지가 까닭을 보인다). TODO 43·56·58·63줄을 체크하고 이 작업을 가리킨다. `maago doc model`에 "times read back are UTC; date shows them in Config.TimeZone" → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
   - **전체 확인**: `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .` 비어 있음, PG와 함께 전체(`-timeout 60m`, 로그의 `go test exit=$?` 줄로 판단)
   - ★ → 에이전트 편의로 결정: **입력 시각은 준 그대로**. Insert가 돌려주는 모델의 auto 시각과 모든 읽기를 UTC로 맞추고, 앱이 준 `CreatedAt` 따위는 바꾸지 않는다. D-111이 "입력을 그대로 담는다(포인터도)"이고, 바꾸려면 포인터마다 새 값을 만들어야 한다. 폼이 준 시각(`Config.TimeZone`)과 읽은 시각은 `Equal`로 비교하라고 Insert의 문서와 `maago doc model`에 쓴다. 보고된 것(auto 시각 Local과 읽기 UTC가 `==`에서 틀림)은 이것으로 풀린다
   - ★ → 에이전트 편의로 결정: **`rt`의 scan 이름을 하나로**(`ScanTime`·`ScanNullTime`). PG에 `ScanSQLiteTime`이라는 이름은 틀린 글이고, `rt`는 "API changes with maago gen"이라 옛 생성 파일은 `maago gen`이 다시 쓴다. SQLite 골든도 이름만 바뀐다
   - ★ → 에이전트 편의로 결정: **폼 없는 API 라우트도 422**(`rt.Taken`). Q9는 폼의 꼴만 말하지만, 경로 값만 받는 API 쓰기(좋아요·가입 같은 unique (user, x))가 가장 흔하게 500을 맞는다. "API 라우트에서 생성된 Insert·Update의 unique 위반은 422" 한 규칙이 폼 유무로 갈리지 않는 편이 에이전트가 외울 것이 적다. 뷰가 먼저 `maago.Status`나 `maago.Invalid`로 바꿔 돌려주면 그것이 이긴다(상태가 500일 때만 바꾼다)
   - ★ → 에이전트 편의로 결정: **막힌 삭제는 없는 행도 409**. 한 POST에 참조 있는 행과 없는 행이 섞일 수 있고, 페이지가 행마다 까닭을 보인다. 상태 하나("지우지 못했다, 페이지를 보라")가 갈래보다 단순하다. 모델 자체가 없을 때의 404는 그대로
   - ★ → 에이전트 편의로 결정: **400의 이유는 admin에서만**(`route` 감싸기). 런타임의 HTML 라우트 전부가 4xx 글을 보이게 하면 `sql.ErrNoRows`의 404가 "sql: no rows in result set"을 사용자에게 보이는 따위로 앱마다 뜻이 바뀐다. admin의 400 글은 admin과 본문 해석이 쓴 것이고, 요청을 되풀이할 뿐이라 새지 않는다(admin 라우트는 로그인만 요구하고 staff는 `Can`이 본다)
   - 하지 않는 것: 쓰는 뷰(D-157)와 탈출구 뷰의 unique 위반(응답을 뷰가 쓴다: 지금처럼 쓰기 전이면 500), HTML 라우트의 unique 위반(Q9: `Fail`을 안 부르면 500), 입력 시각의 UTC 바꿈(위 ★), pgx 코덱 설정(런타임이 드라이버를 import하지 않는다), admin의 오류를 HTML 페이지로 그리기(평문 이유로 충분), JSON 필드 안의 시각, admingen 폼의 `now`(SQL 인자로만 가는 순간이라 위치가 보이지 않는다)
   - 겹침: 1번(Catches 병렬)과 3번(DB 단계) worktree가 `cmd/maago` 시험과 골든을 건드린다. 이 작업의 게시판 변경은 0025·0036 두 패치뿐이라, 합칠 때 Catches 행이 그 줄을 쓰는지 다시 본다
   - 계획 리뷰 codex·fable 반영: 입력 시각 보존(KST 입력의 `==`·`Equal`, codex), 포인터 시각의 PG 길(policy `PublishAt`과 `TestPostgresTime`의 NULL, 둘 다), `rt`의 `time.Time` src 단위 줄과 시험 이름(fable N3), 두 fixture 라우트의 Tx·NoTx(둘 다), 뷰가 준 상태가 이긴다(500일 때만, 409 시험 줄, fable B1), admin 감싸기는 400만과 500 본문 확인(fable N1), "생성된 Insert·Update의"로 좁힌 글(fable N5), D-132의 409 두 뜻(fable N6), admingen `now`는 하지 않는 것에(fable N7). 받지 않음: 없음
   - 구현 리뷰 반영: GET 확인 페이지의 200(fable 1, `delete_test.go`)과 403·404 본문을 `Forbidden`·`Not Found`로 정확히(fable 2: 감싸기를 모든 4xx로 넓히면 실패를 확인), `maago doc model`의 긴 줄을 접음(fable 4). 받지 않음: 네 길 중 둘(Tx+폼 없음, NoTx+폼)의 동작 시험(fable 3: 폼 조각과 폼 없는 조각은 두 길에서 같은 글이고, 각 조각과 각 길이 동작 시험에 한 번씩 든다, 나머지 둘은 골든과 `TestViewSignature`·`TestFormsGen`이 지킨다), 그 오류 덩이를 `define` 하나로(ponytail 1: 라우트와 변수 이름 둘을 넘길 템플릿 함수가 없어 여섯 줄 겹침보다 커진다), `rt/sqlitetime.go`의 이름(ponytail 2: 이름만), admin 삭제의 경합(codex: 이 작업 전부터 있던 것이고 데이터는 안전하다, TODO M13)
   기록(2026-11-22): 시각은 두 방언 모두 UTC(`rt.ScanTime`·`ScanNullTime`을 `dest` 한 곳에서, auto 시각은 `time.Now().UTC()`, 준 시각은 그대로), API 라우트에서 생성된 Insert·Update의 unique 위반은 생성 어댑터의 422(폼이면 `Fail`의 자리, 없으면 `rt.Taken`, 뷰가 준 상태가 이긴다), admin의 막힌 삭제 409와 400의 이유(`route` 감싸기, 400만), 사용자 모델 폼의 `default=''`, 게시판 0025·0036, 결정 고침 D-105·D-111·D-120·D-154·D-132. 시험: `TestTimesUTC`·`TestNullTimeUTC`(두 방언, 고치기 전 SQLite·PG 빨강), `TestScanTime`·`TestSQLiteTimes`·`TestPostgresTime`, site `TestFormTaken`(422 필드 있음·없음·폼 없는 라우트, 뷰의 409), `TestTaken`, `contrib/admin`의 `TestDelete`(409, 400의 이유, GET 200, 403·404·500 본문), `TestModelFormDefaultEmpty`, `TestBoardTutorial`·`TestBoardMap`. PG 전체 시험 초록. 구현 리뷰는 ponytail·codex·fable: 막는 것 없음, fable의 시험 빈틈 둘(GET 200, 403·404 본문)과 긴 줄을 고침, 네 길 중 둘의 동작 시험·템플릿 `define`·파일 이름은 받지 않음(까닭은 위), codex의 삭제 경합은 TODO 메인이 다시 돌린 PG 전체 시험(데이터베이스 `m13_2`)에서 `TestAddAuth`가 빨강: auth 복사본의 admin 시험(`cmd/maago/add_test.go`)이 세션 있는 사용자의 거절된 삭제에 옛 200을 기대 → 409로, `TestAddAuth`·`TestAddOutbox` 두 방언 초록(나머지 패키지는 그 실행에서 초록)
3. **DB 단계의 쿼리 계속**(Q8) → 확인: 두 방언에서 모델에 열을 더하고 makemigrations 없이 gen하면 "run makemigrations"와 쿼리의 "missing" 진단이 모두 위치와 함께 한 실행에(모든 쿼리를 본다), 새 열을 맞게 쓴 쿼리는 "run makemigrations" 하나뿐, 손 마이그레이션에만 있는 것을 쓰는 쿼리는 이 길에서 오류가 없다. 게시판 0012·0039 글의 "먼저 makemigrations"를 다시 봄

   계획

   따라가 본 길: `checkDatabase`(`internal/build/build.go:315`)가 `compared`로 마이그레이션의 연습 DB `a`와 모델 DDL의 스키마를 얻고, `migrate.Diagnose`가 차이를 내면 거기서 돌려준다(`build.go:326`). 그때 `checkQueries`(사용자 쿼리의 `mapper.Describe`와 `mapper.Results`, 정책 probe, CRUD·admin의 prepare)는 돌지 않는다. makemigrations(`draftDatabase`, `makemigrations.go:68`)는 같은 `a`에 초안을 `migrate.ApplyDraft`로 적용하고, 다시 읽어 차이가 남지 않음을 확인한 뒤 파일을 쓴다. 0012의 왕복에서 둘째 오류는 prepare가 아니라 `Results`의 `missing`이다(`result.go:170`): 결과가 `Post`인데 새 열을 고르지 않았다.

   - ★ → 에이전트 편의로 결정: **쿼리를 "마이그레이션 + makemigrations의 초안"에 prepare한다. 모델 DDL만의 연습 DB에는 하지 않는다.** 차이가 나면 이미 열린 `a`에 `dialects[d].draft(apps, cs)`를 적용하고, 다시 읽어 차이가 없을 때만 `checkQueries`의 사용자 쿼리 부분을 돌린다. 이 DB는 다음 실행이 실제로 보는 스키마다. 모델과 같은 스키마에 손 마이그레이션에만 있는 것이 더해진 것이다. 그래서 다음 경우가 꾸밈 없이 갈린다:
     - 새 열을 맞게 쓴 쿼리 → 통과("run makemigrations" 하나뿐)
     - 오타·모델 결과의 `missing` → 오류
     - 모델에서 뺐는데 쿼리가 아직 쓰는 열 → 오류(`TestQueriesDroppedField`가 makemigrations 뒤에야 보던 것)
     - 손 마이그레이션에만 있는 것 → 통과

     모델 DDL만의 DB에 prepare하면 넷째가 거짓 오류다. "a에서도 실패할 때만 낸다"는 꼴은 셋째를 놓친다. 오류 글을 차이와 맞춰 보는 꼴은 방언마다 깨지기 쉽다. D-162의 "모델 DDL의 연습 DB에"라는 글을 이 꼴로 고친다(이 작업에서, D-162는 이 작업의 결정이다). 사람에게 묻지 않는 까닭: Q8은 사람이 아니라 에이전트 편의로 정했고 꼴을 "(계획에서 꼴)"로 이 계획에 맡겼으며, D-162의 두 목표(한 실행, 손 마이그레이션에 거짓 오류 없음)는 그대로다. 바뀌는 것은 "어느 DB에"라는 수단의 글뿐이다.
   - **손 마이그레이션에만 있는 것이 실제로 무엇인가.** `sqlite.Read`는 view·trigger·virtual table을, `postgres.Read`는 extension·view·sequence·function·type·rule·trigger를 "not supported"로 거절한다. 남는 표·열·인덱스·FK·CHECK는 `Diff`가 차이로 낸다. `Diff`가 보지 않는 것 중 이름은 prepare에 닿는다(`indexKey`는 이름을 보지 않는다, `diff.go:168`): SQLite의 `INDEXED BY 이름`, PG의 `ON CONFLICT ON CONSTRAINT 이름`. 그래서 SQLite에도 있다. 초안이 표를 재구성해 인덱스 이름을 모델의 것으로 바꾸면 그 오류는 makemigrations 뒤에도 같은 오류라 거짓이 아니다. PostgreSQL에는 이름 말고도 `Read`가 묻지 않는 catalog가 남는다: text search configuration, collation, operator 따위. `to_tsvector('kor', …)`는 prepare에서 이름을 푼다: 연습 스키마에 `kor`가 없으면 `text search configuration "kor" does not exist`(psql로 확인함, 2026-11-22). 음성 시험은 SQLite에서 손으로 이름 지은 인덱스와 `INDEXED BY`(재구성 없는 ADD COLUMN 차이로), PG에서 `kor`로 한다.
   - **초안이 적용되지 않으면 지금처럼 "run makemigrations"만 낸다.** 데이터 마이그레이션이 넣은 행 위에 기본값 없는 NOT NULL 열을 더하는 경우가 그렇다. 이때는 makemigrations도 "write this migration by hand"로 멈춘다. 초안이 비었거나, 적용 뒤 다시 읽어 차이가 남는 경우도 같다. 쿼리 오류는 나중 실행으로 미뤄질 뿐 거짓으로 나오지 않는다. 초안의 적용이 실패하거나 차이가 남으면 `a`를 다시 읽지 않는다: `checkDatabase`는 차이의 진단만 돌려주고 이미 있는 `defer a.Close()`가 `a`를 버린다(PG는 늘 롤백되는 트랜잭션, SQLite는 메모리 DB라 남는 것이 없다). 따로 연결을 여는 길은 받지 않는다(codex): 버리는 경계가 이미 `a`의 닫기이고, 연결을 하나 더 여는 것은 PG에서 연습 스키마를 하나 더 만들 뿐이다.

   바꿀 파일
   1. `internal/build/makemigrations.go`
      - 고치지 않는다(구현에서 바꿈): 둘이 같이 쓰는 것은 `ApplyDraft`와 `read()`의 두 호출뿐이고, makemigrations는 두 실패를 다른 글로 가르므로 뺀 함수가 둘을 다시 갈라 돌려줘야 한다. `againstModels`가 두 호출을 직접 한다.
      - → 확인: makemigrations의 시험이 그대로 초록이다(`TestMakeMigrations*`, `TestCheckDraft`, `draft_test.go`, 두 방언).
   2. `internal/build/build.go`
      - `checkDatabase`: `Diagnose`의 진단을 먼저 계산한다(`a`를 바꾸기 전). 차이가 있으면 `append(ds, againstModels(ctx, s, a, cs, diff)...)`를 돌려준다.
      - `againstModels`는 초안을 만들어 `dialect.Split`하고 1의 함수로 적용한다. 실패하거나 차이가 남으면 nil을 준다. 아니면 사용자 쿼리를 prepare해 그 진단에 "against the models"를 붙인다.
      - `checkQueries`를 둘로 나눈다. 사용자 쿼리·`Results`·정책 probe가 한쪽이고, CRUD·admin의 prepare(maago 버그의 안전띠, `checkGenerated`)가 다른 쪽이다(Q8: 그 prepare는 같을 때만). 같을 때의 길은 지금과 같다.
      - `checkDatabase`·`checkQueries`의 주석을 고친다.
      - → 확인: 아래 시험.
   3. `internal/diag/diag.go`: `See`처럼 생긴 `Note(suffix string, ds []Diagnostic) []Diagnostic`를 둔다. 필드를 내보내지 않으므로 글 끝에 붙이는 길이 따로 필요하다. 붙는 글은 ` (checked against the models)`이고, prepare 오류, `Results`의 `missing`, 정책 probe 모두에 붙는다. "run makemigrations"가 같은 출력에 있으니 무엇과 비교한 오류인지만 말하면 된다. fix는 쿼리의 fix 그대로다.
   4. 시험 `cmd/maago/queries_build_test.go`: `TestQueriesAfterModels`를 두 방언(`ordersProject(t, d)`)으로 넓힌다. 이름은 `TestQueriesBeforeMigration`으로 바꾸고 주석도 고친다. 기대는 모두 `diagnostics(text(…))`의 줄 전체를 `slices.Equal`로 본다(`expect`는 앞머리만 맞춰 접미사의 있고 없음을 못 본다). PG에서 글이 다른 곳만 앞머리와 `HasSuffix`로. 한 프로젝트에서 차례로:
      - (a) `Customer`에 필드 `Email string`을 더하고, `email`을 맞게 쓴 `Emails :many Customer`를 둔다 → 진단 줄이 정확히 하나: `models.go:17:2: migrations do not match models: column "email" of "customers" is missing`
      - (b) 같은 상태에 둘을 더한다. `emial` 오타 쿼리, 그리고 `SELECT id`인 `Customer` 결과(`missing "name", "email"`) → 한 실행에 세 줄(차이 하나와 두 쿼리). 쿼리 둘은 파일:줄:열과 ` (checked against the models)`까지 본다. 다른 쿼리(`search_orders` 따위)에는 진단이 없다.
      - (c) makemigrations를 하면 (b)의 두 쿼리 오류만 남고, 그때는 ` (checked against the models)`가 없다(같은 줄과 열).
      - 끝의 `bad.sql`(머리 오류) 확인은 그대로 둔다.
      - (d) 새 `ordersProject`에서 makemigrations 없이 `Priority`를 지운다 → 차이 줄과 `queries/order_by_amount.sql:…: … priority … (checked against the models)`가 한 실행에, 줄 전체를 방언마다 `slices.Equal`로. `TestQueriesDroppedField`(makemigrations 뒤의 길)는 그대로 둔다. (fable: 같은 프로젝트에서 (b) 뒤에 지우면 (b)의 쿼리와 allow-drop 없는 DROP이 섞인다.)
      - (e) 정책 probe의 접미사는 시험 없음: 정책은 사용자의 타입(사용자를 받는 뷰)을 요구하고 모든 쿼리를 규칙 6 아래로 옮겨야 해서 orders 위에서 길어진다. 접미사는 `checkQueries`의 결과 전체에 붙으므로 probe의 줄도 같은 길로 받는다.
      - 쿼리가 돌려주지 않는 모델(`Customer`)에 필드 `Email`을 더한다(구현에서 바꿈): `Order`에 더하면 열을 고르는 `order_by_amount.sql`이 맞게 `missing "note"`를 받아 "하나뿐"의 확인이 흐려진다.
      - → 확인: 이 시험을 구현 전에 돌려 빨강을 본다.
   5. 시험, 같은 파일:
      - `TestQueriesHandMigration`(두 방언): 손 마이그레이션에만 있는 이름. SQLite는 `0001_initial.sql`의 `customer_id` 인덱스 이름을 손으로 바꾸고 쿼리에 `INDEXED BY 그 이름`, PG는 `0002_kor.sql`에 `CREATE TEXT SEARCH CONFIGURATION kor (COPY = simple);`와 `to_tsvector('kor', status)`인 쿼리. 먼저 마이그레이션이 맞는 상태에서 gen이 통과함을 본다(오늘 `Read`가 받는다는 확인). 그다음 `Customer`에 nullable 필드를 더하고(초안이 `orders`를 건드리지 않아 손 이름이 그대로다) 오타 쿼리 하나를 같이 두면, 진단이 정확히 둘: 차이 줄과 오타 쿼리 + ` (checked against the models)`, 손 이름 쿼리의 줄은 없다(`slices.Equal`). 오타 줄이 있으니 쿼리를 실제로 본 갈래이고, 손 이름의 줄이 없으니 손 마이그레이션이 있는 DB에서 본 것이다: 모델 DDL만의 DB로 되돌아가는 회귀를 CI가 잡는다. codex의 "고를 수 있는 시험 seam"은 받지 않는다: 이 두 줄의 기대가 seam 없이 같은 것을 고정한다.
        → 확인: `againstModels`를 일부러 모델 DDL만의 연습 DB에 prepare하게 바꾸면 두 방언 모두 빨강임을 한 번 돌려 보고 되돌린다.
      - `TestQueriesDraftFails`(SQLite·PG): `0002_data.sql`이 `customers`에 행 하나를 넣는다(필드를 `Customer`에 더하므로). 기본값 없는 NOT NULL 필드를 더하고, 오타 쿼리 하나를 둔다 → gen의 진단이 차이 줄 하나뿐임을 `slices.Equal`로(되돌아가는 갈래를 고정한다). 같은 상태에서 makemigrations가 "maago did not write"로 멈추는 것도 본다(실패 갈래에 실제로 들어갔다는 증명). NOT NULL의 꼴은 구현에서 방언마다 실제로 실패하는 것으로 고른다(PG `ADD COLUMN … NOT NULL`, SQLite 재구성의 `INSERT … SELECT`). 두 가드(`ApplyDraft`의 실패, 다시 읽어 남은 차이)는 함께만 고정된다: 적용은 됐는데 차이가 남는 것은 maago 버그라 seam 없이 만들 수 없다.
   6. 문서
      - `internal/ref/topics.go:140`(`maago doc query`): "a practice database built from the migrations" 뒤에 한 문장. 마이그레이션이 아직 모델과 다르면 makemigrations의 초안을 더한 DB에 prepare하고, 그 오류는 "checked against the models"라고 말한다.
      - `docs/ARCHITECTURE.md` §3의 DB 검사 줄(119): "같으면 같은 DB에서" 뒤에 "다르면 초안을 더한 DB에서 사용자 쿼리만".
      - `docs/DECISIONS.md` D-162의 글.
      - D-23·D-34·D-32의 고침은 9번의 목록대로 9번에서 한다.
      - → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
   7. 게시판 0012·0039 글을 다시 봤다. 0012는 "makemigrations로 0004를 쓰고, gen은 recent_posts.sql에서 멈춘다"는 차례이고, 이 차례는 바뀐 뒤에도 맞다(먼저 gen하면 둘이 한 번에 나올 뿐이다). 0039에는 차례의 주장이 없다. 고치지 않는다. 모델과 마이그레이션이 다른 상태를 기대하는 `Maago-Expect`·Catches 줄도 없다(모든 단계 patch가 마이그레이션을 함께 가진다).
   8. 끝 확인: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .`. PG 전체 시험(`MAAGO_TEST_PG_URL`, `-timeout 60m`, 로그의 `go test exit=` 줄). 마이그레이션이 다른 상태에서 쿼리를 가진 다른 시험의 기대가 바뀌면 여기에 적고 고친다. 지금 찾은 것은 4번의 `TestQueriesAfterModels`뿐이다. `TestCheckEditedDraft`·`check_test.go:260`의 프로젝트에는 쿼리가 없다.

   하지 않는 것
   - 모델 DDL만의 연습 DB에 prepare하기(위의 ★)
   - 차이가 있을 때 CRUD·admin의 prepare(Q8). 겉으로 보이는 차이가 없는 안전띠라 시험도 없다
   - 단계를 넘는 갈래(Q7, D-61 그대로)
   - `TestMaxlen`·`TUTORIAL-FUTURE.md` §3 고치기(Q7의 것이고 이 작업에 닿지 않는다)
   - makemigrations의 진단 글 바꾸기
   - `--rename`을 짐작해 초안에 넣기. 초안은 지움과 더함이고, prepare에는 같은 스키마다.
   - PG의 `search_path` 밖(예: `public`)에 만든 것(TODO "연습 PG의 `search_path`" 그대로)

   계획 리뷰 codex·fable 반영: D-162와의 충돌(codex)은 리뷰가 갈렸고 Q8이 꼴을 계획에 맡긴 에이전트 편의의 결정이라 에이전트 편의로 ★를 지키고 까닭을 적음, 초안 실패 뒤 `a`를 다시 읽지 않고 닫기로 버림(따로 연결은 받지 않음), 음성 회귀를 seam 대신 오타 쿼리와 두 줄 `slices.Equal`로 CI에(codex·fable), SQLite에도 이름이 닿음(`INDEXED BY`)을 고치고 음성 시험을 두 방언에, (c)·(d)를 바꿔 지움은 새 프로젝트에서, 모든 기대를 줄 전체로, 정책 probe 접미사는 시험 없이 같은 길로(4e), DraftFails의 gen도 `slices.Equal`, `kor` 쿼리는 `status`로. 글 ` (checked against the models)`는 그대로(fable 7).
   기록(2026-11-22): `checkDatabase`가 마이그레이션과 모델이 다르면 `againstModels`로 마이그레이션의 연습 DB에 makemigrations의 초안을 적용하고 사용자 쿼리·`Results`·정책 probe를 돌려 "run makemigrations"와 한 실행에 낸다(글 끝 ` (checked against the models)`, `diag.Note`). 초안이 실패하거나 차이가 남으면 차이만, CRUD·admin의 prepare(`checkGenerated`)는 같을 때만. `maago doc query`·ARCHITECTURE·D-162의 글, 패키지 문서. 시험: `TestQueriesBeforeMigration`·`TestQueriesHandMigration`·`TestQueriesDraftFails`(두 방언, 줄 전체 `slices.Equal`), `TestPackageDoc` 골든. 모델 DDL만의 DB로 되돌리는 변이에서 `HandMigration`이 두 방언 빨강. PG 전체 시험 초록. 구현 리뷰는 ponytail·codex·fable: codex 지적 없음(샌드박스로 시험은 못 돎). fable의 막는 것 하나, "makemigrations는 쿼리 파일을 읽지 않는다"의 고정이 사라짐 → `BeforeMigration`에서 머리 오류의 `bad.sql`을 makemigrations 앞에 두고 makemigrations가 통과함을 본다(makemigrations가 쿼리를 읽게 하는 변이에서 빨강을 확인). 계획 글의 어긋남 셋(반영 줄의 정책 probe 시험, DraftFails의 `customers`, `stages` 주석)과 두 가드가 함께만 고정됨, CRUD·admin 안전띠의 시험 없음을 계획에 적음. 받지 않은 것: 다시 읽어 남은 차이의 가드를 지우기(ponytail 1, 계획이 일부러 둔 안전띠이고 차이가 있을 때만 한 번 더 읽는다), 도중의 연결 실패 진단에도 붙는 접미사(fable 5, 드물고 해가 없다), `Note`·`checkGenerated`는 그대로(ponytail 2·3)
4. **저장소 경계**(Q3): `MediaStore`와 로컬 구현(서명된 로컬 URL 포함), `OpenMedia`·`ServeMedia`를 그 위로, `WithMediaDir`의 처지(남기거나 지움, 계획에서 하나), `MediaDir`과 저장소를 함께 주면 `Handler` 오류 → 확인: 미디어 시험을 저장소를 받는 함수 하나로 만들어 로컬과 메모리 가짜 둘에(7번의 S3도 같은 함수), Range(206)·404·nosniff·attachment, 서명된 URL의 기한 지남·바뀐 서명 거절, 게시판 0029·0038·0039가 고치지 않고 걷는다

   계획

   따라가 본 길: 디스크는 `media.go` 한 파일에 박혀 있다. `mediaRoot`가 ctx의 디렉터리 문자열(`mediaKey{}`)을 `os.OpenRoot`로 열고, `SaveMedia`는 `.tmp-<key>`에 쓴 뒤 `Rename`, `OpenMedia`는 `*os.File`, `DeleteMedia`는 `Remove`(없으면 nil), `ServeMedia`는 `OpenMedia` → `Stat().ModTime()` → `http.ServeContent`(seek로 Range). ctx에 디렉터리를 넣는 곳은 `Handler`의 `serve`(`http.go:255`)와 `WithMediaDir`뿐이고, `Handler`는 `r.Files`가 있으면 `MediaDir`을 요구하고 `os.Stat`으로 디렉터리를 본다(`http.go:148`). `WithMediaDir`·`OpenMedia`를 부르는 곳은 `media_test.go`뿐(게시판·site fixture·contrib·복사 앱 모두 없음). 게시판은 `SaveMedia`·`DeleteMedia`·`ServeMedia`와 `MediaDir: "media"`만 쓰고, 시험이 `os.ReadDir(cfg.MediaDir)`가 열쇠 이름의 파일 하나뿐임(0029:143·185)과 `filepath.Join(cfg.MediaDir, a.Key)`의 이름 바꾸기로 404(0038:77)를 본다: 로컬 저장소는 디렉터리에 열쇠 이름 파일만 두는 지금의 꼴을 지켜야 한다(옆 파일·하위 디렉터리 없음). site fixture에 사용자 라우트 `GET /media/{key}`가 있다(`cmd/maago/testdata/web/site/urls.go:26`). `/static/`은 생성기가 이미 예약한다(`internal/web/routes.go:406`, `conflicts`로 겹침도). 복사된 auth 앱은 이미 `auth.SiteURL`(게시판 `BOARD_SITE_URL`)이라는 이름을 쓴다. `maago` 내부 시험은 `maago`를 import하는 패키지를 import할 수 없고, `contrib/s3`(6번)의 시험은 같은 모듈이라 `internal/`을 import할 수 있다.

   공개 API(루트, 새 것과 바뀌는 것 그대로)

   ```go
   // MediaStore is where media is kept: this machine's disk (LocalMedia,
   // for one instance) or a bucket (contrib/s3). Maago makes the keys and
   // checks them before a store sees one.
   type MediaStore interface {
   	// Put keeps r's bytes under key, of media type typ, whole or not at
   	// all, and returns how many it kept. A failed Put leaves nothing.
   	Put(ctx context.Context, key, typ string, r io.Reader) (int64, error)
   	// Open opens key's file. None is an error that is fs.ErrNotExist.
   	Open(ctx context.Context, key string) (MediaFile, error)
   	// Delete deletes the file of name, a name List gave; none is no error.
   	Delete(ctx context.Context, name string) error
   	// List returns the files whose names sort after after, in byte order,
   	// a page at a time; none when there are no more. It gives every file,
   	// keys or not (a half-written one's .tmp- name, a .gitignore).
   	List(ctx context.Context, after string) ([]fs.FileInfo, error)
   	// URL is a link to m's file, to download, that anyone may follow
   	// for ttl: whole seconds from one to 7 days, as MediaURL checks first.
   	URL(m Media, ttl time.Duration) (string, error)
   }

   // MediaFile is a media file open to read from any offset, as
   // http.ServeContent reads a Range. Stat tells its size and time.
   type MediaFile interface {
   	fs.File
   	io.Seeker
   }

   // LocalMedia keeps media in Dir on this machine's disk: for one
   // instance, since another machine does not see the files.
   type LocalMedia struct {
   	Dir     string // the directory, which must exist
   	SiteURL string // the site's address as browsers and other services reach it, Config.Prefix included: https://example.com/app; URL needs it
   }
   func (l LocalMedia) Put(ctx context.Context, key, typ string, r io.Reader) (int64, error)
   func (l LocalMedia) Open(ctx context.Context, key string) (MediaFile, error)
   func (l LocalMedia) Delete(ctx context.Context, name string) error
   func (l LocalMedia) List(ctx context.Context, after string) ([]fs.FileInfo, error)
   func (l LocalMedia) URL(m Media, ttl time.Duration) (string, error)

   func WithMedia(ctx context.Context, s MediaStore) context.Context      // WithMediaDir 대신
   func OpenMedia(ctx context.Context, key string) (MediaFile, error)     // 바뀜: *os.File → MediaFile
   func MediaURL(ctx context.Context, m Media, ttl time.Duration) (string, error)
   // 그대로: SaveMedia(ctx, File) (Media, error), DeleteMedia(ctx, key) error, ServeMedia(w, r, Media) error, Media

   type Config struct {
   	…
   	MediaDir string     // 그대로, 이제 Media: maago.LocalMedia{Dir: MediaDir}의 줄임
   	Media    MediaStore // 새 칸
   }
   ```

   바꿀 파일과 새 파일
   1. `media.go`
      - 위의 타입과 함수. `mediaRoot`·`WithMediaDir`를 지우고(`mediaKey`는 저장소의 ctx 열쇠로 남는다) `mediaStore(ctx) (MediaStore, error)`가 ctx의 저장소를 준다. 없으면 `maago: no media store in the context; Handler puts Config's there, and a test or a worker uses maago.WithMedia`.
      - `SaveMedia`: 열쇠를 만들고(그대로) `f.Open()`을 `s.Put(ctx, key, f.Type, in)`에. 반환의 `Size`는 지금처럼 `f.Size`(센 크기는 5번 `SaveMediaFrom`의 것). `OpenMedia`·`DeleteMedia`는 `validKey`를 본 뒤 저장소로(틀린 열쇠의 뜻은 그대로). `ServeMedia`의 몸은 그대로이고 `OpenMedia`가 주는 `MediaFile`의 `Stat()`·seek를 쓴다.
      - `MediaURL`: `validKey`, ttl이 1초 아래이거나 7일(S3 서명 URL의 상한) 위면 오류, 그다음 `s.URL(m, ttl.Truncate(time.Second))`.
      - `LocalMedia`의 Put·Open·Delete는 지금 `SaveMedia`·`OpenMedia`·`DeleteMedia`의 `os.Root` 몸을 옮긴 것이다(임시 이름과 `Rename`, 0600, 링크가 밖으로 못 감). 셋 모두 먼저 이름이 디렉터리 안의 파일 이름 하나인지 본다: 빈 이름, `.`, `..`, `/`·`\`가 든 이름은 오류(codex 3: `os.Root`가 밖으로는 막지만 하위 디렉터리로는 간다). `Open`은 `*os.File`을 그대로 준다(그것이 `MediaFile`이다). `List`는 `os.ReadDir(l.Dir)`(이름 차례)에서 일반 파일만(링크와 디렉터리는 빠진다), 이름이 `after` 뒤인 것의 `Info()`를 한 쪽에 모두 준다(다음 부름은 빈 쪽).
      - `LocalMedia.URL`: 먼저 `validKey`(codex 1). `SiteURL`이 비면 `maago: a local media store makes URLs only with its SiteURL: set Config.Media to maago.LocalMedia{Dir: …, SiteURL: …} in place of Config.MediaDir`. `SiteURL`은 끝의 `/` 하나를 떼고(`https://example.com/`이 흔한 꼴이라 거절하지 않는다) `http`·`https`, 호스트가 있고, 사용자 정보·쿼리·조각이 없어야 한다(아니면 오류, `Handler`도 같은 검사, codex 2·fable 7). 그러면 `SiteURL + "/maago/media?" + q`, `q`는 `url.Values{key, name, type, expires(유닉스 초), sig}.Encode()`(열쇠도 쿼리에: 아래 ★). `sig`는 프로세스의 무작위 32바이트 열쇠로 만든 HMAC-SHA256(`sig`를 뺀 값들의 `url.Values.Encode()` 하나)의 base64url. 문서 주석에 "only the process that made a URL serves it, and not after a restart"(fable 6).
      - 서명된 라우트 `serveSigned(w, r) error`: 쿼리에서 `sig`를 떼고 남은 값의 `Encode()`로 다시 계산해, 디코드하지 않은 글끼리 `hmac.Equal`(fable 4). 서명이 틀리거나 `expires`가 숫자가 아니거나 지났으면 `Status(403, …)`, 맞으면 `ServeMedia(w, r, Media{Key, Name, Type})`(attachment·nosniff·Range·없으면 404가 같은 코드).
      - → 확인: 아래 시험들.
   2. `http.go`
      - `Config.Media`와 문서. `MediaDir`의 문서에 "short for Media: maago.LocalMedia{Dir: MediaDir}".
      - `Handler`: `Media`와 `MediaDir`이 둘 다면 `maago: Config.Media and Config.MediaDir are both set; MediaDir is short for Media: maago.LocalMedia{Dir: MediaDir}: keep one`. 파일 폼이 있는데 둘 다 비면 지금 글에 덧붙여 `maago: forms take files, but Config.MediaDir is empty; set it to the directory to keep them in, such as media, or set Config.Media`(`Config.MediaDir is empty`를 보는 site 시험 그대로). `MediaDir`의 두 `Stat` 오류 글은 한 글자도 바꾸지 않는다. `Media`가 `LocalMedia`(값이나 포인터)면 같은 두 검사를 `Config.Media's Dir`라는 이름으로, `Dir`이 비면 오류, `SiteURL`이 있는데 위의 꼴이 아니거나 그 경로(끝 `/`를 뗀)가 `Config.Prefix`와 다르면 오류(Prefix는 프록시가 떼어도 사이트가 있는 경로라 같아야 한다)(`Prefix: "/app"`에 `https://example.com`은 모든 URL이 조용히 404, fable 7).
      - 검사는 `handler.media(files)` 하나에 모으고, 저장소(줄임이면 `LocalMedia{Dir: MediaDir}`)를 `handler.store`에 두어 `serve`가 `WithMedia(ctx, h.store)`. 로컬 저장소면(줄임도: 같은 프로세스의 일꾼이 제 `LocalMedia{Dir, SiteURL}`로 만든 URL을 받아야 한다) `h.mux.Handle("GET /maago/media", h.serve(&rtcore.Route{Method: "GET", Path: "/maago/media", Name: "maago_media", Serve: serveSigned}))`: 오류·panic이 `report`·`OnError`를 지난다. `h.routes`에는 넣지 않는다(CORS 사전 요청의 대상이 아니다).
      - → 확인: `TestMediaDirConfig`
   3. `internal/web/routes.go`: `/static/` 옆에 `/maago`·`/maago/…` 경로를 `route path %q is under /maago/, where maago serves signed media URLs`로 거절한다. 저장소가 로컬인지 생성기는 모르므로 늘. `GET /maago/media`는 글자뿐인 경로라 `/maago` 밖의 어느 GET·POST·PUT·PATCH·DELETE 라우트보다 구체적이어서 겹침(`conflicts`) 검사는 없다(실측: `GET /{a}/{b}`·`GET /{a}/media`·`GET /{a}/{b}/edit`·`POST /{a}/{b}`와 함께 panic 없음; 겹치는 것은 `HEAD /{a}/{b}`뿐이고 라우트 표에 `HEAD`는 없다). → 확인: `internal/web/static_test.go`의 표에 줄 둘(`/maago/x`, `/maago`), site fixture의 `/media/{key}`는 그대로 통과(`TestWeb`)
   4. 새 `internal/mediatest/mediatest.go`: `func Store(t *testing.T, s maago.MediaStore)`. 모두 `ctx := maago.WithMedia(t.Context(), s)` 위의 런타임 함수(`SaveMedia`·`OpenMedia`·`DeleteMedia`·`ServeMedia`·`MediaURL`)로 부르고, 저장소의 메서드는 런타임이 부르지 않는 `List`와 `Put`(성공 하나: 돌려준 n이 길이, fable 10; 실패 하나)만 직접 부른다. 저장 바이트는 `pngBytes`꼴의 104바이트(두 파일은 끝 바이트가 달라 열쇠 바꾸기에서 본문으로 가린다). `File`은 multipart 요청과 `rtcore.NewFile`로 만든다(`sentFile`을 옮김, fable 8).
      - 저장과 열기: `SaveMedia` 둘(열쇠가 다르다) → `OpenMedia`의 내용이 같고 `Stat().Size()`가 길이, `ModTime()`이 지금의 1분 안. 모양이 맞고 없는 열쇠는 `fs.ErrNotExist`
      - `ServeMedia`: GET 200과 본문, `Content-Type`, `X-Content-Type-Options: nosniff`, `Content-Disposition`(이름의 `"`·`\`·한글·CRLF·빈 이름, 지금 `TestServeMedia`의 표를 옮긴다), 형식 없음·틀린 형식의 `application/octet-stream`, `Last-Modified`, `Range: bytes=1-3` → 206 `PNG`, HEAD, 없는 열쇠 → `rtcore.Code` 404이고 쓴 것 없음
      - `List`: 빈 쪽이 올 때까지 `after`를 마지막 이름으로 넘기며(쪽 수에 상한: `after`를 무시하는 구현이 끝없이 돌지 않고 실패한다) 모은 이름에 두 열쇠가 있고, 차례가 오름이고, 크기가 맞다. 작은 열쇠를 `after`로 준 모음에 그 열쇠는 없고 큰 열쇠는 있다(`>=` 구현을 잡는다, fable 3). `DeleteMedia` 뒤에는 없고, 두 번 지워도 nil
      - 실패한 `Put`: 중간에 오류를 내는 Reader → 오류, 그 열쇠로 `OpenMedia`가 `fs.ErrNotExist`, `List`에 그 열쇠를 담은 이름이 없다(로컬의 `.tmp-`가 지워짐)
      - URL: `MediaURL`(1시간) → `http.Get` 200, 본문, `Content-Type`, `Content-Disposition`이 `attachment`로 시작. 쿼리를 `Encode()`로 다시 쓴 URL도 200(아래 4xx가 다시 쓰기 탓이 아님). 바뀐 URL은 모두 4xx이고 본문이 파일이 아니다: URL 안의 열쇠를 다른 저장 열쇠로(로컬은 쿼리, S3는 경로), 그리고 쿼리 값마다 하나씩 첫 글자를 다른 글자로 바꾼 것(로컬의 `key`·`name`·`type`·`expires`·`sig`, S3의 `X-Amz-*`·`response-*`: 서명이 덮지 않는 값이 있으면 잡힌다; 첫 글자는 base64·hex의 남는 비트가 없다, fable 4). 따로 만든 1초 URL은 2초 뒤 4xx. ttl 0·8일의 `MediaURL` 오류
   5. 새 `internal/mediatest/mediatest_test.go`: 메모리 가짜(`fstest.MapFS`: 그 파일이 `Seek`·`Stat`을 가져 `MediaFile`이고 `fs.ReadDir`가 `List`의 차례를 준다. `List`는 한 쪽에 하나씩(fable 3). URL은 가짜 자신의 `httptest.Server`가 경로의 열쇠·HMAC·기한을 보고 attachment로 준다)에 `Store`. 디스크에 기대는 것(`*os.File`, `os.Stat`)이 런타임에 남았으면 여기서 빨강.
   6. 새 `media_store_test.go`(package `maago_test`, `maago` 안의 시험은 `mediatest`를 import할 수 없다): `httptest.NewServer` 위에 `Handler(Config{DB: sql.OpenDB(nil), Apps: rt.NewApp으로 만든 라우트 하나, Media: LocalMedia{Dir: t.TempDir(), SiteURL: srv.URL}})`를 두고 `mediatest.Store`. `Prefix: "/app"`(`http.StripPrefix`, `SiteURL: srv.URL + "/app/"`: 끝 `/`를 뗌도 함께)에서는 URL 부분만, `MediaURL`의 GET이 200(`LocalMedia.URL`은 Prefix를 읽지 않으므로 `Store` 전체를 다시 돌릴 것이 없다, fable 7). 줄임(`MediaDir`)의 Handler에 같은 프로세스의 `LocalMedia{Dir: 같은 디렉터리, SiteURL}`이 만든 URL이 200인 줄 하나.
   7. `media_test.go`(package `maago`): 로컬에만 있는 것만 남긴다. `TestMedia`: 열쇠 이름의 파일 하나만 디렉터리에(게시판이 기대는 꼴), 0600, `../`가 이름에만, 밖으로 가는 링크, 저장소 없는 ctx의 오류가 `maago.WithMedia`를 말함. 새 `TestLocalMediaList`: `.gitignore`·`.tmp-x`·하위 디렉터리를 둔 디렉터리에서 `List`가 파일 둘만 주고 `Delete(".tmp-x")`가 지운다(fable 3). `Put`·`Open`·`Delete`가 `""`·`..`·`a/b`·`a\b`를 거절(codex 3). `TestServeMedia`는 `mediatest`로 옮겨 지운다. `TestMediaDirConfig`에 줄: `Media`와 `MediaDir` 둘 다, `Media: LocalMedia{}`와 `&LocalMedia{}`(빈 Dir, fable 9), 없는 Dir, 상대 `SiteURL`, 쿼리·사용자 정보가 있는 `SiteURL`, `Prefix: "/app"`에 경로 없는 `SiteURL`과 빈 Prefix에 경로 있는 `SiteURL`, `Media`만으로 파일 폼이 통과, ctx의 저장소가 줄임이면 `LocalMedia{Dir: dir}`, 로컬이 아닌 저장소의 Handler에서 `GET /maago/media`는 404(fable 5). 새 `TestLocalMediaURL`: `SiteURL` 없는 URL의 오류 글, 틀린 열쇠의 오류, 끝 `/`를 뗌, 같은 키·다른 `name`의 두 URL의 `sig`를 바꿔 끼우면 403(쿼리 값 바꾸기와 다른 꼴의 위조), 숫자 아닌 `expires`는 패키지 안의 서명 함수로 맞게 서명한 URL로 403(fable 2: 손으로 바꾼 값은 서명에서 먼저 막힌다)
   8. 문서: `internal/ref/topics.go` 폼 절의 "needs maago.Config.MediaDir"를 "needs a media store: maago.Config.MediaDir, or Config.Media"로, `API`의 `WithMediaDir`를 `MediaStore`·`MediaFile`·`LocalMedia`·`WithMedia`·`MediaURL`로 바꿔 더함(`TestPackageDoc`이 모든 공개 이름을 한 주제에 요구한다). main 절의 Handler 줄 "without Config.MediaDir" → "without a media store", Config 목록에 `Media`. 미디어 문단(단일 인스턴스·서명된 URL·청소)의 다시 쓰기는 9번. `doc.go`는 `-update`(PG 주소와 함께). `docs/ARCHITECTURE.md` 39줄 옆에 "M13이 더한 것: Config.Media·MediaStore·MediaFile·LocalMedia·WithMedia·MediaURL(WithMediaDir를 지움, D-160)". `rt/app.go:23` 주석의 `Config.MediaDir` → "a media store". `docs/DECISIONS.md` D-160에 이 작업의 고침 한 줄(아래 ★들). D-140의 고침은 9번 목록대로 9번에서
      - → 확인: `TestPackageDoc`, `TestDocExamples`, `TestNoDecisionsInUserText`
   9. 게시판: 고치지 않는다 → 확인: `TestBoardTutorial`(PG)·`TestBoardTutorialCatches`·`TestBoardMap`, `git diff --stat examples/`가 비어 있다

   단계와 확인
   1. `internal/mediatest`와 두 시험 파일을 먼저 쓴다 → 확인: `go vet ./...`가 없는 이름으로 실패(빨강)
   2. `media.go`·`http.go`·`routes.go` → 확인: `go test . ./internal/mediatest ./internal/web`
   3. 헛것(하나씩 넣고 실패를 보고 되돌린다): (a) `Handler`의 둘 다 검사를 지움 → `TestMediaDirConfig` (b) `serveSigned`가 `expires`를 보지 않음 → `Store`의 1초 URL(로컬) (c) HMAC 글에서 `type`을 뺌 → `Store`의 쿼리 값 바꾸기(로컬) (d) 메모리 가짜의 서명에서 열쇠를 뺌 → `Store`의 열쇠 바꾸기(가짜: 시험 함수가 가짜에도 실제로 문다) (e) `ServeMedia`가 `MediaFile`을 `*os.File`로 단언 → 메모리 가짜 빨강 (f) 로컬 `Put`이 실패 때 `.tmp-`를 남김 → `Store`의 실패한 Put (g) `routes.go`의 `/maago` 예약을 지움 → `static_test.go`의 `/maago/x` (h) 가짜 `List`가 `after`를 무시 → `Store`의 쪽 상한 (i) `serveSigned`가 숫자 아닌 `expires`를 지나침 → `TestLocalMediaURL`
   4. 문서·골든 → 확인: `TestPackageDoc`·`TestDocExamples`(`-update`는 PG 주소와 함께)
   5. 끝 확인: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_4`로 전체(`-timeout 60m`, 로그의 `go test exit=` 줄), 게시판 걷기 포함

   ★ → 에이전트 편의로 결정
   - **`WithMediaDir`를 지우고 `WithMedia(ctx, MediaStore)`.** 디렉터리 문자열은 저장소 하나를 줄인 것일 뿐이라 둘을 두면 같은 일에 길이 둘이다. Q2가 "일꾼의 ctx에 저장소를 넣는 공개 함수는 경계 뒤에 한 번만 이름 짓는다"고 한 그 함수가 이것이다. 5번은 새 이름을 짓지 않고 일꾼에서의 쓰임(문서와 `TestAddOutbox`)만 더한다. 지금 부르는 곳은 `media_test.go`뿐이고, 깨지는 API로 v0.4.0 태그 글에 적는다(10번, `OpenMedia`와 함께)
   - **`MediaFile`은 `fs.File` + `io.Seeker`.** `*os.File`이 그대로 맞으므로 로컬은 감싸지 않고, `OpenMedia`로 `Read`·`Seek`·`Stat`·`Close`만 쓰던 코드는 고치지 않아도 컴파일된다. 크기와 시각은 `Stat()`의 `fs.FileInfo`(stdlib 타입 하나로 `List`와 같이 쓴다). S3(6번)는 범위 GET을 미루는 Reader와 작은 `FileInfo`를 만든다
   - **`List(ctx, after)`는 이름 차례의 쪽, 빈 쪽이 끝.** S3의 `start-after`와 디스크의 정렬된 `ReadDir`가 같은 꼴이라 이음새가 없고, 이어 읽을 표시가 따로 없다. 열쇠 아닌 이름도 준다: 청소(7번)는 `.tmp-<key>`를 지워야 하고 `.gitignore`(게시판 0029의 `media/.gitignore`)는 남겨야 하므로 거르는 것은 7번의 몫이고, 저장소의 `Delete`는 `List`가 준 이름을 받는다(열쇠만 받는 것은 런타임의 `DeleteMedia`)
   - **`URL`은 ctx를 받지 않는다.** 로컬은 HMAC, S3는 SigV4 계산뿐이라 네트워크가 없다
   - **`LocalMedia`는 칸 둘의 구조체 값**(생성자 함수가 아니라). `maago.LocalMedia{Dir: "media", SiteURL: os.Getenv("BOARD_SITE_URL")}`이 한눈에 읽히고(PHILOSOPHY 2), 복사된 auth 앱의 `auth.SiteURL`과 같은 이름이다. `SiteURL`에 `Config.Prefix`를 넣는다: `LoginURL`처럼 "URL로 준 것은 그대로"
   - **`SiteURL` 없는 로컬의 `URL`은 오류**(상대 경로를 주지 않는다). 밖의 서비스(readmate의 OCR)에 상대 경로는 쓸모가 없고 조용히 틀린다(PHILOSOPHY 6·7). 오류가 `Config.Media: maago.LocalMedia{Dir, SiteURL}`로 고치라고 말한다. 줄임의 저장소에는 `SiteURL`이 없으므로 요청 안의 `MediaURL`은 이 오류다. 라우트는 그래도 모든 로컬 저장소에 단다(fable 5의 "SiteURL 있는 `Media`일 때만"은 받지 않음: 그러면 줄임의 Handler와 제 `LocalMedia{Dir, SiteURL}`을 쓰는 일꾼의 URL이 오류 없이 404가 된다. 다는 조건이 하나 적은 쪽이 조용히 틀리지 않는다)
   - **서명 열쇠는 프로세스마다 무작위 32바이트**(설정 칸 없음). 한계: 다시 시작하면 그 전의 URL이 403이 되고, 다른 프로세스의 일꾼이 만든 URL은 웹 프로세스가 받지 않는다. 로컬은 단일 인스턴스용(D-160)이고 URL은 짧은 기한이 필수라 감당할 만하다. 디렉터리에 열쇠 파일을 두는 길은 받지 않는다: 게시판 시험이 디렉터리에 열쇠 이름 파일만 있음을 본다(0029:143·185). 같은 프로세스 안에서는 `Handler`의 저장소와 일꾼의 `LocalMedia`가 값이 달라도 같은 열쇠로 서명한다(그래서 줄임에도 라우트를 단다). 여러 인스턴스는 `contrib/s3`
   - **서명된 라우트는 `GET /maago/media?key=…`(열쇠도 쿼리에)이고 생성기가 `/maago`·`/maago/…`만 예약한다.** `/media/`는 사용자가 흔히 쓰는 경로라(site fixture가 그렇다) 예약하면 에이전트가 왕복을 맞는다. `/maago/`는 사용자가 쓸 일이 없고 URL은 사람이 치지 않는다. 처음 꼴 `/maago/media/{key}`는 `/{board}/{id}/edit` 같은 흔한 라우트와 겹쳐 미디어 없는 프로젝트에도 새 거절이 생긴다(fable 1, 실측). 글자뿐인 경로는 아무것과도 겹치지 않아 거절이 "maago가 쓰는 경로" 한 줄(`/static/` 옆)만 넓어진다. 로컬 저장소일 때만 단다(S3의 URL은 버킷을 가리킨다)
   - **서명이 덮는 것은 `sig` 밖의 쿼리 전부(열쇠·`name`·`type`·`expires`, 덧붙인 값도)**, 거절은 403(S3의 `AccessDenied`와 같은 상태). 시험 함수는 "모든 쿼리 값과 경로의 열쇠"를 바꿔 4xx를 본다: 저장소마다 매개변수 이름이 달라도 서명이 덮지 않는 값을 잡는다
   - **ttl은 1초~7일, 초 단위로 자른다**(`MediaURL`에서). S3 서명 URL의 `X-Amz-Expires`가 정수 초이고 상한이 7일이라, 로컬에서 통과한 ttl이 S3에서 실패하지 않게 런타임이 한 번 본다
   - **시험 함수는 `internal/mediatest`**(공개 `maagotest`가 아니라). 쓰는 것은 이 모듈의 시험뿐이다(루트, 메모리 가짜, 6번의 `contrib/s3`). 사용자가 저장소를 쓰는 길은 D-160이 기각했다(SDK 어댑터를 앱이). 런타임 의존은 그대로(`TestRuntimeDependencies`: 루트는 이것을 import하지 않는다), `TestRuntimeImports`는 stdlib와 이 모듈이라 통과
   - **권한은 부르는 쪽**: `MediaURL`의 문서 주석이 "anyone holding the URL may download the file until it expires: call it once the view or worker has checked the user may have the file"라고 말한다(codex 5). 게시판 시험으로 고정하는 것은 게시판이 URL을 쓰는 8번에서
   - **`SaveMedia`의 `Size`는 그대로 `f.Size`**, `Put`의 센 값은 5번의 `SaveMediaFrom`에서 쓴다(Q2). 형식은 `Put` 전에 정해진다(`File.Type`, 5번은 첫 512바이트)

   하지 않는 것
   - `SaveMediaFrom`과 일꾼의 쓰임(5번), `contrib/s3`(6번), `SweepMedia`와 태그 `media`(7번), `maago doc`의 미디어 문단 다시 쓰기와 D-140 고침(9번), 태그 글(10번)
   - 서명 열쇠의 설정 칸이나 파일(위 ★), 여러 인스턴스의 로컬 저장소
   - 상대 URL을 주는 로컬 `URL`(위 ★)
   - 서명된 라우트의 CORS·캐시 조정(`ServeMedia`의 `Cache-Control: private` 그대로)
   - `List`가 열쇠만 거르기(7번의 몫, 위 ★)
   - `Media` 칸에 사용자가 만든 저장소의 시험 도우미 공개(위 ★)

   계획 리뷰 codex·fable 반영: 서명된 라우트를 `GET /maago/media?key=…`로(fable 1의 막는 것, 실측: `/maago/media/{key}`는 `/{a}/{b}/edit`와 겹친다), 그래서 `conflicts` 줄·`/{a}/{b}/edit` 시험·헛것 (g)를 `/maago` 예약의 헛것으로 바꿈. `LocalMedia.URL`도 `validKey`(codex 1, 열쇠는 `url.Values`가 인코딩). `SiteURL`은 http(s)·호스트·사용자 정보·쿼리·조각 없음, 끝 `/`는 떼고(거절하지 않음: 흔한 꼴), `Handler`에서는 경로가 `Prefix`와 같음까지(codex 2·fable 7의 "끝남"은 빈 Prefix에 `/app`을 지나치므로 같음으로). 로컬의 Put·Open·Delete는 파일 이름 하나만(codex 3). `MediaURL` 문서에 bearer와 권한 먼저(codex 5, 게시판 시험은 8번). 숫자 아닌 `expires`는 맞게 서명한 URL로(fable 2), 가짜 `List`는 한 쪽에 하나와 쪽 상한·`after`의 `>`·`.gitignore`·`.tmp-x`·하위 디렉터리(fable 3), 바꾸는 것은 첫 글자이고 디코드하지 않은 글끼리 비교(fable 4), 로컬이 아닌 저장소의 404 줄(fable 5의 뒤 절), 재시작·다른 프로세스의 한계를 `LocalMedia.URL` 주석과 9번 목록에(fable 6), Prefix 실행은 URL 부분만(fable 7), `File`은 `rtcore.NewFile`(fable 8), `&LocalMedia{}` 줄(fable 9), 성공한 `Put`의 n(fable 10). 받지 않은 것: `crypto/rand` 실패의 처리(codex 4: Go 1.24부터 `rand.Read`는 오류를 돌려주지 않고 프로세스를 끝내므로 0값·부분 열쇠가 생길 수 없다), 줄임에 라우트를 달지 않기(fable 5의 앞 절: 위 ★, 줄임의 Handler와 일꾼의 URL이 오류 없이 404가 된다)
   구현 리뷰 반영(ponytail·codex·fable): 런타임의 열쇠 검사를 `Store`에서 고정(`DeleteMedia(".gitignore")`·`MediaURL(Key "x")`가 오류, fable 1), `Put`이 실패하면 `SaveMedia`가 오류와 빈 `Media`(`TestMedia`의 `fullStore`, fable 2), 메모리 가짜가 `Put`의 형식을 `MapFile.Sys`에 두어 URL의 `Content-Type`으로 주고(형식은 쿼리에서 뺌) 초 단위가 아닌 ttl을 오류로 하며 `Store`가 1.5초의 `MediaURL`을 부른다(fable 3), `TestLocalMediaList`에 링크(fable 4), `Config.Media: (*LocalMedia)(nil)`은 panic 대신 `Dir is empty` 오류(fable 5), `MediaStore.URL` 주석에 "whole seconds from one to 7 days, as MediaURL checks first"(위 API 글도, fable 6), `TestLocalMediaURL`에 덧붙인 `x=1`이 403(fable 7)과 표의 `{ok, 404}` 한 갈래(ponytail 2). 받지 않은 것: `expires`의 `>=`(codex): `expires`는 유닉스 초로 내림이라 `>=`면 1초 URL이 만들자마자 죽을 수 있다, `>`는 ttl보다 짧지 않고 길어야 1초 더. `LocalMedia.URL`의 `validKey`를 지우기(ponytail 1, 계획 리뷰 codex 1이 일부러 넣은 것이고 `URL`을 직접 부르는 6번의 시험에도 안전띠). 아직 받는 곳 없는 `Put`의 n·`List`·`Delete(name)`(ponytail 3, 5번·7번이 쓴다, TODO), 두 벌의 `sentFile`·`pngBytes`(ponytail 4, `package maago`의 시험은 `mediatest`를 import하면 순환)
   기록(2026-11-22): `MediaStore`·`MediaFile`·`LocalMedia{Dir, SiteURL}`·`WithMedia`·`MediaURL`·`Config.Media`(`WithMediaDir`를 지움), `Handler`의 검사(`handler.media`: 둘 다 오류, 로컬이면 Dir 셋과 `SiteURL`의 꼴·Prefix, 로컬 저장소면 `GET /maago/media`를 단다), 생성기의 `/maago` 예약, 문서(`maago doc` form·main, `doc.go` 골든, ARCHITECTURE, `rt/app.go`, D-160 고침). 시험: `internal/mediatest.Store`를 로컬(`TestLocalMediaStore`, Handler가 URL을 받음)과 메모리 가짜(`TestMemory`, `fstest.MapFS`, 한 쪽에 하나)에, `TestLocalMediaURLServed`(Prefix와 끝 `/`, 줄임 Handler와 일꾼의 `LocalMedia`), `TestMedia`·`TestLocalMediaList`·`TestLocalMediaURL`·`TestMediaDirConfig`, `TestStaticDiags`의 줄과 골든. 헛것 (a)~(i) 모두 이름 붙인 시험이 빨강(되돌린 뒤 diff가 같음): (d)·(h)는 가짜를 바꾼 것이고 (h)는 `>=`로도 쪽 상한에 걸림. `GOTOOLCHAIN=go1.25.14`로 새 시험 초록. 게시판은 고치지 않음(`examples/` diff 없음). 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=…/m13_4`로 PG 전체 시험 `go test exit=0`(cmd/maago 373초, 게시판 걷기 포함). 구현 리뷰는 ponytail·codex·fable: 막는 것 없음. 반영은 위 줄: fable 표의 살아남은 변이 일곱과 nil `*LocalMedia`의 panic을 시험 줄로(변이마다 다시 넣어 `TestLocalMediaStore`·`TestMemory`·`TestMedia`·`TestLocalMediaList`·`TestLocalMediaURL`·`TestMediaDirConfig` 중 하나가 빨강을 확인), ponytail 2의 표. codex의 `>=`와 ponytail 1·3·4는 받지 않음(까닭은 위 줄). 반영 뒤 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, PG 전체 시험 `go test exit=0`(cmd/maago 367초)
5. **바이트로 미디어 저장**(Q2): `SaveMediaFrom`, 일꾼의 ctx에 저장소를 넣는 공개 함수 → 확인: 저장소 시험 함수에 바이트·빈 입력·형식·센 크기, 복사된 outbox 일꾼의 handle 안에서 저장이 성공(`TestAddOutbox`, 두 방언), `maago doc main` 예(`TestDocExamples`)

   계획

   따라가 본 길: 일꾼의 ctx에 저장소를 넣는 공개 함수는 4번이 이미 지었다(`WithMedia(ctx, MediaStore)`, D-160 고침: "Q2의 한 함수"). 그래서 이 작업의 새 공개 이름은 `SaveMediaFrom` 하나이고, 일꾼 쪽은 쓰임(문서·시험)만 더한다. `SaveMedia`는 지금 `mediaStore` → `f.Open()` → 열쇠 → `s.Put(ctx, key, f.Type, in)`이고 `Size`는 `f.Size`(`Put`의 n은 버린다, TODO 90줄). 업로드의 형식은 `rt/file.go:81~83`: 첫 512바이트를 `io.ReadFull`, `mime.ParseMediaType(http.DetectContentType(head[:n]))`로 매개변수를 뗀다(글은 `text/plain`이지 `; charset=utf-8`이 아니다). `rt`는 `maago`를 import하므로 `maago`가 `rt`의 것을 부를 수 없다. outbox의 `RunOnce`는 `context.WithoutCancel`·`WithTimeout`으로 ctx를 감싸는데 둘 다 값을 지키므로, `main`이 `outbox.Run`에 준 ctx의 저장소가 `handle`의 ctx에 그대로 온다: 복사되는 `outbox.go`의 코드는 고칠 것이 없다. `Handler`는 파일 폼이 없어도 `Config.Media`를 받고 로컬이면 `Dir`을 `Stat`한다(`http.go:200`). `rtcore.NewFile`로 `File`을 만드는 시험(`media_test.go`·`mediatest`)은 모두 PNG 바이트에 `image/png`라 형식을 다시 정해도 같다. site fixture의 `Upload`(`testdata/web/site/upload.go`)도 PNG만 받는다. 게시판 0029·0039는 `SaveMedia`의 `Name`·`Type`·`Size`만 쓰고, 끝까지 읽은 업로드에서 센 크기는 `f.Size`와 같다. 복사되는 outbox 원본은 `internal/scaffold/testdata/outbox`이고 걷기(0040)는 `maago add outbox`를 실행하므로 패치에 `outbox.go`가 없다.

   공개 API(루트, 새 것과 바뀌는 것 그대로)

   ```go
   // SaveMediaFrom keeps r's bytes, read to its end, in the media store
   // under a random key, as name, and returns what to keep in a row: Size
   // is how many bytes it read, Type what the first 512 tell, as a form's
   // File. A worker's ctx needs the store: maago.WithMedia. Nothing bounds
   // r here: to bound it, pass io.LimitReader(r, max+1) and, when Size is
   // over max, DeleteMedia the file and refuse it.
   // If the row is not written after, delete the file with DeleteMedia.
   func SaveMediaFrom(ctx context.Context, name string, r io.Reader) (Media, error)

   // 그대로(서명): SaveMedia(ctx context.Context, f File) (Media, error), 몸만 SaveMediaFrom(ctx, f.Name, f.Open()의 것)
   ```

   바꿀 파일과 새 파일
   1. `media.go`
      - `SaveMediaFrom`: 먼저 `mediaStore(ctx)`(없으면 r을 한 바이트도 읽지 않고 지금의 "no media store … maago.WithMedia" 오류). `head := make([]byte, 512)`, `n, err := io.ReadFull(r, head)`: `io.EOF`·`io.ErrUnexpectedEOF`는 짧은 입력이라 받고 다른 오류는 `maago: saving %q: %w`. `typ, _, _ := mime.ParseMediaType(http.DetectContentType(head[:n]))`(`rt/file.go:83`과 같은 식). 열쇠를 만들고(지금 `SaveMedia`의 세 줄을 옮김) `size, err := s.Put(ctx, key, typ, io.MultiReader(bytes.NewReader(head[:n]), r))`, 오류면 `maago: saving %q: %w`와 빈 `Media`. 돌려주는 것 `Media{Key: key, Name: name, Type: typ, Size: size}`
      - `SaveMedia`: `f.Open()`(오류 글 그대로) → `defer in.Close()` → `return SaveMediaFrom(ctx, f.Name, in)`. `Size`는 이제 센 값, `Type`은 같은 식으로 다시 정한 값(업로드 때 `f.Type`을 정한 바로 그 식이라 같다). 검사 차례가 `Open` 다음 `mediaStore`로 뒤집혀 저장소도 파일도 없으면 "no file" 오류가 된다: 받는다(둘 다 참인 오류, 줄을 더하지 않는다, 계획 리뷰 fable 6)
      - 주석: `Media`의 "a file SaveMedia kept" → "SaveMedia or SaveMediaFrom", `Media.Type`의 "(File.Type)" → "as its first 512 bytes tell". `File.Type`의 주석(`file.go`)은 그대로
      - → 확인: 아래 시험
   2. `internal/mediatest/mediatest.go`: `Store`에 `saveFrom(t, ctx, s)` 하나를 더한다(런타임 함수로 부르고 결과를 `OpenMedia`로 읽어 본다)
      - 길이 0, 100, 512, 1000바이트의 고르지 않은 바이트(바이트 i가 `byte(i*7)`꼴, 앞 512와 뒤가 다르다)를 `iotest.OneByteReader`에 감싸 저장: `Size`가 길이, 읽은 내용이 같다(머리를 버리거나 뒤를 버리거나 머리를 두 번 쓰면 빨강. `Read` 한 번으로 머리를 채우는 구현은 `MultiReader`가 나머지를 이어 이 줄로는 초록이고 형식 줄이 잡는다), 이름이 준 그대로. 이름은 `../../evil.txt`: `Name`은 그대로이고 `Key`는 32자이고 `.`·`/`가 없어 경로로 쓰이지 않는다(계획 리뷰 codex 4, 구현 리뷰 fable 2가 구현에 맞춤)
      - 형식(모두 `iotest.OneByteReader`에 감싸): PNG 바이트 → `image/png`, 글(`"hello\n"` 되풀이) → `text/plain`(매개변수 없음: 업로드와 같은 뜻), 빈 입력 → `text/plain`이고 `Size` 0이며 연 파일이 비었다(아래 ★)
      - 실패: 머리 안에서 오류를 내는 Reader(10바이트를 준 뒤 오류를 **한 번** 내고 그다음은 `io.EOF`: 늘 오류인 `iotest.ErrReader`면 머리의 오류를 버리는 구현도 `Put`에서 다시 걸려 헛것이 된다, 이 Reader면 그 구현은 10바이트 파일로 성공해 빨강)와 머리 뒤(600바이트 뒤)에서 오류를 내는 Reader → 둘 다 오류, 빈 `Media`, `List`의 이름이 저장 전과 같다(실패한 저장은 열쇠를 돌려주지 않으므로 "남김 없음"을 이름 모음으로 본다)
      - 이것으로 로컬(`TestLocalMediaStore`)과 메모리 가짜(`TestMemory`) 둘에 돌고, 6번의 S3도 같은 함수로 받는다
   3. `media_test.go`(package `maago`): `TestMedia`의 "저장소 없는 ctx" 줄 옆에 `SaveMediaFrom`도 같은 오류(`maago.WithMedia`를 말함)이고, 그때 Reader를 읽지 않았다(읽으면 실패하는 Reader로) 한 줄
   3a. `rt/file_test.go`의 `TestBindFile`: "the type is the content's"로 바인딩한 HTML 파일을 `maago.SaveMedia`(로컬 저장소 `t.TempDir()`)로 저장하면 `Type`이 `f.Value.Type`과 같다 한 줄. 업로드의 식(`rt/file.go`)과 `media.go`의 식이 갈라지면 빨강(계획 리뷰 fable 2)
   4. 새 시험은 복사본이 아니라 `cmd/maago/add_test.go`의 `TestAddOutbox`가 쓴다: `addProject` 뒤 `outbox/media_test.go`에 상수 `outboxMediaTest`를 쓰고 `goTest`(두 방언). 내용: `db := maagotest.DB(t)`, `ctx := maago.WithMedia(t.Context(), maago.LocalMedia{Dir: t.TempDir()})`, 복사본의 `job(t, db)`로 일 하나, `RunOnce(ctx, db, time.Now(), handle)`의 `handle`은 **제 인자 `ctx`로** `maago.SaveMediaFrom(ctx, "export.txt", strings.NewReader("done"))`를 부르고(바깥 ctx를 잡으면 헛것 (d)가 빨강이 될 수 없다) 그 오류를 닫힘 밖 변수에 둔다. `RunOnce`는 `handle`이 실패해도 `(true, nil)`(기록하고 다시 돌림)이므로 시험은 `found`·오류 없음에 더해 그 변수가 nil인지를 먼저 보고(헛것 (d)의 메시지가 "no media store"), 그다음 `maago.OpenMedia(ctx, m.Key)`의 내용이 `done`. `RunOnce`를 지나므로 복사본이 ctx를 `context.Background()`로 갈아 끼우면 빨강(아래 헛것 (d))
   5. `internal/scaffold/testdata/outbox/outbox.go`의 패키지 문서: "Once Serve has stopped" 문단 앞에 한 문장, "To keep files from deliver with maago.SaveMediaFrom, put the store Config.Media has in its ctx: ctx = maago.WithMedia(ctx, store)." 예의 코드는 그대로라 `outboxMain`도 그대로(`go doc ./outbox`를 읽는 에이전트가 길을 안다)
   6. `cmd/maago/docs/main.txt`(`maago doc main`의 예, `TestDocExamples`): 
      - `notes/models.go`의 `Note`에 `Export string // the key of the note's file, once the worker kept it`
      - `main.go`: `media := maago.LocalMedia{Dir: "media"} // the directory must exist` 하나를 `handler(db, media)`의 `Config.Media`와 일꾼의 `outbox.Run(work, db, deliver(db, media))`에 같이 준다(값 하나: 둘을 따로 쓰면 디렉터리가 갈라지는 실수를 예가 보이지 않게). `handler(db *sql.DB, media maago.MediaStore)`
      - `deliver(db maago.DB, media maago.MediaStore) func(context.Context, outbox.Job) error`(일꾼이 행을 쓰려면 db가 필요하다): 닫힘의 첫 줄이 `ctx = maago.WithMedia(ctx, media)`(main과 시험이 한 길을 지나 그 줄을 시험이 문다, 계획 리뷰 fable 1), Kind 검사 그대로, 페이로드의 `ID`로 `notes.GetNote`, `maago.SaveMediaFrom(ctx, fmt.Sprintf("note-%d.txt", n.ID), strings.NewReader(n.Text))`, `old := n.Export`, `n.Export = m.Key`, `notes.UpdateNote`이 실패하면 `errors.Join(err, maago.DeleteMedia(ctx, m.Key))`(어느 행도 가리키지 않는 파일을 남기지 않고 삭제의 오류도 버리지 않는다, 계획 리뷰 codex 2), 성공하면 `old != ""`일 때 `maago.DeleteMedia(ctx, old)`(다시 도는 일이 앞 파일을 남기지 않는다: form 주제의 "delete it with maago.DeleteMedia"와 맞고 7번이 와도 그대로 맞다, 계획 리뷰 fable 5)
      - `main_test.go`: `media := maago.LocalMedia{Dir: t.TempDir()}`를 `handler(db, media)`와 `outbox.RunOnce(ctx, db, time.Now(), deliver(db, media))`에. 그 뒤 `notes.AllNotes`의 첫 행의 `Export`가 비지 않고 `maago.OpenMedia(maago.WithMedia(ctx, media), n.Export)`의 내용이 `hello`. 두 번째 `RunOnce`(한 시간 뒤 없음)는 그대로
   7. 문서: `internal/ref/topics.go`
      - form 절: "maago.SaveMedia keeps the file." 뒤에 "maago.SaveMediaFrom keeps bytes the app made, such as a worker's (maago doc main)." `API`에 `SaveMedia` 다음 `SaveMediaFrom`(`TestPackageDoc`이 모든 공개 이름을 한 주제에)
      - main 절: outbox 문단 끝에 "Its deliver keeps files with maago.SaveMediaFrom once its ctx has the media store Config.Media has, maago.WithMedia(ctx, media), as the example's deliver does." 미디어 문단(단일 인스턴스·서명된 URL·청소)과 Kind 둘의 다시 쓰기는 9번
      - `doc.go`는 `-update`(PG 주소와 함께, `TestPackageDoc`)
      - `docs/ARCHITECTURE.md` 40줄의 "M13이 더한 것"에 `SaveMediaFrom`
      - `docs/TODO.md` 90줄: `Put`의 n은 `SaveMediaFrom`이 쓰므로 그 절을 지우고 `List`·`Delete(name)`(7번)만 남긴다
      - `docs/DECISIONS.md` D-160에 고침 한 줄(M13 5번: 아래 ★들)과 끝의 기록 줄
      - → 확인: `TestPackageDoc`, `TestDocExamples`, `TestNoDecisionsInUserText`
   8. 게시판: 고치지 않는다 → 확인: `TestBoardTutorial`(PG)·`TestBoardTutorialCatches`·`TestBoardMap`, `git diff --stat examples/`가 비어 있다

   단계와 확인
   1. 시험 먼저: `mediatest`의 `saveFrom`, `media_test.go`의 줄, `rt/file_test.go`의 줄, `outboxMediaTest` → 확인: `mediatest`·`media_test.go`는 `go vet ./...`가 없는 이름 `SaveMediaFrom`으로 실패(빨강), `rt/file_test.go`의 줄은 `media.go` 전에 초록이어도 되고(지금도 같은 식) 헛것 (i)가 문다, 문자열 상수 `outboxMediaTest`는 vet이 컴파일하지 않아 단계 2 전의 `TestAddOutbox`가 컴파일 실패로 빨강(계획 리뷰 fable 4)
   2. `media.go` → 확인: `go test . ./internal/mediatest`, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_4 go test -run 'TestAddOutbox' ./cmd/maago`(두 방언)
   3. `outbox.go` 문서, `main.txt`, `topics.go`, 골든 → 확인: `go test -run 'TestDocExamples|TestDoc$|TestPackageDoc' ./cmd/maago .`(`-update`는 PG 주소와 함께)
   4. 헛것(하나씩 넣고 실패를 보고 되돌린다): (a) `Put`에 머리를 빼고 r만 → `Store`의 내용(512·1000바이트) (b) `Size`를 `int64(n)`(머리의 길이)으로 → `Store`의 1000바이트 크기 (c) `ParseMediaType`을 빼고 `DetectContentType` 그대로 → `Store`의 `text/plain` (d) 복사 원본 `RunOnce`의 `context.WithoutCancel(ctx)`를 `context.Background()`로 → `TestAddOutbox`의 `outboxMediaTest`("no media store") (e) `ReadFull`의 다른 오류를 버림 → `Store`의 머리 안 실패 (f) `main.txt`의 `deliver` 안 `WithMedia` 줄을 뺌 → `TestDocExamples/main` (g) `ReadFull` 대신 `r.Read` 한 번 → `Store`의 형식(한 바이트 Reader에서 `image/png`가 아니게 된다) (h) `SaveMediaFrom`이 `mediaStore`보다 먼저 읽음 → `TestMedia`의 읽으면 실패하는 Reader (i) `media.go`의 식에서 `ParseMediaType`을 빼고 `rt`는 그대로 → `TestBindFile`의 HTML 형식 줄(`text/html; charset=utf-8` ≠ `text/html`)
   5. 끝 확인: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_4`로 전체(`-timeout 60m`, 앞에서 끝까지 기다려 로그의 `go test exit=` 줄), 게시판 걷기 포함. 새 시험은 `GOTOOLCHAIN=go1.25.14`로도

   ★ → 에이전트 편의로 결정
   - **일꾼의 함수는 새로 짓지 않고 4번의 `WithMedia`.** 같은 일에 이름이 둘이면 에이전트가 고른다. 이 작업은 그 쓰임을 셋에 박는다: `maago doc main`의 예(`TestDocExamples`), outbox 패키지 문서의 한 문장, `TestAddOutbox`의 `RunOnce` 안 저장
   - **빈 입력은 받는다**(`Size` 0, 형식 `text/plain`: 업로드의 빈 파일과 같은 식의 결과). 일꾼이 만든 빈 내보내기는 오류가 아니고, 오류로 하면 outbox가 12번 다시 돌린 뒤 일을 남긴다. 업로드와 뜻이 하나다(빈 파일 칸도 같은 결과). 빈 것을 막을 앱은 `Size == 0`을 본다
   - **형식은 업로드와 같은 식 한 줄을 `media.go`에 다시 쓴다**(새 공용 함수 없이). `rt`는 `maago`를 import해 `maago`가 `rt`의 것을 부를 수 없고, 한 줄을 위한 `rtcore` 함수는 이름만 는다. 두 곳이 갈라지지 않음은 `TestBindFile`의 한 줄(업로드의 `f.Type`과 `SaveMedia`의 `Type`이 같다)이 고정하고, `Store`의 `text/plain`은 `media.go` 쪽 식을 문다
   - **상한의 문서는 `io.LimitReader(r, max+1)`과 `Size > max`면 `DeleteMedia`.** Q2의 "`io.LimitReader`를 문서에"를 그대로 `LimitReader(r, max)`로 적으면 넘친 파일이 잘린 채 성공한다(PHILOSOPHY 7: 조용히 버리지 않는다). `max+1`이면 넘침이 `Size`에 보인다
   - **`SaveMedia`의 `Size`도 센 값**(Q2의 "얇은 함수"). 끝까지 읽은 업로드에서 `f.Size`와 같고, 크기의 출처가 `Put` 하나가 된다. 4번 ★("`SaveMedia`의 `Size`는 그대로 `f.Size`, 센 값은 5번에서")를 여기서 닫는다
   - **저장소를 먼저 찾고 r을 읽는다.** 저장소 없는 ctx에서 일꾼이 Reader(네트워크 응답 따위)를 반쯤 먹고 실패하지 않게
   - **일꾼 시험은 복사본이 아니라 `TestAddOutbox`가 쓰는 파일.** 미디어를 쓰지 않는 outbox 사용자의 복사본을 무겁게 하지 않고, 길은 패키지 문서 한 문장이 알린다. 두 방언은 `TestAddOutbox`의 두 줄이 이미 돈다
   - **`Put`은 업로드에서도 길이를 모르는 Reader를 받는다**(얇은 `SaveMedia`가 seek하는 `multipart.File` 대신 `MultiReader`를 준다). Q2·Q3 그대로이고, 6번의 `contrib/s3`는 seek를 기대하지 말고 늘 Q3의 임시 파일 길로 센다(6번 계획에서 이 줄을 본다)
   - **`main.txt`의 저장소 값은 하나를 `Config.Media`와 `WithMedia`에 같이.** 에이전트가 따라 쓸 꼴이므로 디렉터리가 갈라질 자리를 두지 않는다. `deliver(db)`는 일꾼이 행을 쓰는 가장 짧은 꼴(닫힘 하나)이다

   하지 않는 것
   - `SweepMedia`·태그 `media`(7번: 예의 `Export` 열에 태그는 7번이 단다), `contrib/s3`(6번), `maago doc`의 미디어 문단·main의 Kind 둘 다시 쓰기와 D-140 고침(9번), 게시판 0041~(8번)
   - 크기 상한 인자나 `MaxBytes` 칸(Q2: 상한은 앱의 몫)
   - `SaveMediaFrom`이 형식을 받는 인자(Q2: 첫 512바이트, 업로드와 같은 뜻)
   - outbox 원본의 코드 바꾸기(값이 이미 지나간다), 복사본에 미디어 시험 넣기(위 ★)
   - `File`을 바이트로 만드는 길(Q2가 기각한 `NewFile`)

   계획 리뷰 codex·fable 반영: 막는 것 없음(둘 다). 받은 것: `deliver(db, media)`가 안에서 `WithMedia`를 해 main과 시험이 한 길(fable 1, 헛것 (f)를 그 줄로, outbox 문서·main 주제의 문장도 그 꼴로), `TestBindFile`의 업로드 형식 = `SaveMedia` 형식 한 줄과 헛것 (i)(fable 2), `saveFrom` 첫 줄의 괄호(fable 3), 단계 1의 빨강을 vet과 `TestAddOutbox`로 나눔(fable 4), 청소 주석 대신 `old`를 지움(fable 5), `UpdateNote` 실패의 `errors.Join`(codex 2), `../../evil.txt` 이름의 저장(codex 4). 받지 않은 것: `SaveMedia`의 검사 차례(fable 6, 위 1의 까닭), 상한 넘침의 `DeleteMedia` 실패를 주석에 적기(codex 1: 그 오류의 처리는 앱의 보통 오류 처리이고, 남은 파일은 7번 청소가 치운다, 주석은 한 줄 짧게 둔다), `SaveMediaFrom` 주석의 권한 문장(codex 3: D-160의 권한은 파일을 밖에 건네는 `MediaURL`의 것이고, 저장은 아무것도 건네지 않는다: 권한을 본 뷰·일꾼이 행을 쓴다)
   기록(2026-11-24): `SaveMediaFrom`(저장소를 먼저 찾고, `ReadFull`의 머리 512바이트로 매개변수를 뗀 형식, `MultiReader`로 `Put`, 센 크기), `SaveMedia`는 그것을 부르는 얇은 함수, `Media` 주석, outbox 패키지 문서의 한 문장, `maago doc main`의 예(`Note.Export`, `deliver(db, media)` 안의 `WithMedia`, `UpdateNote` 실패의 `errors.Join`, 앞 파일 지우기, `main_test.go`의 파일 읽기), `topics.go`의 form·main 문장과 `API`, `doc.go` 골든, ARCHITECTURE·TODO·D-160 고침과 기록 줄. 시험: `mediatest.Store`의 `saveFrom`(로컬·메모리 가짜), `TestMedia`의 읽지 않은 Reader, `TestBindFile`의 형식 한 줄, `TestAddOutbox`의 `outboxMediaTest`(두 방언). 헛것 (a)~(i) 모두 이름 붙인 시험이 빨강(되돌린 뒤 diff 같음): (a)는 머리를 `head[:0]`로, (b)는 `int64(n)`으로 컴파일되게 넣음, (g)는 `TestMemory`의 PNG 형식, (d)·(f)는 "no media store". `GOTOOLCHAIN=go1.25.14`로 새 시험 초록. 게시판은 고치지 않음(`examples/` diff 없음). 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=…/m13_4`로 PG 전체 시험 `go test exit=0`(cmd/maago 368초, 게시판 걷기 포함)
   기록(2026-11-24, 구현 리뷰 반영): 구현 리뷰는 ponytail·codex·fable: 막는 것 없음(셋 다). 받은 것: `saveFrom`의 형식 표에 300바이트 뒤 NUL → `application/octet-stream`(fable 1, 머리 16바이트 변이가 `TestLocalMediaStore`·`TestMemory`에서 빨강임을 봄), 계획 2번의 `../../evil.txt` 글을 구현에 맞춤(`Key`는 32자이고 `.`·`/` 없음, fable 2), `topics.go` form 문단 다시 접기(fable 4), D-160 기록 줄의 날짜 11-24(fable 5), `rt/file.go`의 형식 줄에 `// as maago.SaveMediaFrom (media.go)`(ponytail 1). 받지 않은 것: 예의 `deliver`를 줄이기(ponytail 2, 행이 가리키지 않는 파일을 지우는 실패 처리가 이 예의 까닭이고 계획 리뷰가 받은 것), `WithMedia`를 Kind 검사 뒤로(ponytail 2의 nit, 계획 리뷰 fable 1이 정한 닫힘 첫 줄이고 모르는 Kind는 저장소를 쓰지 않아 차이 없음), `failOnce`(ponytail 3, 지적 없음), codex(지적 없음). 예의 정리 두 갈래(앞 파일 지우기, `UpdateNote` 실패 때 `DeleteMedia`)는 시험이 물지 않는다: 헛것 (f)만 `TestDocExamples/main`이 물고 두 갈래는 예의 글일 뿐이다(fable 3, 예의 시험을 늘리지 않음). 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=…/m13_4`로 PG 전체 시험 `go test exit=0`(cmd/maago 363초, 게시판 걷기 포함)

6. **`contrib/s3`**: stdlib SigV4(머리 서명과 서명된 URL), Range GET, LIST(페이지 나눔), 크기 모르는 Reader → 확인: S3 문서의 예 벡터(머리 서명과 서명된 URL 둘), 4번의 저장소 시험 함수를 가짜 S3에(가짜는 서명을 다시 계산해 틀리면 403), 길이 모르는 Reader의 `Content-Length`, CI에 MinIO 서비스와 `MAAGO_TEST_S3_URL`(D-91의 꼴, 없으면 CI에서 실패), `TestRuntimeImports`

   계획

   따라가 본 길: 경계는 4번이 정했다(`media.go`의 `MediaStore{Put, Open, Delete, List(ctx, after), URL}`, `MediaFile`은 `fs.File`+`io.Seeker`). 런타임은 열쇠(32자 hex)를 본 뒤에만 저장소를 부르고(`OpenMedia`·`DeleteMedia`·`MediaURL`의 `validKey`), `MediaURL`이 ttl을 1초~7일로 보고 초 단위로 잘라 `URL`에 준다. `SaveMediaFrom`은 늘 길이를 모르는 `io.MultiReader`를 `Put`에 준다(5번 ★: S3는 seek를 기대하지 않고 임시 파일로 센다). `ServeMedia`는 `OpenMedia` → `Stat()`(크기·시각) → `http.ServeContent`이고, ServeContent는 크기를 알려고 `Seek(0, io.SeekEnd)`, 그다음 `Seek(start)`와 `io.CopyN`으로 읽는다(Range가 없으면 0부터). `Handler`는 `LocalMedia`만 검사하고 다른 저장소는 그대로 ctx에 넣는다(`http.go`의 `handler.media`): S3 저장소는 `Handler`를 고치지 않는다. 시험 함수 `internal/mediatest.Store`가 저장소에 묻는 것: 두 저장의 열기·크기·1분 안의 시각, `Put`의 n, 런타임의 열쇠 검사, `ServeMedia`의 Range 206 `PNG`·HEAD·없음 404, `List`를 빈 쪽까지(쪽 상한 1000, `after`는 `>`), 실패한 `Put`이 남김 없음, URL의 200·`Content-Type`(`Put`의 형식)·`attachment`, `parsed.Query().Encode()`로 다시 쓴 쿼리도 200(공백이 `+`가 된다), 경로의 열쇠와 쿼리 값마다 첫 글자를 바꾸면 4xx, 1초 URL은 2초 뒤 4xx, 1.5초 `MediaURL`이 성공, `saveFrom`(한 바이트 Reader, 빈 입력, 실패한 Reader의 남김 없음을 `List`로). 메모리 가짜(`internal/mediatest/mediatest_test.go`)가 그 꼴의 본보기다. contrib의 문서 규칙: `TestNoDecisionsInUserText`가 `go doc -all`로 읽는 패키지 목록(`cmd/maago/doc_test.go:369`), `TestRuntimeDependencies`의 `contrib/auth` 줄(`runtime_test.go`), `TestRuntimeImports`는 `cmd/maago` 밖 모든 시험 아닌 파일(새 패키지도 저절로). PG 시험의 D-91 꼴은 `cmd/maago/practicedb_test.go`의 `pgURL`(없으면 skip, `CI`면 Fatal). TODO 91줄(4번 구현 리뷰 fable 1·3)이 이 작업에 넘긴 것: `Put`의 형식이 객체의 `Content-Type`, ttl이 `X-Amz-Expires` 정수 초(초 단위가 아니면 오류), 열쇠 검사 줄이 S3에서도 런타임 검사를 본다(이것은 `Store`가 이미 런타임 함수로 부르므로 그대로 덮인다).

   조사한 바깥 사실(2026-11-24): MinIO의 공식 배포가 없어졌다. Docker Hub `minio/minio` 저장소는 "object not found", `quay.io/minio/minio`는 익명으로 태그를 못 읽고, `dl.min.io`의 바이너리는 410, GitHub `minio/minio`는 archived, Homebrew `minio`(2025-10-15 bottle)는 deprecated이고 2027-02-17에 꺼진다. 남은 MinIO 이미지: `bitnamilegacy/minio`(Bitnami가 빌드한 MinIO, 2025-08에 얼어붙음, 태그 `2025.7.23-debian-12-r5`), `pgsty/minio`(제3자 fork, 2026-08). AWS 문서의 SigV4 예 벡터는 지금 페이지가 API 목차로 옮겨져, Wayback의 사본(`sig-v4-header-based-auth.html`, `sigv4-query-string-auth.html`)에서 읽었다: 자격 `AKIAIOSFODNN7EXAMPLE`/`wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`, `20130524T000000Z`, `us-east-1`, `examplebucket.s3.amazonaws.com`. GET Object(`Range: bytes=0-9`, `SignedHeaders=host;range;x-amz-content-sha256;x-amz-date`) → `f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41`, GET Bucket(`?max-keys=2&prefix=J`) → `34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7`, 서명된 URL(GET `/test.txt`, `X-Amz-Expires=86400`) → `aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404`. 시험의 주석에 출처 URL을 적는다.

   공개 API(새 패키지 `contrib/s3`, 그대로)

   ```go
   // Package s3 keeps a Maago project's media in a bucket of Amazon S3 or
   // of a service that speaks its API (Cloudflare R2, MinIO), so that every
   // instance of the site sees the same files: set maago.Config.Media to a
   // Store. It signs its requests itself (AWS Signature Version 4) and uses
   // the standard library only.
   package s3

   // Store keeps media in Bucket, a bucket of the site's own. Its methods
   // are maago.MediaStore's; the app calls maago's media functions.
   type Store struct {
   	Endpoint        string // the service's address, no bucket in it: https://s3.us-east-1.amazonaws.com, https://ACCOUNT.r2.cloudflarestorage.com, http://127.0.0.1:9000
   	Region          string // us-east-1; auto for R2
   	Bucket          string // the bucket's name
   	AccessKeyID     string
   	SecretAccessKey string
   }

   // Put spools r to a temporary file to count and hash it, then sends it
   // in one request: whole or not at all. One file is at most 5 GB.
   func (s Store) Put(ctx context.Context, key, typ string, r io.Reader) (int64, error)
   // Open asks the size and time of key's file; reading fetches from where
   // the file is read, so a Range costs only its bytes.
   func (s Store) Open(ctx context.Context, key string) (maago.MediaFile, error)
   func (s Store) Delete(ctx context.Context, name string) error
   // List gives a page of at most 1,000 names, the bucket's.
   func (s Store) List(ctx context.Context, after string) ([]fs.FileInfo, error)
   // URL is a presigned GET of m's file that downloads it, under m.Name,
   // with the type Put kept; ttl is whole seconds from one to 7 days.
   func (s Store) URL(m maago.Media, ttl time.Duration) (string, error)
   ```

   그 밖의 공개 이름은 없다(`var _ maago.MediaStore = Store{}`는 컴파일 검사 한 줄).

   바꿀 파일과 새 파일
   1. 새 `contrib/s3/s3.go`(위 API와 내보내지 않는 것들)
      - `check()`: 부를 때마다 칸을 본다. `Endpoint`는 `url.Parse`로 http·https, 호스트 있음, 사용자 정보·쿼리·조각 없음, 경로는 비거나 `/`(아니면 `s3: Store.Endpoint %q is the service's address, such as https://s3.us-east-1.amazonaws.com: http or https, a host, and no path; the bucket goes in Store.Bucket`, `%q`는 `u.Redacted()`: 사용자 정보에 넣은 비밀 열쇠가 오류 글로 `OnError`와 로그에 가지 않게, 파싱이 안 되면 값을 찍지 않는다). `Region`·`Bucket`·`AccessKeyID`·`SecretAccessKey`가 비면 그 칸 이름을 말하는 오류. `Bucket`은 S3의 버킷 이름 규칙(소문자·숫자·`.`·`-`, 3~63자, 처음과 끝은 영숫자: `.`·`..`·`\`·제어 문자·대문자·`/`가 모두 빠진다, 아니면 `s3: Store.Bucket %q is not a bucket's name: 3 to 63 of a-z, 0-9, . and -, a letter or digit first and last`): 경로의 한 마디로만 선다
      - 이름 검사: `Put`·`Open`·`Delete`는 먼저 이름이 한 마디인지 본다. 빈 이름, `.`, `..`, `/`·`\`가 든 이름은 오류(`LocalMedia.root`와 같은 규칙). 빈 이름은 버킷 자체의 요청(`DELETE /bucket` = DeleteBucket, `PUT /bucket` = CreateBucket)이 된다. 오류 글은 `s3: %q is not the name of one file in the bucket`. `URL`은 더 좁게 `m.Key`가 열쇠(32자 소문자 hex)인지 본다(`LocalMedia.URL`과 같다: `URL`을 바로 부르는 코드가 버킷의 다른 객체나, 빈 이름으로 버킷 목록 `GET /bucket/`의 링크를 만들지 못한다), 오류 글은 `s3: media key %q is not one maago.SaveMedia makes`
      - `escape(s string) string`: RFC 3986의 unreserved(`A-Z a-z 0-9 - . _ ~`)만 그대로, 나머지 바이트는(`/`도) `%XX` 대문자(AWS의 UriEncode, 이름에 `/`가 없으므로 `slash` 갈래는 두지 않는다). 경로는 `"/" + escape(Bucket) + "/" + escape(name)`이고, 같은 글을 요청의 `URL.RawPath`에(서명한 글이 그대로 선 위로 간다). 쿼리도 `escape`로 키 차례·값 차례로 이은 글 하나를 서명과 `RawQuery`에 같이(`url.Values.Encode`는 공백을 `+`로 써서 쓰지 않는다)
      - `(Store) sign(req *http.Request, payload string, now time.Time)`(자격·지역은 `Store`의 칸, 벡터 시험은 자격만 채운 `Store`로 부른다): `x-amz-date`·`x-amz-content-sha256`(payload)을 넣고, `host`(`req.URL.Host`)와 **서명하는 때 `req.Header`에 있는 모든 머리**를 소문자로 정렬해 정규 요청을 만든다. 날짜·지역·`s3`·`aws4_request`로 이어 만든 열쇠의 HMAC-SHA256을 `Authorization`에. 전송이 뒤에 더하는 `User-Agent`·`Accept-Encoding`·`Content-Length`는 서명 밖이다. 벡터 시험이 임의의 호스트·경로·머리로 부를 수 있게 `Store`의 경로 짓기와 따로다
      - `(Store) presign(u *url.URL, now time.Time, ttl time.Duration, q url.Values) string`: `X-Amz-Algorithm`·`Credential`·`Date`·`Expires`(정수 초)·`SignedHeaders=host`와 q를 정규 쿼리로, 몸 해시는 `UNSIGNED-PAYLOAD`, 끝에 `X-Amz-Signature`
      - `(Store) do(ctx, method, name string, query url.Values, header http.Header, body io.Reader, n int64, payload string) (*http.Response, error)`(name이 비면 버킷 자체: `List`와 `TestServer`의 버킷 만들기·지우기): `http.NewRequestWithContext` → `RawPath`·`RawQuery` → `sign(…, time.Now())` → 리다이렉트를 따르지 않는 패키지의 `client`(`CheckRedirect`가 `http.ErrUseLastResponse`: Go는 302의 PUT·DELETE를 GET으로 바꿔 따라가 200이면 한 적 없는 일이 nil이 된다). 몸이 있으면 `ContentLength = n`, n이 0이면 `http.NoBody`(Go는 몸이 nil 아닌 0 길이를 "모름"으로 보고 chunked로 보낸다: S3는 411). 2xx가 아니면 몸을 닫고 S3 오류 XML(`<Error><Code>…<Message>…`)을 `s3: %s %s/%s: %s: %s: %s`(방법, 버킷, 이름, `resp.Status`, Code, Message)로(3xx도 오류: 따라가지 않는다, 몸 없는 HEAD는 상태만). 404는 열쇠의 HEAD·GET(`Open`·`Read`)일 때만 `fs.ErrNotExist`를 싼다(없는 버킷의 `Put`·`Delete`·`List`가 "이미 없음"으로 삼켜지지 않게). 2xx면 응답을 받은 쪽이 몸을 늘 닫는다. 정규 쿼리는 `k=v` 글을 정렬한다(명세는 키 차례: 쓰는 키들에서는 결과가 같고, 가짜도 같은 꼴이라 시험이 가르지 못한다)
      - `Put`: `check`·이름 검사 → `os.CreateTemp("", "maago-s3-*")`(0600), `defer` 닫고 지움 → `io.Copy(io.MultiWriter(f, h), r)`로 n과 SHA-256 → `f.Seek(0, 0)` → `PUT`에 `Content-Type: typ`(빈 형식이면 머리 없음)와 hex 해시. 읽기가 실패하면 요청을 보내지 않는다(남김 없음). 돌려주는 것은 n
      - `Open`: `HEAD` → 404면 `do`의 오류가 `fs.ErrNotExist`, 그 밖의 2xx 아님은 오류, 2xx면 `Content-Length` 머리(못 읽거나 음수면 오류)와 `Last-Modified`(`http.ParseTime`, 없거나 못 읽으면 0 시각이 아니라 오류)로 `*object{s, ctx, info, off, body}`. `Read`: `off >= size`면 `io.EOF`(요청 없음, 빈 파일도), `body`가 없으면 `GET`에 `Range: bytes=off-`, 206이 아니거나 `Content-Range`의 시작이 off가 아니면 오류(200도 오류), 읽은 만큼 `off`를 민다. `Seek`는 `off`를 바꾸고 달라졌으면 몸을 닫는다(음수·없는 whence는 오류). `Stat`은 작은 `info{name, size, mod}`(모드 0444, `List`도 같은 것). `Close`는 몸을 닫는다
      - `Delete`: `DELETE`, 2xx만 nil(S3는 없는 열쇠에도 204). 404는 사실상 `NoSuchBucket`이라 오류로 둔다: `Bucket`을 잘못 적은 사이트의 `DeleteMedia`가 조용히 성공하지 않게. `Open`의 HEAD 404는 몸이 없어 없는 열쇠와 없는 버킷을 가르지 못한다: `Store` 문서에 한 줄(없는 버킷도 `Put`과 `Delete`가 칸 이름과 Code를 말한다)
      - `List`: `GET /bucket/?list-type=2&start-after=after`(`after`가 비면 그 값 없이), `encoding/xml`로 `ListBucketResult.Contents{Key, Size, LastModified}`(`LastModified`는 RFC 3339, 못 읽으면 0 시각이 아니라 오류: 7번 청소가 그 시각으로 여유를 잰다), 응답 한 번이 한 쪽, `Contents`가 없으면 빈 쪽. 버킷에 `/`가 든 남의 객체가 있으면 주지만 `Delete`는 이름 규칙으로 거절한다: `List`의 문서에 한 줄(7번 청소는 열쇠와 `.tmp-*`만 지운다)
      - `URL`: `check`, 열쇠 검사(위), ttl이 1초 아래·7일 위·초 단위가 아니면 오류(`MediaURL`이 먼저 보지만 `URL`을 바로 부르는 코드에도), `response-content-disposition`은 `ServeMedia`와 같은 글(`mime.FormatMediaType("attachment", filename=m.Name)`, 이름이 비면 `attachment`), 그다음 `presign`. `response-content-type`은 넣지 않는다(아래 ★)
      - → 확인: 아래 시험들
   2. 새 `internal/s3fake/s3fake.go`: `func New(t *testing.T) s3.Store`. `httptest.Server` 하나와 버킷 하나(이름 `media`, 자격·지역 고정)를 만들고 그것을 가리키는 `s3.Store`를 준다(서버는 `t.Cleanup`에서 닫는다). 7번의 `SweepMedia` 시험(다른 패키지)도 이것을 쓴다(그래서 `_test.go`가 아니라 internal 패키지, `mediatest`와 같은 꼴)
      - **서명 검증은 가짜가 따로 쓴 SigV4**(`contrib/s3`의 함수를 부르지 않는다): 받은 요청에서 정규 요청을 다시 짓는다. 방법, `r.URL.EscapedPath()`(선 위로 온 경로), `r.URL.RawQuery`를 풀어(`+`는 공백, 실제 서버처럼) RFC 3986으로 다시 이스케이프해 정렬한 쿼리(서명된 URL이면 `X-Amz-Signature`를 뺀다), `SignedHeaders`에 적힌 머리의 받은 값(`host`는 `r.Host`). 머리 서명은 `x-amz-content-sha256`이 받은 몸의 SHA-256과 같아야 하고(아니면 400 `XAmzContentSHA256Mismatch`), 서명된 URL은 `UNSIGNED-PAYLOAD`, `X-Amz-Date`+`X-Amz-Expires`가 지났으면 403, `X-Amz-Expires`가 정수 1~604800이 아니면 400. 자격의 범위(키·날짜·지역·`s3`)가 다르면 403. 서명이 다르면 403 `SignatureDoesNotMatch`
      - PUT: `Content-Length`가 없거나 chunked면 411 `MissingContentLength`, 몸과 `Content-Type`·시각을 지도에. GET·HEAD: `http.ServeContent`(Range 206·`Last-Modified`·길이), `Content-Type`은 PUT의 것, 서명된 URL이면 `response-content-disposition`을 `Content-Disposition`으로. 없으면 404 `NoSuchKey`(XML). DELETE: 204(없어도). LIST(`list-type=2`): `start-after`보다 큰 이름을 바이트 차례로 **한 쪽에 둘**(7번의 "LIST 페이지 둘 이상"이 저절로), `IsTruncated`, `LastModified`는 S3처럼 밀리초의 RFC 3339. 버킷이 `media`가 아니면 404 `NoSuchBucket`, 그 밖의 경로·방법은 400. 서명 검증 전에 받은 값(`X-Amz-Date`·`X-Amz-Expires`·`Authorization`)이 꼴에 맞지 않으면 400이나 403이지 500·panic이 아니다(`mediatest`가 쿼리 값의 첫 글자를 바꾼다). 경로는 `ServeMux`가 아니라 `HandlerFunc`에서 `EscapedPath()`를 직접 나눈다(mux는 경로를 정리한다)
   3. 새 `contrib/s3/s3_test.go`(package `s3_test`)
      - `TestFake`: `mediatest.Store(t, s3fake.New(t))`
      - `TestPutLength`: `t.Setenv("TMPDIR", t.TempDir())`. 길이 1000의 `iotest.OneByteReader`와 빈 Reader를 `Put` → 성공(가짜가 411을 내지 않음)이고 n이 길이, `Open`의 내용이 같다. 그다음 `SecretAccessKey`를 바꾼 저장소의 `Put` → 오류에 `403`과 `SignatureDoesNotMatch`(가짜가 실제로 검증한다), 그 오류 뒤에도 TMPDIR이 비어 있다(임시 파일을 늘 지움). `Bucket`만 다른 저장소의 `Delete` → 오류에 `NoSuchBucket`(404를 삼키지 않음)
      - `TestDisposition`: `사진 "1".png`·`a.png`·빈 이름의 `Media`로 `URL`을 만들어 가짜에 GET하면 `Content-Disposition`이 `ServeMedia`의 글(`mime.FormatMediaType("attachment", …)`, 빈 이름은 `attachment`)과 같다(`mediatest`의 `signedURLs`는 `a.png`의 `attachment` 머리만 본다)
      - `TestNames`: `Put`·`Open`·`Delete`가 `""`·`.`·`..`·`a/b`·`a\b`를 이름 규칙의 오류 글(`is not the name of one file`)로, `URL`이 그 다섯과 `a.png`(한 마디이지만 열쇠가 아님)를 열쇠의 오류 글(`is not one maago.SaveMedia makes`)로 거절한다(가짜의 400·404도 오류라, 오류가 있는지만 보면 검사를 지워도 초록: 글을 본다)
      - `TestConfig`: 칸마다 하나씩 빈 저장소, `Endpoint`에 경로(`https://s3.amazonaws.com/media`)·쿼리·사용자 정보·`ftp://`, `Bucket`의 `a/b`·`.`·`..`·`a\b`·`ab`(짧음)·`Media`(대문자)·`-ab`·`a\nb`(제어 문자) → 모든 메서드가 칸 이름을 말하는 오류. 사용자 정보 `http://KEY:SECRET@…`의 오류 글에 `SECRET`이 없다. `URL`의 ttl 0·1500ms·8일이 오류, 1초·7일은 성공(TODO 91줄: 초 단위)
   4. 새 `contrib/s3/internal_test.go`(package `s3`: 내보내지 않는 서명 함수를 부른다. `s3fake`는 `s3`를 import하므로 여기서는 쓰지 않는다)
      - `TestVectors`: 위 세 벡터. GET Object와 GET Bucket은 `http.NewRequest`에 문서의 머리(`Range`)를 넣고 `sign(req, 빈 몸 해시, 2013-05-24)` 뒤 `Authorization`이 문서 글 그대로, 서명된 URL은 `presign(…, 86400초, nil)`의 `X-Amz-Signature`가 문서 값
      - `TestReads`: 서명을 보지 않는 작은 `httptest` 서버(HEAD는 길이·`Last-Modified`, GET은 Range를 무시하고 200과 몸 전체 또는 `Content-Range`의 시작이 틀린 206)로 `Open`한 파일의 `Seek(5)` 뒤 `Read`가 오류다(200·틀린 시작을 받지 않음). 바른 206을 주는 같은 서버에서 `io.ReadAll` 한 번이 GET 하나임을 센다(차례로 읽으면 다시 열지 않음). `Last-Modified`가 없는 HEAD의 `Open`은 오류
      - `TestEscape`: 0~255의 바이트마다 `escape`가 unreserved면 그대로, 아니면 `%XX` 대문자, `/`는 `slash`일 때만 그대로. 공백은 `%20`(`+`가 아님), `~`는 그대로
      - `TestServer`(D-91 꼴): `MAAGO_TEST_S3_URL`(꼴 `http://KEY:SECRET@127.0.0.1:9000`, 사용자 정보가 자격, 지역은 `us-east-1`)이 없으면 skip, `CI`면 Fatal. 실행마다 새 버킷 `maago-<16 hex>`을 `do(ctx, "PUT", "", …)`(이름이 비면 버킷 자체)로 만들고 `t.Cleanup`에서 `List`·`Delete`로 비운 뒤 `DELETE /버킷`(worktree 여럿이 한 서버를 써도 `Store`의 `List` 비교가 부딪히지 않는다, PG의 시험마다 스키마와 같은 뜻). 그 위에서 `mediatest.Store`
   5. `runtime_test.go` `TestRuntimeDependencies`: `contrib/s3`의 의존이 런타임의 `want`와 `maago`, 그리고 자신뿐이라는 줄(`contrib/auth` 줄 아래, 같은 꼴). → 확인: 헛것으로 `s3.go`에 `internal/web`을 import하면 빨강. `TestRuntimeImports`도 헛것 하나: `s3.go`에 stdlib 밖 import(`golang.org/x/…`, 컴파일하지 않아도 파일을 읽는 검사)를 넣으면 빨강
   6. `cmd/maago/doc_test.go:369` `TestNoDecisionsInUserText`의 목록에 `./contrib/s3`
   7. `.github/workflows/ci.yml`: 이미지의 기본 명령(`run.sh`, Bitnami의 설정 스크립트)에 기대지 않고 test 앞 단계에서 `docker run -d -p 9000:9000 -e MINIO_ROOT_USER=maago -e MINIO_ROOT_PASSWORD=maago-secret --entrypoint /opt/bitnami/minio/bin/minio bitnamilegacy/minio:2025.7.23-debian-12-r5 server /tmp/data`로 MinIO를 이름 붙여 띄운다(확인한 사실(2026-11-24, Docker Hub 레지스트리의 이미지 설정과 층 목록): `Entrypoint`는 `entrypoint.sh`, `Cmd`는 `run.sh`, `User` 1001, 바이너리는 `/opt/bitnami/minio/bin/minio`, `/tmp`는 1777이라 1001이 쓴다. `services:`는 명령을 받지 않아 이 꼴을 쓸 수 없다), job env에 `MAAGO_TEST_S3_URL: http://maago:maago-secret@127.0.0.1:9000`, test 앞에 러너에서 기다리는 단계(`for i in $(seq 60); do curl -sf http://127.0.0.1:9000/minio/health/live && exit 0; sleep 1; done; exit 1`: 이미지 안에 curl이 있는지에 기대지 않는다)
   8. 문서
      - `contrib/s3`의 패키지 문서(위 API 글)
      - `docs/ARCHITECTURE.md`: `contrib/` 나무에 `s3/  S3 호환 미디어 저장소(stdlib SigV4, M13): Store{Endpoint, Region, Bucket, AccessKeyID, SecretAccessKey}가 maago.MediaStore (D-160)`, 원칙의 "예외는 넷"을 다섯으로(`Config.Media`에 주는 `contrib/s3`, D-160)
      - `docs/DECISIONS.md` D-160에 고침 한 줄(M13 6번: 아래 ★들, S3의 URL은 ttl보다 1초까지 짧다)과 끝의 기록 줄. D-123의 고침(루트 밖 공개 패키지의 셋째)과 `maago doc`의 미디어 문단은 9번 목록대로 9번(ARCHITECTURE의 "예외는 다섯"과 D-123의 "셋째"는 세는 법이 다르다: 9번에서 하나로 맞춘다)
      - `docs/TODO.md` 91줄 체크(`Put`의 형식은 `Store`의 URL `Content-Type` 줄이 가짜와 MinIO에서, 초 단위 ttl은 `TestConfig`가, 열쇠 검사는 `Store`의 런타임 줄이 문다)
      - → 확인: `TestNoDecisionsInUserText`, `go doc ./contrib/s3`가 패키지 문서와 `Store`를 보인다
   9. 게시판·런타임: 고치지 않는다(`media.go`의 `MediaStore` 주석도: 외부 이름을 지우지 않는 계약은 7번이 `SweepMedia`와 함께 적는다) → 확인: `git diff --stat 32376cb -- examples/ ':(glob)*.go'`(6번 구현 직전 커밋 기준, 루트의 `.go`만)에 `runtime_test.go` 밖의 파일이 없다
   10. `internal/mediatest/mediatest.go`: `Store`의 `List` 검사에 시각 한 줄(나열한 `m.Key`의 `ModTime`이 1분 안, 세 저장소 공통: 7번 청소가 그 시각으로 여유를 잰다) → 확인: `TestMemory`·`TestLocalMediaStore` 초록, 헛것 (p)

   단계와 확인
   1. 시험 먼저: `internal_test.go`의 벡터·escape, `s3_test.go`, `s3fake` → 확인: `go vet ./...`가 없는 패키지·이름으로 실패(빨강)
   2. `s3.go` → 확인: `go test ./contrib/s3 ./internal/s3fake`(벡터 셋이 문서 값과 같고 `TestFake`·`TestPutLength`·`TestNames`·`TestConfig` 초록, `TestServer`는 skip)
   3. 진짜 서버 한 번(기록: 6번 계획 리뷰 뒤 시도, 로컬에서는 안 된다. 받은 bottle을 풀어 돌리는 것을 이 실행의 권한이 막았고 docker도 없다. 그래서 6번을 합치기 전에 push를 물어 CI의 `TestServer` PASS를 보고, 늦어도 10번 태그 앞에 본다: 가짜는 같은 사람이 같은 이해로 쓴 것이라 벡터가 덮지 않는 HEAD 404·`start-after`·쿼리의 `+`는 진짜 서버만 가른다. 원래 길:) Homebrew의 `minio` bottle을 `brew fetch`로 받아 scratchpad에 풀고(설치하지 않음) `MINIO_ROOT_USER=maago MINIO_ROOT_PASSWORD=maago-secret minio server <scratch>/data --address 127.0.0.1:9000`을 띄워 `MAAGO_TEST_S3_URL=http://maago:maago-secret@127.0.0.1:9000 go test -run TestServer -v ./contrib/s3` → 확인: PASS(skip이 아님). 이 길이 안 되면 그렇게 기록하고 첫 진짜 서버 실행은 push 뒤 CI다. 로컬이 되든 안 되든 CI 파일(이미지가 명령 없이 뜨는지, env 이름, health 경로)은 로컬에서 볼 수 없으므로 M13 끝의 "push 뒤 GitHub Actions" 항목에 CI의 `TestServer` PASS(skip이 아님)를 늘 더한다(push는 묻고 한다)
   4. 헛것(하나씩 넣고 실패를 보고 되돌린다): (a) `sign`이 `range` 머리를 빼고 서명 → `TestVectors`의 GET Object (b) `escape` 대신 `url.QueryEscape` → `TestEscape`(공백·`~`) (c) 빈 몸에 `http.NoBody` 대신 빈 파일 → `TestPutLength`의 빈 Reader와 `Store`의 빈 저장(가짜 411) (d) PUT의 몸 해시를 빈 몸의 해시로 → 가짜 400, `TestFake` (e) `URL`이 `response-content-disposition`을 서명 뒤에 덧붙임 → `Store`의 쿼리 값 바꾸기(가짜) (f) `Read`가 다시 열 때 늘 `bytes=0-` → `Store`의 Range 206 `PNG` (g) 가짜 `List`가 `start-after`를 무시 → `Store`의 쪽 상한 (h) 가짜가 서명을 보지 않음 → `TestPutLength`의 틀린 비밀 열쇠 (i) 이름 검사를 지움 → `TestNames`(오류 글) (n) `s3.go`에 stdlib 밖 import → `TestRuntimeImports` (j) `URL`이 초 단위가 아닌 ttl을 받음 → `TestConfig` (k) `Endpoint`의 경로를 받음 → `TestConfig` (l) 임시 파일을 지우지 않음 → `TestPutLength`의 TMPDIR (m) 가짜의 만료 검사를 지움 → `Store`의 1초 URL (o) `URL`의 열쇠 검사를 지움 → `TestNames` (p) `List`가 `LastModified`를 읽지 않음 → `mediatest.Store`의 시각 줄(가짜에서) (q) `URL`이 `"attachment; filename=" + m.Name`으로 이음 → `TestDisposition` (r) `Read`의 206·`Content-Range` 검사를 지움, 또는 `Read`마다 다시 열기 → `TestReads` (s) `Delete`가 404를 nil로 → `TestPutLength`의 `NoSuchBucket` (t) `Endpoint` 오류 글이 `Redacted` 없이 → `TestConfig`의 비밀 (u) `Bucket` 규칙을 `/`만으로 → `TestConfig` (v) `http.DefaultClient`로 리다이렉트를 따름 → `TestRedirect`(모든 요청에 302, 그 끝은 200) (w) 모든 404를 `fs.ErrNotExist`로 → `TestPutLength`의 `NoSuchBucket` 줄
   5. 문서와 CI 파일 → 확인: `go test -run 'TestNoDecisionsInUserText' ./cmd/maago`, `go test -run TestRuntimeDependencies .`
   6. 끝 확인: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_4`로 전체(`-timeout 60m`, 앞에서 끝까지 기다려 로그의 `go test exit=` 줄), 게시판 걷기 포함. 새 시험은 `GOTOOLCHAIN=go1.25.14`로도

   ★ → 에이전트 편의로 결정
   - **칸은 `Endpoint`·`Region`·`Bucket`·`AccessKeyID`·`SecretAccessKey`, 요청은 늘 경로 꼴(`/bucket/key`).** 버킷 주소 하나(`https://bucket.s3…`)를 받으면 서비스 주소와 버킷 주소가 같은 꼴이라, 에이전트가 AWS 문서의 서비스 주소를 넣으면 `Put`이 열쇠 이름의 버킷을 만드는 요청(CreateBucket)이 된다: 조용히 틀린다(PHILOSOPHY 7). 칸을 나누면 `Endpoint`의 경로를 거절할 수 있다. 경로 꼴은 S3·R2·MinIO에서 같고, 점이 든 버킷 이름도 TLS에서 깨지지 않는다. 칸 이름은 AWS 설정의 이름 그대로라 환경 변수와 한 대 한으로 잇는다. 대가: AWS가 2020년에 미룬 경로 꼴의 폐기가 다시 오면 칸 하나를 더한다
   - **타입은 `s3.Store`, 칸의 값**(생성자가 아니라): 4번의 `LocalMedia{Dir, SiteURL}`과 같은 꼴(`Config{Media: s3.Store{…}}`가 한눈에 읽힌다). 버킷 주소 칸을 두지 않으므로 메서드 `URL`과 이름이 부딪히지 않는다. `http.Client` 칸은 없다(리다이렉트를 따르지 않는 패키지의 클라이언트 하나, 기한은 ctx가)
   - **`Put`은 늘 임시 파일에 받아 세고 해시한다**(5번 ★, Q3). 길이와 실제 몸 해시를 머리에 실어 S3가 받은 바이트를 검증한다(`UNSIGNED-PAYLOAD`를 쓰지 않는다). 한 PUT의 상한 5GB는 문서에 적고 나누어 올리기는 하지 않는다(상한은 앱의 몫, Q2)
   - **`Open`은 `HEAD` 하나, 읽기는 그 자리부터의 범위 GET.** ServeContent가 크기를 먼저 seek로 묻고 Range의 시작으로 seek하므로, 처음부터 몸을 여는 GET은 Range 요청마다 버려진다. 읽기 자리가 몸의 자리와 다를 때만 다시 연다(처음부터 끝까지 읽는 일꾼은 GET 하나)
   - **서명하는 머리는 `host`와 그때 요청에 있는 모든 머리.** 머리 목록을 따로 들지 않아 `Range`·`Content-Type`·`x-amz-*`가 저절로 서명되고, AWS의 벡터(서명된 `range`)를 그대로 재현한다
   - **URL은 `response-content-disposition`만 덧붙이고 형식은 `Put`의 것**(TODO 91줄). `ServeMedia`와 같은 `attachment` 글이고, 형식은 저장 때 정해진 하나라 URL에서 다시 정하지 않는다. `MediaURL`이 ttl을 이미 보지만 `URL`도 초 단위·1초~7일을 본다(`X-Amz-Expires`는 정수 초, 메모리 가짜와 같다)
   - **이름 규칙은 `LocalMedia`와 같은 "한 마디"**: 빈 이름이 버킷 자체의 요청이 되는 것을 막고, 두 저장소의 `Delete(name)`이 같은 이름을 받는다. 버킷에 `/`가 든 남의 객체가 있으면 `List`는 주지만 지우지 못한다(7번 청소는 열쇠 꼴과 `.tmp-`만 지운다)
   - **가짜는 `internal/s3fake`이고 서명을 제 코드로 다시 계산한다.** `contrib/s3`의 서명 함수를 부르면 같은 버그를 둘이 나눠 가진다: 클라이언트의 계산은 문서 벡터가, 서명한 것과 선 위로 간 것이 같은지는 가짜가 받은 요청에서 본다. SigV4 약 40줄이 둘이 되지만 시험의 기준이 따로 서는 값이다. internal 패키지라 7번의 `SweepMedia` 시험(루트)이 같은 가짜를 쓴다. 한 쪽에 둘이라 쪽 넘김이 늘 돈다(공개 쪽 크기 칸은 두지 않는다)
   - **진짜 서버 시험은 실행마다 새 버킷**, `MAAGO_TEST_S3_URL`은 자격을 사용자 정보에 담은 서버 주소이고 지역은 `us-east-1`(MinIO의 기본). `Store`의 `List` 비교는 버킷을 혼자 쓸 때만 맞고, PG의 시험마다 스키마(D-113)와 같은 뜻이다. 없으면 skip, `CI`면 실패(D-91)
   - **CI의 MinIO는 `bitnamilegacy/minio:2025.7.23-debian-12-r5`를 `docker run`으로 `minio server`를 직접**(공식 `minio/minio`·quay·`dl.min.io`가 모두 없어짐, 위 사실). MinIO 자신의 코드를 이름 있는 배포자가 빌드한 이미지이고, 명령을 적어 띄우므로 Bitnami 설정 스크립트의 뜻(환경 변수 이름, 데이터 경로)에 기대지 않는다(codex 계획 리뷰 5). 얼어붙은 이미지이지만 시험 전용이고 태그를 고정한다. 그 이미지마저 사라지면 같은 줄을 다른 S3 호환 서버로 바꾼다(시험은 서버를 모른다). 새 종류의 의존이 아니라 계획된 MinIO의 배포처가 바뀐 것으로 보았다
   - **런타임은 그대로**: `Handler`는 S3 칸을 검사하지 않는다(첫 요청이 칸 이름을 말하는 오류를 낸다). `Handler`가 `contrib/s3`를 알면 의존이 거꾸로 선다. 고른 것(fable 계획 리뷰 8): (a) 이대로 두고 `Store` 문서에 "칸은 첫 요청에서 본다", (b) 런타임이 `interface{ Check() error }`인 저장소를 `Handler`에서 부른다(런타임 4줄, 공개 이름 하나) 중 (a). (b)는 문서에만 사는 선택적 인터페이스라 에이전트가 찾지 못하고, 빠뜨린 칸은 어차피 첫 저장에서 칸 이름을 말한다. 8번 게시판의 `main`이 환경에서 칸 다섯을 채우며 빈 값을 기동 때 거절하는 꼴을 보인다. codex는 이 줄을 짚지 않았다
- **S3의 서명된 URL은 ttl보다 1초까지 짧다**: `X-Amz-Date`가 초에서 잘린다(로컬은 1초까지 길다). D-160 고침에 적는다(fable 계획 리뷰 9)

   하지 않는 것
   - 가상 호스트 꼴(`bucket.s3…`), 세션 토큰·STS·IAM 역할의 자격, 나누어 올리기(5GB 넘는 파일), 다시 시도, 서버 쪽 암호화 머리, 버킷 안의 접두(버킷은 사이트 혼자의 것), `response-content-type`
   - 환경 변수나 URL 글에서 `Store`를 읽는 공개 함수(게시판 8번이 칸 다섯을 환경에서 채운다), `Handler`의 S3 칸 검사
   - CI에서 진짜 AWS(자격 비밀이 든다), 로컬 시험이 MinIO를 스스로 띄우기
   - `maago doc`의 미디어 문단(단일 인스턴스·S3·서명된 URL)과 D-123·D-140 고침(9번), `SweepMedia`(7번), 게시판(8번)

   계획 리뷰 codex·fable 반영: 막는 것은 `URL`의 이름 검사(둘 다, 32자 열쇠 검사로 `LocalMedia.URL`과 같게, `TestNames`와 헛것 (o)), `Bucket` 검사(codex 2, `/`만이 아니라 S3 버킷 이름 규칙 하나로, 헛것 (u)), CI의 기본 명령(codex 5, 레지스트리에서 이미지 설정을 읽어 `docker run --entrypoint …/minio … server /tmp/data`로). 받은 것: 응답 상태·길이·시각·몸 닫기(codex 3, 못 읽은 시각은 오류), `List`의 시각 줄을 `mediatest.Store`에(fable 2, 계획 10항과 헛것 (p)), `TestDisposition`(fable 3, 헛것 (q)), `TestReads`(fable 4, 헛것 (r)), `Delete`의 404는 오류이고 `Open`의 한계는 문서에(fable 5, 헛것 (s)), `Endpoint` 오류 글의 `Redacted`(fable 6, 헛것 (t)), 9항 확인의 기준 커밋(fable 7: `main...`은 4·5번의 `media.go`까지 보이므로 `32376cb`와 `':(glob)*.go'`로), `Handler`의 S3 칸을 ★로(fable 8, (a)를 고른 까닭은 ★에), S3 URL이 1초까지 짧음(fable 9), 로컬 MinIO가 안 되면 합치기 전에 push를 물음(fable 10, 단계 3의 기록), 이름 오류 글과 예외 세는 법(fable 작은 것 둘). 받되 고쳐서: codex 4(외부 이름의 계약)는 이 작업에서 `Store.List`의 문서에만 적고 `MediaStore` 주석과 `SweepMedia` 시험은 7번에 넘긴다(런타임을 고치지 않는 9항을 지키고, 그 계약을 쓰는 곳이 7번이다)
   기록(2026-11-24): `contrib/s3/s3.go`(`Store`, `check`의 `Redacted`와 버킷 규칙, `checkName`, `isKey`, `escape`·`canonicalQuery`, `sign`·`presign`·`signature`, `do`의 비 2xx 오류와 404의 `fs.ErrNotExist`, 임시 파일의 `Put`, HEAD의 `Open`과 범위 GET의 `object`, `List`의 RFC 3339 시각), `internal/s3fake`(제 SigV4, 411·`NoSuchBucket`·`NoSuchKey`, 한 쪽에 둘), `s3_test.go`(`TestFake`·`TestPutLength`·`TestRedirect`·`TestNames`·`TestConfig`·`TestDisposition`), `internal_test.go`(`TestVectors`·`TestEscape`·`TestReads`·`TestServer`), `mediatest.Store`의 `List` 시각 줄, `TestRuntimeDependencies`의 `contrib/s3` 줄, `TestNoDecisionsInUserText`의 목록, CI의 `minio` 단계와 `MAAGO_TEST_S3_URL`, ARCHITECTURE·TODO 91줄·D-160 고침과 기록 줄. 벡터 셋이 문서 값과 같다. 헛것 (a)~(u)와 `Read`마다 다시 열기(r2), `s3.go`의 `internal/web` import(`TestRuntimeDependencies`) 모두 이름 붙인 시험이 빨강((v)·(w)는 첫 커밋 뒤 advisor가 짚은 리다이렉트와 404의 넓은 `ErrNotExist`를 고치며 더함)(되돌린 뒤 같은 파일): (h)·(m)은 컴파일되게 `false &&`로, (r2)는 처음에 초록이라 `TestReads`의 몸을 1000바이트로 늘려(`io.ReadAll`이 여러 번 읽게) 빨강을 봄. 진짜 서버는 로컬에서 돌리지 못했다(단계 3의 기록): `TestServer`는 skip이고 첫 진짜 서버는 CI다. D-91 꼴: `CI=1`이고 `MAAGO_TEST_S3_URL`이 없으면 `TestServer`가 FAIL(`MAAGO_TEST_S3_URL is not set; CI must run the S3 tests`). `GOTOOLCHAIN=go1.25.14`로 새 시험 초록. 9항의 `git diff --stat 32376cb -- examples/ ':(glob)*.go'`는 `runtime_test.go`뿐. 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=…/m13_4`로 PG 전체 시험 `go test exit=0`(cmd/maago 369초, (v)·(w)를 고친 뒤 다시 373초, 게시판 걷기 포함)
   기록(2026-11-24, 구현 리뷰 반영): 구현 리뷰는 ponytail·codex·fable: 막는 것 없음(셋 다). 받은 것: `escape`의 `slash` 인자를 지움(ponytail 1, 본 코드는 늘 false), `presign`의 nil 분기를 지우고 시험이 `url.Values{}`를 넘김(ponytail 2), `List` 앞의 `check`를 지움(ponytail 3, `do`가 먼저 본다; `Put`·`Open`·`Delete`의 `checkName`은 이름 검사라 그대로), `sign`이 `do`의 정규 `RawQuery`를 그대로 서명(ponytail 4, `TestVectors`의 쿼리는 이미 정렬된 꼴), `object.at`을 지우고 `Seek`가 자리가 바뀌면 몸을 닫음(ponytail 5), 없는 버킷의 `List`가 `NoSuchBucket`이고 `fs.ErrNotExist`가 아님(fable 1, `TestPutLength`), `TestReads`에 못 읽는 `LastModified`의 `List`(fable 2: 0 시각이면 7번 청소가 새 파일을 지울 수 있다), 206인데 짧은 몸의 `io.ErrUnexpectedEOF`·`Open`과 `Read` 사이에 지워진 파일의 `fs.ErrNotExist`·`Content-Length` 없는 HEAD(fable 3), 계획 글의 어긋남 둘(`TestServer`의 이름 `""`, 오류 글의 `resp.Status`, fable 6)과 `escape`·`object`의 글. 변이 여섯(`name != ""`, `LastModified` 오류, `ErrUnexpectedEOF`, 404의 `GET`, `Content-Length` 검사, `Seek`의 몸 닫기)을 하나씩 넣어 `TestPutLength`·`TestReads`가 빨강임을 보고 되돌림(`cmp` 같음). 받지 않은 것: 가짜의 몸 해시 검사(fable 4)는 직접 시험하지 않는다, 헛것 (d')로 적는다: 가짜의 해시 검사를 끄면 시험 없음(지금은 (d)가 고치지 않은 가짜에서 빨강으로 덮는다, `s3fake`는 `contrib/s3`를 import해 안쪽 시험이 손 요청을 서명할 수 없다), `Content-Type` 서명의 시험(fable 5, 서버가 서명 안 된 `Content-Type`도 받아 동작 위험이 없다), `Put`의 `GetBody`(fable 7, "하지 않는 것: 다시 시도"와 맞음), codex(지적 없음, 쓰기 금지 환경이라 실행은 못 함). 합치기 조건: push 뒤 CI의 `TestServer`가 PASS(skip 아님)인지 본다(fable, TODO). `GOTOOLCHAIN=go1.25.14`로 `contrib/s3` 초록. 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=…/m13_4`로 PG 전체 시험 `go test exit=0`(cmd/maago 370초, 게시판 걷기 포함)

7. **고아 청소**(Q4): 태그 `media`, 생성된 "쓰는 중" 쿼리, `SweepMedia`(6번 계획 리뷰 codex 4에서 넘어옴: `MediaStore.Delete` 주석의 "a name List gave"를 열쇠와 `.tmp-*`로 좁히고, 버킷의 남의 이름(`/`가 든 것)을 지우지 않음을 시험에) → 확인: 오래된 고아만 지움, 여유 안의 새 것과 `.tmp-`, 태그 없는 프로젝트의 오류, 열쇠 열이 둘인 모델·두 앱에서 둘 다 쓰는 중으로 봄, 로컬과 가짜 S3 둘(LIST 페이지 둘 이상)

   계획

   따라가 본 길: 저장소의 이름 모음은 4번의 `MediaStore.List(ctx, after)`(이름 차례의 쪽, 빈 쪽이 끝, 열쇠 아닌 이름도: 로컬의 `.tmp-<key>`·`.gitignore`, S3의 남의 `a/b`)이고 시각은 `fs.FileInfo.ModTime()`(로컬 mtime, S3 `LastModified`, 6번이 못 읽으면 오류로). `Delete(name)`은 로컬이 `root`의 한 마디 규칙, S3가 `checkName`으로 `/`를 거절한다. 열쇠 검사는 `validKey`(32자 소문자 hex, `media.go`). 생성기의 모든 생성물은 앱마다다(`build.go:158`의 `models_gen.go`·`admin_gen.go`·`queries_gen.go`·`urls_gen.go`): 프로젝트 하나에 하나인 생성 파일은 없다. 모든 앱을 받는 생성기는 이미 있다(`admingen.File(d, app, apps, …)`). `maago gen`은 앱 디렉터리에서 불러도 `project.Root`로 프로젝트 전체를 읽는다(`build.go:38`, `TestGenThenCheck`가 `blog/`에서 gen하고 루트에서 check): 한 실행의 `s.apps`는 늘 프로젝트의 모든 앱이라, 앱마다 쓰는 프로젝트 전체의 쿼리가 실행마다 같다. 생성 코드가 런타임에 남기는 상태의 자리는 `internal/rtcore`(루트와 `rt`가 함께 본다, 마지막 `Handler`의 Prefix·TimeZone처럼)이고, 생성 코드는 공개 `rt`만 부른다(`rt`는 `maago`를 import하므로 루트는 `rt`를 못 부른다). 지금 생성 코드에 `init`은 없다(`mapper.Names`는 사용자의 `init`을 이름 충돌에서 뺀다, `names.go:170`). 일꾼은 `Handler`를 지나지 않고(`outbox.Run(work, db, …)`), `DBFrom`은 없으면 panic이고 문서가 "a test uses WithDB"라고 한다: 생성된 쿼리와 `outbox`는 모두 db를 인자로 받는다. 태그는 `ref.FieldTags`의 닫힌 목록을 `model.fieldOptions`가 읽고 목록에 있는데 `case`가 없으면 panic(`check.go:410`). `TestDocKeysAreRead`는 목록의 항목마다 그것을 빼면 그 목록을 보이는 주제(`model`)의 예를 `maago check`가 거절해야 하므로 `docs/model.txt`가 `media`를 써야 한다. `TestGeneratedCallsAppAPI`는 생성 코드의 주석 속 `maago.X`도 세고 앱 코드(`internal/ref` 포함)에 그 이름이 있어야 한다. 생성된 SQL의 안전띠는 `checkGenerated`(`build.go:394`, 모델 DDL의 연습 DB에 prepare). 뿌리 시험(`package maago`)은 DB 드라이버를 import하지 않는다(`maagotest.DB`는 쓰는 쪽이 드라이버를 넣는다): 진짜 DB로 생성 코드를 돌리는 자리는 `cmd/maago/internal/gentest`(두 방언, 같은 시험 파일, `gentest.Open`이 fixture 모든 앱의 표를 만들고 `*sql.Conn`을 준다)와 `TestDocExamples`. `gentest`는 같은 모듈이라 `internal/s3fake`를 import할 수 있다. `s3fake`는 PUT에 `time.Now()`를 찍고 `s3.Store`는 `/`가 든 이름을 보내지 않아, 지금은 오래된 객체도 남의 `a/b`도 만들 수 없다. 5번이 `maago doc main` 예의 `Note.Export`에 태그를 7번으로 넘겼다.

   공개 API(새 것과 바뀌는 것 그대로)

   ```go
   // package maago (media.go)

   // SweepMedia deletes the media files no row keeps, from the store in
   // ctx (maago.WithMedia): a file older than olderThan whose key no field
   // tagged media holds, in any app, and what a failed save left. olderThan
   // is 0 for 24 hours, or at least an hour, longer than the longest a
   // file waits for its row (a job that saves files, then writes rows).
   // Any error stops it before it deletes another file. Run it now and
   // then, such as daily, beside the server or the worker. The store must
   // be this database's alone: a file that only another database's rows
   // keep is deleted.
   func SweepMedia(ctx context.Context, db DB, olderThan time.Duration) error

   // MediaStore.Delete의 주석만 바뀜:
   // Delete deletes the file of name, a key or .tmp- and a key, as List
   // gave it; none is no error.

   // package rt (rt/app.go): 생성 코드만 부른다
   // MediaInUse tells maago.SweepMedia the query of the media keys that
   // the project's fields tagged media hold, every app's: the generated
   // code of each app tells it, the same, as the program starts.
   func MediaInUse(query string)
   ```

   태그(`ref.FieldTags`, `maago doc model`): `{"media", "The field keeps a media key, Media.Key. Tag every field that keeps one: maago.SweepMedia deletes the old file of a key that no tagged field keeps. A string or *string.", nil}`

   생성물: 프로젝트에 `media` 필드가 하나라도 있으면 **모델이 있는 모든 앱**의 `models_gen.go` 끝에
   ```go
   // The media keys the project's rows keep, every app's, for maago.SweepMedia.
   func init() {
   	rt.MediaInUse(`SELECT "main" FROM "files_file" WHERE "main" IS NOT NULL UNION ALL SELECT "thumb" FROM "files_file" WHERE "thumb" IS NOT NULL UNION ALL SELECT "attachment" FROM "notes_note" WHERE "attachment" IS NOT NULL`)
   }
   ```
   (자리표시 없음: 두 방언이 같은 글. 차례는 앱·모델·필드의 차례.) 태그 없는 프로젝트는 지금과 바이트까지 같다(gentest 골든과 `examples/` 생성물이 그대로).

   바꿀 파일과 새 파일
   1. `internal/ref/ref.go`: `FieldTags`에 `media`(위 글). → 확인: `TestDoc`, `TestDocKeysAreRead`(아래 `model.txt`와 함께)
   2. `internal/model/check.go`: `Field`에 `Media bool // keeps a media key: maago.SweepMedia reads it`. `fieldOptions`에 `case "media":` `c.shape(o, where, NoValue, 0, "write it alone, like media")`이고 `f.Type != String`이면 `media keeps a media key in a string, but %s is %s`(고치는 법 `declare the field as string, or *string to allow NULL`), 맞으면 `f.Media = true`. `ID`는 이미 있는 규칙이 막는다(`ID may only have label and help`). → 확인: `internal/model`의 새 `TestMediaTag`(아래)
   3. `internal/mapper/crud.go`: `func MediaInUse(apps []*model.App) string`: 모든 앱·모델의 `Media` 필드마다 `SELECT q FROM t WHERE q IS NOT NULL`(`dialect.Quote`로 열과 표)을 ` UNION ALL `로 이은 글, 없으면 `""`. `IS NOT NULL`은 늘(포인터 아닌 열에도: 한 꼴이 짧고, NULL을 `string`으로 읽다 실패하는 길이 없다)
   4. `internal/mapper/gen.go`·`templates/models.go.tmpl`: `ModelsFile(d project.Dialect, app *model.App, apps []*model.App, pols []*Policy, user User) []byte`(모든 앱을 받는다: `admingen.File`과 같은 꼴). 안에서 `MediaInUse(apps)`가 비지 않으면 템플릿의 `.MediaInUse`(`GoString`)로 위의 `init`을 쓴다. 어느 앱이 담는지를 `ModelsFile` 안에서 정하므로 `build.go`와 `TestGenerated`가 한 길을 지난다
   5. `internal/build/build.go`: `ModelsFile` 부름에 `s.apps`. `checkGenerated`의 `sqls`에 `mapper.MediaInUse(s.apps)`(비지 않을 때). → 확인: `go test ./internal/build ./cmd/maago -run 'TestGenThenCheck|TestGenerated|TestGenMedia'`. 새 `cmd/maago/build_test.go`의 `TestGenMedia`: `internal/testdata/media`의 복사본(첫 마이그레이션은 시험이 `sqlite.DDL`로 쓴다)에 `build.Gen`을 돌려 `files/models_gen.go`·`tags/models_gen.go`가 gentest sqlite 골든과 같다(`TestGenerated`는 `ModelsFile`을 제 `apps`로 부르므로 `build.go`의 부름은 이것만 문다)
   6. `internal/rtcore/app.go`: `var MediaQueries []string // what generated code told rt.MediaInUse`. `rt/app.go`: `MediaInUse(query)`가 `rtcore.MediaQueries`에 덧붙인다(같은 글이면 덧붙이지 않는다). `init`은 한 고루틴에서 차례로 돌고 `SweepMedia`는 그 뒤라 잠금이 없다
   7. `media.go`
      - `SweepMedia`: 차례대로, 어느 오류든 바로 돌려준다(그 뒤로 지우는 것 없음)
        1. `olderThan`: 0이면 24시간, 음수이거나 1시간 아래면 `maago: SweepMedia's olderThan is 0 for 24 hours, or at least an hour, not %v: a file is saved before its row is written`
        2. `rtcore.MediaQueries`가 비면 `maago: SweepMedia knows no field that keeps media keys: tag each with media, such as Key string `+"`"+`maago:"media"`+"`"+`, run maago gen, and import the project's apps in this program`. 글이 둘 이상이면(어느 앱의 생성물이 낡음) `maago: the apps' generated code tells SweepMedia different media fields; run maago gen`
        3. `mediaStore(ctx)`(지금의 "no media store … maago.WithMedia" 오류)
        4. `before := time.Now().Add(-olderThan)`. `List`를 빈 쪽까지(`after`는 쪽의 마지막 이름) 걸으며 후보를 모은다: 이름이 `validKey`이거나 `.tmp-`와 `validKey`이고 `ModTime().Before(before)`인 것. 그 밖의 이름(`.gitignore`, `.tmp-x`, 대문자 hex, `a/b`)은 저장소에 넘기지 않는다(S3의 이름 오류에 기대지 않음)
        5. 후보가 없으면 nil(DB를 읽지 않는다). 있으면 **후보를 고른 뒤** 쿼리를 `db.QueryContext`로 읽어(`Scan`과 `rows.Err()`의 오류도 돌려준다: 끊긴 읽기의 모자란 모음으로 지우지 않는다) 열쇠의 모음(`map[string]bool`, `// ponytail: every key in use in memory; a lookup per page of candidates if rows outgrow it`). 쓰는 중인 열쇠의 파일은 모음을 읽은 때보다 늦게 행을 잃었어도 다음 청소까지 남는다
        6. 후보마다 `.tmp-`면 지우고, 열쇠면 모음에 없을 때만 `s.Delete`. 첫 오류에서 돌려준다
      - `MediaStore.Delete`의 주석(위)
      - → 확인: 아래 시험들
   8. `internal/s3fake/s3fake.go`: `func Touch(s s3.Store, name string, mod time.Time)`: s의 가짜 버킷에서 name의 시각을 mod로, 없으면 빈 파일로 만든다(오래된 파일과 `contrib/s3`가 쓰지 않는 이름을 위해). `New`가 가짜를 패키지의 `sync.Map`(`Endpoint → *fake`)에 두고 `t.Cleanup`에서 뺀다
   9. 새 fixture `internal/testdata/media/`: 앱 셋. `files/models.go`(`File{ID; Name string; Main string `+"`"+`maago:"media"`+"`"+`; Thumb *string `+"`"+`maago:"media"`+"`"+`}`: 열쇠 열이 둘인 모델, 하나는 NULL을 받는다), `notes/models.go`(`Note{ID; Attachment string `+"`"+`maago:"media"`+"`"+`}`: 둘째 앱), `tags/models.go`(`Tag{ID; Name string}`: 태그 없는 앱)
   10. `cmd/maago/gen_test.go`: `gentests`에 `{"files", "../../internal/testdata/media", "files"}`·`{"tags", "../../internal/testdata/media", "tags"}`, `ModelsFile`에 `apps`. 새 `cmd/maago/internal/gentest/{sqlite,postgres}/{files,tags}/`(`models.go`는 fixture의 복사, `models_gen.go`·`admin_gen.go`는 `-update`, 시험 파일은 두 방언이 같다)
      - `files_test.go`의 `TestSweep`: `gentest.Open(t, "internal/testdata/media")` 위에서 저장소 둘(`maago.LocalMedia{Dir: t.TempDir()}`, `s3fake.New(t)`: 한 쪽에 둘이라 쪽이 여럿)을 서브시험으로. `SaveMediaFrom`으로 다섯(a, b, c, 오래될 고아 d, 새 고아 e), `InsertFile`로 행 둘(`Main: a.Key, Thumb: &b.Key`와 `Main: "", Thumb: nil`), `notes_note`에 c를 SQL 글로(`INSERT … VALUES ('<c.Key>')`: 열쇠는 hex, 다른 앱의 CRUD는 이 패키지에 없다). 남의 이름: `.gitignore`, `.tmp-x`, 오래될 `.tmp-<새 열쇠>`, 새 `.tmp-<새 열쇠>`, 그리고 S3는 `s3fake.Touch`의 `a/b`(로컬 `List`는 정규 파일만 주므로 하위 디렉터리는 아무것도 물지 않는다). e와 새 `.tmp-` 밖은 모두 48시간 전으로(로컬 `os.Chtimes`, S3 `s3fake.Touch`)
        - `SweepMedia(ctx, db, 30*time.Minute)`와 저장소 없는 ctx의 `SweepMedia`가 오류(앞은 `at least an hour`, 뒤는 `maago.WithMedia`를 말함). `Delete`가 오류인 저장소(저장소를 감싼 시험 타입)에서 그 오류가 돌아오고 `Delete`는 한 번만 불린다(첫 오류에서 멈춤), `SweepMedia(ctx, db, 72*time.Hour)`가 nil이고 아무것도 지우지 않음(여유를 본다)
        - `SweepMedia(ctx, db, 0)`이 nil, 그 뒤 `List`로 모은 이름이 정확히 {a, b, c, e, `.gitignore`, `.tmp-x`, 새 `.tmp-`, (S3) `a/b`}: d와 오래된 `.tmp-`만 없다. 두 번째 `SweepMedia(ctx, db, 0)`도 nil이고 이름이 같다
      - `tags_test.go`의 `TestSweep`: 태그 없는 앱 하나만 링크한 시험 이진에서 48시간 전의 파일 둘 중 `files_file`의 행(SQL 글)이 가진 것은 남고 고아는 지워진다(그 앱의 생성물도 프로젝트의 쿼리를 알린다)
   11. `media_test.go`(package `maago`): 새 `TestSweepMedia`(DB 없이 닿는 갈래, db는 `testDB`): `rtcore.MediaQueries`를 저장하고 `t.Cleanup`에서 되돌린다. 48시간 전의 고아 하나(`os.Chtimes`)를 둔 `LocalMedia`의 ctx에서 `olderThan` -1·30분의 오류, 빈 `MediaQueries`의 오류(`maago:"media"`를 말함), 다른 글 둘의 오류(`run maago gen`), 글 하나를 알리고 `http_test.go`의 `testDB`(`QueryContext`가 "no driver")로 부르면 그 오류가 돌아온다(쓰는 중을 못 읽으면 지우지 않는다). 줄마다 제 오류 글의 조각을 본다(`olderThan` -1·30분은 `at least an hour`). 매 줄 뒤에도 고아가 남아 있다
   12. `internal/model/model_test.go`: 새 `TestMediaTag`: `Key string media`·`Thumb *string media`는 `Media`, `Count int64 media`·`Raw string media=x`·`ID int64 media`는 오류(글을 본다), `medai`는 did you mean
   13. `cmd/maago/docs/model.txt`: `Post`에 `Cover string `+"`"+`maago:"media"`+"`"+` // a media key: maago.SweepMedia keeps its file while a row has it`(`TestDocKeysAreRead`). `cmd/maago/docs/main.txt`: `Note.Export`에 `maago:"media"`, `main_test.go`의 일꾼 뒤에 `maago.SweepMedia(maago.WithMedia(ctx, media), db, 0)`이 nil이고 그 뒤에도 `OpenMedia`가 `hello`(실제 `maago gen`의 생성물이 쿼리를 알림과 두 앱의 같은 글만 본다: 새 파일이라 후보가 없어 쿼리는 돌지 않는다. 생성된 쿼리를 진짜 DB에 돌리는 것은 gentest이고, 그 골든이 `build.Gen`의 것과 같음은 `TestGenMedia`). 주기적으로 부르는 `main.go`의 꼴은 9번
   14. 문서
       - `internal/ref/topics.go` form 절: "A file no row points at stays: delete it with maago.DeleteMedia when you delete the row (the admin does not); one saved by a request that failed before writing its row stays too." → "A file no row points at is deleted by maago.SweepMedia(ctx, db, 0), run now and then, once the fields that keep keys are tagged media (maago doc model); maago.DeleteMedia deletes a row's file at once." `API`에 `SweepMedia`(`TestPackageDoc`). 미디어 문단의 다시 쓰기(단일 인스턴스·서명된 URL·청소)는 9번
       - `doc.go`는 `-update`(PG 주소와 함께)
       - `docs/ARCHITECTURE.md`: 루트의 "M13이 더한 것"에 `SweepMedia(D-161)`, `rt` 목록에 `MediaInUse`, `rtcore`에 "생성 코드가 알린 미디어 쿼리"
       - `docs/TODO.md` 90줄 체크(`List`·`Delete(name)`을 `SweepMedia`가 쓴다)
       - `docs/DECISIONS.md` D-161에 고침 한 줄(아래 ★들)과 끝의 기록 줄
       - → 확인: `TestPackageDoc`, `TestDocExamples`, `TestNoDecisionsInUserText`, `TestGeneratedCallsAppAPI`(생성 주석의 `maago.SweepMedia`는 `topics.go`가 앱 코드로 센다)
   15. 게시판: 고치지 않는다(태그와 청소는 8번) → 확인: `TestBoardTutorial`(PG)·`TestBoardTutorialCatches`·`TestBoardMap`, `git diff --stat examples/`가 비어 있다

   단계와 확인
   1. 시험 먼저: `TestMediaTag`, `TestSweepMedia`, fixture와 gentest의 `models.go`·시험 파일 → 확인: `go vet ./...`가 없는 이름(`SweepMedia`, `Field.Media`, `rtcore.MediaQueries`, `s3fake.Touch`)으로 실패(빨강)
   2. 태그·생성기·`build.go`·`rt`·`rtcore`·`media.go`·`s3fake` → 확인: `go test ./internal/model ./internal/mapper ./internal/build .`, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_4 go test -run TestGenerated ./cmd/maago -update`(`-update` 뒤의 패키지는 시험 이진의 인자가 되므로 패키지가 먼저) 뒤 `go test ./cmd/maago/internal/gentest/...`(두 방언)
   3. 문서·예·골든 → 확인: `go test -run 'TestDoc|TestPackageDoc|TestNoDecisions|TestGeneratedCallsAppAPI' ./cmd/maago .`(`-update`는 PG 주소와 함께)
   4. 헛것(하나씩 넣고 이름 붙인 시험의 빨강을 보고 되돌린다. 생성물을 바꾸는 것은 PG 주소와 `-update` 뒤 gentest를 돌리고, 되돌린 뒤 다시 `-update`해 diff가 같음을 본다): (a) `MediaInUse`가 제 앱의 필드만 → `files`의 `TestSweep`(c가 지워진다) (b) `ModelsFile`이 `media` 필드가 있는 앱에만 `init` → `tags`의 `TestSweep` (c) `IS NOT NULL`을 뺌 → `files`의 `TestSweep`(NULL `Thumb`를 읽다 오류) (d) 후보의 시각을 보지 않음 → 72시간 줄 (e) 이름을 거르지 않고 모두 후보로 → `.gitignore`·`.tmp-x`가 지워지거나 S3 `a/b`의 `Delete` 오류 (f) `.tmp-`를 후보에서 뺌 → 오래된 `.tmp-`가 남음 (g) `List`의 첫 쪽만 → S3 줄(`.`로 시작하는 이름이 첫 쪽을 채워 고아 열쇠는 늘 뒤 쪽) (h) 1시간 하한을 지움 → `TestSweepMedia`·`files`의 30분 줄 (i) 다른 글 둘을 받음 → `TestSweepMedia` (j) 빈 `MediaQueries`를 받음(아무것도 쓰지 않는다고 보고 모두 지움) → `TestSweepMedia`의 남은 고아 (k) `media`가 `int64`를 받음 → `TestMediaTag` (l) `main.txt`의 `Export` 태그를 뺌 → `TestDocExamples/main`("knows no field") (m) `model.txt`의 `media`를 뺌 → `TestDocKeysAreRead/model` (n) 쿼리 오류를 빈 모음으로 받음(쓰는 중 없음으로 보고 오래된 파일을 모두 지움) → `TestSweepMedia`의 오류 `DB` 줄 (o) `build.go`가 `ModelsFile`에 제 앱만 넘김 → `TestGenMedia`(두 파일 다) (p) `Delete` 오류 뒤에도 계속 → `files`의 `Delete` 오류 줄. `checkGenerated`의 prepare는 안전띠라(지금 CRUD처럼) 헛것이 없다. `rows.Err()`는 읽다 끊기는 드라이버 가짜가 없어 헛것이 없다
   5. 끝 확인: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_4`로 전체(`-timeout 60m`, 앞에서 끝까지 기다려 로그의 `go test exit=` 줄), 게시판 걷기 포함. 새 시험은 `GOTOOLCHAIN=go1.25.14`로도

   ★ → 에이전트 편의로 결정
   - **쿼리는 생성 코드의 `init`이 `rt.MediaInUse`로 알리고, 모델이 있는 모든 앱이 프로젝트 전체의 같은 쿼리를 담는다.** `SweepMedia`는 앱 목록을 받지 않고 일꾼은 `Handler`를 지나지 않는다. 앱마다 제 열만 알리면, 또는 `Config.Apps`에 매달면, 앱 하나만 링크한 프로그램이 다른 앱의 살아 있는 파일을 조용히 지운다(Q4가 손 `inUse`를 기각한 바로 그 실패). 모든 앱이 전체를 담으면 어느 앱 하나만 링크해도 맞고, 하나도 없으면 오류, 글이 다르면(낡은 생성물) 오류다: 어느 쪽도 지우지 않는 쪽으로 실패한다. 에이전트는 태그 하나를 달고 `SweepMedia`를 부를 뿐 이을 것이 없다. 숨은 동작이 아니라 `models_gen.go`에 보이는 `init` 한 개다(PHILOSOPHY 5). 기각: `SweepMedia(ctx, db, olderThan, board.MediaInUse, …)`처럼 앱이 넘기기(빠뜨린 앱의 파일을 지운다), 태그 있는 앱에만 담기(태그 없는 앱만 링크한 일꾼이 쓸 수 없다, 헛것 (b))
   - **`SweepMedia(ctx, db, olderThan) error`: db는 인자, 저장소는 ctx**(D-161의 `(ctx, olderThan)`에 db를 더한다: D-161 고침). 생성된 쿼리·`outbox.Run`·`RunOnce`가 모두 db를 인자로 받고, `DBFrom`은 없으면 panic이며 "시험에서"라는 `WithDB`를 일꾼에 요구하게 된다. 저장소는 `SaveMediaFrom`·`DeleteMedia`처럼 ctx(`WithMedia`): 일꾼이 이미 그 ctx를 가진다. `maago.DB`라 `*sql.Conn`·`*sql.Tx`도 된다. 지운 수는 돌려주지 않는다(쓰는 곳 없음)
   - **`olderThan`은 0이면 24시간(Q4의 기본), 아니면 1시간 이상.** 작은 값은 저장과 행 쓰기 사이의 파일을 지운다(긴 일꾼은 파일 여럿을 저장한 뒤 행을 쓴다). `SweepMedia(ctx, db, 24)`(24나노초)처럼 단위를 빠뜨린 실수가 모든 새 파일을 지우는 대신 오류가 된다(PHILOSOPHY 6). 1시간도 가장 긴 일보다 짧으면 위험하므로 주석이 "the longest a file waits for its row"를 말한다
   - **지우는 것은 열쇠와 `.tmp-`+열쇠뿐**(Q4의 "`.tmp-*`"를 Maago가 만드는 꼴로 좁힘). 저장소에 넘기기 전에 런타임이 거른다: `.gitignore`, 사용자의 `.tmp-x`, S3의 남의 `a/b`는 저장소의 이름 오류에 기대지 않고 손대지 않는다. `.tmp-`+열쇠는 쓰는 중이어도(같은 열쇠의 행이 있어도) 여유 밖이면 지운다: `Put`이 이름을 바꾸기 전의 반쪽이다
   - **실패는 닫힌 쪽으로**: `List`의 한 쪽, 쿼리(마이그레이션을 적용하지 않아 없는 열 따위), `Delete` 어느 오류든 바로 돌려주고 그 뒤로 지우지 않는다. 다음 청소가 다시 한다
   - **후보를 고른 뒤 쓰는 중을 읽는다**, 쿼리는 열쇠 전부를 메모리에(`ponytail:` 주석). 앞에 읽으면 그사이 쓰인 행의 파일을 볼 수 없다. 후보가 없으면 DB를 읽지 않는다. 행 수가 커지면 후보 쪽마다 `IN`으로 묻는 길이 있다(지금은 하지 않음)
   - **`UNION ALL` 한 문장, 자리표시 없음**: 두 방언이 같은 글이고 `checkGenerated`가 모델 DB에 prepare한다(CRUD와 같은 안전띠)
   - **`s3fake.Touch`**: 오래된 객체와 `/`가 든 남의 이름을 가짜에 두는 시험 도우미 하나. 런타임에 시각을 주입하는 갈고리(시험만 쓰는 공개 이름)는 두지 않는다: 로컬은 `os.Chtimes`, S3 가짜는 `Touch`로 파일을 늙힌다
   - **진짜 DB의 시험은 gentest**(두 방언, 열쇠 열이 둘인 모델·두 앱·태그 없는 앱), 생성 경로 전체는 `TestDocExamples/main`(실제 `maago gen`과 복사된 outbox 앱). 뿌리 시험은 DB 없이 닿는 갈래만

   하지 않는 것
   - 태그를 빠뜨린 열쇠 필드를 생성 때 찾기(필드 이름으로 짐작하지 않는다, PHILOSOPHY 6; 태그가 하나도 없는 프로젝트는 `SweepMedia`의 오류가 말한다), 행 하나에 열쇠 여럿(JSON·글 속의 열쇠: 열 하나에 열쇠 하나, 8번의 "이름 → 열쇠 행"), 손 마이그레이션에만 있는 표의 열
   - `SweepMedia`를 스스로 주기적으로 부르기(`Handler`·일꾼 안), 지운 수나 목록의 반환, 시각 주입 갈고리, 후보 쪽마다의 `IN` 쿼리
   - 빈 DB나 다른 프로젝트의 DB를 준 청소를 알아보기: 표가 있고 행이 없으면 오래된 파일을 모두 지운다. 받아들이고 `SweepMedia`의 주석이 말한다("The store must be this database's alone". 표가 없으면 쿼리 오류로 멈춘다, 지키는 장치를 더하지 않음)
   - `main.go`가 청소를 주기적으로 부르는 예와 미디어 문단 다시 쓰기·D-140 고침(9번), 게시판의 태그와 청소(8번)

   계획 리뷰 codex·fable 반영: 막는 것은 `build.go`의 부름을 무는 확인(fable 1: `TestGenMedia`와 헛것 (o)). 받은 것: `main.txt`의 주장을 낮춤(fable 2: 늙힌 파일을 예에 넣으면 예가 길어지고, 생성된 쿼리는 gentest와 `TestGenMedia`가 문다), 오류 줄마다 제 글의 조각(fable 3), `Scan`·`rows.Err()`의 오류(fable 4, 헛것 없음), `testDB`(fable 5), 주석의 "this database's alone"(fable 6), 태그 글의 "Tag every field that keeps one"(fable 7), `Delete` 오류 줄(fable 8, 헛것 (p))과 로컬 `a/b`를 뺌(fable 8). codex 4(후보 쪽마다 `IN`)는 이미 미룬 대로(`ponytail:` 주석). 받지 않은 것(codex가 막는다고 한 셋): (1) DB와 쿼리의 프로젝트 식별자 — 가장 그럴듯한 실수는 같은 프로젝트의 스테이징 DB에 운영 버킷을 준 것이고 그 DB는 같은 식별자를 가져 장치가 잡지 못한다. fable은 같은 위험을 보고 주석만 청했다: 리뷰가 갈린 자리를 D-161(태그로 지킴)과 에이전트 편의(이을 것 없는 한 함수)로 주석 쪽으로 정한다 (2) 행 추가와 삭제의 경쟁에 advisory lock — 쓰는 중 읽기는 `UNION ALL` 한 문장이라 한 스냅숏을 본다(한 트랜잭션에서 열쇠를 다른 행으로 옮겨도 늘 보인다). 후보는 `olderThan` 넘게 묵은 파일이라, 그 열쇠를 처음 가리키는 행은 주석의 계약("longer than the longest a file waits for its row")이 이미 막는다. SQLite에는 advisory lock이 없고 손 쿼리의 쓰기를 덮지도 못한다 (3) `.tmp-`의 lease — 저장소에 `.tmp-<key>`를 만드는 것은 `LocalMedia.Put`뿐이고(S3의 `Put`은 OS 임시 디렉터리에 받는다) 쓰는 동안 mtime이 움직인다. 멈춘 `Put`의 `.tmp-`를 지우면 `Rename`이 실패해 `Put`이 오류를 돌려주고(`media.go`의 `LocalMedia.Put`) 행은 쓰이지 않는다: 조용히 잃는 것이 없다
   기록(2026-11-25): 태그 `media`(`ref.FieldTags`, `model.Field.Media`, 문자열만), `mapper.MediaInUse`(앱·모델·필드 차례의 `UNION ALL`, 늘 `IS NOT NULL`), `ModelsFile`이 모든 앱을 받아 모델이 있는 앱마다 `init`의 `rt.MediaInUse`, `build.go`의 `s.apps`와 `checkGenerated`의 prepare, `rtcore.MediaQueries`(`app.go`), `maago.SweepMedia`, `MediaStore.Delete` 주석, `s3fake.Touch`(`sync.Map`), fixture `internal/testdata/media`와 gentest `files`·`tags`(두 방언), `TestGenMedia`·`TestSweepMedia`·`TestMediaTag`, `model.txt`의 `Cover`, `main.txt`의 `Export` 태그와 청소, form 절과 `doc.go`, ARCHITECTURE·TODO·D-161 고침과 기록 줄, `TUTORIAL-FUTURE.md`의 허용 목록(전체 시험의 `TestShortTutorial`이 잡음). 헛것 (a)~(p) 모두 이름 붙인 시험이 빨강(되돌린 뒤 같은 파일, 생성물을 바꾸는 (a)(b)(c)는 PG 주소와 `-update` 뒤 gentest 두 방언이 빨강이고 되돌려 다시 `-update`한 diff가 비어 있음): (a) `files`의 `TestSweep`(다른 앱의 n이 지워짐)과 `tags`의 오류 (b) `tags`의 `TestSweep`("knows no field") (c) NULL `Thumb`의 `Scan` 오류 (d) 72시간 줄 (e) 로컬은 `.gitignore`·`.tmp-x`가 지워지고 S3는 `a/b`의 이름 오류 (f) 오래된 `.tmp-`가 남음 (g) S3 줄만 (h) `TestSweepMedia`의 -1·30분과 `files`의 30분 줄 (i)(j)(n) `TestSweepMedia`(j·n은 고아가 지워짐) (k) `TestMediaTag` (l) `TestDocExamples/main`("knows no field") (m) `TestDocKeysAreRead/model` (o) `TestGenMedia`의 `files`·`tags` 둘 (p) `Delete` 오류 줄(2번 불림). 첫 단계의 "시험 먼저 빨강"은 이 실행에서 다시 보지 않았고 헛것이 그 증거다. 계획 글을 구현에 맞춤: `rtcore`의 파일, `testDB`, `sync.Map`, `tags`의 시험, `-update`는 패키지 뒤. 새 시험은 `GOTOOLCHAIN=go1.25.14`로도 초록. `MAAGO_TEST_PG_URL`로 전체 시험 `go test exit=0`(cmd/maago 383초), `go mod tidy -diff`·`go vet`·`gofmt -l .` 깨끗, `examples/` 그대로
   기록(2026-11-25, 구현 리뷰 반영): 구현 리뷰는 ponytail·codex·fable: 막는 것 없음(셋 다). 받은 것: `files`의 `TestSweep`에 2시간 전의 고아 `h`가 `SweepMedia(ctx, db, 0)` 뒤에도 남음(fable 1, 기본 24시간을 1시간으로 바꾸면 빨강), 오래된 `.tmp-`를 쓰는 중인 열쇠 `a`의 것으로(fable 2, "`.tmp-`를 뗀 열쇠가 쓰는 중이면 남김"으로 바꾸면 빨강), `TestSweepMedia`에 `time.Hour`의 "no driver" 줄(하한이 1시간을 받음, `<`를 `<=`로 바꾸면 빨강)과 빈 디렉터리·`testDB`의 nil(후보가 없으면 DB를 읽지 않음, 그 갈래를 지우면 빨강)(fable 4, `rows.Err()`는 그대로 헛것 없음), 헛것 (q) 겹침 거르기(`rt/app.go`)를 뺌 → `TestDocExamples/main`의 "different media fields"(fable 3, fable이 복사본에서 봄), D-161 기록 줄의 날짜 11-25(fable 5), 새 열쇠의 `.tmp-`를 상수 `".tmp-"+strings.Repeat("f", 32)`로 하고 `newKey`·`crypto/rand`·`encoding/hex`를 지움(ponytail 2), `files`의 30분·저장소 없음 줄을 지움(ponytail 3: 30분은 `TestSweepMedia`, 저장소 없음은 `mediaStore`의 기존 오류를 `TestMedia`가 문다; 그래서 헛것 (h)는 이제 `TestSweepMedia`의 30분 줄만), `TestSweepMedia`의 -1 줄을 지움(ponytail 4, 30분과 같은 갈래), `switch len(...)`의 빈 `case 1`을 두 `if`로(ponytail 5). 받지 않은 것: `s3fake`의 `sync.Map`(ponytail 1, 남의 이름 `a/b`와 오래된 객체를 심을 길이 이것뿐이고 `New`의 반환을 바꾸면 `contrib/s3`의 호출 다섯을 고친다: ponytail도 받아들일 만하다고 함), 낡은 바이너리의 청소 글(fable 6, 9번의 미디어 문단으로 TODO), codex(지적 없음, 쓰기 금지 환경이라 실행은 못 함). 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=…/m13_4`로 PG 전체 시험 `go test exit=0`(cmd/maago 384초, 게시판 걷기 포함)

8. **게시판 0041~**: 일꾼이 만든 파일 여럿(내보내기 따위, 이미지 처리 아님: D-72)을 이름 → 열쇠 행으로 저장하고 뷰가 이름으로 찾아 준다(readmate의 `media/marked/<id>/` 꼴), 저장소를 환경으로 고름, 서명된 URL로 밖에 건넴, 청소(태그), outbox의 Kind 둘(웹훅과 이 일: `main`의 일꾼이 Kind로 가름), lease를 넘어 다시 도는 일이 남긴 파일은 청소가 치운다 → 확인: `TestBoardTutorial`·`TestBoardTutorialCatches`·`TestBoardMap`, Catches(일꾼 ctx에 저장소 없음, 태그 빠뜨림 따위)

   계획

   따라가 본 길: 0040의 끝(풀어 본 게시판)은 `main`이 `MediaDir: "media"`를 `Config`에 주고 `outbox.Run(work, db, board.Deliver)`를 띄우며, `Deliver`는 제 Kind(`postPublished`, 내보내지 않음)만 받는다. 첨부는 `Attachment.Key`(`unique`, 태그 없음), `Attach`가 행 실패 때 `DeleteMedia`, `deletePost`가 `AfterCommit`으로 파일을 지운다. 시험의 설정은 `web_test.go:39`의 `config(t, db)`이 `MediaDir: t.TempDir()`이고 `attach_test.go`가 `cfg.MediaDir`을 세 곳(79·97·148)에서 읽는다. 걷기(`cmd/maago/tutorial_test.go`)는 단계마다 gen·check·migrate·`go test ./...`이고, 늘 **마지막 단계**의 `main`을 `ship`한다(PG에서 일꾼이 웹훅을 보내는지). 게시판은 다른 모듈이라 앱 시험은 `internal/s3fake`·`internal/mediatest`를 import할 수 없다: S3 길은 `cmd/maago`의 `ship`만 걸을 수 있고(같은 모듈이라 `s3fake.New`·`Touch`를 쓴다) 앱 시험은 `LocalMedia`와 `os.Chtimes`로 늙힌다. `SweepMedia`는 여유(1시간 이상) 밖의 파일만 보고 후보가 없으면 DB를 읽지 않으므로 늙히지 않은 시험은 아무것도 보이지 않는다. `maago doc main`의 예는 main의 `deliver(db, media)`가 닫힘의 첫 줄에서 `WithMedia`를 하고 Kind를 본다(9번이 Kind 둘로 다시 쓴다). Catches의 꼬리 줄 넷(0033 "no migrations at the start"·"another port", 0040 "no graceful stop"·"no worker")은 뒤 단계를 모두 걷으므로, 뒤 패치의 덩어리가 그 줄을 바꾸거나 문맥으로 가지면 바꾼 복사본에서 뒤 패치의 `git apply`가 실패한다(M12 10번의 "no graceful stop"을 옮긴 일).

   풀어 본 게시판(scratchpad, PG `m13_8`의 스키마 하나)에서 손으로 재 본 것(2026-11-25):
   - 태그만 더하면(`Key string maago:"unique,media"`) `makemigrations`는 `no changes`이고, gen 뒤 모델이 있는 앱 셋(board·auth·outbox)의 `models_gen.go`에 같은 `rt.MediaInUse` 글이 생긴다
   - 새 모델의 표 이름은 `board_export_file`(`board_exportfile`로 짐작해 쓴 쿼리 셋은 "run makemigrations"와 함께 한 실행에 `relation "board_exportfile" does not exist … (checked against the models)`: 3번의 쿼리 계속이 돈다). 마이그레이션은 `0008_export_files.sql`
   - 내보내기 일꾼(파일을 먼저 저장하고 `WithTx`에서 그 게시판의 행을 지우고 새로 넣음)을 두 번 돌리고(lease를 넘어 다시 도는 일) 모든 파일을 48시간 늙혀 `SweepMedia(ctx, db, 0)` → 남은 파일이 행의 열쇠와 같다. `ExportFile.Key`의 태그만 빼면(첨부의 태그는 남김) 같은 시험이 빨강(쓰는 파일이 조용히 지워진다: Catches로 둘 값이 있는 꼴)
   - main이 `contrib/s3`를 import해도 게시판의 `go.mod`(maago v0.3.0)와 `go.work`, `GOPROXY=off`·`-mod=readonly`로 빌드·vet 통과(공개 판의 `go.mod`는 10번)
   - `LocalMedia{Dir: "media", SiteURL: auth.SiteURL + "/app"}`: `BOARD_SITE_URL=http://localhost:8000`이면 뜬다. `http://localhost:8000/`이면 auth는 받는데 `Handler`가 `Config.Media's SiteURL "http://localhost:8000//app" has path "//app", not Config.Prefix "/app"`로 멈춘다(아래 불편 1)
   - `BOARD_S3_BUCKET`만 주고 띄우면 기동 때 `media.List(ctx, "")`가 `s3: Store.Endpoint is empty`로 멈춘다
   - 모델·쿼리·일꾼·`PostPublished` 이름 바꾸기를 넣은 채 게시판의 `go test ./...`(auth·board·outbox) 초록: admin이 새 모델을 받아도 있는 시험이 깨지지 않는다

   단계 넷(패치, 글은 영어, `D-xx` 없음. 이름은 고칠 수 있다)
   1. **0041 `Sweep the files no row keeps`**: `Attachment.Key`에 `media`(`maago:"unique,media"`, 마이그레이션 없음). main은 `MediaDir: "media"` 대신 `media := maago.LocalMedia{Dir: "media"}`를 `Config.Media`에, 일꾼 고루틴 바로 뒤(`err = maago.Serve(…)`에서 네 줄 넘게 떨어지게: 아래 꼬리 줄의 규칙. 그 사이의 주석 "Serve stops on SIGTERM… finishes the one in hand." 세 줄은 건드리지 않고 청소의 설명은 고루틴 위 제 주석에)에 청소 고루틴: 시작할 때와 그 뒤 24시간마다 `maago.SweepMedia(maago.WithMedia(work, media), db, 0)`, 오류는 로그, `work`가 끝나면 멈춘다. 글: 커밋에 실패한 첨부·죽은 프로세스의 반쪽은 하루 뒤 청소가, `Attach`·`deletePost`의 즉시 지우기는 그대로. 시험의 `config(t, db)`은 main과 함께 `Media: maago.LocalMedia{Dir: t.TempDir()}`로 바꾸고 `attach_test.go`의 세 곳(79·97·148)은 그 `Dir`을 읽는다. 시험 `attach_test.go`의 `TestSweep`: 올린 첨부 하나와 `SaveMediaFrom`으로 둔 고아 하나를 **둘 다** 늙혀 청소 → 첨부는 열리고 고아는 없다
   2. **0042 `Export a board in the background`**: 모델 `ExportFile{ID; BoardID fk=Board; Name; Key unique,media; Type; Size; CreatedAt auto=create}`, `unique=(BoardID,Name)`(마이그레이션 `--name export_files`). 쿼리 `board_files.sql`(`BoardFiles`, 게시판의 파일, 이름 차례)·`board_file.sql`(`BoardFile`, 게시판과 이름)·`delete_board_files.sql`(`DeleteBoardFiles`, `:exec`). `board/files.go`
      - `const ExportFiles = "export_files"`(있는 CSV 내보내기의 라우트 `board_export`·`ExportBoard`와 다른 말), `postPublished`는 `PostPublished`로 내보낸다(main이 Kind로 가른다)
      - `WriteExportFiles(ctx, db, j outbox.Job) error`: 페이로드 `{"id": 게시판}`, **누구에게나 나간 글만**(`ExportRows`·`GetPost`에 사용자 nil, 쪽마다) `post-<ID>.md`(제목과 본문)와 `index.md`(목록, 글이 없어도 하나)를 `SaveMediaFrom`으로 먼저 저장하고, `WithTx`에서 그 게시판의 행을 지우고 새 행을 넣는다. 앞 내보내기의 파일, 반쯤 돈 실행과 lease를 넘어 다시 돈 실행의 파일은 어느 행도 갖지 않아 청소가 치운다(지우는 코드 없음)
      - 라우트(staff만, 아니면 403: `u auth.User`이라 익명은 로그인으로): `GET /boards/{id}/files`(`board_files`, 뷰 `ShowFiles`, 파일 목록과 "다시 내보내기" 폼, 새 템플릿 `files_page.html`), `POST /boards/{id}/files`(`board_files_export`, 뷰 `ExportFilesAgain`, 같은 tx에서 `outbox.Enqueue(ctx, tx, ExportFiles, …)` 뒤 SeeOther), `GET /boards/{id}/files/{name}`(`board_file`, 쓰는 뷰 `DownloadFile`: `BoardFile` → `ServeMedia`, 없으면 404)
      - main: 일꾼의 handle을 `main.go` 끝의 이름 있는 함수 `deliver(db maago.DB, media maago.MediaStore) func(context.Context, outbox.Job) error`로(`outbox.Run(work, db, deliver(db, media))`): 닫힘의 첫 줄 `ctx = maago.WithMedia(ctx, media)`, `switch j.Kind`로 `board.PostPublished` → `board.Deliver`, `board.ExportFiles` → `board.WriteExportFiles(ctx, db, j)`, 그 밖은 오류(일이 남아 admin에 보인다). `maago doc main`의 `deliver(db, media)`와 이름·꼴이 같다(Kind 둘은 9번이 문서에)
      - 시험 `files_test.go`: `TestWriteExportFiles`(재 본 것: 초안은 파일이 없다, 두 번 돌린 뒤 늙혀 청소하면 행의 파일만 남고 내용이 맞다), 웹 시험 하나(staff가 POST → `outbox.RunOnce(ctx, db, now, …)`(시험이 `WithMedia`를 한 handle) → 목록에 이름, 이름으로 내려받기의 본문·`attachment`, 없는 이름 404). `TestPermissions`에 새 라우트 셋의 행
   3. **0043 `Keep the files in a bucket`**: main이 `BOARD_S3_BUCKET`이 있으면 `s3.Store{Endpoint, Region, Bucket, AccessKeyID, SecretAccessKey}`를 `BOARD_S3_ENDPOINT`·`BOARD_S3_REGION`·`BOARD_S3_BUCKET`·`BOARD_S3_ACCESS_KEY_ID`·`BOARD_S3_SECRET_ACCESS_KEY`로 채우고 없으면 `LocalMedia`. 0041의 `media := maago.LocalMedia{Dir: "media"}` 줄은 여기서 `var media maago.MediaStore = maago.LocalMedia{Dir: "media"}`로 다시 쓰고, 그 줄과 `media = s3.Store{` 줄 사이에 주석을 세 줄 넘게 둔다(0044가 앞 줄에 `SiteURL`을 더해도 "the bucket ignored" 줄이 0044 덩어리의 문맥 밖: 단계마다 새 것 하나). 기동 때 `media.List(ctx, "")` 한 번(실패하면 `log.Fatal`: 빈 칸·틀린 열쇠·없는 버킷이 첫 파일이 아니라 여기서 칸 이름을 말한다). 글: 서버가 여럿이면 버킷(디스크는 한 대의 것), R2·MinIO도 같은 칸, 0033의 "바이너리 밖은 DB와 `media/`뿐"이 버킷이면 DB와 버킷이라는 한 줄. Dockerfile은 그대로(기본은 볼륨의 `media/`)
   4. **0044 `Hand the files out by signed URLs`**: `GET /api/boards/{id}/files`(`api_board_files`, `maago.API`, staff만): 파일마다 `{Name, Type, Size, URL}`, URL은 권한을 본 **뒤** `maago.MediaURL(ctx, f.media(), time.Hour)`. 다른 서비스(프런트엔드, 파일을 받아 갈 서비스)가 앱을 지나지 않고 받는다(S3면 버킷에서 바로). main은 로컬 줄에 `SiteURL: strings.TrimSuffix(auth.SiteURL, "/") + "/app"`만 더한다(구현 리뷰 codex: auth는 끝 `/`를 받는다)(0043의 거리 두기로 "the bucket ignored" 줄의 문맥 밖). 글: 로컬 URL은 그 프로세스만 받고 다시 시작하면 403. 시험의 `config(t, db)`은 0041의 `LocalMedia`에 `SiteURL: "http://example.com"`만 더한다(URL을 만들려면). `TestUnderApp`(`web_test.go:547`, `Prefix: "/app"`)은 `SiteURL`의 경로가 Prefix와 달라 `Handler`가 거절하므로 그 시험의 `Media`도 `SiteURL: "http://example.com/app"`으로(불편 1이 시험에서도 보인다). 웹 시험: `httptest` 서버를 `SiteURL`로 한 Handler에서 staff 토큰으로 목록을 받아 URL마다 `http.Get`이 200·본문·`attachment`. `TestPermissions`에 `api_board_files` 행(익명 401, staff 아닌 역할 403, staff 200)

   바꿀 파일과 새 파일
   - `examples/board/steps/0041-…`~`0044-…`(새 패치 넷, 풀어서 커밋을 더한 뒤 `steps.sh pack`)
   - `examples/board/MAP.md`: 둘째 표에 0041~0044 행. 원본 50·52(파일 업로드) 행의 게시판 칸은 그대로(원본은 앱이 만든 파일이 없다)
   - `cmd/maago/tutorial_test.go`
     - `ship`(PG 갈래): `s3fake.New(t)`의 칸을 `BOARD_S3_*`로 환경에, 기동 전 `s3fake.Touch`로 48시간 전의 고아 열쇠 하나와 SQL로 게시판 하나·그 `board_export` 일 하나. 모르는 Kind(`nope`)의 일 하나도. 그리고 `serve`와 따로 새 도우미 `refuse(root, bin, env, want)`: 같은 바이너리를 `BOARD_SITE_URL`·`PORT=0`·`BOARD_S3_BUCKET`만 준 환경으로 돌려 10초 안에 **스스로** 0 아닌 상태로 끝나고(Kill 전), 출력에 `s3: Store.Endpoint is empty`가 있고 `listening on`이 없음을 본다(기동 때의 `List`: 청소 고루틴의 로그 글과 같은 글이라 스스로 끝남과 `listening on`까지 본다). 오류는 `board with BOARD_S3_BUCKET alone: `로 시작한다(`board `라 Catches의 꼬리 줄). `use`는 웹훅을 기다린 뒤 10초까지 셋을 기다리고 오류 글은 저마다: `board_export_file`에 그 게시판의 `index.md` 행이 있고 그 열쇠가 가짜의 `List`에 있음("no export file …"), 늙힌 고아가 없어짐("the orphan … is still in the bucket"), `nope` 일이 `last_error`와 함께 남음("the job of kind nope …": `deliver`의 "그 밖은 오류"). 주석의 줄글에 버킷·청소를 더한다
     - Catches 새 줄(앞 셋은 `go test: `, 뒤 셋은 main에만 있는 잘못이라 `serve: ` 꼬리 줄): "an untagged attachment key"(0041, 태그 하나도 없음 → `SweepMedia`의 "knows no field"), "an untagged export file"(0042, 첨부는 태그를 가진 채 `ExportFile.Key`만: 살아 있는 파일을 조용히 지운다), "a URL to anybody"(0044, staff 검사를 지움 → `TestPermissions`), "no sweep"(청소 고루틴의 부름을 지움 → 고아가 남음), "no store in the worker"(handle의 `WithMedia` 줄 → 내보내기 파일이 생기지 않음), "the bucket ignored"(0043, `media = s3.Store{…}`를 버림 → 로컬로 떠 `refuse`가 문다). 꼬리 줄의 `old`는 그 줄을 마지막으로 쓰는 패치에 둔다. 꼬리 줄 셋의 `by`는 접두만이 아니라 제 확인의 글까지(`serve: the orphan`, `serve: no export file`, `board with BOARD_S3_BUCKET alone: `): 다른 확인이 먼저 물어 초록이 되는 헛것을 막는다
     - **꼬리 줄의 규칙**: Catches는 패치 글의 `old`를 바꾸므로 줄은 그 줄을 `+`로 더하는 패치에만 설 수 있고, 그 뒤 어느 패치의 덩어리도 그 줄의 세 줄 안을 바꾸거나 문맥으로 갖지 않아야 한다(아니면 복사본이 뒤 패치의 `git apply`에서 깨진다). 그래서 main의 덩어리 자리: 청소 고루틴은 일꾼 고루틴 바로 뒤, `deliver`는 `main.go`의 끝, 저장소 고르기는 `Handler` 앞의 한 덩어리(0043이 쓰고 0044는 로컬 줄에 `SiteURL`만: 그 줄과 `media = s3.Store{` 줄 사이에 주석 네 줄, 구현 리뷰 뒤에는 `where` 줄과 주석 세 줄과 `if`)
     - 옮길 줄: "no worker"(0042가 `outbox.Run(work, db, board.Deliver)`를 `outbox.Run(work, db, deliver(db, media))`로 바꾼다 → 0042의 그 줄로). "no graceful stop"은 0040에 그대로(청소 고루틴을 네 줄 넘게 떨어뜨려 0041이 그 줄을 문맥으로 갖지 않게). "another port"·"no migrations at the start"는 패치를 싼 뒤 Catches로 본다(깨지면 같은 규칙으로 옮긴다)
   - `docs/TODO.md` `## M13`: 아래 "쓰기 불편한 곳" 중 고치지 않는 것
   - `docs/TASKS-M13.md`: 기록 줄

   단계와 확인
   1. 풀어 쓰기: `examples/board/steps.sh unpack <scratch>`, `.env`를 `MAAGO_TEST_PG_URL`의 데이터베이스 `m13_8`의 스키마로 돌리고 0041~0044를 커밋마다 `maago gen`·`makemigrations`·`migrate`·`go test ./...`(게시판의 `go.work`로), `steps.sh pack` → 확인: `git status`에 새 패치 넷과 바뀐 것 없음(0001~0040은 `--zero-commit`이라 그대로), 패치에 `D-\d`가 없다
   2. 걷기 → 확인: `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_8 go test -run 'TestBoardTutorial$|TestBoardMap' -timeout 60m ./cmd/maago`
   3. Catches → 확인: 같은 주소로 `-run TestBoardTutorialCatches -v`, 새 여섯 줄과 옮긴 줄이 PASS이고 줄마다 실패가 제 접두(`walk: <nil>`이 아님)
   4. 헛것(하나씩 넣고 빨강을 보고 되돌린다): (a) `ship`의 고아 기다림을 지움 → "no sweep" 줄이 `walk: <nil>` (b) `ship`이 S3 환경을 주지 않음 → `TestBoardTutorial`이 빨강(가짜에 열쇠 없음) (b') `refuse`를 부르지 않음 → "the bucket ignored" 줄이 `walk: <nil>` (c) `TestWriteExport`가 늙히지 않음 → "an untagged export file" 줄이 `walk: <nil>`(시험이 문다는 증거) (d) `ship`의 내보내기 행 기다림을 지움 → "no store in the worker"가 `walk: <nil>` (e) 풀어 둔 게시판의 main에서 기동 때의 `media.List` 줄을 지우고 패치를 싸 `TestBoardTutorial` → 깨진 S3 환경의 실행이 멈추지 않아 `ship`이 빨강(Catches 줄은 두지 않는다: 꼬리 줄 하나가 단계 넷과 `ship`을 다시 걷는 값에 비해 `ship`의 한 실행이 이미 문다) (f) 0044 끝에서 `Attachment.Key`의 `media`만 뺌(`ExportFile.Key`는 남김) → `TestSweep`이 빨강(첨부가 지워짐: D-161의 본뜻, 열쇠 열이 늘 때) (g) `deliver`의 `default`가 `nil` → `TestBoardTutorial` 빨강(`nope` 일이 사라짐)
   5. 끝 확인: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .` 비어 있음, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_8`로 전체(`-timeout 60m`, 앞에서 끝까지 기다려 로그의 `go test exit=` 줄). 시간: 1번의 기준(로컬 Catches 8분, `cmd/maago` 12분) 아래. 가늠: 걷기 한 번에 단계 넷 약 10초, Catches는 꼬리 줄이 일곱(있던 넷 + 셋)으로 늘어 줄마다 단계 넷과 `ship`을 더 걸어 2~3분 더(229초 → 약 6분), `cmd/maago` 약 7~9분. 잰 값을 기록에

   ★ → 에이전트 편의로 결정
   - **단계는 넷, 청소가 먼저.** 청소가 있으면 내보내기 일꾼은 파일을 지우는 코드가 없다: 앞 내보내기의 파일, 반쯤 돈 실행, lease를 넘어 다시 돈 실행이 모두 "어느 행도 갖지 않는 파일"이라 한 길로 치운다(일꾼마다 `DeleteMedia`의 갈래를 쓰지 않는다). 단계마다 새 것 하나라 게이트(11번)가 글 하나씩 따른다. 첨부의 즉시 지우기(`Attach`·`deletePost`)는 고치지 않는다
   - **내보내기는 누구에게나 나간 글만**(사용자 nil). 요청은 staff가 하지만 파일은 URL로 밖에 나가므로 초안이 새지 않게 일꾼이 가장 좁은 사용자로 읽는다. 권한은 파일을 건네는 뷰마다 staff(목록·이름으로 내려받기·URL). 0029의 "파일의 권한은 뷰가 본다"와 같은 자리
   - **이름 → 열쇠는 게시판의 최신 내보내기 하나**(`unique=(BoardID,Name)`, 행을 통째로 바꿈): readmate의 `media/marked/<id>/` 꼴이 이름으로 찾는 것이고 판을 쌓으면 목록·청소의 뜻이 하나 더 는다. 두 실행이 겹치면(lease를 넘음) 나중 실행의 삽입이 unique 위반으로 실패하고 일이 다시 돈다(시험 없음: PG READ COMMITTED의 추론)
   - **Kind는 main의 닫힘이 가르고 `WithMedia`도 거기 한 줄**: 과제의 "main의 일꾼이 Kind로 가름"이고 `maago doc main`의 `deliver(db, media)`와 같은 꼴이라 에이전트가 문서의 예를 그대로 옮긴다. 그래서 일꾼 ctx의 저장소는 앱 시험이 아니라 `ship`이 문다(앱 시험은 handle을 제가 만든다)
   - **저장소의 환경은 `BOARD_S3_*` 다섯, 버킷 이름이 있으면 S3.** 칸 이름 그대로라 한 대 한이고 `BOARD_WEBHOOK_*`와 같은 접두. 기동 때의 확인은 빈 칸 검사 다섯 대신 `media.List(ctx, "")` 한 줄: `contrib/s3`의 칸 검사(`s3: Store.Endpoint is empty`)를 그대로 쓰고 틀린 비밀 열쇠·없는 버킷까지 기동 때 멈춘다. 대가: 버킷이 내려가 있으면 서버가 뜨지 않는다(DB의 `Migrate`와 같다). 6번 ★의 "빈 값을 기동 때 거절하는 꼴"을 이것으로 보인다
   - **밖에 건네는 길은 API 하나**(`GET /api/boards/{id}/files`, 1시간). 웹훅에 URL을 싣는 길은 Kind가 하나 더 늘고 URL의 기한이 일의 재시도와 엉킨다. ttl 1시간은 받아 가는 쪽의 한 번 내려받기에 넉넉하고 7일 상한과 멀다
   - **청소는 main의 고루틴**(시작할 때와 하루마다, 일꾼과 같은 `work`로 멈춤). 플래그와 cron은 배포마다 이음새가 하나 더 든다. 인스턴스가 여럿이면 저마다 돌지만 지우기는 되풀이해도 같다. `ship`이 늙힌 고아가 기동 뒤 없어짐을 본다. 대가: 7번 계획 리뷰가 꼽은 가장 그럴듯한 실수(스테이징 DB에 운영 버킷)가 일부러 부른 청소가 아니라 **기동하자마자** 운영 파일을 지운다. 받는다: 그 실수는 부르는 때와 관계없이 하루 안에 같은 것을 지우고, `SweepMedia`의 주석("The store must be this database's alone")과 0041·0043의 글 한 줄(a bucket is one site's: never point a staging copy at production's)이 말한다. codex의 "DB와 저장소의 환경 식별자가 다르면 기동 실패"는 받지 않는다: D-161이 "저장소는 그 DB만의 것"을 주석의 계약으로 정했고(fable도 이 대가가 그와 맞다고 봄), 식별자는 런타임의 새 계약이라 이 작업(런타임을 고치지 않음) 밖이다. 청소 고루틴의 끝을 기다리는 `done`(codex)도 받지 않는다: 중간에 멈춘 청소는 어느 행도 갖지 않는 파일만 덜 지울 뿐 잃는 것이 없고 다음 기동이 다시 치운다. 기다리려면 "no graceful stop" 줄 옆의 끝 덩어리를 고쳐야 한다
   - **`ship`은 S3(가짜)로 띄운다**: 환경 → `s3.Store` → 일꾼의 `WithMedia` → `SaveMediaFrom` → 생성된 쓰는 중 쿼리(PG) → 청소가 내보내는 바이너리 하나에서 이어진다. 로컬 갈래는 `refuse` 뒤, 일을 넣기 전에 버킷 없이 끝 `/`가 있는 `BOARD_SITE_URL`로 한 번 더 `serve`한다(구현 리뷰 fable 3: main의 로컬 줄을 `/nope`으로 바꿔도 초록이었다)
   - 결정 기록 없음: 게시판 단계의 선택이고 D-160·D-161의 뜻 그대로(D-142처럼 단계를 더하는 결정은 이 과제가 이미 정했다). 고침이 생기면 D-160·D-161에 한 줄

   쓰기 불편한 곳(이 작업의 값진 결과: 작으면 고치고 아니면 TODO `## M13`)
   1. **`SiteURL` 둘의 뜻이 다르다**: 복사된 auth의 `auth.SiteURL`은 경로가 없어야 하고(`siteURL`이 거절) `LocalMedia.SiteURL`은 `Config.Prefix`를 담아야 한다. 게시판은 `auth.SiteURL + "/app"`이라 쓰고, 끝 `/`가 있는 주소는 auth가 받는데 `//app`으로 `Handler`가 멈춘다(재 봄). 멈춤과 글은 맞으므로 조용히 틀리지는 않는다. 고치는 길(로컬 URL이 `rtcore`의 Prefix를 붙이고 `SiteURL`은 주소만)은 D-160의 4번 고침을 뒤집는 공개 계약이라 이 작업에서 하지 않고 TODO(9번이나 M14에서 사람에게: 두 꼴과 대가)
   2. **없는 표의 진단이 모델의 표 이름을 말하지 않는다**: `board_exportfile`로 짐작한 쿼리에 `relation … does not exist`뿐이고 같은 실행의 "table "board_export_file" is missing"을 읽어야 안다(재 봄). did you mean은 M14 진단 묶음의 꼴이라 TODO
   3. 구현에서 더 찾는 것(예상): `config(t)`의 `MediaDir` → `Media`로 바꾸며 시험이 `Dir`을 다시 읽는 꼴, 일꾼 ctx에 저장소를 빠뜨렸을 때의 오류가 admin의 `LastError`에만 남는 것, `MediaURL`의 오류가 시험에서 500으로만 보이는 것. 기록 줄과 TODO에 적는다

   계획 리뷰 codex·fable 반영: 기동 실행을 `serve`와 따로 `refuse` 도우미로(`BOARD_SITE_URL`·`PORT=0`, 스스로 끝남·`listening on` 없음까지, codex·fable), 꼬리 줄의 `by`를 제 확인의 글까지(fable 막는 것), 헛것 (b)를 고치고 (b')·(f)·(g)를 더함, `TestSweep`은 첨부와 고아를 둘 다 늙힘, `ship`에 모르는 Kind의 일, 0043은 로컬 줄과 S3 줄 사이를 주석으로 떼고 `SiteURL`은 0044가, 0041은 Serve 위 주석 세 줄을 건드리지 않음, Kind는 `ExportFiles`(CSV의 `board_export`와 다른 말)·쿼리 `BoardFiles`·`BoardFile`·`DeleteBoardFiles`, 시험의 `config`는 0041에서 `Media`로, 겹친 실행의 unique 위반은 시험 없는 추론이라 표시(fable). 받지 않은 것: 환경 식별자와 청소 고루틴의 `done`(codex, 까닭은 위 ★ 청소)
   구현 리뷰 반영(ponytail·codex·fable, 막는 것은 codex 하나), 모두 고침: 끝 `/`의 `BOARD_SITE_URL`에서 기동 실패(codex 막음) → 0044 main이 `strings.TrimSuffix(auth.SiteURL, "/") + "/app"`. 그 줄을 걷기가 띄우지 않음(fable 3) → `ship`이 `refuse` 뒤·일을 넣기 전에 버킷 없이 `BOARD_SITE_URL=http://localhost:8000/`로 `serve`(오류는 `board with no bucket: `, 일을 넣은 뒤면 그 일꾼이 일을 가져간다). 쪽 넘김을 시험이 물지 않음(fable 1) → `TestWriteExportFiles`가 게시된 글 둘에 `exportPage = 1`. 뷰의 Kind를 아무도 보지 않음(fable 2) → `TestFiles`의 handle이 `ExportFiles`가 아니면 오류. 버킷 이름을 틀리면 조용히 로컬(fable 4) → 0043 main이 기동 때 `files in bucket <이름>`이나 `files in media/`를 로그에. `APIFiles`의 `GetBoard`에 시험 없음(fable 5) → `TestFileURLs`에 없는 게시판의 404. `Deliver`의 Kind 검사와 그 시험 줄이 main의 `switch`와 겹침(ponytail 1, `web_test`의 `RunOnce(…, Deliver)`는 `PostPublished`만 만난다) → 0042에서 지움. 0043의 버킷 주석이 0041 청소 주석의 경고를 되풀이(ponytail 2) → 주석에서 지움(0043 커밋 글의 한 줄은 둠, `var media`와 `media = s3.Store{` 사이는 여전히 세 줄 넘게). `shipped`가 폴링마다 `withPostgres`를 두 번(ponytail 3) → 한 번. 쓰이지 않는 `ID: 1`(ponytail 4) → 뺌
   기록(2026-11-25): 단계 넷(0041 `Sweep the files no row keeps`, 0042 `Export a board in the background`, 0043 `Keep the files in a bucket`, 0044 `Hand the files out by signed URLs`), `MAP.md`의 0041~0044 행, `tutorial_test.go`의 `ship`(가짜 S3의 칸을 `BOARD_S3_*`로, 48시간 전의 고아, 게시판 하나와 `export_files`·`nope` 일)·`shipped`(차례: `nope`의 `last_error` → 내보내기 행과 버킷의 열쇠 → 고아 없음, 글마다 제 접두. 계획의 차례와 다른 것은 내보내기가 실패해도 `nope`을 먼저 보아 (g)가 제 글로 나게)·`refuse`(`PORT=0`, 스스로 끝남·0 아닌 상태·글·`listening on` 없음), Catches 새 여섯 줄과 "no worker"를 0042로(`by`는 `serve: the webhook heard nothing`), `internal/mapper/testdata/tables.golden`(새 쿼리 둘, PG 주소와 `-update`). 이름: 시험 `TestSweep`·`TestWriteExportFiles`·`TestFiles`·`TestFileURLs`, 뷰 `ShowFiles`·`ExportFilesAgain`·`DownloadFile`·`APIFiles`, 응답 `FileLink`, 도우미 `staffOnly`. `TestPermissions`는 저장소를 알아야 해서 `app(t)` 대신 `config`로 Handler를 만든다. 확인: 풀어 쓴 게시판의 `go test ./...`가 단계마다 초록, `TestBoardTutorial`·`TestBoardMap`(PG) 초록 104초, Catches(PG, `-v`) 40줄 모두 PASS 399초(새 꼬리 줄: no sweep 30초·no worker 26초·no store 26초·the bucket ignored 21초). 헛것: (a) 고아 확인을 끔 → "no sweep" `walk: <nil>`, (b) S3 환경을 주지 않음 → `TestBoardTutorial` 빨강(`no export file … 404`), (b') `refuse`를 부르지 않음 → "the bucket ignored"가 다른 글(`serve: no export file …`)로 빨강(`by`를 글까지 적은 값: 접두만이면 초록이었다), (c) `TestWriteExportFiles`가 늙히지 않으면 태그와 관계없이 빨강(앞 실행의 파일이 남아 "행의 파일만"이 실패: 늙히지 않은 채 초록이 될 길이 없다), (d) 내보내기 확인을 끔 → "no store in the worker" `walk: <nil>`, (e) 0043의 기동 `List`를 지움 → `TestBoardTutorial` 빨강(`board with BOARD_S3_BUCKET alone: still running after 10s`), (f) 0044 끝에서 `Attachment.Key`의 `media`만 뺌 → `TestSweep` 빨강(`the attachment after the sweep: 404`), (g) `deliver`의 그 밖이 `nil` → `TestBoardTutorial` 빨강(`the job of kind nope is gone`). 끝 확인: `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 깨끗, PG 전체 시험 `go test exit=0`(`cmd/maago` 571초. 첫 실행은 `TestTablesOfQueries`의 골든만 빨강이라 고치고 처음부터 다시). 쓰기 불편한 곳: 계획의 1·2는 TODO, 구현에서 찾은 "청소나 일꾼이 미디어를 쓰면 `Config.MediaDir` 줄임은 쓸 수 없다(같은 저장소를 `WithMedia`에)"는 9번 문서의 TODO. 시험이 `cfg.Media.(maago.LocalMedia).Dir`로 디렉터리를 다시 읽는 꼴은 시험 세 곳뿐이라 두었다. 일꾼 ctx에 저장소가 없을 때의 오류(`maago: no media store in the context; … a worker uses maago.WithMedia`)는 일의 `last_error`에 고칠 길까지 적혀 그대로. `SiteURL` 없는 로컬 저장소의 `MediaURL`은 시험에서 500이지만 `OnError`의 로그가 고칠 길(`LocalMedia{Dir: …, SiteURL: …}`)을 말해 그대로(시험의 `config`에서 `SiteURL`을 빼 재 봄). 열쇠 열이 있는 모델마다 손으로 쓰는 `media()`와 `Media`에서 `NewXInput`으로의 칸 복사(첨부와 내보내기, 둘째)는 TODO. 런타임·`contrib/s3`는 고치지 않았다
   기록(2026-11-25, 구현 리뷰 반영): 0042(`Deliver`의 Kind 검사와 그 시험 줄을 지움, `TestWriteExportFiles`가 게시된 글 둘에 `exportPage = 1`, `TestFiles`의 handle이 Kind를 봄, `ID: 1`을 뺌)·0043(버킷 주석의 되풀이한 경고를 지움, 기동 때 `files in …` 로그)·0044(`strings.TrimSuffix(auth.SiteURL, "/")`, `TestFileURLs`의 없는 게시판 404)를 풀어 쓴 게시판에서 다시 쓰고 싸 넣음, `ship`이 `refuse` 뒤 버킷 없이 끝 `/`의 `BOARD_SITE_URL`로 `serve`(`board with no bucket: `), `shipped`의 조회 둘을 `withPostgres` 하나에. 그 `serve`가 먼저 물게 되어 Catches의 "another port"·"no graceful stop"의 `by`를 `board with no bucket: serve: `·`board with no bucket: serve: SIGTERM did not stop`으로(another port는 접두만: 8000이 쓰이고 있으면 같은 변이가 `stopped before`로 난다)(처음 전체 시험에서 둘만 빨강, 고친 뒤 처음부터 다시). 확인: 풀어 쓴 게시판의 `go test ./...`(PG) 초록, 헛것: 쪽 넘김의 `after = 0` → `TestWriteExportFiles`가 끝나지 않음(-timeout 30s 빨강), 뷰가 `PostPublished`로 Enqueue → `TestFiles` 빨강, `APIFiles`의 `GetBoard`를 지움 → `TestFileURLs` 빨강, main의 `TrimSuffix`를 뺌 → `TestBoardTutorial` 빨강(`board with no bucket: serve: board stopped before /app/ answered`, `"//app"`), `+ "/nope"` → 같은 빨강. `TestBoardTutorial`·`TestBoardMap`(PG) 초록 101초. 끝 확인: `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 깨끗, PG 전체 시험 `go test exit=0`(`cmd/maago` 560초, Catches 396초, 걷기 102초: 1번의 기준 12분·8분 아래). 구현 리뷰는 ponytail·codex·fable: 막는 것은 codex의 끝 `/` 하나(고침), fable 다섯과 ponytail 넷(nit)을 모두 고쳤다. 미룬 것 없음(TODO의 `SiteURL` 둘의 뜻은 그대로 9번이나 M14에서 사람에게)

   하지 않는 것
   - 런타임·`contrib/s3` 고치기(불편이 작게 고쳐질 때만, 그때 계획을 먼저 고친다), `maago doc`의 미디어·main 문단 다시 쓰기와 결정 고침(9번), 게시판 `go.mod`의 판(10번)
   - 이미지 처리(D-72), 내보내기 판 여럿·지난 판 보기, 내보내기의 진행 상태, 웹훅에 URL 싣기(위 ★), 청소의 플래그·cron
   - 게시판 시험의 S3(다른 모듈이라 가짜를 import할 수 없다)
   - `Attach`·`deletePost`의 즉시 지우기를 청소에 맡기기(고치지 않은 것은 고치지 않는다)
   - Dockerfile의 볼륨 바꾸기(기본은 여전히 로컬)
9. **문서**: `maago doc`의 미디어(단일 인스턴스, 서명된 URL과 그 한계: 만든 프로세스만 받고 다시 시작하면 403, 청소)·main(일꾼의 저장소, Kind 하나와 모르는 Kind 오류)·model(JSON의 시작값, 태그 `media`는 7번이 실음), 결정 고침: D-140·D-153·D-159·D-123(루트 밖 공개 패키지의 다섯째 `contrib/s3`)·D-09·D-72(앱이 만든 바이트는 이미지 처리가 아니다)·D-148·D-23·D-34·D-32·D-91, 확인: D-61·D-105·D-111·D-154·D-120·D-132, MILESTONES v1에서 옮김 → 확인: `TestDocExamples`, `TestPackageDoc`(요약·목록이 그대로임을 봄), `TestNoDecisionsInUserText`·`TestNoDecisionsInStrings`

   계획

   따라가 본 길: `maago doc TOPIC`은 `ref.Print`(제목·요약·Text·목록)를 찍고 `cmd/maago/docs/TOPIC.txt`의 예를 붙인다. `TestDocExamples`가 그 예를 두 방언에서 makemigrations·gen·check하고, SQLite에서는 `go vet`과 예의 시험까지 돌린다. `doc.go`(`TestPackageDoc`의 골든)에는 요약·목록·API 이름만 들어가고 **Text는 들어가지 않는다**(`packagedoc_test.go`의 `packageDoc`). 그래서 아래의 글 고침은 `TestPackageDoc`이 물지 않는다. 문 것은 `main.txt`의 예(`TestDocExamples/main`)와 D 번호 없음(`TestNoDecisionsInUserText`: `go doc -all`과 `docs/*.txt`, `TestNoDecisionsInStrings`: `topics.go`의 `(D-숫자`)뿐이다. Text의 내용은 시험이 물지 않고 찍어 읽기가 유일한 확인이다. 요약·목록 항목은 바꾸지 않는다. 그러니 `-update`도 필요 없다(바뀌면 PG 주소와 함께 `-update`). 지금 글의 낡은 곳은 이렇다. form 절 `topics.go:419`의 "Media stays in Config.MediaDir, on this machine"(버킷이면 틀림). main 절은 일꾼의 저장소를 한 문장으로만 말하고 청소·기동 때 `List`·Kind 가르기가 없다. `main.txt`의 `deliver`는 `if j.Kind != "note_created"` 하나이고 청소 고루틴이 없다. 게시판 0041~0044의 글은 `maago doc`을 가리키지 않는다(grep). 그래서 게이트(11번)가 보는 이름은 0041 `SweepMedia`·태그 `media`·`WithMedia`, 0042 Kind 가르기, 0043 `contrib/s3`·기동 `List`, 0044 `MediaURL`·`SiteURL`이다. 이 이름들을 `maago doc`이 설명하는지 아래에서 맞춘다. 9번으로 미룬 것: 3번(D-23·D-34·D-32), 4번(D-140, 재시작·다른 프로세스의 한계, fable 6), 5번(main의 Kind 둘), 6번(D-123의 셈), 7번(청소를 주기적으로 부르는 main), 초안 리뷰(JSON 필드의 `default`), TODO 93·94·96.

   - ★ → 에이전트 편의로 결정: **`main.txt`의 Kind는 진짜 일 하나와 모르는 Kind의 오류로 보인다. 둘째 일을 지어내지 않는다.** 작업 줄의 "Kind 둘"과 8번 계획("9번이 Kind 둘로 다시 쓴다")에서 물러선다. 에이전트가 이 예에서 가져갈 것은 둘째 일의 몸이 아니라 Kind를 가르는 자리다. 일을 하나 더할 때 바꿀 곳이 `case` 한 줄과 앱의 상수 하나로 보이면 충분하다. 노트 예에는 일이 하나뿐이라, 둘째 일을 지으면 라우트와 Enqueue를 더해 약 15줄이 는다. 그런데도 새 API는 가르치지 않고, 에이전트가 베낄 글에 지어낸 일이 섞인다. 둘인 꼴은 게시판 0042가 실제로 보인다. `case` 하나인 `switch`를 `if`로 줄이지 않는 까닭은 7번 ponytail 5의 빈 `case`와 다르다는 데 있다. 이 `switch`는 일을 더하는 자리이고, `if`로 두면 둘째 Kind를 더할 때 꼴을 바꿔야 한다. 대신 이렇게 쓴다:
     - Kind 값은 앱의 공개 상수 `notes.NoteCreated = "note_created"`이다. 뷰의 `Enqueue`와 main의 `switch`가 같은 이름을 쓴다. 게시판 0042의 `board.PostPublished`·`ExportFiles`와 같은 꼴이다.
     - `deliver`는 첫 줄에서 `WithMedia`를 한 뒤 `switch j.Kind { case notes.NoteCreated: return keepText(ctx, db, j) }`로 가르고, 그 밖은 `fmt.Errorf("no job of kind %q", …)`이다. `deliver(db, media)`의 꼴은 게시판과 같다.
     - 일의 몸은 `main.go`의 `keepText`로 옮긴다.
     - 시험은 두 갈래를 다 지난다. `RunOnce`로 노트의 일이 성공하는 것은 이미 있다. 여기에 `deliver(db, media)(ctx, outbox.Job{Kind: "nope"})`가 오류라는 줄을 더한다.
     - Text는 "a case for each kind; a kind it does not know is an error, so the job stays and the admin shows it"라고 말하고, 둘인 꼴은 게시판이 보인다.
   - ★ → 에이전트 편의로 결정: **미디어 문단의 주인은 form 절이다**(`MediaURL`·`SweepMedia`·`LocalMedia` 따위 API 이름이 거기 있다, `TestPackageDoc`은 이름 하나에 주제 하나를 요구한다). 저장소를 어디에 두는지, 서명된 URL과 그 한계, 청소의 뜻을 여기서 말한다. main 절은 main이 엮는 법만 맡는다: 저장소 하나를 Config·일꾼·청소에 주기, 기동 때 `List`, 청소 고루틴, 다시 빌드.
   - ★ → 에이전트 편의로 결정: **`MediaURL`은 예에 넣지 않고 글의 한 줄로 둔다.** 예의 `LocalMedia`에 `SiteURL`을 주고 시험에서 httptest 서버를 띄우면 main 예가 약 15줄 는다. 서명·기한·403은 `internal/mediatest`(로컬·메모리·가짜 S3)와 `TestLocalMediaURL`·`TestLocalMediaURLServed`가 이미 문다. 쓰는 꼴은 게시판 0044가 보인다.
   - ★ → 에이전트 편의로 결정: **`MediaDir`이 "쓸 수 없다"고 쓰지 않는다**(TODO 96의 글보다 약하게). `TestLocalMediaURLServed`가 보이듯, 줄임 `MediaDir`과 일꾼이 같은 디렉터리로 따로 만든 `LocalMedia`도 함께 돈다. 글은 이렇게 쓴다: "일꾼이나 청소가 미디어를 쓰면 저장소 하나를 만들어 `Config.Media`·`WithMedia`·청소에 같은 값으로 준다(두 값이 어긋나지 않게). `MediaDir`은 요청 안에서만 미디어를 쓰는 앱의 줄임이다."
   - ★ → 에이전트 편의로 결정: **루트 밖 공개 패키지는 ARCHITECTURE의 셈(다섯) 하나로 센다.** 다섯은 `maagotest`·`contrib/auth`·`contrib/admin`·`rt`·`contrib/s3`이다. D-123의 "둘째 예외(첫째 `maagotest`)"는 다섯의 셈에서도 맞아 그대로 두고, 고침 줄에는 지금 예외가 다섯이고 `contrib/s3`가 다섯째라는 것만 적는다. D-160의 "D-123(루트 밖 공개 패키지의 셋째)"을 "다섯째"로 고친다. ARCHITECTURE 52·103줄은 이미 이 셈이다.
   - ★ → 에이전트 편의로 결정: **TODO 93(태그를 더한 배포는 청소하는 프로그램을 다시 빌드)의 자리는 main 절의 청소 문단이다.** 배포와 바이너리의 일이라서다.

   바꿀 파일
   1. `cmd/maago/docs/main.txt`
      - `main.go`: 저장소를 한 값으로 둔다(`media := maago.LocalMedia{Dir: "media"}`). 주석 "the directory must exist"를 지우고 기동 때의 `if _, err := media.List(ctx, ""); err != nil { log.Fatal(err) }`를 `handler(db, media)` 뒤에 둔다(fable 2). 없는 디렉터리는 `Handler`가 고칠 길과 함께 잡는다(`http.go`의 "create the directory"). `List`가 앞이면 `os.ReadDir`의 날것 오류가 그 글을 가린다. `List`는 버킷으로 바꿨을 때 빈 칸·틀린 열쇠·없는 버킷을 첫 파일이 아니라 기동에서 잡는 줄이고(게시판 0043), 주석도 그렇게 쓴다.
      - 일꾼 곁에 청소 고루틴을 둔다(게시판 0041과 같은 꼴, 약 12줄). `work`의 ctx에서 `maago.SweepMedia(maago.WithMedia(work, media), db, 0)`을 부르고, 오류는 로그에 남긴 뒤 24시간마다 다시 돈다. `work.Done()`이면 끝난다. 주석은 "the store is this database's alone"이다.
      - `deliver`는 위 ★의 꼴로 쓰고 `keepText`를 둔다.
      - `notes/views.go`: `const NoteCreated = "note_created"`를 두고 `Enqueue`가 그것을 쓴다.
      - `main_test.go`: 모르는 Kind 줄을 더한다. 청소·`OpenMedia` 줄은 그대로 둔다.
      - → 확인: `go test ./cmd/maago -run 'TestDocExamples/main'`(두 방언, PG는 `MAAGO_TEST_PG_URL=…/m13_8`) 초록. 헛것 둘을 한 번씩 돌리고 되돌린다:
        - (a) `deliver`의 그 밖을 `return nil`로 바꾸면 빨강
        - (b) `case`의 Kind를 `"note_made"`로 바꾸면 빨강(`worker again`, 실패한 일이 남음)

        기동 `List`와 청소 고루틴은 `main()` 안이라 시험이 돌리지 않는다. gofmt·`go vet`·컴파일만 이들을 문다. 같은 꼴을 실제로 도는 것은 게시판 `ship`(0041·0043)이다.
   2. `internal/ref/topics.go` main 절 Text
      - Config 목록의 "MediaDir or Media (where media is kept)"에 ★의 한 문장(저장소 하나를 셋에, `MediaDir`은 요청만의 줄임)을 붙인다.
      - outbox 문단의 끝 문장(`WithMedia`)을 Kind 가르기와 저장소로 다시 쓴다: "deliver puts the store in ctx with maago.WithMedia, where SaveMediaFrom finds it (the sweep gets its own, maago.WithMedia(work, media)), and goes by j.Kind: a case for each kind, which the app names as a constant its views enqueue; a kind it does not know is an error, so the job stays and the admin shows it."
      - 새 문단을 둔다: 청소는 main이 일꾼 곁에서 기동 때와 하루마다 `SweepMedia`(예의 main처럼), 저장소는 이 DB만의 것(스테이징이 운영 버킷을 가리키면 운영 파일을 지운다), 태그를 더한 배포는 청소하는 모든 바이너리를 다시 빌드(옛 바이너리는 새 열의 열쇠를 모르고, 하루 넘은 파일의 열쇠를 새 열로 옮기면 그 파일을 지운다, TODO 93), 기동 때 `media.List(ctx, "")`로 칸·버킷을 첫 요청 전에 본다.
      - 요약은 그대로 둔다.
   3. `internal/ref/topics.go` form 절의 미디어 문단(`topics.go:419~424`). 다시 쓰는 줄거리:
      - **어디에**: `Config.MediaDir`·`maago.LocalMedia`는 이 기계의 디스크라 인스턴스 하나용이다. 둘 이상이거나 디스크를 잃는 컨테이너면 버킷이다: `Config.Media`에 `contrib/s3`의 `s3.Store`(AWS·R2·MinIO, `go doc github.com/maago-web/maago/contrib/s3`).
      - **서명된 URL**: `maago.MediaURL(ctx, m, ttl)`이다. 권한을 본 뒤에만 만든다. 기한까지는 가진 누구나 받는다. ttl은 1초~7일이고 늘 내려받기다.
        - 로컬 저장소는 `LocalMedia.SiteURL`(사이트 주소에 `Config.Prefix`까지, 예: `https://example.com/app`)이 있어야 하고, `Handler`가 `/maago/media`에서 준다. 그 URL은 만든 프로세스만 받고, 다시 시작하면 403이다.
        - 다시 시작해도 살아야 하거나 다른 인스턴스·외부 서비스가 받을 URL은 버킷의 것이다.
      - **청소**: 지금 글(태그 `media`, `SweepMedia(ctx, db, 0)`, `DeleteMedia`)은 그대로 두고, 부르는 자리는 maago doc main이라고 가리킨다.
      - **라우트 없음**: 지금 글을 그대로 둔다. 디렉터리를 서빙하는 라우트는 없다. 서명된 URL 말고는 뷰가 권한을 본 뒤 `ServeMedia`로 준다.
      - 글의 근거는 이렇다: 403·기한·ttl 범위는 `mediatest`·`TestLocalMediaURL`이 문다. "다시 시작하면 403"은 시험이 없고 `signKey`가 프로세스마다 무작위라는 데서 온 글이며, `LocalMedia.URL` 주석과 같다.
   4. `internal/ref/topics.go` model 절(아래 "초안 리뷰" 절의 readmate, fable): "time.Time, []byte, maago.UUID and maago.JSON have no default" 뒤에 한 문장을 둔다. JSON 필드의 시작값(빈 map 따위)은 앱이 `InsertPost`의 입력에 준다. 태그 `media`는 7번이 `FieldTags`와 `model.txt`의 `Cover`에 이미 실었다(`TestDocKeysAreRead`·`TestDocExamplesUseEveryKey`가 문다). 그래서 model 절 Text에는 더하지 않는다.
   5. `http.go`의 `Config.MediaDir` 주석(go doc): "It is short for Media: …" 뒤에 한 문장을 붙인다: "A worker or SweepMedia needs the store too: then make one, set Media to it in place of MediaDir, and give the same one to maago.WithMedia." 두 칸을 함께 채우면 `Handler`가 "both set"으로 멈추니 "in place of"를 쓴다(fable 5). 동작은 바꾸지 않는다.
      - → 확인(2~5): `go test ./cmd/maago -run 'TestDoc$|TestDocExamples|TestNoDecisionsInUserText|TestNoDecisionsInStrings'`와 루트 `go test -run TestPackageDoc .` 초록, `maago doc main`·`form`·`model`을 찍어 읽고(줄바꿈·낡은 글) `grep -n 'D-[0-9]\|on this machine'`이 비어 있음을 본다.
   6. `docs/DECISIONS.md`
      - **확인만 한다**(2번이 이미 "고침(M13 2번, D-162)"을 달았다): D-105·D-111·D-120·D-154·D-132. D-61에는 "확인(M13, D-162): 그대로, 단계를 넘는 갈래는 두지 않는다"를 단다(D-162가 D-61은 그대로라 했다, fable 4).
      - **고침 줄을 단다**(표의 끝 칸 앞에 "고침(M13 N번): …"):
        - D-23: 모델 결과의 `missing`도 마이그레이션이 모델과 다를 때 같은 실행에 나온다(초안을 더한 연습 DB, ` (checked against the models)`, 3번).
        - D-34: 다르면 사용자 쿼리는 마이그레이션의 연습 DB에 makemigrations의 초안을 더한 것에 prepare한다. 이전 상태는 여전히 적용한 결과다(3번).
        - D-32: "기계가 검증한다"가 makemigrations 전에도 쿼리를 본다(3번).
        - D-148: 뒤로의 "외부 미디어 저장소"는 M13(D-160·D-161), "단계를 넘어 계속하는 검사"는 목록에서 지우고 까닭을 적는다(Q7).
        - D-140:
          - 외부 저장소는 `MediaStore`·`contrib/s3`다(4·6번).
          - "공개 미디어 라우트는 없다"는 "기한부 서명 URL 하나"로 바뀐다. 로컬은 `/maago/media`, 프로세스의 열쇠라 다시 시작하면 403이다.
          - 고아는 문서만이 아니라 태그 `media`와 `SweepMedia`가 치운다(D-161).
          - `MediaDir`은 `LocalMedia`의 줄임이고, 일꾼·청소가 쓰면 저장소 하나를 셋에 준다.
          - `OpenMedia`는 `MediaFile`을 돌려준다.
          - 이미지 처리는 여전히 없다.
        - D-153: main이 일꾼 곁에 청소 고루틴(기동 때와 하루마다)을 두고 기동 때 `media.List`로 저장소를 본다(8번 게시판 0041·0043, `maago doc main`).
        - D-159: 일꾼의 handle은 main의 `deliver(db, media)`이다. 첫 줄에서 `WithMedia`를 하고, Kind는 앱의 상수로 가른다. 모르는 Kind는 오류라 일이 남는다(8번 0042).
        - D-123: "둘째"는 그대로 맞다. 지금 예외는 ARCHITECTURE의 다섯이고 `contrib/s3`가 다섯째다(복사본 없는 라이브러리, main이 import). D-160의 "셋째"를 "다섯째"로 고친다.
        - D-09: `contrib/s3`는 SDK 없이 stdlib SigV4로 쓰고, 같은 `TestRuntimeImports`가 문다(6번).
        - D-72: 앱이 만든 바이트(일꾼의 내보내기 따위)를 미디어로 두는 것은 이미지 처리가 아니다. 받아 둘 뿐 바꾸지 않는다(D-160).
        - D-91: S3 시험도 같은 꼴이다. `MAAGO_TEST_S3_URL`이 없으면 로컬은 skip, CI는 실패한다(6번).
      - 끝의 기록에 "**2026-11-2x (M13 9번)** — D-09·D-23·D-32·D-34·D-72·D-91·D-123·D-140·D-148·D-153·D-159·D-160 고침(문서), D-61은 확인, D-105·D-111·D-120·D-132·D-154는 2번에서"를 단다.
   7. `docs/MILESTONES.md`
      - 145줄 "외부 저장소는 v1" 뒤에 "(M13으로, D-160)"를 붙인다.
      - v1 목록 288줄 "미디어의 외부 저장소(S3 등)와 고아 파일 청소 (D-140)"를 지운다(M13의 완료 조건에 있다). 소셜 로그인 줄은 M14의 것이라 건드리지 않는다.
   8. `docs/TODO.md`
      - 93·96을 체크하고 "→ M13 9번: …"을 단다.
      - 94(`SiteURL` 둘의 뜻)는 열린 채 둔다. 글에 "M13 9번: 지금 계약(Prefix 포함)을 `maago doc`에 적음, 바꿀지는 사람에게, v0.4.0 태그(10번) 전이면 깨는 값이 싸다"를 덧붙인다.
      - → 확인(6~8): 고친 결정 줄이 서로·본문(`maago doc`)과 어긋나지 않는지 읽는다. 사용자 글이 아니라 시험은 없다.
   9. 끝 확인
      - 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .`이 비어 있어야 한다.
      - PG 전체 시험: `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_8`, `-timeout 60m`. 앞에서 돌리고 로그 파일의 `go test exit=` 줄로 본다.
      - 구현 리뷰는 fable과 ponytail이다. 까닭: 문서 작업이고 런타임 동작을 바꾸지 않는다(WORKFLOW 6의 "문서 작업은 fable 하나"). 다만 예에 청소 고루틴과 `switch`가 새로 생겨 ponytail은 건너뛰지 않는다(WORKFLOW 6, fable 덧붙임).

   계획 리뷰 codex·fable 반영(막는 것 없음): 작업 줄을 계획에 맞춤(Kind 하나와 모르는 Kind 오류, model은 JSON의 시작값, D-123은 다섯째, D-61은 확인 쪽, `TestPackageDoc`은 요약·목록을 봄, codex 1·fable 6). 확인(2~5)에 `TestNoDecisionsInStrings`와 찍은 글의 grep, Text는 읽기가 유일한 확인이라고 밝힘(fable 1). 기동 `List`를 `handler` 뒤로, 없는 디렉터리는 `Handler`의 고칠 길 글이 잡고 `List`는 버킷의 줄(fable 2). D-123의 "둘째"는 그대로, 고침 줄은 다섯째만(fable 3). D-61은 고침이 아니라 확인(fable 4). `MediaDir` 주석은 "in place of MediaDir"(fable 5). TODO 93의 손실 조건(하루 넘은 파일의 열쇠를 새 열로 옮길 때)을 main 절 글에(fable 덧붙임). 구현 리뷰에 ponytail(fable 덧붙임). 받지 않은 것: `TestDocExamples/main`에서 기동 `List`·청소·종료를 실제로 돌리기(codex 2). 그러려면 예의 `main()`을 `run(ctx)` 따위로 쪼개야 해 에이전트가 베낄 main이 실제 앱과 다른 꼴이 되고, 같은 꼴(0041 청소 고루틴·0043 기동 `List`)은 게시판 `ship`이 실제로 띄워 문다(헛것 (a)·(e)가 빨강을 보였다). 예의 시험은 `SweepMedia`를 이미 부른다.
   기록(2026-11-25): `main.txt`(저장소 하나와 그 주석, `handler` 뒤의 기동 `List`, 청소 고루틴, `deliver`의 `switch`와 `keepText`, `notes.NoteCreated`, 시험의 `Kind: "nope"` 줄), `topics.go` main 절(Config 목록의 `MediaDir` 한 문장, outbox 문단의 Kind 가르기, 새 청소 문단: 기동 때와 하루마다·이 DB만의 저장소·태그를 더한 배포의 다시 빌드·`handler` 뒤의 `List`)·form 절(디스크는 인스턴스 하나, 버킷은 `contrib/s3`, 청소는 maago doc main이 부름, `MediaURL`과 로컬 URL의 `SiteURL`·`/maago/media`·다시 시작하면 403)·model 절(JSON의 시작값은 `NewPostInput`에), `http.go`의 `MediaDir` 주석, DECISIONS 고침 열둘과 D-61 확인·기록 줄, MILESTONES 145·288줄, TODO 93·96 체크와 94의 덧글. 글의 "on this machine"은 버킷과 헷갈려 "on the server's disk"로 썼다. 확인: `TestDocExamples/main`(두 방언) 초록, 헛것 (a) 그 밖이 `return nil` → 빨강(`a kind deliver does not know: no error`), (b) `case "note_made"` → 빨강(`worker again: true <nil>`), 게이트 `TestNoDecisionsInStrings`는 form 글에 `(D-160)`을 넣으면 빨강. `TestDoc$|TestDocExamples|TestNoDecisionsInUserText|TestNoDecisionsInStrings`(PG)와 `TestPackageDoc` 초록(골든 그대로, `-update` 없음), 찍은 `maago doc main`·`form`·`model`의 `grep -n 'D-[0-9]\|on this machine'` 비어 있음, 줄바꿈을 읽음. 끝 확인: `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 깨끗, PG 전체 시험 `go test exit=0`(`cmd/maago` 556초). 런타임 동작·`contrib/s3`는 고치지 않았다
   기록(2026-11-25, 확인 6~8): 고친 결정 줄을 읽어 셋을 고침. D-148은 고침 줄만 달고 목록의 "단계를 넘어 계속하는 검사"가 남아 있었다 → 목록에서 지우고 고침 줄은 "지웠다". D-153의 새 고침이 결정 칸 끝에 들어가 기존 고침(근거 칸)과 갈렸다 → 근거 칸 끝으로. `maago doc main` Config 목록의 76자 줄을 다시 접음. D-72(칸 둘)·나머지는 그대로 맞다. DECISIONS를 읽는 시험 `TestSearchOrdersIsTheDecisionsExample`과 글 시험(`TestDoc$`·`TestNoDecisions*`·`TestPackageDoc`) 초록, md와 글 줄바꿈뿐이라 전체 시험은 다시 돌리지 않았다. 복사되는 outbox의 패키지 문서는 `deliver`의 꼴을 보이지 않아(`outbox.Run(work, db, deliver)` 한 줄) 고칠 것 없음
   기록(2026-11-25, 구현 리뷰 반영): 구현 리뷰는 ponytail·codex·fable: 막는 것 없음(셋 다). 받은 것: `topics.go` main 절에서 청소 문장에 나이("the files over a day old that no row keeps", fable 2), 옛 바이너리의 손실을 옮긴 열쇠만이 아니라 "deletes that column's files once they are a day old"로(fable 1, TODO 93의 체크 글도 같은 조건으로), `MediaDir` 문장을 "a short form for an app with no worker or sweep: the example's main makes one store …"로(fable 3, 청소하는 앱만 남는 글과 어긋나 보였다), 기동 `List` 문장을 "main checks a bucket's store as it starts, after Handler (the example's comment)."로 줄임(ponytail 3, 예의 주석과 겹침), `TestDoc`이 찍은 주제마다 괄호 없는 `D-숫자`도 잡음(fable 4: `TestNoDecisionsInStrings`는 `(D-`만 본다; 주제 Text에 `D-160`을 넣으면 main·auth가 빨강을 확인하고 되돌림), 예의 고루틴에서 `WithMedia`를 빼도 초록: `main()` 안이라 (d)·(e)와 같이 게시판 `ship`에 맡긴다(fable 5). 받지 않은 것: `keepText`·`switch`(ponytail 1, Kind ★의 꼴), 기동 `List`의 겹침(ponytail 2, 버킷 앱이 베낄 줄), `MediaDir` 규칙을 세 곳에서(ponytail 4, go doc·maago doc·예로 읽는 길이 다름), D-148과 D-153의 고침 꼴 맞추기(fable 6, 계획이 D-148 목록에서 지우라 했고 D-153은 원문이 틀리지 않아 고침만), codex(지적 없음, 쓰기 금지 환경이라 실행은 못 함). 확인: `TestDoc$|TestDocExamples/main|TestNoDecisions*`(PG)·`TestPackageDoc` 초록(골든 그대로), 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 깨끗, `MAAGO_TEST_PG_URL=…/m13_8`로 PG 전체 시험 `go test exit=0`(`cmd/maago` 561초)

   하지 않는 것
   - **`SiteURL` 둘의 뜻을 맞추는 계약 바꾸기**(TODO 94). 로컬 URL이 Prefix를 스스로 붙이게 하는 것은 D-160 4번 고침을 뒤집는 런타임 계약이라 문서 작업 밖이다. 지금 계약을 글에 적고 사람에게 넘긴다. 바꾼다면 v0.4.0(10번) 전이 싸다.
   - `maago doc`의 예에 `contrib/s3`·`MediaURL`·둘째 일을 넣기(위 ★, 게시판 0042~0044가 보인다).
   - 요약·목록 항목 바꾸기(`doc.go`가 그대로다).
   - ARCHITECTURE 트리 고치기: 40·41·58·103줄이 이미 M13의 것이다.
   - 열쇠 열 모델의 `media()`·칸 복사 생성(TODO 97, M14).
   - 없는 표의 did you mean(TODO 95, M14).

   **고침(SiteURL)** (10번 v0.4.0 전, TODO의 "`SiteURL` 둘의 뜻이 다르다")

   결정(메인 세션이 codex·fable의 의견을 듣고 에이전트 편의로 정함, 2026-11-25): **B**. `LocalMedia.SiteURL`은 주소만(scheme과 host, 경로 없음, 끝 `/` 하나는 뗀다)이고 `auth.SiteURL`과 같은 뜻이다. 경로가 있으면 오류다. 로컬 URL의 접두어는 런타임이 붙인다: `SiteURL + Prefix + /maago/media?…`(D-122의 "접두어는 경로를 만드는 자리에서 한 번"). `Handler`의 "`SiteURL`의 경로 == `Config.Prefix`" 검사는 지우고 꼴 검사만 남긴다. 기각: A(지금 그대로, codex 추천. 일꾼이 완성 URL을 들고 있다는 근거는 서명 열쇠가 프로세스마다라 약하다), `LocalMedia.Prefix` 칸(맞출 값 둘이 남는다). v0.4.0 전이라 깨지는 사용자가 없다. 위 "하지 않는 것"의 첫 줄(계약 바꾸기는 문서 작업 밖)은 이 절이 받는다. 그 줄은 9번 본문의 기록이라 고치지 않는다.

   계획

   따라가 본 길:
   - `LocalMedia.URL`(`media.go:391`)은 열쇠 → `SiteURL`이 빔 → `l.site()` → 서명 → `u.JoinPath("maago", "media")` 차례다. `site()`(`media.go:410`)는 끝 `/` 하나를 떼고 parse한 뒤 scheme·host·user·query·fragment를 본다. 경로는 보지 않는다.
   - 경로 검사는 `Handler`에만 있다. `handler.media`(`http.go:236~243`)가 `local.SiteURL != ""`이면 `site()`를 부르고 `u.Path != cfg.Prefix`면 오류를 낸다. 그 뒤 로컬 저장소(`LocalMedia`·`*LocalMedia`·`MediaDir`)일 때만 `GET /maago/media`를 단다. 버킷이나 남의 저장소면 그 전에 `return nil`이다.
   - 접두어는 프로세스 값이다. `Handler`의 맨 끝(`http.go:187`)이 `rtcore.Current.Store(&rtcore.Process{Prefix, TZ, Static})`를 한다(D-122, 마지막 `Handler`가 이긴다). `h.media`는 그보다 앞에서 불리므로 `media` 뒤의 검사(`MaxBody`·`Prefix`·`LoginURL`·`Origins`)가 실패하면 라우트는 달렸어도 `Current`는 그대로다. `rtcore.Prefix()`는 `Handler`가 없을 때와 접두어가 없을 때 둘 다 `""`라 가드에 쓸 수 없다.
   - 복사된 auth의 `siteURL`(`internal/scaffold/testdata/auth/auth.go:162`)은 `url.Parse(SiteURL)` 뒤 `(scheme이 http|https) && Host != "" && u.Scheme+"://"+u.Host == strings.TrimSuffix(SiteURL, "/")`이다. 이 한 비교가 경로·user·query(`?`만 있는 것 포함)·fragment를 모두 거절한다. 거절 목록 시험은 `auth_test.go:524`의 `"", "example.com", "https://", "ftp://example.com", "https://example.com/app", "https://ann@example.com", "https://example.com?x=1"`와 532줄의 끝 `/` 받음이다. 런타임은 복사본을 import할 수 없으니 같은 식을 `site()`에 쓴다.
   - `MediaURL`을 부르는 곳은 루트 시험(`TestLocalMediaURLServed`), `internal/mediatest`(`Store`의 `signedURLs`, `TestLocalMediaStore`가 `site()` 뒤에 부른다), `contrib/s3`(제 `URL`, 상관없음), 게시판 0044의 `APIFiles`(뷰, 요청 안이라 `Handler` 뒤)이다. `maago doc main`의 예(`cmd/maago/docs/main.txt`)와 게시판 0040~0043의 일꾼은 `MediaURL`을 부르지 않는다(grep). 그래서 "Handler 전에 URL" 길에 걸리는 코드는 시험 말고는 없다.
   - 시험이 기대는 프로세스 값: 루트 패키지 시험은 `t.Parallel`이 없고 `process_test.go`가 끝에 `Current.Store(nil)`을 한다. `TestLocalMediaURL`은 `Handler` 없이 `l.URL`을 부른다(가드가 생기면 빨강). `TestLocalMediaURLServed`는 `/app` 사이트와 줄임 사이트의 `Handler`를 둘 다 만든 **뒤** `put(prefixed)`를 부른다. B에서는 마지막 `Handler`(접두어 `""`)의 값이 붙어 `/app` 서버에 접두어 없는 URL이 가고 404가 된다. `site()` 도우미는 `cfg(srv.URL + prefix)`로 경로를 넣어 준다.
   - 게시판 0044: main이 `strings.TrimSuffix(auth.SiteURL, "/") + "/app"`(`strings` import는 0044가 이 줄 때문에 더했다), `web_test.go`의 `config`는 `SiteURL: "http://example.com"`, `TestUnderApp`은 `/app`을 담은 `SiteURL`로 바꾼 덩어리가 있다(`MediaURL`은 부르지 않는다). `TestFileURLs`는 접두어 없는 `config`라 `srv.URL`이 그대로 맞다. 걷기의 `ship`(`tutorial_test.go:440`)은 버킷 없이 `BOARD_SITE_URL=http://localhost:8000/`로 띄운다. Catches의 "another port"·"no graceful stop"의 `by`가 그 `serve`(`board with no bucket: serve: `)에 기댄다. MAP 125줄은 "로컬 저장소의 `SiteURL`은 Prefix까지(D-160)"다.
   - 글: `topics.go:447~451`(form 절, "the site's address with Config.Prefix (https://example.com/app)"), `media.go:299`(`SiteURL` 주석), `media.go:388~390`(`URL` 주석), `media.go:416`(오류 글의 "such as https://example.com/app"), `http.go:236~243`의 오류 글. `http.go`의 `Config.Prefix` 주석(65~74줄)과 `rtcore.Process` 주석은 접두어를 붙이는 자리를 센다. `Config.Media` 주석(`http.go:94~97`)은 그대로 맞다. `cmd/maago/docs/*.txt`에는 `LocalMedia`의 `SiteURL`이 없고(auth의 것뿐), `doc.go` 골든에는 Text가 들어가지 않으며 요약·목록은 바꾸지 않는다. 그래서 fable이 짐작한 "cmd/maago/docs 골든"은 바꿀 것이 없고 `-update`도 필요 없다(바뀌면 PG 주소와 함께).

   ★ → 에이전트 편의로 결정: **가드는 `rtcore.Process`의 새 칸 하나다.** `Process{…, LocalMedia bool}`이고 `Handler`가 로컬 라우트를 달았을 때 참으로 `Store`한다. `LocalMedia.URL`은 `p := rtcore.Current.Load()`가 nil이거나 `!p.LocalMedia`면 오류를 내고, 아니면 `p.Prefix`를 붙인다. 까닭:
   - `Current != nil`(Handler가 돌았음)만 보면 새 상태가 없지만, 마지막 `Handler`가 버킷이나 저장소 없음이면 `/maago/media`가 없어 URL이 조용히 404가 된다. 결정의 말("서명된 로컬 라우트를 걸기 전")은 라우트를 본다.
   - 접두어와 같은 `Store`에 실려 "마지막 `Handler`가 이긴다"(D-122)가 그대로다. `h.media`가 라우트를 단 뒤 `Handler`가 실패해도 칸은 서지 않는다.
   - 표시는 `handler`에 칸 하나(`local bool`, `media`가 `Handle` 줄에서 세운다)이고 `Store` 줄에서 옮긴다. `media`의 반환 꼴은 바꾸지 않는다.
   - 대가: `Handler` 없이 뷰나 일꾼을 시험하며 `MediaURL`을 `LocalMedia`에 부르면 이제 오류다(`WithMedia` 주석의 "a view tested without HTTP"). 그런 시험은 먼저 `Handler`를 만든다. 오류 글이 그렇게 말한다. 다른 프로세스의 일꾼은 서명 열쇠가 달라 원래도 그 URL이 403이었다. 이제는 URL을 만들 때 오류다(그 프로세스가 `Handler`를 만들지 않았다면). 글에는 "만든 프로세스만 준다, 다시 시작하면 403"을 그대로 둔다.

   ★ → 에이전트 편의로 결정: **`URL`의 차례는 열쇠 → `SiteURL` 빔 → `site()`(꼴) → 가드 → 서명이다.** 입력의 잘못이 프로세스 상태보다 먼저 나와 `TestLocalMediaURL`의 오류 줄이 시험 순서와 상관없이 같다.

   ★ → 에이전트 편의로 결정: **D-122에도 고침 한 줄을 단다.** "접두어는 경로를 만드는 자리에서 한 번"의 목록에 로컬 미디어 URL이 붙고, 그 자리만은 `Handler`가 없으면 접두어 없는 값이 아니라 오류다(서명 라우트가 없으면 URL이 쓸모없다). D-160만 고치면 D-122의 목록이 하나 모자란다.

   바꿀 파일
   1. `internal/rtcore/process.go`: `Process`에 `LocalMedia bool`(주석: 마지막 `Handler`가 `/maago/media`에서 `LocalMedia`의 URL을 준다), 타입 주석의 "the generated URL functions and the template functions"에 `LocalMedia.URL`을 더한다.
   2. `http.go`
      - `handler`에 `local bool`. `media()`의 `h.mux.Handle("GET /maago/media", …)` 줄 곁에서 `h.local = true`. `Store` 줄에 `LocalMedia: h.local`.
      - `media()`의 `if local.SiteURL != ""` 덩어리에서 경로 비교를 지우고 `site()`의 오류만 돌려준다.
      - `Config.Prefix` 주석의 "It goes before the paths the generated URL functions, {{url}} and the login page's next make"에 "and a LocalMedia's URLs"를 더한다.
   3. `media.go`
      - `SiteURL` 주석: "the site's scheme and host as browsers and other services reach it, as auth.SiteURL: https://example.com; Handler's Config.Prefix goes after it; URL needs it".
      - `site()`: 복사된 auth의 식 그대로(parse한 값의 `Scheme+"://"+Host`가 끝 `/` 하나를 뗀 `SiteURL`과 같아야 한다). 오류 글은 "is not a site's address such as https://example.com: http or https and a host, no path (Config.Prefix is added), user, query or fragment".
      - `URL`: 위 차례로 가드를 두고 `u.Path = p.Prefix + "/maago/media"`. 가드의 오류 글: "maago: no Handler serves this process's local media URLs yet: make maago.Handler, with this store in Config.Media or Config.MediaDir, before a URL; a process that serves no HTTP needs a bucket's URLs". 주석은 "under SiteURL and the last Handler's Config.Prefix. Only the process that made a URL serves it…"로 고친다.
      - `MediaURL` 주석에 "A LocalMedia's needs this process's Handler first, in a test too." 한 문장(`WithMedia` 주석의 "a view tested without HTTP"가 그 길로 오류를 만나지 않게).
   4. 시험 (`media_test.go`, `media_store_test.go`), 먼저 써서 빨강을 본다:
      - `TestLocalMediaURL`: `defer`로 `Current`를 되돌리는 꼴(`maagotest/permissions_test.go:148`). `SiteURL` 없음 오류는 그대로. `Current.Store(nil)`에서 맞는 `SiteURL`의 `URL`이 가드 오류. `Handler(Config{DB: testDB, Media: l, Prefix: "/app", Apps: …})` 뒤 `https://example.com/maago/media?`가 아니라 `https://example.com/app/maago/media?`로 시작. `l`의 `SiteURL`은 끝 `/`가 있는 `https://example.com/`. 거절 목록에 auth와 같은 것을 맞춘다: 지금 목록 + `https://example.com/app`·`https://example.com//`·`https://example.com?`(auth의 목록 `auth_test.go:524`와 같은 뜻). 거절 줄마다 오류 글의 머리 `maago: LocalMedia.SiteURL`을 보고, 가드 줄은 가드 글의 머리를 본다: 지금 거절 줄은 `err == nil`만 보므로 가드가 생기면 꼴 검사가 없어도 가드 오류로 초록이 된다(헛것 (c)를 놓친다). `/app` 로컬 `Handler` 뒤 `Prefix: "/b"`와 `MaxBody: -1`(`media()` 뒤의 검사에서 실패)인 `Handler`가 실패해도 `URL`은 그대로 `/app`(실패한 `Handler`는 `Current`를 덮지 않는다). 저장소가 버킷인 `Handler`(`Media: struct{ MediaStore }{}`) 뒤에도 가드 오류: 이 줄은 `/app` 로컬 `Handler`와 접두어 확인 **뒤**에 둔다(로컬 `Handler` 뒤의 버킷 `Handler`가 칸을 내린다, 마지막이 이긴다).
      - `TestMediaDirConfig`: 212줄(`https://example.com`과 `Prefix: "/app"`)은 받음(""), 213줄(`https://example.com/app`)은 `maago: LocalMedia.SiteURL "https://example.com/app" is not`, 214줄은 `https://example.com/`와 `/app`으로 받음.
      - `media_store_test.go`: `site()`가 `cfg(srv.URL)`(주소만)을 넘긴다. `TestLocalMediaURLServed`는 `/app` 사이트를 만들고 **바로** `put(prefixed)`, 그다음 줄임 사이트와 `put`, 그리고 둘을 받는다(URL은 만든 때의 마지막 `Handler`를 따른다는 것을 주석 한 줄로). `prefixed`의 `SiteURL`은 `srv.URL + "/"`(끝 `/`는 상관없다).
      - → 확인: 고치기 전에 바꾼 시험이 빨강(`TestLocalMediaURL`의 접두어·가드·경로 거절, `TestMediaDirConfig`의 세 줄, `TestLocalMediaURLServed`는 옛 `Handler`의 `has path "", not Config.Prefix` 오류. `/app` 404는 헛것 (a)에서 본다). 고친 뒤 `go test -run 'TestLocalMedia|TestMediaDirConfig|TestMedia$|TestSweepMedia' .`과 `go test ./internal/mediatest/... ./internal/rtcore/... ./rt/... ./maagotest/...` 초록. 헛것을 하나씩 넣고 이름 붙인 시험의 빨강을 본 뒤 되돌린다(되돌린 뒤 diff 같음):
        - (a) `URL`이 접두어를 붙이지 않음 → `TestLocalMediaURL`, `TestLocalMediaURLServed`
        - (b) 가드를 지움 → `TestLocalMediaURL`(nil과 버킷 `Handler` 줄)
        - (c) `site()`가 경로를 받음(옛 검사) → `TestLocalMediaURL`·`TestMediaDirConfig` 213줄
        - (d) `site()`가 끝 `/`를 떼지 않음 → `TestLocalMediaURL`·`TestMediaDirConfig` 214줄·`TestLocalMediaURLServed`
        - (e) `Handler`에 옛 `Path != Prefix` 검사를 남김 → `TestMediaDirConfig` 212줄
        - (f) `h.local`을 세우지 않음(늘 거짓) → `TestLocalMediaStore`·`TestLocalMediaURLServed`
        - (h) `h.local`을 `media()` 머리에서 늘 참으로 → `TestLocalMediaURL`(버킷 `Handler` 줄)
        - (i) `Current.Store`를 `media()` 뒤의 검사들 앞으로 → `TestLocalMediaURL`(실패한 `Handler` 줄)
   5. 글: `internal/ref/topics.go` form 절 447줄 끝~450줄의 LocalMedia 문장만(버킷 문장, 450줄 끝 "A"~452줄은 그대로). "A LocalMedia makes one only with its SiteURL, the site's scheme and host as auth.SiteURL (https://example.com); Handler's Config.Prefix goes after it, and Handler serves it at /maago/media. Only the process that made it serves it, after its Handler: one made before is an error, a restart answers 403." 줄바꿈은 지금 너비로.
      - → 확인: `go test ./cmd/maago -run 'TestDoc$|TestNoDecisionsInUserText|TestNoDecisionsInStrings'`, 루트 `go test -run TestPackageDoc .` 초록(골든 그대로), `maago doc form`을 찍어 읽고 `grep -n 'D-[0-9]\|example.com/app'`이 비어 있음.
   6. 게시판 0044(`examples/board/steps.sh unpack` → 풀린 저장소에서 0044 커밋을 `git rebase`로 고침 → `pack`):
      - `main.go`: `maago.LocalMedia{Dir: "media", SiteURL: auth.SiteURL}`, 0044가 더한 `strings` import 덩어리를 뺀다. 주석은 "Its signed URLs start with SiteURL, the site's scheme and host, and Handler adds the prefix."
      - `web_test.go`: `TestUnderApp`의 `cfg.Media` 덩어리를 뺀다(`config`의 `http://example.com`이 `/app` 아래에서도 맞다). `config`의 줄은 그대로.
      - 커밋 글의 둘째 항목: "A local store makes its URLs from its SiteURL, the site's scheme and host as auth.SiteURL: main gives it auth.SiteURL, and Handler puts the prefix after it, as it does for every path it makes. Handler refuses a SiteURL with a path. …(만든 프로세스만·다시 시작하면 그만·버킷은 버킷의 것은 그대로)". 끝 문단의 "TestUnderApp's has /app in it, as its Prefix."를 지운다.
      - 트레일러(`Maago-Run`·`Maago-Expect`)가 있으면 그대로 둔다.
      - `examples/board/MAP.md` 125줄: "로컬 저장소의 `SiteURL`은 주소만, 접두어는 런타임이(D-160)".
      - → 확인: `go test ./cmd/maago -run 'TestBoardTutorial$|TestBoardMap'`(PG, `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_9b`) 초록, 그다음 `TestBoardTutorialCatches` 초록(`by` 글 그대로: `ship`의 끝 `/` `serve`는 바꾸지 않는다). 헛것 (g): 풀린 저장소의 main을 `auth.SiteURL + "/app"`로 바꿔 `TestBoardTutorial`이 빨강(`board with no bucket: serve: …`, `Handler`의 경로 거절)인 것을 한 번 보고 되돌린다. 풀린 게시판의 `go test ./...`(PG)도 초록.
   7. `docs/DECISIONS.md`
      - D-160: 결정 칸의 "고침(M13 6번)" 덩어리 끝에 "고침(M13 9번 고침): `LocalMedia.SiteURL`은 `auth.SiteURL`과 같은 뜻(scheme과 host, 끝 `/` 하나는 뗀다, 경로가 있으면 오류)이고 로컬 URL은 `SiteURL` + 마지막 `Handler`의 `Prefix` + `/maago/media`다. `Handler`는 꼴만 본다. 로컬 라우트를 단 `Handler`가 없으면 `URL`은 오류(접두어 없는 링크가 조용히 나오지 않는다). 기각: 지금 그대로(일꾼이 완성 URL을 든다는 근거는 열쇠가 프로세스마다라 약하다), `LocalMedia.Prefix` 칸(맞출 값 둘)". 4번 고침의 "경로는 `Config.Prefix`와 같아야"는 원문이라 그대로 두고 이 고침이 뒤집는다.
      - D-122: "고침(M13 9번 고침, D-160): 로컬 미디어 URL도 접두어를 그 자리에서 붙인다. `Handler` 없이는 접두어 없는 값이 아니라 오류다."
      - 끝의 기록에 "**2026-11-25 (M13 9번 고침)** — D-160·D-122 고침(`LocalMedia.SiteURL`은 주소만, 접두어는 런타임이, 로컬 라우트를 단 `Handler` 전의 `URL`은 오류). 에이전트 편의로 결정(codex·fable 의견)"을 단다.
   8. `docs/TODO.md` 94줄: 체크하고 "→ M13 9번 고침: B(주소만, 접두어는 런타임, `Handler` 전의 URL은 오류), 게시판 0044는 `SiteURL: auth.SiteURL`".
   9. 끝 확인과 기록
      - 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`, `gofmt -l .`이 비어 있음.
      - PG 전체 시험: `MAAGO_TEST_PG_URL=postgres://maago@127.0.0.1:55432/m13_9b go test -timeout 60m ./... 2>&1 | tee 로그; echo "go test exit=${pipestatus[1]}"`(zsh, `$?`는 `tee`의 값이다)를 앞에서 끝까지 기다린다(약 10분, 명령 timeout 넉넉히). 로그의 `go test exit=` 줄로만 판단한다.
      - 새 시험은 `GOTOOLCHAIN=go1.25.14`로도 돌린다(CI의 Go).
      - 이 절에 기록 줄, README 8줄의 다음 작업을 "10번"으로(고침을 지움).
      - 구현 리뷰는 ponytail·codex·fable. 까닭: 공개 계약(`SiteURL`의 뜻)과 프로세스 상태의 가드, 서명된 URL(보안) 경로를 바꾼다.

   계획 리뷰는 codex·fable 둘(공개 API의 뜻과 뒤 작업 v0.4.0이 기대는 약속).

   계획 리뷰 codex·fable 반영(막는 것 없음): 전체 시험의 종료값을 `tee`가 아니라 `pipestatus[1]`로(codex 1). 실패한 `Handler`가 `Current`를 덮지 않는 줄과 헛것 (i)(codex 2, fable 6은 같은 까닭을 이 줄이 물어 따로 두지 않음). `TestLocalMediaURLServed`의 고치기 전 빨강은 옛 경로 검사 오류, 404는 헛것 (a)(fable 1). 가드 오류 글 끝에 HTTP 없는 프로세스는 버킷의 URL(fable 2). `MediaURL` 주석 한 문장(fable 3). topics는 LocalMedia 문장만(fable 4). 버킷 `Handler` 줄은 `/app` 로컬 줄 뒤, 헛것 (h)(fable 5). 받지 않은 것: 없음.

   기록(2026-11-25, 9번 고침): `rtcore.Process.LocalMedia`(마지막 `Handler`가 `/maago/media`를 달았는지), `handler.local`, `media()`는 `site()`의 꼴만, `LocalMedia.URL`은 열쇠 → `SiteURL` 빔 → `site()` → 가드 → `u.Path = Prefix + "/maago/media"`, `site()`는 auth의 식(`Scheme+"://"+Host`가 끝 `/`를 뗀 값과 같음). 주석: `SiteURL`·`URL`·`MediaURL`·`Config.Prefix`·`Process`. topics form 절의 LocalMedia 문장. 게시판 0044: main은 `SiteURL: auth.SiteURL`(`strings` import 뺌), `TestUnderApp`의 `cfg.Media` 덩어리 뺌, 커밋 글, MAP 125줄. D-160·D-122 고침과 기록 줄, TODO 94 체크, README 8줄. 확인: 바꾼 시험이 고치기 전 빨강(`TestLocalMediaURL`의 경로 거절·가드·접두어, `TestMediaDirConfig` 세 줄, `TestLocalMediaURLServed`는 옛 `has path "", not Config.Prefix`), 고친 뒤 초록(`GOTOOLCHAIN=go1.25.14`로도). 헛것 (a)~(i)마다 이름 붙인 시험이 빨강, 되돌린 뒤 diff 같음. (i)는 처음에 `Prefix: "app"`로 썼다가 초록이었다: `url.URL.String`이 host 뒤 경로에 `/`를 붙여 `app/maago/media`도 `/app/…`이 된다 → `Prefix: "/b"`와 `MaxBody: -1`로 바꿔 빨강(`https://example.com/b/maago/media`). (g) 0044 main을 `auth.SiteURL + "/app"`로 → `TestBoardTutorial` 빨강(`board with no bucket: serve: … LocalMedia.SiteURL "http://localhost:8000//app" is not a site's address`), 되돌림. `TestBoardTutorial$|TestBoardMap`(PG) 초록, `TestBoardTutorialCatches`(PG) 초록, 풀린 게시판 `go test ./...`(PG) 초록. `TestDoc$|TestNoDecisions*`·`TestPackageDoc` 초록(골든 그대로, `-update` 없음), 찍은 `maago doc form`에 `D-숫자` 없음(`example.com/app`은 예의 모듈 경로뿐). 끝 확인: `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 깨끗, `MAAGO_TEST_PG_URL=…/m13_9b`로 PG 전체 시험 `go test exit=0`(종료값은 `go test`의 것을 파일로, `cmd/maago` 571초)
   기록(2026-11-25, 9번 고침 구현 리뷰 반영): 구현 리뷰는 ponytail·codex·fable: 막는 것 없음(셋 다). 받은 것: D-160의 M13 4번 고침에 남은 "경로는 `Config.Prefix`와 같아야"를 "SiteURL의 뜻과 접두어는 아래 M13 9번 고침"으로(codex, 이번 결정과 어긋남), D-122의 고침 문장을 근거 칸 끝("사람의 결정, M4 11번" 앞)으로(fable 3, D-153의 관례). 받지 않은 것: `handler.local` 대신 `media()`가 `(bool, error)`(ponytail 1, 크기가 같다), 실패한 `Handler` 줄(ponytail 2, 헛것 (i)를 지킨다), 긴 기록 줄(ponytail 3, 이 레포의 꼴), `topics.go`의 "after its Handler, the last one the process made"(fable 1, `URL` 주석의 "the last Handler's"와 D-122의 대가가 이미 말하고 버킷 줄 시험이 문다), 게시판 `TestUnderApp`의 `/app/maago/media` 줄(fable 2, maago의 `TestLocalMediaURLServed`가 접두어를 문다). 런타임·게시판 코드는 고치지 않았다. 반영 뒤 루트에서 `go mod tidy -diff`·`go vet ./...`·`gofmt -l .` 깨끗, `MAAGO_TEST_PG_URL=…/m13_9b`로 PG 전체 시험 `go test exit=0`(`cmd/maago` 586초)

   하지 않는 것
   - `LocalMedia.Prefix` 칸이나 `SiteURL`에 경로를 받는 꼴(기각, 위).
   - 다른 프로세스의 일꾼이 쓸 URL(서명 열쇠를 나누기 따위): 열쇠는 프로세스마다이고(D-160) 그런 URL은 버킷의 것이다. 글은 그대로 둔다.
   - Catches에 0044 main의 `SiteURL` 줄을 더하기: 걷기 하나가 수 분이고 같은 꼴은 `ship`의 끝 `/` `serve`와 헛것 (g)가 본다.
   - `ship`이 서명된 URL을 실제로 받아 보기: 접두어 아래의 받기는 루트 `TestLocalMediaURLServed`(`StripPrefix("/app")`의 진짜 HTTP)가, 게시판의 받기는 `TestFileURLs`가 문다.
   - 복사된 auth의 `siteURL`과 식을 한 곳에 모으기: 복사본은 런타임을 import하지만 이 검사는 런타임의 공개 API가 아니다. 같은 거절 목록을 시험이 양쪽에서 문다.
   - `cmd/maago/docs/*.txt`·`doc.go` 골든: 바꿀 것이 없다(위 따라가 본 길).
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
- [ ] push 뒤 GitHub Actions(1번: `cmd/maago` 20분 아래, TODO 81줄에 잰 값을 적고 체크)
