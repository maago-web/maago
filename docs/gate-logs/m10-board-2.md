# GATE-LOG

Setup 13:42: go install maago@v0.1.0 OK, `which maago` = gateb2/bin/maago. go mod init + go get OK.
Saved `maago doc <topic>` outputs into ../ref/ for reference. The doc topics are rich: each has a full worked example.

## 0001 (~2 min)
- Wrote .env, .gitignore (.env, board.db*), main.go (OpenDB + driver import).
- `maago gen` -> "generated 0 files"; `maago check` OK; go vet OK.
- `go run .` fails: `maago: cannot open SQLite file "board.db": it is not in the working directory ...; call maago.Migrate before OpenDB, which makes it, or start the app from the project root`.
  Clear message. At 0001 there are no migrations yet, so the tutorial's "main.go opens the database with maago.OpenDB" alone cannot run on a fresh checkout. Not a blocker; `maago migrate` (0002) creates the file.

## 0002 (~3 min)
- board/board/models.go: Board(Name max=100 unique label), Post(unique=(BoardID,Title), BoardID fk=Board, Title max=200, Body, Status choices=draft/published/archived default 'draft', CreatedAt auto=create, UpdatedAt auto=update), Comment(PostID fk=Post, Body, CreatedAt).
- makemigrations -> 0001_initial.sql, migrate, gen -> models_gen.go; check OK. TestModels (Insert/Get/Update/Delete, default status, unique) passes.
- Guesses: the tutorial does not list the fields of Board/Post/Comment besides those named (I invented Board.Name, Post.Body, Comment.Body, and "archived" in choices, which only appears at 0014). Board.Name unique: guessed from 0011 "a name already taken". `maago doc model` example was enough to write tags.
- go doc maagotest is terse (DB(t) has no doc line in the -short listing) but the tutorial explained it.

## 0003 (~1 min)
- Added `Views int64 maago:"default=0"`. makemigrations -> 0002_add_board_post_views.sql (one ALTER ... DEFAULT 0). NewPostInput.Views is maago.Opt[int64]. Test checks Views==0.

## 0004 (~2 min)
- Renamed Comment.Body -> Text. Plain makemigrations draft had three ⚠ notes, the first saying exactly what to do (delete file, run --rename board_comment.body:text). Did so: one RENAME COLUMN. gen/check/test pass.

## 0005 (~3 min)
- board/queries/post_list.sql (PostSummary, LIMIT/OFFSET, comments count via subquery with `-- column: comments int64`), search_posts.sql (same PostSummary shape, `q? string`, a `{q}` line), post_detail.sql (PostWithBoard, JOIN). gen wrote queries_gen.go first try. TestQueries passes.
- Guess: what PostSummary holds is not given; I chose id,title,status,views,created_at,comments. `maago doc query` example was enough (header syntax, {q}, maago.Ptr).

## 0006 (~1 min)
- recent_posts.sql with SELECT on line 4 without body. `maago gen` (exit 1):
  `board/queries/recent_posts.sql:4:1: result columns do not match model Post: missing "body"`
  `  fix: select every column of Post, like SELECT * or SELECT p.*, or declare a new result type name (e.g. ":many PostSummary")`
  `  see: maago doc query`
  First line matches Maago-Expect exactly. (The fix line has more text than the tutorial shows; fine.) The 4:1 position required me to guess the file layout so the SELECT lands on line 4 (I put a comment line as line 3).

## 0007 (~1 min)
- Added body last in the SELECT. gen/check pass; TestRecentPosts checks Body scanned right despite column order.

