# M0 작업 목록 — 뼈대

[MILESTONES.md](MILESTONES.md)의 M0를 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.
순서 예외: 9번(골든 파일 도구)은 5번 다음, 6번 앞에 한다. 6·7·8번이 그것을 쓰기 때문이다(2026-09-24 결정).

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

**계획**
- 바꿀 파일: `cmd/maago/main.go`. 새 파일: `cmd/maago/main_test.go`
- `main`은 `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))` 한 줄. 판단은 모두 `run`에 두어 테스트가 프로세스를 띄우지 않고 부른다. 명령은 `os.Exit`를 부르지 않고 코드를 돌려준다
- 명령 표: 이름과 `func(stdout, stderr io.Writer) int`의 슬라이스(순서가 정해져 사용법 문구가 흔들리지 않는다). 새 명령(M2의 `makemigrations` 등)은 한 줄 더하는 일이 된다
- 명령마다 `flag.FlagSet`(`ContinueOnError`, 출력은 주입된 stderr, 기본 `Usage`는 끈다). M0에는 플래그가 없지만 모르는 플래그를 거부하는 자리다
- 종료 코드: 0 성공·도움말, 1 검사 실패, 2 사용법 오류. 완료 때 DECISIONS의 D-68에 적는다
  - `maago`(인자 없음)·모르는 명령·모르는 플래그·남는 위치 인자(`maago gen extra`) → 사용법을 stderr에, 2. 남는 인자를 조용히 버리지 않는다(PHILOSOPHY 6·7)
  - `maago help`·`-h`·`--help`·`maago gen -h`(`flag.ErrHelp`) → 사용법을 stdout에, 0. 명령별 도움말은 없고 같은 사용법을 낸다
- `gen`·`check`의 몸통은 7·8번에서 채운다. 그 전까지는 stderr에 `maago gen: not implemented yet (M0 task 7)` 한 줄과 함께 1. 아무것도 하지 않고 0으로 끝나면 "검사가 빠졌는데 통과"가 된다(PHILOSOPHY 7, D-64의 정신). 진단 형식(6번)이 아직 없어 평문이다
- 문구는 영어(D-06)
- 루트 패키지 blank import는 그대로 둔다. 7번에서 파이프라인 import로 바뀐다
- 하지 않는 것: 서브커맨드 라이브러리(D-68), 명령별 도움말, 버전 명령(요청 없음)
- 리뷰: codex(`ErrHelp`와 출력 방향, 종료 코드 대응, 위치 인자, 최상위 `-h`), fable(stub 문구에 힌트, 종료 코드는 DECISIONS로, 위치 인자, `SetOutput`·기본 Usage, 반대쪽 출력이 비었는지 확인, 명령 표 순서, `gen -h`) 모두 반영. 구현 리뷰: codex(출력 중복을 못 잡음 → `usage:` 횟수 검사, `gen --`·빈 인자 추가), fable(돌연변이 19개 중 17개를 잡음. 남은 둘: 쓰지 않는 `-help` 별칭 → 지움, 명령 순서 → 사용법 전체를 비교). `help gen`·`-h extra`의 남는 말을 거부하자는 지적(둘 다)은 받지 않음: 도움말 요청은 해가 없고, 도움말을 보여주는 것이 맞는 답이다

→ 확인 (`cmd/maago`에서 `go test -count=1 ./...`. 모든 경우에 코드와 두 출력을 함께 본다: 한쪽에 나오면 다른 쪽은 비어 있어야 한다)
1. `maago nope`, `maago` → stderr에 사용법(`gen`, `check` 포함), stdout 비어 있음, 2
2. `maago help`, `-h`, `--help`, `gen -h` → stdout에 사용법, stderr 비어 있음, 0
3. `maago gen -x` → stderr에 `-x`가 들어간 오류와 사용법, 2
4. `maago gen extra` → stderr에 `extra`와 사용법, 2
5. `maago gen`, `maago check` → stderr에 `not implemented`, 1
6. 빌드한 바이너리로 `maago nope; echo $?`가 2
## 6. 진단(오류) 형식 (D-61, D-62, D-63, D-06)

