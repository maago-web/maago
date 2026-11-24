# TODO

## 첫 릴리스 전

- [x] 게시판 튜토리얼(M8)도 `replace` 대신 태그한 버전을 쓴다: 0001·0013의 `go.mod`가 `v0.1.0`과 그 `go.sum` 두 줄(M10 12번). 걷기의 `go.work` `replace`는 이 저장소의 지금 코드를 보려고 남는다
- [x] `cmd/maago/go.mod`의 `replace`를 뺀다: 모듈 하나가 되어 `cmd/maago/go.mod`가 없다(M10 1번, D-151)

## M2

- [ ] `dialect.Normalize`는 `"title"`과 `title`을 다르게 본다. SQLite에서는 같은 이름이라, 손으로 따옴표 없이 쓴 CHECK가 모델의 것과 다르다고 나올 수 있다(M2 3번). 조용히 통과하지는 않는다

- [ ] PG 연습용 DB의 `search_path`에 `public`이 없어, `public`에 설치된 확장(`citext`, `uuid-ossp` 등)의 타입·함수를 이름만으로 쓰는 마이그레이션이 연습용 DB에서 실패한다(M2 2번). 필요해지면 확장이 설치된 스키마만 뒤에 붙이는 법을 정한다

- [ ] 서로 가리키는 두 모델을 한 번에 지우면 `DropTable` 순서로는 PG 초안이 실패한다(FK 고리, M2 6번). 9번이 쓰기 전 다시 적용해 잡는다. 7번이 한 문장 `DROP TABLE a, b`로 풀지 정한다

- [ ] 모델의 테이블 이름이 `maago_`로 시작하면 거부한다. 읽기가 `maago_migrations`를 건너뛰고 초안이 `maago_new_*`를 임시 이름으로 쓴다(M2 8번)

- [ ] 테이블(모델) 이름 변경의 `--rename`. v0은 컬럼만이고, 테이블 이름 변경은 삭제+추가로 보여 allow-drop이 막는다(M2 9번)

- [x] SQLite의 FK는 연결마다 `PRAGMA foreign_keys = ON`이 있어야 강제된다. 연습용·실제 DB 연결은 켠다(M2 10번). 런타임 연결은 `maago.OpenDB`가 드라이버 주소의 `_pragma`로 연결마다 켠다(M3 6번, `TestOpenDBSQLiteConnections`).

## 모델 태그

- [ ] 태그 값 안에 `"`를 쓸 수 없다(백쿼트 태그 + 백슬래시 금지의 결과, M1 3번). label·help에 큰따옴표가 필요해지면 방법을 정한다.
- [ ] Django 필드 타입(~30)·필드 옵션(~20)·Meta 옵션(~20) 전체를 Maago 태그 원칙으로 옮긴 대응표.
      항목마다 갈래(태그 / Go 타입 / Go 메서드 / 제외), 태그 표기, 세 DB(SQLite·MySQL·PostgreSQL)에서의 결과.
  - 이미 정한 원칙: `maago:"…"` 필수, 옵션은 `,`, 문자열은 `'…'`(안의 `'`는 `''`), 묶음은 `(a,b)`,
    선택지는 `a/b/c`, 관계는 ID 필드 + `fk=모델`, 모델 옵션은 임베드한 `maago.Model`의 태그,
    라벨·도움말은 `label='…'`·`help='…'`
  - 제외 후보: 다중 테이블 상속, proxy 모델, GenericForeignKey, `db_tablespace`, `swappable`

## M3

- [x] 앱의 `go test`는 패키지 디렉터리에서 돌아 루트의 `.env`를 보지 못한다(`maago.OpenDB`는 작업 디렉터리의 `.env`, D-101). 게시판 튜토리얼은 절대 경로 환경 변수로 돌린다. 서비스 시험 방법(U-07, M4)의 입력 → M4 10번: `maagotest.DB(t)`가 루트의 `.env`를 찾는다(D-113)
- [x] 게시판 튜토리얼 모듈은 `github.com/maago-web/maago`를 `v0.0.0-…`로 require하고 걷기가 `go.work`로 이 저장소를 가리킨다. 릴리스 전에 태그한 판으로(첫 릴리스 전 항목과 함께)
- [ ] 모델 결과 타입 오류의 fix가 예로 드는 이름(`":many PostSummary"`)이 그 앱에 이미 다른 모양으로 있을 수 있다(게시판 튜토리얼 6단계). 앱에 없는 이름을 예로 들지
- [ ] 빌드 제약(`//go:build ignore` 등)이 붙은 `.go` 파일: 매퍼의 `declared`(이름 충돌)와 `PackageName`이 보통 파일처럼 읽는다. 도구 파일이 다른 package 절을 가지면 그것을 쓸 수 있다. `go/build`의 제약 판정으로 거를지(M3 9번 fable 6)

