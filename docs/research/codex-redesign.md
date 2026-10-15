## 1. 판정

결론부터 말하면, Maago는 “작동하는 데까지 가는 것”은 실제로 쉽지만, “안전한 서비스를 쓰게 하는 것”에는 아직 실패한다.

### Easy: 상당히 좋다

두 에이전트가 소스 코드를 보지 않고 `go doc`과 튜토리얼만으로 게시판 12단계를 약 8분에 끝냈고, 계획에 없던 Maago 오류도 각각 12개 중 11개, 10개 중 9개를 오류 메시지만으로 고쳤다. 짧은 튜토리얼에서는 두 에이전트 모두 관리자 CRUD까지 132~139초에 도달했다. (`evidence.md A.7–A.9`, `docs/TASKS-M9.md:99–103`)

특히 다음 설계는 검증됐다.

- 생성 시점에 실제 SQLite/PostgreSQL에서 SQL을 `prepare`한다.
- `gen`이 독립적인 오류를 한 번에 보여준다.
- 오류가 파일·줄·칸과 `fix:`를 포함한다.
- 템플릿 내부의 필드 오타, URL 이름 오타, Go 타입 오류까지 잡는다.
- 생성물은 오류가 있으면 쓰지 않는다. (`docs/ARCHITECTURE.md:97–120`, `evidence.md A.14`)

즉, 에이전트가 “무엇을 배워야 하는가”를 이미 알고 있다면 생산성이 높다.

그러나 현재 에이전트는 중요한 문법을 `go doc`에서 배울 수 없다. 모델 태그, SQL header, `{cond}`, 뷰 입력 구조, `maago:"form"`, `Validate`, `Result`의 사용법, `DBFrom`, `base.html` 계약이 공개 API 문서에 없다. 결국 생성기 오류를 교과서처럼 사용한다. (`evidence.md A.9`, `docs/gate-logs/board-1.md:12–20,32–39,48–58,68–79`)

따라서 현재의 “easy”는 언어 설계의 결과라기보다, 좋은 오류 메시지와 잘 만든 튜토리얼에 의존한 easy다.

### Safe: 핵심 영역에서는 아직 아니다

가장 불편한 사실은 Maago가 가장 안전하게 만들겠다고 약속한 권한 부여를 정적으로 검증하지 못한다는 점이다.

M6~M8의 보안 버그 8개는 정적으로 하나도 잡히지 않았고, draft 공개 버그는 튜토리얼 31단계를 통과한 뒤 리뷰어가 발견했다. 권한 변경은 모두 앱 테스트에서만 발견됐다. (`evidence.md B.1`)

현재 기계가 검증하는 것은 대체로 “이 SQL이 실행 가능한가”, “이 타입과 템플릿이 맞는가”이지, “이 사용자가 이 행을 읽거나 수정해도 되는가”가 아니다.

동시성과 데이터 무결성도 마찬가지다.

- 애플리케이션 코드에 트랜잭션이 없다.
- check-then-act 경쟁 조건이 허용된다.
- 중복 검사 후 삽입하면 경쟁 상황에서 500이 된다.
- 댓글 수를 확인한 뒤 삭제하는 흐름이 경쟁에 취약하다.
- 전체 행 `UpdateX`는 lost update를 만들 수 있다. (`evidence.md B.5`)
- `Field.Error`를 뷰에서 설정해도 `Valid()`는 false가 되지 않는다. 그 결과 중복 INSERT가 실행되어 unique 오류 500이 된다. (`form.go:36–41`, `evidence.md A.10`)

마이그레이션도 위험한 순간이 있다. 이름 변경 경고가 터미널이 아니라 생성된 SQL 파일 안에만 출력된다. 에이전트가 파일을 읽지 않고 그대로 적용하면 데이터 손실 가능성이 있다. (`docs/gate-logs/board-1.md:27–30`, `evidence.md A.14`)

따라서 판정은 다음과 같다.

