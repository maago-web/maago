# M5 작업 목록 — 인증 (계정 · 이메일)

[MILESTONES.md](MILESTONES.md)의 M5를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M5 완료 조건** (MILESTONES. D-123·D-130으로 고친 문구)
- 게시판 튜토리얼(M8)에서 이 마일스톤이 받는 커밋이 모두 통과한다 → 12번
- 가입 → 인증 메일 링크 → 로그인 → 비밀번호 재설정이 통합 테스트로 돈다 → 4·6·7번, 13번
- 인증 테이블이 사용자 프로젝트의 `makemigrations` 결과에 들어간다 → 2번
- User에 필드를 더한 프로젝트에서도 가입·로그인·staff만 들어가는 화면이 된다(admin은 M6) → 2·9번
- 옛 해시(다른 접두어나 적은 반복 수)로 로그인하면 새 해시로 다시 해싱된다 → 3번
- 로그인하면 세션 ID가 바뀐다 → 4번
- API 라우트를 토큰으로 호출할 수 있다 → 11번

**M4에서 넘어온 것** ([TASKS-M4.md](TASKS-M4.md) 끝, [TODO.md](TODO.md))
- 게시판의 "작성자만 고치고 지우기"와 로그인 필수 화면 → 12번(작성자는 `fk=auth.User`, 1번)
- 401에 `WWW-Authenticate`가 없다(D-121, 스킴은 M5) → 4번(코어)
- 메일 확인(U-07의 나머지, D-113) → 5번(`maagotest`의 메일함)
- `Auth`는 `func(r)`라 쿠키를 쓸 수 없다(M4 9번) → D-125, 4번
- 탈출구 뷰의 경로 값 정수(TODO M4 절)는 M8 그대로

---

## 먼저 확인한 사실 (2026-10-05, 리뷰가 코드로 다시 봄)

- **모델은 프로젝트 루트 아래의 파일만 읽는다.** `project.GoFiles`는 `go.mod` 루트에서 `./...`처럼 훑고 하위 모듈은 건너뛴다(`go list`가 아니다). 다른 모듈(이 저장소의 `contrib/…`, 모듈 캐시)의 모델은 `makemigrations`도 매퍼도 보지 못한다. 그리고 `*_gen.go`는 앱의 패키지 디렉터리에 써야 하는데 모듈 캐시는 읽기 전용이다
- **`fk=`는 같은 앱의 모델 이름만**(D-78, `internal/model/check.go`). 테이블 이름은 `패키지이름_모델`(D-76)이고 앱 사이의 테이블 겹침은 이미 오류다. PG 초안은 새 테이블의 FK를 모든 `CREATE TABLE` 뒤 `ALTER TABLE … ADD CONSTRAINT`로 쓰고 SQLite는 만들 때 FK를 보지 않아, 앱을 넘는 FK의 순서 문제는 없다. 매퍼 생성물은 FK 대상 타입을 부르지 않으므로 앱 사이 FK는 Go import를 만들지 않는다(고리도 Go가 막지 않는다)
- **타입 있는 뷰는 응답 머리를 쓸 수 없다.** `func(ctx, In) (P, error)`에는 `ResponseWriter`가 없고, `Set-Cookie`는 생성된 어댑터나 탈출구 뷰(D-41)만 쓴다. 로그인·로그아웃·세션 갈아 끼우기는 쿠키가 필요하다
- **템플릿은 `templates/` 바로 아래의 `.html`만**이고 하위 디렉터리는 생성 오류다(`internal/web/templates.go`). 생성 코드는 `text/template`을 쓰지 않는다(D-66)
- **빈 라우트 표는 생성 오류**(M4 2번). **401에 `WWW-Authenticate`가 없고**(`http.go`의 `fail`), CORS preflight는 `Content-Type` 밖의 요청 머리를 거절한다(`Authorization` 포함)
- **stdlib만으로 되는 것**(D-09): `crypto/pbkdf2`(Go 1.24, 루트 `go.mod`는 1.25), `crypto/rand`·`hmac`·`sha256`·`subtle`, `net/smtp`(동결, 동작). argon2·bcrypt·scrypt는 `golang.org/x/crypto`라 D-09를 고쳐야 한다. OWASP의 일반 권고 1순위는 argon2id이고 PBKDF2-HMAC-SHA256 600,000은 FIPS가 필요할 때의 권고다
- **걸리는 결정**: D-73 "프레임워크 코드를 사용자 리포에 복사하지 않는다", D-31 "세 방언을 감당하는 것은 Maago가 생성하는 SQL(CRUD·인증·admin)뿐", D-42 "템플릿 오버라이드는 v1, admin·인증 템플릿에", D-69(5분 흐름)

