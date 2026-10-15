# Gate log — TUTORIAL.md §2–5

Working dir: `gate2/`, project `gate2/myblog`. `export PATH=$PWD/bin:$PATH` before every maago call.
Go: go1.26.3 darwin/arm64.

## Commands, in order

| # | Command | Result |
|---|---|---|
| 1 | `go version`, `maago help` | ok; help lists gen, check, makemigrations, showmigrations, migrate, add, tailwind |
| 2 | `mkdir myblog && cd myblog` | ok |
| 3 | `go mod init example.com/myblog` | ok |
| 4 | `go mod edit -require=github.com/maago-web/maago@v0.0.0-00010101000000-000000000000 -replace=github.com/maago-web/maago=/Users/jinto/projects/maago` (in place of `go get github.com/maago-web/maago`, per gate rules) | ok |
| 5 | `go get modernc.org/sqlite` | ok, v1.60.1 |
| 6 | wrote `.env` (`MAAGO_DATABASE_URL=sqlite:app.db`) and `.gitignore` (`.env`) | — |
| 7 | `maago add auth` | ok, wrote 36 files under `auth/` |
| 8 | wrote `blog/models.go` from §3 with the deliberate `maxlen=200` | — |
| 9 | `maago gen` | **expected failure** (tutorial's deliberate one), exit 1, exactly as the tutorial shows: `blog/models.go:15:33: unknown tag key "maxlen" in Post.Title` / `fix: did you mean "max"? allowed: ...` |
| 10 | changed `maxlen` back to `max` | — |
| 11 | `maago gen` | **expected failure**, exit 1, exactly as tutorial: `auth/urls.go:5:5: app auth has routes, but no package uses auth.App` / `fix: add auth.App to maago.Config.Apps in main (D-114), then run maago gen: go build works after gen`. Tutorial says this is fixed in §5, so I moved on. |
| 12 | `maago makemigrations` | `created migrations/0001_initial.sql`; the `posts` table is byte-for-byte the tutorial's SQL |
| 13 | `maago showmigrations` | `[ ] 0001_initial.sql` / `0 of 1 applied` (as tutorial) |
| 14 | `maago migrate` | `applied 0001_initial.sql` (as tutorial); `app.db` created |
| 15 | wrote `main.go` = §5's block verbatim | — |
| 16 | `maago gen` | ok, exit 0, silent. Wrote `auth/*_gen.go` (admin, models, queries, urls, views) and `blog/{admin,models}_gen.go` |
| 17 | `go build ./...` | ok |
| 18 | `MAAGO_SUPERUSER_EMAIL=me@example.com MAAGO_SUPERUSER_PASSWORD='<my password>' go run . -createsuperuser` | ok, exit 0, **no output at all** |
| 19 | `go build -o ../myblog-server . && PORT=18002 ../myblog-server &` (background) | server up. I built a binary instead of `go run .` only so the background process could be killed cleanly (killing `go run` can leave the child running). Same code. |
| 20 | `curl -i http://localhost:18002/admin/` | `303` → `Location: /login?next=%2Fadmin%2F` |
| 21 | `curl -c cj.txt 'http://localhost:18002/login?next=%2Fadmin%2F'` | 200, form `method=post action="/login?next=%2fadmin%2f"`, fields `email`, `password`, no CSRF token field |
| 22 | `curl -b cj.txt -c cj.txt -X POST '.../login?next=%2fadmin%2f' --data-urlencode email=me@example.com --data-urlencode password=...` | 303 → `/admin/`, `Set-Cookie: session=...; HttpOnly; Secure; SameSite=Lax` |
| 23 | `curl -b cj.txt .../admin/` | 200, links `/admin/blog/Post/`, `/admin/auth/User/`, `/admin/auth/Permission/` (curl still sends the `Secure` cookie to `http://localhost`) |
| 24 | `GET /admin/blog/Post/` and `/admin/auth/User/` | Post: `Rows: 0 · Add` → `/admin/blog/Post/add/`. User: my superuser, row link `/admin/auth/User/1/` |
| 25 | `GET /admin/blog/Post/add/` | form `method=post` (same URL), fields `author_id` (number), `title`, `text`, `status` (select draft/published), `published_at` (placeholder `2006-01-02T15:04:05`) |
| 26 | `POST /admin/blog/Post/add/` `author_id=1 title='첫 글' text='안녕하세요' status=draft published_at=` | **create ok**: 303 → `/admin/blog/Post/`; list shows row 1 |
| 27 | `GET /admin/blog/Post/1/` | change form (same fields, prefilled), link `Delete` → `/admin/blog/Post/delete/?id=1` |
| 28 | `POST /admin/blog/Post/1/` `title='첫 글 (고침)' text='고친 본문' status=published published_at=2026-10-15T09:00:00 author_id=1` | **change ok**: 303; list shows the new values, 게시일 `2026-10-15 09:00` |
| 29 | `GET /admin/blog/Post/delete/?id=1` | confirm page: `<form method="post">` with hidden `id=1` |
| 30 | `POST /admin/blog/Post/delete/` `id=1` | **delete ok**: 303; list shows `Rows: 0`, `No rows.` |
| 31 | extra: `POST /admin/blog/Post/add/` without cookie | 401 |
| 32 | extra: same with cookie and `Origin: http://evil.example` | 403 (cross-origin protection; explains why the forms have no CSRF token) |
| 33 | extra: `-createsuperuser` again with same email | `auth: me@example.com has an account already`, exit 1 |
| 34 | extra: `-createsuperuser` with password `password` | `auth: This password is too common.`, exit 1 |
| 35 | `kill <pid>`; `curl .../admin/` → `000` | server stopped |

## Failures

None unexpected. The only two failures were the two the tutorial causes on purpose (#9, #11); both
messages matched the tutorial character for character, and each `fix:` line said what to do.
I fixed #9 from the `did you mean "max"?` hint; #11 the tutorial itself says to leave until §5.

## How I found the admin's URLs and form field names

Nothing in the tutorial names them except `/admin/` and `/login`. I found everything by following
the HTML: the 303 `Location` to `/login?next=...`; the login form's `name="email"`/`name="password"`;
the admin index links (`/admin/<app>/<Model>/`); the list page's `Add` link; the add/change forms'
`name=` attributes (snake_case column names: `author_id`, `published_at`, ...) with `method="post"`
and no `action` (posts to itself); the change page's `Delete` link to the confirm page, whose form
posts hidden `id` back to `/admin/blog/Post/delete/`. The `published_at` format came from the
input's `placeholder`.

## Unclear / wrong / guessed spots (most important first)

1. **§5 never says the login URL or that admin redirects to it.** The tutorial ends at "log in at
   /admin/"; for a curl/agent user the field names (`email`, `password`), the `next` param and the
   session cookie are only discoverable by reading HTML. Fine for a browser, a guess for an agent.
2. **The session cookie is `Secure` even on `http://localhost`.** curl sent it anyway (curl treats
   localhost as secure), but an HTTP client that honors `Secure` strictly on plain http (I did not
   test which ones) would silently drop the session and loop back to /login. Not mentioned anywhere.
3. **No CSRF token in forms; cross-origin POSTs get a bare 403.** It works (Origin-based), but nothing
   says so; an agent may wonder whether it must scrape a token. A one-line note would help.
4. **`-createsuperuser` prints nothing on success.** Exit 0 and silence; I had to check the User list
   to be sure. A line like `created superuser me@example.com (id 1)` would confirm it.
5. **§3 shows the second `maago gen` error, then says to fix it in §5 — but between them §4 runs
   `makemigrations`/`migrate` while `gen` is still failing.** It works (makemigrations doesn't need
   gen to pass), but it reads as if you're continuing with a broken project; I had to trust it.
6. **Error text contains `(D-114)`.** That's an internal decision number; meaningless to a user.
7. **§2's `.env` is shown inside a `sh` code block with `# .env` as a comment.** Running it as shell
   does nothing; you have to infer "create a file .env with this line". Same for `.gitignore` (said in
   prose only).
8. **§5's `'…'` in the createsuperuser command** — clear from the next paragraph, but the rules
   (8+ chars, not common, not like the email) come after the command; a reader who copy-pastes first
   gets `This password is too common.`-type errors. (The error itself is clear.)
9. **§5 says `go run .` and then "<http://localhost:8000/admin/>"** — fine, and `PORT` is in main's
   comment, but the port is only discoverable by reading main.go.
10. **Admin form field names are column names (`author_id`) while the model fields are Go names
    (`AuthorID`).** Reasonable, but unstated.
11. **The admin date input is plain text with a placeholder `2006-01-02T15:04:05`** — Go's reference
    time is a puzzle for non-Go readers; the timezone the value is read in is not stated.
