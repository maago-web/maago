# Gate log (board tutorial, steps 0001-0012)

Start 11:47 KST.

## Setup
- go mod init / edit / go get modernc.org/sqlite as instructed. OK.
- `go doc -all github.com/maago-web/maago` read. GAP: the root package docs never list the model field tags
  (fk=, choices=, default=, auto=, unique=, max=, optional...). `go doc maago.Model` shows only table= and max=.

## 0001 Start the project (~3 min)
- Wrote main.go (maago.OpenDB with DBConfig{}), .env, .gitignore.
- `go build ./...` FAIL: `go: build output "board" already exists and is a directory` (module name board + dir board/). Not Maago's fault; use `go vet ./...`/`go test ./...` instead. Tutorial could mention it.
- maago gen OK, check OK, migrate "nothing to apply", go run OK.
- Unclear: tutorial does not say whether migrate must run before OpenDB (DBConfig doc says the SQLite file must exist, maago migrate makes it). Running migrate first made it.

## 0002 Board, Post, Comment (~6 min)
- Guessed tag syntax (no doc). Failures, all from `maago gen`:
  1. `board/models.go:23:62: default "draft" of Post.Status is not one of its choices` / `fix: use one of: draft|published`
     I had written `choices=draft|published`. MISLEADING: the message treats "draft|published" as one choice and echoes it back as if it were a list; it should say `|` is not the separator. Cost two extra tries.
  2. `auto in Post.CreatedAt is not written as expected` / `fix: write auto=create or auto=update` (x3) -> fixed from message alone. Tutorial says "auto 태그" only.
  3. (my wrong try) `default=draft` -> `default of Post.Status must be quoted text / fix: write text in quotes, like default='draft'` (fine, the tutorial had it right).
  4. (my wrong try) `choices=(draft,published)` -> `choices in Post.Status is not written as expected / fix: list them with /, like choices=draft/published` -> fixed by message.
  5. `migrations do not match models: table "board_board" is missing / fix: run maago makemigrations` -> as expected.
- makemigrations -> 0001_initial.sql; migrate; gen -> models_gen.go; check OK.
- Guess: unique on Board.Name (`maago:"max=100,unique"`) - I added it because step 0011 talks about a taken board name. Worked first try.
- Guess: the auto values (create/update) - message told.
- Test board/models_test.go: insert/default/auto/unique(BoardID,Title)/update/delete. PASS.

## 0003 Post.Views (~2 min)
- Added `Views int64 \`maago:"default=0"\``. gen: `board/models.go:25:2: migrations do not match models: column "views" of "board_post" is missing / fix: run maago makemigrations` (expected).
- makemigrations -> 0002_add_board_post_views.sql (`ADD COLUMN ... NOT NULL DEFAULT 0`). migrate, gen, check OK. NewPostInput.Views is maago.Opt[int64] as promised.
- Test TestPostViewsDefault (unset -> 0, maago.Set[int64](7) -> 7). PASS.
- Guess: whether a numeric default is quoted (`default=0`) - guessed unquoted, worked.