## M4

- [x] 탈출구 뷰(D-41)의 경로 값은 URL 함수에서 `string`이라 템플릿 검사가 정수를 거절한다(런타임 `URLFunc`은 정수도 받는다). M8에서 튜토리얼을 PG로 다시 묶을 때 검사기를 고치고 0016(지금 0017)의 `{{url "board_export" (print .Board.ID)}}`에서 `print`를 지운다(M4 12·13번, 사람의 결정) → M8 1번: 템플릿 검사는 탈출구 라우트의 값을 `string or int`로 본다(`TestTemplatesPass`의 `feed`)

- [x] `web`의 모델 폼은 `default=''`를 보지 않아 그 문자열 칸의 빈 입력을 거절한다. admin은 받는다(D-132, M6 5번). 사용자 폼도 받을지 → M13 2번: 받는다(`TestModelFormDefaultEmpty`)

## M8

- [x] 게시판의 초안과 예약 글(미래 `publish_at`)은 상세 화면(`ShowPost`, `post_detail.sql`)에서 누구에게나 보인다. 첨부 파일은 0029가 초안이면 작성자·staff만으로 막았다. 막으려면 단계가 하나 더 생겨 D-142가 바뀐다: 사람이 정한다(M8 4번 codex·fable 리뷰). → 단계 0032 `Drafts are their author's`(M8 5번, 사람의 결정 2026-10-14)
- [x] 운영 DB에 개발 기계가 닿지 않는 곳(Cloud Run·fly 따위)에서는 "소스에서 운영 주소로 `maago migrate`"가 어렵다. 바이너리나 이미지가 마이그레이션을 싣는 길을 정할지(M8 6번 fable) → `maago.Migrate`(M10 8번, D-153), 게시판 0033이 시작할 때 부른다(M10 9b)

## M10

- [ ] 모델 폼(`model=X`)은 모델에 없는 필드를 받지 않아, 요청을 건너는 버전(숨은 칸)을 실을 곳이 없다. 게시판 0031의 `ChangePost`는 폼의 action URL에 `?version=`(In의 `query=version`)으로 싣는다. admin처럼 폼이 버전을 실을지. 적어도 10번 전에 `model X has no field Version`의 fix가 그 길을 가리키게(에이전트가 먼저 시도할 길, D-150, M10 9b Opus)
- [x] 한 페이지에 폼 둘(게시판 글 페이지의 댓글과 첨부): In은 폼 하나만 읽어, 다른 폼의 `Field`는 이름·라벨이 비고 템플릿에 손으로 쓴다. 묶지 않은 폼을 기술하는 길이나 예(M10 13번 게시판 둘 다) → M12 2번: 폼마다 생성된 `NewX()`, 게시판은 4번
- [ ] 권한 표가 admin을 빼면 그 라우트가 안 보이는 빈틈: 런타임으로 막는 길(마지막 Handler가 서빙한 앱 중 `Permissions`의 apps에 없는 것을 알린다), D-156
- [x] auth를 쓰는 시험의 API 토큰 도우미(Bearer 역할). 로그인한 쿠키는 M11 3번의 `auth.SignIn`·`maagotest.As`, 확인된 계정은 `SignIn`의 Go 문서 → M12 3번: 복사된 auth의 `SignInToken`, `maagotest.Bearer`
- [x] 생성된 `InsertX`가 돌려주는 시각은 Local, `GetX`는 UTC라 구조체를 `==`로 비교하면 틀린다(게시판 1번의 보고, 아직 확인하지 않음). 맞으면 같은 위치로 → M13 2번: 재현(PG는 읽기도 Local이었다), auto 시각과 모든 읽기를 두 방언 모두 UTC로(`TestTimesUTC`·`TestNullTimeUTC`), 준 시각은 그대로
- [ ] 게시판의 `Maago-Expect` 줄:열이 튜토리얼이 보이지 않는 파일 모양에 기댄다(0006·0009·0024·0027, 둘 다 줄을 맞춰 넣었다). 단계 글이 머리 줄을 보이거나 확인이 줄:열 없이
- [x] admin: 거절된 삭제(참조가 있는 행)가 200, 잘못된 POST가 글 없는 400. 이유가 보이는 응답으로 → M13 2번: 막힌 삭제는 같은 페이지의 409, 400은 이유를 쓴다
- [ ] `maago doc`을 주제 없이 부르면 목록을 보이고 exit 1. `go doc contrib/admin`은 main이 import하기 전에는 패키지를 찾지 못한다

