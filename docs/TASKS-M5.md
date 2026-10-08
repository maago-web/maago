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
- scaffold 자리(2번 계획에서 `internal/scaffold/testdata/<앱>/`, embed), scaffold 시험(임시 프로젝트, 두 방언)은 2번 계획에서 확인한다

## 모양

```
 이 저장소                                         사용자 프로젝트
 internal/scaffold/testdata/auth/ ── maago add auth ─▶ auth/  models.go(User·Session·Token·APIToken·LoginAttempt)
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

### 계획

있는 것: `model.Field.FK`는 같은 앱의 모델 이름이고, 방언 코드가 앱마다 만든 이름→테이블 표(`dialect.Tables(app)`)로 `REFERENCES`의 테이블을 찾는다(SQLite `table`·초안의 재생성, PG `foreignKeys`). 그 밖의 소비자는 "FK인가"만 본다(`CheckNames`의 제약 이름, FK 열의 자동 인덱스). 다른 디렉터리에 같은 패키지 이름의 앱은 지금도 된다(테이블이 겹치면 오류).

- **`Field.FK`의 뜻을 바꾼다: 가리키는 모델의 테이블 이름.** `model.Check`가 모든 앱의 모델(그 `table=`까지)을 만든 뒤 한 번에 푼다. 그러면 `dialect.Tables`와 방언 코드의 표 인자가 사라진다(쓰는 곳이 없어지므로 지운다)
- `fk=` 값의 꼴: `모델`(같은 앱, 지금 그대로) 또는 `앱.모델`(D-124, 같은 앱이어도 된다). 앱은 Go 패키지 이름
- 오류(지금의 같은 앱 오류는 문구·자리 그대로):
  - `fk=autth.User`: 없는 앱 → `unknown app "autth" in …` + `did you mean "auth"?`(있는 앱 이름 목록, 지금의 `c.unknown`)
  - `fk=auth.Usr`: 그 앱에 없는 모델 → 그 앱의 모델 목록으로 `did you mean`
  - `fk=blog.Post`인데 `package blog`인 앱이 둘: `app name "blog" is ambiguous: blog/ and shop/` + fix "rename one package, or …"(가리킬 때만 오류, 앱이 겹치는 것 자체는 그대로 된다)
  - `fk=a.b.C`·`fk=.User`·`fk=auth.`: 꼴 오류(`name a model, like fk=Post or fk=auth.User`)
  - 형(`int64`) 검사는 지금처럼 이름 풀기와 따로(둘 다 틀리면 둘 다)
- 고리: 막지 않는다(D-124). 같은 모델을 가리키는 것(`fk=Post` 안의 `ParentID`)도 지금처럼 된다
- 매퍼: FK는 열 하나(`int64`)일 뿐이라 바뀌는 것 없음. `RecentPosts`처럼 모델 결과는 열을 그대로

파일
- `internal/model/check.go`(풀기, `Field.FK` 주석), `internal/dialect/dialect.go`(`Tables` 지움), `internal/dialect/sqlite/{sqlite,draft}.go`·`internal/dialect/postgres/postgres.go`(표 인자 지움, `f.FK` 그대로)
- 시험: `internal/model/model_test.go`의 오류 골든(새 경우들)과 성공 경우(`fk=auth.User`의 `Field.FK == "auth_user"`, `table=`를 쓴 대상), `cmd/maago`에 두 앱 프로젝트(`blog.Post.AuthorID fk=auth.User`, `auth`는 `table=`가 없는 것과 있는 것)의 `makemigrations` → `check` → `migrate`를 두 방언으로, 모델과 DB가 같은지(`TestM2Done`의 비교)

하지 않기로 한 것
- `ondelete=`(D-84 그대로), FK 대상의 PK가 `ID int64`가 아닌 경우(지금도 모든 모델의 PK는 `ID int64`)

단계
1. `Field.FK`를 테이블 이름으로, 방언 코드의 표 지우기 → 확인: 지금의 시험이 그대로 통과(뜻만 바뀌고 출력은 같다)
2. `앱.모델` 풀기와 오류 → 확인: 오류 골든, 성공 경우
3. 두 앱 프로젝트의 마이그레이션 → 확인: 두 방언
4. 두 모듈 확인 명령

계획 리뷰 (codex·fable) — 위 계획을 이렇게 고친다
- 표를 쓰는 자리에 `internal/dialect/draft.go`의 `Model`(둘째 반환값 `Tables(app)`)도 있다: 그것도 지운다(codex 1, fable 8). SQLite 재생성(`rebuilt`)이 다른 앱을 가리키면 지금은 `REFERENCES ""`가 된다
- 형 검사와 이름 풀기는 지금 한 `switch`라 하나만 나온다. 풀기를 미루면 둘이 독립이 되어 둘 다 틀리면 둘 다 낸다(D-61, codex 3·fable 2). 골든에 둘 다 틀린 필드 하나
- FK 열의 자동 인덱스는 "FK인가"만 보므로, `c.model` 때 적힌 대상을 임시로 넣고 모든 앱을 안 뒤 테이블 이름으로 바꾼다(codex 2)
- 점 꼴 오류는 따로 써야 한다(태그의 맨 값은 `.`를 받는다): `a.b.C`·`.User`·`auth.`는 지금의 꼴 오류 문구, fix는 하나로 "name a model, like fk=Post or fk=auth.User"(fable 3)
- 겹치는 앱 이름: FK 값의 자리에, 후보 디렉터리를 정렬해 `, `로, fix "rename one package, or write fk=Post from inside the app", 없는 앱의 did-you-mean 목록은 겹친 이름을 한 번만(codex 5, fable 6). 앱 이름은 Go 패키지 이름 그대로 비교(대문자도, `Snake`하지 않는다)
- 대상 모델의 `table=`가 틀려도 그 모델의 기본 테이블로 풀린다(그 모델의 오류는 따로 나니 이어지는 오류가 없다, codex 4·fable 7)
- 시험 더함: 1단계는 `valid.golden`이 바뀐다(`fk=Author` → `fk=people`, DDL·초안은 같다, fable 1). 오류 골든의 "다른 앱의 없는 모델"은 셋째 디렉터리(고유한 패키지 이름)에(fable 5). `cmd/maago`의 두 앱 시험에 대상 앱의 `table=`, 서로 가리키는 두 앱, 그리고 **다른 앱을 가리키는 테이블의 SQLite 재생성**(제자리에서 더할 수 없는 CHECK, `max=`를 더해 재생성을 강제. 기본값 없는 NOT NULL은 행이 있는 테이블에 적용할 수 없다) 뒤 행과 `check`(codex 6, fable 4). 두 앱을 쓰는 작은 도우미

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: `valid.golden`의 `fk=Author` → `fk=people`(대상의 `table=`), 오류 골든의 새 경우 일곱, `TestCrossAppFK`(대상 테이블·자동 인덱스), `TestCrossAppMigrations`(두 방언, 서로 가리키는 두 앱, 행을 넣은 뒤 SQLite가 `blog_post`를 실제로 재생성)
- ponytail(스스로·fable): 지울 것 없음. `ref` 목록이 D-61의 독립과 값의 자리를 함께 주는 가장 작은 모양
- codex: 받음 — 풀리지 않은 FK는 비운다(1: 적힌 이름이 테이블처럼 남지 않게, `lookup`이 ""를 돌려주고 `TestUnresolvedFK`), 점 꼴 `a.b.C`·`.User`도 골든에(2). 받지 않음 — 3 기본값 없는 NOT NULL로 재생성: 같은 `rebuilt`를 CHECK가 지나고, 그 필드는 행이 있는 테이블에 적용할 수 없다
- fable(변이 여섯 모두 잡힘): 받음 — 떨어진 `tables` 주석(1), `dialect.Model` 주석 줄 바꿈(2), 모호함 문구 "ambiguous: blog/, shop/"(3), 점 꼴 오류의 자리를 같은 계열처럼 키에(4), fix는 "rename one of the packages"만(5: 셋째 앱에서 쓴 FK에는 "안에서 fk=Post"가 맞지 않는다), 계획의 재생성 문장을 시험과 맞춤(6)

## 2. `maago add`와 scaffold (D-123)

`cmd/maago add <앱>`: embed한 scaffold를 `<앱>/`에 쓴다. 이미 있는 파일이 하나라도 있으면 아무것도 쓰지 않고 거절. 처음 scaffold는 `User` 모델 하나, 라우트 없음(세션·토큰 모델과 라우트는 그것을 쓰는 4·6·11번이 더한다). scaffold 시험 도구(임시 프로젝트, 두 방언).
→ 확인: `add auth` → `makemigrations`에 `auth_user`(완료 조건 3의 첫 몫, 모두는 13번), User에 필드를 더해도 통과, 있는 파일이면 거절

### 계획

- **자리**: 루트 모듈의 `internal/scaffold`(`//go:embed testdata`, 앱마다 `testdata/<앱>/`). `cmd/maago`는 import 경로가 루트 아래라 `internal`을 쓸 수 있다(지금도 `internal/build`). `testdata`라 go 도구가 그 안의 `.go`를 컴파일하지 않고, 사용자 바이너리에 따라가지 않는다(`contrib/auth` 라이브러리와 따로). 결정의 `contrib/auth/testdata/scaffold/`를 이렇게 고친다: `cmd/maago`(다른 모듈)는 루트 모듈의 디렉터리를 embed할 수 없고, `contrib/auth`에 두면 라이브러리를 import한 사용자 바이너리가 scaffold를 싣는다
- **`scaffold.Add(root, app string) ([]string, []diag.Diagnostic)`**: 모르는 앱은 오류(`did you mean`, 있는 앱 목록). `root/<앱>/` 아래에 쓸 파일 중 하나라도 이미 있으면 아무것도 쓰지 않고 그 파일들을 오류로(D-123). 쓰기는 모두 확인한 뒤. 쓴 파일 목록을 돌려준다. 생성 표시 없음(사람의 파일, D-67)
- **CLI** `maago add <앱>`: 명령 표에 한 줄. 인자 없음·둘 이상은 사용법 오류(2). 쓴 파일을 한 줄씩. 루트는 `gen`처럼 작업 디렉터리에서 `go.mod`까지
- **처음 scaffold**: `auth/models.go`의 `User` 하나(세션·토큰·API 토큰 모델은 그것을 쓰는 4·6·11번이 더한다): `ID`, `Email`(`max=254,unique,label='Email'`), `PasswordHash`, `IsActive`(`default=true`, 탈퇴·관리자가 끔, D-130), `IsStaff`(`default=false`), `VerifiedAt *time.Time`(이메일 인증, D-128. 인증 전과 탈퇴를 가른다), `CreatedAt`(`auto=create`). 라우트는 아직 없다(빈 라우트 표는 오류이고 라우트가 생기면 `main`이 `auth.App`을 써야 한다, 4번). 문구는 영어(D-06과 같은 기본, 사용자가 고친다)
- **시험**(`cmd/maago`): 빈 프로젝트에 `add auth` → `makemigrations`에 `auth_user` → `check` 통과, 두 방언. `User`에 필드를 더한 뒤 다시 `makemigrations`·`check`(완료 조건 4의 첫 몫). 다시 `add auth`는 거절하고 아무것도 바꾸지 않음(스냅숏). 모르는 앱. CLI의 사용법 오류. scaffold의 `.go`가 gofmt인지(`gofmt -l`이 `testdata`를 보지 않으면 시험이 본다)

