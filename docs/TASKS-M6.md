# M6 작업 목록 — Admin

[MILESTONES.md](MILESTONES.md)의 M6를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M6 완료 조건** (MILESTONES)
- 게시판 튜토리얼(M8)에서 이 마일스톤이 받는 커밋이 모두 통과한다 → 9번
- 필드 이름이 `password`여도 태그가 없으면 숨겨지지 않는다(추측 금지 확인) → 1번(검사), 2번(생성), 3·4번(그린 화면)
- 로그인하지 않으면 admin에 들어갈 수 없다 → 3번
- 권한이 없는 모델은 목록에 보이지 않고 URL로 들어가도 403이다 → 5번
- 목록 컬럼 지정 같은 커스터마이징 하나가 골든으로 검증된다 → 2번

**M5에서 넘어온 것**
- 모델별 권한·그룹·`IsSuperuser`는 M6(D-130) → D-133, 5번(그룹은 v1)
- TUTORIAL-FUTURE §5의 `admin.App`(🔮), D-69의 "admin CRUD + 로그인까지 5분" → 3번, 10번
- 복사된 `auth`의 `User.PasswordHash`와 Hash를 가진 모델 → D-133, 5번
- 모델 수준 검증 메서드(D-15, D-119가 M6로 미룸) → D-135: 하지 않는다

---

## 먼저 확인한 사실 (2026-10-13, 리뷰가 코드로 다시 봄)

- **admin 코드는 아직 없다.** ARCHITECTURE의 자리만: 런타임 `contrib/admin`, 생성 `internal/admingen`, 앱마다 `admin_gen.go`
- **생성된 CRUD는 PK로**: `GetX`·`InsertX`·`DeleteX`, `ID`만 있는 모델이 아니면 `UpdateX`(D-111, `internal/mapper/crud.go`). 목록·개수·필터·검색·정렬·페이지는 없다. 생성 SQL은 두 방언(D-31)
- **모델 폼은 admin이 그대로 못 쓴다.** 폼 struct는 사람이 선언하고, 바인딩은 어떤 `In`이 그 폼을 읽을 때만 생성된다(`internal/web/gen.go`의 `f.used`). 모델 폼에 `ID`·`auto` 필드는 오류다(`internal/web/forms.go`: "not for a form to set"). admin은 라우트 표가 없으니 `admingen`이 폼과 바인딩을 스스로 쓴다(규칙 글은 `web`의 것을 꺼내 함께 쓴다)
- **생성물의 import 게이트**(D-66): `noreflect_test.go`의 `TestGeneratedImports`는 `mapper.Imports`·`web.Imports`만 허용한다. `admin_gen.go`가 `contrib/admin`을 import하려면 허용 목록에 더한다
- **`maago.NewApp`·`AppRoute`는 "생성 코드만 부른다"**(`http.go`). `contrib/admin`이 첫 라이브러리 호출자가 된다(ARCHITECTURE §2 문구). admin 라우트는 생성기의 이름·패턴 충돌 검사를 지나지 않아, 사용자 라우트가 `/admin/…`이면 대개 더 구체적인 사용자 패턴이 admin 화면을 조용히 가리고, 어느 쪽도 더 구체적이지 않으면 기동 때 `ServeMux`가 panic한다(M6 2번 계획 리뷰가 실측) → 생성 오류로(D-131)
- **로그인 필수는 이미 있다**: `AppRoute.LoginRequired`면 `Handler`가 303/401(D-121)
- **`ordering`은 소비자가 없다**(`internal/model/check.go`만). admin이 첫 소비자. 없을 때의 기본은 D-132
- **`Field.FK`는 표 이름**이다(모델이 아니다). 링크와 역방향 개수에는 표 → (앱, 모델) 역변환이 필요하다
- **검색의 대소문자**: SQLite의 `LIKE`·`lower()`는 ASCII만 대소문자를 무시하고 PG는 `ILIKE`가 따로 있다. `LOWER(칸) LIKE LOWER(:q) ESCAPE '\'`가 두 방언에서 같은 글이다(D-134)
- **태그 어휘는 닫힌 목록**: 지금 모델 4(`table`·`ordering`·`unique`·`index`) + 필드 9(`internal/model/check.go`) + 폼 6. M6이 다섯을 더한다(D-132)
- **코어도 admin도 사용자 타입을 모른다**(D-51): `IsStaff`·권한은 복사된 auth가 구현한 함수 `Can`으로 받는다(D-133)
- **FK는 `ON DELETE NO ACTION`**(D-84). 생성기는 모든 앱의 FK를 안다(D-124)
- **교차 출처 검사는 `Handler`가 모든 라우트에 건다**(D-47을 D-131로 고쳤다)
- **M2M은 없다**(U-04)

## 결정 (2026-10-13, 초안 리뷰 뒤 사람의 결정)

초안의 Q1~Q8을 codex·fable 리뷰 뒤 사람이 골랐다. 자세한 것은 DECISIONS.

- **D-131 생성 + 라이브러리**: `contrib/admin`을 import할 때만 앱마다 `admin_gen.go`, 빠뜨린 `X.Admin`과 `/admin/` 아래의 사용자 라우트는 생성 오류, 들어오는 것은 `Can`이 보기를 허락한 모델이 있을 때
- **D-132 태그 다섯**(사람의 결정: 초안대로 전부): `admin_list`·`admin_search`·`admin_filter`(모델), `admin=hidden`·`admin=readonly`(필드), `admin=hidden`(모델). 숨긴 필드가 영값이 될 모델은 추가 없음. 기본 정렬과 기본 칸. 일괄 지우기. FK는 번호
- **D-133 권한**(사람의 결정: 동작까지): `IsSuperuser`와 `Permission{UserID, App, Model, Action}`, 기본 거부, `Can` 하나, 그룹은 v1, Hash를 가진 모델과 `PasswordHash`는 숨김
- **D-134 목록 SQL**: 검색의 `LOWER … LIKE … ESCAPE`, 50행 페이지, 개수
- **D-135 지우기**: 역방향 FK 개수로 막음, 일괄도 행마다. D-15는 미룸(사람의 결정)

## 모양

```
 사용자 프로젝트                                      이 저장소
 main.go: Apps: {board.App, auth.App,                 contrib/admin  App(Config) → maago.App
   admin.App(admin.Config{                              라우트 /admin/, 화면(embed.FS), CSS 한 파일
     Models: [][]admin.Model{board.Admin, auth.Admin},      Config.Can(ctx, app, model, action) (bool, error)
     Can: auth.Can})}                                 internal/admingen → 앱마다 admin_gen.go
 board/admin_gen.go: var Admin = []admin.Model{…}       모델 기술, 목록·개수 쿼리(두 방언), 폼·바인딩,
 auth/: IsSuperuser, Permission, Can (복사본)             행의 값, 역방향 FK 개수
```

- `admin.Model`의 정확한 모양(인터페이스냐 struct냐, 무엇을 담나)은 2번 계획에서 정한다

## 작업

