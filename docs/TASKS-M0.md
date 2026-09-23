# M0 작업 목록 — 뼈대

[MILESTONES.md](MILESTONES.md)의 M0를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M0 완료 조건** (MILESTONES)
- `reflect`를 import한 임시 파일을 넣으면 CI가 실패한다
- 가짜 오류 두 개를 내는 검사기를 붙이면 두 오류가 한 번에 나오고, 생성물은 하나도 바뀌지 않는다

---

## 1. `docs/ARCHITECTURE.md` 작성

코드를 쓰기 전에 프레임워크 내부의 모양을 정한다. 담을 것:

- 패키지 구성: CLI, 진단(오류), 파이프라인, 그리고 M1~M6의 모델·마이그레이션·매퍼·HTTP·인증·admin이 들어갈 자리
- `maago gen` 파이프라인: 읽기 → 검사 → (연습용 DB) → 생성 → 쓰기. 단계마다 무엇을 입력받고 무엇을 내놓는지
- 생성 코드의 위치와 이름 규칙(`*_gen.go`), 사람이 쓰는 파일과 섞이지 않는 방법
- 오류(진단)의 모양: 위치, 메시지, 고치는 법. 연쇄 오류를 어떻게 빼는지
- reflect 금지 게이트가 어디를 검사하는지 (D-66)

→ 확인: M1~M6의 각 구성 요소가 어느 패키지에 들어가는지 문서만 보고 답할 수 있다. DECISIONS와 부딪히는 곳이 없다

## 2. 모듈 둘

- 루트 `go.mod`: `github.com/maago-web/maago`, `go 1.25`, 외부 의존 없음 (D-03, D-04, D-09)
- `cmd/maago/go.mod`: CLI 모듈. 루트 모듈을 `replace`로 가리킨다. 드라이버는 M2에서 더한다

→ 확인: 두 모듈에서 각각 `go build ./...`가 통과하고, 루트 `go.mod`에 `require`가 없다

**계획**
- 새 파일: `go.mod`, `doc.go`(`package maago` 패키지 주석), `cmd/maago/go.mod`(모듈 경로 `github.com/maago-web/maago/cmd/maago`), `cmd/maago/main.go`
- CLI 모듈 경로가 루트 아래여야 `cmd/maago`가 루트의 `internal/`을 import할 수 있다
- 루트에 `.go` 파일이 없으면 `go build ./...`가 "matched no packages" 경고만 내고 통과해 확인이 비게 된다. `doc.go`로 패키지를 하나 둔다
- `cmd/maago/main.go`는 루트 패키지를 blank import 하고 빈 `main`. import가 없으면 `go mod tidy`가 `require`를 지운다. 임시이고, 5번에서 실제 코드로 바뀐다
- `.gitignore`에 `/cmd/maago/maago`: `go build ./...`가 그 자리에 바이너리를 떨군다
- 루트의 `./...`는 `cmd/maago`를 포함하지 않는다. 모든 명령은 두 모듈에서 따로 돈다(4번 CI도)
- 하지 않는 것: `go.work`(작업이 `replace`를 정했다), CI(4번). `go.sum` 확인(외부 의존이 없어 생기지 않는다)
- 미룬 것: `replace`가 있는 모듈은 `go install …/cmd/maago@버전`이 거부한다. 첫 릴리스 전에 루트를 태그하고 `replace`를 뺀다 → TODO
- 리뷰: codex(모듈 경로, `go test`, tidy의 go 버전), fable(`git diff`는 새 파일을 못 봄 → `tidy -diff`, D-04 확인, 경고 대신 `go list`, replace 고정) 모두 반영. 구현 리뷰: codex 없음, fable 두 가지 — 4번 확인을 경로 비교로 바꿈, Go 1.25로 실제 빌드하는 확인은 4번 CI에서

→ 확인 (두 모듈 각각)
1. `go build ./... && go vet ./... && go test ./...` 통과
2. `grep -x 'go 1.25' go.mod` 통과, `grep toolchain go.mod` 비어 있음 (D-04)
3. 루트: `go list ./...`가 정확히 `github.com/maago-web/maago`. `grep require go.mod` 비어 있음
4. `cmd/maago`: `go mod tidy -diff` 통과. `go list -m -f '{{.Replace.Dir}}' github.com/maago-web/maago`가 `git rev-parse --show-toplevel`과 같음
5. 임시로 루트에 `internal/x` 패키지를 두고 `cmd/maago`에서 import해 빌드되는지 본 뒤 지운다
6. `gofmt -l .`가 비어 있음, `go build` 뒤 `git status`에 바이너리가 없음

## 3. reflect 금지 게이트 (D-66)

- 런타임 코드(`_test.go` 제외)와 생성기 템플릿에 `reflect` import가 있으면 실패하는 테스트

