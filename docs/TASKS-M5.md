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
