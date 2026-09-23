# TODO

## 첫 릴리스 전

- [ ] `cmd/maago/go.mod`의 `replace`를 뺀다. `replace`가 있으면 `go install …/cmd/maago@버전`이 거부한다.
      루트 모듈을 먼저 태그하고 그 버전을 `require`한다.

## 모델 태그

- [ ] Django 필드 타입(~30)·필드 옵션(~20)·Meta 옵션(~20) 전체를 Maago 태그 원칙으로 옮긴 대응표.
      항목마다 갈래(태그 / Go 타입 / Go 메서드 / 제외), 태그 표기, 세 DB(SQLite·MySQL·PostgreSQL)에서의 결과.
  - 이미 정한 원칙: `maago:"…"` 필수, 옵션은 `,`, 문자열은 `'…'`(안의 `'`는 `''`), 묶음은 `(a,b)`,
    선택지는 `a/b/c`, 관계는 ID 필드 + `fk=모델`, 모델 옵션은 임베드한 `maago.Model`의 태그,
    라벨·도움말은 `label='…'`·`help='…'`
  - 제외 후보: 다중 테이블 상속, proxy 모델, GenericForeignKey, `db_tablespace`, `swappable`

## 언젠가

- [ ] 마이그레이션 롤백(down 마이그레이션)
- [ ] 템플릿 컴파일(T3): 같은 html/template 파일을 Go 코드로 번역해 런타임 reflect까지 0으로.
      이스케이프는 stdlib 결과를 재사용한다. 필요해질 때만.