→ 확인: `reflect`를 import한 임시 파일을 넣으면 `go test ./...`가 파일 경로와 함께 실패하고, 지우면 통과한다

**계획**
- 새 파일: 루트 `noreflect_test.go`(package `maago`) 하나. 테스트 파일이라 게이트 자신은 대상이 아니다
- 저장소 루트(= 루트 패키지 디렉터리. `go test`는 패키지 디렉터리에서 돈다)부터 `filepath.WalkDir`로 훑는다. `cmd/maago`는 별도 모듈이라 루트의 `go test ./...`에 안 잡히지만, 파일 트리를 걸으면 잡힌다(ARCHITECTURE §7)
- `.go`(`_test.go` 제외): `go/parser`의 `ImportsOnly`로 import 경로만 본다. 별칭·blank·dot import 모두 경로로 잡힌다. 해석 실패와 걷기 오류는 건너뛰지 않고 실패로 낸다(PHILOSOPHY 7)
- 생성기 템플릿: 확장자를 `.tmpl`로 정하고 ARCHITECTURE §7에 적는다. Go로 해석할 수 없는 글이라 `"reflect"` 문자열이 있는 줄을 실패로 낸다. 주석 속 `"reflect"`도 걸리는데, 그 편이 단순하다
- 건너뛰는 디렉터리: `.git`만. `testdata/`도 본다. 그래서 두 규칙을 ARCHITECTURE §7에 적는다: 생성 코드의 골든 파일은 `.go` 확장자를 유지한다(검사 대상이 되도록), 일부러 깨진 Go 입력 픽스처는 `.go`가 아닌 확장자를 쓴다
- 실패 메시지: `경로:줄:칸: imports "reflect" (D-66)` + 고치는 법 한 줄. 모든 위반을 모아 한 번에 낸다(D-61)
- 하지 않는 것: `unsafe` 등 다른 금지 목록(요청 없음), 사용자 프로젝트의 생성 코드 검사(생성기가 생긴 뒤 골든 테스트가 맡는다), symlink 처리(저장소에 없다. `WalkDir`는 디렉터리 symlink를 따라가지 않는다), vendor 정책(외부 의존이 없어 생기지 않는다)
- 리뷰: codex(걷기 오류, 템플릿 규칙을 문서에, 줄 번호) 반영. cwd·symlink·vendor는 위 이유로 받지 않음. fable(루트 패키지의 임시 파일은 컴파일러가 먼저 실패시켜 게이트를 시험하지 못한다 → 임시 파일은 `testdata/`와 `cmd/maago/`에, 실패 문구에 `D-66` 요구, `-count=1`, 골든·픽스처 확장자 규칙) 모두 반영. 구현 리뷰: codex·fable 모두 `.tmpl` 오류에 칸이 없음 → 추가. fable의 "파일 시스템 오류에도 고치는 법 줄" 제안은 받지 않음(검사 대상 코드의 진단이 아니라 게이트 자신의 오류)

→ 확인 (모두 루트에서 `go test -count=1 ./...`, 실패는 출력에 `D-66`이 있어야 한다)
1. 그대로 통과
2. `testdata/x.go`에 `import _ "reflect"` → 경로·줄과 함께 실패. 지우면 통과
3. `cmd/maago/x.go`에 별칭 import `r "reflect"` → 실패
4. `testdata/x_test.go`의 `reflect` import → 통과
5. `testdata/x.tmpl` 속 `"reflect"` → 실패
6. `testdata/x.go`가 해석되지 않는 글 → 실패(D-66 게이트 이름과 함께, 건너뛰지 않음)

## 4. CI

- `gofmt -l`, `go vet ./...`, `go test ./...`

→ 확인: 같은 명령을 로컬에서 돌려 통과한다. 3번의 임시 파일을 넣은 브랜치에서는 CI가 실패한다. CI는 두 모듈을 모두 돈다

