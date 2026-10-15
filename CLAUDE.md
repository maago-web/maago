# Maago에서 일하기

- 먼저 [README.md](README.md)의 문서 목록을 읽는다. 일하는 방법은 [docs/WORKFLOW.md](docs/WORKFLOW.md)의 루프를 따른다.
- **다음 작업**: [docs/MILESTONES.md](docs/MILESTONES.md)에서 진행 중인 마일스톤의 `TASKS-Mn.md`를 연다. 끝난 작업은 `git log`에 `(Mn N번)`으로 커밋되어 있다. 그다음 번호가 다음 작업이다. 진행 중인 마일스톤의 `TASKS-Mn.md`가 아직 없으면 다음 작업은 그 초안이다(지난 `TASKS`의 끝 절 마지막 항목, [README.md](README.md)에도 적는다).
- 리뷰어 부르는 법
  - codex: `codex:codex-rescue` 서브에이전트. "읽기 전용 리뷰, 파일을 고치지 말 것"이라고 명시한다.
  - fable: `general-purpose` 서브에이전트에 `model: fable`.
  - 두 리뷰어는 한 메시지에서 병렬로 띄운다.
- 확인 명령: 루트에서 `go mod tidy -diff && go vet ./... && go test ./...`(모듈 하나, D-151). `gofmt -l .`은 비어야 한다.
- PG 테스트(M2~): `MAAGO_TEST_PG_URL`이 있어야 돈다(D-91). M1 예제 스키마가 받을 것과 막을 것은 `cmd/maago`의 `TestExample*`(두 방언, M2 11번에서 스모크 스크립트를 옮김).
- 커밋 메시지는 한국어, `종류(범위): 내용 (Mn N번, D-xx)`. 공동 작성자 표시는 붙이지 않는다. push는 묻고 한다.
