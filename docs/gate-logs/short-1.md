# Gate log: TUTORIAL.md §2–5

Result: **reached the end** (superuser created, logged in, Post created/changed/deleted in admin with curl).
Unexpected failures: **0**. The only failure was the tutorial's deliberate one (`maxlen`), plus the
expected "auth has routes" message; both matched the tutorial character for character.

Working dir: `gate1/`, `export PATH=$PWD/bin:$PATH` each shell. Go 1.26.3 darwin/arm64.

## Commands, in order

### Setup / §1
1. `go version` → go1.26.3
2. `maago help` → command list (gen, check, makemigrations, showmigrations, migrate, add, tailwind). OK.

### §2 프로젝트 만들기
3. `mkdir myblog && cd myblog && go mod init example.com/myblog` → OK
4. (instead of `go get github.com/maago-web/maago`, per gate rule)
   `go mod edit -require=github.com/maago-web/maago@v0.0.0-00010101000000-000000000000 -replace=github.com/maago-web/maago=/Users/jinto/projects/maago` → OK
5. `go get modernc.org/sqlite` → OK (added v1.60.1, marked `// indirect` until main imports it)
6. Wrote `.env` = `MAAGO_DATABASE_URL=sqlite:app.db`; `.gitignore` = `.env` (tutorial says to)