## M11

- [x] API 뷰의 unique 위반: `f.Fail(err)`는 칸에 문구를 쓰지만 API 뷰는 그것을 422로 돌려줄 길이 없다(생성된 `invalidX`는 생성 코드만 부른다, D-120). 지금 쓸 길은 `var ue *maago.UniqueError; errors.As(err, &ue)` 뒤 `maago.Invalid(&f.Title, "…")`나 `maago.InvalidForm("…")`(M11 1번 fable) → M13 2번: 뷰가 오류를 돌려주면 생성 어댑터가 422(폼이 있으면 `Fail`의 자리, 없으면 필드 없는 422, 뷰가 준 상태가 이긴다)

- [ ] 앱을 넘는 정책 사용: 다른 앱의 쿼리가 정책 모델을 읽으면 지금은 오류(정책은 쿼리 앱의 `policies.sql`에서만 찾는다, M11 6번)
- [ ] 복사된 auth의 모델에 정책을 걸면 auth의 쿼리가 `-- policy:`가 없어 실패한다. 표 함수가 읽는 표는 규칙 6이 보지 않는다(규칙 글에 한 줄)
- [ ] 사용자 타입을 dot import한 뷰는 구문에서 타입을 풀지 못해 정책이 "no view takes the user"라고 한다(M11 6번 fable)

- [x] 규칙 6 진단의 fix가 모델의 첫 정책을 권한다(게이트 1번): 정책 이름을 고르지 않고 늘어놓기로. 0035 글이 v0.2.0의 출력을 인용하므로 v0.2.1에서 글과 함께(M11 10번) → M12 1번: 정책이 여럿이면 `<name>`과 이름들, `-- get:`의 정책, 걷기가 글의 `fix:` 인용을 gen 출력과 맞춘다
- [x] `go doc maago.UniqueError`가 생성되는 폼의 `Fail`을 가리키게, `GetX`의 정책 param에 무엇을 넘기는지(`now`는 `time.Now()`) `maago doc query`에 한 마디(M11 10번 게이트 2번) → M12 1번, `func(w, r)`의 `&u`·nil도 `maago doc view`에

## M12