## 0004 Rename Comment.Body -> Text (~2 min)
- Plain makemigrations wrote the drop+add draft with the ⚠ comments, exactly as told. Deleted it, ran `--rename board_comment.body:text` -> `ALTER TABLE ... RENAME COLUMN "body" TO "text"`. migrate/gen/check OK, test updated (Text). PASS.
- Note: plain makemigrations exits 0 with a dangerous draft; fine because the comment explains it, but `maago migrate` on it would have run (I didn't try).

## 0005 Post queries (~5 min)
- Tutorial does not give query file syntax beyond `-- name:` and `{q}` / `q?`. Guessed `-- name: PostList :many PostSummary`, `:param` placeholders.
- gen failures (fixed by message alone):
  `board/queries/post_detail.sql:5:14: parameter id is not declared / fix: declare it with -- param: id type; ...` (and board_id, limit, q).
  Message says "type" but not which type vocabulary (Go types? SQL types?). Guessed `int64`, `string`, and `-- param: q? string` for the optional one; worked first try. Note: `offset` was not reported in the same run (only limit on line 6) - only the first undeclared parameter of a line is reported?
- Guess: whether inside SQL I write `:q?` or `:q` - wrote `:q?` first, but gen failed on the declaration before telling; switched to `:q` with `-- param: q? string`. SearchPostsParams.Q is *string.
- gen/check OK. Tests: paging (LIMIT/OFFSET), PostDetail JOIN + sql.ErrNoRows, search with/without q. PASS.
- Didn't need `-- column:` (no count(*)).

## 0006 Deliberate mistake (~1 min)
- recent_posts.sql `:many Post` without body. gen printed exactly the Maago-Expect line + fix line, exit 1, wrote nothing. MATCH. (Line 4 matched because my SELECT happened to be on line 4 after name + 2 params.)

## 0007 Select every column (~1 min)
- Added body at the end. gen/check OK, RecentPosts returns []Post. Test TestRecentPosts PASS.

## 0008 Serve boards and posts (~6 min)
- Wrote board/urls.go (Routes: home, board_detail, board_rows with maago.Block("rows"), post_detail), board/views.go, templates base/home_page/board_page/post_page, a helper query board_all.sql (`:many Board`) for the home page. main.go: maago.Handler(Config{DB, Apps: board.App, OnError}), PORT env.
- Tutorial said "path=id", "query=page" but not the tag key; I guessed Go-style `path:"id"`. One gen run gave all fixes (all fixed from the messages alone):
  - `board/urls.go:5:5: app board has routes, but no package uses board.App / fix: add board.App to maago.Config.Apps in main (D-114), then run maago gen: go build works after gen`
  - `board/urls.go:6:12: route path "/" ends in /, so it would match every path below it / fix: end it in {$} to match that path alone, such as /{$}`
  - `board/urls.go:7:12: view BoardDetail reads no {id} of route board_detail / fix: add a field tagged maago:"path=id" to the In of BoardDetail` (x3)
  - `board/views.go:21:2: field ID says not where its value comes from / fix: tag it with one source: maago:"path=name", maago:"query=name" or maago:"form"` (x3)
- Guesses that worked first time: `in struct{}` for a view with no input; In/page type names; `*int64` for optional ?page; templates under board/templates/; base.html uses `{{block "content" .}}{{end}}`; `{{date "layout" .CreatedAt}}` (found in go doc DateFunc). Block route reuses the same view.
- gen/check OK. Test board/views_test.go via maago.Handler + httptest: home links, paging (?page=2), /rows returns only the block (no <html>/<h1>), post page, 404 missing post/board, 400 ?page=abc, 404 /posts/abc. PASS first run.
- Unclear: whether `maago.Handler` must be built in tests with OnError, and how to build URL in tests - used generated BoardDetailURL etc.

## 0009 Two template typos (~2 min)
- gen printed both errors in one run, exit 1. Message text matched exactly; my post_page.html first had the title on line 2, so the position was 2:13 vs expected 3:13. Added an <article> line so the h1 is on line 3 -> now byte-exact match with both Maago-Expect lines. (Position depends on how the template is laid out; the tutorial gives no template text, so an exact line:col match is luck.)

## 0010 Fix typos (~1 min)
- gen/check/test OK.

## 0011 Write boards and posts (~6 min)
- Wrote board/forms.go (`BoardForm{maago.Form \`maago:"model=Board"\`; Name maago.Field[string]}`, PostForm with Title, Body for model=Post), board/write_views.go (BoardNew, PostNew returning (maago.Result[XPage], error); In has `Form XForm \`maago:"form"\``; PostNew's In has BoardID `maago:"path=id"`), queries board_name_taken.sql / post_title_taken.sql, templates board_form_page.html / post_form_page.html, links on home/board pages.
- GUESSES (tutorial/doc give no example): where the model= goes (tag on the embedded maago.Form - worked), how the form reaches the view (`maago:"form"` field in In - from the earlier gen fix text "maago:\"form\""), return type maago.Result[Page] (from go doc SeeOther/Show).
- gen failures:
  1. `board/queries/board_name_taken.sql:1:29: query BoardNameTaken :one has no result type / fix: name a model or a new type after :one, like :one PostSummary` (x2). A single-column scalar result (`SELECT EXISTS(...)`) cannot be returned as bool; needs a struct type. I named both `:one Taken` (same shape -> shared type). Fixed from message. Friction: a "taken?" check is a struct with one bool field (taken.Taken).
  2. `board/urls.go:10:28: route name "board_new" is taken by board/urls.go:9 / fix: route names are one namespace across the project; rename one` (x2). The tutorial says "라우트 표에 두 줄, 같은 경로와 같은 뷰" but not that the two lines need different names; I named the POST ones board_create / post_create. Fixed from message, but had to invent names.
  3. `board/write_views.go:25:6: non-boolean condition in if statement / fix: go build reports the same error; fix it there` - my bug (taken is Taken struct). Good that gen surfaces the type error.
- Labels are "Name", "Title", "Body" (derived from field names); I did not set a label on the model (no doc for a label= model tag; tutorial mentions 라벨 from the model but not its syntax).
- gen/check OK. Tests board/forms_test.go: GET empty form; POST empty -> 200 + "This field is required."; 101 chars -> 200 + "at most 100" + Raw value redrawn; create -> 303 to /boards/N; duplicate name -> 200 + my message; cross-origin Origin -> 403; post form 404 on missing board (GET and POST); duplicate title -> 200; create -> 303 -> page shows. PASS first run.
- Unclear: Handler's CSRF check - test sets Origin to the test server URL for same-origin posts; worked. Not told whether missing Origin is allowed.

## 0012 Pin notices and schedule posts (~7 min)
- Added `PinnedFrom, PinnedUntil, PublishAt *time.Time` to Post. gen: `migrations do not match models: column "pinned_from" of "board_post" is missing` (x3, expected). `maago makemigrations --name pin_and_schedule` -> 0004_pin_and_schedule.sql (3 ADD COLUMN TEXT). migrate.
- gen then blocked recent_posts.sql as the step says:
  `board/queries/recent_posts.sql:4:1: result columns do not match model Post: missing "pinned_from", "pinned_until", "publish_at"` -> added the three columns (fixed from message).
- post_list.sql / search_posts.sql take `-- param: now time.Time`, filter `publish_at IS NULL OR publish_at <= :now`, add `-- column: pinned bool` expression and `ORDER BY pinned DESC, ...` (both share PostSummary, so both got the column). pinned_overlap.sql `:one Taken` with `-- param: until? time.Time` and a `{until}` line. gen OK first try.
  - GUESS: param type spelling `time.Time` (worked); that SQLite text times compare correctly with `<=` (they do in tests; doc says only "as SQLite stores it (D-105)", nothing about ordering).
- PostForm: three `maago.Field[time.Time] \`maago:"optional"\``; `func (f *PostForm) Validate()` (GUESS at the signature: no doc names it; gen accepted it and the generated code calls f.Validate()). Uses time.Now().Truncate(time.Minute) for "this minute is not the past". Errors via field .Error, overlap via f.AddError.
  - GUESS: whether `optional` is needed when the model field is a pointer - added it anyway.
  - GAP: "뷰가 모델에 nil을 넘긴다(given)" - "(given)" reads like a helper name, but there is no maago.Given/given function in go doc. I wrote my own `func given[T any](f maago.Field[T]) *T`.
  - CONFUSION: after f.Valid() was true, setting `f.Title.Error` by hand (step 0011's advice) does not make Valid() false (Valid reads what FinishForm recorded + form errors). AddError does. So my view checks `f.Valid() && f.Title.Error == ""` before inserting. Docs: "Valid reports whether the form was sent and has no error: its fields read, Validate passed, and no form error was added" - a field Error set by the view afterwards is not counted; easy to miss and would insert a duplicate (then 500 from the unique index).
- Template `{{define "time"}}` in post_form_page.html, called three times with `{{template "time" .Form.PinnedFrom}}`. Verified gen checks inside it: a deliberate `{{.Lable}}` gave `board/templates/post_form_page.html:2:11: PostFormPage.Form.PinnedFrom has no field or method "Lable" / fix: did you mean "Label"?`.
- gen/check/vet OK, gofmt clean. Tests board/schedule_test.go: pinned first, expired pin not pinned, scheduled hidden until :now passes, search same; overlap query; form: until before from -> 200 + message; past schedule -> 200; "nope" -> "Enter a date and time." with Raw redrawn; this minute -> 303; pin -> 303; overlapping pin -> 200 + form error; empty time fields -> nil columns. PASS first run.
- Ran the server on :18012 (PORT env), GET / 200, POST /boards/new with Origin -> 303, stopped it.

## Totals
- All 12 steps pass gen/check/vet/tests. Both Maago-Expect steps matched the message text exactly (0009 position after aligning my template's line).
- Failures hit: 0002: 4 runs (choices separator x2 tries, auto x3, default quoting once from my own wrong try), migrations-missing x4 (expected), 0005 undeclared params x5, 0008 x8 (app not used, "/" path, path tags x6), 0011 x6 (no result type x2, route name taken x2, go type error x2), 0012 recent_posts columns (expected). Counting distinct problems: ~10 real problems (excluding the deliberate/expected ones); 9 were fixed from the message alone, 1 (choices "|" separator) had a misleading message and took one extra guess.
- Wall time 11:47 -> 11:55 by `date` (~8 min). The per-step "(~N min)" figures above are relative effort guesses written before I checked the clock; they overstate: scale them down by about 5x.
