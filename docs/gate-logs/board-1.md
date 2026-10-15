# Gate log: board tutorial (gateb1)

Start 11:47. Setup: go mod init / edit / go get modernc.org/sqlite as instructed; PATH=$PWD/bin.
Docs read up front: `go doc` of maago (package overview, Model, DBConfig, OpenDB, Config, App, Opt, Route(s), Result, Field, Form, Rule), maagotest, contrib/auth.

## 0001 Start the project (~1 min)
- main.go: maago.OpenDB(ctx, maago.DBConfig{}) + `_ "modernc.org/sqlite"`. .env, .gitignore (.env, board.db*).
- `maago gen` 0, `maago check` 0, `maago migrate` -> "nothing to apply" (made board.db), `go run .` ok.
- No failures. Source of truth: OpenDB/DBConfig doc (clear).
- Unclear: nothing; main has nothing to serve yet so it just opens and exits.

## 0002 Board, Post, Comment (~2 min)
- GAP (big): the model tag grammar is nowhere in `go doc` (Model doc shows only table=, max=). fk=, unique=(..), choices, default, auto, label syntax all guessed, then learned from gen errors.
- Failures from first `maago gen` (9 in one run, all reported at once - good):
  - `board/models.go:13:38: label in Board.Name is not written as expected` / `fix: write text in quotes, like label='Title'` -> fixed by message.
  - `board/models.go:14:30: auto in Board.CreatedAt is not written as expected` / `fix: write auto=create or auto=update` -> fixed by message (tutorial says "auto 태그" which reads like bare `auto`).
  - `board/models.go:24:62: default "draft" of Post.Status is not one of its choices` / `fix: use one of: draft|published` -> MISLEADING: I had written `choices=draft|published`; it parsed that as ONE choice "draft|published", so the fix text echoes my wrong syntax back as if valid. Should say choices are separated by `/`.
- 2nd try `choices=(draft,published)`: `board/models.go:24:30: choices in Post.Status is not written as expected` / `fix: list them with /, like choices=draft/published` -> fixed by message.
- 3rd: `migrations do not match models: table "board_board" is missing` / `fix: run maago makemigrations` (x3) -> expected; makemigrations + migrate + gen ok.
- Guessed and accepted: `fk=Board`, model tag `unique=(BoardID,Title)`, `default='draft'`, `max=100`. Required-ness: no tag, strings are NOT NULL by default (guess; confirmed by migration SQL).
- Table names are app_model (board_board...) - learned from migration file, tutorial 0004 uses board_comment so consistent.
- Tests: board/models_test.go (Insert defaults status/auto times, unique (BoardID,Title) rejected, Update, Get, Delete -> sql.ErrNoRows). Pass.

## 0003 Post.Views (~1 min)
- Added `Views int64 \`maago:"default=0"\``. gen: `column "views" of "board_post" is missing` / `fix: run maago makemigrations` (expected). makemigrations -> 0002_add_board_post_views.sql (ADD COLUMN ... DEFAULT 0). NewPostInput.Views is maago.Opt[int64]. Test for default and maago.Set[int64](7). Pass.

## 0004 Rename Comment.Body -> Text (~1 min)
- Plain makemigrations: created 0003_board_comment.sql with the ⚠ comments, exit 0, terminal printed only "created migrations/0003_board_comment.sql". NOTE: the warning is only inside the file; an agent that does not open the file would migrate and lose data (the DROP is guarded by allow-drop so migrate may refuse - not tried). Suggest printing ⚠ lines to stderr.
- rm file; `maago makemigrations --rename board_comment.body:text` -> single RENAME COLUMN. migrate/gen/check ok.
- go test build failure (my own test): `unknown field Body in struct literal of type NewCommentInput` -> renamed in test. Added TestCommentText. Pass.

## 0005 Post queries (~2 min)
- GAP: query file grammar is not in `go doc` (only the tutorial prose). Guessed `-- name: PostList :many PostSummary` (sqlc-like), `:board_id` named params (from `:now` in 0012), `{q} AND ...` lines.
- Failure 1: `board/queries/post_list.sql:4:18: parameter board_id is not declared` / `fix: declare it with -- param: board_id type; if it is not a parameter, put a space after the colon, like a[1: 2]` (+ same for limit). Tutorial never mentions `-- param:`; message alone fixed it. Type spelling `int64` guessed (Go type), accepted. Note: offset was not reported in the same run although also undeclared (only first 2?) - minor.
- `-- param: q? string` guessed for the optional param -> SearchPostsParams.Q *string. Accepted first try.
- `-- column: comment_count int64` guessed, accepted. I added a correlated count(*) of comments to PostSummary to exercise it.
- Failure 2 (from adding the column to only one query): `board/queries/search_posts.sql:1:10: result type PostSummary has another shape in board/queries/post_list.sql: 6 fields there, 5 here` / `fix: select the same columns in the same order as there, or give this result another name` -> fixed by message.
- Params struct names (PostListParams, PostDetailParams{ID}) learned from `go doc ./board` after gen - that worked well.
- Tests board/queries_test.go: paging order/limit/offset, comment count, detail join + sql.ErrNoRows, search with and without q. Pass.

