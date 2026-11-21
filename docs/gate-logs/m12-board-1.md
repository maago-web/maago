# GATE-LOG: board steps 0037-0040

Setup (02:24): copied board/ to board.orig for diffing. Env in GATE/env.sh. Generated files (*_gen.go) were not in the tree; `../bin/maago gen` wrote 12 files. Baseline: check, vet, gofmt, `go test ./...` all pass (~4s). `showmigrations`: 0 of 5 applied in m12gate1 DB (not migrated yet; left alone until a step adds a migration).

## 0037 Draw both forms, and ask as an API client
Start: 02:24. End: 02:26.
Read: step message; board/views.go, forms.go, templates/post_page.html, attach_test.go, permissions_test.go, web_test.go (TestAPI, TestMe); board/forms_gen.go (NewAttachmentForm/NewCommentForm exist after gen); auth/auth.go (SignInToken already in the copied auth); `go doc .../maagotest` (Bearer exists).
Changes (all from the step message):
- views.go: postPage sets AttachForm: NewAttachmentForm(); Attach redraws with NewCommentForm().
- post_page.html: file field drawn from .AttachForm.File with label and name, as the step quotes.
- permissions_test.go: role cat_token (maagotest.Bearer with auth.SignInToken for cat@example.com) after bob, before ann; every row got cat's code = bob's code; bob's own write rows got skip for cat plus a cat row ("cat의 게시판", "cat의 글", "cat의 API 글"). Done with a script over Want: want(...).
- web_test.go: TestAPI and TestMe use auth.SignInToken.
- attach_test.go: checks `<label>파일 <input type="file" name="file"></label>` on the page and `<textarea name="text">` after an attach with no file. Verified it fails with CommentForm{} put back.
Commands: go vet ./board ok; go test -run 'TestPermissions|TestAPI|TestMe' ok first try (token requests to every route, POSTs included, gave bob's codes); TestAttachments ok; gen/check/vet/gofmt/test all ok.
Guesses: none needed. The cat column matched bob's on every row without exceptions (CSRF does not block token POSTs).
Unexpected failures: none.
Unclear: nothing.

## 0038 Type the views that write the response themselves
Start: 02:26. End: 02:27.
Read: step message; board/export.go, views.go (DownloadAttachment), urls.go, web_test.go (TestExport, the policy test at ~864), attach_test.go; `maago doc view` (section "A view that answers with something other than a page ... takes w and r after In, and returns only an error").
Changes:
- export.go: ExportIn{ID int64 path=id}; ExportBoard typed (u *auth.User, db, in, w, r) error; returns GetBoard/ExportRows/csv errors; headers and the CSV header line set once the first page is read; abort() and the log/errors/sql imports removed (step message).
- views.go: DownloadIn; DownloadAttachment typed, returns GetAttachment/GetPost/ServeMedia errors; the ServeMedia switch is gone; unused imports (database/sql, io/fs, strconv) removed (go build told me).
- main.go: OnError comment says it hears a self-writing view's error after it wrote (step message).
- tests: BoardExportURL(b.ID), AttachmentURL(a.ID) (sed over fmt.Sprint(...)); "/boards/abc/posts.csv" for the non-number id; TestAttachments renames the file away and wants 404. Mutation check: ServeMedia's error dropped -> test fails with 200 (as the step says).
Commands/results: `go build` first failed with "not enough arguments in call to ExportBoard" in views_gen.go (expected: gen not run yet) plus unused imports; `maago gen` fixed the first. vet: "fmt imported and not used" in attach_test.go, removed. Then gen/check/vet/gofmt/test all ok.
Slip (mine): while doing the mutation check I copied views.go to /tmp/v.go as a backup, outside GATE; removed it right after. No other effect.
Guesses: "the first page is read" taken as: after ExportRows returns the first page without error (also for an empty board). No test asserts that a 500 has no Content-Disposition; I did not add one (step did not ask).
Unexpected failures: none (build errors before gen are expected).
Unclear: nothing.

## 0039 Give each post a key, and each image its size
Start: 02:27. End: 02:30.
Read: step message; board/models.go, policies.sql, queries (post_list, search_posts, recent_posts, post_attachments, post_detail); `maago doc model` (UUID, JSON[T], "no default: ... a pointer"), `maago doc query` (policies, :one Model needs every column); `go doc` maago NewUUID, JSON, File, SaveMedia; generated NewAttachmentInput / PostByKeyParams.
Changes:
- models.go: Post.Key *maago.UUID `maago:"unique,admin=readonly"`; Attachment.Meta *maago.JSON[AttachmentMeta]; type AttachmentMeta{Width, Height int} (no json tags: Go names, as the step says).
- `maago gen` -> "migrations do not match models: column key/meta missing; fix: run maago makemigrations" (expected). `maago makemigrations --name post_key_attachment_meta` -> 0006 (ADD COLUMN meta JSONB; key UUID; UNIQUE), no warning, as the step says.
- `maago gen` -> "result columns do not match model Attachment: missing "meta"" (post_attachments.sql) and "...Post: missing "key"" (recent_posts.sql), exactly as the step predicted. Added the columns to the explicit lists.
- post_list.sql, search_posts.sql select p.key (PostSummary gets Key *maago.UUID).
- new queries/post_by_key.sql: `PostByKey :one Post`, param key maago.UUID, policy visible p, SELECT * ... WHERE p.key = :key AND visible(p). Generated PostByKeyParams{Key, Now} as the step quotes.
- urls.go: GET /api/posts/{key} "api_post" APIPost maago.API. views.go: APIPostIn{Key maago.UUID path=key}; APIPost calls PostByKey with time.Now().
- savePost: Key: &key with key := maago.NewUUID().
- Attach: fileMeta(f) opens the maago.File, image.DecodeConfig (gif/jpeg/png registered with blank imports), before SaveMedia; Meta into InsertAttachment; nil when not an image.
- post_page.html: `{{with .Meta}} · {{.V.Width}}×{{.V.Height}}{{end}}` in the attachment line.
- tests: go test -> "maagotest.Permissions: route api_post (GET /api/posts/{key}) has no row" (expected by the table's design; step did not mention it). Gave out and unseen keys (UpdatePost) and added two api_post rows (200 all; draft 404/404/404/200/200). TestAPI: the API's post has a key; by key 404 without token, 200 with ann's token incl. "Key":"<uuid>"; random UUID and "/api/posts/abc" 404; the list has the key once published. TestAttachments: 3x2 PNG -> Meta {3,2}, a.txt Meta nil, page shows "3×2", download of the PNG is byte-identical (checks that Open before SaveMedia does not consume the file).
- `maago migrate`: applied 0001..0006 (the dev DB had none applied before).
All of gen/check/vet/gofmt/test ok.
Guesses: (1) where to put the image read: a helper fileMeta in views.go; on Open error return the error, on a decode error nil (not an image). (2) Tested the read-by-key with ann's token rather than a session. (3) Did not add a form-route test for the key; savePost is shared by form and API, the API test checks it.
Unexpected failures: the permission table's missing api_post row (minor; the table says "A route added without its row fails here").
Unclear: the step does not say the new route needs a row in TestPermissions, nor which keyed fixtures to give the table (existing InsertPost fixtures have no key, so APIPostURL needs posts given a key). Also "A template reads the value as .V" - only gives the inline snippet; I added a " · " separator myself.

## 0040 Send the webhook from an outbox
Start: 02:30. End: 02:33.
Read: step message; `maago doc main` (outbox section and example main with outbox.Run/stop/worked); board/notify.go, notify_test.go, TestDrafts' webhook part in web_test.go; after `add outbox`: `go doc ./outbox`, outbox/models.go, outbox.go, queries, outbox_test.go (head); `maago doc admin` (Models).
Commands:
- Maago-Run `../bin/maago add outbox` -> wrote outbox/{driver_test.go,models.go,outbox.go,outbox_test.go,queries/claim_job.sql,queries/retry_job.sql}, rc 0. (My shell pipeline after it ended in `head -0`, an error of mine, not maago's.)
- `../bin/maago makemigrations --name outbox` -> migrations/0007_outbox.sql (CREATE TABLE outbox_job + run_at index).
- `../bin/maago gen` -> "app outbox has models, but no package uses outbox.Admin; fix: add outbox.Admin to admin.Config.Models in main" - as the step predicted.
Changes:
- main.go: outbox.Admin in admin.Config.Models; worker beside Serve exactly as the doc's main: work/stop context, outbox.Run(work, db, board.Deliver) in a goroutine, stop() and <-worked after Serve, then log.Fatal on Serve's error.
- notify.go: notify/send/retryWait gone; const postPublished = "post_published"; Deliver(ctx, outbox.Job) error posts j.Payload once to Webhook within notifyWithin, Authorization Bearer token, Idempotency-Key = job id; any non-2xx or no answer is an error (4xx too); unknown kind is an error (copied from the doc's deliver).
- views.go moveTo: on "published" and Webhook.URL != "", outbox.Enqueue(ctx, tx, postPublished, map{id,key,title,path}) as the step quotes; AfterCommit there removed; deletePost's AfterCommit kept.
- notify_test.go: TestDeliver (200 ok; 400 and 503 errors with one call each; headers; no server; other kind; notifyWithin timeout).
- web_test.go TestDrafts: draft gets its own key; with Webhook empty, a post written and published via the form leaves no job (RunOnce found=false); webhook 503: RunOnce(now) takes and fails it, RunOnce(now) again finds nothing; job read by id (from the Idempotency-Key) with outbox.GetJob, LastError set; webhook up, RunOnce(j.RunAt) -> webhook heard id, key (= draftKey), title, path, same Idempotency-Key; GetJob -> sql.ErrNoRows; archiving writes no job.
Failures:
- TestDrafts "the webhook down: heard []": I had taken `now := time.Now()` before publishing, so the job's RunAt (set at Enqueue) was after `now`, not due. Fixed by stepping at time.Now() after the publish. (My test bug.)
- While fixing that with sed I deleted another `now := time.Now()` in newUser -> vet "undefined: now"; restored. (My bug.)
- Mutation checks: dropping the `Webhook.URL != ""` guard fails "publish with no webhook"; dropping "key" from the payload fails the heard check. Both restored.
Then gen/check/vet/gofmt/test all ok (outbox's own tests pass too); `maago migrate` applied 0007_outbox.sql (7 of 7).
Guesses: (1) the kind's name, "post_published" (step only names the Go identifier postPublished). (2) Deliver refuses unknown kinds (from the doc's example). (3) The job's id for GetJob comes from the Idempotency-Key header, since outbox has no "list jobs" query. (4) Added outbox.Admin to the tests' adminApp() too, since its comment says it is main's admin (gen did not ask for it); all checks still pass, no new permission rows needed.
Unexpected failures: none from maago; two of my own test mistakes above.
Unclear: "at the job's RunAt, read from the database": the step doesn't say how the test finds the job (no query lists jobs); the Idempotency-Key is the only handle. The step's "The test" doesn't name which test; I used TestDrafts, where the old webhook check was, and replaced TestNotify with TestDeliver.

## Summary
Reached the end of 0040. Times: 0037 ~2 min, 0038 ~1.5 min, 0039 ~3 min, 0040 ~3 min (02:24-02:33).