하지 않기로 한 것
- `--force`·덮어쓰기·`--diff`(D-123: 빈 디렉터리에 다시 `add`하고 `git diff --no-index`), 앱 이름 바꾸기(`add auth --as users`)

단계
1. `internal/scaffold`와 `User` → 확인: 단위 시험(쓰기·거절·모르는 앱)
2. `maago add` → 확인: CLI 시험
3. 두 방언의 `makemigrations`·`check`, 필드 더하기 → 확인: `cmd/maago` 시험
4. 두 모듈 확인 명령

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-07)**: `VerifiedAt *time.Time`을 둔다(D-130 고침). 로그인은 `IsActive && VerifiedAt != nil` 하나로 4·6·9번이 같이 쓰고 `createsuperuser`가 채운다. 복사본의 문구는 영어(D-123에 한 줄)
- 자리의 근거를 고친다(codex 5, fable 8): `cmd/maago`는 중첩 모듈이어도 import 경로가 `github.com/maago-web/maago` 아래라 `internal`을 쓸 수 있다(지금도 `internal/build`). 무게는 `internal`이라는 것과 `contrib/`가 아직 없다는 것. "모양" 그림의 자리도 고친다. D-123 본문은 자리를 말하지 않아 DECISIONS는 그대로
- `Email`의 `label='Email'`은 뺀다(기본 라벨이 필드 이름, fable 4). gofmt 시험은 뺀다: `gofmt -l .`은 `testdata`도 본다(fable 3). `TestNoReflect`는 `testdata`의 `.go`도 읽고 `TestGeneratedImports`는 생성 표시가 있는 파일만 본다(codex 6): scaffold는 올바른 Go이고 표시가 없으면 된다
- **시험이 scaffold를 컴파일한다**(fable 1, codex 7): `add auth` 뒤 임시 프로젝트(`webProject`처럼 `go.work`)에서 `go vet ./...`. `testdata`는 go 도구가 보지 않아 이것이 없으면 import·형 오류가 사용자의 `go build`에서야 드러난다
- **CLI**(fable 2, codex 3): 지금 `run`은 모든 명령의 위치 인자를 거절한다. `command`에 위치 인자 하나를 받는 표시를 더하고 사용법에 `add <app>`. 종료 코드: 성공 0, 인자 없음·둘 이상 2, 모르는 앱·있는 파일·쓰기 실패 1(진단은 stderr, 쓴 파일은 stdout). `TestRun`의 꼴로
- **쓰기**(codex 2·4, fable 9): 먼저 쓸 자리마다 `Lstat`로 확인해 무엇이든(파일·디렉터리·링크) 있으면 모두 진단하고 아무것도 쓰지 않는다. 쓰기는 `O_CREATE|O_EXCL`(확인과 쓰기 사이의 경쟁). 도중에 실패하면 이번에 쓴 파일을 지우고 진단. 진단의 자리는 루트 기준 경로(D-88), fix는 D-123의 길("run maago add in an empty directory and compare with git diff --no-index"). 임시 디렉터리를 거치는 단계는 두지 않는다
- 필드 속성은 두 방언의 `makemigrations`가 쓴 SQL에서 본다: `email` 길이·unique, `is_active` 기본 참, `is_staff` 기본 거짓, `verified_at` NULL 허용(codex 8)
- 이미 `add`한 프로젝트에 나중 작업의 scaffold를 넣는 길은 없다(D-123의 대가, 지금 쓰는 것은 이 저장소의 시험과 12번뿐, fable 10). scaffold는 그대로 복사한다: 앱은 패키지 하나라 모듈 경로를 채울 일이 없다(fable 11). 앱 이름은 embed한 목록에 있어야 해서 경로를 넘을 수 없다(fable 12)
- 6번에: `Email`의 unique는 대소문자를 가린다. 가입·로그인이 소문자로 바꿀지 그 계획에서(fable 13)

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: scaffold에 컴파일 오류를 넣으면 `TestAddAuth`의 `go vet`이 잡는다, 두 방언의 SQL 속성, 하위 디렉터리에서 루트 찾기. 확인 명령의 `go test … | grep`이 실패 코드를 가려 `TestUsage` 실패 뒤에도 끝 줄이 나왔다: 이제 종료 코드를 따로 본다
- ponytail(스스로·fable): 지울 것 없음. `Apps()`는 embed한 디렉터리에서(앱이 늘면 스스로 들어온다)
- codex: 받음 — 실패하면 이번에 만든 빈 디렉터리도 지움(1: `undo`, 실패한 파일은 디렉터리만 넘긴다. 그사이 다른 것이 생겼을 수 있다), `Lstat`의 다른 오류는 "볼 수 없음"으로 따로(2). 받지 않음 — 3 여러 파일의 중간 실패·`O_EXCL` 경쟁 시험: 지금 scaffold는 파일 하나라 만들 수 없다. 파일이 늘어나는 4번에서 중간 실패 시험을 더한다. 4 실행 환경 문제
- fable: 받음 — 거절 뒤 그 파일이 그대로인지(1), 복사되는 파일의 D-번호를 빼고 `maago add auth`를 한 줄로(3: 사용자의 프로젝트에는 DECISIONS가 없다), ARCHITECTURE §2의 `internal/scaffold`와 `contrib/auth`의 뜻·§3의 `add`(4), 사용법의 "add add"(5), "something is in the way"(6: 디렉터리·링크도). 되돌리기 코드는 둔다(2: 4번에서 시험이 생긴다)