- Easy: happy path에서는 매우 유망하다.
- Safe: 컴파일·SQL·템플릿 안전성은 좋지만, 권한·트랜잭션·경쟁·데이터 보존 안전성은 부족하다.
- 현재 상태를 “coding agents가 안전하게 real service를 만든다”고 부르면 과장이다.

## 2. 구조적 변경안 — 영향 순위

### 1위: 권한을 뷰 코드의 관례가 아니라 데이터 접근 계약으로 만든다

현재는 `LoginRequired`가 “로그인했는가”만 보장하고, 행 소유권·조직 경계·draft 공개 여부는 일반 Go 코드에 맡긴다. `Config.Auth`도 요청자의 사용자만 context에 넣는다. (`http.go:52–61`, `http.go:98–108`)

예:

```go
var Posts = maago.Resource[Post]{
    Read:  maago.Policy("post.read", PostVisibleToViewer),
    Edit: maago.Policy("post.edit", PostOwnedByViewer),
    Delete: maago.Policy("post.delete", PostOwnedByViewer),
}
```

또는 더 강하게:

```go
posts, err := PostQuery.VisibleTo(ctx, viewer).List(ctx, db)
```

생성기는 다음을 요구해야 한다.

- 보호된 라우트에 정책이 명시되어 있는가
- 목록 쿼리가 정책 범위 없이 전체 행을 반환하지 않는가
- `Get`, `Update`, `Delete`가 정책 조건을 포함하는가
- 정책 테스트가 존재하는가

비용은 크다. 기존 뷰와 SQL의 자유로운 조합을 깨고, `queries/*.sql`의 API와 생성기를 다시 설계해야 한다. 임의 SQL을 완전히 분석해 권한을 증명할 수는 없으므로, 정책 선언 또는 명시적인 `Unsafe` 탈출구가 필요하다. 이것은 완전한 정적 증명이 아니라 “권한 누락을 기본적으로 빌드 실패로 만드는 구조”다. 이 변경이 필요한 이유는 현재 보안 버그가 정적으로 0개 잡혔기 때문이다. (`evidence.md B.1`)

### 2위: 트랜잭션과 원자적 명령을 1급 개념으로 만든다

현재 `DB`는 `*sql.DB`, `*sql.Tx`, `*sql.Conn`을 같은 인터페이스로 받지만, 프레임워크가 요청 단위 transaction을 제공하지 않는다. (`db.go:19–27`, `evidence.md B.5`)

최소한 다음이 필요하다.

```go
err := maago.WithTx(ctx, db, func(ctx context.Context, tx maago.Tx) error {
    // validation, authorization, write, outbox insert
    return nil
})
```

그리고 생성 CRUD에는 다음을 넣어야 한다.

- unique violation을 폼 필드 오류 또는 409로 변환
- 조건부 update: `WHERE id=? AND version=?`
- affected rows가 0이면 충돌 오류
- 삭제 전 검사와 삭제를 하나의 transaction으로 묶기
- 파일 저장과 DB row 생성 사이의 실패 보상

기존의 `UpdateX(ctx, db, *X)` 전체 행 덮어쓰기 계약은 깨진다. 호출자가 버전 필드를 관리하거나, 생성기가 patch/update input을 만들어야 한다. (`docs/DECISIONS.md:86`, `evidence.md B.5`)

### 3위: 공개 문법을 문서가 아니라 기계가 생성하게 한다

현재 약 20개 개념군과 약 90개 규칙을 에이전트가 알아야 하며, root package의 91개 export 중 약 40개는 생성 코드 전용이다. (`evidence.md B.3, B.7`)

`go doc`만으로 다음을 볼 수 있어야 한다.

```text
maago help model-tags
maago help query-files
maago help forms
maago help views
maago explain diagnostic-code
```

또는 `go doc`에 생성된 package-level reference를 넣어야 한다.

비용은 문서 빌드 파이프라인과 예제 유지 비용이다. 하지만 현재 문서 drift가 약 57건이고, 규칙이 코드·결정 문서·튜토리얼에 흩어져 있다. (`evidence.md B.8`) 이 변경 없이는 새 기능마다 같은 문제가 반복된다.