1. **태그**(D-132): `admin_list`·`admin_search`·`admin_filter`·`admin`(필드 `hidden`·`readonly`, 모델 `hidden`), 검사(없는 필드, `hidden`과 `admin_list`·`admin_search`·`admin_filter`가 겹침, 검색은 문자열만, 필터는 선택지·bool·FK만). → 확인: 진단 표, `TestAdminTags`(태그 없는 `password` 필드는 숨겨지지 않는다, 완료 조건 2의 검사 쪽)
2. **admingen 뼈대**(D-131): `contrib/admin`을 import한 프로젝트만, `admin_gen.go`의 모델 기술(필드·라벨·종류·숨김·읽기 전용·FK의 대상), 목록과 개수 쿼리(정렬·페이지, 검색·필터는 7번) 두 방언과 연습용 DB의 prepare, 빠뜨린 `X.Admin`과 `/admin/` 충돌의 생성 오류, import 게이트에 `contrib/admin`. → 확인: 골든(`admin_list` 지정, 완료 조건 5), 두 방언의 목록 실행, 진단 둘
3. **`contrib/admin` 런타임**: `admin.App(Config)`, 라우트(`/admin/` 인덱스·모델 목록·한 행·추가·고치기·지우기 확인), `LoginRequired`, `Can` 이음새(보이는 모델이 없으면 403), 인덱스·목록 화면, CSS(`embed.FS`), ARCHITECTURE §2의 `NewApp` 문구. → 확인: `httptest`로 로그인 없이 303(완료 조건 3), 보이는 모델 없음 403, 목록의 칸·페이지 경계, 태그 없는 `password` 칸이 그린 목록에 있음(완료 조건 2)
4. **추가·고치기**: 폼 struct와 바인딩 생성(규칙 글은 `web`과 함께), `hidden`은 화면에도 요청에도 없고 `readonly`·`auto`·`ID`는 요청으로 바뀌지 않음(서버에서), 포인터의 NULL, 선택지, FK 번호와 링크, 추가 없는 모델(D-132), 오류 다시 그리기. → 확인: 두 방언, 숨김·읽기 전용 값을 요청에 실어도 바뀌지 않음, 다른 출처의 POST 403
5. **권한**(D-133): 복사본 auth의 `IsSuperuser`·`Permission`·`Can`, Hash 모델과 `PasswordHash`의 `admin=hidden`, `-createsuperuser`가 `IsSuperuser`도, 동작별 403(목록·한 행·추가·고치기·지우기의 GET과 POST). → 확인: 사용자(staff 아님·staff·권한 행·superuser) × 동작 넷 행렬(완료 조건 4), admin의 User 화면에 해시 없음, `TestAddAuth` 두 방언
6. **지우기와 일괄 지우기**(D-135): 역방향 FK 개수, 막힌 지우기의 까닭, 행마다 권한·FK, 지운 것과 못 지운 것. → 확인: 두 방언
7. **검색·필터**(D-132·D-134): `admin_search`의 `LOWER … LIKE … ESCAPE`, `admin_filter`(선택지·bool·FK 번호), 개수와 페이지가 함께. → 확인: 두 방언, `%`·`_` 입력, 빈 검색어, 필터와 검색을 함께
8. **복사본과 문서의 상한**: admin이 지나치는 뷰의 일(D-135: `User.Email` 정규화, 비활성 때 세션 지우기), 그룹·비밀번호 바꾸기·고르는 상자·템플릿 오버라이드는 v1 → 문서
9. **게시판 튜토리얼의 M6 단계**: `admin.App`과 `auth.Can`, 게시판 모델의 태그(목록·검색·필터), 편집자의 권한 행. → 확인: `TestBoardTutorial`
10. **완료 조건과 문서**: "M6 끝"에 조건마다 시험 이름, TUTORIAL-FUTURE §5의 🔮, D-69의 5분 흐름 점검, ARCHITECTURE·MILESTONES

## 1. 태그 (D-132)

### 계획

이 작업의 끝에 모델 검사(`internal/model`)가 admin 태그를 읽고 검사해 `Model`·`Field`에 담는다. 쓰는 곳은 2번부터.

- `Field.Admin string`: `""`·`"hidden"`·`"readonly"`(`Auto`처럼). 필드 태그 `admin=hidden`·`admin=readonly`(맨 값 하나, 다른 값은 `unknown`의 "did you mean"). `ID`는 지금 검사가 그대로 막는다(`label`·`help`만: 늘 읽기 전용이고 링크에 쓴다). `auto=` 필드의 `admin=readonly`는 오류가 아니다(겹칠 뿐, FK 자동 인덱스와 겹치는 `index`처럼)
- `Model`에 평평하게(기존 `Ordering`·`Uniques`처럼): `AdminHidden bool`, `AdminList`·`AdminSearch`·`AdminFilter []string`(Go 필드 이름, 쓴 순서). 모델 태그 `admin=hidden`(그 밖의 값은 오류), `admin_list=(…)`·`admin_search=(…)`·`admin_filter=(…)`(괄호 묶음만, `unique=(…)`처럼). 같은 키 두 번은 오류(`unique`·`index`만 반복을 받는다: 조건을 넓힌다)
- 검사(모두 `파일:줄:칸`, 항목 오류는 그 값의 칸에, 문구는 있는 줄을 따른다): 없는 필드(`item`의 제안), 한 태그 안의 같은 필드 두 번(다른 태그끼리는 된다: 목록과 검색에 같은 필드), 숨긴 필드가 목록·검색·필터에, 검색은 문자열(`*string` 포함)만, 필터는 선택지 있는 문자열·bool·FK(`f.FK != ""`, 포인터 포함)만. NULL을 고르는 필터는 v0에 없다(7번). 숨긴 모델에 남은 `admin_list` 같은 설정은 오류가 아니다(쓰이지 않을 뿐)
- 기본값은 여기서 채우지 않는다: `admin_list`가 없을 때의 칸(숨기지 않은 모든 필드, 선언 순서)은 2번의 생성기가 정한다. FK의 대상 모델이 숨김이면 링크 없이 번호만(2·4번)
- `allowed:` 목록은 끝에 `admin`(모델은 `admin`·`admin_list`·`admin_search`·`admin_filter`): `internal/model`의 시험·`testdata/errors.golden`, ARCHITECTURE, TUTORIAL-FUTURE. DECISIONS D-83의 닫힌 목록에 D-132 한 줄. 작업 목록의 완료 조건 2 줄을 "1번(검사), 2번(생성), 3·4번(그린 화면)"으로

시험(`internal/model`)
- `TestCheckErrors`(진단 표)에 위 검사마다 한 줄: `admin=foo`, `admin_list=Title`(괄호 없음), `admin_list=()`, `admin_list=(Title,Title)`, 없는 필드, 숨긴 필드가 목록에, 검색에 숫자, 필터에 선택지 없는 문자열, 같은 키 두 번(`admin_list` 둘), 모델 `admin=shown`
- `TestAdminTags`(작은 `load` 픽스처): 맞는 모델이 담김, `*int64` FK·`*bool`·`*string`+선택지 필터, **`Password string` 필드가 태그 없으면 `Admin == ""`**(완료 조건 2의 검사 쪽: 이름으로 숨기지 않는다), `auto` + `readonly`는 진단 없음

하지 않기로 한 것: 기본 칸 채우기(2번), 숨긴 필드의 영값과 "추가 없음"(4번), 필터 값 고르기 화면(7번), 군더더기 오류(`auto`의 `readonly`, 숨긴 모델의 설정)

단계
1. 필드 태그 `admin`, 모델 태그 넷 → 확인: 진단 줄, `TestAdminTags`
2. `allowed:` 목록·문서, 두 모듈 확인 명령

### 계획 리뷰