## 3. 비밀번호 해싱 (`contrib/auth`, D-127)

`Hash`·`Check(password, encoded) (ok, rehash bool)`, 더미 해시, 최대 길이. 복사된 앱의 로그인이 `rehash`면 새 해시를 쓴다.
→ 확인: Django가 만든 해시 벡터, 적은 반복 수·`pbkdf2_sha1` 해시로 로그인하면 바뀐다(완료 조건 5)

### 계획

새 패키지 `github.com/maago-web/maago/contrib/auth`(루트 모듈, stdlib만, D-123의 둘째 예외). 이 작업은 해싱만, 로그인에서 쓰는 것은 4번.

- `var Iterations = 600_000`: 새 해시의 반복 수(D-127). 시험·튜토리얼이 낮춘다(패키지 변수, M4 12번의 관례)
- `Hash(password string) (string, error)`: `pbkdf2_sha256$반복$소금$해시`. 소금은 `crypto/rand`의 영숫자 22자(Django와 같은 꼴·엔트로피 약 131비트), 해시는 32바이트의 표준 base64. 비밀번호가 4096바이트를 넘으면 `ErrTooLong`(긴 입력 DoS, D-127)
- `Check(password, encoded string) (ok, rehash bool)`: `pbkdf2_sha256`과 읽기만 하는 `pbkdf2_sha1`(Django의 옛 해셔, 20바이트). 비교는 `subtle.ConstantTimeCompare`. `ok`이고 (접두어가 `pbkdf2_sha256`이 아니거나 반복 수가 `Iterations`와 다르면) `rehash`(Django의 `must_update`와 같다). 모르는 꼴·깨진 글·4096바이트 넘는 비밀번호는 `false, false`
- **더미 해시**: `encoded`가 빈 글이면(계정이 없을 때 뷰가 그대로 넘긴다) 같은 비용의 PBKDF2를 한 번 돌리고 `false, false`(D-127·D-128의 "없는 계정에도 더미 해시"). 형식이 틀린 해시도 같은 비용을 치른다(빨리 끝나면 계정의 상태가 드러난다)
- 오류 값은 `ErrTooLong` 하나. 빈 비밀번호의 거절은 검증기(6번)의 일