### §3 모델
7. `maago add auth` → wrote 36 files under auth/ (models, forms, views, urls, queries/*.sql, templates, tests). OK.
8. `mkdir blog`; wrote `blog/models.go` from the tutorial with the deliberate `maxlen=200` typo.
9. `maago gen` → exit 1 (expected, deliberate):
   ```
   blog/models.go:15:33: unknown tag key "maxlen" in Post.Title
     fix: did you mean "max"? allowed: max, unique, index, default, choices, label, help, fk, auto, admin
   ```
   Identical to tutorial. Fix: `maxlen` → `max` (the `fix:` line said so).
10. `maago gen` → exit 1 (expected):
   ```
   auth/urls.go:5:5: app auth has routes, but no package uses auth.App
     fix: add auth.App to maago.Config.Apps in main (D-114), then run maago gen: go build works after gen
   ```
   Identical to tutorial; tutorial says this is fixed in §5, so I moved on. No `*_gen.go` written.

### §4 마이그레이션
11. `maago makemigrations` → `created migrations/0001_initial.sql` (works even though gen still fails; good).
    File contains the auth_* tables plus `posts`, and the `posts` part matches the tutorial exactly.
12. `maago showmigrations` → `[ ] 0001_initial.sql` / `0 of 1 applied`
13. `maago migrate` → `applied 0001_initial.sql`
14. `maago showmigrations` → `[x] 0001_initial.sql` / `1 of 1 applied`
    (Skipped the "필드 이름을 바꾸면" subsection: it is narrative; no command to run as part of the build.)

### §5 관리자 화면과 로그인
15. Wrote `main.go` = the tutorial's code block verbatim (`sed -n '197,248p' TUTORIAL.md > main.go`).
16. `maago gen` → exit 0, silent. Wrote auth/{urls,views,queries,models,admin}_gen.go, blog/{models,admin}_gen.go.
17. `MAAGO_SUPERUSER_EMAIL=me@example.com MAAGO_SUPERUSER_PASSWORD='<16-char pw>' go run . -createsuperuser` → exit 0, silent.
18. Server: `go build -o ../server.bin . && PORT=18001 ../server.bin &` (built a binary instead of `go run .`
    so I could stop it by name; same code).
19. `curl -si http://localhost:18001/admin/` → `303 Location: /login?next=%2Fadmin%2F`
20. `curl http://localhost:18001/login?next=...` → login form; fields `email`, `password`; no CSRF token field.
21. `curl -si /login?next=%2fadmin%2f --data-urlencode email=... --data-urlencode password=...`
    → `303 Location: /admin/`, `Set-Cookie: session=...; HttpOnly; Secure; SameSite=Lax`
    **Snag (not a failure of the framework):** the cookie is `Secure`, so curl's cookie jar (`-c/-b`) silently
    dropped it over plain http://localhost. I passed it by hand with `-H "Cookie: session=..."`.
22. `GET /admin/` → index listing `/admin/blog/Post/`, `/admin/auth/User/`, `/admin/auth/Permission/`
23. `GET /admin/blog/Post/` → "Rows: 0", link `/admin/blog/Post/add/`
24. `GET /admin/blog/Post/add/` → form: `author_id` (number), `title`, `text`, `status` (select draft/published),
    `published_at` (placeholder `2006-01-02T15:04:05`)
25. **Create**: `POST /admin/blog/Post/add/` author_id=1, title=첫 글, text=안녕하세요, status=draft, published_at=
    → `303 /admin/blog/Post/`; list shows row id 1.
26. `GET /admin/blog/Post/1/` → change form (same fields) + link `/admin/blog/Post/delete/?id=1`
27. **Change**: `POST /admin/blog/Post/1/` title=고친 글, text=고친 본문, status=published, published_at=2026-10-15T11:00:00
    → 303; list shows `고친 글 … published 2026-10-15 11:00`.
28. `GET /admin/blog/Post/delete/?id=1` → confirm page, form with hidden `id=1`
29. **Delete**: `POST /admin/blog/Post/delete/?id=1` id=1 → 303; list "Rows: 0"; `sqlite3 app.db 'select count(*) from posts'` → 0.
30. Extra probes: cross-site POST (`Origin: http://evil.example`, `Sec-Fetch-Site: cross-site`) → 403 (good).
31. Stopped server (`pkill -f server.bin`); `curl :18001` → connection refused.
32. Extras: `maago check` → exit 0; `go vet ./...` → OK; `go mod tidy -diff` → non-empty (sqlite should be a direct require).

### How I found the admin URLs and field names
Not from the tutorial (it only gives `http://localhost:8000/admin/`). I followed HTML: `/admin/` redirect →
`/login` form (`name="email"`, `name="password"`) → admin index links → list page "Add" link → add form
`name=` attributes → list row link `/admin/blog/Post/1/` → its "Delete" link → confirm form (hidden `id`).
Field names are the snake_case column names; `author_id` is a raw number input (I guessed user id 1 for the
only user; the change page then showed a link `auth.User 1`, confirming it).

## Unclear / wrong / had to guess (most important first)
1. **Secure session cookie on http://localhost.** Login answers 303 but a curl cookie jar does not keep the
   cookie, so the next request silently redirects back to /login. Nothing in the tutorial or the response
   says so. An agent following "sign in with curl -c/-b" would loop. (Browsers accept Secure on localhost; curl 8.7 did not here.)
2. **Admin URL scheme and form fields are not documented** (`/admin/<app>/<Model>/`, `add/`, `<id>/`,
   `delete/?id=`, fields = column names, POST form-encoded, no CSRF token but cross-origin check). Discoverable
   by crawling HTML, but a one-paragraph note would save an agent several requests.
3. **Silent success** of `maago gen` and `-createsuperuser`: no output, so "did it work?" needs a separate check.
   A line like `wrote 7 files` / `created superuser me@example.com (id 1)` would help (the id is needed for `author_id`).
4. `author_id` in the admin is a bare number input with no hint which user ids exist.
5. Error text contains internal decision ids (`(D-114)`) which mean nothing to a user of the framework.
6. §2: `go get modernc.org/sqlite` leaves it `// indirect`; after §5 `go mod tidy -diff` is non-empty. The
   tutorial never says to run `go mod tidy` (harmless, but `go mod tidy` would be the natural last step).
7. §3 error position `blog/models.go:15:33` only matches if the `// blog/models.go` comment line is copied
   too (it is in the code block, so fine, but an agent dropping the comment sees line 14).
8. §5 tells you to run `go run .` and visit port 8000; fine, but `go run` in the background is awkward to stop
   (child process); I used `go build` + binary. Not a tutorial bug.
9. The header note mentions `TestShortTutorial`, `cmd/maago`, `M9 3번`, `D-69` — internal references that
   confuse a first-time reader.
10. Bad-password login returns 200 (re-rendered form); fine, just noting no status distinguishes it for curl.
