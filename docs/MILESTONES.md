# 마일스톤

[DECISIONS.md](DECISIONS.md)의 단계 열을 만드는 순서로 풀었다. 번호는 순서이자 의존 관계다.
앞 마일스톤이 뒤 마일스톤의 도구가 된다. 괄호 속 번호는 해당 결정이다.

**v0 목표**: 모델 정의 → `makemigrations` → `migrate` → admin CRUD + 로그인까지 5분 (D-69).
v0의 DB는 SQLite와 PostgreSQL이다 (D-30).

---

## v0

### M0. 뼈대

모든 검증이 거쳐 갈 길을 먼저 깐다. 이후 마일스톤은 여기에 검사를 더하기만 한다.

- `go.mod` (`github.com/maago-web/maago`), 단일 바이너리 `maago`와 `flag` 디스패치 (D-03, D-68)
- `maago gen` / `maago check`의 틀: 오류가 있으면 아무 파일도 쓰지 않음, 독립 오류 전부 출력,
  `파일:줄:칸: 메시지` + 고치는 법 (D-60~D-63)
- reflect 금지 게이트와 CI (D-66)

완료 조건
- [x] `reflect`를 import한 임시 파일을 넣으면 CI가 실패한다
- [x] 가짜 오류 두 개를 내는 검사기를 붙이면 두 오류가 한 번에 나오고, 생성물은 하나도 바뀌지 않는다
      (테스트로 확인. CLI 연결은 M1, [TASKS-M0.md](TASKS-M0.md) 끝)

### M1. 모델

- struct + `maago:"…"` 태그를 `go/ast`로 읽는다. 닫힌 목록, 모르는 키는 “did you mean” (D-10, D-17)
- v0 태그 부분집합: 기본 필드 타입, `max`·`unique`·`index`·`default`·`choices`·`label`·`help`,
  `fk=모델`, 모델 태그의 `table`·`ordering`·`unique=(a,b)`·`index=(a,b)` (D-12~D-14)
- 지원하지 않는 필드는 이유와 함께 생성 파일에 주석 (D-18)
- 모델 → 목표 스키마(방언별 DDL)

완료 조건
- [x] `maxlen=200`은 `file:line:col`과 `did you mean "max"`로 실패한다
- [x] 예제 모델 셋(FK, 복합 unique 포함)의 DDL이 SQLite와 PG 골든 파일과 같다 (D-67)
      ([TASKS-M1.md](TASKS-M1.md) 끝)

### M2. 연습용 DB와 마이그레이션

SQL 검증(M3)이 스키마를 필요로 하므로 마이그레이션이 먼저다. 작업 목록은 [TASKS-M2.md](TASKS-M2.md)(진행 중).

- 연습용 DB: SQLite는 메모리, PG는 개발 DB의 임시 스키마. 연결 실패는 켜는 법과 함께 실패 (D-35, D-64)
- 스키마 읽기 → 모델과 비교 → DDL 초안. 외부 라이브러리 없이 (D-33, D-34)
- `makemigrations` / `showmigrations` / `migrate`, 번호 붙은 SQL 파일 (D-32, D-38)
- 이름 변경은 ⚠ 초안 + `--rename`, `DROP`은 `-- maago: allow-drop` 없으면 실패 (D-36, D-37)
- SQLite 테이블 재생성

완료 조건
- [ ] 빈 프로젝트에서 모델 추가 → 필드 추가 → 필드 이름 변경 → 필드 삭제를 차례로 했을 때,
      모든 마이그레이션을 적용한 결과가 모델과 같다 (SQLite, PG 모두)
- [ ] 초안을 손으로 고쳐 모델과 달라지게 만들면 `check`가 실패한다
- [ ] `allow-drop` 없는 `DROP`은 `check`가 실패한다

### M3. SQL 매퍼