### 4위: `:many Model`을 없애거나 의미를 바꾼다

현재 모델 결과는 모든 컬럼을 선택해야 하므로 새 필드를 추가할 때 기존 쿼리가 반복적으로 깨졌다. (`evidence.md B.2`, `docs/gate-logs/board-1.md:41–46,81–86`)

선택지는 둘 중 하나다.

- `:many Post`를 제거하고 항상 명시적인 `PostSummary`를 요구한다.
- 또는 `:many Post`를 “생성기가 현재 모델의 projection을 자동 생성”하는 특별한 문법으로 바꾼다.

두 번째는 편하지만 SQL의 실제 SELECT가 무엇인지 덜 보이게 만든다. Maago의 “보이는 대로 동작” 원칙과 충돌한다. 안전성 우선이라면 첫 번째가 낫다.

### 5위: 모듈·릴리스 구조를 하나로 합친다

현재 루트와 `cmd/maago`가 별도 모듈이고, `replace`가 있는 두 모듈 구조는 `go install …@v`를 어렵게 하며 두 태그를 다른 커밋에 붙여야 한다. (`docs/ARCHITECTURE.md:84–92`, `evidence.md B.10`, `docs/TASKS-M9.md:51–61`)

CLI를 같은 모듈의 `cmd/maago`에 두고, 개발 시에도 릴리스 시에도 동일한 module graph를 사용해야 한다. 기존 사용자의 `go.mod`, 설치 경로, CI를 깨뜨리는 큰 변경이다. 그래도 v0 전에 하지 않으면 배포 문제가 제품의 첫 경험이 된다.

## 3. 실제 서비스를 만들 때 빠진 것

다음은 “나중에 있으면 좋은 기능”이 아니라 real service의 기본 안전장치다.

- graceful shutdown, server read/write/idle timeout
- background 작업의 수명 관리와 재시도
- transaction helper와 outbox/job 모델
- unique violation의 구조화된 변환
- optimistic locking 또는 동시 수정 검출
- decimal, UUID/ULID, JSON 타입
- production에서 실행 가능한 migration binary 또는 migration package
- object storage와 orphan media 정리
- rate limit, 로그인 시도 제한의 운영 설정
- structured logging, request ID, health/readiness endpoint
- secret/config 검증
- authorization 정책과 행 단위 scope
- API versioning과 generated client/schema
- admin FK 선택 위젯과 모델 수준 validation

근거는 현재 production gap 목록에 직접 정리되어 있다. (`evidence.md B.11`) UUID·JSON은 v1로 밀려 있고, `D-16`도 v1로 표시되어 있다. (`docs/DECISIONS.md:34`) 미디어는 로컬 디스크에만 남고, row가 사라져도 orphan 파일을 청소하지 않는다. (`doc.go:26–35`)

특히 migration을 소스에서 `maago migrate`로 실행해야 한다는 것은 배포 시스템에서 실수하기 쉽다. (`evidence.md B.11`) “단일 바이너리 배포”라는 방향과도 어긋난다.

## 4. 처음부터 다시 쓴다면

### 유지할 것

증거가 강하게 지지하는 베팅은 유지한다.

- Go struct 기반 모델
- SQL 파일을 사람이 작성
- 생성 시점의 실제 dialect DB 검증
- typed view와 typed template
- 명시적인 route table
- `go/ast`, `go/types` 기반 검사
- 생성 파일 커밋
- SQLite와 PostgreSQL을 같은 계약으로 검증
- 오류의 위치와 수정 방법 출력

이것들이 실제 에이전트 gate에서 가장 잘 작동한 부분이다. (`docs/PHILOSOPHY.md:6–19`, `docs/ARCHITECTURE.md:97–120`, `evidence.md A.14`)

### 바꿀 것

개념을 네 층으로 줄인다.

