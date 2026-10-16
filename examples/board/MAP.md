# 대응표: 원본 89 커밋 → 게시판

게시판 튜토리얼이 [release-your-product-with-fastapi](https://github.com/puddingcamp/release-your-product-with-fastapi)의 커밋을 어떻게 옮겼는지(D-142, [MILESTONES.md](../../docs/MILESTONES.md)의 M8).
게시판 칸은 [steps/](steps/)의 단계 번호, `—`는 대응 없음이고 Maago 칸이 그 이유다.
Maago 칸의 머리 말: "복사한 auth가 준다"는 0020의 `maago add auth`가 쓴 코드와 그 시험이 보이는 것, "게시판이 시험한다"는 게시판의 시험이 보는 것, "main이 쓴다"는 `main`이 설정만 하고 시험이 부르지 않는 것.
`cmd/maago`의 `TestBoardMap`이 이 표를 `steps/`와 맞춘다.

| # | 원본 커밋 | 게시판 | Maago | 마일스톤 |
|---|---|---|---|---|
| 1 | `a22f553` Initial commit | 0001 | main이 쓴다: 저장소의 시작. 게시판은 `go.mod`와 `main.go`로 시작한다 | M0 |
| 2 | `03aa544` ADD Initial commit | 0001 | main이 쓴다: 모듈과 `main`, 의존은 `go.mod`(Poetry 대신) | M0 |
| 3 | `c186d7e` ADD User, OAuthAccount 모델 | 0020 | User는 복사한 auth가 준다: `maago add auth`가 복사한 `auth.User`(D-123). OAuthAccount는 대응 없음: 소셜 로그인은 v1(D-50) | M5 |
| 4 | `e0656ab` ADD Calendar model | 0002 | 게시판이 시험한다: 모델은 struct와 `maago` 태그(Calendar → Board) | M1 |
| 5 | `ce231de` ADD TimeSlot 모델 | 0002 | 게시판이 시험한다: TimeSlot → Post(고정·예약 시각은 0012에서) | M1 |
| 6 | `a558b75` ADD Booking 모델 | 0002 | 게시판이 시험한다: Booking → Comment, `fk=Post` | M1 |
| 7 | `1b41ed3` ADD db 파일 | 0001 | main이 쓴다: DB 연결은 `maago.OpenDB`, 주소는 `MAAGO_DATABASE_URL`이나 `.env`(D-101) | M3 |
| 8 | `9757487` ADD Alembic 기본 설정 | 0002 | 게시판이 시험한다: Alembic 설정 대신 `maago makemigrations`·`migrate`, 설정 파일 없음(D-32) | M2 |
| 9 | `0d90bc6` ADD 마이그레이션 스크립트 템플릿 수정 | 0002 | 게시판이 시험한다: 원본의 첫 마이그레이션: 0002의 `0001_initial.sql`. 템플릿은 없고 초안이 SQL 파일이다(D-32) | M2 |
| 10 | `4c70bb2` ADD pytest 의존 패키지 추가와 설정 반영 | 0002 | 게시판이 시험한다: pytest 대신 `go test`, DB는 `maagotest.DB(t)`(D-113) | M4 |
| 11 | `1bbf537` ADD test hello | 0002 | 게시판이 시험한다: 첫 시험(모델을 넣고 읽기) | M4 |
| 12 | `23d0bb9` ADD calendar 관련 구현과 테스트 코드 | — | 달력 그리기의 날짜 계산(월의 첫 요일·마지막 날): 프레임워크 밖의 순수 함수, Go는 stdlib `time` | — |
| 13 | `f068e95` ADD user_detail API 구현과 테스트 코드 | — | 공개 프로필이 없다: 작성자는 글에 있고 복사한 auth는 패치 밖이라 User에 필드를 더하지 않는다(D-142) | — |
| 14 | `3c6ada1` ADD user_detail에 대한 테스트 코드 | — | 13의 시험, 같은 이유 | — |
| 15 | `b9e62e2` REFACTOR 라우터 등록을 별도 함수로 분리 | 0008 | 게시판이 시험한다: 라우트 등록은 앱마다 `urls.go`의 라우트 표 하나(D-115) | M4 |
| 16 | `9159914` ADD conftest 에서 세션, 애플리케이션 서버 앱, 테스트 클라이언트 픽스쳐 정의 | 0008 | 게시판이 시험한다: 세션·앱·클라이언트 픽스처 대신 `maagotest.DB`와 `maago.Handler`, `httptest`(D-113) | M4 |
| 17 | `3a1f246` ADD 데이터베이스 세션을 의존성 주입해주기 위한 각주 정의 | 0008 | 게시판이 시험한다: DB 주입은 뷰의 매개변수 `db maago.DB`(D-149) | M4 |
| 18 | `74216ae` ADD 계정 ID로 사용자 정보 가져오는 API와 테스트코드 | — | 13과 같다: 계정 ID로 사용자를 보이는 API가 없다 | — |
| 19 | `f0f941f` FIX 구현 변경으로 테스트 깨지는 것 수정 | — | 구현 변경에 따른 시험 고침: 걷기가 단계마다 시험을 돌려 깨진 채로 남지 않는다 | — |
| 20 | `0ac48c9` ADD 회원가입 함수와 테스트 코드 | 0020 | 복사한 auth가 준다: 가입 뷰와 그 시험 | M5 |
| 21 | `952c4a2` ADD 회원가입 API에서 SignupPayload 로 입력값 검증 | 0020 | 복사한 auth가 준다: 가입 폼의 검증(`maago.Form`) | M5 |
| 22 | `533e340` ADD UserOut 스키마를 API 에 반영 | 0030 | 게시판이 시험한다: 내보내는 모양은 `MePage`, User를 통째로 JSON에 넣지 않는다(비밀번호 해시) | M4 |
| 23 | `bd2f4a5` ADD 로그인 API, JWT 구현 | 0021·0022 | 복사한 auth가 준다: 로그인과 세션 쿠키(D-128). JWT 대신 API 토큰(D-129): 게시판은 0022에서 Bearer로 쓴다 | M5 |
| 24 | `3f041eb` ADD @me API 구현과 테스트 코드 | 0030 | 게시판이 시험한다: `/me`와 `GET /api/me`, 뷰 하나에 라우트 둘 | M4·M5 |
| 25 | `729845e` ADD 회원가입하면 비밀번호를 해싱 처리한다. | 0020 | 복사한 auth가 준다: PBKDF2 해싱(D-127) | M5 |
| 26 | `f041c43` ADD 회원가입 직후에 로그인을 성공하는 테스트 시나리오 | 0020 | 복사한 auth가 준다: 가입 뒤 인증·로그인 시나리오. 게시판은 0021부터 `login` 도우미로 로그인한 브라우저를 쓴다 | M5 |
| 27 | `28a5dc4` ADD 계정 정보 변경 API와 테스트 코드 | 0020 | 복사한 auth가 준다: 비밀번호 바꾸기. 이메일·이름 바꾸기는 v0에 없다(D-142) | M5 |
| 28 | `3bc2a84` ADD 로그아웃 API | 0020 | 복사한 auth가 준다: 로그아웃. 게시판은 0021에서 머리에 로그아웃 폼 | M5 |
| 29 | `e350745` ADD 회원 탈퇴 API | 0020 | 복사한 auth가 준다: 탈퇴는 비활성, 행은 남는다(D-130) | M5 |
| 30 | `c1092bf` ADD 호스트 username 으로 캘린더 정보 가져오는 구현 | 0008 | 게시판이 시험한다: 게시판을 ID로 보기(호스트 이름 대신 경로 값) | M4 |
| 31 | `b477925` ADD 호스트나 캘린더가 없는 경우에 대한 구현과 테스트 코드 | 0008 | 게시판이 시험한다: 없는 게시판·글은 `sql.ErrNoRows`가 404(D-116) | M4 |
| 32 | `cdacf18` ADD 호스트의 캘린더 보기 API 구현과 테스트 코드 | 0008·0016 | 게시판이 시험한다: 게시판 화면(0008), JSON 목록(0016) | M4 |
| 33 | `80763dd` ADD 캘린더 생성 API와 테스트 코드 | 0011 | 게시판이 시험한다: 게시판 만들기 폼 | M4 |
| 34 | `264d057` ADD 호스트가 중복 캘린더 생성하지 못하게 막는 구현과 테스트 코드 | 0011 | 게시판이 시험한다: 같은 이름의 게시판: DB를 보는 검사는 뷰가 하고 폼 오류로(D-119) | M4 |
| 35 | `69e8402` ADD 캘린더 정보 변경 API와 테스트 코드 | 0031 | 게시판이 시험한다: 글 고치기(작성자·staff) | M4·M5 |
| 36 | `a46038f` ADD 타임슬롯 추가 API와 테스트 코드 | 0012 | 게시판이 시험한다: 타임슬롯 → 공지 고정 기간 | M4 |
| 37 | `c62ecb6` ADD 타임슬롯 생성 시 payload 유효성 검사 테스트와 구현 | 0012 | 게시판이 시험한다: 폼의 `Validate`: 시작과 끝을 함께, 끝은 시작보다 뒤 | M4 |
| 38 | `02fffbd` ADD 겹치는 시간대 검사 구현과 테스트 코드 | 0012 | 게시판이 시험한다: 같은 게시판에서 고정 기간이 겹치지 않게(`pinned_overlap.sql`) | M4 |
| 39 | `b2b28d5` ADD sqlite3 & postgresql 동시 지원 버전 | 0013 | 게시판이 시험한다: 분기 대신 하나로: PostgreSQL로 바꾼다(D-31·D-141) | M8 |
| 40 | `099dc37` ADD 부킹 생성 시 호스트, 타임슬롯 검사 구현과 테스트 코드 | 0011 | 게시판이 시험한다: 경로의 게시판이 없으면 404, 있는 게시판에 글쓰기 | M4 |
| 41 | `793be6e` ADD 과거일자엔 예약하지 못하게 대응하는 구현과 테스트 코드 | 0012 | 게시판이 시험한다: 지난 시각으로 예약하지 못한다 | M4 |
| 42 | `e1c4893` ADD 중복 신청 대응 구현과 테스트 코드 | 0011 | 게시판이 시험한다: 같은 게시판에 같은 제목은 폼 오류(모델의 `unique=(BoardID,Title)`) | M4 |
| 43 | `b02fca1` ADD 호스트의 부킹 내역 list API와 테스트 코드 | 0008 | 게시판이 시험한다: 게시판의 글 목록, 페이지와 검색 | M3·M4 |
| 44 | `cb5c122` ADD 게스트가 호스트의 부킹 목록 응답하는 API와 테스트 코드 | 0030 | 게시판이 시험한다: 내가 쓴 글 목록 | M4·M5 |
| 45 | `bc16ffa` ADD 특정 예약 데이터 가져오는 API와 테스트 코드 | 0008 | 게시판이 시험한다: 글 하나와 댓글 | M4 |
| 46 | `0cdc945` ADD 부킹 변경에 대한 시나리오와 테스트 코드 구현 | 0031 | 게시판이 시험한다: 글 고치기, 같은 제목 검사는 제 글을 뺀다(`skip_id?`) | M3·M4 |
| 47 | `bd66caf` ADD attendance_status 모델 필드 추가 | 0003 | 게시판이 시험한다: 필드 더하기와 그 마이그레이션(`Post.Views`) | M2 |
| 48 | `cbdf317` ADD 부킹의 참석 상태를 변경하는 API | 0014 | 게시판이 시험한다: 상태 전이 draft → published → archived, 규칙 밖은 409 | M4 |
| 49 | `e3160f7` ADD 게스트가 자신의 부킹을 취소하는 API와 테스트 코드 | 0023 | 게시판이 시험한다: 취소 → 지우기: 댓글이 없으면 지우고 있으면 숨긴다 | M4·M5 |
| 50 | `53fc127` ADD 파일 업로드 연습 구현 | 0029 | 게시판이 시험한다: 파일 폼 `maago.Field[maago.File]`(D-139) | M7 |
| 51 | `9e2b7ec` ADD fastapi-storages 패키지 추가 | 0029 | 게시판이 시험한다: 패키지 대신 `maago.SaveMedia`·`ServeMedia`(D-140) | M7 |
| 52 | `914ecb5` ADD 부킹에 파일 업로드 구현과 테스트 코드 | 0029 | 게시판이 시험한다: 글 첨부, 올리기는 작성자·staff | M7 |
| 53 | `b48cecf` ADD BookingFile 마이그레이션 스크립트 | 0029 | 게시판이 시험한다: Attachment 모델의 마이그레이션 | M7 |
| 54 | `557b4fd` FIX 잘못된 import namespace 조정 | — | import 경로 고침: `go vet`과 컴파일러가 단계마다 잡는다 | — |
| 55 | `d17620e` ADD sqladmin 패키지 추가 | 0024·0025 | 게시판이 시험한다: 패키지 대신 `contrib/admin`(D-131). 0024는 일부러 틀리는 단계라 시험은 0025가 돈다 | M6 |
| 56 | `099f50a` ADD User 모델에 대한 UserAdmin 정의 | 0025 | 게시판이 시험한다: admin의 User 화면(`maago gen`이 쓰는 `auth.Admin`)에서 staff를 켠다 | M6 |
| 57 | `5a46147` ADD SQLAdmin 기본 기능을 활용해 어드민 페이지 정의 | 0024·0025 | 게시판이 시험한다: admin 열기(0024), 게시판 모델의 목록·검색·필터 태그(0025, D-132) | M6 |
| 58 | `18b5fb3` ADD SQLAdmin 메서드 오버라이딩 구현 | 0025 | 게시판이 시험한다: admin 커스터마이징은 모델 태그. `account_status`는 복사한 auth의 `IsActive` | M6 |
| 59 | `87ad555` FIX 이미지 경로 변경 | — | admin 열의 이미지 경로: 첨부는 admin에 행으로만 있고 미리 보기가 없다(공개 미디어 라우트가 없다, D-140) | — |
| 60 | `21436c7` ADD SQLAdmin 에 인증 백엔드 적용 | 0024·0025 | 게시판이 시험한다: 로그인하지 않은 admin은 로그인 화면으로, 권한 행으로 되는 것과 안 되는 것(0025). `Can`은 복사한 auth의 것(D-133) | M6 |
| 61 | `ab9a715` ADD 정적 파일 처리 | 0026 | 게시판이 시험한다: 앱의 `static/`과 `{{static}}`, 출력 CSS는 Tailwind(D-136~D-138) | M7 |
| 62 | `8df5e19` ADD TimeSlot 가져오는 API | 0016 | 게시판이 시험한다: JSON 목록 API | M4 |
| 63 | `0c18f12` FIX 토큰 만료 시 admin 인증 페이지로 가도록 False 반환 | 0025 | 게시판이 시험한다: 로그인하지 않은(세션이 끝난) admin 요청은 로그인 화면으로 303 | M5·M6 |
| 64 | `1bfec56` ADD CORS 설정 | 0016 | 게시판이 시험한다: 믿는 출처 `Config.Origins`, preflight(D-47) | M4 |
| 65 | `7392c1a` ADD HTTP Header로 넘어오는 인증 토큰으로도 인증 검사 | 0022 | 게시판이 시험한다: `Authorization: Bearer` 토큰으로 글쓰기(D-129) | M5 |
| 66 | `c25ceca` FIX timeslots API Path 변경 및 TimeSlot id 을 스키마에 추가 | 0016 | 게시판이 시험한다: JSON의 키는 Go 필드 이름(`ID` 포함) | M4 |
| 67 | `f208177` ADD 호스트 목록 가져오는 API | 0008 | 게시판이 시험한다: 게시판 목록은 홈 화면. 따로 JSON API는 두지 않는다 | M4 |
| 68 | `34d39a7` ADD file upload UI 에 대응한 수정 | 0008·0029 | 게시판이 시험한다: 목록 응답의 모양: 쪽(0008), 글 화면의 첨부 목록(0029) | M4·M7 |
| 69 | `38fc41d` ADD 약속잡기 Front-end 구현체 | 0016 | 게시판이 시험한다: 프런트엔드는 튜토리얼에 없다: 다른 출처에서 부르는 것을 시험이 흉내 낸다 | M4 |
| 70 | `fef5984` ADD 인증 토큰 없는 경우에 대한 exception | 0022 | 게시판이 시험한다: 토큰 없는 API는 401과 `WWW-Authenticate: Bearer` | M5 |
| 71 | `40c07c5` ADD google api sdk | 0018 | 게시판이 시험한다: SDK 대신 stdlib `net/http` | M4 |
| 72 | `d361824` ADD 구글 캘린더 이벤트 생성 구현과 테스트 코드 | 0018 | 게시판이 시험한다: 외부 API 부르기 → 웹훅, 가짜 서버로 시험 | M4 |
| 73 | `77f2c7a` REFACTOR 소소한 코드 스타일링 처리 | — | 스타일 리팩터: `gofmt`가 단계마다 본다 | — |
| 74 | `9d19e1f` ADD 구글 캘린더 일정 수정, 삭제, 가져오기, 목록 구현과 테스트 코드 | 0018 | 게시판이 시험한다: 실패와 재시도(5xx는 다시, 4xx는 그만) | M4 |
| 75 | `292b6f4` ADD 구글 캘린더 연동 구현 | 0018 | 게시판이 시험한다: 연동: 게시하면 알린다(0032에서 쓸 때가 아니라 게시할 때로) | M4 |
| 76 | `86e3d97` ADD BackgroundTasks 로 구글 캘린더 작업 처리 | 0018 | 게시판이 시험한다: 응답 뒤의 일은 goroutine과 `context.WithoutCancel`(D-118) | M4 |
| 77 | `b951305` ADD BackgroundTasks와 StreamingResponse | 0017 | 게시판이 시험한다: 스트리밍 응답은 탈출구 뷰(D-41) | M4 |
| 78 | `c294fc9` ADD Calendar 에 Streaming 처리 | 0017 | 게시판이 시험한다: 게시판의 CSV를 쪽마다 흘려 보낸다 | M4 |
| 79 | `17bc140` ADD Sentry 패키지 추가와 설정 | 0008 | main이 쓴다: Sentry 대신 `Config.OnError`, 5xx와 panic이 간다(원본도 설정만) | M4 |
| 80 | `8c4996b` FIX 기본 경로를 /app 으로 변경 | 0019 | 게시판이 시험한다: `/app` 아래에 올리기 `Config.Prefix`(D-122) | M4 |
| 81 | `54162ca` FIX 경로를 /app 이하로 조정 | 0019 | 게시판이 시험한다: 링크와 URL 함수에 접두어가 붙는다 | M4 |
| 82 | `0d0d2f7` FIX pytest settings | — | pytest 설정: `go test`에는 설정이 없다 | — |
| 83 | `4f80046` Add Booking.files에 lazy 설정. | — | ORM의 lazy 설정: Maago에는 ORM이 없고 SQL은 손으로 쓴다(D-71) | — |
| 84 | `4d99439` Fix Booking.files 에 lazy 설정하는 경우, unique() 호출 | — | 83의 `unique()`, 같은 이유 | — |
| 85 | `85748f9` Add pytest-lazy-fixtures 패키지 | — | pytest 플러그인: Go 시험에는 필요 없다 | — |
| 86 | `845c17a` Add pytest-lazy-fixtures 패키지 | — | 85와 같다 | — |
| 87 | `0001f65` Fix api url 수정 | — | 프런트엔드의 API 주소: 프런트엔드가 튜토리얼에 없다 | — |
| 88 | `953b4a6` Fix api url 변경, useEffect 의존성 수정 | — | 87과 같다(useEffect) | — |
| 89 | `96f37fb` Fix get_booking_by_id()에 unique() 추가 | — | 83의 `unique()`, 같은 이유 | — |

## 원본에 없는 단계

| 게시판 | 왜 있나 | 마일스톤 |
|---|---|---|
| 0004 | 필드 이름 바꾸기와 `--rename`(D-36). 원본은 필드를 더하기만 한다 | M2 |
| 0005 | 쿼리 파일과 `{조건}` 줄: SQL을 손으로 쓰는 매퍼(원본 12의 일부) | M3 |
| 0006 | 일부러 틀린다: 모델 결과에 빠진 열(`Maago-Expect`) | M3 |
| 0007 | 0006을 고친다 | M3 |
| 0009 | 일부러 틀린다: 템플릿의 오타 둘이 한 번에(D-61) | M4 |
| 0010 | 0009를 고친다 | M4 |
| 0015 | 댓글과 숨기기: 지울 수 없는 글(FK)을 숨긴다 | M4 |
| 0027 | 일부러 틀린다: 없는 static 파일 | M7 |
| 0028 | 0027을 고친다 | M7 |
| 0032 | 초안과 예약 글은 작성자·staff만(D-142) | M8 |
| 0033 | 내보내기: 바이너리 하나, 환경 변수, Dockerfile(D-142, 원본에는 배포 커밋이 없다) | M8 |