**계획**
- 새 파일: `.github/workflows/ci.yml` 하나. 모든 브랜치의 push에서 돈다(임시 브랜치로 실패를 확인하기 위해). pull_request는 같은 저장소 브랜치면 push와 겹쳐 빼고, 외부 기여가 생기면 더한다
- Go는 `1.25.x` 하나, `GOTOOLCHAIN=local`: 최소 버전(D-04)으로 실제 빌드한다. `auto`면 더 새 툴체인을 몰래 받아 확인이 빈다. 행렬은 두지 않는다(요청 없음)
- 단계: `out=$(gofmt -l .)` 후 `test -z "$out"`(루트에서 한 번, `cmd/maago`까지 훑는다. `gofmt -l`은 목록을 내도 0으로 끝나므로 `test -z`로 실패시키고, 해석 실패는 `bash -e`에서 대입이 실패해 멈춘다) → 모듈마다(`.`, `cmd/maago`) `go vet ./...`, `go test ./...`
- reflect 게이트는 루트 모듈의 테스트다. 루트에서 `go test ./...`가 돌아야 한다(3번 리뷰)
- 액션: `actions/checkout@v7`, `actions/setup-go@v7`(2026-09 최신 릴리스 확인). `cache: false`: 두 모듈 모두 `go.sum`이 없다
- 하지 않는 것: 린터, 스크립트 파일(명령이 짧아 워크플로에 직접), 커밋 SHA 고정(공식 actions 조직의 메이저 태그로 충분)
- push·원격 브랜치 삭제는 사람의 승인을 받는다(WORKFLOW)
- 리뷰: codex(gofmt 종료 코드, fail-fast, GOTOOLCHAIN, 액션 버전 확인) 반영, SHA 고정·vendor는 받지 않음. fable(push 승인, 위반은 커밋 하나에 하나씩, 두 번째 모듈은 gofmt가 아니라 그 모듈만 잡는 실패로, `test -z`, GOTOOLCHAIN, cache, 트리거 중복·stable 행렬) 모두 반영. 구현 리뷰: codex·fable 모두 "해석 안 되는 `.go`가 gofmt 단계를 통과한다"(파이프·명령 치환이 종료 코드를 버림) → 고침, codex의 `permissions: contents: read` 반영. fable: 3번 확인 6은 import 앞뒤의 깨짐만 잡는다(`ImportsOnly`). 나머지는 이제 gofmt 단계가 잡는다. `go.mod`의 `toolchain` 줄 검사는 받지 않음(가치 낮음)

→ 확인
1. 워크플로의 명령을 로컬에서 그대로 돌려 통과. `gofmt` 단계는 안 된 파일을 넣으면 로컬에서도 0이 아닌 코드로 끝남
2. (승인 후) `main` push → CI 통과, 로그의 `go version`이 1.25
3. (승인 후) 임시 브랜치 `ci-check`에 위반을 커밋 하나씩 push. 각각 CI 실패:
   a. `testdata/x.go`에 `import _ "reflect"` → 로그에 `D-66`과 경로
   b. a를 되돌리고 `cmd/maago/x_test.go`에 실패하는 테스트 → `cmd/maago` 단계에서 실패
   c. b를 되돌리고 gofmt 안 된 파일 → gofmt 단계에서 실패
4. (승인 후) 임시 브랜치 삭제

## 5. CLI 틀 (D-68)

- 단일 바이너리 `cmd/maago`(별도 모듈), `flag` 디스패치
- M0의 명령은 `gen`과 `check` 둘뿐. 나머지(`makemigrations` 등)는 해당 마일스톤에서 붙인다
- 모르는 명령은 사용법과 함께 실패

→ 확인: `maago nope`이 사용법을 출력하고 0이 아닌 코드로 끝난다

## 6. 진단(오류) 형식 (D-61, D-62, D-63, D-06)

- 진단 하나 = 위치(`파일:줄:칸`) + 메시지 + 고치는 법. 고치는 법이 없는 진단은 만들 수 없게 한다
- 영어로 쓴다. 경고 수준은 없다
- 출력은 파일·줄 순으로 정렬하고, 같은 진단은 한 번만

→ 확인: 진단 몇 개를 넣은 출력이 골든 파일과 같다

## 7. 파이프라인과 원자적 쓰기 (D-60, D-61)

- 단계(읽기 → 검사 → 생성) 안의 독립 오류는 전부 모은다. 오류가 난 단계에서 멈춰 다음 단계의 연쇄 오류를 막는다
- 생성물은 메모리에 먼저 만들고, 오류가 하나도 없을 때만 디스크에 쓴다
- 검사기를 붙이는 자리를 만든다. M1부터는 여기에 검사기를 더하기만 한다

→ 확인: 테스트용 가짜 검사기가 오류 두 개를 내면, 두 오류가 한 번에 나오고 생성물 디렉터리는 한 바이트도 바뀌지 않는다

## 8. `maago check`의 최신성 검사 (D-60)

- 생성해야 할 결과와 디스크의 생성물을 비교한다. 내용이 다른 파일, 새로 생겨야 할 파일, 사라져야 할 파일을 모두 잡는다
- 아무것도 쓰지 않는다

→ 확인: 생성물을 손으로 고치거나, 하나 지우거나, 남는 `*_gen.go`를 하나 두면 각각 `check`가 파일 이름과 함께 실패한다

## 9. 골든 파일 테스트 도구 (D-67)

- 기대 출력 파일과 비교하고, `-update` 플래그로 갱신한다
- 6·7·8번의 테스트가 이것을 쓴다

→ 확인: 기대 파일을 한 글자 바꾸면 테스트가 차이를 보여주며 실패한다

---

## M0 끝

- [ ] 1~9 완료
- [ ] 위 M0 완료 조건 두 개를 실제로 돌려 확인
- [ ] MILESTONES.md의 M0 체크박스 갱신
