# GATE log: steps 0037-0040

Setup (02:24 KST): read STEPS.md, listed board, snapshot copied to ../m12gate2-snap0. Baseline verify (gen, check, vet, gofmt, test): all pass (generated 12 files; auth ok, board ok).

## 0037 Draw both forms, and ask as an API client
Start 02:24 KST, end 02:26 (about 2 min).

Read: STEPS.md 0037; board/views.go, board/forms.go, board/templates/post_page.html, board/attach_test.go, board/permissions_test.go, board/web_test.go (TestAPI, TestMe); auth/auth.go (SignIn, SignInToken already present in the copied auth); auth/permissions_test.go (cat by token, as an example); `go doc .../maagotest` and `go doc .../maagotest Bearer`. `grep` showed forms_gen.go already has NewCommentForm/NewAttachmentForm.

Changes (all from the step message):
- views.go: postPage builds PostPage with AttachForm: NewAttachmentForm(); Attach draws the page again with NewCommentForm() instead of CommentForm{}.
- post_page.html: file field drawn as the step shows ({{with .AttachForm.File}}<label>{{.Label}} <input type="file" name="{{.Name}}"></label> ...).
- permissions_test.go: new role cat_token (maagotest.Bearer(h, auth.SignInToken(cat))) between bob and ann; every want() got a column equal to bob's; the three bob-only write rows (board_create, post_create, api_post_create) now skip cat, and cat gets its own rows "cat의 게시판", "cat의 글", "cat의 API 글".
- web_test.go: TestAPI and TestMe use auth.SignInToken instead of authlib.NewToken + InsertAPIToken (authlib import still used elsewhere).
- attach_test.go: GET of the post checks `<label>파일 <input type="file" name="file"></label>`; the no-file attach checks `<textarea name="text">`.

Commands: gen (generated 12 files), check, go vet, gofmt -l (empty), go test ./... : all pass on first run.
Guesses: the token's column for admin rows and redirects is exactly bob's (step said "Its column is bob's"); it held.
Unexpected failures: none.
Unclear: nothing. (Correction: the board as delivered had no *_gen.go files; my baseline `maago gen` wrote them, NewCommentForm/NewAttachmentForm included, so they were there when I started 0037.)

## 0038 Type the views that write the response themselves
Start 02:26 KST, end 02:28 (about 2 min).

Read: STEPS.md 0038; board/export.go, board/urls.go, views.go DownloadAttachment, TestExport and TestAttachments; `maago doc` (topic list) and `maago doc view` (the section "A view that answers with something other than a page ... takes w and r after In, and returns only an error"; error before writing answers as a typed view, after cuts the connection and OnError hears it; maago.ServeMedia returns its error).

Changes:
- export.go: type ExportIn{ID int64 path=id}; ExportBoard(ctx, u *auth.User, db, in ExportIn, w, r) error. Returns GetBoard's, ExportRows's and the csv.Writer's errors on any page; headers and the CSV header line set once the first page is read; abort() and the log/errors/database/sql imports gone. (step message)
- views.go: type DownloadIn; DownloadAttachment typed, returns maago.ServeMedia's error; the switch on fs.ErrNotExist and the panics gone; unused imports database/sql, io/fs, strconv removed (told by maago gen, below).
- tests: AttachmentURL(a.ID), BoardExportURL(b.ID) without fmt.Sprint (go vet told me: "cannot use fmt.Sprint(a.ID) (value of type string) as int64 value in argument to AttachmentURL"); the non-number test asks "/boards/abc/posts.csv"; fmt import dropped from attach_test.go (vet: imported and not used). TestAttachments renames the media file away, expects 404, renames it back.
- main.go: OnError comment says it also hears the error of a self-writing view after it wrote (connection cut).

Commands and results:
- `maago gen` (first, before fixing imports): exit 1, "board/views.go:5:2: "database/sql" imported and not used / fix: go build reports the same error; fix it there" (also io/fs, strconv), plus the go build lines from the old views_gen.go ("not enough arguments in call to ExportBoard"). Expected consequence of my edit, not a step surprise.
- gen again: generated 12 files; build ok. vet: the two URL-type errors above, fixed.
- verify: gen, check, vet, gofmt (empty), go test ./...: all pass.
- Mutation check: made DownloadAttachment drop ServeMedia's error -> TestAttachments failed "a row whose file is gone: 200", as the step says; restored. (Note: I briefly put a backup copy of views.go in /tmp/v.bak, outside GATE, and deleted it right after; nothing else outside GATE touched.)