파일: `contrib/auth/password.go`, `contrib/auth/password_test.go`

시험
- 벡터: Django의 알고리즘(`hashlib.pbkdf2_hmac` + base64, Django 해셔의 `encode`와 같은 식)으로 Go 밖에서 만든 `pbkdf2_sha256`·`pbkdf2_sha1` 해시 각각을 `Check`가 받는다(`uv run python`으로 한 번 만들어 시험에 박는다, 만든 명령을 주석에)
- `Hash` → `Check`가 참, 다른 비밀번호는 거짓, 같은 비밀번호의 두 해시는 다르다(소금)
- `rehash`: 낮은 반복 수의 sha256, sha1 → 참. 지금 반복 수 → 거짓
- 더미·깨진 해시·모르는 접두어·숫자가 아닌 반복 수·4097바이트: `false, false`. 더미와 깨진 해시도 PBKDF2를 한 번 도는 것은 코드의 모양으로 지킨다(모든 길이 한 함수 `derive`를 한 번 부르고 결과를 비교한다). 시간을 재는 시험은 두지 않는다(기계마다 흔들린다)

하지 않기로 한 것: 페퍼(D-127), argon2(D-127), 해시 꼴의 이주 명령(로그인 때 다시 해싱으로 충분)

단계
1. `Hash`·`Check`·`Iterations`·`ErrTooLong` → 확인: 위 시험
2. 두 모듈 확인 명령, ARCHITECTURE §2의 `contrib/auth` 줄은 2번에서 고쳤다

계획 리뷰 (codex·fable)와 사람의 결정 — 위 계획을 이렇게 고친다
- **사람의 결정(2026-10-07)**: 다시 해싱은 반복 수가 다르면 양쪽(Django의 `must_update`. Django 6.1의 1,500,000 해시도 600,000으로 바뀐다), 저장된 해시의 반복 수는 1~10,000,000만(그 밖은 깨진 해시, 이 기계에서 10M ≈ 1초), `auth.Iterations`는 둔다(D-127 고침)
- **깨진 해시의 비용**(codex 1·3·5, fable 2): 빈 글·모르는 접두어·필드가 넷이 아님·반복 수가 숫자가 아니거나 범위 밖은 모두 **고정 소금과 지금의 `Iterations`**로 더미를 한 번 돌리고 `false, false`. 공격자가 준 소금·반복 수·길이는 더미 비용에 쓰지 않는다
- **비교**(fable 5, codex 4·5의 뜻): Django의 `verify`처럼 읽은 소금·반복 수로 다시 만든 글 전체를 `subtle.ConstantTimeCompare`. base64를 풀지 않아 그 분기가 없고, 접두어마다 길이(32·20)가 한 곳에 산다. 꼴은 `strings.SplitN(encoded, "$", 4)`가 정확히 넷(Django와 같다). 소금은 읽을 때 `$` 없는 비지 않은 글이면 된다(옛 Django의 12자 소금도)
- **소금**(fable 4): 만들 때 `crypto/rand.Text()`(26자 base32, 128비트 이상). Django의 `decode`는 `$`로 나누기만 하니 둘 다 읽는다. 손으로 짠 영숫자 뽑기는 두지 않는다
- **벡터**(fable 6): 진짜 Django(`uv run --with django`, 6.1.2)가 만든 `pbkdf2_sha256`·`pbkdf2_sha1` 해시(1,500,000번, 명령과 버전을 주석에)와 빠른 길의 낮은 반복 수 벡터 하나. Django 벡터는 `ok, rehash`가 `true, true`(반복 수가 다르다)
- **길이**(codex 6, fable 8): 4096은 UTF-8 바이트, 정규화하지 않는다. 길이 거절은 공격자의 입력에만 달려 계정 상태를 새지 않으므로 더미 없이 일찍 돌아간다. 4096·4097바이트와 여러 바이트 글자 경계를 시험
- **`Iterations`**(codex 2, fable 9): 주석에 "시험·튜토리얼이 서버를 띄우기 전에 낮춘다, `main`은 건드리지 않는다". 1보다 작으면 `Hash`가 panic. 시험 바이너리마다 따로 낮춘다(4·12번 계획에)
- `Hash`의 오류는 `ErrTooLong`과 `pbkdf2.Key`가 돌려주는 것(FIPS 전용 모드). `Check`는 오류를 깨진 해시로(fable 10)
- **stdlib만 게이트**(fable 3): 루트의 `TestRuntimeDependencies`가 `./contrib/auth`도 본다(D-123의 약속을 시험이)
- ARCHITECTURE §2의 "유일한 예외"(maagotest)를 "예외 둘(`maagotest`, `contrib/auth`)"로 지금 고친다(fable 11)
- 4번에 넘김(fable 7·12): `encoded == ""`(계정 없음·쓸 수 없는 비밀번호)는 같은 비용으로 거절, `IsActive && VerifiedAt != nil`은 `Check` 뒤에(앞이면 응답 시간이 계정 상태를 말한다). 복사본도 `package auth`라 라이브러리를 부를 때 별칭(예 `authlib`)을 4번 계획에서 정한다
- 하지 않기로 한 것에 더함: Django의 `must_update_salt`(fable 13)