- 파일 머리(이름·결과·파라미터), `:이름` 바인딩과 슬라이스 펼치기 (SQL 규칙 1·2)
- `{조건}` 줄, `WHERE { }`·`SET { }` 정리 → Go 코드로 번역 (규칙 3·4, D-22)
- 커버 조합을 연습용 DB에 prepare, 결과 타입 검사와 새 타입 생성 (규칙 5, D-23)
- PK 기준 기본 CRUD 생성 (D-21)

완료 조건
- [ ] DECISIONS의 `SearchOrders` 예제가 생성되고, 조합별로 기대한 SQL이 나온다(골든)
- [ ] 빈 WHERE의 DELETE, 라벨이 보장하지 않는 파라미터, 컬럼이 빠진 모델 결과 타입이 각각 생성 오류다
- [ ] 생성 코드에 `text/template`도 `reflect`도 없다

### M4. HTTP

- 라우트 표 → `ServeMux` 등록과 URL 함수 (D-40)
- 타입 있는 뷰 시그니처와 생성된 어댑터, 표준 핸들러 탈출구 (D-41)
- html/template + 생성 시점 검사: 필드, `{{url}}` 이름·인자 (D-42)
- 태그에서 생성된 폼, (값, 있음, 오류) 구분 (D-43)
- API 라우트와 RFC 9457 오류, `{{block}}` 부분 응답, CSRF (D-44, D-45, D-47)

완료 조건
- [ ] 템플릿의 `{{.Post.Titel}}`과 없는 URL 이름이 생성 오류다
- [ ] 잘못된 path 값과 `sql.ErrNoRows`는 404, 교차 출처 POST는 403
- [ ] 빈 숫자 칸, 범위 밖 숫자, 해석 실패가 폼에서 서로 다른 결과를 낸다

### M5. 인증 (계정 · 이메일)

- 가입·로그인·로그아웃·세션·비밀번호 변경/재설정, 이메일 인증 (D-50)
- User는 contrib 앱, 코어는 인터페이스만 (D-51)
- **선행 결정 필요**: contrib 앱의 모델이 사용자 프로젝트의 스키마·마이그레이션에 들어오는 방식
  (DECISIONS U-06, 지금은 “나중에”로 미뤄져 있다. M5 전에 정해야 한다)

완료 조건
- [ ] 가입 → 인증 메일 링크 → 로그인 → 비밀번호 재설정이 통합 테스트로 돈다
- [ ] 인증 테이블이 사용자 프로젝트의 `makemigrations` 결과에 들어간다

### M6. Admin

- 모델에서 생성된 목록·생성·수정·삭제, 페이지네이션, 목록 필터(생성된 Go가 조립) (D-48)
- 표시·숨김·필수·읽기 전용은 태그에서만, 이름으로 추측하지 않음 (D-49)
- 로그인한 사용자만 접근 (M5)

완료 조건
- [ ] 필드 이름이 `password`여도 태그가 없으면 숨겨지지 않는다(추측 금지 확인)
- [ ] 로그인하지 않으면 admin에 들어갈 수 없다

### M7. v0 완료 게이트

- [ ] 빈 디렉터리에서 모델 정의 → `makemigrations` → `migrate` → admin CRUD + 로그인까지 5분 (D-69)
- [ ] 보통 규모 예제에서 `maago gen`이 SQLite 1초, PG 3초 이내 (D-65)
- [ ] 에이전트에게 위 흐름을 맡겨, 오류 메시지만 보고 스스로 고쳐 끝내는지 확인한다

---

## v1

- MySQL 방언: DDL, 스키마 읽기, 생성 SQL (D-30)
- 소셜 로그인, MFA (D-50)
- Django 필드 타입·옵션·Meta 전체 대응 (D-16, [TODO.md](TODO.md))
- 템플릿 오버라이드: `embed.FS` 내장 + 사용자 FS 우선 (D-42)
- `maago new` 스캐폴드 (D-73)

## 언젠가

- 마이그레이션 롤백 (D-39)
- 템플릿 컴파일(T3) ([TODO.md](TODO.md))
- M2M admin 위젯, 다중 DB·테넌시 (U-04, U-05)
