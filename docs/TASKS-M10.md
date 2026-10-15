# M10 작업 목록 — 연구 반영과 첫 릴리스

[MILESTONES.md](MILESTONES.md)의 M10을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
설계의 갈림길은 "Claude·Codex가 Maago로 서비스를 만들 때 편한가"로 정한다(사람의 결정, 2026-10-13).
근거는 [RESEARCH-AGENT-FIT.md](RESEARCH-AGENT-FIT.md), 결정은 D-147~D-153과 아래의 고침.

**M10 완료 조건** (MILESTONES)
- 게이트의 에이전트가 짚은 함정(D-148의 목록)이 각각 시험으로 막혀 있다 → 2·3번
- 뷰의 서명이 사용자와 DB를 말하고, 쓰기 뷰는 요청 트랜잭션 안에서 돈다. 게시판 튜토리얼이 그 꼴로 처음부터 끝까지 걷는다 → 4~9번
- 태그·쿼리 파일·뷰·폼의 문법을 `maago doc`과 `go doc`에서 읽을 수 있고, 문서의 예는 시험이 돌린다 → 10·11번
- 저장소가 공개되고 v0.1.0이 태그되어, 빈 디렉터리에서 `go install …/cmd/maago@v0.1.0`부터 에이전트 게이트를 다시 지난다 → 12·13번

**앞에서 넘어온 것**
- M9 2번(첫 릴리스, 두 모듈 기준이던 계획 → D-151로 태그 하나), 11번(마지막 게이트), 8번(v0 끝) → 12·13·14번
- TODO M8: 운영 DB에 개발 기계가 닿지 않는 곳의 마이그레이션 → 8번(D-153)
- TODO 첫 릴리스 전 셋(`replace`, 튜토리얼의 태그한 판) → 1·12번

**순서의 이유**: 문서(10·11번)는 API가 끝난 뒤에 쓴다. 새 꼴(4~8번)은 옛 꼴과 함께 받아 걷기가 작업마다 초록이고, 게시판을 한 번만 다시 쓰는 9번에서 옛 꼴을 지운다(지우기와 새 생성 오류가 게시판을 고치는 커밋과 같이 들어간다).

---

## 먼저 확인한 사실 (2026-10-15, 초안 리뷰로 고침)

- **바뀔 사용처**: 게시판 마지막 단계의 뷰 16개(그중 탈출구 뷰 2: CSV·다운로드), 라우트 22줄(그중 `maago.LoginRequired` 15), `UserFrom` 4, `DBFrom` 16. 복사되는 auth의 라우트 20줄, GET과 POST를 한 함수가 받는 짝 9(나눌 함수는 게시판 `NewBoard`·`NewPost`·`ShowPost`(댓글)·`EditPost`와 합쳐 13). 게시판 패치 33개, 6,236줄
- **`DBFrom`·`UserFrom`에 기대는 곳**: 탈출구 뷰, admin, `Config.Auth`(`auth.FromSession`이 DB를 읽는다), 라이브러리 코드. 그래서 지우지 않는다(Q4)
- **루트 패키지의 export 약 94개**, 그중 약 40개는 생성 코드만 쓴다(`Bind*`·`Parse*`·`Render`·`Pages`·`URLFunc`…, ARCHITECTURE)
- **사용자에게 보이는 `(D-xx)`**: 진단 글자열 약 20, `go doc -all`에 83. 검사 범위는 진단만이 아니라 `go doc -all`, 생성된 프로젝트, 골든, 사용자 문서
- **이미 되는 것**: 모델 결과의 `SELECT *`(실측), pgx `(*PgError).SQLState()`·modernc `(*Error).Code()`(unique 위반 판정, D-148의 뒤로 미룬 일에 쓴다)
- **모듈 하나**: 런타임 패키지만 import하는 사용자 모듈에 드라이버가 들어오지 않는다(`replace`와 `file://` GOPROXY로 실측, M9 리뷰). 공개 프록시 경로만 12번에 남는다
- **시험의 시간**: `cmd/maago` 로컬 약 7~8분, CI 13분 25초(`-timeout 30m`). 게시판을 다시 쓰면 Catches 줄마다 걷기가 다시 돈다
- **CI의 Go 1.25**: cookiejar가 http에서 `Secure` 쿠키를 버린다(1.26은 지닌다). `TestShortTutorial`의 클라이언트가 지금은 쿠키를 직접 들고 다닌다(6c787ee), 2번이 근본을 고친다
- **리뷰어**: codex는 백그라운드로 부르면 돈다, fable은 사용 한도가 자주 찬다(그때는 general-purpose Opus가 대신하고 기록에 적는다)

