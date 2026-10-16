# 게시판 튜토리얼

Maago로 게시판을 만드는 긴 튜토리얼([MILESTONES.md](../../docs/MILESTONES.md)의 M8). 커밋 하나에 기능 하나를 더한다.
`steps/`의 패치가 그 커밋들이고(D-103), 커밋 메시지가 튜토리얼 글과 짝이다.
원본 튜토리얼의 89 커밋이 어느 단계로 갔는지는 [MAP.md](MAP.md)(`TestBoardMap`이 `steps/`와 맞춘다).

- 걷기: `cmd/maago`의 `TestBoardTutorial`이 패치를 하나씩 적용하며 `maago gen`·`check`·`migrate`와 앱의 `go test`를 돌린다.
  커밋 메시지에 `Maago-Expect: <오류 첫 줄>`이 있는 단계는 일부러 틀린 단계이고, `maago gen`이 정확히 그 오류를 내야 한다(찾기: `grep -l Maago-Expect steps/*.patch`).
  `Maago-Run: maago add <앱>`이 있는 단계는 패치를 적용하기 전에 그 명령을 돌린다. 끝에서 마지막 단계를 내보내듯이 확인한다: cgo 없이 리눅스용으로 짓고, 걷기의 스키마를 비운 뒤 이 기계용 바이너리 하나로 `-createsuperuser`를 돌리고(바이너리가 시작할 때 마이그레이션한다), 빈 `PORT`와 `BOARD_SITE_URL`로 띄워 `/app/`이 200인지, SIGTERM에 상태 0으로 멈추는지 본다(Docker는 돌리지 않는다). 여기서 난 오류는 마지막 패치의 것이다.
  `Maago-Database: postgres`가 있는 단계부터는 PostgreSQL이다(D-141): 걷기가 `MAAGO_TEST_PG_URL`의 서버에 걷기마다 스키마를 만들어 `.env`를 그리로 바꾼다.
  그 변수가 없으면 로컬은 그 단계 앞에서 건너뛰고(뒤 단계의 `TestBoardTutorialCatches` 줄과 CSS 비교도 건너뛴다. 로컬 통과는 거기까지의 증명이다) CI는 실패한다
- 고치기: `./steps.sh unpack DIR`로 풀면 단계마다 커밋이 있는 git 저장소가 된다(`Maago-Run:`의 명령도 그 자리에서 돈다).
  `git rebase`로 고치고 `./steps.sh pack DIR`로 다시 패치를 뽑는다. 템플릿의 클래스를 바꾼 단계는 `DIR/.git/maago tailwind`로
  `board/static/tailwind.css`를 다시 만들어 그 단계에 함께 커밋한다(`TestBoardTutorial`이 마지막 단계의 CSS를 비교한다). 풀어 둔 곳에는 `.env`와 이 저장소를 가리키는 `go.work`,
  그리고 이 저장소의 maago를 빌드한 `DIR/.git/maago`가 있다. `.env`는 마지막 단계의 것이라 `Maago-Database: postgres` 뒤면
  `postgres://localhost:5432/board`다(다른 서버면 고친다). 그 앞 단계로 돌아가 `gen`을 돌리면 PG로 검사된다
- 패치에는 생성물(`*_gen.go`), `maago add`가 쓴 앱(`auth/`), `.env`, DB 파일이 없다. 명령이 만드는 것은 그 명령이 다시 만든다:
  복사된 앱의 코드를 고쳐도 패치는 낡지 않는다(그 모델이 바뀌면 그 단계의 마이그레이션은 다시 만든다, M6 5번). 그래서 단계는 `auth/`를 손으로 고치지 않는다. 공개할 저장소를 만들 때는 커밋마다
  `maago gen`을 돌리고 `Maago-Run:`의 명령을 돌려 그것도 커밋한다(D-67, D-103)
- 앱의 시험은 `maagotest.DB(t)`로 시험마다 새 DB를 얻는다(D-113): `board/`에서 돌아도 루트의 `.env`를 찾고, `maago migrate`가 쓰는 DB는 건드리지 않는다. 그냥 `go test ./...`