구현 리뷰 (ponytail·codex·fable)
- 만들며 확인한 것: Django 6.1.2가 만든 벡터 넷(다시 만드는 명령을 주석에), `-race`, 변이 다섯(sha1의 다시 해싱 조건, 빈 소금, `Check`의 길이 검사, 반복 수 상한, 저장값 길이 상한)이 모두 잡힌다. 더미가 실제로 도는 것과 상수 시간 비교는 결과로 보이지 않아 코드의 모양으로 지킨다(계획대로)
- ponytail(fable 4): `algorithm` 구조체 대신 이름 → 해시 함수 맵, 길이는 `h().Size()`
- codex: 받음 — 저장값 전체 길이 상한 256바이트(1: 다만 지적의 전제는 틀렸다. 소금은 PBKDF2의 첫 블록에만 쓰여 반복마다 들지 않는다. 그래도 싸서 둔다), 반복 수가 다른 옛 해시는 첫 로그인까지 계정이 있음을 시간으로 말한다는 한계를 주석에(2: 사람이 고른 "다르면 양쪽"의 대가), `Iterations`도 1~10,000,000 밖이면 panic(3), 상한·긴 값 시험(4). 받지 않음 — 4의 더미가 도는지 세는 훅·경쟁 시험(계획: 시간·훅 없이 모양으로)
- fable: 받음 — 가드가 없으면 통과했을 해시로 거절을 시험(1: 변이 넷이 살아남았다), 다시 만들 수 있는 벡터 명령(2), `Check` 주석의 "같은 비용"을 지금 해시에 한정(3, codex 2와 같다), 맵(4), `Check`도 panic할 수 있음을 주석에(5), 문구(6)

## 4. 쿠키 자리(코어), 세션, 로그인·로그아웃 (D-125, D-128)

코어: `maago.SetCookie`(D-125), API 401의 `WWW-Authenticate: Bearer`, preflight의 `Authorization`(D-129). `contrib/auth`: 세션 쿠키·`FromSession`·`SafeNext`(시도 제한은 8번). 복사본: `Session` 쿼리, 로그인 폼·뷰, 로그아웃(POST).
→ 확인: 로그인 전후 쿠키 값이 다르다(완료 조건 6), 로그아웃 뒤 이전 쿠키는 로그인하지 않은 것, 만료, `next`의 `//evil`·`https://…`·`/\evil`는 기본 자리로, 없는 이메일과 틀린 비밀번호가 같은 응답

### 계획

세 층. 이 작업의 끝에 `maago add auth`한 프로젝트가 로그인·로그아웃하고, 뷰가 `maago.UserFrom[auth.User]`로 사용자를 안다. 시도 제한은 8번.

코어(`maago`, D-125·D-129)
- `maago.SetCookie(ctx, *http.Cookie)`: `serve`가 요청 ctx에 `*written`을 싣고, `SetCookie`는 거기 모은다. `written`이 첫 최종 응답(`WriteHeader`의 200 이상, `Write`, `Flush`) 직전에 `http.SetCookie`로 붙인다. 1xx에는 붙이지 않는다. 생성된 어댑터·탈출구 뷰·303·JSON 모두 이 래퍼를 지나므로 어댑터 템플릿은 바뀌지 않는다. `report`는 아직 쓰지 않았으면 모은 쿠키를 버린다(500·422·panic에 세션이 붙지 않게). 이미 쓴 뒤에는 거둘 수 없다(스트리밍은 쓰기 전에 맡긴다). panic: `serve` 밖(`DBFrom`처럼), 이미 쓴 뒤, `c.Valid()`가 틀린 쿠키(`http.SetCookie`는 말없이 버린다)
- API 라우트의 401에 `WWW-Authenticate: Bearer`(D-129, `fail`의 API 분기 한 줄. HTML 401은 범위 밖). preflight가 `Authorization`도 허락(`preflightOK`와 `Access-Control-Allow-Headers: Content-Type, Authorization`)