## 결정 (2026-10-05, 초안 리뷰 뒤 사람의 결정)

초안의 Q1~Q7과 "정한 것"을 codex·fable 리뷰 뒤 사람이 모두 추천대로 골랐다. 자세한 것은 DECISIONS.

- **D-123 contrib 앱(U-06의 답)**: `maago add auth`가 앱 층(모델·쿼리·라우트·뷰·폼·템플릿·시험)을 프로젝트의 `auth/`로 복사, 보안의 모양을 가진 것은 모두 `contrib/auth`(stdlib만). 커스텀 User는 복사된 `User`에 필드를 더하는 것 하나. 복사되는 SQL은 두 방언에서 돈다. D-73·D-31·D-42를 고침
- **D-124** `fk=앱.모델`(앱은 Go 패키지 이름, 같은 앱 안에서도 됨, 고리는 막지 않음). D-78을 고침
- **D-125** `maago.SetCookie(ctx, c)`, 어댑터가 응답 전에 적용
- **D-126** 메일 본문은 복사된 앱의 Go 문자열, `auth.Mail` 패키지 변수(`auth.SMTP`·`auth.ConsoleMail`), `maagotest.Mailbox(t)`
- **D-127** PBKDF2-SHA256(stdlib), Django 글 꼴, 600,000, 최대 4096바이트, 옛 해시는 로그인 때 다시, 더미 해시
- **D-128** 세션(DB, SHA-256, 쿠키 속성, `Path`는 `maago.Prefix()`), 로그인·변경·재설정 때의 세션, 일회 토큰은 지운 행 수로 한 번, 인증 전 로그인 막음, `next` 규칙, 같은 응답, 시도 제한(이메일·`RemoteAddr`, 15분 10번, `X-Forwarded-For` 불신)
- **D-129** 개인 접근 토큰, Bearer, API 401의 `WWW-Authenticate: Bearer`, `Config.Origins`의 preflight에 `Authorization`. D-121·D-47을 고침
- **D-130** `IsStaff`·`IsActive`만, staff 검사는 뷰의 한 줄, 권한·그룹·`IsSuperuser`는 M6, 탈퇴는 비활성, 묻지 않는 `-createsuperuser`
- scaffold 자리(`contrib/auth/testdata/scaffold/`, embed), scaffold 시험(임시 프로젝트, 두 방언)은 2번 계획에서 확인한다

## 모양

```
 이 저장소                                         사용자 프로젝트
 contrib/auth/testdata/scaffold/ ─ maago add auth ─▶ auth/  models.go(User·Session·Token·APIToken·LoginAttempt)
                                                          queries/*.sql, urls.go, views.go, forms.go, templates/, *_test.go
 contrib/auth (stdlib만) ◀──────── import ───────── auth/views.go
   Hash·Check, NewToken·HashToken, SafeNext,               main.go: Config{Apps: {board.App, auth.App},
   SessionCookie, FromSession(…), Limit, Mail·SMTP                   Auth: auth.FromSession(…), LoginURL: …}
 maago     SetCookie(ctx, c), API 401의 WWW-Authenticate, preflight의 Authorization
 maagotest Mailbox(t)
```

