# 게시판 튜토리얼

Maago로 게시판을 만드는 긴 튜토리얼([MILESTONES.md](../../docs/MILESTONES.md)의 M8). 커밋 하나에 기능 하나를 더한다.
`steps/`의 패치가 그 커밋들이고(D-103), 커밋 메시지가 튜토리얼 글과 짝이다.

- 걷기: `cmd/maago`의 `TestBoardTutorial`이 패치를 하나씩 적용하며 `maago gen`·`check`·`migrate`와 앱의 `go test`를 돌린다.
  커밋 메시지에 `Maago-Expect: <오류 첫 줄>`이 있는 단계는 일부러 틀린 단계이고, `maago gen`이 정확히 그 오류를 내야 한다
- 고치기: `./steps.sh unpack DIR`로 풀면 단계마다 커밋이 있는 git 저장소가 된다. `git rebase -i`로 고치고
  `./steps.sh pack DIR`로 다시 패치를 뽑는다. 풀어 둔 곳(`.env`와 이 저장소를 가리키는 `go.work`가 있다)에서 maago는
  이 저장소 루트에서 `go build -C cmd/maago -o "$PWD/DIR/.git/maago" .`로 만들어 부른다
- 패치에는 생성물(`*_gen.go`), `.env`, DB 파일이 없다. 공개할 저장소를 만들 때는 커밋마다 `maago gen`을 돌려 생성물도 커밋한다(D-67)
- `go test`는 `board/`에서 돌아 루트의 `.env`를 보지 못한다: `MAAGO_DATABASE_URL=sqlite:$PWD/board.db go test ./...`
