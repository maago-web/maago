# M7 작업 목록 — 정적 파일과 업로드

[MILESTONES.md](MILESTONES.md)의 M7을 작업 단위로 쪼갠 것이다. 위에서부터 순서대로 한다.
각 작업은 `→ 확인:`이 통과해야 끝난다. 작업마다 [구현 루프](WORKFLOW.md)를 한 번 돈다.

**M7 완료 조건** (MILESTONES)
- 게시판 튜토리얼(M8)에서 이 마일스톤이 받는 커밋이 모두 통과한다 → 7번
- 템플릿의 `{{static "없는.css"}}`가 생성 오류이고, 있는 파일은 해시가 붙은 이름으로 나온다 → 2번(검사), 1번(이름)
- 파일 내용이 바뀌면 이름이 바뀌고, 해시 이름의 응답은 오래 캐시되는 헤더를 가진다 → 1번
- Node 없는 환경에서 템플릿에 Tailwind 클래스를 쓰면 그 클래스가 든 CSS가 나온다 → 6번(standalone만 부르고 `node`를 부르지 않는다: CI의 러너에는 Node가 있어 "없는 환경"은 그렇게 본다)
- 크기 제한을 넘는 업로드는 413, `../`가 든 파일 이름은 미디어 폴더 밖에 쓰이지 않는다 → 3번(413), 5번(열쇠)
- 권한이 없는 사용자는 비공개 미디어를 받을 수 없다 → 7번(권한은 사용자의 뷰, D-140), 5번(`ServeMedia`의 헤더)

**앞에서 넘어온 것**
- 정적 파일 URL의 접두어(MILESTONES M4 할일, D-122) → D-137, 1·2번
- 끝이 `/`인 라우트 경로는 생성 오류이고 "M7의 정적 파일이 하위 트리를 필요로 하면 표기를 더한다"(TASKS-M4) → 더하지 않는다: `/static/`은 `Handler`가 등록한다(D-137)
- 폼의 파일 칸과 multipart(TASKS-M4, `internal/web/forms.go`의 "a file field comes with M7") → D-139, 4번
- M8 표: 글 첨부 파일(원본의 `fastapi-storages`), 정적 파일, 서버 렌더링 + Tailwind → 7번

---

## 먼저 확인한 사실 (2026-10-13, 리뷰가 코드로 다시 봄)