## 질문과 답 (사람의 결정, 2026-10-15, 모두 추천)

- **Q1 튜토리얼 글의 언어**: 9번에서 다시 쓰며 영어로(게시판의 커밋 메시지, TUTORIAL-FUTURE). D-103을 고친다
- **Q2 `Secure`**: `auth.SiteURL`의 scheme이 정한다(`https`면 붙이고 `http`면 뺀다). 호스트 이름을 보지 않는다. D-128을 고친다
- **Q3 쓰기 뷰의 경계**: POST(GET이 아닌) 라우트의 타입 있는 뷰가 `maago.DB`만 받으면 생성 오류(`fix: take maago.Tx`). 오류든 panic이든 롤백. 실패해도 남아야 하는 쓰기(시도 횟수 같은)는 `Tx`와 함께 `maago.DB`도 받아 그쪽으로. D-150을 고친다
- **Q4 `DBFrom`·`UserFrom`**: 탈출구 뷰·라이브러리·`Config.Auth`를 위해 남긴다. 타입 있는 뷰 안에서 부르면 생성 오류(`fix: take it as a parameter`). D-149를 고친다

## 작업 (초안 리뷰 뒤)

1. **모듈 하나**(D-151, D-03을 고침): `cmd/maago`의 `go.mod`를 지우고 루트 모듈에 합친다(드라이버는 루트의 `require`). 런타임 패키지의 import 허용 목록 시험(stdlib + 이 모듈의 런타임 패키지만, D-09·D-66)이 드라이버를 막는지 일부러 어겨 본다. 생성물 머리에 생성기 판, 런타임이 기동 때 어긋나면 "vX로 다시 gen". CI·시험의 임시 프로젝트(`replace`)·`steps.sh`·문서 → 확인: 한 `go test ./...`, `go mod tidy -diff`, 허용 목록 시험이 드라이버 import에 실패, 판이 다른 생성물에 기동 오류
2. **런타임의 작은 함정**(D-148, D-128을 고침): `Valid()`가 필드의 `Error`도 센다, 쿠키의 `Secure`를 `auth.SiteURL`의 scheme으로(Q2, `TestShortTutorial`의 직접 쿠키를 다시 `cookiejar`로), 성공한 `-createsuperuser`의 한 줄(복사본) → 확인: 함정마다 재현 시험이 먼저 실패, `TestShortTutorial`을 `GOTOOLCHAIN=go1.25.14`로도
3. **생성기의 작은 함정과 진단**(D-148): `choices` 구분자의 오류, 모델 폼의 포인터 필드는 optional, `:one` 스칼라(`bool`·`int64`·`string`·`time.Time`), 이름 바꾸기·DROP의 ⚠를 stderr에, `gen`의 성공 한 줄(`generated N files`), `go list` 오류 전문, In의 fix에 `maago:"form"`, 모델 결과의 fix에 `SELECT *` → 확인: 게이트 기록의 오류마다 시험. `(D-xx)` 빼기는 `see:`와 함께 10번에서
4. **뷰의 서명, 생성기**(D-149): `internal/web`이 서명에서 사용자(`U` 필수·`*U` 선택)와 DB(`maago.DB`·`maago.Tx`)를 읽고 어댑터가 넘긴다. 매개변수의 순서와 꼴, 사용자 타입은 서명에서 추론하고 뷰들 사이에 어긋나면 생성 오류, GET 라우트의 `Tx`는 생성 오류, 사용자 매개변수가 있는 뷰에 `LoginRequired`는 중복 오류. 옛 꼴(`LoginRequired`·`UserFrom`·`DBFrom`)은 아직 받는다 → 확인: 생성기의 골든과 오류 표, 비로그인 요청이 사용자 필수 뷰에서 로그인으로 보내지는 시험, 걷기 초록
5. **뷰의 서명, 복사되는 auth와 admin**(D-149): auth의 9짝을 GET·POST로 나누고 새 꼴로, admin과 `Config.Auth`는 `DBFrom`·`UserFrom` 그대로(Q4). 권한 회귀를 막는 시험: 비로그인·다른 사용자·staff 아님이 각 auth 라우트에서 막히는지 → 확인: `TestAdmin`·`TestStaff`·`TestAPIToken`, 그 권한 시험이 나누기 전과 뒤에 같은 결과, 걷기 초록
6. **요청 트랜잭션**(D-150): `maago.Tx`를 받는 뷰의 어댑터가 Begin·Commit·Rollback. 순서를 못 박는다: 뷰가 돌려준 뒤 오류면 롤백, panic이면 롤백하고 다시 panic(복구 미들웨어가 500), 렌더·리다이렉트는 커밋 뒤(커밋이 실패하면 500이고 응답은 쓰이지 않았다). SQLite는 `BEGIN IMMEDIATE`(읽고 쓰는 트랜잭션의 `SQLITE_BUSY` 피하기). `maago.AfterCommit(ctx, f)`: 커밋 뒤에만, 롤백이면 버림, 그 ctx는 요청이 끝나도 취소되지 않음, panic은 잡아 기록. `Tx`와 `DB`를 함께 받는 뷰 → 확인: panic 뒤 롤백, 오류 뒤 롤백, `AfterCommit`이 롤백 뒤에는 돌지 않음, 커밋 실패의 500, SQLite·PG
7. **잃어버린 갱신**(D-150, D-21·D-111·D-118을 고침): 생성된 Update의 전제 조건(`auto=update`가 있는 모델은 `WHERE id=… AND updated_at=…`, 0행이면 `ErrConflict`, 409), 고친 열만 쓰는 갱신, admin의 409(D-132의 "나중 저장이 이긴다"를 고침) → 확인: 동시 갱신 시험(둘째가 409), 고친 열만 바뀜, SQLite·PG
8. **운영의 최소선**(D-153): `maago.Serve(ctx, addr, h)`(서버 타임아웃, SIGINT·SIGTERM에 graceful shutdown, 진행 중 요청 다음 `AfterCommit` 기다리기, 기다림에 상한), `maago.Migrate(ctx, db, fsys)`(바이너리에 실린 마이그레이션, CLI의 `migrate`와 같은 기록표·검사: 적용된 파일의 체크섬이 다르면 오류, 모르는 기록이 있으면 오류) → 확인: 시험이 신호를 보내 진행 중인 요청과 커밋 뒤의 일이 끝나는지, 바이너리의 `-migrate`가 CLI와 같은 기록표, 두 검사의 오류
9. **게시판 튜토리얼 다시 쓰기와 옛 꼴 지우기**(D-148, D-103을 고침). 단계가 커서 셋으로 나눈다. 각 커밋에서 걷기가 초록
   - 9a. 0001~0016을 새 서명·`Tx`로, 메시지는 영어(Q1), GET·POST 나누기
   - 9b. 0017~0033을 같은 꼴로, 0033은 `maago.Serve`와 바이너리의 마이그레이션. MAP.md의 번호
   - 9c. 옛 꼴 지우기: `maago.LoginRequired`·`WithDB`, 타입 있는 뷰의 `UserFrom`·`DBFrom`은 생성 오류(Q4), POST의 `DB`만 받는 뷰는 생성 오류(Q3). `TestBoardTutorialCatches`의 줄을 새 꼴로(권한 빼기 줄: 서명에서 사용자 빼기가 생성 오류인지 시험 실패인지 줄마다 적는다), TUTORIAL-FUTURE와 `TestShortTutorial` → 확인: `TestBoardTutorial`(PG), Catches, `TestShortTutorial`, `TestBoardMap`