- 진단 하나 = 위치(`파일:줄:칸`) + 메시지 + 고치는 법. 고치는 법이 없는 진단은 만들 수 없게 한다
- 영어로 쓴다. 경고 수준은 없다
- 출력은 파일·줄 순으로 정렬하고, 같은 진단은 한 번만

→ 확인: 진단 몇 개를 넣은 출력이 골든 파일과 같다

**계획**
- 새 파일: `internal/diag/diag.go`, `internal/diag/diag_test.go`, `internal/diag/testdata/write.golden`. 바꿀 파일: ARCHITECTURE §4(공개 필드 → 비공개 필드와 `New`)
- 위치는 stdlib `go/token.Position`을 그대로 쓴다. 새 위치 타입을 만들지 않는다. 경로는 부르는 쪽이 프로젝트 기준 상대 경로로 준다. `Write`는 경로를 고치지 않는다
- `Diagnostic`의 필드는 비공개. 만드는 길은 `diag.New(pos, msg, fix)` 하나. `msg`나 `fix`가 비었거나 줄바꿈이 있으면 패닉한다. 고치는 법 없는 진단은 생성기의 버그이므로 프로그램 오류로 다룬다(D-62). 줄바꿈 금지는 출력 모양이 깨지지 않게 하려는 것이고, 필요해지면 그때 푼다
- `New`는 `pos.Offset`을 0으로 만든다. 출력되지 않는 값이 중복 제거를 막지 않게 하려는 것이다
- 수준 필드는 없다(D-63). 문구는 영어(D-06)
- 출력 `diag.Write(w, ds) error`: 부르는 쪽 슬라이스를 복제한 뒤 파일·줄·칸·메시지·고치는 법 순으로 정렬하고(모든 필드를 비교해야 불안정 정렬에서도 결과가 같다), 같은 진단은 한 번만(D-61). 모양:
  ```
  blog/models.go:12:18: unknown tag key "maxlen" in Post.Title
    fix: did you mean "max"? ...
  ```
  - 파일 이름이 없으면 위치 없는 진단(DB 연결 실패 등): `메시지` + `fix:` 줄. 빈 파일 이름이 가장 앞에 정렬된다(ARCHITECTURE §4). `token.Position.String()`은 이때 `-`를 내므로 쓰지 않는다
  - 파일만 있으면 `파일: 메시지`, 줄까지면 `파일:줄: 메시지`, 칸까지면 `파일:줄:칸: 메시지`
- 정렬·중복 제거는 `slices.SortFunc`·`slices.Compact`, 비교는 `cmp.Or`/`cmp.Compare`
- 하지 않는 것: `error` 인터페이스 구현, JSON(D-62), 색, 경로 정규화, 파이프라인 연결(7번)
- 리뷰: codex(`Offset` 무시, 모든 필드 비교, 줄바꿈, 경로 기준, 위치 없음 정의), fable(정렬 키와 중복 제거 불일치, `-` 출력, 골든은 손으로 쓴다, ARCHITECTURE §4, 슬라이스 복제) 모두 반영. 구현 리뷰: codex(`\r` 거부, 출력되지 않는 위치 부분(파일 없는 줄, 줄 없는 칸)을 `New`에서 지움 → 중복 제거) 반영, 쓰기 오류 테스트는 받지 않음(`io.WriteString` 결과를 그대로 돌려줄 뿐). fable(돌연변이 20개 중 16개를 잡음 → 같은 줄·다른 칸, 같은 위치·다른 메시지(메시지와 fix 순서가 반대) 입력 추가, §4에 `Write`, 영값 문구 완화) 반영