## 0006 Model result missing a column (~1 min)
- recent_posts.sql `:many Post` without body. To hit line 4 I put 3 header lines (name, param, a comment).
- Output EXACTLY as Maago-Expect: `board/queries/recent_posts.sql:4:1: result columns do not match model Post: missing "body"` + fix line. gen exit 1, nothing written.

## 0007 Select every column (~1 min)
- Appended body at end of SELECT. gen/check ok; RecentPosts returns []Post. Test TestRecentPosts. Pass.

## 0008 Serve boards and posts (~4 min)
- GAP: the shape of In tags. Tutorial says "path=id", "query=page" but not the struct tag key. Guessed `maago:"path=id"` / `maago:"query=page"`; accepted (verified in views_gen.go). A view with no input: guessed `in struct{}`, accepted.
- GAP: how to get the DB in a view is not in the tutorial; found `maago.DBFrom(ctx)` in the package index.
- GAP: base.html contents (what the frame must call). Guessed `{{block "content" .}}{{end}}`; works.
- GAP: templates are embedded by generated code (`//go:embed templates/*.html` in views_gen.go) - not said anywhere; I only knew after reading generated code.
- Failures from first gen (reported together):
  - `board/urls.go:5:5: app board has routes, but no package uses board.App` / `fix: add board.App to maago.Config.Apps in main (D-114), then run maago gen: go build works after gen` -> fixed by message (wrote maago.Handler in main).
  - `board/urls.go:6:12: route path "/" ends in /, so it would match every path below it` / `fix: end it in {$} to match that path alone, such as /{$}` -> fixed by message.
- Then: `board/views.go:15:6: page HomePage has no template board/templates/home_page.html` / `fix: create it; a page type's template is its name in snake_case (D-116)` (x3, expected, told me exact paths).
- Pagination: no arithmetic in templates, so I put NextPage in BoardPage (my design).
- Tests board/web_test.go via maago.Handler + httptest: home links, board page, ?page=abc 400, missing board 404, missing post 404, /posts/abc 404, rows block without the frame, paging. Pass.
- Ran server on :18011 (PORT env), GET / -> 200, stopped.