- codex: 포인터 필터(★) → `*string`+선택지·`*bool`·`*int64` FK 받음, NULL 필터는 v0에 없음. 숨긴 FK → 목록·검색·필터에 오면 오류, 그 밖은 2·4번. `auto`와 `readonly`의 순서 → 오류를 두지 않아 사라짐. `ID` → 지금 검사가 막음. `allowed:` 목록의 범위 → 네 곳. 같은 필드 두 번은 한 태그 안만(★ → 다른 태그끼리는 됨). 괄호만은 맞음, 빈 괄호·중복 시험
- fable: 옵션 순서(필드·모델 둘 다) → 군더더기 오류를 빼서 사라짐. 같은 키 두 번의 조건을 넓힘. `allowed:` 네 곳. 필터의 FK는 `f.FK != ""`. 작업 목록의 완료 조건 2 줄. 진단 문구는 있는 줄을 베낌. 모양은 평평하게. `TestAdminTags`는 작은 픽스처로. ★군더더기 오류 → 둘 다 뺌(겹치는 `index`의 선례, 사람에게 묻지 않은 작은 것). ★FK 대상이 숨김 → 2·4번에서 링크 없음

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. 변형으로 확인: 숨김 검사를 빼면 `TestCheckErrors`, 필터의 FK 조건을 빼면 `TestAdminTags`, 같은 키의 옛 조건으로 돌리면 `TestCheckErrors`가 잡는다
- codex: 숨긴 필드가 검색에 숫자로 오면 오류가 하나만(D-61) → 검사 셋을 따로 내고, `Adm2.Code`(숨김·`int32`)가 두 줄을 낸다. 시험의 빈틈 → 같은 줄로 메움
- fable: `Model` 주석의 "빈 목록은 기본값"은 `AdminList`만 → 고침. 계획의 `admin_list=()`는 파서(`tag.go`)가 먼저 막아 여기서는 시험할 수 없음 → 뺌. D-83 덧붙임에 날짜. 2번으로 넘김: 다른 패키지에서 Go 이름으로 `*Field` 찾기(지금은 비공개 `field`), FK의 테이블→모델 대응(숨긴 대상의 링크). 그대로 둠: 잘못된 `fk=` 필드를 필터에 올리면 오류가 한 줄 더(연쇄, 드묾), 모델 `admin=shown`의 fix가 `allowed: hidden`뿐

---

## 2. admingen 뼈대 (D-131)

### 계획

`contrib/admin`의 타입(이번에는 타입과 경로 상수만, `App`·라우트·화면은 3번):

```go
const Prefix = "/admin/" // admin이 받는 경로, admingen이 충돌 검사에 쓴다

type Model struct {
	App, Name string   // 앱의 패키지 이름, Go 모델 이름
	Fields    []Field  // 숨기지 않은 필드, 선언 순서
	List      []string // 목록 칸의 Go 이름: admin_list, 없으면 Fields 전부
	Rows      func(ctx context.Context, db maago.DB, q Query) ([]Row, error)
	Count     func(ctx context.Context, db maago.DB, q Query) (int64, error)
}
type Query struct{ Limit, Offset int } // 7번이 검색어·필터를 더한다
type Field struct {
	Name, Label, Help string // Label은 label=, 없으면 Go 이름(폼과 같은 규칙)
	ReadOnly          bool   // admin=readonly, ID, auto=
	FK                string // 가리키는 모델 "앱.모델", 아니면 ""
}
type Row struct {
	ID    int64
	Cells []any // List 순서의 타입 있는 Go 값. nil 포인터는 typed nil이라 cell == nil이 아니다
}
```

- struct와 함수 칸: 인터페이스는 구현이 생성물 하나뿐이다. 숨긴 필드는 `Fields`에 아예 없어 런타임은 그 이름도 모른다. 태그 없는 `Password`는 있다(완료 조건 2의 생성 쪽)
- 칸 그리기는 런타임(3번, ★사람의 결정: 런타임이 그림): 생성물은 타입 있는 값을 넘기고 `contrib/admin`이 type switch로 그린다. 표시의 개선이 maago를 올리면 따라온다. D-131의 "행 표시"를 "목록 행의 값"으로 고친다
- `Query`는 검색·필터가 올 자리(codex): 7번에서 시그니처가 뒤집히지 않는다. `Field`의 선택지·NULL·기본값은 쓰는 작업(4·7번)에서 더한다
- 생성물은 앱마다 `admin_gen.go`의 `var Admin = []admin.Model{…}`, 함수는 리터럴로. 머리에 한 줄: "칸·검색·숨김은 모델의 admin 태그에서 온다(D-132): 태그를 고치고 maago gen". 모델 `admin=hidden`은 빠지고, 모두 숨으면 빈 슬라이스(파일과 `X.Admin` 요구는 그대로, 규칙 하나). import는 파일마다 센다(빈 슬라이스면 `contrib/admin`만)
- `Rows`는 `models_gen.go`의 `scanX`를 다시 쓴다: `SELECT 모든 칸 FROM t ORDER BY … LIMIT ? OFFSET ?`를 `maago.Many`로 읽고 `Cells`만 고른다. 새 스캔 코드가 없고 SQLite의 시간(D-105)도 그대로. 숨긴 칸은 서버 메모리까지 읽히지만 `Row`·`Fields`에 담기지 않는다(받음, 시험으로 고정)
- 정렬(D-132): `ordering`, 없으면 `"id" DESC`, `ordering`에 `ID`가 없으면 끝에 `"id" DESC`로 끊는다. 포인터 칸에는 `NULLS LAST`(PG는 NULL이 가장 크고 SQLite는 가장 작다, 두 리뷰어 찬성, modernc는 3.30+). 대가는 PG에서 그 칸의 DESC 인덱스를 못 탈 수 있는 것
- 개수: `SELECT COUNT(*) FROM t`, `db.QueryRowContext(…).Scan(&n)` 한 줄(`InsertX`의 꼴). 페이지 셈(50행, 음수·0은 1, D-134)은 런타임(3번)
- SQL은 `internal/admingen`이 `mapper.CRUD`처럼 `Statement`로 만든다(두 방언의 자리표). 연습용 DB의 prepare는 `checkQueries`에서 CRUD 옆에 같은 함수로 만들어 돌린다(파일 단계가 아니라 DB 단계). 뜻은 gentest가 두 방언에서 돌려 고정

언제 생성하나(D-131)
- 프로젝트의 생성물·시험이 아닌 Go 파일 하나라도 `github.com/maago-web/maago/contrib/admin`을 import하면 모델이 있는 앱마다 `admin_gen.go`. 아니면 하나도 없고, 있던 것은 `pipeline.Write`가 지운다
- 파일 읽기: `web.Read`의 파싱 루프를 `web.Parse(fset, root, files) map[dir][]*ast.File`로 꺼내 `web.Read`와 `admingen.Use`가 함께 쓴다(파싱 한 번)
- 빠뜨린 `X.Admin`: 생성물·시험이 아닌 파일 어디에서도 `별칭.Admin`으로 쓰지 않은 모델 앱 → 오류(앱의 첫 모델 자리), fix "add blog.Admin to admin.Config.Models in main (D-131), then run maago gen: go build works after gen". `markUsed`(D-114)를 선택자 이름과 import 경로 목록을 받게 넓혀 둘이 함께 쓴다. dot-import 문구에도 선택자 이름
- `/admin/` 충돌: 그때 사용자 라우트의 경로가 `admin.Prefix` 아래이거나 `/admin`이면 그 경로 자리에 오류, fix "move the route out of /admin/, which the admin serves (D-131)"
- 이름: admin을 쓸 때만 `mapper.Names`의 `More`로 `Admin`과 import `contrib/admin`을 넣는다(쓰지 않는 프로젝트의 `Admin`은 거짓 오류가 아니다)
- import 게이트: `admingen.Imports`(`context`·maago·`contrib/admin`)를 `generatedImports`가 받는다
- 단계: 사용 여부·두 오류·이름은 `early`(DB 없이), prepare는 DB 단계, 파일은 `models_gen.go`와 같은 단계