→ 확인 (`go test -count=1 ./internal/diag`)
1. 손으로 쓴 `testdata/write.golden`과 `Write` 출력이 같다. 입력은 순서가 뒤섞였고: 완전한 중복, `Offset`만 다른 중복, 같은 위치·다른 메시지, 위치 없음·파일만·줄까지·칸까지를 모두 담는다
2. 부른 뒤에도 입력 슬라이스의 순서가 그대로다
3. `msg`·`fix`가 비었거나 줄바꿈이 있는 `New`는 각각 패닉
4. 루트에서 `go test -count=1 ./...` 통과(reflect 게이트 포함)

## 7. 파이프라인과 원자적 쓰기 (D-60, D-61)

- 단계(읽기 → 검사 → 생성) 안의 독립 오류는 전부 모은다. 오류가 난 단계에서 멈춰 다음 단계의 연쇄 오류를 막는다
- 생성물은 메모리에 먼저 만들고, 오류가 하나도 없을 때만 디스크에 쓴다
- 검사기를 붙이는 자리를 만든다. M1부터는 여기에 검사기를 더하기만 한다

→ 확인: 테스트용 가짜 검사기가 오류 두 개를 내면, 두 오류가 한 번에 나오고 생성물 디렉터리는 한 바이트도 바뀌지 않는다

**계획**
- 새 파일: `internal/pipeline/pipeline.go`, `internal/pipeline/pipeline_test.go`. 바꿀 파일: ARCHITECTURE §3(이름 바꾸기 실패 처리)
- 타입
  ```go
  type Run struct {
      Root  string            // 프로젝트 루트
      Files map[string][]byte // 생성물. 키는 Root 기준 슬래시 경로
  }
  type Step func(*Run) []diag.Diagnostic
  ```
  검사기와 생성기는 모두 `Step`이다. 생성기는 `Run.Files`를 채우는 `Step`일 뿐이다. 단계는 `[]Step`, 파이프라인은 `[][]Step`(단계 이름은 읽는 곳이 없어 두지 않는다)
- `Generate(r *Run, stages [][]Step) []diag.Diagnostic`: 단계를 차례로 돈다. 한 단계의 모든 `Step`을 돌려 진단을 모으고(D-61), 진단이 하나라도 있으면 다음 단계로 가지 않고 돌려준다(연쇄 오류 차단)
- `Gen(root string, stages [][]Step) []diag.Diagnostic`: `Generate` 후 진단이 없을 때만 `Write`(D-60). CLI는 이것만 부른다. 이 순서가 CLI가 아니라 여기 있어야 테스트가 잡는다
- `Write(root string, files map[string][]byte) []diag.Diagnostic`:
  1. 모든 키를 먼저 검사한다: `filepath.FromSlash` 후 `filepath.IsLocal`이 아니면 진단, 아무것도 쓰지 않는다. symlink는 따로 막지 않는다(프로젝트 안의 symlink는 사용자의 선택)
  2. 키를 정렬해 차례로: 디스크와 바이트가 같으면 건너뛴다. 아니면 부모 디렉터리를 `MkdirAll`(0755), 같은 디렉터리에 임시 파일을 쓰고 모드를 0644로
  3. 2에서 하나라도 실패하면 만든 임시 파일을 모두 지우고 진단. 기존 파일은 바뀌지 않는다(새로 만든 빈 디렉터리는 남을 수 있다)
  4. 모두 성공하면 하나씩 `os.Rename`(유닉스·윈도 모두 기존 파일을 덮는다). 실패하면 남은 임시 파일을 지우고, 실패한 파일과 함께 `fix: rerun maago gen` 진단. 같은 파일은 건너뛰므로 다시 돌리면 나머지만 쓴다. 이미 바뀐 파일 목록은 따로 만들지 않는다 → ARCHITECTURE §3을 이렇게 고친다
  - I/O 진단은 위치에 해당 파일, 메시지는 OS 오류, 고치는 법은 경우별 한 줄(권한·경로 확인, 다시 돌리기)
