# GATE-LOG

## Setup (about 12:24-12:26; the first command printed 12:24:56, later times from `date +%H:%M`)
- Wrote env.sh. Read STEPS.md, main.go, board/*.go, outbox/*.go.
- Baseline at 0040: `maago gen` (15 files), check, vet, gofmt, go test: all pass.

## Step 0041: Sweep the files no row keeps
- Start: 12:26
- Read: `maago doc` (topic list), `maago doc model` (media tag), `go doc maago SweepMedia/WithMedia/LocalMedia/MediaStore/SaveMediaFrom`, `go doc maago Config` (MediaDir/Media).
- Changed:
  - board/models.go: Attachment.Key tag `maago:"unique,media"`.
  - main.go: `media := maago.LocalMedia{Dir: "media"}` given to Config.Media (MediaDir removed); worker ctx is `maago.WithMedia(work, media)`; new `sweep(ctx, db)` goroutine runs `maago.SweepMedia(ctx, db, 0)` at start and every 24h, logs errors, stops with the worker ctx; main waits for it after the worker.
  - board/web_test.go: config uses `Media: maago.LocalMedia{Dir: t.TempDir()}`; helper `mediaDir(cfg)` reads the dir back (type assertion). attach_test.go uses it in place of cfg.MediaDir.
  - board/attach_test.go: TestSweep (upload, SaveMediaFrom a lost file, os.Chtimes both to -48h, SweepMedia, attachment downloads, lost file gone).
- Commands: gen, check, vet, gofmt -l, go test ./...: all pass first try.
- Failure (my shell, not the app): `grep -rn ... --include=*_test.go .` -> `(eval):1: no matches found: --include=*_test.go` (zsh globbing); fixed by quoting the pattern.
- Sanity check: removed the media tag and ran TestSweep: it fails with "maago: SweepMedia knows no field that keeps media keys: tag each with media, such as Key string `maago:"media"`, run maago gen, and import the project's apps in this program" (a clear message). Restored.
- Guesses: "two days old" done with os.Chtimes on the files in the LocalMedia dir (assumed SweepMedia goes by List's ModTime; it worked). olderThan 0 = 24h per go doc. The sweep is waited for at shutdown like the worker (not stated, seemed right).
- Unclear: nothing important. The step text says "reads its directory back"; I read it as a helper doing a type assertion on cfg.Media.
- End: 12:26

## Step 0042: Export a board in the background
- Start: 12:27
- Read: `maago doc query` (exec kind, policies, -- policy: out p), `maago doc view` (routes, w/r views, ServeMedia), `maago doc main` (its example main has deliver(db, media) switching on j.Kind and a sweep loop: matched my 0041 main closely), `go doc maago WithTx/ServeMedia/Media/OpenMedia`, auth/auth.go (auth.RequireStaff exists), board/permissions_test.go, notify_test.go, templates.
- Changed:
  - board/models.go: model ExportFile {ID, BoardID fk=Board, Name max=200, Key unique,media, Type, Size, CreatedAt}, unique=(BoardID,Name); method media().
  - `maago makemigrations --name export_files` -> migrations/0008_export_files.sql (table board_export_file, as the step said).
  - Queries: export_posts.sql (ExportPosts, policy out, paged by after/limit), board_export_files.sql, export_file_by_name.sql, delete_export_files.sql (:exec).
  - board/notify.go: postPublished -> exported PostPublished; Deliver no longer checks the kind. views.go uses PostPublished. notify_test.go: dropped the "another kind" case (Deliver no longer knows kinds); kept the "no server" case with its own closed server.
  - board/export.go: const ExportFiles = "export_files"; payload {"board_id": N}; WriteExportFiles(ctx, db, j outbox.Job): ExportPosts with u=nil (out to everybody), pages of exportPage, SaveMediaFrom each post-ID.md then index.md, then maago.WithTx: DeleteExportFiles + InsertExportFile for each. Views BoardFiles (GET), ExportBoardFiles (POST, outbox.Enqueue in request tx, 303 to board_files), DownloadBoardFile (w/r view, ServeMedia), all auth.RequireStaff first.
  - board/urls.go: GET /boards/{id}/files board_files, POST /boards/{id}/files board_files_export, GET /boards/{id}/files/{name} board_file. Template board_files_page.html.
  - main.go: deliver(db, media) closure: WithMedia, switch Kind {PostPublished: board.Deliver; ExportFiles: board.WriteExportFiles}, unknown kind -> error. outbox.Run uses it.
  - Tests: new board/files_test.go (TestWriteExportFiles, TestFiles); permissions_test.go now builds its handler from config(t, db) to reach cfg.Media, keeps an index.md ExportFile, and has rows for the three routes.
- Commands/failures:
  - First `go test` after adding routes: TestPermissions failed: "maagotest.Permissions: route board_files (GET /boards/{id}/files) has no row; add one with each role's status" (and the same for board_files_export, board_file). Expected; fixed by adding the rows (message told exactly what).
  - Note: `maago gen`/`go vet` did NOT notice main.go still calling board.Deliver for every kind after I removed Deliver's kind check; only my reading caught it.
  - Then all passed: gen, check, vet, gofmt -l (empty), go test -count=1.
- Guesses: payload JSON shape {"board_id"}; file names post-<id>.md and index.md; Markdown content "# Title\n\nBody\n" and index "- [Title](post-ID.md) date"; WriteExportFiles takes the outbox.Job (parses the payload itself) so main's case and the test's handle are one line; board_files_export is a POST on the same path as board_files; the page size reuses exportPage (the CSV's), set to 1 in the test for "a post a page"; non-staff signed-in users get 403 (RequireStaff), nobody gets the login redirect/401.
- Unclear: "TestFiles ... runs the job as main does with outbox.RunOnce, by its kind": main's deliver is in package main and the test is in package board, so the test writes its own small handle that checks the kind. The step said nothing about whether a board deletion should take its export rows (fk NO ACTION); left alone, no code deletes boards.
- End: 12:29

## Step 0043: Keep the files in a bucket
- Start: 12:29
- Read: `go doc maago/contrib/s3` and `go doc .../s3 Store` (fields Endpoint, Region, Bucket, AccessKeyID, SecretAccessKey), `maago doc main` example main (checks `media.List(ctx, "")` after Handler), Dockerfile.
- Changed:
  - main.go: `var media maago.MediaStore = maago.LocalMedia{Dir: "media"}`; with BOARD_S3_BUCKET set, `s3.Store{...}` from BOARD_S3_ENDPOINT/REGION/ACCESS_KEY_ID/SECRET_ACCESS_KEY. Same store to Config.Media, the worker and the sweep (already). After Handler: `media.List(ctx, "")` -> log.Fatal on error; then `log.Printf("files in %s", where)` ("bucket NAME" or "media/"). Comment on one bucket per site/database.
  - Dockerfile: comment only (volume is for the no-bucket case); VOLUME kept.
- Failure (my shell): first manual run used `timeout 10 ...` -> `(eval):59: command not found: timeout`, exit 127, twice (macOS has none); replaced with running in the background, sleep, kill.
- Commands: built the binary to the scratch gate dir and ran it against the m13gate2 DB (this applied migrations 0001-0008 there):
  - BOARD_S3_BUCKET=x only -> "s3: Store.Endpoint is empty", exit 1 (as the step said).
  - all fields, bucket "x" -> "s3: Store.Bucket "x" is not a bucket's name: 3 to 63 of a-z, 0-9, . and -, ...", exit 1.
  - bucket board-files, endpoint http://127.0.0.1:1 -> 'Get "http://127.0.0.1:1/board-files?list-type=2": dial tcp ...: connection refused', exit 1 (a down bucket stops the start).
  - no bucket -> "files in media/", "listening ...", clean shutdown on kill.
  - gen, check, vet, gofmt -l, go test -count=1: all pass, no failures in this step.
- Guesses: no automated test for the env switch (the step names none, and a real bucket is not available); manual runs above instead. The log line is printed after the List check, so a bad bucket stops before it says which store.
- Unclear: whether the "files in ..." line should come before the check (to show a misspelt bucket name even when List fails). The fatal error from List does not name the bucket for a NoSuchBucket in all cases I could not test (no S3 here); I put it after, as the step's order reads.
- End: 12:31

## Step 0044: Hand the files out by signed URLs
- Start: 12:31
- Read: `go doc maago MediaURL`, `go doc maago.LocalMedia.URL`, `go doc maago Config` (Prefix: goes before a LocalMedia's URLs; last Handler sets it), auth/auth.go (SiteURL).
- Changed:
  - board/export.go: type APIBoardFile {Name, Type, Size, URL}; view APIBoardFiles(ctx, u auth.User, db, in ExportIn): RequireStaff, GetBoard (404), BoardExportFiles, then maago.MediaURL(ctx, f.media(), time.Hour) for each (after the check). Returns [] not null for none.
  - board/urls.go: GET /api/boards/{id}/files "api_board_files", maago.API.
  - main.go: `maago.LocalMedia{Dir: "media", SiteURL: auth.SiteURL}` (auth.SiteURL is set earlier in main) with a comment.
  - board/web_test.go: the tests' store has SiteURL "http://example.com".
  - board/permissions_test.go: row api_board_files want(401, 403, 403, 403, 200).
  - board/files_test.go: TestFileURLs: httptest.NewServer wrapping a handler var, cfg.Media swapped to LocalMedia{Dir: mediaDir(cfg), SiteURL: srv.URL} before maago.Handler; staff's API token; list -> 2 files, index.md first; http.Get(URL) with no sign-in -> 200, body matches, length = Size; board+1000 -> 404.
- Commands: gen, check, vet, gofmt -l, go test -count=1: all pass on the first try.
- Extra manual check: ran the binary with BOARD_SITE_URL=http://localhost:8123/app -> "maago: LocalMedia.SiteURL "http://localhost:8123/app" is not a site's address such as https://example.com: http or https and a host, no path (Config.Prefix is added), user, query or fragment", exit 1 (the step's "Handler refuses a SiteURL with a path"). With a trailing slash it is accepted.
- Guesses: JSON field names are the Go names (Name, Type, Size, URL), as the step lists them and as the other API answers (Post) go without json tags; the test's server-URL trick (handler var captured by the httptest server) to make the store's SiteURL the server's address.
- Unclear: whether the config's default SiteURL should be some fixed value; I used http://example.com, only TestFileURLs follows its URLs.
- End: 12:32

## Side effects of the manual runs
Each run of the binary used the m13gate2 database (migrations 0001-0008 applied there by the first run), polled its outbox, and the runs that got past the store check ran the real SweepMedia over board/media. `ls -la board/media` afterwards: only `.gitignore` (12:10 mtime, under a day old, so the sweep kept it); unchanged.

## Summary
All four steps done; at the end of each, maago gen, maago check, go vet ./..., gofmt -l . (empty) and go test ./... passed.

Approximate durations (minute clock): setup 2 min, 0041 about 1 min, 0042 about 3 min, 0043 about 1-2 min, 0044 about 2 min.

Where the step text, docs or messages fell short:
- `maago doc` has no media topic; the media story came from `go doc` (SweepMedia, WithMedia, LocalMedia, MediaStore, MediaURL, Config.Media) and the example main in `maago doc main`, which was the most useful single source.
- 0042 "TestFiles runs the job as main does": main's deliver is in package main, the test in package board, so the test re-implements the kind switch; main's deliver and sweep loop are only covered by compiling and booting.
- 0042: removing Deliver's kind check left main routing every kind to it; no tool (gen, check, vet, tests) flagged it.
- 0043: the bucket path could only be exercised for an empty field, a bad bucket name and a refused connection; no S3 to test against.
- Guessed shapes: export file names and Markdown layout, the job payload {"board_id"}, the route paths, JSON field names of the API, the tests' default SiteURL.
