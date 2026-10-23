# GATE-LOG: steps 0034-0036 on board (maago v0.2.0)

Env: GOWORK=off GOFLAGS=-mod=mod, GOPATH/GOMODCACHE as given. Run from board/.
Setup (14:01): board/ has no git; copied it to m11gate1/snap-0033 to diff against.
`../bin/maago migrate` -> applied 0001..0005 (ok).

## 0034: Let the database refuse what is taken (start 14:01)

Read: STEPS.md 0034, board/views.go, forms.go, models.go, the tests that
named the old messages, `maago doc form` (Fail section),
`go doc maago UniqueError`, `go doc maago Messages`, `go doc maagotest.DB`.

Doc note: `maago doc form` line ~68 explains Fail, but its full example
further down (~line 220, CreatePost/APICreatePost with titleTaken) still
asks first with a read before the write - the very pattern Fail replaces.
Mixed message for an agent copying the example.

Changes (told by the message; Fail's behaviour confirmed by `maago doc form`):
- deleted board/queries/board_name_taken.sql, post_title_taken.sql.
- views.go CreateBoard: InsertBoard, then `if f.Fail(err)` show the form,
  then `if err != nil` return (the message's snippet verbatim).
- savePost: PinnedOverlap stays first; the title read is gone; `f.Fail(err)`
  after InsertPost -> false. APICreatePost unchanged (f.Title.Error -> 422).
- ChangePost: title read gone; `case f.Fail(err):` after ErrConflict.
- comments of CreateBoard/savePost updated to say who refuses.
Commands: `../bin/maago gen` -> "generated 12 files"; check, vet ok first try.
Tests (web_test.go): the four old messages -> "This value is already taken."
(board name, title on board, API 422 detail, edit). Added: a post whose title
and pin both clash shows the pin's error and not the taken one
(TestPinAndSchedule). Added TestWriteRace: tx := db.BeginTx, InsertBoard in
it, the request POST in a goroutine, sleep 100ms, tx.Commit; the request
answers 200 with the message and value="자유", one board after.
Guess: the 100ms sleep to make "the request gets to the index first" likely;
the test passes in either order, which the message's "if it gets there first"
suggests. Not checked on SQLite (this .env is PG); on SQLite the request
would wait for the write lock instead - should still pass.
Result: gen, check, vet, `go test ./...` (auth ok, board ok), gofmt -l empty.
Unexpected failures: none.
Unclear: "goes on to commit its page" - a refused request writes nothing,
so commit is not observable; I check 200 + one board. Whether Fail also
returns true for the (BoardID,Title) refusal on the API's form (PostForm,
same form) - yes, the API 422 test passed with the new message.
End 0034: 14:04 (~3 min).

## 0035: Write who may read a post (start 14:04)

Read: STEPS.md 0035, `maago doc query` (rule 6, the Read policies section,
the header lines of policies.sql and `policy`).
Changes (all told by the message; doc confirmed the syntax):
- board/board/policies.sql: the two blocks of the message, verbatim.
- post_list, search_posts, export_rows: `-- policy: out p`, and the copied
  "not hidden, not a draft, out by now" lines replaced by `AND out(p)`.
  They keep their own `-- param: now` (doc: "the query may declare one too,
  of the same type"); post_list uses :now in ORDER BY too.
- my_posts: `-- policy: visible p`, `NOT p.hidden` -> `visible(p)`.
- recent_posts: `-- policy: none board_post for the tests, ...`.
- pinned_overlap: `-- policy: none board_post a count of the notices ...`
  (the message's line).
- post_detail.sql left as it was.
Command: `../bin/maago gen` -> exit 1, output:
    board/queries/post_detail.sql:5:6: query reads board_post p without a policy (rule 6)
      fix: declare -- policy: out p and write out(p) in the WHERE of the SELECT that reads it (policies of Post: out, visible); or -- policy: none p why, if it may read every row
      see: maago doc query
First line equals the Maago-Expect trailer. Reproduced.
gen wrote nothing on failure (generated files unchanged; go vet still ok
on the old generated code).
Guess: whether my_posts should keep `NOT p.hidden` beside visible(p);
dropped it, visible says hidden is nobody's.
Note: only one diagnostic printed; I can't tell if gen would report more
problems after the first (e.g. the views not yet taking the user).
End 0035: 14:05 (~1 min).

## 0036: Read posts under the policies (start 14:05)

Changes:
- post_detail.sql: `-- policy: visible p`, `AND NOT p.hidden` -> `AND visible(p)`,
  its doc line "none if it is hidden" -> "none if the user may not read it".
- `../bin/maago gen` -> exit 1 with 12 type errors, each with
  "fix: go build reports the same error; fix it there", e.g.
    board/views.go:115:34: not enough arguments in call to GetPost; have (context.Context, maago.DB, int64); want (context.Context, *"example.com/board/auth".User, maago.DB, int64, time.Time)
  (gen had written the new code; the errors are the project's calls.)
  Expected - the message says every policy query now takes the user.
- views.go (told by the message): ShowBoard and APIPosts take u *auth.User;
  SearchPosts/PostList/MyPosts/PostDetail get u (Me passes &u, Now);
  GetPost(ctx, &u, tx, id, time.Now()) in Attach, RemovePost, moveTo,
  editable; maySee deleted; Attach's and editable's Hidden checks deleted;
  postPage reads PostDetail alone (PostDetailParams{ID, Now: time.Now()}).
  DownloadAttachment: GetPost(ctx, requestUser(r), db, a.PostID, now) and
  the existing ErrNoRows -> 404.
- added requestUser(r) *auth.User (maago.UserFrom, as DownloadAttachment
  already did inline) because ExportBoard needs it too ("reads it from the
  request, as DownloadAttachment does"). My choice: one helper over two copies.
- export.go: ExportRows(ctx, requestUser(r), db, ...).
- mayChange kept.
Then gen ok, check ok; `go vet` failed on tests:
    vet: board/admin_test.go:125:35: not enough arguments in call to GetPost
Tests: every GetPost(ctx, db, ...) in *_test.go -> GetPostIgnoringPolicies
(message: "the tests that read back what they wrote"). queries_test.go:
PostList/SearchPosts/PostDetail get nil user and Now: time.Now() (PostList
and SearchPosts tests passed no Now before - zero time, which worked only
because publish_at was NULL).
`go test` then failed exactly on the four rows the message lists:
    permissions_test.go:122: post_publish POST /posts/2/publish as bob: 404, want 403
    ... /posts/3 as bob, as ann; api /api/posts/4 as bob; /api/posts/5 as bob, as ann
-> permission table rows 403 -> 404 as the message's table says, with a comment.
Added: TestHiddenAttachments checks the author's second delete of the
hidden post is now 404 (the message: "they no longer ... delete it").
Result: gen, check, vet, go test ./... ok, gofmt -l empty.
Unexpected failures: none; everything failed where the message said it would.
Guesses: none of substance. Kept the queries' own `-- param: now` beside the
policy's (doc allows it).
End 0036: 14:07 (~2 min).

## Notes across the steps
- go.sum gained /go.mod lines (sqlite, modernc, x/tools ...) from GOFLAGS=-mod=mod
  during go vet/test; not something I changed by hand.
- The step messages were precise enough to need no guessing beyond the
  race test's timing. The gen diagnostics (rule 6 with its fix line, the
  type errors naming the wanted signature) pointed straight at each fix.
- `maago doc form`'s long example still shows the read-before-write
  titleTaken pattern that 0034 retires (gap).
- 0035's "fix:" names the first policy (out) for post_detail; the message
  warns about it, but an agent without the message would likely take out,
  which hides drafts from their authors - the diagnostic's suggestion is
  arbitrary there.