10. **`maago doc`**(D-152, D-06·D-07·D-62를 고침): 검사기의 닫힌 목록(모델 태그, 폼 태그, In 태그, 뷰의 서명, 쿼리 머리, 조건 줄 규칙, 라우트 옵션, 템플릿 함수, admin URL·필드)을 하나의 표로 모으고 검사기와 `maago doc <주제>`가 함께 읽는다. 주제마다 영어로 짧은 설명과 완전한 예 하나. 진단의 `(D-xx)`를 `see: maago doc <주제>`로 → 확인: 표에 없는 키를 검사기가 받으면 시험이 실패(표가 원천), 주제의 예를 시험이 빈 프로젝트에서 `gen`·`check`, 진단에 `(D-`가 0
11. **`go doc`과 생성 코드 전용 export**(D-152): 루트의 패키지 문서를 10번의 표로 생성(사용자 API의 지도와 주제마다 한 단락), 생성 코드만 쓰는 export를 하위 패키지로 → 확인: 생성 코드가 하위 패키지만 부르는지(골든), `go doc maago`에 생성 전용 이름이 없음, 패키지 문서의 예가 `Example` 시험, `go doc -all`·생성된 프로젝트·골든·사용자 문서에 `(D-`가 0
12. **첫 릴리스**(M9 2번, D-146·D-151): 사람이 저장소를 공개하고 `v0.1.0`을 태그. 확인 셋(`git ls-remote`, `GOPROXY=direct`, 기본 프록시), 빈 `GOPATH`·`GOWORK=off`에서 `go install github.com/maago-web/maago/cmd/maago@v0.1.0`, 게시판의 `go.mod`가 v0.1.0, TODO "첫 릴리스 전" 체크. 공개 뒤 고칠 것이 나오면 태그를 옮기지 않고 v0.1.1 → 확인: 그 설치와 `maago help`
13. **마지막 게이트**(M9 11번): `go install`부터 짧은 튜토리얼과 게시판(메시지만)을 서로 모르는 실행 둘씩, M9과 같은 프롬프트에서 `replace` 줄을 뺀 것. 게시판은 0001~0012에 더해 0020~0022(PG)와 0032~0033(초안·배포)까지 → 확인: 기록, `TestTutorialProject`, 게이트 기록의 함정이 다시 나오지 않음
14. **v0 끝**(M9 8번): MILESTONES의 M10 체크, README, 다음은 v0.2(D-148의 뒤로 미룬 것)와 v1의 순서