1. Schema: 모델, migration, 제약
2. Query: 읽기와 쓰기 SQL
3. Endpoint: route, request, response
4. Policy: 인증과 행 접근 권한

현재의 `Opt`, `Result`, `Field`, `Rule`, `App`, `Routes`, `Block`, `API`, `LoginRequired`, `DBFrom`, `WithDB`, `Invalid`, `Unprocessable` 등이 각각 따로 학습 대상이 되는 구조는 줄인다. 현재 약 90개 규칙이 필요하다는 사실이 실패 신호다. (`evidence.md B.7`)

예:

```go
type CreatePost struct {
    Title string `maago:"required,max=200"`
}

func CreatePost(ctx context.Context, in CreatePost) (PostPage, error) {
    return posts.Create(ctx, in)
}

var Routes = maago.Routes{
    maago.POST("/posts", CreatePost).
        Require("post.create"),
}
```

폼과 API는 별도 개념이 아니라 같은 request schema의 두 serialization으로 만든다.

```go
type PostInput struct {
    Title string `maago:"required,max=200"`
}
```

### 권한 설계

권한은 핸들러 안의 `if user.ID != post.AuthorID`가 아니라 query/resource contract가 되게 한다.

```go
post, err := Posts.Get(ctx, db, viewer, id)
```

생성기는 이 함수가 정책을 호출하는지, 정책이 query scope를 적용하는지 확인한다. 임의 SQL이나 임의 DB 접근은 다음처럼 명시적인 탈출구로 남긴다.

```go
// maago:unsafe reason="report query joins external tenant table"
```

기계가 완전히 비즈니스 권한을 증명할 수는 없다. 대신 다음을 강제할 수 있다.

- 로그인 필요 여부
- resource별 정책 존재
- policy 없는 민감한 query 금지
- 권한 matrix test 생성
- 익명·다른 사용자·다른 tenant의 negative test 생성
- list/detail/update/delete 모두에 scope가 있는지 검사

현재 draft leak이 31단계까지 통과했다는 사실 때문에, “컴파일되면 안전하다”는 주장을 해서는 안 된다. (`evidence.md B.1`)

### 데이터 무결성

생성기가 mutation을 다음 형태로 만들게 한다.

```go
err := maago.Tx(ctx, db, func(tx maago.Tx) error {
    if err := tx.Posts.Create(...); err != nil {
        return err
    }
    return tx.Outbox.Enqueue(...)
})
```

모델에 `Version`이 있으면 update는 자동으로 optimistic lock을 사용한다. unique 제약은 DB 오류를 내부 오류로 내보내지 않고 필드 오류 또는 409로 변환한다.

### 문서와 reference

문서를 사람이 따로 관리하지 않고 생성한다.

- 각 태그/parser 규칙에서 reference 페이지 생성
- 각 export의 “누가 호출하는가” 표시
- `maago init`이 현재 버전의 최소 예제를 생성
- `maago doctor`가 Go 버전, DB, 드라이버, migration 상태를 진단
- `maago explain`이 진단 코드와 실제 수정 예제를 출력
- CI에서 reference 예제와 tutorial을 실제로 컴파일

현재 root package 문서가 36줄이고 핵심 문법을 다루지 않는 문제가 출발점이다. (`evidence.md B.3`)

### 모듈·릴리스·툴체인

가급적 하나의 Go module로 만든다.

```text
github.com/maago-web/maago
├── runtime/
├── cmd/maago/
├── generate/
├── contrib/
└── examples/
```

사용자는 다음 한 줄로 설치할 수 있어야 한다.

```sh
go install github.com/maago-web/maago/cmd/maago@v0.1.0
```

개발용 `replace`, `go.work`, 두 모듈의 서로 다른 태그는 공개 릴리스 경로에 없어야 한다. 현재 구조가 이 설치 경로를 복잡하게 만든다는 기록이 이미 있다. (`evidence.md B.10`, `docs/TASKS-M9.md:51–61`)

### 일부러 만들지 않을 것

다음은 v0에서도, 당분간도 만들지 않겠다.