- **템플릿은 앱마다 `templates/*.html`**(하위 디렉터리는 생성 오류, `internal/web/templates.go`), 생성된 `views_gen.go`가 HTML 라우트가 있는 앱에서만 `//go:embed templates/*.html`로 싣고 `maago.Pages`에 함수 표 `url`·`date`를 준다(`internal/web/templates/views.go.tmpl`, `gen.go`). `maago.Pages`는 패키지 변수에서 불려 틀리면 기동 때 panic. 생성 때 함수 이름은 닫힌 목록(`funcNames`), 호출은 `walk.go`의 `call`이 본다: `url`은 글자 그대로의 이름만 받고 라우트 표에서 찾아 "did you mean"을 준다. `{{static}}`은 같은 자리에, 앱마다의 `static/` 파일 목록을 따로 모으는 단계와 함께
- **`//go:embed`는 `.`·`_`로 시작하는 이름을 뺀다**(`all:`이 아니면). 생성 검사가 디스크를 읽으므로 같은 규칙이어야 `check`를 지난 파일이 런타임에 없을 수 없다(D-136)
- **생성물은 `.go`뿐이다.** 파이프라인은 첫 줄 표시가 있는 `.go`만 자기 것으로 본다(`internal/pipeline`의 `Owned`는 `GoFiles`). D-136(해시는 런타임)과 D-138(Tailwind 출력은 사람의 파일)로 넓히지 않는다
- **생성물의 import 허용 목록**(D-66): `web.Imports`는 `embed`·`html/template`·`net/http`·`net/url`·`strconv`(D-66 본문에는 `net/url`이 빠졌다: 고친다). 해시·multipart는 루트에 두어 목록이 그대로다
- **multipart는 지금 400이다**(`FormValues`가 urlencoded만, `form_test.go`). 413은 저장소 어디에도 없다. 본문 상한은 JSON의 `MaxBytesReader(nil, r.Body, 10MB)`(`form.go:334`, 넘으면 400)와 `ParseForm`의 암묵 10MB(400). `MaxBytesReader`는 응답을 쓰지 않고 `*http.MaxBytesError`를 돌려줄 뿐이고 `status()`는 모르는 오류를 500으로 한다(`http.go`) → 413은 명시적으로
- **`ParseMultipartForm`은 메모리 한도를 넘는 부분을 임시 파일에 두고, 지우는 것은 부르는 쪽**(`r.MultipartForm.RemoveAll`)
- **폼의 `Field[T]`는 `string`·`int64`·`int32`·`float64`·`bool`·`time.Time`**(`internal/web/forms.go`의 `formTypes`). 바인딩은 `url.Values`에서 읽는다
- **정적 바이트를 내보내는 선례가 없다.** admin의 CSS는 `embed`로 읽어 `<style>`에 넣는다(라우트도 캐시 헤더도 없다. MILESTONES M6: admin은 M7에 기대지 않는다). `http.ServeContent`는 ETag를 만들지 않고 미리 둔 것으로 304·Range를 한다. `nosniff`는 스스로 붙이지 않는다
- **탈출구 뷰**(D-41, `func(w, r)`)는 Auth·`LoginRequired`·교차 출처 검사를 지난다. 게시판 0016의 CSV가 쓴다. path 값 해석과 틀린 id의 404는 뷰의 몫
- **모델에 파일 타입은 없다.** `[]byte` 칸은 있다(admin은 고치지 않는다)
- **CI는 네트워크를 쓴다**(`setup-go`, `cache: false`인 모듈 받기). Tailwind 실행 파일의 출처(GitHub releases) 하나가 늘 뿐이고, 재현성은 버전·체크섬 고정이 지킨다(D-138)
- **루트 모듈은 `require`가 없다**(D-90). brotli는 루트에 둘 수 없다(D-09)
- **튜토리얼 걷기는 `Maago-Run: maago add APP`만 안다**(`cmd/maago/tutorial_test.go`, `steps.sh`). Tailwind 출력은 패치에 든 보통 파일이라(D-138) 걷기에 네트워크가 필요 없다
- **게시판 패치(0001~0024)에 정적 파일·CSS·첨부가 없다.** M8 표의 `Attachment` 모델도 아직 없다. 글 지우기(0022)는 댓글 없는 글만, FK는 `NO ACTION`(D-84)
- **D-72**: 이미지 처리는 범위 밖(썸네일·크기 바꾸기 없음)

## 결정 (2026-10-13, 초안 리뷰 뒤 사람의 결정)

초안의 Q1~Q8을 codex·fable 리뷰 뒤 정했다. 자세한 것은 DECISIONS.

- **D-136 정적 파일의 자리**: 앱마다 `static/`(하위 디렉터리 없음, `.`·`_` 이름은 오류), embed, 해시는 기동 때 `maago.Static`, 부른 이름이 없으면 기동 panic
- **D-137 URL과 템플릿**(사람의 결정: 해시 없는 이름도 서빙, 늘 `앱/이름`, 압축 없음): `/static/<앱>/<이름>`을 `Handler`가, `/static/` 아래 사용자 라우트는 늘 생성 오류, 해시 이름은 1년 `immutable`·해시 없는 이름은 `no-cache`, ETag와 `ServeContent`
- **D-138 Tailwind**(사람의 결정: 새 외부 의존, 출력은 사람의 파일): `maago tailwind`가 고정 버전·체크섬의 standalone을 받아 `<앱>/tailwind.css` → `<앱>/static/tailwind.css`, `gen`·`check`는 오프라인이고 낡음을 보지 않는다
- **D-139 업로드 칸**: `Field[File]`, 파일 칸 있는 폼은 multipart만(urlencoded면 400), 본문 상한 하나에 모든 본문 413, `accept=png/jpeg/pdf`는 닫힌 종류를 내용으로
- **D-140 미디어**: `MediaDir`은 필수(파일 칸이 있으면), 무작위 열쇠, ctx로 받는 `SaveMedia`·`OpenMedia`·`DeleteMedia`, 공개 라우트 없음, `ServeMedia`는 늘 attachment·`nosniff`