- CLI: `gen`은 stub 그대로 둔다. 단계가 하나도 없는 `gen`이 0으로 끝나면 5번이 막은 "검사가 빠졌는데 통과"가 된다. M1이 첫 단계를 만들 때 `Gen`에 연결한다. 단계 목록을 어디서 조립할지(ARCHITECTURE §2는 CLI가 디스패치만 한다고 한다), `Run`에 무엇을 더할지는 그때 데이터를 보고 정한다
- 하지 않는 것: 병렬 실행, 진행 표시, 이름 바꾸기 실패 재현 테스트(이식성 있게 일으킬 방법이 없고, 처리가 "다시 돌리기" 한 줄이다)
- 리뷰: codex(모드 0644, `MkdirAll`, 이름 바꾸기 실패 뒤 임시 파일 정리, 키 정렬, 부모가 일반 파일인 실패 테스트, `FromSlash`+`IsLocal` 선검사, symlink 정책 명시) 반영. codex는 단계 0개로 `gen`을 연결해도 된다고 봤고 fable은 5번의 원칙과 부딪힌다고 봤다 → 5번에 적은 PHILOSOPHY 7 판단을 따라 연결하지 않는다. fable(조립 위치·`Run` 확장은 M1로, `Gen`으로 순서를 테스트 가능하게, 두 단계 통과 경우, 정렬로 5번 확인을 결정적으로, `Stage.Name` 삭제, 이름 바꾸기 실패는 다시 돌리기로 단순화) 반영. 구현 중: 실패하는 단계에 생성기가 없으면 "무조건 `Write`"를 잡지 못함 → 실패하는 단계에 생성기를 넣음. 돌연변이 7개 모두 잡힘. 구현 리뷰: codex(없음 외의 읽기 오류는 진단, 줄바꿈 있는 경로 거부 → `diag.New` 패닉 방지) 반영, 대소문자·`a`와 `a/b` 충돌 검사는 받지 않음(키는 maago 자신의 생성기가 만들고, 충돌해도 진단으로 끝나 데이터를 잃지 않는다). fable(오류 하나인 단계 경우, 디렉터리 모드 0755 확인) 반영. fable이 8번에 넘긴 것: 어떤 디스크 파일을 생성물로 볼지 한 곳에서 정하고 `gen`도 사라져야 할 파일을 지우게 할지

→ 확인 (`go test -count=1 ./internal/pipeline`, 모두 `t.TempDir()`에서)
1. 같은 단계의 가짜 검사기 둘이 오류를 하나씩 → `Gen`이 두 오류를 한 번에 돌려준다. 다음 단계(파일을 만드는 가짜 생성기)는 돌지 않는다
2. 1의 경우, 기존 파일이 든 디렉터리의 모든 파일 이름과 바이트가 전과 같다
3. 두 단계 모두 오류가 없으면 둘째 단계도 돌고, `Gen`이 새 파일을 쓰고(모드 0644, 없던 하위 디렉터리 포함) 바뀐 파일을 고친다. 같은 파일은 수정 시각이 그대로
4. `../x` 같은 경로가 하나라도 있으면 진단, 디렉터리는 그대로
5. `a.go`(쓸 수 있음)와 `b/c.go`(`b`가 일반 파일이라 실패) → 진단, `a.go`는 전 내용 그대로, 임시 파일이 남지 않는다
6. 루트에서 `go test -count=1 ./...` 통과(reflect 게이트 포함)

## 8. `maago check`의 최신성 검사 (D-60)

- 생성해야 할 결과와 디스크의 생성물을 비교한다. 내용이 다른 파일, 새로 생겨야 할 파일, 사라져야 할 파일을 모두 잡는다
- 아무것도 쓰지 않는다

→ 확인: 생성물을 손으로 고치거나, 하나 지우거나, 남는 `*_gen.go`를 하나 두면 각각 `check`가 파일 이름과 함께 실패한다

## 9. 골든 파일 테스트 도구 (D-67)

- 기대 출력 파일과 비교하고, `-update` 플래그로 갱신한다
- 6·7·8번의 테스트가 이것을 쓴다

→ 확인: 기대 파일을 한 글자 바꾸면 테스트가 차이를 보여주며 실패한다

