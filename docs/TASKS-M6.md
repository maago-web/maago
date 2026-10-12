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
- **`maago.NewApp`·`AppRoute`는 "생성 코드만 부른다"**(`http.go`). `contrib/admin`이 첫 라이브러리 호출자가 된다(ARCHITECTURE §2 문구). admin 라우트는 생성기의 이름·패턴 충돌 검사를 지나지 않아, 사용자 라우트가 `/admin/…`이면 기동 때 `ServeMux`가 panic한다 → 생성 오류로(D-131)
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
     Models: [...]admin.Model{board.Admin…, auth.Admin…},   Config.Can(ctx, app, model, action) bool
     Can: auth.Can})}                                 internal/admingen → 앱마다 admin_gen.go
 board/admin_gen.go: var Admin = []admin.Model{…}       모델 기술, 목록·개수 쿼리(두 방언), 폼·바인딩,
 auth/: IsSuperuser, Permission, Can (복사본)             행 표시, 역방향 FK 개수
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

## M6 끝

- [ ] 1~10 완료
- [ ] 위 M6 완료 조건을 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로), 조건마다 시험 이름
- [ ] MILESTONES.md의 M6 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M7 초안

## 초안 리뷰 (2026-10-13)

- codex: CRUD 사실(`ID`만이면 `UpdateX` 없음), CSRF는 `Handler`(D-47 고침), ★Q2의 한 키 두 뜻 → `hidden`은 필드·모델 한 뜻, ★폼 필드 순서·묶음 → v0에 없음(선언 순서, 8번 문서), ★Q3의 기본 거부·superuser·Permission 관리 → D-133, ★`IsSuperuser`와 `IsStaff` → `Can`이 `IsStaff &&`, FK 번호의 없는 대상 → 4번의 폼 오류, 지우기의 경쟁 → D-135, 검색의 두 방언 → D-134, 정렬은 생성된 번호로만, 숨김·읽기 전용은 서버에서, admin POST의 다른 출처 403, 동작별 권한 행렬, 순서(권한 계약을 먼저 → 1·2번이 이음새를 정하고 5번이 채움), scaffold 갱신, 페이지 경계, 완료 조건과 시험 이름
- fable: 모델 폼을 admin이 못 쓴다(폼·바인딩 생성), import 게이트, `NewApp`과 `/admin/` 충돌(★ → 생성 오류), 완료 조건 3은 `LoginRequired`, `ordering` 첫 소비자와 기본값, `Field.FK`는 표 이름, 검색의 ASCII와 `ESCAPE`, D-47 낡음, ★태그 넷을 둘로·일괄 지우기 빼기 → 사람의 결정: 초안대로 전부, ★모델 `admin=-` → `hidden` 하나, ★필터를 SQL 하나로 → 생성된 Go의 조립(D-48 그대로, 2·7번 계획에서 다시), ★모델 단위 권한 → 사람의 결정: 동작까지, Permission의 모델 이름 오타 → 닫히는 쪽(D-133), ★`Can` 하나로 들어오기까지 → D-131, ★opt-in과 빠뜨림 → 사람의 결정: import할 때만·오류, ★D-15 → 사람의 결정: 미룸, Hash 모델 넷 숨김, 숨긴 필드의 영값 → 추가 없음, 2번을 쪼개고 검색·필터를 뒤로, 3·4 경계, 5·6 합침, 그린 화면의 숨김 시험