Guesses: where to set headers relative to the CSV header line: I set them with the first page, before the first Write. BoardExportURL(2) for "no such board" kept as before (the board's id is 1).
Unexpected failures: none (the gen/vet errors followed from the edit, and vet's message pointed straight at the fix).
Unclear: nothing significant. "main's comment on OnError says so now" is an instruction to edit main's comment, phrased as a fact; I read it as an edit.

## 0039 Give each post a key, and each image its size
Start 02:28 KST, end 02:30 (about 2.5 min).

Read: STEPS.md 0039; board/models.go, all board/queries/*.sql, board/policies.sql; `maago doc model` (UUID and JSON field types, "time.Time, []byte, maago.UUID and maago.JSON have no default: such a field added to a table with rows is a pointer", admin=readonly), `maago doc query` (policy header, model results must be every column, maago.UUID is a param type); `go doc` for maago.File (Open), NewUUID, UUID, JSON, SaveMedia; TestAPI, TestAdmin.

Changes:
- models.go: Post.Key *maago.UUID `maago:"unique,admin=readonly"`; Attachment.Meta *maago.JSON[AttachmentMeta]; type AttachmentMeta struct{ Width, Height int }. (step message)
- `maago makemigrations --name post_key_attachment_meta`: "created migrations/0006_post_key_attachment_meta.sql", no warning (ADD COLUMN meta JSONB; ADD COLUMN key UUID; UNIQUE (key)).
- `maago migrate`: applied 0001..0006 (all six: the m12gate2 database had none applied before; not a problem, just noting the database was empty).
- `maago gen`: exit 1, exactly as the step predicted:
    board/queries/post_attachments.sql:4:1: result columns do not match model Attachment: missing "meta"
    board/queries/recent_posts.sql:5:1: result columns do not match model Post: missing "key"
  Fixed by adding meta / key to those SELECTs. post_list.sql and search_posts.sql select p.key (PostSummary.Key *maago.UUID). New board/queries/post_by_key.sql (:one Post, param key maago.UUID, policy visible p, SELECT * ... WHERE p.key = :key AND visible(p)); gen made PostByKey(ctx, u, db, PostByKeyParams{Key, Now}).
- views.go: savePost gives InsertPost Key: &key (maago.NewUUID()); APIPostIn{Key maago.UUID path=key} and APIPost returning PostByKey(... Now: time.Now()); fileMeta(f) opens an image/* file and reads image.DecodeConfig before SaveMedia (decoders image/gif, jpeg, png registered with blank imports); InsertAttachment gets Meta.
- urls.go: maago.GET("/api/posts/{key}", "api_post", APIPost, maago.API).
- post_page.html: `{{with .Meta}} · {{.V.Width}}×{{.V.Height}}{{end}}` beside the size.
- Tests: TestPermissions failed first with "maagotest.Permissions: route api_post (GET /api/posts/{key}) has no row; add one with each role's status" (expected: a new route needs its row; the step did not say so, but the table's comment does). Gave the table's posts keys; rows api_post for out (200 x5) and for the draft unseen (404,404,404,200,200). TestAPI: list and GET by key have "Key":"<text>"; random UUID and "/api/posts/abc" are 404. TestAttachments: uploads a 3x2 PNG, row Meta == {3,2}, the text file's Meta nil, page shows "3×2". TestAdmin: posts get keys; staff's row page shows the key; after the admin saves, the key is unchanged.
- verify: gen, check, vet, gofmt (empty), go test: pass; migrate: nothing to apply.

Guesses: which image kinds to decode (gif/jpeg/png, the stdlib ones); a non-decodable image gives nil Meta rather than an error; AttachmentMeta fields as int. APIPost returns Post (as APIPublish does).
Unexpected failures: none (the gen error was predicted; the permission-row failure is the table doing its job).
Unclear: the step does not say a permission row is needed for api_post (obvious from the table, though). "maago gen named the two that list theirs" was accurate.

## 0040 Send the webhook from an outbox
Start 02:30 KST, end 02:34 (about 3.5 min, of which ~1 min was my own import mistake below).

Read: STEPS.md 0040; board/notify.go, board/notify_test.go, TestDrafts (web_test.go, its webhook part relied on AfterCommit); after `maago add outbox`: outbox/models.go, outbox/outbox.go (package doc with the main example), outbox/queries/*.sql, outbox/driver_test.go; `maago doc main` (outbox section), `maago doc admin`.

Commands and results:
- Maago-Run `../bin/maago add outbox`: wrote outbox/driver_test.go, models.go, outbox.go, outbox_test.go, queries/claim_job.sql, queries/retry_job.sql.
- `maago makemigrations --name outbox`: created migrations/0007_outbox.sql (CREATE TABLE outbox_job ..., index on run_at). `maago migrate`: applied 0007_outbox.sql.
- `maago gen`: exit 1, as the step predicted: "outbox/models.go:16:6: app outbox has models, but no package uses outbox.Admin / fix: add outbox.Admin to admin.Config.Models in main, then run maago gen: go build works after gen".
- Added outbox.Admin to main's admin.Config.Models, plus the worker and board changes; gen: SAME error again. Cause: MY mistake: my scripted edit of main's import block did not match (the block has a blank line before the pgx import), so main referred to outbox.Admin without importing example.com/board/outbox. gen's message stayed "no package uses outbox.Admin" instead of saying the identifier is unresolved / the import is missing, so I spent a few experiments (adding it to the test's adminApp, stripping the worker from main, removing board's outbox imports; all restored) before seeing the import was missing. These experiments were confounded (main never compiled during them), so they say nothing about whether a test file's use of outbox.Admin counts. With the import: gen "generated 15 files", build ok.
- go vet: my syntax slip in TestDrafts (`if x := ...; work(); cond` is not Go), fixed.
- verify: gen (15 files), check, vet, gofmt (empty), go test ./... (auth, board, outbox ok); migrate: nothing to apply.

Changes:
- notify.go: notify/send/retryWait gone; const postPublished = "post_published"; Deliver(ctx, j outbox.Job) error posts j.Payload once to Webhook (read when the job goes) within notifyWithin, with Authorization and Idempotency-Key = j.ID; any non-2xx (4xx too) is an error, so the outbox retries; unknown kind and empty URL are errors (kept, not dropped). (step message + outbox package doc)
- views.go moveTo: when published and Webhook.URL != "", outbox.Enqueue(ctx, tx, postPublished, {"id","key","title","path"}) in the request's transaction, its error returned; AfterCommit there gone, deletePost's AfterCommit kept; comment updated.
- main.go: imports outbox; outbox.Admin in the admin; worker beside Serve exactly as outbox's package doc shows (work/stop context, worked channel, stop() and <-worked after Serve, then log.Fatal on Serve's error).
- Tests: notify_test.go now TestOutbox (replacing TestNotify): no webhook -> publish -> RunOnce finds no job; webhook 503 -> RunOnce takes the job, 1 call, RunOnce at now again finds nothing; webhook up -> outbox.GetJob(took.ID).RunAt -> RunOnce(RunAt) delivers: id, key (draft made with its own key), title, path, same non-empty Idempotency-Key as the first attempt; GetJob afterwards is sql.ErrNoRows. No sleep. TestDrafts: runs outbox.RunOnce(ctx, db, time.Now(), Deliver) after each request instead of relying on AfterCommit. The tests' adminApp() ("main's admin") also gets outbox.Admin.

Guesses: the kind name "post_published" (the step uses an identifier postPublished, not its value); what Deliver does with no URL (error, so the job stays and shows in the admin); replacing TestNotify entirely (the step describes only the outbox test; notify's timeout test went with notify).
Unexpected failures: (1) TestDrafts: the step does not mention it, but its webhook part expected AfterCommit's delivery before the response. I updated it up front; afterwards I checked by disabling the RunOnce calls: it fails "web_test.go:916: publish: 303, heard []". Restored, passes. (2) gen kept saying "no package uses outbox.Admin" when main used outbox.Admin but lacked the import: my error, but the message pointed the wrong way (it could say "main uses outbox.Admin but does not import example.com/board/outbox", or just let go build report it).
Unclear: "maago gen refuses until main gives the admin outbox.Admin, as it did for auth.Admin": clear. "Set its Attempts to 0 to send it again": RunAt also has to have passed (outbox's doc says so; the step does not). The step does not name the job kind's string. "The test runs the worker one step at a time..." does not say which test (new one? TestNotify? the webhook part of TestDrafts, whose draft already exists?), nor what becomes of TestNotify and its notifyWithin test; I wrote a new TestOutbox in notify_test.go replacing TestNotify, and adapted TestDrafts.
Added after review: TestAttachments also checks the image, whose bytes fileMeta read before SaveMedia, downloads whole (passes). verify re-run: all pass.

## Housekeeping
Outside GATE I created only scratch files in my own scratchpad (env.sh, verify.sh helper scripts, a snapshot copy m12gate2-snap0 of board at 0036, and a short-lived experiment dir, removed) and one short-lived /tmp/v.bak (deleted right away). Nothing in the maago repository or the module cache was read or changed. go.mod untouched.