시험
- 골든: `TestGenerated`가 픽스처 셋 × 두 방언의 `admin_gen.go`도 본다. 예제 `blog`에 태그(세 벌의 `models.go`를 함께): `Post`에 `admin_list=(Title,Status,PublishedAt)`(완료 조건 5)과 `Slug admin=readonly`, `Author.Code admin=hidden`(그래서 Author는 4번에서 추가 없음, 4번의 픽스처로 쓴다). `plain`에 모델 `admin=hidden` 하나(모두 숨은 앱의 빈 슬라이스와 import). DDL은 바뀌지 않는다
- 두 방언의 실행: gentest `blog`의 `TestAdmin`: 글 넷(`PublishedAt` NULL 둘, 같은 값 둘), `Rows`의 순서·칸(NULL이 끝, 같은 값은 ID 내림차순), 페이지 둘, 범위 밖은 빈 슬라이스(nil 아님), `Count`, 빈 표의 `Count` 0, `Author`의 `Fields`·`Cells`에 `Code`가 없음. NULL의 오름차순은 `ordering=(PublishedAt)`인 작은 모델로(어느 픽스처에 둘지는 구현에서)
- 진단 둘과 생성 여부: `cmd/maago`의 `TestAdminGen`(작은 프로젝트에 `build.Check`/`Gen`): import 없음 → 파일 없음, `_test.go`에만 import → 파일 없음, import 있음 → 앱마다, import를 지운 뒤 `check`가 "no longer generated"·`gen`이 지움, `X.Admin` 빠뜨림 오류, `/admin/x` 라우트 오류
- `mapper.Names`: admin을 쓰는 앱에 사용자 `Admin` → 충돌, 쓰지 않으면 없음
- import 게이트: `TestGeneratedImportsGate`에 `contrib/admin` 줄

하지 않기로 한 것: 런타임과 칸 그리기(3번), 폼·바인딩(4번), 검색·필터의 WHERE(7번), 역방향 FK 개수(6번), FK 대상이 숨김이거나 `Can`이 막을 때의 링크(런타임이 `Config.Models`로 정한다, 3·4번), 모델 이름의 사람 말(그냥 Go 이름)

단계
1. `contrib/admin` 타입, `internal/admingen`의 SQL과 파일 → 확인: 골든, gentest `TestAdmin` 두 방언
2. `web.Parse`·`markUsed` 넓힘, 사용 여부·두 오류·`Names`·게이트·prepare, build에 잇기 → 확인: `TestAdminGen`, 두 모듈 확인 명령

### 계획 리뷰