라이브러리(`contrib/auth`, stdlib만)
- `NewToken() (token, hash string)`: 32바이트 `crypto/rand`, 토큰은 `base64.RawURLEncoding`(43자), 해시는 SHA-256의 hex(DB에 넣는 것). `HashToken(token) string`
- `SessionCookie(token, path string, maxAge time.Duration) *http.Cookie`: 이름 `session`, `HttpOnly`·`Secure`·`SameSite=Lax`, `Path`는 부르는 쪽이 넘긴다(라이브러리는 `maago`를 import하지 않는다. 복사본이 `cmp.Or(maago.Prefix(), "/")`). 지우는 쿠키는 `maxAge < 0`(같은 Path)
- `FromSession[U any](find func(ctx context.Context, hash string) (U, error)) func(*http.Request) (any, error)`: 쿠키가 없거나 43자 base64url이 아니면 `find`를 부르지 않고 `nil, nil`. `find`가 `sql.ErrNoRows`면 `nil, nil`(만료·지운 세션은 로그인하지 않은 것), 다른 오류는 그대로, 성공만 `U`를 담는다(제네릭: 오류 때의 영값 `User`가 "사용자 있음"이 되지 않게). Bearer는 11번
- `SafeNext(next, fallback string) string`(D-128): `/`로 시작, 둘째 글자가 `/`·`\`가 아님, 제어 문자 없음, `%2f`·`%5c`(대소문자 무관)가 없음. 아니면 `fallback`

복사본(`internal/scaffold/testdata/auth/`)
- 모델 `Session`: `ID`, `UserID`(`fk=User`), `Hash`(`max=64,unique`), `ExpiresAt time.Time`, `CreatedAt`(`auto=create`)
- 쿼리(두 방언, 시각은 `-- param: now time.Time`): `user_by_email.sql`(`:one User`, `SELECT *`: 사용자가 `User`에 필드를 더해도 깨지지 않게), `session_user.sql`(`:one User`, `SELECT u.* … JOIN auth_session s … WHERE s.hash = :hash AND s.expires_at > :now AND u.is_active AND u.verified_at IS NOT NULL`, D-130의 조건을 세션에도), `delete_user_sessions.sql`(`:exec`, 로그아웃), `delete_expired_sessions.sql`(`:exec`, 로그인 때)
- 라우트: `GET /login "login"`, `POST /login "login_submit"`, `POST /logout "logout"`. `main`은 `auth.App`을 쓴다(D-114). `Config.LoginURL`은 글자 그대로 `"/login"`(`Handler` 전에는 `Prefix()`가 비어 있다, D-122)
- `forms.go`: `LoginForm{Email, Password}`(평범한 폼)
- `views.go`(라이브러리는 `authlib`로 import): `Login`이 두 라우트를 맡는다(GET은 폼, POST는 로그인, 픽스처의 `PostForm`처럼), In은 `Next *string query=next`와 `Form LoginForm`. 이메일은 `strings.ToLower(TrimSpace)`, `UserByEmail`의 `ErrNoRows`는 빈 해시로 `authlib.Check`(같은 비용) → 맞고 **그다음** `IsActive && VerifiedAt != nil`(D-128·D-130) → 틀리면 폼 오류 하나 "Incorrect email or password."(무엇이 틀렸는지 말하지 않는다) → `rehash`면 새 해시를 저장 → 만료 세션 지우기, 새 세션(요청이 가져온 쿠키는 읽지도 쓰지도 않는다: 세션 고정 방지, 완료 조건 6) → `maago.SetCookie` → `SeeOther(SafeNext(next, Prefix()+"/"))`. 인증 전 계정은 6번이 "메일의 링크"를 보이고, 4번은 같은 오류. `Logout`(POST, `LoginRequired` 없음): 사용자가 있으면 그 사용자의 세션을 **모두** 지운다(타입 있는 뷰는 쿠키를 읽지 않는다, 사람의 결정 2026-10-08), 지우는 쿠키, `SeeOther` 로그인 페이지
- `auth.go`: `Current = authlib.FromSession(…)`(`Config.Auth`에 넣는 값, `SessionUser`를 `time.Now()`로), `SessionAge = 14 * 24 * time.Hour`
- 템플릿: `templates/base.html`(앱마다 하나, D-116, 첫 줄에 "사이트의 레이아웃으로 바꾸라"), `login_page.html`(영어, 비밀번호 칸에 `value`를 쓰지 않는다, `form`의 `action`은 `?next=`를 지닌다)
- 시험(`auth_test.go`, 복사본에 들어간다): `maagotest.DB` + `maago.Handler`로. 로그인 성공(303, 쿠키 속성, 다음 요청에서 사용자), 틀린 비밀번호·없는 이메일·인증 전·비활성이 같은 응답(200, 같은 오류 문구, `Set-Cookie` 없음, 본문에 비밀번호 없음), 가져온 `session=attacker`가 로그인 뒤 그대로 쓰이지 않음, 로그인 1의 쿠키 S1을 들고 로그인 2 → S2 ≠ S1(완료 조건 6), 로그아웃 뒤 이전 쿠키는 로그인하지 않은 것, 만료, 인증이 지워진 사용자의 세션, `next`의 `//evil`·`https://evil`·`/\evil`·`/%2F%2Fevil`. 쿠키는 jar 없이 `Set-Cookie` 값을 옮긴다(`Secure`는 속성으로 본다). `authlib.Iterations`를 낮춘다(시험 바이너리마다)
- 드라이버: 복사본의 `driver_test.go`는 방언마다 하나(`driver_test.go.sqlite`·`driver_test.go.postgres`로 담고), `scaffold.Add`가 프로젝트의 방언(`project.Database`)에 맞는 것을 `driver_test.go`로 쓴다. 그대로 복사의 유일한 예외(사람의 결정 2026-10-08, D-123 고침)

시험(`cmd/maago`의 `TestAddAuth`)
- 임시 프로젝트: `testdata/web`의 `go.mod`·`go.sum`(module 줄만 바꿈), `main.go`(`auth.App`), `add auth` → `makemigrations` → `gen`·`check` → **`go test ./...`**(`GOPROXY=off`, 두 방언) → `User`에 `Nickname` → 다시. CLI `migrate`는 돌리지 않는다(`maagotest.DB`가 새 DB에 적용한다). 2번의 `go vet`은 `go test`가 대신
- `scaffold`: 방언 없는 프로젝트(`.env`도 변수도 없음)는 `Add`가 진단으로 거절. 두 방언에서 `driver_test.go`의 import
- 라이브러리 단위 시험: 토큰 길이·서로 다름·해시, 쿠키 속성, `FromSession`(쿠키 없음·빈 값·모양이 틀린 값은 `find`를 부르지 않음, `ErrNoRows`, 다른 오류), `SafeNext` 표
- 코어 단위 시험(`func(w, r) error` 라우트): 쿠키 뒤 쓰기 → 있음, 쿠키 뒤 오류·panic → 없음, 1xx 뒤 → 최종 응답에만, `serve` 밖·쓴 뒤·틀린 쿠키 → panic. API 401의 `WWW-Authenticate`, preflight의 `Authorization`

하지 않기로 한 것: Bearer(11번), 시도 제한(8번), "로그인 상태 유지" 선택, 기기별 로그아웃

