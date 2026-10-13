# M8 작업 목록 — 튜토리얼 검증: 게시판

[MILESTONES.md](MILESTONES.md)의 M8을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).

**M8 완료 조건** (MILESTONES)
- 원본 89개 커밋이 모두 대응표에 있고(대응하지 않는 것은 이유와 함께), 대응한 것은 튜토리얼의 커밋으로 있다 → 6번(표), 3~5번(빈 곳)
- 커밋을 하나씩 걷는 스크립트로 첫 커밋부터 마지막까지 `maago check`와 테스트가 통과한다(PostgreSQL로 바꾼 뒤는 PG에서) → 2·3번
- 일부러 틀리는 커밋마다 기대한 오류 메시지가 나온다 → 이미 `Maago-Expect`가 걷기에서 강제(D-103), README에 찾는 법 한 줄(7번)
- 에이전트가 튜토리얼을 처음부터 따라 하는 확인은 M9 게이트의 셋째 항목과 함께 한다 → M9

**앞에서 넘어온 것**
- TODO M4: 탈출구 뷰의 경로 값은 URL 함수에서 `string`이라 템플릿 검사가 정수를 거절한다. "M8에서 튜토리얼을 PG로 다시 묶을 때 검사기를 고치고 0016의 `print`를 지운다" → 1번(0028의 `(print .ID)`도)
- TODO 첫 릴리스 전: 튜토리얼도 `replace` 대신 태그한 버전 → M8 밖(릴리스, 태그는 사람의 일)
- MILESTONES M8 할일: 배포(단일 바이너리, 설정, 컨테이너 이미지 예시) → 5번

---

## 먼저 확인한 사실 (2026-10-13, 리뷰가 원본과 코드로 다시 봄)

- **원본**(`puddingcamp/release-your-product-with-fastapi`)은 커밋 89개다. 게시판은 지금 0001~0028
- **대략의 대응**(6번에서 한 줄씩): 모델 셋(4~6) → 0002, Alembic(8·9) → `makemigrations`(설정 없음), pytest·conftest(10·16) → `maagotest`, 회원가입·로그인·로그아웃·탈퇴·해싱(20~26·28·29) → 0019·0020(복사한 auth가 준다), JWT(23) → 세션과 API 토큰(D-129), 캘린더 CRUD·중복(30~34·42) → 0008·0011, 목록·상세(43·45) → 0008, 타임슬롯·겹침·과거 막기(36~38·41) → 0011·0012, 상태 필드와 변경(47·48) → 0003·0013, 취소(49) → 0022, 파일 업로드(50~53·68) → 0028, SQLAdmin(55~58·60·63) → 0023·0024, 정적 파일(61) → 0025, CORS·API·프런트엔드(62·64·66·67·69) → 0015, 헤더 토큰·토큰 없음(65·70) → 0021, 구글 캘린더·BackgroundTasks(71~76) → 0017, Streaming(77·78) → 0016, Sentry(79) → `Config.OnError`(0008), `/app`(80·81) → 0018. 대응 없음: 59(이미지 경로)·73(스타일 리팩터)·82~89(pytest 설정·ORM의 lazy·unique()·프런트엔드 URL)와 리팩터·테스트 고침
- **원본 39는 바꾸기가 아니다**: 코드의 방언 분기(`is_sqlite`면 다른 SQL)다. 원본은 끝까지 SQLite로 시험한다
- **빈 곳**: (a) PostgreSQL로 바꾸기(39의 자리), (b) 내 것: 게스트의 자기 목록(44)·`@me`(24), (c) 글 고치기(35 캘린더 변경·46 부킹 변경), (d) 배포
- **걷기는 SQLite뿐이다**(`walk`가 `.env`를 `sqlite:board.db`로 쓴다). `.env`는 패치 밖(D-103). `expected()`는 `Maago-Expect`·`Maago-Run`만 알고 모르는 `Maago-` 트레일러를 거절한다. `steps.sh unpack`도 `Maago-Run`만
- **`maagotest`·`maago migrate`는 `MAAGO_DATABASE_URL`(`.env`)만 읽는다.** `MAAGO_TEST_PG_URL`은 이 저장소의 PG 시험만 읽고 CI가 그것만 준다. `build.Migrate`는 URL로 바로 연다: 같은 서버의 `public`을 다른 시험과 함께 쓰지 않으려면 걷기가 스키마를 따로 만든다(`maagotest`의 `search_path` 꼴)
- **0012 뒤의 마이그레이션은 넷뿐**(0014의 hide_posts, 0019의 accounts, 0021의 post_author, 0028의 attachments). 게시판 쿼리에 방언에만 있는 SQL은 없다(`LIKE '%' || :q || '%'`는 둘 다 된다)
- **복사한 auth는 패치 밖**(D-103, `steps.sh`의 `info/exclude`, README "단계는 `auth/`를 손으로 고치지 않는다"): 단계가 `User`에 필드를 더할 수 없다
- **환경 변수**: 패치에 있는 것은 `MAAGO_DATABASE_URL`·`BOARD_WEBHOOK_URL`·`BOARD_WEBHOOK_TOKEN`·`MAAGO_SUPERUSER_*`·`BOARD_SITE_URL`(0020). 포트는 0018의 `:8000` 고정

## 결정 (2026-10-13, 초안 리뷰 뒤 "에이전트가 편한가"로)

초안의 Q1~Q5를 codex·fable 리뷰 뒤 정했다. 자세한 것은 DECISIONS.