- [ ] UUID를 넣을 때 채우는 태그(`auto=create`처럼): 지금 포인터 아닌 `maago.UUID`는 default가 없어 admin의 새 행 폼이 손 입력을 요구하고, 앱이 `NewUUID()`로 채운다(M12 7번)
- [ ] UUID·JSON 필드의 `default`(M12 7번)
- [ ] JSON의 `-- param:`과 바깥 조인 쪽 JSON 열을 새 결과 타입에 넣는 길: 지금은 모델 열의 결과 필드로만, 헤더에 쓰면 진단(M12 7번 fable)
- [ ] `int32`의 `decimal=`(지금 `int64`만, M12 7번)
- [ ] 있는 행에 UUID 열쇠를 채우는 길이 없다(손 SQL뿐): 게시판 0039의 열쇠는 새 글부터, 웹훅의 `"key"`도 옛 글은 null(M12 8·10번)
- [ ] `maago doc main`의 예는 일꾼의 멈춤 차례(`<-worked`)를 컴파일만 본다: `TestDocExamples`의 PG 가지는 gen·check만 하고 go test는 돌리지 않는다(모든 주제가 같다). 진짜 검사는 게시판 0040의 ship(M12 11번)
- [ ] outbox의 Kind가 여럿이 되면 `main`의 일꾼 함수가 Kind로 가르는 꼴(게시판 0040은 하나, M12 10번)
- [ ] `TestBoardTutorialCatches`가 줄마다 걷기를 처음부터 돌아 단계가 늘수록 길어진다: 로컬 `cmd/maago` 약 27분, CI가 30분을 넘어(89d5726·3ffe25a) `-timeout 60m`으로 올림(M12 12번, 사람의 결정). 줄을 병렬로 돌리거나 같은 단계까지의 걷기를 나눠 쓴다. M13 1번에서 앞부분을 한 번 걷고 줄마다 복사: 로컬 Catches 229초, `cmd/maago` 446초, 리뷰 반영 뒤 369초(PG와 함께). CI의 20분 아래를 잰 뒤 체크(M13 끝)
- [ ] 같은 PG 서버에서 두 worktree의 `cmd/maago` 시험이 동시에 돌면 서로의 스키마를 지운다: 걷기와 `maagotest`의 스키마 이름이 패키지 경로·시험 이름에서만 나온다. 병렬 묶음은 저마다 다른 데이터베이스(`…/m13_1` 따위)를 `MAAGO_TEST_PG_URL`에 준다(M13 1번)
- [ ] `maago gen`의 "app outbox has models, but no package uses outbox.Admin"이 main이 컴파일되지 않을 때(`outbox` import가 빠짐) 엉뚱한 쪽을 가리킨다: 풀리지 않은 이름이라고 하거나 `go build`에 맡긴다(M12 게이트 2번)
- [ ] 게시판 단계 글의 남은 빈틈(M12 게이트): 0040의 시험이 일을 찾는 길(목록 쿼리가 없어 `Idempotency-Key`와 `outbox.GetJob`, 1번), 웹훅 주소가 없을 때의 `Deliver`(2번), "Set its Attempts to 0"에 RunAt이 지나야 한다는 말(2번), 0038의 "main's comment on OnError says so now"(2번), 0039의 `{{with .Meta}}`를 둘 자리(둘 다 " · "로 씀, 패치는 ", ", 짚은 것은 1번), 0039에 Go가 읽지 못하는 파일의 Meta는 nil이라는 말이 없다(둘 다 짐작, 맞음)
- [ ] 0040의 시험 `adminApp()`에 `outbox.Admin`이 없다: 두 게이트 모두 0025의 "main's admin"이라는 주석을 보고 넣었다. 패치를 맞추거나 글에 한 줄(M12 게이트, fable)

## M13