단계
1. 코어 → 확인: 루트 단위 시험
2. 라이브러리 → 확인: `contrib/auth` 단위 시험
3. 복사본과 scaffold의 방언 → 확인: `TestAddAuth`가 복사본의 시험을 두 방언으로 돌린다
4. 두 모듈 확인 명령

### 계획 리뷰

- codex: 세션 조회가 `verified_at`을 보지 않음 → 넣음. `SafeNext`의 `%2F`·`%5C` → 거절. 1xx·스트리밍·오류의 쿠키 → 최종 응답에만, 쓴 뒤는 panic. `TestAddAuth`의 go.mod/go.sum → `testdata/web`의 것. `Secure` 쿠키와 httptest → 값을 손으로 옮김. 토큰 인코딩 → `RawURLEncoding` 43자, 모양이 틀리면 조회하지 않음
- fable: ★로그아웃이 쿠키를 읽을 길이 없음 → 사람의 결정: 사용자의 세션을 모두 지움. 같은 경로 라우트 이름 → `login`·`login_submit`. `:one User`는 `SELECT *`(필드 추가에 견딤). `report`의 쿠키 버리기와 `c.Valid()` panic. `FromSession` 제네릭(오류 때 영값 사용자). `HashToken` 하나로, 지우는 쿠키는 `SessionCookie`의 음수 수명. `LoginURL`은 글자 그대로. 비밀번호 `value` 없음과 다시 그리기의 `Set-Cookie` 없음. 문구 "Incorrect email or password.". ★드라이버 import → 사람의 결정: `add`가 방언을 보고 씀. CLI `migrate` 뺌. 완료 조건 6은 S1을 들고 다시 로그인해 S2 ≠ S1로. 머리말의 시도 제한은 8번

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. 계획과 달리 `create_session.sql`은 두지 않았다(생성된 `InsertSession`)
- codex: `ResponseController.Flush`가 `written`을 건너뛴다 → 아님(`Flusher`를 `Unwrap`보다 먼저 본다), 그래도 시험 라우트를 `NewResponseController`로. 저장할 때의 이메일 정규화 → 가입이 생기는 6번에 적음. `rand.Read`의 오류 → Go 1.24부터 오류를 돌려주지 않는다(멈춘다), 그대로. 만료 세션 지우기의 시험 → 로그인 뒤 그 행이 없음을 봄. 로그아웃의 CSRF는 코어의 교차 출처 시험이 본다
- fable: 복사본의 D-번호 셋 → 지움. 시험 구멍 셋(다른 사용자의 세션이 로그아웃에 남음, 두 번째 로그인 뒤 첫 세션이 삶, 세션 경로의 `is_active`) → 더하고 변형으로 잡힘을 확인. DECISIONS 변경 기록과 ARCHITECTURE의 `add` 문장 → 고침. `authMain`의 SQLite import → 주석. 쿠키 이름 → `authlib.CookieName`. `TestRehash`의 `Iterations` → `defer`. `SetCookie`의 nil 쿠키 → 검사 순서. Safari와 `Secure` → 12번에 적음

## 5. 메일 (`contrib/auth`, `maagotest`, D-126)

`auth.Mail`·`Message`·`SMTP`·`ConsoleMail`, `maagotest.Mailbox(t)`.
→ 확인: 가짜 SMTP 서버(`net` 리스너)로 한 통, 메일함 시험

### 계획

이 작업의 끝에 복사본(6·7번)이 `authlib.Mail(ctx, authlib.Message{…})`로 보내고, 시험은 `maagotest.Mailbox(t)`로 받은 것을 본다. 복사본은 아직 보내지 않는다(6번).

`contrib/auth/mail.go`(stdlib만)
- `Message{To, Subject, Body string}`: `To`는 주소 하나(`net/mail`이 읽는 것, ASCII), 본문은 글(text/plain, UTF-8)
- `var Mail func(ctx context.Context, m Message) error`: `main`이 채운다, 시험은 `maagotest.Mailbox`. 처음 값은 오류 "auth: Mail is not set; main sets auth.Mail to auth.SMTP(addr, from, a) or auth.ConsoleMail"(조용히 버리지도 운영 로그에 토큰을 흘리지도 않는다, PHILOSOPHY 7)
- `SMTP(addr, from string, a smtp.Auth) func(context.Context, Message) error`: `SendMail`의 순서(HELLO, 서버가 하면 STARTTLS(`ServerName`은 addr의 호스트), `a`가 있으면 AUTH, MAIL, RCPT, DATA, QUIT)를 `net.Dialer.DialContext`의 연결 위에서. 연결에는 고정 데드라인 30초(`net/smtp`는 시간 제한이 없고 요청 ctx도 대개 끝이 없다), `context.AfterFunc(ctx, conn.Close)`. ctx 때문에 실패하면 `ctx.Err()`를 돌려준다. TLS가 아닌 연결에서 AUTH는 루프백 서버에만(`PlainAuth`가 아닌 `smtp.Auth`도). `from`·`To`는 `mail.ParseAddress`로 읽어 봉투에는 주소만, 머리에는 그 `String()`. 주소가 ASCII가 아니거나 읽히지 않거나 `Subject`에 CR·LF가 있으면 연결 전에 오류. 머리: `From`·`To`·`Subject`(`mime.QEncoding`)·`Date`·`MIME-Version`·`Content-Type: text/plain; charset=utf-8`·`Content-Transfer-Encoding: quoted-printable`(서버가 `8BITMIME`을 하지 않아도 되고 홀로 선 CR도 안전). 점 스터핑과 CRLF는 `c.Data()`의 DotWriter가 한다. `ponytail:` STARTTLS만(465의 암시적 TLS 없음), `Message-ID` 없음
- `ConsoleMail(ctx, m) error`: 표준 출력에 받는 사람·제목·본문(개발용, 시험 없음)

