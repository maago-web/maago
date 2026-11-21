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
6. **`contrib/s3`**: stdlib SigV4(머리 서명과 서명된 URL), Range GET, LIST(페이지 나눔), 크기 모르는 Reader → 확인: S3 문서의 예 벡터(머리 서명과 서명된 URL 둘), 4번의 저장소 시험 함수를 가짜 S3에(가짜는 서명을 다시 계산해 틀리면 403), 길이 모르는 Reader의 `Content-Length`, CI에 MinIO 서비스와 `MAAGO_TEST_S3_URL`(D-91의 꼴, 없으면 CI에서 실패), `TestRuntimeImports`
7. **고아 청소**(Q4): 태그 `media`, 생성된 "쓰는 중" 쿼리, `SweepMedia` → 확인: 오래된 고아만 지움, 여유 안의 새 것과 `.tmp-`, 태그 없는 프로젝트의 오류, 열쇠 열이 둘인 모델·두 앱에서 둘 다 쓰는 중으로 봄, 로컬과 가짜 S3 둘(LIST 페이지 둘 이상)
8. **게시판 0041~**: 일꾼이 만든 파일 여럿(내보내기 따위, 이미지 처리 아님: D-72)을 이름 → 열쇠 행으로 저장하고 뷰가 이름으로 찾아 준다(readmate의 `media/marked/<id>/` 꼴), 저장소를 환경으로 고름, 서명된 URL로 밖에 건넴, 청소(태그), outbox의 Kind 둘(웹훅과 이 일: `main`의 일꾼이 Kind로 가름), lease를 넘어 다시 도는 일이 남긴 파일은 청소가 치운다 → 확인: `TestBoardTutorial`·`TestBoardTutorialCatches`·`TestBoardMap`, Catches(일꾼 ctx에 저장소 없음, 태그 빠뜨림 따위)
9. **문서**: `maago doc`의 미디어(단일 인스턴스, 서명된 URL과 그 한계: 만든 프로세스만 받고 다시 시작하면 403, 청소)·main(일꾼의 저장소, Kind 둘)·model(태그 `media`), 결정 고침: D-140·D-153·D-159·D-123(루트 밖 공개 패키지의 셋째 `contrib/s3`)·D-09·D-72(앱이 만든 바이트는 이미지 처리가 아니다)·D-61·D-148·D-23·D-34·D-32·D-91·D-105·D-111·D-154·D-120·D-132, MILESTONES v1에서 옮김 → 확인: `TestDocExamples`, `TestPackageDoc`, `TestNoDecisionsInUserText`
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