- [ ] admin 삭제의 경합: 참조를 센 뒤 잠그지 않고 지워, 그사이 다른 트랜잭션이 참조를 더하거나 행을 지우면 `Delete`의 FK 오류는 500, `sql.ErrNoRows`는 404가 된다(409 페이지가 아니다). 데이터는 안전하다(FK가 막고 트랜잭션이 되돌린다). 고치려면 행 잠금(방언마다)이나 FK 오류 알아보기(런타임은 드라이버를 모른다, D-09)가 든다(M13 2번 구현 리뷰, codex)
- [x] `MediaStore`에서 런타임이 아직 부르지 않는 것: `List`·`Delete(name)`(7번 `SweepMedia`, `Put`의 n은 5번 `SaveMediaFrom`이 쓴다). 그 작업이 쓰지 않으면 지운다(M13 4번 구현 리뷰, ponytail 3). 7번에서: `SweepMedia`가 `List`와 `Delete(name)`(열쇠와 `.tmp-`+열쇠)을 쓴다
- [ ] 6번 `contrib/s3`의 진짜 서버: 로컬에서 MinIO를 돌리지 못해 `TestServer`는 늘 skip이었다. push 뒤 CI의 `TestServer`가 PASS(skip 아님)인지, `docker run` 줄·health 경로·MinIO의 HEAD 404·`start-after`·쿼리의 `+`를 본다(M13 6번 구현 리뷰, fable)
- [x] 6번 `contrib/s3`: `Put`의 형식이 객체의 `Content-Type`이 되고 ttl이 `X-Amz-Expires` 정수 초가 됨을 `Store`가 문다(메모리 가짜처럼 `Put`의 형식을 URL의 응답에, 초 단위 아닌 ttl은 오류). `Store`의 열쇠 검사 줄이 S3에서도 런타임 검사를 본다(M13 4번 구현 리뷰, fable 1·3). 6번에서: 형식은 `Store`의 URL `Content-Type` 줄이 가짜에서(진짜 MinIO는 CI), 초 단위 ttl은 `TestConfig`, 열쇠 검사는 `Store`의 런타임 줄과 `TestNames`의 `URL`이 문다
- [x] 9번 미디어 문단: "태그를 더한 배포에서는 `SweepMedia`를 부르는 프로그램도 함께 다시 빌드한다". 글이 둘 이상이면 내는 오류는 한 바이너리 안의 낡은 생성물만 잡고, 옛 바이너리로 도는 일꾼은 옛 쿼리로 새 열의 파일을 고아로 본다(묵은 열쇠를 새 열로 옮기는 데이터 마이그레이션 때 실제로 지워진다)(M13 7번 구현 리뷰, fable 6). M13 9번에서: `maago doc main`의 청소 문단이 "태그를 더한 배포는 청소하는 바이너리를 모두 다시 빌드, 옛 것은 새 열을 몰라 그 열의 파일을 하루가 지나면 지운다"를 말한다
- [x] `SiteURL` 둘의 뜻이 다르다: 복사된 auth의 `auth.SiteURL`은 경로가 없어야 하고(`siteURL`이 거절) `LocalMedia.SiteURL`은 `Config.Prefix`를 담아야 한다. 게시판 0044는 `strings.TrimSuffix(auth.SiteURL, "/") + "/app"`이라 쓴다: 끝 `/`가 있는 주소(`http://localhost:8000/`)는 auth가 받는데 그대로 붙이면 `Handler`가 `"//app"`으로 멈췄다(구현 리뷰 codex, `ship`이 끝 `/`로 띄운다). 고치는 길(로컬 URL이 Prefix를 스스로 붙이고 `SiteURL`은 주소만)은 D-160의 4번 고침을 뒤집는 공개 계약이라 9번이나 M14에서 사람에게: 두 꼴과 대가(M13 8번). M13 9번: 지금 계약(`LocalMedia.SiteURL`은 `Config.Prefix`까지)을 `maago doc form`에 적었다. 바꿀지는 사람에게, v0.4.0 태그(10번) 전이면 깨는 값이 싸다 → M13 9번 고침: B(주소만, 접두어는 런타임, `Handler` 전의 URL은 오류), 게시판 0044는 `SiteURL: auth.SiteURL`
- [ ] 없는 표의 진단이 모델의 표 이름을 말하지 않는다: 새 모델 `ExportFile`의 표를 `board_exportfile`로 짐작해 쓴 쿼리에 `relation "board_exportfile" does not exist … (checked against the models)`뿐이고, 같은 실행의 `table "board_export_file" is missing`을 읽어야 안다. did you mean은 M14 진단 묶음에(M13 8번)
- [x] 9번 main 문단: 청소나 일꾼이 미디어를 쓰면 `Config.MediaDir` 줄임은 쓸 수 없다(같은 저장소를 `WithMedia`에 줘야 한다). 게시판 0041이 `media := maago.LocalMedia{Dir: "media"}`를 만들어 `Config.Media`·일꾼·청소에 함께 준다: 문서의 예도 이 꼴로, `MediaDir`은 요청 밖에서 미디어를 쓰지 않는 앱의 줄임이라고. 0041·0043이 main에 손으로 쓴 청소 고루틴(약 12줄)과 기동 때의 `List`(약 3줄)도 모든 앱이 같은 꼴로 쓸 것이라 문서의 예에 싣는다(M13 8번). M13 9번에서: `maago doc main`의 예가 `media := maago.LocalMedia{Dir: "media"}` 하나를 `Config.Media`·일꾼·청소에 주고 청소 고루틴과 `handler` 뒤의 기동 `List`를 싣는다. "쓸 수 없다"가 아니라 "`MediaDir`은 요청 안에서만 미디어를 쓰는 앱의 줄임, 일꾼이나 청소가 쓰면 저장소 하나를 같은 값으로"라고 쓴다(따로 만든 같은 디렉터리의 `LocalMedia`도 돈다)
- [ ] 열쇠 열이 있는 모델마다 `func (x X) media() maago.Media`를 손으로 쓰고, `SaveMediaFrom`이 준 `Media`를 `NewXInput`에 칸마다 옮긴다(게시판의 첨부와 내보내기, 둘째). 태그 `media`가 있는 모델에 생성기가 둘을 만들지는 M14 진단·생성 묶음에서 본다(M13 8번)

## 언젠가

- [ ] 마이그레이션 롤백(down 마이그레이션)
- [ ] 템플릿 컴파일(T3): 같은 html/template 파일을 Go 코드로 번역해 런타임 reflect까지 0으로.
      이스케이프는 stdlib 결과를 재사용한다. 필요해질 때만.