## 모양

```
 사용자 프로젝트                                     이 저장소
 board/static/app.css            사람                 루트: Static(embed.FS, 이름…) 해시·ETag, Handler가 /static/<앱>/,
 board/tailwind.css → board/static/tailwind.css            StaticFunc({{static "앱/이름"}}, Prefix), 본문 상한과 413,
   (maago tailwind, 사람이 커밋)                            File·multipart 바인딩, Save/Open/Delete/ServeMedia, Config.MediaDir
 auth/templates/base.html: {{static "board/tailwind.css"}} internal/web: static/ 목록·검사, /static/ 예약, 파일 칸
 board/views.go: Field[maago.File], SaveMedia,        cmd/maago: maago tailwind(받기·확인·캐시·돌리기)
   AttachmentDownload(w, r) → 권한 → ServeMedia
```

## 작업

1. **정적 파일 런타임**(D-136·D-137): `maago.Static`(해시 이름, ETag, 부른 이름 확인의 panic), `Handler`가 앱마다 `/static/<앱>/` 등록, 해시 이름과 해시 없는 이름의 `Cache-Control`, `ServeContent`, `StaticFunc`과 `Prefix`. → 확인: 내용을 바꾸면 이름이 바뀌고 같은 내용은 같은 이름(완료 조건 3), 두 `Cache-Control`, `If-None-Match` 304, 없는 이름 404, `Prefix` 아래의 URL
2. **`{{static}}` 생성**(D-136·D-137): `static/` 목록(하위 디렉터리·`.`·`_` 이름은 오류), `views_gen.go`의 embed와 `maago.Static` 호출(부른 이름 목록), `walk.go`의 검사(글자 그대로만, 앱·파일이 없으면 did you mean), `/static/` 아래 사용자 라우트의 생성 오류, `static/`은 있는데 HTML 라우트가 없는 앱(계획에서: embed의 자리). → 확인: 진단 표, 골든, 완료 조건 2
3. **본문 상한과 413**(D-139): `Config`의 상한 하나, urlencoded·JSON·multipart의 `MaxBytesError` → 413(HTML은 글, API는 RFC 9457). → 확인: 세 본문의 413, 상한 아래는 그대로
4. **파일 칸**(D-139): `Field[File]`, 생성기의 파일 칸(모델 폼 밖의 칸만), 루트의 multipart 해석과 `RemoveAll`, 파일 칸 있는 폼의 urlencoded 400, `accept`의 닫힌 종류(모르는 종류는 did you mean)와 내용으로 확인. → 확인: 틀린 형식은 폼 오류, 임시 파일이 남지 않음, 파일 칸 없는 폼의 multipart 400 그대로, 진단 표
5. **미디어**(D-140): `Config.MediaDir`(파일 칸이 있으면 비면 `Handler` 오류), 무작위 열쇠, `SaveMedia`·`OpenMedia`·`DeleteMedia`(ctx, 열쇠 형식 검사), `ServeMedia`(attachment, `mime.FormatMediaType`, `nosniff`, Range). → 확인: `../`가 든 이름의 파일이 `MediaDir` 아래 열쇠 이름으로만 생김(완료 조건 5), `OpenMedia("../x")` 거절, 따옴표·CR/LF·한글 이름의 `Content-Disposition`, Range 206
6. **`maago tailwind`**(D-138): 고정 버전·플랫폼별 SHA-256, 사용자 캐시(임시 파일 → 확인 → rename), 받기·확인 실패의 오류, `<앱>/tailwind.css`가 있는 앱마다 돌리기, 입력이 보는 파일. → 확인: 템플릿의 클래스가 든 CSS(완료 조건 4, `node`를 부르지 않음), 틀린 체크섬 거절, CI에서 받기
7. **게시판 튜토리얼의 M7 단계**: Tailwind와 `{{static}}`로 화면(복사본 auth의 `base.html`은 손대지 않으니 게시판 화면만인지 계획에서), `Attachment` 모델과 글 폼의 파일 칸(413), 첨부 내려받기 뷰(글을 볼 수 있는 사람만, 숨긴 글의 첨부는 403), 글 지우기가 첨부 행과 파일도(`DeleteMedia`), 일부러 틀리는 단계 `{{static "board/없는.css"}}`. → 확인: `TestBoardTutorial`, `TestBoardTutorialCatches`
8. **완료 조건과 문서**: "M7 끝"에 조건마다 시험 이름, 상한(동적 클래스, 낡은 Tailwind CSS를 `check`가 보지 않음, 고아 파일, 외부 저장소·이미지 처리 없음, API의 파일 없음), D-66의 `net/url`, ARCHITECTURE·MILESTONES·TUTORIAL-FUTURE