`maagotest.Mailbox(t) *Inbox`(Go에서 함수와 타입이 한 이름을 나눌 수 없어 타입은 `Inbox`): `authlib.Mail`을 받은 것을 모으는 함수로 바꾸고 `t.Cleanup`에서 되돌린다. 받기와 `(*Inbox).Messages() []auth.Message`(복사본)는 mutex 아래(7번이 동시 요청을 보낸다). 패키지 변수라 `t.Parallel` 시험에서는 쓰지 않는다고 문서 한 줄. `maagotest`가 `contrib/auth`를 import한다(같은 모듈, 고리 없음, 런타임 의존 게이트는 그대로)

시험
- `contrib/auth`: 가짜 SMTP 서버(`net.Listen`, 광고할 확장을 시험이 고른다)로 한 통 → 봉투(`MAIL FROM`·`RCPT TO`)와 머리·본문(한글 제목, `.`로 시작하는 줄, 홀로 선 CR, 이름 있는 `from`). AUTH: `250-AUTH PLAIN` + `PlainAuth`(127.0.0.1) → `AUTH PLAIN` 줄, 루프백이 아닌 이름 + 다른 `smtp.Auth` → 거절(가짜 서버는 TLS가 없다), 서버에 AUTH가 없으면 오류. 머리 주입·이름 붙은 `To`가 아닌 깨진 주소·ASCII가 아닌 주소는 연결 전에 거절(서버가 아무것도 받지 않음). 답하지 않는 서버 + 짧은 ctx → `context.DeadlineExceeded`. 처음 `Mail`의 오류
- `maagotest`: `Mailbox`가 동시에 받은 것(`-race`), 시험이 끝나면 원래 `Mail`로 돌아감

하지 않기로 한 것: STARTTLS 시험(인증서), SMTPUTF8, HTML 메일, 첨부, 여러 받는 사람, 재시도·큐, 메일 템플릿(D-126). 6번에 넘김: 사용자 행을 넣은 뒤 메일이 실패하면 무엇이 남는지(순서는 6번 계획), `main`이 `authlib.Mail`을 채우는 모양(12번)

단계
1. `mail.go`와 시험 → 확인: `contrib/auth` 시험(`-race`)
2. `Mailbox` → 확인: `maagotest` 시험
3. ARCHITECTURE, 두 모듈 확인 명령

### 계획 리뷰

- codex: 8bit 본문 → quoted-printable. SMTPUTF8·주소 → `net/mail`, ASCII만, 봉투와 머리를 나눔. 평문 AUTH → 어떤 `smtp.Auth`든 루프백만. 취소 → `AfterFunc` 하나와 `ctx.Err()`. 머리 줄 길이 접기 → 받지 않음(`QEncoding`이 낱말을 나눈다, 제목은 짧다). 홀로 선 CR → quoted-printable이 맡는다. 가짜 서버의 확장 선택 → 받음, STARTTLS는 뺌. `Mailbox` 동시성 → mutex. `ConsoleMail`의 형식 → 개발용 세 줄
- fable: 고정 데드라인(요청 ctx는 끝이 없다) → 30초. ctx 오류로 바꿔 돌려주기 → 받음. `Mailbox` mutex → 받음. DotWriter가 이미 함 → 문장 고침. AUTH 가지 시험 → 받음. 처음 `Mail`은 오류 → 그대로. 메일 실패 뒤의 행·`main`의 모양 → 6·12번. 두 번째 `Mailbox` 가드 넣지 않음. 오류 접두어 `auth:`

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. D-126의 `maagotest.Mailbox(t)`는 그대로, 돌려주는 타입은 `Inbox`(Go에서 함수와 타입이 한 이름을 나눌 수 없다)
- codex: 머리 줄 길이 → 접지 않고 900바이트 넘으면 거절. ctx 오류 바꾸기의 경합 → `stop()`이 false일 때(ctx가 연결을 닫았을 때)만. 거절 시험의 "연결 없음" → 닫힌 포트로(연결했다면 다른 오류)
- fable: 연결 자체에 데드라인이 없음 → `net.Dialer{Deadline}`. 가짜 서버의 줄 읽기 → `textproto.Reader`. 거절 시험 → codex와 같이. 본문이 ASCII뿐 → 한글을 넣어 quoted-printable을 시험. `Mailbox` 시험이 개수만 봄 → 받는 사람 열 개를 비교. `maagotest` 패키지 문서 → `Mailbox`와 예외 문장. ASCII 검사 → `utf8.RuneSelf`. `wait`의 시간 제한은 둔다(실패 문구가 분명하다)
- 고친 뒤 변형 넷(ctx 오류, 머리 길이, quoted-printable, 받은 메일)이 시험에 잡힘을 확인

## 6. 비밀번호 검증기, 가입과 이메일 인증 (D-128)

검증기는 Go 함수 목록(D-15: 길이, 흔한 비밀번호 작은 목록, 이메일과 비슷함). 이메일은 저장할 때도 `ToLower(TrimSpace)`(4번의 로그인이 그렇게 찾는다, codex 4번 구현 리뷰). 가입 폼(모델 폼, 이메일 중복은 뷰의 쿼리 D-119, 같은 응답), 인증 토큰 메일, 링크로 활성.
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

`Secure` 쿠키는 Safari가 `http://localhost`에서 받지 않으니 튜토리얼은 그것을 한 줄 말한다(fable 4번 리뷰). `maago add auth`(그 뒤 `maago gen`이 `auth.App`을 쓰라고 막는 것까지 단계의 글), `main`(`auth.App`·`Auth`·`LoginURL`·메일·`-createsuperuser`), `Post.AuthorID fk=auth.User`(마이그레이션), 로그인 필수 쓰기, 작성자만 고치고 지우기(뷰의 Go 코드, 아니면 403), 지우기는 댓글이 없으면 진짜 지우기·있으면 숨기기(D-84), 가입·로그인 화면. 시험은 낮은 반복 수와 메일함. 댓글의 작성자는 계획에서.
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