- **D-141 PostgreSQL로 바꾸기**: 원본 39의 자리(0012 뒤, MILESTONES가 정한 대로). 그 단계는 SQLite의 마이그레이션을 지우고 `makemigrations`로 PG의 첫 초안을 새로 쓴다(데이터는 옮기지 않는다: 새 DB에서 시작하는 연습, D-31). 트레일러 `Maago-Database: postgres`가 걷기와 `steps.sh unpack`의 `.env`를 그 단계부터 PG로(모르는 값·두 번은 오류). 걷기는 `MAAGO_TEST_PG_URL`의 서버에 걷기마다 스키마를 만들어(`search_path`, 끝에 지움) `.env`의 `MAAGO_DATABASE_URL`로 쓴다. 로컬에 PG가 없으면 그 단계 앞에서 건너뛰고(이유), `CI`면 실패(D-91). 뒤 단계의 마이그레이션 넷은 PG로 다시
- **D-142 더할 단계 셋**: `Me`(`/me` 내 글 목록, `GET /api/me` 토큰으로. 복사한 auth는 손대지 않는다, 커스텀 User 필드는 메시지의 한 문단으로), `Edit a post`(작성자·staff), `Ship it`(마지막: 바이너리 하나, 환경 변수, `PORT`(기본 8000), `Dockerfile` 예시: 다단계·`CGO_ENABLED=0`·`gcr.io/distroless/static`·`media/` 볼륨. 걷기의 확인은 `CGO_ENABLED=0 GOOS=linux go build`와 이미 있는 `-createsuperuser`의 기동. Docker는 돌리지 않는다)
- 대응표는 `examples/board/MAP.md`, 89줄 한 줄씩(원본 커밋 → 게시판 커밋 → Maago 기능 → 마일스톤, 대응 없음은 이유). "복사한 auth가 준다"와 "게시판이 시험한다"를 가른다. 원본 13·14·18(공개 프로필)은 대응 없음(작성자는 글에), 27(계정 변경)은 복사본의 비밀번호 바꾸기(이메일·이름 바꾸기는 v0에 없음), 39는 "같은 자리에 D-31의 답(분기 없이 하나로)"

## 작업

1. **탈출구 뷰의 정수 경로 값**(TODO M4): 템플릿 검사가 탈출구 라우트의 `{{url}}`에 정수를 받는다(런타임 `URLFunc`은 이미 받는다). → 확인: `internal/web`의 시험, 0016·0028의 `(print …)`를 지운 뒤 `TestBoardTutorial`
2. **걷기의 PG**(D-141): 트레일러 `Maago-Database`(`expected`, `steps.sh unpack`), 걷기마다 스키마, 로컬은 건너뛰고 CI는 실패. → 확인: `TestBoardTutorialCatches`에 줄(모르는 값, 두 번), PG 없이 `CI`면 실패
3. **PostgreSQL로 바꾸는 단계**(D-141): 0012 뒤 새 단계, 마이그레이션 넷 다시, 패치 번호 밀림(`TestBoardTutorialCatches`의 패치 이름, README·MILESTONES의 번호). → 확인: `TestBoardTutorial`(PG), 0001~0012는 바이트 그대로
4. **`Me`와 `Edit a post`**(D-142): 단계 둘. → 확인: `TestBoardTutorial`, `TestBoardTutorialCatches`에 단계마다 한 줄(권한 검사 빼기)
5. **`Ship it`**(D-142): 마지막 단계. → 확인: 걷기의 교차 빌드
6. **대응표** `examples/board/MAP.md`(번호가 정해진 뒤). → 확인: 시험이 표의 게시판 커밋 열을 `steps/*.patch`와 맞춤(모든 패치가 한 번은 나오고, 표의 단계가 다 있음), 원본 줄은 89
7. **완료 조건과 문서**: "M8 끝"에 조건마다 시험 이름, README(일부러 틀리는 단계를 찾는 법 `grep -l Maago-Expect steps/*.patch`), MILESTONES의 M8 표·할일, TUTORIAL-FUTURE

---

## M8 끝

- [ ] 1~7 완료
- [ ] 위 M8 완료 조건을 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로), 조건마다 시험 이름
- [ ] MILESTONES.md의 M8 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M9 초안

## 초안 리뷰 (2026-10-13)

- codex: 받음 — `maagotest`·`migrate`는 `MAAGO_DATABASE_URL`만(걷기가 `MAAGO_TEST_PG_URL`을 `.env`로 옮긴다), 트레일러는 `expected`와 `steps.sh` 둘에, 걷기마다 PG 스키마, 건너뛰는 자리는 바꾸는 단계 앞, 대응표의 "따위"를 커밋마다, Q4에 권한 시험, 배포 시험이 약함(→ 교차 빌드와 `PORT`). 받지 않음: Q3의 이메일·이름 바꾸기(복사본을 고쳐야 한다: D-103), Docker 빌드 시험(새 외부 의존)
- fable(원본 89 커밋을 실측): 받음 — ★Q3의 `Nickname`은 복사본이 패치 밖이라 안 된다 → `Me`(원본 44·24), 원본 39는 바꾸기가 아니라 방언 분기, 비용은 "16단계 rebase"가 아니라 마이그레이션 넷과 번호, 걷기의 PG 스키마, 미배정 커밋 15개(42·43·45·59·62·66·67·70·73·82~89)와 79는 0008, Q4·Q5의 확인(Catches 한 줄, 교차 빌드), 베이스 이미지 하나(`distroless/static`), `BOARD_SITE_URL`은 0020에 있다, 대응표의 확인이 동어반복 → 패치와 맞춤, 대응표는 번호가 정해진 뒤, 2번을 셋으로(검사기·걷기·단계), 일부러 틀리는 목록은 README에 grep 한 줄. 받지 않음(선택): 일부러 틀리는 "SQLite 마이그레이션을 PG에서" 단계(PG 오류 글이 길다)