**계획**
- 새 파일: `internal/golden/golden.go`, `internal/golden/golden_test.go` (ARCHITECTURE §2의 자리)
- API 하나: `golden.Check(t testing.TB, path string, got []byte)`. `path`는 테스트 패키지 디렉터리 기준(`go test`가 거기서 돈다. 보통 `testdata/…`)
- 실패는 `t.Errorf` 후 `return`으로만 낸다(`Fatalf` 없음). 부르는 쪽 테스트는 계속 돌고, 가짜 TB로도 실패를 관찰할 수 있다
- `-update` 플래그는 이 패키지가 등록한다. 이 패키지를 import하지 않는 테스트 바이너리는 `-update`를 모르므로 `go test ./... -update`는 안 된다. 갱신은 패키지를 지정해서 한다: `go test ./internal/diag -update`. 켜면 디렉터리(0755)를 만들고 파일(0644)을 쓴 뒤 통과한다. CI 명령에는 `-update`가 없다
- 비교는 바이트 그대로. 다르면 처음 달라진 줄 번호와 그 줄의 기대·실제 값(`%q`로 공백이 보이게, 한쪽이 끝났으면 `end of file`)을 보여주고 실패. stdlib에 diff가 없고, 첫 차이만으로 고칠 곳을 찾는다
- 기대 파일이 없으면 `-update`로 만들라는 안내와 함께 실패(조용히 만들지 않는다)
- 런타임 코드가 아니지만 reflect 게이트의 대상이다(ARCHITECTURE §7). 비교는 `bytes`·`strings`로 한다
- 자기 테스트: `testing.TB`를 임베드하고 `Errorf`·`Helper`만 바꾼 가짜로 실패를 관찰한다. 갱신 모드는 CLI 플래그가 아니라 패키지 변수를 테스트 안에서 켰다 끄고(`t.TempDir()`에서), 자기 테스트는 `-update`에 영향받지 않는다
- 하지 않는 것: 전체 diff 출력, 줄 수 표시, 줄바꿈 정규화(CRLF), 여러 파일 한 번에 비교, 원자적 쓰기(테스트 도구의 기대 파일이라 필요 없다)
- 리뷰: codex(`./... -update` 불가, 경로 기준, 가짜 TB, 파일 모드) 반영, CI의 `-update` 차단·원자적 쓰기는 받지 않음(CI 명령에 없고, 기대 파일은 git이 되돌린다). fable(`./...` 주장 오류, 자기 테스트와 플래그 분리, `Errorf`만, 끝 줄바꿈 하나 차이 경우 추가, 문구) 모두 반영. 구현 중: 끝 줄바꿈이 하나 더 있을 때 줄 번호가 하나 밀림 → `SplitAfter`로 줄바꿈을 줄에 붙여 고침(빠진 끝 줄바꿈도 보인다). `-update`로 돌리면 자기 테스트가 쓰기 모드가 됨 → 테스트 안에서 비교 모드를 고정. 구현 리뷰: codex(빈 파일 경우 추가, 기존 파일의 모드 유지는 받지 않음: 모드는 git이 관리한다), fable(돌연변이 13개 중 10개를 잡음 → want·got 표시가 바뀌어도 통과, got이 짧을 때 경우 없음 → 둘 다 테스트 추가)

→ 확인 (`go test -count=1 ./internal/golden`)
1. 같은 내용 → 통과. 한 글자 다른 기대 파일 → 실패, 메시지에 줄 번호와 두 값
2. 실제 값 끝에 줄바꿈이 하나 더 → 실패, `end of file`이 보이고 패닉 없음
3. 기대 파일이 없으면 `-update` 안내와 함께 실패
4. 갱신 모드면 없는 디렉터리까지 만들어 쓰고 통과, 끄고 다시 비교하면 통과
5. 루트에서 `go test -count=1 ./...` 통과(reflect 게이트 포함)

---

## M0 끝

- [ ] 1~9 완료
- [ ] 위 M0 완료 조건 두 개를 실제로 돌려 확인
- [ ] MILESTONES.md의 M0 체크박스 갱신