---

## M7 끝

- [ ] 1~8 완료
- [ ] 위 M7 완료 조건을 실제로 돌려 확인(PG는 `MAAGO_TEST_PG_URL`로), 조건마다 시험 이름
- [ ] MILESTONES.md의 M7 체크박스 갱신
- [ ] push 뒤 GitHub Actions에서 PG 시험이 돈 것을 확인
- [ ] TASKS-M8 초안

## 초안 리뷰 (2026-10-13)

- codex: 받음 — `{{static}}`은 `funcNames`·`call`만이 아니라 파일 목록 단계(2번), ETag·304·`ServeContent`(D-137), 413은 `MaxBytesError`를 명시적으로(3번), multipart의 임시 파일과 `RemoveAll`(4번), `Content-Disposition`은 `mime.FormatMediaType`와 이상한 이름 시험, `nosniff`는 직접(5번), 받기의 임시 파일·rename(6번), ★gzip은 프록시로 → 사람의 결정: 압축 없음, ★Q4 사람이 직접 받기 → 사람의 결정: `maago tailwind`(fable의 모양). 받지 않음: "JSON에 `MaxBytesReader`가 없다"(`form.go:334`에 있다), 개발은 디스크 서빙(D-136 기각: 두 길), 해시 충돌 검증(SHA-256의 앞부분 길이는 1번 계획에서)
- fable: 받음 — `accept`의 `/`는 목록 구분자 → 닫힌 종류(D-139), ★`check`가 캐시에 있을 때만 보는 조건부 검사 → 출력을 사람의 파일로(D-138, 파이프라인 작업 지움), "처음 쓸 때"의 숨은 잠금 → 패키지 변수에서 한 번과 기동 panic(D-136), `MediaDir` 기본값 없음(D-140), `/static/` 예약은 늘(D-137), D-66의 `net/url`(8번), "CI는 네트워크가 없다"는 틀림(고침), 걷기의 트레일러(사실에), embed의 `.`·`_` 규칙(D-136), 조건 4의 "Node 없는 환경"과 조건 6의 대응(머리), 조건 5의 시험 모양(5번), ★해시 없는 이름 404 → 사람의 결정: `no-cache`로 서빙, ★다른 앱 참조 → 사람의 결정: 늘 `앱/이름`, 칸별 상한 대신 하나와 urlencoded도 413(D-139), 파일 칸 있는 폼의 urlencoded 400, 늘 attachment, 지우기가 첨부도(7번), 순서(Tailwind를 업로드 뒤로, 문서 상한은 끝에 합침)

---

## 1. 정적 파일 런타임 (D-136, D-137)

### 계획

이 작업의 끝에 루트에 정적 파일의 런타임이 있다: 생성될 코드가 부를 `maago.Static`과 `App.WithStatic`, `Handler`가 서빙하는 `/static/<앱>/<이름>`, 템플릿 함수 `StaticFunc`. 생성(embed·검사·`views_gen.go`)은 2번이고, 이 작업의 시험은 `fstest.MapFS`로 손으로 만든 앱을 쓴다.

