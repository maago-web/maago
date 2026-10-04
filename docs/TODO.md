# TODO

## 첫 릴리스 전

- [ ] 게시판 튜토리얼(M8)도 `replace` 대신 태그한 버전을 쓴다.
- [ ] `cmd/maago/go.mod`의 `replace`를 뺀다. `replace`가 있으면 `go install …/cmd/maago@버전`이 거부한다.
      루트 모듈을 먼저 태그하고 그 버전을 `require`한다.

## M2

- [ ] `dialect.Normalize`는 `"title"`과 `title`을 다르게 본다. SQLite에서는 같은 이름이라, 손으로 따옴표 없이 쓴 CHECK가 모델의 것과 다르다고 나올 수 있다(M2 3번). 조용히 통과하지는 않는다

- [ ] PG 연습용 DB의 `search_path`에 `public`이 없어, `public`에 설치된 확장(`citext`, `uuid-ossp` 등)의 타입·함수를 이름만으로 쓰는 마이그레이션이 연습용 DB에서 실패한다(M2 2번). 필요해지면 확장이 설치된 스키마만 뒤에 붙이는 법을 정한다

- [ ] 서로 가리키는 두 모델을 한 번에 지우면 `DropTable` 순서로는 PG 초안이 실패한다(FK 고리, M2 6번). 9번이 쓰기 전 다시 적용해 잡는다. 7번이 한 문장 `DROP TABLE a, b`로 풀지 정한다

- [ ] 모델의 테이블 이름이 `maago_`로 시작하면 거부한다. 읽기가 `maago_migrations`를 건너뛰고 초안이 `maago_new_*`를 임시 이름으로 쓴다(M2 8번)

- [ ] 테이블(모델) 이름 변경의 `--rename`. v0은 컬럼만이고, 테이블 이름 변경은 삭제+추가로 보여 allow-drop이 막는다(M2 9번)

- [x] SQLite의 FK는 연결마다 `PRAGMA foreign_keys = ON`이 있어야 강제된다. 연습용·실제 DB 연결은 켠다(M2 10번). 런타임 연결은 `maago.OpenDB`가 드라이버 주소의 `_pragma`로 연결마다 켠다(M3 6번, `TestOpenDBSQLiteConnections`).

## 모델 태그

- [ ] 태그 값 안에 `"`를 쓸 수 없다(백쿼트 태그 + 백슬래시 금지의 결과, M1 3번). label·help에 큰따옴표가 필요해지면 방법을 정한다.
- [ ] Django 필드 타입(~30)·필드 옵션(~20)·Meta 옵션(~20) 전체를 Maago 태그 원칙으로 옮긴 대응표.
      항목마다 갈래(태그 / Go 타입 / Go 메서드 / 제외), 태그 표기, 세 DB(SQLite·MySQL·PostgreSQL)에서의 결과.
  - 이미 정한 원칙: `maago:"…"` 필수, 옵션은 `,`, 문자열은 `'…'`(안의 `'`는 `''`), 묶음은 `(a,b)`,
    선택지는 `a/b/c`, 관계는 ID 필드 + `fk=모델`, 모델 옵션은 임베드한 `maago.Model`의 태그,
    라벨·도움말은 `label='…'`·`help='…'`
  - 제외 후보: 다중 테이블 상속, proxy 모델, GenericForeignKey, `db_tablespace`, `swappable`

## M3

- [x] 앱의 `go test`는 패키지 디렉터리에서 돌아 루트의 `.env`를 보지 못한다(`maago.OpenDB`는 작업 디렉터리의 `.env`, D-101). 게시판 튜토리얼은 절대 경로 환경 변수로 돌린다. 서비스 시험 방법(U-07, M4)의 입력 → M4 10번: `maagotest.DB(t)`가 루트의 `.env`를 찾는다(D-113)
- [ ] 게시판 튜토리얼 모듈은 `github.com/maago-web/maago`를 `v0.0.0-…`로 require하고 걷기가 `go.work`로 이 저장소를 가리킨다. 릴리스 전에 태그한 판으로(첫 릴리스 전 항목과 함께)
- [ ] 모델 결과 타입 오류의 fix가 예로 드는 이름(`":many PostSummary"`)이 그 앱에 이미 다른 모양으로 있을 수 있다(게시판 튜토리얼 6단계). 앱에 없는 이름을 예로 들지
- [ ] 빌드 제약(`//go:build ignore` 등)이 붙은 `.go` 파일: 매퍼의 `declared`(이름 충돌)와 `PackageName`이 보통 파일처럼 읽는다. 도구 파일이 다른 package 절을 가지면 그것을 쓸 수 있다. `go/build`의 제약 판정으로 거를지(M3 9번 fable 6)

## 언젠가

- [ ] 마이그레이션 롤백(down 마이그레이션)
- [ ] 템플릿 컴파일(T3): 같은 html/template 파일을 Go 코드로 번역해 런타임 reflect까지 0으로.
      이스케이프는 stdlib 결과를 재사용한다. 필요해질 때만.