- codex: `Rows`·`Count`에 검색·필터 자리가 없음 → `Query`. `Field`에 선택지·NULL·기본값 → 쓰는 작업에서 더함. prepare가 DB 단계에 없음 → `checkQueries`에서. 숨긴 칸을 메모리에 읽음 → 받음, 시험으로 고정. NULL 정렬 두 방향·같은 값·범위 밖·옛 파일 지우기 시험 → 더함. 7번의 `\` 이스케이프·조합 시험 → 7번 계획에서. ★`NULLS LAST` → 붙임(SQLite 3.30+)
- fable: "`ServeMux`가 panic" 사실이 틀림(대개 조용히 가림, 실측) → 사실 고침, `admin.Prefix`. 모두 숨은 앱의 import → 파일마다 셈, `plain` 픽스처. `Admin` 이름은 admin을 쓸 때만, import도 → `More`로. 개수는 `QueryRowContext` 한 줄. ★칸을 누가 그리나 → 사람의 결정: 런타임, typed nil을 `Row` 주석에, D-131 문구 고침. 라벨은 생성기가 폼 규칙으로. `Field.Type`은 소비자가 없음 → 뺌. 빠진 시험(숨긴 모델, 범위 밖, 빈 표, `_test.go`만의 import, import를 지운 뒤). 세 벌의 `models.go`. 생성물 머리의 태그 안내 한 줄. dot-import 문구

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. 계획과 다른 것: `markUsed`를 넓히는 대신 `web.Uses`(선택자로 쓰인 import 경로)를 꺼내 둘이 함께 쓴다. `orders`의 정렬은 `Priority`가 아니라 `CustomerID`(`TestQueriesDroppedField`가 `Priority`를 지운다). 변형으로 확인: `NULLS LAST`(blog DESC는 PG, orders ASC는 SQLite), ID 끊기, 숨긴 필드, `Admin` 빠뜨림 검사, `/admin/x`·`/admin` 라우트, `Admin` 이름을 늘 넣기, import 판정(어떤 import든)
- codex: 결함 없음. 시험 빈틈 → `ordering=(ID)`는 다시 끊지 않음과 다른 앱의 FK `shop.Item`을 `admingen` 단위 시험으로(변형 둘 잡힘). 별칭·dot import → 판정은 경로만 보므로 같다(받음). `makemigrations`는 admin을 보지 않는다(파싱이 없다, 계획대로). 시험을 돌리지 못함(샌드박스)
- fable: 둘째 정렬 키 `Title`을 시험이 못 잡음 → NULL 글의 제목 순서를 바꿔 갈림(첫 키만 내는 변형 잡힘). ARCHITECTURE의 공개 경로 예외 셋, import 허용 목록, `admin_gen.go`의 설명, 앱 찾기의 `Admin`, 트리의 D-131 → 고침. import를 원문 대신 `Unquote`로. `Query{}`는 빈 페이지라는 주석, 패키지 주석의 "화면과 라우트"는 3번에. 받음: 모델만 있는 앱을 dot-import하면 dot-import 진단 없이 "no package uses X.Admin"만, 라우트 없는 모델 앱의 `Admin` 충돌 가지(`more == nil`)는 시험 밖. 3번에: 칸 머리의 `Label`은 이름→`Field` 도우미, `Cells`의 type switch는 포인터 타입 전부

---

## 3. `contrib/admin` 런타임: 들어오기·인덱스·목록 (D-131, D-133)

### 계획

모양
```go
type Config struct {
	Models [][]Model // 앱마다의 Admin: [][]admin.Model{board.Admin, auth.Admin}
	Can    func(ctx context.Context, app, model, action string) bool // 복사본의 auth.Can (D-133)
}
const View, Add, Change, Delete = "view", "add", "change", "delete" // 타입 없는 상수: Permission.Action의 글자
func App(cfg Config) maago.App // main의 Config.Apps에: admin.App(admin.Config{…})
```
- `action`은 `string`(fable): 정의된 타입이면 복사본 `auth.Can`이 `contrib/admin`을 import해야 하고, 그러면 `maago add auth`만 한 프로젝트가 admin 프로젝트가 된다(`admingen.Use`의 판정)
- `Models`가 `[][]Model`인 까닭: 앱마다의 `X.Admin`을 그대로 적는다(`slices.Concat` 없이, 틀리게 적으면 컴파일 오류), `TestAdminGen`의 fix 문구("add blog.Admin to admin.Config.Models")와 맞는다
- `App`은 잘못된 설정에 panic(기동 때 한 번, `maago.Pages`의 선례): `Can`이 nil, 같은 앱·모델이 두 번, 보일 모델이 하나도 없음(모두 숨긴 앱만이면 인덱스가 늘 403이다). 조용히 모두 거부하지 않는다(PHILOSOPHY 7)
- 라우트 둘, 패턴은 `admin.Prefix`에서 만든다(admingen의 충돌 검사와 한 출처), 모두 `LoginRequired`이고 `API`가 아니다(완료 조건 3: 로그인하지 않은 GET은 `Handler`가 `LoginURL`로 303). `/admin`·`/admin/blog/Post`는 `ServeMux`가 `/`를 붙여 307로 보낸다(실측)
  - `GET /admin/{$}` 인덱스: `Can(…, View)`가 참인 모델만 앱별로, 목록으로 가는 링크. 하나도 없으면 403(D-131). 모델마다 `Can`을 부른다(5번에서 권한 쿼리가 모델 수만큼, v0은 받음)
  - `GET /admin/{app}/{model}/{$}` 목록: URL의 글자는 `Model.App`·`Model.Name` 그대로(★사람의 결정: Go 이름 그대로, 권한 행의 글자와 하나), 다른 글자는 없는 모델. **`Can`을 먼저**: `Can(ctx, app, model, View)`가 거짓이면 그 모델이 있든 없든 403이라 볼 수 없는 사람에게 모델이 있음을 드러내지 않는다(codex). 참인데 없는 모델이면 404
  - `?page=N`: 정수가 아니면 400(`maago.Query`와 같다), 음수·0은 1. `Count`를 먼저 읽어 쪽수를 세고, N이 쪽수보다 크면 `Rows`를 부르지 않고 빈 표(D-134, 아주 큰 N의 OFFSET 넘침이 500이 되지 않는다, fable). 50행, "N쪽 중 M쪽"과 개수, 앞뒤 링크. 7번은 같은 `Query`를 `Rows`와 `Count`에 함께 넘긴다
  - 한 행·추가·고치기·지우기 라우트는 그 화면을 만드는 작업에서(4·6번): 빈 라우트를 미리 두지 않는다. FK 칸은 번호만(링크는 한 행 화면이 생기는 4번)
- 화면: `contrib/admin/templates`(`base.html`·`index.html`·`list.html`)를 `embed.FS`로, `maago.Pages`·`maago.Render`로 그린다. CSS 한 파일(`admin.css`)을 `base.html`의 `<style>`에 넣는다(라우트를 늘리지 않음, JS 없음). 링크는 `maago.Prefix()`를 앞에(전역 `maago.Config.Prefix`, D-122: admin에 따로 두지 않는다)
- 칸 그리기(2번의 사람의 결정): 템플릿 함수 `cell`이 `string`을 돌려준다(`template.HTML`이 아니다: 사용자 데이터는 늘 이스케이프). 닫힌 type switch: 모델 타입 일곱(`int64`·`int32`·`float64`·`string`·`bool`·`time.Time`·`[]byte`)과 그 포인터. bool은 `yes`/`no`, 시간은 `maago.DateFunc("2006-01-02 15:04", …)`(Config.TimeZone), `[]byte`는 `(N bytes)`, nil 포인터는 빈칸. 그 밖의 타입은 오류(maago의 버그: 500, `fmt.Sprint`로 얼버무리지 않는다, PHILOSOPHY 6). 머리는 `List`의 이름으로 찾은 `Field.Label`(작은 도우미)
- 이 저장소 쪽: "생성 코드만 부른다" 주석(`NewApp`·`AppRoute`·`Pages`·`Render`·`ParseQuery`·`Query` 중 admin이 부르는 것)과 ARCHITECTURE §2 목록에 `contrib/admin`을 더한다

시험(`contrib/admin`, DB 없이: `Rows`·`Count`를 손으로 만든 `Model`, 연결하지 않는 `*sql.DB`, `Auth`는 머리로 사용자를 고르는 가짜. `maago.Prefix()`가 프로세스 전역이라 이 패키지에 `t.Parallel`을 쓰지 않는다)
- `TestLogin`: 사용자 없이 인덱스·목록 GET → 303, `Location`을 파싱해 `next`가 요청 경로(완료 조건 3). 같은 `Handler`의 API 앱 라우트는 사용자 없이 401(admin과 섞여도 각자의 꼴)
- `TestCan`: 보이는 모델이 없으면 인덱스 403, 볼 수 있는 모델만 인덱스에, 볼 수 없는 모델의 목록 403, 볼 수 없는 없는 모델도 403, 볼 수 있는(모든 것을 허락하는) 사용자의 없는 모델·`/admin/blog/post/`(소문자)·더 깊은 경로 404. `Can`이 받은 `app`·`model`·`"view"`
- `TestList`: 칸 머리는 `Label`, 칸 값(문자열, `<b>`가 든 문자열은 `&lt;b&gt;`, nil과 값 있는 포인터, bool, 시간, `[]byte`), 개수, 페이지 경계(50행이면 1쪽, 51행이면 2쪽, `page=0`·`-1`은 1쪽, 범위 밖과 `page=1000000000000000000`은 빈 표이고 `Rows`를 부르지 않음, `page=x`는 400), `Rows`가 받은 `Limit`·`Offset`. 모르는 타입의 칸은 500
- `TestPassword`: 태그 없는 `Password` 필드가 그린 목록의 머리와 값에 있다(완료 조건 2의 그린 쪽). 생성 쪽은 `admingen` 단위 시험에 `Password` 필드 하나(태그 없으면 `Fields`·`List`에)
- `TestConfig`: `Can` nil·같은 모델 두 번·모델 없음은 panic과 문구
- `TestPrefix`: `maago.Config.Prefix`가 `/app`이면 링크가 그 아래이고 한 번만 붙는다

하지 않기로 한 것: 한 행·추가·고치기·지우기(4·6번), 검색·필터(7번), 권한 행과 `auth.Can`(5번, 여기 `Can`은 시험의 가짜), 모델 이름의 사람 말, 정렬 바꾸기(D-134: 정렬은 생성된 것만), 고르는 상자, 사용자 앱 이름이 `admin`일 때의 겹침(`Handler`가 "app admin is in Config.Apps twice"로 막는다, 생성 검사는 필요해지면)

단계
1. `Config`·`App`·라우트 둘·화면·CSS·`cell` → 확인: 위 시험들
2. 주석·ARCHITECTURE, `admingen`의 `Password` 줄 → 확인: 두 모듈 확인 명령

### 계획 리뷰

- codex: 403/404 순서가 모델이 있음을 드러냄 → `Can`을 먼저. `TestPrefix`의 주체 → 전역 `maago.Config.Prefix`. admin과 API 라우트가 섞일 때 → `TestLogin`에 API 401. `next`는 파싱해서 본다. `{$}`의 경계 → 소문자·더 깊은 경로 404(`/admin`은 307). 7번의 `Count`도 같은 `Query` → 적음
- fable: ★`Action` 타입이면 복사본 auth가 `contrib/admin`을 import해 admin 프로젝트가 됨 → 타입 없는 상수. ★URL의 글자 → 사람의 결정: Go 이름 그대로. 아주 큰 `page`의 OFFSET 넘침 → `Count` 먼저, 넘으면 `Rows` 없음. `cell`은 `string`, 이스케이프 시험. 값 있는 포인터 시험과 닫힌 type switch(모르는 타입은 500). `Prefix()` 전역이라 `t.Parallel` 없음. "생성 코드만" 주석의 범위. 패턴은 `admin.Prefix`에서. 모델이 없으면 panic. 인덱스의 `Can` 호출 수는 5번 메모. 사용자 앱 `admin`은 `Handler`가 막음

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. 변형으로 확인(19개, 모두 잡힘): `LoginRequired` 둘, 인덱스의 `Can`·403, `Can` 먼저, 넘은 쪽에 `Rows` 없음, 쪽의 하한, 모르는 타입, 포인터 하나 빠뜨림, 모델 없음 panic, `Prefix`, Prev(처음엔 3쪽이 우연히 같아 살아남음 → 5쪽으로), 머리 `Label`, 앱 묶기, 정확히 50행, 마지막 쪽의 Next, 큰 수, `List`⊄`Fields` panic
- codex: 아주 큰 `Count`의 쪽수 셈이 넘침 → `n/50 + min(n%50, 1)`. `*[]byte` 칸이 500 → 더함(시험 줄). `Can` 먼저·이스케이프·`template.CSS`는 embed 파일 하나뿐·초기화의 `Pages`는 문제없음. 시험을 돌리지 못함(샌드박스)
- fable: `List`에 없는 필드가 요청 때 panic → `App`에서 검사하고 머리를 미리 만듦(요청마다의 찾기도 사라짐). 실수가 `1e+06` → `'f'`. 시험 빈틈 셋(앱 묶기, 정확히 50행, 마지막 쪽의 Next) → 더함. 주석의 D-18 → D-77, `home`의 `maago.Config.Prefix`, ARCHITECTURE 목록에 `ParseQuery`·`Query`. 화면: "Rows: N", 빈 모델은 "No rows.", 넓은 표는 가로로 굴림. 4번에: `Can` 먼저→404 두 단계를 `s.model(r, action)`으로 뽑기, 칸에 `Field`를 묶어 넘겨 FK·한 행 링크
- 사람에게 물음(구현 중): admin이 사용자 코드와 맞물리는 방식 → 사람의 결정: 지금대로(생성 파일은 reflect 금지의 다리, 사용자는 import·`main` 한 줄·태그). 맞물림 셋(`Admin` 이름, `X.Admin` 빠뜨림 오류, `/admin/`)은 검사로 둔다

---

## 4. 추가·고치기 (D-131, D-132)

### 계획

나누기(★사람의 결정: 바인딩은 생성물, D-131의 원래 뜻): 모델 타입에 닿는 일(행 읽기, 글→값 바인딩, 넣기, 고치기, FK 대상 확인)은 **생성물**이 `web`의 모델 폼처럼 타입 있는 Go로 쓰고(`maago.Field[T]`와 `maago.Bind*`, 규칙 글은 `web`의 `ruleLit`·`modelRules`와 하나, D-119), 그리는 일(입력 꼴, 값→글, 링크, 쪽)은 **런타임**. 칸 순서·타입이 어긋나면 컴파일 오류다(`[]any`로 주고받지 않는다)

생성물에 더하는 것(`admin.Model`)
```go
Form func(ctx context.Context, db maago.DB, id int64, vals url.Values) (Form, error)
Add  bool // 추가 화면이 있다
Edit bool // 고칠 칸이 하나라도 있다
```
`id`가 0이면 추가, `vals`가 nil이면 GET(그리기만). 돌려주는 `admin.Form{Inputs []Input, Saved bool}`, `Input{Name, Label, Help, Kind, Raw, Error, Choices, FK, Required}`(`Kind`: `text`·`number`·`checkbox`·`select`·`datetime-local`·`readonly`). 고치기에서 행이 없으면 `sql.ErrNoRows`(→ `Handler`가 404, GET과 POST 사이에 지워진 행도)
- 고칠 수 있는 칸: 숨김·`admin=readonly`·`ID`·`auto`·`[]byte`(v0은 고치지 않는다)가 아닌 것. 나머지 보이는 칸은 `readonly` 입력(값은 3번의 `cell`)
- 바인딩: 포인터는 선택(빈 입력이 nil), `*bool`은 세 값 `select`(빈·yes·no), 선택지 있는 `*string`의 `select`엔 빈 항목, bool은 체크 상자, 그 밖은 필수(D-119). `Max`·`Choices`는 모델에서
- FK: 생성물이 FK마다 `SELECT 1 FROM 대상표 WHERE "id" = ?`(D-124, `checkQueries`로 prepare)로 있는지 보고 없으면 그 칸의 오류. 문구는 `maago.Messages`에 한 줄(`NoRow: "Select a row that exists."`, D-119의 바꿀 수 있는 문구). 확인과 저장 사이에 대상이 지워지면 DB가 막아 500(D-135와 같은 받음). 화면은 번호 입력과 대상의 한 행 링크(대상이 `Config.Models`에 있고 `Can(View)`일 때만)
- 넣기: `InsertX(NewXInput{…})`. 고칠 수 있는 칸은 보낸 값(기본값이 있는 칸은 `maago.Set`), 그 밖은 비워 둬 기본값·nil. 추가 화면의 기본값 칸은 모델의 `default` 글로 미리 채운다(fable 6)
- 추가 없음(D-132, ★사람의 결정: 읽기 전용도 막음): 숨김·읽기 전용·`[]byte` 칸 가운데 `ID`·`auto`·포인터·`default`가 아닌 것이 있으면 `Add`가 거짓. 그 까닭을 `admin_gen.go` 머리에 D-18처럼 적는다("Post: no adding: Slug is admin=readonly, with no default"). D-132 문장을 고친다
- 고치기: `UpdateX`를 쓰지 않는다(행 전체를 써서 그사이 바뀐 숨긴 칸, 복사본 `User.PasswordHash`를 되돌린다). admingen이 **고칠 수 있는 칸과 `auto=update`만** 쓰는 `UPDATE t SET … WHERE "id" = ?`를 만든다(두 방언, SQLite 시간 인자 D-105, prepare). 숨긴·읽기 전용 칸은 문장에 없어 요청에 실어도 바뀌지 않는다. 고칠 칸이 없으면 `Edit`이 거짓이고 화면은 읽기만
- 동시 고침(★사람의 결정: 나중 저장이 이김, Django와 같다): 고칠 수 있는 칸만 쓰므로 숨긴 칸은 되돌리지 않는다. 충돌 검출은 v1(문서에)
- 시간(★사람의 결정: 마이크로초까지, 구현 리뷰 뒤 글 입력으로): `type="text"`에 `2006-01-02T15:04:05.999999`(`Config.TimeZone`)로 그리고 `maago.BindTime`이 소수 초도 읽는다(`time.Parse`가 받는다). 손대지 않은 칸이 저장 때 깎이지 않는다. 실수는 `'f', -1`이 정확히 왕복한다. "그린 글과 같으면 원래 값" 분기는 두지 않는다
- 값→글(GET의 `Raw`)은 런타임 함수 `admin.Raw`(닫힌 type switch, 3번의 `cell`과 짝), `Input`을 만드는 것은 제네릭 `admin.Input…` 도우미: 생성물은 짧은 호출만

런타임(`contrib/admin`)
- 라우트 넷 더함(모두 `LoginRequired`). `Can` 먼저→없는 모델 404를 `s.model(r, action)` 하나로(3번 리뷰):
  - `GET·POST /admin/{app}/{model}/add/{$}`: `Can(Add)`, `Add`가 거짓이면 404. 리터럴 `add`가 `{id}`보다 앞선다(`ServeMux`의 더 구체적인 쪽, 시험 한 줄)
  - `GET·POST /admin/{app}/{model}/{id}/{$}`: GET은 `Can(View)`, POST는 `Can(Change)`. `id`가 정수가 아니면 404. `Change`가 없거나 `Edit`이 거짓이면 읽기만(입력 `disabled`, 저장 단추 없음)
- 성공은 303으로 목록, 실패는 같은 화면을 오류와 함께(200, 보낸 글 그대로). 폼 전체 오류와 DB 제약(unique 등)의 문구는 v1: 드라이버 오류는 500(받음, 문서에)
- 목록: 첫 칸이 한 행 링크, `Add`이고 `Can(Add)`면 "Add" 링크

시험
- gentest(두 방언) `TestAdminForm`: 실제 생성물에 `admin.App`+`httptest`로(fable 1의 이음새): `orders`의 추가 POST(기본값 칸·포인터 FK의 빈 입력 nil·없는 FK 번호의 오류·`auto` 시간), 고치기 GET의 값(마이크로초 시간)과 손대지 않은 POST 뒤 같은 값, `blog`의 고치기가 고칠 수 있는 칸만 쓰고 숨긴 `Author.Code`·읽기 전용 `Post.Slug`·`CreatedAt`을 POST에 실어도 그대로(DB 행으로 확인), `UpdatedAt`은 바뀜, 지워진 행의 POST 404, `Post`·`Author`는 추가 404
- 골든: 세 픽스처의 `admin_gen.go`(폼, `Update`·FK SQL, 추가 없음의 머리 주석)
- `admingen` 단위: 추가 없음의 판정(숨김·읽기 전용·`[]byte`, 포인터·`default`·`auto`·`ID`는 막지 않음), 고칠 칸 없는 모델의 `Edit` 거짓
- 런타임(손으로 만든 `Model`): 입력 꼴 다섯과 `readonly`, `Can`이 받은 `"add"`·`"change"`, `Change` 없는 사용자의 POST 403과 GET 읽기만, `Add` 없음 403, 추가 없는 모델의 add 404, `add`와 `{id}`, 다른 출처의 POST 403(`Handler`가 사용자보다 먼저), 성공 303
- `maago.BindTime`의 소수 초, `Messages.NoRow`(기본값과 바꾼 값, `check`)
- 완료 조건 2의 폼 쪽: 태그 없는 `Password` 칸이 고치기 화면에 입력으로 있다(`admingen` 단위와 런타임)

하지 않기로 한 것: 지우기(6번), 권한 행렬 전체(5번), FK 고르는 상자(v1), `[]byte` 고치기, 폼 전체 오류와 DB 제약 오류의 문구(v1), 충돌 검출(v1), 모델 수준 검증(D-135), 고친 이력

단계
1. 코어: `BindTime`의 소수 초, `Messages.NoRow` → 확인: 코어 시험
2. admingen·`contrib/admin` 타입: 폼·넣기·고치기·FK 확인과 그 SQL, 추가 없음, prepare → 확인: 골든, `admingen` 단위
3. 런타임: 라우트 넷, 폼 그리기, 목록 링크 → 확인: 런타임 시험, gentest `TestAdminForm` 두 방언, 두 모듈 확인 명령

### 계획 리뷰

- codex: ★동시 고침 → 사람의 결정: 나중 저장이 이김(고칠 칸만 쓰므로 숨긴 칸은 안전). ★시간 비교 → 사람의 결정: 마이크로초까지 그려 비교 분기 없음. ★읽기 전용과 추가 → 사람의 결정: 막음. FK 확인과 저장의 경쟁 → 500 받음(D-135처럼). `[]byte`·`Opt[*T]`·시간대의 넣기 시험 → `TestAdminForm`. 권한 순서(`Can` 먼저) → `s.model`. `add`와 `{id}` → 시험 한 줄. 다른 출처 POST가 사용자보다 먼저 403
- fable: ★`[]any`의 두 순서가 어긋나면 열이 조용히 바뀜, 계획이 D-131("폼과 바인딩은 생성")과 어긋남 → 사람의 결정: 바인딩은 생성물. 실제 생성물에 HTTP로 넣는 통합 시험 → gentest `TestAdminForm`. 추가 없는 까닭을 생성물 주석에, `ID`는 규칙 밖, `[]byte{}`의 예외 없앰. `*bool`은 세 값 select. 기본값 칸을 미리 채움. FK 확인은 생성된 `SELECT 1`(숨긴 대상·존재 누설 없음), 문구는 `Messages`에. 고칠 칸 없는 모델은 읽기만. 선택지 있는 포인터의 빈 항목. 지워진 행의 POST 404. 생성물에 자리 주석이 필요 없게 이름으로

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. 계획과 다른 것: gentest의 `TestAdminForm`은 HTTP가 아니라 생성된 `Form`을 직접 부른다(`gentest.Open`이 연결 하나라 `Handler`에 못 넣는다. 바인딩이 생성물로 가 칸 순서는 컴파일러가 잡고, 그리기는 런타임 시험이 본다). `BindTime`의 레이아웃은 바꾸지 않았다(`time.Parse`가 초 뒤의 소수를 받는다, 바꾼 변형이 살아남아 알았다, 시험으로 고정). 변형으로 확인(15개 중 14개, 나머지 하나가 위의 것): UPDATE 칸, 고칠 칸의 정의, `NoRow`, 추가 막음, `Opt`, `Raw`의 정밀도, `Ptr`, 권한 넷, `Add`·`Edit` 검사, FK 링크의 `Can`, 목록의 Add 링크. 리뷰 뒤 템플릿 변형 셋(`checked`, 필수 select의 빈 항목, `selected`)도 잡힘
- codex: 같은 키 두 번의 POST가 첫 값으로 저장 → 400(admin의 입력은 키가 하나씩). 추가 없는 모델의 생성된 `Form`이 `id == 0`에 panic → `admin.NoAdding`(404). 따옴표·HTML이 든 값의 이스케이프 시험 → 더함. UPDATE 자리표·인자 순서, SQLite 시간, 숨김·읽기 전용 빠짐, FK 확인, 권한 순서, 303의 접두어는 맞음. 시험을 돌리지 못함(샌드박스)
- fable: ★`datetime-local`은 소수 초를 세 자리까지만 받아 마이크로초 값은 빈칸으로 그려진다(HTML 표준, 브라우저 없는 시험이 못 봄) → 사람의 결정: 글 입력(`type="text"`, `2006-01-02T15:04:05.999999`, Django처럼 고르는 창 없음). 고칠 칸이 없는데 추가가 있는 모델의 POST가 아무것도 안 함 → `InsertX(NewXInput{})`(단위 시험 단언). 입력 꼴(select·checkbox·글 시간·readonly)의 HTML을 그리는 시험이 없음 → 더함. `*bool` 경로의 생성물 시험 → `TestNullBool`. ARCHITECTURE의 `admin_gen.go` 설명과 `NoRow` → 고침. 추가 없음의 까닭을 모두 적음. 정수 칸은 `step="1"`(`integer`). 값 없는 필수 select엔 빈 항목. 그대로 둠: 연속한 `if id != 0` 둘, 첫 칸이 빈 글이면 링크가 안 보임(v0)

---

## 5. 권한 (D-133)

### 계획

복사본 auth(`internal/scaffold/testdata/auth`)
- `User`에 `IsSuperuser bool default=false`, `PasswordHash`에 `admin=hidden`(그래서 admin에서 `User`는 추가가 없다, D-133. 고치기는 4번대로 해시 칸을 쓰지 않는다)
- 새 모델 `Permission{ID, UserID fk=User, App max=100, ModelName max=100, Action choices=view/add/change/delete}`, `unique=(UserID,App,ModelName,Action)`(★사람의 결정: `ModelName`. `Model`은 임베드한 `maago.Model`과 겹쳐 컴파일되지 않는다). `App`·`ModelName`은 admin URL의 글자 그대로(대소문자까지, 3번의 사람의 결정), 오타는 아무것도 허락하지 않는다(닫히는 쪽)
- `Session`·`Token`·`Attempt`·`APIToken`은 모델 `admin=hidden`
- `Can(ctx, app, model, action string) (bool, error)`(★사람의 결정: 오류는 500. `Config.Can`도 바꾼다: `contrib/admin`의 호출과 시험의 가짜): 요청의 사용자가 없거나 `IsStaff`가 아니면 거짓(`IsSuperuser`여도), `IsSuperuser`면 참, **`auth.User`·`auth.Permission`의 보기 밖 동작은 superuser만**(★사람의 결정: 권한을 올리는 길은 superuser의 일, 권한 행이 있어도), 그 밖은 권한 행이 있을 때. 쿼리는 `COUNT(*)`의 `:one int64`(행 없음 분기가 없다). 비활성·미인증은 `Current`가 이미 사용자 없음으로 본다(세션·API 토큰 둘 다 `is_active && verified_at`, `Can`은 다시 보지 않는다, 주석에)
- `CreateSuperuser`가 `IsSuperuser`도 참으로
- 같은 권한 행을 두 번 추가하면 unique가 막아 500(4번의 받은 상한, 8번 문서). 마지막 superuser를 끄는 일은 superuser만 할 수 있다(위 규칙), 잠김은 8번 문서에

튜토리얼
- 0019(계정 더하기)의 마이그레이션은 그때의 복사본에서 만들어졌고 걷기는 지금의 복사본으로 `maago add auth`를 돌린다 → 0019를 다시 만든다(`steps.sh unpack`→마이그레이션 다시·커밋 글의 "모델 다섯"을 여섯으로→`pack`). 0020~0022의 패치는 바이트 그대로인지 `pack` 왕복으로 확인(트레일러로 옮기는 길은 0019의 diff가 비어 `git am`이 거절한다, M5 12번 실측)
- README와 D-103의 "복사본을 고쳐도 패치는 낡지 않는다"를 한정: 코드는 낡지 않지만 복사본의 모델이 바뀌면 그 단계의 마이그레이션은 다시 만든다

시험
- 복사본 `auth_test.go`의 `TestCan`(두 방언): 사용자 없음, staff 아님(권한 행과 `IsSuperuser`가 있어도 거짓), staff(행 없음), staff+행(`blog.Post`의 `view`만: 같은 모델의 `change`는 거짓, 앱만 맞는 행·모델만 맞는 행 둘은 아무것도 허락하지 않음, 다른 사용자의 행도), superuser(모두), staff+`auth.User`·`auth.Permission`의 `change`·`add` 행은 그 동작이 거짓이고 `view` 행은 참. DB 오류는 오류(닫은 DB)
- `contrib/admin`: `Can`의 오류가 인덱스·목록·한 행·추가에서 500이고 `OnError`가 듣는다
- `TestAddAuth`의 프로젝트에 admin을 켠다: `authMain`에 `admin.App(admin.Config{Models: [][]admin.Model{auth.Admin}, Can: auth.Can})`, 마이그레이션 기대에 `auth_permission`·`is_superuser`. `site`의 `TestAdmin`(두 방언, 진짜 `auth.Current`·`auth.Can`·DB): 네 사용자 × (인덱스, `User`·`Permission` 목록, `User` 한 행 GET·POST, `Permission` 추가 GET·POST)의 기대 코드 행렬(완료 조건 4), 인덱스에 숨긴 모델 넷이 없음, `User` 한 행 화면에 해시가 없고 고친 뒤에도 해시가 그대로, superuser가 admin에서 권한 행을 추가하면 그 staff의 동작이 열림. 이 작업은 view·add·change, 지우기 칸은 6번
- `TestBoardTutorial`: 다시 만든 0019로 통과

하지 않기로 한 것: 그룹(v1), 지우기(6번), 권한 행의 앱·모델을 고르는 상자(v1), 권한 캐시, 필드 단위 권한

단계
1. 복사본 auth와 `Config.Can`의 오류 → 확인: 복사본 시험(두 방언), `contrib/admin` 시험
2. `TestAddAuth`에 admin과 `site`의 `TestAdmin` → 확인: 두 방언
3. 튜토리얼 0019 다시, README·D-103 문구 → 확인: `TestBoardTutorial`, 두 모듈 확인 명령

### 계획 리뷰

- codex: ★User 고치기로 자기 권한 올리기, ★Permission 추가로 스스로 권한 주기 → 사람의 결정: `auth.User`·`auth.Permission`의 보기 밖 동작은 superuser만. ★`Can`이 DB 오류를 403으로 삼킴 → 사람의 결정: `(bool, error)`, 500. 비활성·미인증 → `Current`가 막는다(주석). 글자 비교는 대소문자까지 정확히. 권한 행 unique → 500 상한(8번). 0019 다시 만들기의 범위 → `pack` 왕복. 동작 넷의 분담 → 5번 view·add·change, 6번 delete
- fable: ★`Permission.Model`은 임베드한 `maago.Model`과 겹쳐 컴파일되지 않음 → 사람의 결정: `ModelName`. `has_permission`은 `COUNT(*)`로(행 없음 분기 없음). 행렬이 놓치는 변이: 권한 행은 `view`만, staff 아님에 `IsSuperuser`·행을 붙임, 앱만·모델만 맞는 행 둘. 0019의 커밋 글(모델 다섯), README·D-103 문구 한정. 9번 글에서 "URL의 글자 그대로"를 짚을 것. Bearer도 `is_active && verified_at`를 거름

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음. 계획과 다른 것: admingen에서 기본값이 `''`인 문자열 칸은 빈 입력을 받는다(`TestAddAuth`가 `User`에 더하는 `Nickname default=''`가 admin 고치기를 막아서 알았다, D-132에 한 줄. `web`의 모델 폼은 기본값을 보지 않아 같은 칸이 사용자 폼에선 필수: 8번 상한 후보). 0019는 마이그레이션과 커밋 글(모델 여섯)을 다시 만들었고 0020~0022는 바이트 그대로. 변형으로 확인(11개, 모두 잡힘): `IsStaff` 조건, superuser만 규칙, 오류 삼키기, 쿼리 조건 넷, `CreateSuperuser`의 `IsSuperuser`, `PasswordHash` 숨김, `Session` 숨김
- codex: superuser만 규칙이 앱 이름 `"auth"`에 묶여 복사본의 패키지 이름을 바꾸면 조용히 열린다 → 상수 `appName`과 패키지 이름을 대는 `TestAppName`. 권한 추가 POST의 행렬 → 줄 더함(`Permission` add 행이 있는 staff도 403). 비활성·미인증·`Can` 오류 전파·0019의 표는 맞음. 시험을 돌리지 못함(샌드박스)
- fable: 앱 이름 규칙(codex와 같음, 주석과 시험으로). `User` 추가 404 줄 → 더함. `optional`의 뜻 바뀜을 기록에 → 위와 D-132. `-createsuperuser` 도움말 "staff account" → 복사본·시험은 고침, 0020의 `main`은 admin을 켜는 9번에서, TUTORIAL-FUTURE는 10번. 모양 그림의 `Can` → 고침. 6·9번은 이 모양에 붙는다(지우기는 `s.model(r, Delete)`, User·Permission 규칙이 delete도 덮는다)

---

## M6 끝

- [ ] 1~10 완료
- [ ] 위 M6 완료 조건을 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로), 조건마다 시험 이름
- [ ] MILESTONES.md의 M6 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M7 초안

## 초안 리뷰 (2026-10-13)

- codex: CRUD 사실(`ID`만이면 `UpdateX` 없음), CSRF는 `Handler`(D-47 고침), ★Q2의 한 키 두 뜻 → `hidden`은 필드·모델 한 뜻, ★폼 필드 순서·묶음 → v0에 없음(선언 순서, 8번 문서), ★Q3의 기본 거부·superuser·Permission 관리 → D-133, ★`IsSuperuser`와 `IsStaff` → `Can`이 `IsStaff &&`, FK 번호의 없는 대상 → 4번의 폼 오류, 지우기의 경쟁 → D-135, 검색의 두 방언 → D-134, 정렬은 생성된 번호로만, 숨김·읽기 전용은 서버에서, admin POST의 다른 출처 403, 동작별 권한 행렬, 순서(권한 계약을 먼저 → 1·2번이 이음새를 정하고 5번이 채움), scaffold 갱신, 페이지 경계, 완료 조건과 시험 이름
- fable: 모델 폼을 admin이 못 쓴다(폼·바인딩 생성), import 게이트, `NewApp`과 `/admin/` 충돌(★ → 생성 오류), 완료 조건 3은 `LoginRequired`, `ordering` 첫 소비자와 기본값, `Field.FK`는 표 이름, 검색의 ASCII와 `ESCAPE`, D-47 낡음, ★태그 넷을 둘로·일괄 지우기 빼기 → 사람의 결정: 초안대로 전부, ★모델 `admin=-` → `hidden` 하나, ★필터를 SQL 하나로 → 생성된 Go의 조립(D-48 그대로, 2·7번 계획에서 다시), ★모델 단위 권한 → 사람의 결정: 동작까지, Permission의 모델 이름 오타 → 닫히는 쪽(D-133), ★`Can` 하나로 들어오기까지 → D-131, ★opt-in과 빠뜨림 → 사람의 결정: import할 때만·오류, ★D-15 → 사람의 결정: 미룸, Hash 모델 넷 숨김, 숨긴 필드의 영값 → 추가 없음, 2번을 쪼개고 검색·필터를 뒤로, 3·4 경계, 5·6 합침, 그린 화면의 숨김 시험