## 0009 Two template typos (~1 min)
- Both errors reported in one run, nothing written (gen=1). post_page line matched exactly the first time.
- home_page reported `home_page.html:4:24` the first time because my line 4 was indented 4 spaces; the column points at the route-name string. I removed the indent to reproduce the Expect text exactly: `board/templates/home_page.html:4:20: unknown route name "board_detial"` + `fix: did you mean "board_detail"? routes are declared in the apps' var Routes`, and `board/templates/post_page.html:3:13: PostPage.Post has no field or method "Titel"` + `fix: did you mean "Title"?`. Exact match. (Not a Maago problem: the expected column depends on the reader's template layout, which the tutorial does not give.)

## 0010 Fix the typos (<1 min)
- gen/check/test pass.

## 0011 Forms (~4 min)
- GAP: the form's model tag place. Guessed `maago.Form \`maago:"model=Board"\`` on the embedded Form; accepted (labels/max came from the model, seen in views_gen.go bindBoardForm).
- GAP: how a view receives a form. Tutorial: "한 뷰가 GET에 빈 폼을 그리고 POST에 저장한다" but no signature. Guessed `func NewBoard(ctx, f *BoardForm)`:
  - `board/write_views.go:13:38: the In of view NewBoard is not struct{} or a struct type named in package board` / `fix: declare type NewBoardIn struct{…} in package board with path= and query= fields, or take struct{}` -> the fix line does NOT mention `maago:"form"`; I learned it from the second error in the same run:
  - `board/write_views.go:36:2: field Form says not where its value comes from` / `fix: tag it with one source: maago:"path=name", maago:"query=name" or maago:"form"` -> fixed by message.
  - `board/write_views.go:14:7: form Form is a pointer` / `fix: declare it as BoardForm; the adapter fills it in place` -> fixed by message.
- GAP: a :one query returning just a boolean. `board/queries/board_name_taken.sql:1:29: query BoardNameTaken :one has no result type` / `fix: name a model or a new type after :one, like :one PostSummary` -> made `:one Taken` with `-- column: taken bool`; both taken queries share the Taken type (same shape). A scalar result (`:one bool`) would be nicer for exists checks.
- GAP: what a view returns with a non-nil error when it returns maago.Result[P]: I return the zero Result[P]{} (wrote a tiny generic helper page0). Not documented whether that is fine (it is, tests pass).
- GAP: templates for a form field: no example of the input markup in docs. Used `{{with .Form.Name}}<input name="{{.Name}}" value="{{.Raw}}">{{.Error}}{{end}}`; Field doc made this clear enough.
- Routes: two lines per form, GET board_new / POST board_create on /boards/new; GET post_new / POST post_create on /boards/{id}/posts/new. Had to invent the second route name (tutorial: "라우트 표에 두 줄, 같은 경로와 같은 뷰" but every line needs a unique name?). Did not test whether the same name twice is allowed.
- CSRF: no token in templates; tests send Origin = server URL (passes) and Origin https://evil.example (403). Requests with no Origin pass (Go client).
- Tests board/forms_test.go: GET empty form, empty / too long name redraws 200 with error and Raw, 303 Location to the new board, duplicate name message, cross-origin 403; post: 404 on missing board (GET and POST), same title in another board ok, duplicate in same board message keeps Raw body. Pass.

## 0012 Pin notices and schedule posts (~5 min)
- Added PinnedFrom/PinnedUntil/PublishAt *time.Time. `maago makemigrations --name pin_and_schedule` -> 0004_pin_and_schedule.sql (3 ADD COLUMN). migrate ok.
- gen, as the tutorial said: `board/queries/recent_posts.sql:4:1: result columns do not match model Post: missing "pinned_from", "pinned_until", "publish_at"` + fix line -> added the three columns.
- `-- param: now time.Time` and `-- param: until? time.Time` (optional, with a `{until}` line) guessed, accepted; params became `Now time.Time`, `Until *time.Time`.
- My own bug, NOT caught by gen: `-- column: pinned bool` over `(p.pinned_from <= :now AND ...)` is NULL when pinned_from is NULL, which would fail Scan into bool at run time. gen accepts a declared column type without knowing nullability (understandable; maybe warn that a `-- column:` over an expression that can be NULL needs a pointer type, or say in docs). Rewrote as `p.pinned_from IS NOT NULL AND ...`.
- SURPRISE: the form fields of a `model=Post` form for nullable (pointer) model fields were NOT optional: generated `maago.Rule{Name: "pinned_from", Label: "PinnedFrom"}` without Optional, so an empty input would be "required". The tutorial said "optional" so I guessed the tag `maago:"optional"` on the form field; accepted -> `Optional: true`. Also labels defaulted to the Go name "PinnedFrom": I added `label='...'` to the model fields. I would expect a pointer model field to give an optional form field by default.
- GAP: Validate's signature. Guessed `func (f *PostForm) Validate()` (no ctx, no return; errors via field .Error / AddError); generated binder calls it only when all fields read. Accepted with no message. Was not told whether `Validate() error` or a value receiver would be accepted.
- "given" from the tutorial: I wrote a small generic helper `given(f maago.Field[T]) *T` (nil when !Given). The tutorial's "(given)" reads like it names something - maybe a maago helper exists that I did not find in `go doc`; I invented it.
- `{{define "time"}}` + `{{template "time" .Form.PinnedFrom}}` x3 in post_form_page.html. Verified gen checks inside it: a deliberate `{{.Lable}}` gave `board/templates/post_form_page.html:21:11: PostFormPage.Form.PinnedFrom has no field or method "Lable"` / `fix: did you mean "Label"?`. Reverted.
- Overlap check via pinned_overlap.sql in the view, form-level error with f.AddError. Pinned posts marked "[공지]" in board_page rows.
- Validate's clock: package var `now = time.Now` so tests can fix it (my design; the docs do not suggest a way to inject the clock).
- Tests board/schedule_test.go: list hides future PublishAt, shows past; current pin first, expired pin not pinned; same result with Now in KST (the SQLite text comparison is safe across zones); Validate cases (end before start, end without start, past minute, this minute ok, invalid text), DB overlap (closed and open-ended), stored nil for empty fields, three datetime-local inputs. One failure was my test data (seconds in the seed made intervals overlap by 30s). Pass.

## Final state (11:55, ~8 min total wall time for 0001-0012)
- All steps 0001-0012 done. `maago gen`, `maago check`, `go vet ./...`, `go test ./...` pass; `gofmt -l .` empty; `go mod tidy -diff` empty; showmigrations 4 of 4 applied.
- Server smoke on :18011: GET / 200, GET /boards/new 200, cross-origin POST 403. Server stopped.

## Counts
- maago failures hit (excluding the 3 deliberate Maago-Expect ones and plain "run maago makemigrations"/"has no template" which were expected): label quoting, auto=create, choices separator (2 runs), undeclared params, PostSummary shape, "/" path, board.App missing, :one with no result type, view In not a struct, form field without source, form is a pointer = 12 distinct errors. Fixed by the message alone: 11. The one not fully fixed by its own message: `default "draft" ... fix: use one of: draft|published` (pointed the wrong way; the next error fixed it). The "In of view is not a struct" fix line omits `maago:"form"`, but the sibling error in the same run supplied it.
- Things that were silent guesses (accepted with no error, so no message to learn from): fk=Board, unique=(BoardID,Title), default='draft', max=, `-- name: X :many T`, `-- param: q? string`, `{q}` lines, `-- column: x type`, `maago:"path=id"`/`query=page`, `in struct{}`, `maago.Form \`maago:"model=Board"\``, `maago:"optional"`, Validate signature, base.html `{{block "content" .}}`.
