# Gate log: steps 0034-0036

Environment: GOWORK=off GOFLAGS=-mod=mod, GOPATH/GOMODCACHE as given. Copied board/ to board-orig/ (inside m11gate2) as a diff baseline; deleted it at the end (it was outside board/ and held a copy of .env).

## Setup (14:01)
- `../bin/maago migrate` -> applied 0001..0005. `maago gen` (generated 12 files), `maago check`, `go vet ./...`, `go test ./...` all pass, `gofmt -l .` empty.
- Read: STEPS.md, board/board/{models,forms,views}.go, queries/*.sql, web_test.go, `maago doc`, `maago doc form`, `go doc maago UniqueError`, `go doc maago Messages`, `go doc maagotest DB`.

## 0034: Let the database refuse what is taken (start about 14:02)
Commands: `rm board/queries/{board_name_taken,post_title_taken}.sql`; edits; `maago gen`, `maago check`, `go vet ./...`, `go test ./...`, `gofmt -l .`; `go test ./board -run TestWriteRace -v -count=3` with a temporary t.Log to confirm the request really waited at the index (it did, all 3 runs; log line removed).

Changes (all from the message unless noted):
- Deleted board_name_taken.sql, post_title_taken.sql (and so skip_id?).
- CreateBoard: InsertBoard, then `if f.Fail(err) { Show }`, then `if err != nil` (the message's snippet verbatim).
- savePost: PinnedOverlap first, then InsertPost, then `if f.Fail(err) { return Post{}, false, nil }`. APICreatePost unchanged (its `f.Title.Error != ""` branch now carries Fail's message). Comment rewritten.
- ChangePost: removed the PostTitleTaken read; `case f.Fail(err):` after `maago.ErrConflict`.
- Tests: the four places that saw the board's own taken messages now expect "This value is already taken." (TestWrite name+title, TestAPI 422, TestEdit). Added a case to TestPinAndSchedule: title "공지" (taken) + overlapping pin -> pin's error, not the taken one (message: "a post whose title and pin both clash shows the pin's error"). Added TestWriteRace: tx inserts board "경주" and holds; a request posts the same name in a goroutine; poll pg_stat_activity (wait_event_type='Lock', INSERT INTO "board_board") up to 1s; commit; request is 200 with the message; one board row.
- Guess: TestWriteRace's "waiting at the unique index" check via pg_stat_activity is my own idea (PG-only; this project's tests are PG). The message did not say how to tell the request is waiting.

Failures: none. Everything passed on the first run.

Unclear / gaps:
- `maago doc form` describes Fail well (UniqueError, Messages.Taken, field-or-form, transaction goes on), but its example program further down still uses the old pattern (`titleTaken` read before insert, `maago.Invalid(&f.Title, "A post has this title.")`). That contradicts the paragraph above it.
- The doc says an API view "takes the error with errors.As and returns maago.Invalid". The step says APICreatePost "stays as it is" because savePost's Fail wrote f.Title.Error; both work, the doc does not mention reusing Fail in an API view.
- `go doc maago` for Form's Fail: Fail is generated per form, so it is not in go doc of maago; only `maago doc form` and the generated forms_gen.go tell its signature. Fine, but worth a line in go doc of UniqueError ("see the generated Fail").
- The message says "TestWriteRace sends two at once" but not which route; I used board_create (the name).

End about 14:04 (about 2-3 min; times from `date` at the start and end of the session, the per-step split is approximate).

## 0035: Write who may read a post (start about 14:04)
Read `maago doc query` (rule 6, the Read policies section, header lines of policies.sql, the example).

Changes:
- board/board/policies.sql: the two blocks of the message verbatim.
- post_list, search_posts, export_rows: `-- policy: out p`, the three copied conditions replaced by `out(p)` as an AND term of the WHERE; removed their own `-- param: now time.Time` (the message: "The policies' :now joins each query's Params"; doc: "the query may declare one too, of the same type"). post_list still uses :now in ORDER BY (pinned now) - guess: the policy's now serves it.
- my_posts: `-- policy: visible p`, `WHERE p.author_id = :author_id AND visible(p)` (dropped its own `NOT p.hidden`, which visible has).
- recent_posts: `-- policy: none board_post for the tests, which see every post they wrote` (why text is my wording).
- pinned_overlap: `-- policy: none board_post a count of the notices pinned at once, drafts too, which shows no post` (verbatim).
- post_detail.sql left as it was.

`../bin/maago gen` (exit 1):
```
board/queries/post_detail.sql:5:6: query reads board_post p without a policy (rule 6)
  fix: declare -- policy: out p and write out(p) in the WHERE of the SELECT that reads it (policies of Post: out, visible); or -- policy: none p why, if it may read every row
  see: maago doc query
```
First line matches Maago-Expect exactly. Checks not run (the step must fail at gen).

Unclear: the message says "board/policies.sql, beside queries/" - in this project the app is board/board/, so the file is board/board/policies.sql (doc: "policies.sql of its app"). Clear enough with the doc.

End about 14:05 (about 1 min).

## 0036: Read posts under the policies (start about 14:05)
Commands: edit post_detail.sql; `maago gen`; `go vet ./...`; view edits; `maago gen`; test edits; `go vet`, `maago check`, `go test ./...`; table edits; `maago gen && maago check && go vet ./... && go test -count=1 ./...`; `gofmt -l .`.

Changes:
- post_detail.sql: `-- policy: visible p`, `WHERE p.id = :id AND visible(p)` (message), its comment line updated.
- First `maago gen` after that: exit 1, no files written; it type-checked the views against the new signatures and listed every call site, e.g.
  `board/export.go:43:108: not enough arguments in call to ExportRows; have (context.Context, *sql.DB, ExportRowsParams); want (context.Context, *"example.com/board/auth".User, maago.DB, ExportRowsParams)`
  `  fix: go build reports the same error; fix it there`
  (12 such: ExportRows, SearchPosts, PostList x2, GetPost x6, PostDetail, MyPosts). Not surprising, and the list was a useful to-do. Small surprise: gen writes nothing while the views don't compile against what it would write, so queries_gen.go keeps the old signatures meanwhile; `go vet` alone would have shown the old API. The diagnostic made that clear enough.
- views.go (message): ShowBoard and APIPosts take `u *auth.User`, pass it to SearchPosts/PostList. postPage reads PostDetail alone (`PostDetail(ctx, u, db, PostDetailParams{ID: id, Now: time.Now()})`), no GetPost, no maySee. Attach, RemovePost, moveTo, editable: `GetPost(ctx, &u, tx, id, time.Now())`; the Hidden checks of Attach and editable removed. DownloadAttachment: `GetPost(ctx, userFrom(r), db, a.PostID, time.Now())`, ErrNoRows -> 404. Me: `MyPosts(ctx, &u, db, MyPostsParams{AuthorID: u.ID, Now: time.Now()})`. maySee deleted; mayChange stays.
- New helper `userFrom(r) *auth.User` in views.go (the maago.UserFrom code that was inline in DownloadAttachment), since ExportBoard needs it too ("reads it from the request, as DownloadAttachment does"). My choice to share it; two callers.
- export.go: `ExportRows(ctx, userFrom(r), db, ...)`.
- Tests: every `GetPost(ctx, db, ...)` in tests -> `GetPostIgnoringPolicies(ctx, db, ...)` (message: "the tests that read back what they wrote"); queries_test's PostList/SearchPosts/PostDetail get a nil user (they read published posts). The admin needed nothing from me (generated).
- First `go test`: TestPermissions failed with exactly the six role-cells of the message's four rows:
  `post_publish POST /posts/2/publish as bob: 404, want 403` (and /posts/3 bob, ann; /api/posts/4 bob; /api/posts/5 bob, ann). Updated those four rows to 404 and the comment above them. All other rows kept their statuses, as the message said.
- Added to TestHiddenAttachments: after ann hides her post, POST archive and delete of it as ann are 404 (message: "they no longer publish, archive or delete it either"). Guess: this is the behaviour change that had no test yet.

Result: `maago gen` (generated 12 files), `maago check`, `go vet ./...`, `go test -count=1 ./...` pass; `gofmt -l .` empty.

Unclear / gaps:
- The message says "The policies' :now joins each query's Params" but not whether a query that already declares `-- param: now` should keep it. `maago doc query` answers it ("the query may declare one too, of the same type"); I removed them, and post_list's ORDER BY uses the policy's :now. Works.
- GetPost's `now` is a positional argument after id; nothing in the message says what time to pass beyond "the policy's now". I used time.Now() everywhere (and one per call, not one per request).
- The message says ShowBoard and APIPosts take `u *auth.User` "so anybody may still ask"; it does not say ShowBoard serves the board_rows route too (maago.Block) - it does, and it just worked.
- No doc tells how a func(w, r) view gets a *auth.User for a policy query; the project's own DownloadAttachment showed maago.UserFrom[auth.User]. `go doc maago UserFrom` would also do.

End 14:07 (by `date`).