## 0008 (~6 min)
- board/urls.go (home, board_detail, board_rows with maago.Block("rows"), post_detail), views.go (Home, ShowBoard, ShowPost; BoardIn with path=id, query=page, query=q), queries/all_boards.sql, templates base/home_page/board_page/post_page.
- First `maago gen`: `board/urls.go:5:5: app board has routes, but no package uses board.App` / `fix: add board.App to maago.Config.Apps in main, then run maago gen: go build works after gen` / `see: maago doc main`. The message alone fixed it (main.go: handler(db) with maago.Handler, OnError, Serve on PORT; copied the shape from `maago doc main`).
- TestPages: home, board pages 1/2 (pageSize 20), search, rows block (no <html>), post page, 404 for missing post/board and /posts/abc, 400 for ?page=abc. All pass first try.
- Guesses: "three views" - I took Home, ShowBoard, ShowPost (third view's name and route post_detail inferred from 0009 PostPage and 0015). The board page's use of search_posts (q) is my choice. Positions for 0009 needed me to lay out templates so the typos land at 4:20 and 3:13 (worked out from the column convention).

## 0009 (~1 min)
- `maago gen` exit 1 with both errors at once, exactly as expected:
  `board/templates/home_page.html:4:20: unknown route name "board_detial"` (fix: did you mean "board_detail"?...)
  `board/templates/post_page.html:3:13: PostPage.Post has no field or method "Titel"` (fix: did you mean "Title"?)

## 0010 (~1 min)
- Fixed both; gen/check/tests pass.

## 0011 (~5 min)
- forms.go (BoardForm model=Board: Name; PostForm model=Post: Title, Body), forms_views.go (NewBoard/CreateBoard, NewPost/CreatePost with BoardID path=id), queries board_name_taken.sql / post_title_taken.sql (`:one bool` SELECT EXISTS), templates board_form_page / post_form_page, links on home/board pages.
- gen/check passed first try. TestWriteBoardsAndPosts: form fields+labels on GET, required/max errors (200), 303 to BoardDetailURL, taken name/title errors with Raw drawn back, 404 for a missing board on GET and POST, 403 for a cross-site POST. Pass.
- Small guess: NewPost needs `db maago.DB` to 404 on a missing board, the tutorial only shows NewBoard's signature (fine, follows the rules). Whether `:one bool` with SELECT EXISTS works on both dialects was a guess (worked on SQLite; checked again at 0013).

## 0012 (~8 min)
- Post gets PinnedFrom/PinnedUntil/PublishAt *time.Time (with labels). `maago makemigrations --name pin_and_schedule` -> 0004_pin_and_schedule.sql (3 ADD COLUMN). migrate.
- `maago gen`: `board/queries/recent_posts.sql:4:1: result columns do not match model Post: missing "pinned_from", "pinned_until", "publish_at"` as the tutorial predicted; added the three columns.
- post_list/search_posts take `-- param: now time.Time`, filter `publish_at IS NULL OR publish_at <= :now`, add a `pinned` bool column (`-- column: pinned bool`), `ORDER BY pinned DESC, ...`. pinned_overlap.sql: `:one bool` with `until? time.Time` and an `{until}` line.
- PostForm gets three maago.Field[time.Time] (optional automatically: pointer model fields), Validate (until without from, end after start, publish_at before now truncated to minute), given(f) helper, package var `now = time.Now`. CreatePost asks PinnedOverlap with tx.
- {{define "time"}} in post_form_page.html, called with {{template "time" .Form.PinnedFrom}}. gen walked it fine.
- TestPinAndSchedule: end-before-start, past schedule, this minute OK, overlapping notice refused, later notice OK, scheduled post hidden from list, pinned first, search at a later :now sees the scheduled post. Pass first try.
- Unclear/guessed: semantics of the pin interval (nil PinnedUntil = forever? PinnedUntil without PinnedFrom?) are not given; I chose open-ended and "until requires from". Whether text comparison of SQLite times is safe (`p.pinned_from <= :now`) is not documented anywhere (maago doc model says only "TEXT in SQLite"); it worked, so the stored format must be sortable, but a sentence in the docs would help.
- Also had to look at `go doc .../rt` once to learn how times are stored; it only lists SQLiteTime, so no answer there.

## 0013 (~2 min)
- .env -> postgres://maago@127.0.0.1:55432/gateb2. Driver import in main.go and board/board_test.go -> github.com/jackc/pgx/v5/stdlib. `go get github.com/jackc/pgx/v5` (printed an odd "downloading github.com/jackc/pgx v3.6.2+incompatible" line, harmless) + `go mod tidy` dropped modernc.org/sqlite.
- Deleted migrations 0001-0004 and board.db; `maago makemigrations` -> new 0001_initial.sql (BIGINT identity, VARCHAR, TIMESTAMPTZ); migrate; gen ("generated 5 files": re-checked queries on PG); check; all tests pass on PG unchanged (SELECT EXISTS as :one bool, bool expression column, LIMIT/OFFSET all fine). No failures.

## 0014 (~5 min)
- `func (p Post) Next(to string) error` in models.go; status_views.go: Publish/Archive -> move(ctx, tx, id, to) (GetPost, Next, UpdatePost; maago.Status(409, err) when refused; SeeOther to the post). Routes post_publish/post_archive. Post page draws only the allowed button.
- My own tooling slip: a perl one-liner choked on `$1{{` (not Maago). Rewrote the template file.
- TestPublishAndArchive: 409 for draft->archived (body has no err text), 303 publish, 409 twice, buttons per status, 404 for a missing post, stale copy UpdatePost -> errors.Is(err, maago.ErrConflict). Pass first try.
- Guess: route names post_publish/post_archive and view name Archive are mine (tutorial gives only Publish's signature).

## 0015 (~7 min)
- `Hidden bool default=false`; `maago makemigrations --name hide_posts` -> 0002_hide_posts.sql; migrate; gen stopped at recent_posts.sql (`missing "hidden"`) as predicted; added it.
- NOT p.hidden in post_list, search_posts, post_detail, pinned_overlap; post_title_taken left counting hidden. New post_comments.sql (PostComments :many Comment).
- CommentForm (model=Comment, Text). ShowPostIn{ID path=id, Form CommentForm form}; ShowPost/CreateComment share postPage(ctx, db, id, f); routes comment_create (POST /posts/{id}), post_delete (POST /posts/{id}/delete -> HidePost). move() also 404s a hidden post (my choice).
- gen/check/vet/tests pass first try. TestCommentsAndHide: empty comment 200 w/ error, comment 303, shown, 404 on missing/hidden, delete hides, comments stay, hidden out of lists/search, hidden title still taken.

## 0016 (~7 min)
- api.go: APIPosts (GET /api/boards/{id}/posts, api_posts, maago.API) and APICreatePost (POST same path, api_post_create, maago.API, takes NewPostIn). savePost(ctx, tx, boardID, *PostForm) moved out of CreatePost: sets field Error AND returns errors.Join(maago.Invalid(&f.Title, msg), ...). HTML view checks `!f.Valid()` after savePost; API returns savePost's error -> 422. main: Origins: http://localhost:5173.
- gen/check/tests pass first try. TestAPI: [] for none, 200 with numeric ID and Go field-name keys, 422 problem+json for a required field and for a taken title, 400 for broken/empty JSON, 404 for missing board, preflight 204+ACAO for vite, 405 no ACAO for another origin, ACAO on the real response, 403 for another origin's POST.
- Unclear: the tutorial says savePost "writes it on the form" and the API view "returns what it found as maago.Invalid(...)". Whether maago.Invalid itself sets f.Title.Error (making Valid() false) is not said in go doc; I set both to be safe. Also the API route paths are not given (I chose /api/boards/{id}/posts).

## 0017 (~6 min)
- export_rows.sql (ExportRow, `p.id > :after ORDER BY p.id LIMIT :limit`, NOT hidden), board/export.go: ExportBoard(w, r) with r.PathValue("id"), maago.DBFrom(ctx), csv.Writer + http.Flusher per batch, cell() for = + - @, panic(err) before the first flush, log + panic(http.ErrAbortHandler) after. Package var exportBatch = 500 (tests use 2). Route board_export; link in board_page with {{url "board_export" .Board.ID}} (int accepted). BoardExportURL(id string) as the tutorial says.
- gen/check/tests pass first try. TestExport: batches of 2 over 6 visible posts, quoting, formula prefix, hidden left out, attachment header, 404.
- Not tested: the mid-stream abort path (would need a failing DB mid-loop).

(Note: durations above are rough; wall clock at the end of 0018 was 13:57, i.e. ~15 min since setup... the clock times I first wrote were wrong and were removed. Actual elapsed at 0018: setup 13:42 -> 13:57.)

## 0018 (~6 min)
- webhook.go: WebhookSettings{URL, Token}, `var Webhook`, notify(ctx, hook, p) with 3 tries, waits retryWait then 2*retryWait, only after 5xx or transport error, one deadline notifyWithin via context.WithTimeout; Bearer token header. savePost registers maago.AfterCommit after InsertPost. main fills board.Webhook from BOARD_WEBHOOK_URL/TOKEN.
- gen/check pass. First test run HUNG (114 s, `httptest.Server blocked in Close after 5 seconds, waiting for connections`): my own fake "never answers" server waited on r.Context().Done(), which the server does not cancel while the body is unread. Test bug, not Maago; replaced with a channel closed on cleanup.
- TestNotify (503,503,200 -> 3 tries ok; 503 always -> 3 tries err; 400 -> 1 try; stuck server -> error within notifyWithin=50ms) and TestWebhookAfterCommit (saved post -> 1 call by the time the 303 is back; refused form -> no call). Pass.
- Guess: the name/type of board.Webhook's struct (WebhookSettings) and the JSON payload are mine.

## 0019 (~4 min)
- main: Config.Prefix "/app"; handler() wraps http.StripPrefix("/app", h) (inside handler so a test serves the same site).
- New main_test.go TestPrefix: BoardDetailURL(1) == "/app/boards/1", GET /app/ links start /app, POST /app/boards/new -> 303 to /app/boards/N, board page under /app; Cleanup makes a Handler without a prefix again (as the tutorial says). gen/check/tests pass first try.

## 0020 (~2 min)
- Maago-Run `maago add auth`: wrote auth/ (models, queries, templates, views, tests, driver_test.go importing pgx). `maago makemigrations --name accounts` -> 0003_accounts.sql; migrate.
- `maago gen`: `auth/urls.go:5:5: app auth has routes, but no package uses auth.App` (+ fix/see lines) - matches Maago-Expect exactly.

## 0021 (~6 min)
- main.go: -createsuperuser flag (auth.CreateSuperuser), auth.SiteURL = cmp.Or(BOARD_SITE_URL, http://localhost:8000), authlib.Mail = authlib.ConsoleMail, Config.Apps {auth.App, board.App}, Auth: auth.Current, LoginURL "/app/login". Copied from the `maago doc auth` example main.
- Home takes `u *auth.User`; HomePage.User; base.html header links {{url "login"}}, {{url "signup"}}, a POST form to {{url "logout"}}.
- gen/check OK. `go test` FAILED in 8 board tests: `maago: routes require login or take the user, but Config.Auth is nil, so none has a user; set it to what tells the user of a request` - the message alone told me: board tests' site() now gives Apps {auth.App, App}, Auth: auth.Current, LoginURL. (Same in main_test's cleanup Handler.)
- TestSignIn (main_test, through handler() with /app): authlib.Iterations=1000, auth.InsertUser verified user, POST /app/login -> 303 + "session" cookie, GET /app/ shows "Signed in as ann@example.com"; anonymous shows the /app/login link. The pattern for adding a verified user came from the copied auth/auth_test.go (newUser) - project code, allowed.
- `go run . -createsuperuser` with MAAGO_SUPERUSER_* worked on the dev DB.
- Unclear: the tutorial's "header links to sign in, sign up and sign out" - base.html has no user (only HomePage does), so whether to show sign-out conditionally is a design question the tutorial leaves; I always draw all three. Also, which field names the login form posts (email, password) came from `maago doc auth`.

## 0022 (~12 min)
- `AuthorID *int64 maago:"fk=auth.User,label='Author'"`; `maago makemigrations --name post_author` -> 0004 (column, index, FK to auth_user). recent_posts.sql + author_id (I added it before gen this time, as the tutorial warned).
- Views that write and the forms' GETs take `u auth.User`: NewBoard, CreateBoard, NewPost, CreatePost, CreateComment, Publish, Archive, HidePost, APICreatePost. savePost(ctx, tx, u, boardID, f) sets AuthorID: &u.ID. mayChange(u, p) -> maago.Status(403, ...) unless staff or author; nil author = staff only.
- Probed the tutorial's claim about maago.LoginRequired: `board/urls.go:7:50: there is no maago.LoginRequired: the view's user says it` / `fix: remove it` / `see: maago doc view`. The fix line only says "remove it"; the tutorial says gen answers "with what to write instead" - the fix could say "take u auth.User in the view" to be complete.
- gen/check OK. go test: 6 tests failed with 401/303/403 - expected, since my old tests posted anonymously. Added test helpers newUser/signedIn (POST /login, take the "session" cookie), mustUser, cookie args to get/postForm; old posts in tests get AuthorID.
- TestAuthors: nobody -> GET 303 to /login?next=..., POST 401, API 401 + WWW-Authenticate: Bearer, comment 401; author set from the session and from an API token (auth.InsertAPIToken with authlib.NewToken); publish/archive/delete matrix nobody 401 / bob 403 / ann 303 / staff 303; author-less post: ann 403, staff 303. All pass.
- Gap: how a test makes an API token is not in the tutorial or `maago doc auth` (it says "a token made at /tokens"); I found auth.NewAPITokenInput in the copied models and authlib.NewToken in go doc.
- TestPrefix's anonymous POST then 401: moved that check into TestSignIn with the session.

## 0023 (~3 min)
- HidePost -> RemovePost (same signature): PostComments decides; none -> DeletePost, else hide. Route post_delete points at RemovePost. gen/check pass.
- TestRemoveWithoutComments: delete -> 303, GetPost is sql.ErrNoRows, title free again. TestCommentsAndHide still covers hide. Pass first try.

## 0024 (~3 min)
- main: adminApp := admin.App(admin.Config{Models: [][]admin.Model{auth.Admin}, Can: auth.Can}) in Config.Apps (shape from `maago doc admin`).
- `maago gen` first said `board/models.go:11:6: app board has models, but no package uses board.Admin` - one line off from the tutorial's 12:6 only because my models.go has one line fewer above `type Board`. Added a package doc comment line; then exactly `board/models.go:12:6: app board has models, but no package uses board.Admin` (+ fix: add board.Admin to admin.Config.Models in main...).

## 0025 (~12 min)
- Models: board.Admin, auth.Admin. Tags: Post admin_list=(Title,BoardID,Status,AuthorID,CreatedAt), admin_search=(Title,Body), admin_filter=(Status,BoardID,Hidden); Comment admin_list/search/filter; Board admin_search=(Name). makemigrations: "no changes". gen/check pass.
- Probed wrong tags: `board/models.go:22:118: admin_search applies to strings, but Post.Views is int64` and `board/models.go:22:124: unknown column field "Bodyy" in Post` / `fix: did you mean "Body"? allowed: ID, BoardID, ...` - excellent messages.
- admin_test.go (package main, through handler()): superuser adds a Permission row for ed (not staff) -> ed gets 403; superuser turns on is_staff via the User change form; Permission rows board/Post view+change and auth/Permission add; editor lists posts (200), not comments, cannot add a post, cannot add Permission rows, cannot delete; editor changes a post (303); a form opened before that save -> 409 and nothing changes; list columns exclude the body; ?q=&status=&board_id= together; search finds a word in the body ignoring case; deleting a post with comments is refused (admin answers 200 with the page again), without comments 303 and gone.
- Failure (my test): the first change POST answered 200 with "This field is required." under status. My form scraper expected `<option value=".." selected>`; the admin writes `<option selected>published</option>` (no value attr). Fixed the scraper. Docs gap: `maago doc admin` gives URLs and that form keys are snake_case, but not the hidden version field's name (`_version`) or the delete POST's body; I had to dump the HTML of a change page to write the test. A sentence ("the change form carries _version; POST the delete URL with the same ?id=") would remove the guessing.

## 0026 (~3 min)
- board/tailwind.css: `@import "tailwindcss" source(none);` + `@source "./templates";` (I knew the Tailwind v4 syntax; the tutorial only names source(none) and @source). `maago tailwind` -> "wrote board/static/tailwind.css" (v4.3.3). base.html links {{static "board/tailwind.css"}} and uses a few classes.
- TestTailwind: GET /app/ has /app/static/board/tailwind.<hash>.css; it serves 200 with the classes, Cache-Control: public, max-age=31536000, immutable.
- Gap: the exact content of tailwind.css (the @import line) is not shown anywhere in maago doc; an agent without Tailwind v4 knowledge would have to guess.

## 0027 (~1 min)
- base.html -> {{static "board/style.css"}}: `board/templates/base.html:4:39: static "board/style.css": no file board/static/style.css` / fix: add it to board/static, or name one of its files. Exactly as expected.

## 0028 (~1 min)
- Back to board/tailwind.css; gen/check/tests pass.

## 0029 (~15 min)
- Attachment model (PostID fk=Post, Key max=32 unique admin=readonly, Name, Type, Size, CreatedAt). `makemigrations --name attachments` -> 0005. I first gave Name max=255, then realized a long sent name would be a DB error; removed it -> 0006_attachment_name_any_length.sql (ALTER TYPE TEXT). Self-inflicted extra migration.
- AttachmentForm{File maago.Field[maago.File]}; AttachIn{ID path=id, Form AttachmentForm form}; Attach(ctx, u, tx, in) -> Result[PostPage] (404 hidden, mayChange 403, SaveMedia, InsertAttachment, DeleteMedia if the insert fails). DownloadAttachment(w, r): GetAttachment, GetPost, 404 hidden, draft only for author/staff via maago.UserFrom[auth.User], maago.ServeMedia. Routes attachment_add (POST /posts/{id}/attachments), attachment (GET /attachments/{id}). post_attachments.sql, delete_post_attachments.sql (:exec); RemovePost's delete path deletes rows first, then files in maago.AfterCommit.
- main: MediaDir (package var mediaDir = "media", tests use t.TempDir()), media/.gitignore.
- Failures:
  1. All handler tests: `maago: forms take files, but Config.MediaDir is empty; set it to the directory to keep them in, such as media` - message alone fixed it.
  2. Tried to give ShowPostIn a second form so the post page's GET could draw both forms: `board/views.go:69:2: ShowPostIn reads a second form, Upload` / `fix: keep one form field in In; a page with two forms posts each to its own route`. Clear, but it leaves an open question (BLOCKER-ish, ~5 min): the post page has two forms (comment + upload) and its GET binds only one, so the other form's Field has no Name/Label (only the bind describes them). And when Attach redraws PostPage, the comment form is unbound (CommentForm{}): name="" in the template. Workaround: wrote name="file"/name="text" and the labels literally in post_page.html. Nothing in maago doc form/view says how to draw a second, unbound form (no "describe" helper, no example of a page with two forms).
  3. My test expected the sent name "../../evil.txt" in the row; Go's multipart strips the path, the row has "evil.txt". Test changed (no file was written outside media/ either way).
- TestAttachments: nobody 401, bob 403, no file 200 + required message, PNG bytes sent as .txt -> Type image/png, file in media dir under its key, draft file nobody 401 / bob 403 / ann,staff 200 attachment download, published file for anybody, 404s, delete without comments removes row and file, hidden post 404 for upload and download. TestAttachTooBig: MaxBody 1024 -> 413. All pass.
- Guess: what a draft's download answers for nobody (401 vs redirect) - I wrote 401 by hand.

## 0030 (~5 min)
- my_posts.sql (MyPost: id, title, status, publish_at, created_at, board_id, board_name; NOT hidden; author_id = :author_id), me.go: MePage{Email, Posts}, Me(ctx, u auth.User, db, in struct{}); routes me and api_me (maago.API) on one view; me_page.html; "My posts" link in base.html.
- gen/check/tests pass first try. TestMe: ann sees draft/published/scheduled, not hidden or bob's, on the page and in JSON (Bearer token); no PasswordHash in JSON; nobody -> 303 /login?next=..., API -> 401 + WWW-Authenticate: Bearer.

## 0031 (~7 min)
- post_title_taken.sql gets `-- param: skip_id? int64` and a `{skip_id} AND id <> :skip_id` line (reflowed the EXISTS so the condition starts a line). savePost compiles unchanged (SkipID nil).
- PostEditForm (model=Post: Title, Body). edit.go: EditPostIn, ChangePostIn{ID, Version int64 query=version, Form}, PostEditPage{Post, Version, Form}; EditPost fills Raw; ChangePost: editable() (404 hidden, mayChange 403), title check with SkipID, changed.UpdatedAt = time.UnixMicro(in.Version), UpdatePost; ErrConflict -> f.AddError + same page with the old version. Routes post_edit/post_change; template with action `{{url "post_change" .Post.ID}}?version={{.Version}}`; Edit link on the post page.
- gen/check/tests pass first try. TestEditPost: nobody GET 303 / POST 401, bob 403 on both, no version 400, 404, taken title refused, own title and another board's title pass, stale form -> 200 with the error, what was written, the old version kept, DB unchanged; pin and schedule times unchanged.

## 0032 (~12 min)
- post_list/search_posts: `p.status <> 'draft'`; export_rows: `:now` + both conditions, ExportBoard reads now() once for every batch. maySee(u *auth.User, p Post) error in status_views.go. postPage(ctx, db, u, id, f) reads GetPost -> maySee -> PostDetail; ShowPost takes `u *auth.User`, CreateComment/Attach pass &u. DownloadAttachment uses maago.UserFrom -> maySee -> http.NotFound. Webhook moved from savePost into moveTo (publishing only); move (HTML) and APIPublish (POST /api/posts/{id}/publish, api_publish, maago.API) share moveTo.
- Failure: `maago gen` stopped at a compile error of mine: `board/api.go:5:2: "time" imported and not used` / `fix: go build reports the same error; fix it there`. Fine; fixed.
- Then 6 old tests failed (lists empty, post pages 404 for nobody on drafts, draft file codes 401/403 -> 404, webhook count). All expected by the tutorial ("tests that read lists make their posts published first"). Updated: Status: maago.Set("published") in list tests, publishAll() helper for posts made through the form, signed-in GETs of draft pages, new webhook expectations (quiet on write/archive/refused move, hears publish).
- TestDrafts: draft and scheduled post pages and comments for nobody/bob (404) vs ann/staff (200/303), files 404 for nobody/bob, published and archived pages open; board list, search, API and CSV show only out+archived; API publish -> JSON published post and one webhook call; bob's API publish 403, no call. All pass.

## 0033 (~6 min)
- main: `//go:embed migrations` + maago.Migrate(ctx, maago.DBConfig{}, migrations) before OpenDB (logs each applied file), -createsuperuser after it, BOARD_SITE_URL required with the tutorial's exact message (checked after the -createsuperuser branch, which does not need it), Serve on PORT (8000 default).
- Dockerfile (golang:1.26 build stage, CGO_ENABLED=0; distroless static nonroot; /srv/media copied with --chown=65532:65532 as a VOLUME; ENTRYPOINT /srv/board) and .dockerignore (.env, *.db, media contents, bin, .git). Not run (no docker).
- go.mod already uses the released maago v0.1.0 (no go.work here).
- gen/check/vet/tests pass; `go mod tidy -diff` clean; gofmt clean.
- `CGO_ENABLED=0 go build -o bin/board .`, run from an empty dir (no .env) against a fresh schema (psql `CREATE SCHEMA ship`, MAAGO_DATABASE_URL=...gateb2?search_path=ship - pgx passed search_path through):
  - without BOARD_SITE_URL: applied 0001..0006, then `BOARD_SITE_URL is not set: set it to ...` and exit.
  - `-createsuperuser` with MAAGO_SUPERUSER_*: `created superuser me@example.com (id 1)`.
  - PORT=18012 BOARD_SITE_URL=http://localhost:18012: GET /app/ 200, POST /app/login 303 + cookie, /app/admin/ 200, create board 303 to /app/boards/1, hashed tailwind CSS 200. SIGTERM -> `maago: shutting down, waiting up to 30s for 0 requests`, stopped. Dropped the schema; port 18012 free.

## Summary
- Last step passing gen/check/tests: 0033 (all steps done). Wall clock ~13:42 -> 14:13 (~31 min of real time per `date`; my per-step durations above are rough guesses and add up to more).
- All 6 Maago-Expect checks reproduced (0006, 0009 x2, 0020, 0024, 0027); 0024's position needed one extra line in models.go (layout, not Maago).
- Time: measured wall clock 13:42 -> 14:12, about 30 minutes. The per-step "~N min" figures above are relative effort guesses (they sum to far more) - read them as proportions.
- 0022: I added author_id to recent_posts.sql before running gen, so the predicted gen stop there was never observed (it was observed at 0012 and 0015).

## Failure tally
- Unexpected, the Maago message alone fixed it (7): 0001 `cannot open SQLite file "board.db" ... call maago.Migrate before OpenDB`; 0008 `app board has routes, but no package uses board.App`; 0012 and 0015 `recent_posts.sql:4:1: result columns do not match model Post: missing ...` (predicted by the tutorial, still hit); 0021 `routes require login or take the user, but Config.Auth is nil`; 0029 `forms take files, but Config.MediaDir is empty`; 0032 `"time" imported and not used`.
- Unexpected, the message said what not to do but not what to do instead (1): 0029 `ShowPostIn reads a second form, Upload`.
- Expected consequences of a step, fixed from the tutorial text (3 rounds): 0022 old tests posting anonymously (401/303/403), 0022 TestPrefix 401, 0032 list/visibility tests.
- My own test/tooling bugs (4): 0014 perl `$1{{`, 0018 httptest server hang, 0025 form scraper vs `<option selected>`, 0029 multipart strips `../` from the file name.
- Not failures: 6 Maago-Expect steps (all matched), probes of maago.LoginRequired and bad admin tags.