## 초안 리뷰 (2026-10-15, codex와 Opus가 fable 대신)

- **순서**: 문서 표가 최종 API 앞에 있었다 → 10·11번으로. 서명·트랜잭션 작업이 걷기를 빨갛게 둔다 → 새 꼴을 옛 꼴과 함께 받고 9번에서 지운다
- **빠진 의존**: 탈출구 뷰·admin·`Config.Auth`가 `DBFrom`·`UserFrom`에 기댄다 → Q4. 사용자 타입을 어떻게 아는지는 설계 결정 → 서명에서 추론(D-149 고침)
- **계약을 못 박지 않았다**: 트랜잭션과 panic의 순서, 커밋 실패, `AfterCommit`의 ctx와 shutdown, SQLite의 `BEGIN IMMEDIATE`, 실린 마이그레이션의 검사 → 6·8번에 적음
- **권한 회귀**: auth를 나누는 동안 막던 것이 풀릴 수 있다 → 5번의 권한 시험
- **크기**: 6·7·9번이 컸다 → 서명 둘(4·5), 트랜잭션 둘(6·7), 게시판 셋(9a~c)
- **사실**: 뷰 16(탈출구 2), auth 짝 9, `go doc -all`의 D-번호 83, 프록시 경로는 `file://` GOPROXY로 이미 연습 → 위에 고침. D-번호 검사 범위를 넓힘(11번). 릴리스에 v0.1.1의 길(12번). 게이트 범위를 PG·배포 단계까지(13번)
- **작업마다 고칠 결정**을 각 작업에 적음

---

## M10 끝

- [ ] 1~14 완료
- [ ] M10 완료 조건을 실제로 돌려 확인, 조건마다 시험 이름
- [ ] MILESTONES.md의 M10 체크박스
- [ ] push 뒤 GitHub Actions