- 런타임 공개 API가 루트 하나라는 원칙(ARCHITECTURE §2)의 둘째 예외가 `contrib/auth`다(첫째는 `maagotest`). `contrib/auth`는 사용자 타입을 모른다(D-51): 복사된 앱이 함수로 넘긴다(`FindSession` 등)

---

## 1. 다른 앱의 `fk=` (D-124)

`fk=앱.모델`을 모델 읽기가 모든 앱을 안 뒤에 푼다. 오류: 없는 앱·모델, 패키지 이름이 같은 두 앱(이미 테이블 겹침으로 잡히는지 확인). FK 순서는 할 일이 없다(사실 절).
→ 확인: 두 앱 프로젝트의 `makemigrations`·`check`·`migrate`(두 방언), 오류 표

## 2. `maago add`와 scaffold (D-123)

`cmd/maago add <앱>`: embed한 scaffold를 `<앱>/`에 쓴다. 이미 있는 파일이 하나라도 있으면 아무것도 쓰지 않고 거절. 처음 scaffold는 모델(User·Session·Token·APIToken·LoginAttempt)과 최소 라우트 하나(빈 라우트 표는 오류). scaffold 시험 도구(임시 프로젝트, 두 방언).
→ 확인: `add auth` → `makemigrations`에 인증 테이블(완료 조건 3), User에 필드를 더해도 통과, 있는 파일이면 거절

## 3. 비밀번호 해싱 (`contrib/auth`, D-127)

`Hash`·`Check(password, encoded) (ok, rehash bool)`, 더미 해시, 최대 길이. 복사된 앱의 로그인이 `rehash`면 새 해시를 쓴다.
→ 확인: Django가 만든 해시 벡터, 적은 반복 수·`pbkdf2_sha1` 해시로 로그인하면 바뀐다(완료 조건 5)

## 4. 쿠키 자리(코어), 세션, 로그인·로그아웃 (D-125, D-128)

코어: `maago.SetCookie`(D-125), API 401의 `WWW-Authenticate: Bearer`, preflight의 `Authorization`(D-129). `contrib/auth`: 세션 쿠키·`FromSession`·`SafeNext`·시도 제한. 복사본: `Session` 쿼리, 로그인 폼·뷰, 로그아웃(POST).
→ 확인: 로그인 전후 쿠키 값이 다르다(완료 조건 6), 로그아웃 뒤 이전 쿠키는 로그인하지 않은 것, 만료, `next`의 `//evil`·`https://…`·`/\evil`는 기본 자리로, 없는 이메일과 틀린 비밀번호가 같은 응답

## 5. 메일 (`contrib/auth`, `maagotest`, D-126)

`auth.Mail`·`Message`·`SMTP`·`ConsoleMail`, `maagotest.Mailbox(t)`.
→ 확인: 가짜 SMTP 서버(`net` 리스너)로 한 통, 메일함 시험

## 6. 비밀번호 검증기, 가입과 이메일 인증 (D-128)

검증기는 Go 함수 목록(D-15: 길이, 흔한 비밀번호 작은 목록, 이메일과 비슷함). 가입 폼(모델 폼, 이메일 중복은 뷰의 쿼리 D-119, 같은 응답), 인증 토큰 메일, 링크로 활성.
→ 확인: 가입 → 메일함의 링크 → 활성 → 로그인(maagotest), 인증 전 로그인 거절, 같은 링크 두 번

## 7. 비밀번호 변경과 재설정

변경(로그인 필수, 옛 비밀번호, 세션 갈아 끼우기), 재설정(메일 링크, 한 번, 모든 세션과 다른 재설정 토큰을 지움, 없는 이메일에도 같은 응답).
→ 확인: 재설정 흐름, 같은 링크 두 번은 거절, 동시 두 요청 중 하나만

## 8. 시도 횟수 제한

로그인·재설정 요청·인증 메일 다시 보내기에 이메일별·IP별 창.
→ 확인: 11번째 시도 429, 창이 지나면 다시