API(루트, 새 파일 `static.go`)
- `func Static(app string, fsys fs.FS, names ...string) *StaticFiles`: `fsys`의 `static/` 파일을 모두 읽어 SHA-256을 낸다(`ParsePages`가 `templates/`를 읽듯, 2번은 `//go:embed static` 디렉터리 꼴로 낸다: `static/*`는 `.keep`을 싣는다). 해시 이름은 마지막 확장자 앞에 앞 16자(`app.css` → `app.1a2b3c4d5e6f7a8b.css`, 확장자 없으면 끝에 `.해시`). `names`(템플릿이 부른 이름, 생성기가 준다) 중 없는 것이 있거나 디렉터리가 있으면 panic(`maago.Pages`처럼 패키지 변수에서 불려 기동 때 실패, D-136). `app`은 패키지 이름(`board`): `NewApp`의 이름은 가져오기 경로라 URL에 쓰지 않는다(admin의 URL과 같음)
- `func (a App) WithStatic(s *StaticFiles) App`: 생성된 `App`이 단다. `NewApp`의 모양은 그대로(admin·생성물이 바뀌지 않음)
- `Handler`: `Apps`의 정적 파일마다 `GET /static/<앱>/{name}`을 mux에 직접(라우트의 `serve`를 지나지 않는다: Auth·DB가 없다. 교차 출처 검사는 GET이라 지나감). 같은 패키지 이름의 정적 파일 둘은 `Handler` 오류(생성 검사는 2번). 이름의 표(앱 → 파일)를 `process`에 `Prefix`와 함께(D-122, 마지막 `Handler`가 이김)
- 응답: 해시 이름은 `Cache-Control: public, max-age=31536000, immutable`, 해시 없는 이름은 `no-cache`. 둘 다 `ETag: "<SHA-256 전체 16진>"`과 `http.ServeContent`(형식은 해시 없는 이름의 확장자, Range·`If-None-Match` 304·HEAD). 어느 이름도 아니면 404(`http.NotFound`)
- `func StaticFunc(ref string) (string, error)`: `{{static "board/app.css"}}` → `Prefix() + "/static/board/app.1a2b….css"`. 표에 없는 앱·파일은 오류(생성기가 막으니 여기 오면 버그, `Render`가 500)

결정에 없는 선택
- 해시 길이 16자(64비트). 이름과 해시 이름을 한 표에 넣고 키가 겹치면 panic(사람이 만든 `a.<해시>.css`). URL이 그대로 두지 않는 이름(`url.PathEscape(n) != n`)과 디렉터리도 panic이고 2번이 생성 오류로 먼저 막는다: 런타임은 escape하지 않는다
- `static/`은 있는데 라우트가 없는 패키지는 생성 오류(2번): `App`이 없어 `WithStatic`을 달 자리가 없고, 내도 `Config.Apps`에서 빠지면 조용히 404. `templates/`가 페이지 없는 앱에서 오류인 것과 같은 닫힌 쪽
- `WithStatic`은 `NewApp`의 이름(가져오기 경로)과 `Static`의 이름(패키지 이름)이 맞는지 보지 않는다: 둘 다 생성물이 같은 앱에서 쓴다
- 정적 라우트는 `serve` 밖이라 recover·`OnError`를 지나지 않는다: 메모리의 바이트를 `ServeContent`로 줄 뿐이다. `Handler`가 앞의 CORS는 그대로 붙인다
- mux의 패턴 충돌(예: `GET /{a}/board/{b}`)은 `Handler`에서 panic: 2번이 정적 패턴을 생성기의 `conflicts()`에 넣어 생성 오류로. `Handler`는 복구하지 않는다(한 자리)
- 여러 `Handler`: D-122의 한계 그대로(마지막 `Handler`의 표)
- `WithStatic` 메서드(새 생성자 대신): `NewApp`을 부르는 곳을 건드리지 않는다
- 정적 라우트는 Auth를 돌리지 않는다: 파일마다 세션을 읽지 않는다(공개 파일, D-140의 미디어와 다르다)

하지 않기로 한 것: 압축(D-137), 하위 디렉터리(D-136), 생성(2번), `Last-Modified`(embed는 시각이 없다, ETag로 충분)

