# 게시판 튜토리얼

Maago로 게시판을 만드는 긴 튜토리얼([MILESTONES.md](../../docs/MILESTONES.md)의 M8). 커밋 하나에 기능 하나를 더한다.
`steps/`의 패치가 그 커밋들이고(D-103), 커밋 메시지가 튜토리얼 글과 짝이다.

- 걷기: `cmd/maago`의 `TestBoardTutorial`이 패치를 하나씩 적용하며 `maago gen`·`check`·`migrate`와 앱의 `go test`를 돌린다.
  커밋 메시지에 `Maago-Expect: <오류 첫 줄>`이 있는 단계는 일부러 틀린 단계이고, `maago gen`이 정확히 그 오류를 내야 한다.
  `Maago-Run: maago add <앱>`이 있는 단계는 패치를 적용하기 전에 그 명령을 돌린다. 끝에서 `main`의 `-createsuperuser`를 한 번 돌린다
- 고치기: `./steps.sh unpack DIR`로 풀면 단계마다 커밋이 있는 git 저장소가 된다(`Maago-Run:`의 명령도 그 자리에서 돈다).
  `git rebase`로 고치고 `./steps.sh pack DIR`로 다시 패치를 뽑는다. 풀어 둔 곳에는 `.env`와 이 저장소를 가리키는 `go.work`,
  그리고 이 저장소의 maago를 빌드한 `DIR/.git/maago`가 있다
- 패치에는 생성물(`*_gen.go`), `maago add`가 쓴 앱(`auth/`), `.env`, DB 파일이 없다. 명령이 만드는 것은 그 명령이 다시 만든다:
  복사된 앱을 고쳐도 패치는 낡지 않는다. 그래서 단계는 `auth/`를 손으로 고치지 않는다. 공개할 저장소를 만들 때는 커밋마다
  `maago gen`을 돌리고 `Maago-Run:`의 명령을 돌려 그것도 커밋한다(D-67, D-103)
- 앱의 시험은 `maagotest.DB(t)`로 시험마다 새 DB를 얻는다(D-113): `board/`에서 돌아도 루트의 `.env`를 찾고, `maago migrate`의 `board.db`는 건드리지 않는다. 그냥 `go test ./...`
