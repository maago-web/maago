# GATE LOG

## Setup (12:24-12:25 KST, 2026-11-25)
- Wrote $GATE/env.sh. `git init` inside board (local only, to diff each step; *_gen.go excluded).
- Baseline: `maago gen` (generated 15 files), `maago check`, `go vet ./...`, `gofmt -l .`, `go test ./...` all passed at step 0040.
- Read: STEPS.md, main.go, board/models.go, outbox/*.go, board/notify.go, board/export.go, Dockerfile, board/web_test.go (config helper), board/attach_test.go.
- Read `maago doc` (topic list) and `maago doc main`: its example main already shows the one-store pattern (LocalMedia, deliver(db, media) with WithMedia, the daily SweepMedia goroutine, `media.List(ctx, "")` after Handler). Very useful for 0041-0043.
- `go doc` on maago: SweepMedia, LocalMedia (Dir, SiteURL), MediaStore, SaveMediaFrom.

## Step 0041: Sweep the files no row keeps (12:25:54 - 12:26:47)
- Read: STEPS.md 0041, `maago doc main` (the example's sweep goroutine and one-store pattern), `go doc maago.SweepMedia`, board/views.go deletePost.
- Changed:
  - board/models.go: Attachment.Key `maago:"unique,media"`; comment.
  - main.go: `media := maago.LocalMedia{Dir: "media"}`, `Config.Media: media` (MediaDir gone); a goroutine that runs `maago.SweepMedia(maago.WithMedia(work, media), db, 0)` at start and every 24h until `work` ends, error to log (copied the shape of the doc example).
  - board/web_test.go: config uses `Media: maago.LocalMedia{Dir: t.TempDir()}`; helper `mediaDir(cfg)` reads the dir back (type assertion); attach_test.go uses it instead of cfg.MediaDir.
  - board/attach_test.go: TestSweep (upload, SaveMediaFrom a stray file, os.Chtimes both to 48h ago, SweepMedia(ctx,db,0), attachment downloads, stray gone).
  - board/views.go: deletePost comment mentions the sweep.
- Commands: gen, check, vet, gofmt, test: all passed first try.
- Sanity check: removed the media tag and reran TestSweep: it fails with "maago: SweepMedia knows no field that keeps media keys: tag each with media, ... run maago gen, and import the project's apps in this program" (a clear message). Restored.
- Guess: "makes both two days old" = os.Chtimes on the files in the LocalMedia dir (the sweep goes by the file's mod time, it seems; the test passing with the stray deleted confirms it).

## Step 0042: Export a board in the background (12:27:02 - 12:29:51)
- Read: STEPS.md 0042; board/views.go, urls.go, notify.go(+test), export.go, permissions_test.go, web_test.go (outbox part), policies.sql, export_rows.sql; auth/auth.go (RequireStaff).
  Docs: `maago doc model` (media tag, unique=(A,B)), `maago doc query` (headers, policies, :exec), `maago doc view` (signatures, w/r views, ServeMedia, permission table), `go doc maago.WithTx`, `maago.DB`, `maago.Media`, `maago.ServeMedia`. The `maago doc main` example's `deliver(db, media)` was the template for main's deliver.
- Changed:
  - board/models.go: model ExportFile (BoardID fk, Name max=200, Key `unique,media`, Type, Size, CreatedAt), unique=(BoardID,Name); media() helper.
  - `maago makemigrations --name export_files` -> migrations/0008_export_files.sql (table board_export_file, as the step says).
  - queries: export_posts.sql (policy out, paged by id with exportPage), board_export_files.sql, export_file_by_name.sql, delete_export_files.sql (:exec).
  - board/files.go: const ExportFiles="export_files"; WriteExportFiles(ctx, db, j) reads posts with ExportPosts(ctx, nil, ...) (nobody: out to everybody), SaveMediaFrom each post-<id>.md then index.md, then maago.WithTx: DeleteExportFiles + InsertExportFile for each. No file deletion. Views BoardFiles (GET), ExportBoardFiles (POST, enqueues in request tx), DownloadBoardFile (w/r view, ServeMedia), all auth.RequireStaff.
  - board/urls.go: board_files GET /boards/{id}/files, board_files_export POST same path, board_file GET /boards/{id}/files/{name}. Template board_files_page.html.
  - board/notify.go: postPublished -> PostPublished; Deliver no longer checks the kind. notify_test.go: removed the "another kind" case (Deliver does not ask any more), kept "no answer at all".
  - main.go: deliver(db, media) puts the store in ctx with WithMedia, switches on Kind (PostPublished -> board.Deliver, ExportFiles -> board.WriteExportFiles), unknown kind -> error.
  - tests: board/files_test.go TestWriteExportFiles (exportPage=1, job twice, Chtimes all to 48h ago, sweep, store == rows' keys, contents checked, draft in none) and TestFiles (staff POST export, outbox.RunOnce with a by-kind handler, list page, index.md by name, 404 for unknown name). TestPermissions now builds cfg itself (needs the store to write an export for board_file's 200), plus the three rows.
- Commands: gen, check, vet, gofmt, test all passed on the first run. No failures.
- Guesses: file names post-<id>.md and index.md; the Markdown is "# Title\n\nBody\n" and the index "# Board\n\n- [Title](post-N.md)" lines; payload {"board_id": N}; WriteExportFiles takes the outbox.Job (like Deliver) rather than a board id; the three routes are staff-only via auth.RequireStaff inside views taking u auth.User (so nobody gets 303/401, others 403); no link to board_files from the board page (non-staff would see a link to a 403).
- Note: `maago gen`/`check` did not complain about the new routes lacking permission rows; only TestPermissions would have (the board's own test comment says so).
- Unclear: whether TestFiles should call main's deliver; it cannot from package board, so the test hands RunOnce a small by-kind function.

## Step 0043: Keep the files in a bucket (12:30:12 - 12:31:16)
- Read: STEPS.md 0043; `go doc -all github.com/maago-web/maago/contrib/s3` (Store fields, "each call checks the fields first ... naming it"); `maago doc main` again (check `media.List(ctx, "")` after Handler).
- Changed:
  - main.go: `mediaStore()` returns s3.Store from BOARD_S3_{BUCKET,ENDPOINT,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY} when BOARD_S3_BUCKET is set, else LocalMedia{Dir:"media"}, plus "bucket NAME"/"media/" for the log. After Handler: `media.List(ctx, "")`, log.Fatal on error, then `log.Printf("files in %s", where)`. Same store already goes to Config.Media, deliver and the sweep (from 0041/0042).
  - Also fixed my own 0041 slip: the store block had been put between the admin's comment and adminApp; moved it above.
  - main_test.go (new): TestMediaStore: no bucket -> LocalMedia media/; bucket with empty endpoint -> the s3.Store from env, and List's error contains "Store.Endpoint is empty" (no network needed).
  - Dockerfile: comment only; the volume stays (files there with no bucket).
- Commands: `go test .` passed; full gen/check/vet/gofmt/test passed. No failures.
- Smoke run of the built binary on the m13gate1 database (applied 0001-0008 there): with BOARD_S3_BUCKET=x and no endpoint it stops with `s3: Store.Endpoint is empty`; without a bucket it logs `files in media/`, then `listening on ...`, and stops on SIGTERM.
- Not tested: a real bucket (no S3/MinIO here), so "a wrong key or a bucket that is not there stops it" is taken from the s3 package doc.
- Guess: "logs which store it is" format `files in bucket NAME` / `files in media/`, taken literally from the step.

## Step 0044: Hand the files out by signed URLs (12:31:35 - 12:32:46)
- Read: STEPS.md 0044; `go doc maago.MediaURL` ("A LocalMedia's needs this process's Handler first, in a test too"), `go doc maago.LocalMedia.URL` (route under SiteURL and the last Handler's Prefix; signing key is the process's own), `go doc maago.Config` (Media: Handler serves a LocalMedia's URLs at /maago/media). Grepped SiteURL use in the app (auth tests set auth.SiteURL; board tests do not).
- Changed:
  - board/files.go: FileLink{Name, Type, Size, URL} (no json tags, like the other API outputs); APIBoardFiles(ctx, u auth.User, db, BoardFilesIn): RequireStaff, GetBoard (404), BoardExportFiles, then maago.MediaURL(ctx, f.media(), time.Hour) for each (const fileURLFor). Returns [] not null when empty.
  - board/urls.go: api_board_files GET /api/boards/{id}/files, maago.API.
  - main.go: mediaStore(auth.SiteURL): LocalMedia{Dir: "media", SiteURL: siteURL}; comments. (auth.SiteURL is set and checked before the store is made.)
  - main_test.go: passes a site URL, expects it in the LocalMedia.
  - board/web_test.go: config's store has SiteURL "https://example.com".
  - board/files_test.go: TestFileURLs: httptest.NewServer wrapping a handler variable, store SiteURL = srv.URL, Handler built after; staff token via auth.SignInToken; real HTTP GET of the list with Bearer; download links[0].URL with plain http.Get (no sign-in): 200 and content; board id+1 -> 404.
  - board/permissions_test.go: row api_board_files want(401, 403, 403, 403, 200).
- Commands: gen/check/vet/gofmt/test passed first run (and again with -count=1).
- Probe (temporary test, deleted): Handler with LocalMedia.SiteURL "https://example.com/app" refuses with: `maago: LocalMedia.SiteURL "https://example.com/app" is not a site's address such as https://example.com: http or https and a host, no path (Config.Prefix is added), user, query or fragment` - clear.
- Guess: the JSON keys are the Go field names (Name, Type, Size, URL), as PostSummary's are.

### After review (12:33 - 12:35)
- Added main_test.go TestDeliverUnknownKind (deliver(nil, nil) on Kind "nope" returns an error): the case removed from notify_test.go in 0042 now lives in main's deliver. Five checks passed again (-count=1); folded into the 0044 commit of my local git.
- Smoke run of the 0044 binary (BOARD_SITE_URL=http://localhost:18321, no bucket): Handler accepts the LocalMedia SiteURL, logs `files in media/`, GET /app/ answers 200, SIGTERM stops it.
- `git diff 27663f2 HEAD -- media go.mod go.sum`: empty (the smoke runs' sweep touched nothing; media/ holds only its .gitignore; go.mod still requires the published maago v0.4.0).
- No test calls t.Parallel, so the process-wide "last Handler's Prefix" (web_test builds one with /app) cannot race TestFileURLs.

## Failures (all of them)
- Orientation: `grep -rn ... --include=*.go` in zsh: `(eval):1: no matches found: --include=*.go` (zsh glob); fixed by quoting `--include='*.go'`.
- Deliberate, to check a test: removing the media tag made TestSweep fail with the SweepMedia message quoted under 0041.
- Otherwise none from maago gen/check, go vet, gofmt or go test in any step.

## Summary
All four steps done; each ended with maago gen, maago check, go vet ./..., gofmt -l . (empty) and go test ./... passing. The only fix was to my own comment placement in main.go (done in 0043).
Local artifacts of mine: board/.git (a local repo, commits base/0041-0044, *_gen.go excluded via .git/info/exclude) and the generated *_gen.go files; $GATE/home and $GATE/cache from env.sh.