단계
1. `static.go`와 `process`의 표, `Handler` 등록 → 확인: `TestStatic`(내용을 바꾸면 이름이 바뀜·같은 내용은 같은 이름, 두 `Cache-Control`, `If-None-Match` 304, Range 206, HEAD, 없는 이름·다른 앱 404, 부른 이름이 없으면 panic, 해시 이름 충돌 panic), `TestStaticFunc`(`Pages` → `Handler` → 그린 URL, `Handler` 전은 오류), `TestStaticPanics`, `TestHandlerConfig`에 같은 패키지 이름 둘
2. 두 모듈 확인 명령, 변형(캐시 헤더 바꾸기, ETag 빼기)으로 시험이 잡는지

### 계획 리뷰

- codex: 패턴 충돌의 panic → 2번이 `conflicts()`에(적음), `serve` 밖의 recover·`OnError` → 적음, `WithStatic`의 이름 불일치 → 생성물이 한 자리라 검사하지 않음(적음), `fs.Sub` → `static/`을 읽음, 이름 규칙(점 여럿·점으로 시작·확장자 없음)과 충돌 → 한 표·시험, `PathEscape` → 런타임은 escape하지 않고 막음, `Content-Type`·304 본문 없음·HEAD·Range → 시험. 받지 않음: "파일마다 같은 패턴을 등록"(앱마다 하나다), 정적 preflight 시험(CSS·JS는 교차 출처 요청을 하지 않는다, `ServeHTTP`가 그대로 붙인다)
- fable: 정적 패턴과 사용자 패턴의 충돌은 2번의 `conflicts()`, embed의 `.`·`_`는 디렉터리 꼴에서만(스크래치로 확인) → `static/`을 읽음, `Pages`의 nil 실행이 `{{static}}`을 `Handler` 전에 부름 → 오류이고 panic 아님, 시험 `TestStaticFunc`. ★`static/`만 있는 패키지 → 생성 오류(닫힌 쪽, `templates/`의 선례라 사람에게 묻지 않음, 2번), HTML 라우트 없는 앱의 embed 자리·`names`는 모든 걷기 뒤 정렬(2번에), `Handler` 오류에 두 가져오기 경로, 다른 앱 404 대신 틀린 해시 404, 304는 해시 없는 이름에서·두 이름의 ETag가 같음, 해시 이름의 `Content-Type`, 빈 `{name}`, `StaticFunc`의 모르는 앱과 파일 → 모두 시험

### 구현 리뷰

- ponytail(나): 잘라낼 것 없음(`static.go` 120줄, stdlib만). 변형으로 확인(6개 중 5개 잡힘): 캐시 헤더, ETag 빼기, `Prefix` 빼기, 해시 길이, `StaticFunc`의 해시 이름 받기. 남은 하나(`ServeContent`에 해시 이름)는 같은 동작이다: 해시 이름은 확장자를 지키고, 확장자 없는 이름은 어느 쪽이든 내용을 본다. Go 1.25에서도 돎
- codex: 결함 없음(샌드박스가 스크래치 디렉터리를 막아 시험은 돌리지 못함, 코드만 읽은 리뷰)
- fable: 결함 없음. 변형 15개 중 살아남은 둘 → 시험: 해시 이름을 `names`로 주면 panic, `POST`는 405. `TestStaticFunc`가 앞 시험의 `current`를 물려받아 "Handler 전"을 지나지 않음 → 머리에서 비움. 디렉터리의 오류 글, 충돌 글은 정렬 순서에 따라 같은 이름 둘을 적음 → 두 파일을 적음. 문서의 `PathEscape`·시험 이름 → 고침. 2번에 넘길 것: 파일 없는 `static/`(`//go:embed static`이 컴파일 오류)은 생성 오류, `names`가 다른 앱의 템플릿 때문에 바뀔 때 진단이 그 템플릿을 말함, `{{static "board/…"}}`의 앱은 패키지 이름, 이름 규칙의 진단을 런타임 글과 맞춤

