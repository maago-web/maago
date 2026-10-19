# Gate log

## Setup
- `export GOBIN=$PWD/bin PATH=$PWD/bin:$PATH` (in every shell)
- go version: go1.26.3 darwin/arm64

## §1 Install
- `go install github.com/maago-web/maago/cmd/maago@latest` -> downloaded v0.1.0, OK
- `which maago` -> gate1/bin/maago (correct)
- `maago help` -> OK, lists commands

## §2 Make the project
- `mkdir myblog && cd myblog`
- `go mod init example.com/myblog` -> OK (go 1.26.3 in go.mod)
- `go get github.com/maago-web/maago` -> added v0.1.0
- `go get modernc.org/sqlite` -> upgraded sqlite v1.59.0 => v1.60.1, libc, x/sys (newer than maago's own requirements; fine)
- wrote `.env` with `MAAGO_DATABASE_URL=sqlite:app.db`, `.gitignore` with `.env`
- Note: go.mod marks maago and sqlite `// indirect` until code imports them (normal Go behavior, not a problem).

## §3 Make an app: the models
- `maago add auth` -> wrote 36 files under auth/ (go, queries, templates, tests). OK.
- `mkdir blog`, wrote blog/models.go with the deliberate `maxlen=200`.
  (I did not include the tutorial's `// blog/models.go` first-line comment, since it is a filename label.)
- `maago gen` (expected failure 1, deliberate):
  ```
  blog/models.go:14:33: unknown tag key "maxlen" in Post.Title
    fix: did you mean "max"? allowed: max, unique, index, default, choices, label, help, fk, auto, admin
    see: maago doc model
  ```
  Matches the tutorial except line 14 vs 15 (tutorial counts its `// blog/models.go` label line; harmless but
  a reader comparing literally might wonder). Fix: `maxlen` -> `max` per the "fix:" line.
- `maago gen` (expected failure 2, deliberate):
  ```
  auth/urls.go:5:5: app auth has routes, but no package uses auth.App
    fix: add auth.App to maago.Config.Apps in main, then run maago gen: go build works after gen
    see: maago doc main
  ```
  Exactly as the tutorial says; it is deferred to §5. No *_gen.go written yet.

## §4 Migrations
- `maago makemigrations` -> `created migrations/0001_initial.sql`. The posts table and index match the
  tutorial exactly. (Works even though gen had not yet succeeded; good.)
- `maago showmigrations` -> `[ ] 0001_initial.sql` / `0 of 1 applied` (as tutorial)
- `maago migrate` -> `applied 0001_initial.sql`; app.db created.
- `maago showmigrations` -> `[x] ... 1 of 1 applied`.
- "When a field is renamed": unclear whether this is a step to do or an illustration (it says "Rename ... and
  run", but §6 onwards still uses `text`). I tried it without migrating, then reverted:
  - renamed `Text` -> `Body`, `maago makemigrations` -> created 0002_posts.sql with three ⚠ lines printed to
    stdout and as comments in the file; the first one says exactly "delete this file, then run maago
    makemigrations --rename posts.text:body". Very clear. Exit code 0 even with ⚠ (tutorial says "there are no
    warnings, only errors" in §3, but makemigrations does print ⚠ warnings; the tutorial itself calls it "a ⚠
    warning" here, so it is consistent-ish: makemigrations warns, check errors).
  - `rm migrations/0002_posts.sql; maago makemigrations --rename posts.text:body` -> `ALTER TABLE "posts" RENAME COLUMN "text" TO "body";`
  - deleted that draft, restored `Text`, `maago makemigrations` -> `no changes`. State back to 0001 only.

## §5 The admin and signing in
- Wrote main.go exactly as in the tutorial (minus the `// main.go` label line).
- `maago gen` -> `generated 8 files` (auth: urls/views/queries/forms/models/admin _gen.go; blog: models/admin _gen.go). Matches tutorial.
- `go vet ./...` -> clean (my own extra check).
- `MAAGO_SUPERUSER_EMAIL=me@example.com MAAGO_SUPERUSER_PASSWORD='short' go run . -createsuperuser`
  (deliberate, to see the message) -> `auth: This password is too short. It must contain at least 8 characters.` exit 1. Clear.
- `MAAGO_SUPERUSER_EMAIL=me@example.com MAAGO_SUPERUSER_PASSWORD='…' go run . -createsuperuser`
  -> `created superuser me@example.com (id 1)`.
- `PORT=18001 go run . > ../server.log 2>&1 &` then polled `curl http://localhost:18001/admin/` until it answered.
  Note: the server prints nothing on start (no "listening on :18001"), so the only way to know it is up was polling.
- `curl -si /admin/` -> 303 to `/login?next=%2Fadmin%2F`.
- `curl /login` -> the HTML form: `<form method="post" action="/login">` with fields `email`, `password`. No CSRF token in the form.
- `curl -c jar.txt -X POST --data-urlencode email=me@example.com --data-urlencode password=... --data-urlencode next=/admin/ '/login?next=%2Fadmin%2F'`
  -> 303 Location /admin/, Set-Cookie session=...; HttpOnly; SameSite=Lax.
  (I sent `next` both in query and body because the form's action drops the query; not sure which one is read.
  Unclear from the login page; `maago doc auth` says "the sign-in goes back to next".)
- `curl -b jar.txt /admin/` -> index with links `/admin/blog/Post/`, `/admin/auth/User/`, `/admin/auth/Permission/`.
- `GET /admin/blog/Post/` -> "Rows: 0", link `/admin/blog/Post/add/`.
- `GET /admin/blog/Post/add/` -> form fields `author_id` (number), `title`, `text`, `status` (select draft/published),
  `published_at` (placeholder `2006-01-02T15:04:05`).
- CREATE: `POST /admin/blog/Post/add/` with author_id=1, title=Hello world, text=First post., status=published,
  published_at=2026-10-19T12:00:00 -> 303 to list; list shows row id 1.
- `GET /admin/blog/Post/1/` -> change form, prefilled; link to `/admin/blog/Post/delete/?id=1`.
- CHANGE, failure 1 (my mistake, not the tutorial's): POST with only `title=Hello again` -> 200 with three
  `<p class="error">This field is required.</p>`. The admin form is a full replace, not a PATCH. Fixed by sending
  all fields (as a browser would): author_id=1, title=Hello, changed, text=First post, edited., status=draft,
  published_at= (empty -> NULL) -> 303; list shows the new values, Published empty. The messages told me which
  fields were missing only by position in the HTML (the error follows each field's input); fine for a browser,
  a bit indirect for curl/grep since the error line does not name the field.
- DELETE: `GET /admin/blog/Post/delete/?id=1` -> confirm page with a form `method=post`, hidden `id=1`.
  `POST -d id=1 /admin/blog/Post/delete/?id=1` -> 303; list says "Rows: 0 / No rows."
- Extra checks: POST to admin without cookie -> 401; POST with `Origin: http://evil.example`,
  `Sec-Fetch-Site: cross-site` -> 403 (matches `maago doc auth`: "There is no CSRF token: a POST from another origin is a 403").
- Stopped the server: `lsof -ti tcp:18001 | xargs kill`. Server log:
  ```
  maago: shutting down, waiting up to 30s for 0 requests
  <nil>
  exit status 1
  ```
  Tutorial bug: `log.Fatal(maago.Serve(ctx, ":"+port, h))` turns a graceful shutdown (Serve returns nil) into
  printing `<nil>` and exit status 1, though the comment says "stops on Ctrl-C, letting requests finish".

### How I found the admin URLs and field names
- Not from the tutorial: §5 only says "Sign in at http://localhost:8000/admin/". I followed the 303 to /login,
  read the login form HTML for `email`/`password`, then crawled the admin's HTML: index -> list -> add form
  (field names from `name=`), row link -> change form, "Delete" link -> confirm form.
- Afterwards I found `maago doc admin` documents all of it: the URL table (`/admin/<app>/<model>/`, `add/`,
  `<id>/`, `delete/?id=`), "a form field's key is the field's name in snake_case", fk takes the row number,
  time format `2006-01-02T15:04:05`. `maago doc auth` documents `/login` and fields `email`/`password` and the
  Origin-based CSRF rule. The tutorial §5 never points to `maago doc admin` / `maago doc auth`.

## Other notes on the tutorial / tool
- `maago doc` with no topic lists topics but exits 1 (with "usage:"), though the tutorial presents it as the
  way to list topics.
- `maago doc admin`'s first snippet shows `Apps: []maago.App{auth.App, blog.App, adminApp}` while its full
  example (and the tutorial §5) has no blog.App (blog has no routes) -> slightly inconsistent.
- Error line numbers (models.go:14 vs tutorial's :15) differ because the tutorial's code blocks start with a
  `// blog/models.go` label comment.
- §4 "When a field is renamed" reads like an instruction ("Rename ... and run") but is meant as an aside; I did it
  and reverted.
- Admin add/change form: `text` is a single-line `<input type="text">`, title has no maxlength attribute; the
  `published_at` placeholder is Go's reference time, which a non-Go user may read as a literal default.
- The tutorial says `go run .` then "Sign in at http://localhost:8000/admin/", but never says how to sign in
  (no URL/fields/that it redirects); fine for a browser user, a guess for an agent with curl.

## Result
Reached the end: superuser created, signed in via POST /login, created, changed and deleted a Post with curl.
Failures: 2 deliberate tutorial ones (maxlen, auth.App), 1 deliberate by me (short password), 1 real mistake
by me (partial change POST) fixed from the "This field is required." messages. Zero tutorial-caused failures.