- 런타임 reflection 기반 ORM
- 숨은 relation loading과 자동 JOIN
- 복잡한 template 자체 언어
- 모델에서 자동 생성되는 magic authorization
- 모든 SQL을 안전하다고 추정하는 분석기
- 내장 task queue
- 자체 JavaScript 프레임워크
- 자동 migration 실행
- 다중 DB와 multi-tenancy
- 처음부터 MySQL까지 지원
- 모든 Django 기능의 복제

Maago의 강점은 생성 시점·명시적 SQL·작은 런타임에 있다. (`docs/PHILOSOPHY.md:21–50`, `docs/ARCHITECTURE.md:8–20`) 범위를 넓히면 현재의 검증 가능성이 빠르게 약해진다.

## 5. v0.1.0 전과 이후

### v0.1.0 전에 반드시 할 일

1. 공개 설치 경로를 실제 빈 `GOPATH`, 빈 module cache, `GOWORK=off`에서 검증한다.  
   현재 두 모듈과 `replace` 때문에 별도 릴리스 절차가 필요하다. (`docs/TASKS-M9.md:51–61`)

2. 모델 태그·SQL header·뷰·폼 계약을 `go doc`와 공식 reference에 넣는다.  
   현재 에이전트가 중요한 개념을 오류 시행착오로 배운다. (`evidence.md A.9`)

3. 성공 명령은 반드시 결과를 출력한다.

```text
generated 7 files
created superuser me@example.com (id 1)
```

현재 `gen`과 `-createsuperuser`가 성공해도 아무것도 출력하지 않는다. (`docs/gate-logs/short-1.md:46–50`)

4. 내부 D-number를 사용자 오류에서 제거한다.  
   `(D-114)`는 사용자에게 아무 의미가 없다. (`docs/gate-logs/short-1.md:80–90`)

5. migration rename/drop 경고를 stdout/stderr에도 출력하고, 위험한 초안은 기본적으로 적용 불가하게 한다.  
   현재 경고가 파일 내부에만 있다. (`docs/gate-logs/board-1.md:27–30`)

6. form 오류 상태를 고친다.  
   `Field.Error`를 추가한 뒤 `Valid()`가 계속 true인 현재 계약은 500과 데이터 중복을 유발한다. (`form.go:36–41`, `evidence.md A.10`)

7. `choices` 진단을 고친다.  
   `choices=draft|published`를 잘못 쓴 뒤 오류가 그 잘못된 표현을 다시 유효한 표현처럼 보여준다. (`evidence.md A.11`, `docs/gate-logs/board-1.md:14–18`)

8. 최소한의 production 안전장치를 추가한다.

   - graceful shutdown
   - HTTP timeouts
   - transaction helper
   - unique violation 변환
   - migration 실행 방식 개선

   이 항목들은 현재 명시적인 production gap이다. (`evidence.md B.11`)

9. 권한 negative test를 v0 gate에 넣는다.  
   정적 분석만으로 충분하지 않다는 것을 인정하고, 익명·타 사용자·draft·삭제·파일 다운로드의 자동 테스트를 생성하거나 요구해야 한다. 현재 보안 버그 8개 중 0개가 정적으로 잡혔다. (`evidence.md B.1`)

### 이후로 미룰 것

- MySQL
- social login과 MFA
- admin 템플릿 확장
- S3 및 orphan media 청소
- i18n
- 전체 Django 타입 대응
- multi-tenancy
- migration rollback
- 복잡한 admin 위젯

이 기능들은 현재 v1 또는 “언젠가” 목록에도 이미 분류되어 있다. (`docs/MILESTONES.md:219–236`)

다만 UUID/JSON/decimal, transaction, authorization scope는 단순한 편의 기능이 아니다. 이들을 모두 v1로 미루면 v0.1.0은 “데모를 빠르게 만드는 프레임워크”일 수는 있어도, “실서비스를 안전하게 만드는 프레임워크”라고 부르기는 어렵다.