## 9. staff·비활성·탈퇴·`createsuperuser` (D-130)

`IsStaff`·`IsActive`, 뷰의 staff 검사 예, 탈퇴(비활성 + 세션·토큰), `-createsuperuser`.
→ 확인: staff만 들어가는 화면(완료 조건 4의 몫), 비활성 사용자는 로그인 못 함

## 10. (9번에 합침)

## 11. API 토큰 (D-129)

토큰 만들기(평문 한 번)·지우기 화면, Bearer, 다른 출처 프런트엔드의 preflight와 실제 요청.
→ 확인: 토큰으로 API 라우트(완료 조건 7), 지운 토큰은 401과 `WWW-Authenticate`

## 12. 게시판 튜토리얼의 M5 단계 (D-103)

`maago add auth`(그 뒤 `maago gen`이 `auth.App`을 쓰라고 막는 것까지 단계의 글), `main`(`auth.App`·`Auth`·`LoginURL`·메일·`-createsuperuser`), `Post.AuthorID fk=auth.User`(마이그레이션), 로그인 필수 쓰기, 작성자만 고치고 지우기(뷰의 Go 코드, 아니면 403), 지우기는 댓글이 없으면 진짜 지우기·있으면 숨기기(D-84), 가입·로그인 화면. 시험은 낮은 반복 수와 메일함. 댓글의 작성자는 계획에서.
→ 확인: `TestBoardTutorial`(SQLite)

## 13. 완료 조건 종단 테스트와 문서

`TestM5Done`(두 방언): 완료 조건마다 성공과 실패 경로. DECISIONS(초안의 D-123~D-130에서 만들며 바뀐 것), ARCHITECTURE §2(둘째 예외)·§5(복사된 앱), TUTORIAL-FUTURE §5(`maago add auth`, `-createsuperuser`), MILESTONES(할일을 D-번호 한 줄로, M8 표의 JWT·contrib User 줄, M6의 "모델별 권한(M5)").
→ 확인: 완료 조건마다 시험 이름, CI에서 PG

---

## M5 끝

- [ ] 1~13 완료
- [ ] 위 M5 완료 조건을 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로)
- [ ] MILESTONES.md의 M5 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M6 초안

## 초안 리뷰 (2026-10-05)

- codex: 받음 — 2번은 `add` 명령과 scaffold를 새로 만드는 일(1), FK와 scaffold의 순서(2: 사실을 고쳐 1번은 작아졌다), 쿠키 `Path`는 `Prefix`(3), `next` 검사(4), 다른 출처 Bearer와 CORS(5, Q5), 더미 해시·같은 응답·IP의 출처(6), 지운 행 수로 한 번만 쓰기와 재설정 때 다른 토큰(7), PBKDF2를 사람에게(8, Q4), 메일 템플릿(9, Q3), 완료 조건과 시험의 대응(10, 13번)
- fable: 받음 — D-73·D-42가 P1(a)와 부딪힘(1), 보안의 모양을 가진 것은 모두 라이브러리로(2), scaffold의 SQL은 두 방언(3, D-31), scaffold는 `testdata/`에(4), `--diff`는 만들지 않음(5), 메일 하위 디렉터리는 지금 오류(6), 빈 라우트 표 오류(7), FK 순서는 이미 문제없음(8), FK는 Go import를 만들지 않음(9), `WWW-Authenticate`는 코어(10), 타입 있는 뷰는 쿠키를 못 씀(11, Q2), 반복 수 손잡이·Django 벡터(12), `Secure` 늘·변경과 재설정의 세션·만료 행(13), 쓴 시각 대신 지우기(14), IP의 출처(15), staff는 뷰 한 줄(16, Q6), 메일 설정 자리(17, Q3), 묻지 않는 `createsuperuser`(18), `next` 규칙(19), 완료 조건 ※ 문구(21), MILESTONES 고칠 곳(22), 검증기를 가입보다 먼저(23), 지우기와 D-84(24), 같은 앱의 `앱.모델` 허용(25)
