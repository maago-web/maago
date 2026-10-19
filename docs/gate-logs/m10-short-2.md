# Gate log

Environment: go1.26.3 darwin/arm64. Each shell: `export GOBIN=$W/bin PATH=$W/bin:$PATH`.

## §1 Install

1. `go install github.com/maago-web/maago/cmd/maago@latest` -> downloaded v0.1.0, OK
2. `which maago` -> $W/bin/maago (correct)
3. `maago help` -> command list, OK

No failures.

## §2 Make the project

4. `mkdir myblog && cd myblog && go mod init example.com/myblog` OK
5. `go get github.com/maago-web/maago` -> v0.1.0 OK
6. `go get modernc.org/sqlite` -> upgraded modernc.org/sqlite v1.59.0 => v1.60.1 (and libc, x/sys). OK, just a note: maago pins an older sqlite and this upgrades it.
7. wrote `.env` (`MAAGO_DATABASE_URL=sqlite:app.db`) and `.gitignore` (`.env`)

No failures. Note: go.mod lists maago as `// indirect` until main imports it (normal Go behaviour).

## §3 Models

8. `maago add auth` -> wrote 36 files under auth/, OK
9. wrote blog/models.go with `maxlen=200` on purpose
10. `maago gen` -> intended failure (exit 1):
    ```
    blog/models.go:14:33: unknown tag key "maxlen" in Post.Title
      fix: did you mean "max"? allowed: max, unique, index, default, choices, label, help, fk, auto, admin
      see: maago doc model
    ```
    Matches tutorial except line 14 vs tutorial's 15 (tutorial counts the `// blog/models.go` comment line it shows inside the code block; if you copy the block literally including the comment, it would be 15. I left the comment out -> 14). Trivial.
11. changed back to `max=200`; `maago gen` -> exit 1, as tutorial says:
    ```
    auth/urls.go:5:5: app auth has routes, but no package uses auth.App
      fix: add auth.App to maago.Config.Apps in main, then run maago gen: go build works after gen
      see: maago doc main
    ```
    Expected per tutorial (deferred to §5). Slightly odd that the tutorial's "this time it says what to do next" is an error with exit 1 and you are told to ignore it until §5; fine once read.

## §4 Migrations

12. `maago makemigrations` -> `created migrations/0001_initial.sql`, exit 0. posts table identical to the tutorial's. (Ran fine even though `maago gen` still fails on the auth.App error; good.)
13. `maago showmigrations` -> `[ ] 0001_initial.sql` / `0 of 1 applied` (matches)
14. `maago migrate` -> `applied 0001_initial.sql` (matches); app.db created
15. Rename experiment (tutorial's "When a field is renamed" is prose, not a step; I tried it and reverted, nothing applied):
    - renamed `Text` -> `Body`; `maago makemigrations` -> created 0002_posts.sql with three ⚠ warnings printed and inlined as comments (drop+add, NOT NULL without default, DROP needs `-- maago: allow-drop`). First warning tells exactly what to do: "delete this file, then run maago makemigrations --rename posts.text:body". Very clear.
    - `rm migrations/0002_posts.sql; maago makemigrations --rename posts.text:body` -> `0002_rename_posts_text.sql` with `ALTER TABLE "posts" RENAME COLUMN "text" TO "body";`
    - reverted: deleted 0002, restored `Text`; `maago makemigrations` -> `no changes`.

No failures. Unclear spot: the tutorial says the rename draft gets "a ⚠ warning" (singular); actually three warnings. Also it doesn't say whether the reader should actually keep the rename (I assumed not, since §6 uses `text`).

## §5 The admin and signing in

16. wrote main.go exactly as the tutorial's block
17. `maago gen` -> `generated 8 files`, exit 0 (matches)
18. `MAAGO_SUPERUSER_EMAIL=me@example.com MAAGO_SUPERUSER_PASSWORD='<16+ chars, not common>' go run . -createsuperuser` -> `created superuser me@example.com (id 1)` (matches). No `go mod tidy` was needed even though go.mod still marked maago `// indirect`.
19. Server: `go build -o ../myblog-server . && PORT=18002 ../myblog-server &` (built a binary so I could stop it cleanly). Later also ran the tutorial's literal `PORT=18002 go run .` in the background: /login 200; stopped with SIGINT (graceful, port freed).
20. `curl -si http://localhost:18002/admin/` -> 303 to `/login?next=%2Fadmin%2F` (only signed-in users get in, as promised)
21. `curl -si -c cj.txt 'http://localhost:18002/login?next=%2Fadmin%2F'` -> 200 sign-in form; fields `email`, `password`; form action `/login?next=%2fadmin%2f`. No CSRF token field.
22. `curl -si -b cj.txt -c cj.txt -X POST 'http://localhost:18002/login?next=%2fadmin%2f' --data-urlencode email=me@example.com --data-urlencode password=…` -> 303 Location /admin/, `Set-Cookie: session=…; HttpOnly; SameSite=Lax`
23. `curl -b cj.txt http://localhost:18002/admin/` -> 200, index links: /admin/blog/Post/, /admin/auth/User/, /admin/auth/Permission/
24. `GET /admin/blog/Post/` -> "Rows: 0 · Add" link /admin/blog/Post/add/
25. `GET /admin/blog/Post/add/` -> form method=post (action = same URL); inputs author_id (number), title, text, status (select draft/published), published_at (placeholder 2006-01-02T15:04:05). Field names = snake_case column names.
26. CREATE: `POST /admin/blog/Post/add/` with author_id=1, title, text, status=published, published_at=2026-10-19T12:00:00 -> 303 /admin/blog/Post/; list shows row id 1.
27. `GET /admin/blog/Post/1/` -> change form, same field names, plus a "Delete" link `/admin/blog/Post/delete/?id=1`
28. CHANGE: `POST /admin/blog/Post/1/` title/text changed, status=draft, published_at empty -> 303; list shows new values, Published empty (NULL).
29. `GET /admin/blog/Post/delete/?id=1` -> confirm page, form method=post with hidden `id=1`
30. DELETE: `POST /admin/blog/Post/delete/?id=1` with id=1 -> 303; list "Rows: 0".
31. Extra probe: same add POST with `Origin: http://evil.example`/`Sec-Fetch-Site: cross-site` -> 403 Forbidden (so CSRF protection is header-based; explains why no token field).
32. `maago check` -> exit 0; `go vet ./...` -> ok.
33. Server stopped (SIGINT), port 18002 free.

How I found the admin's URLs and field names: nothing in the tutorial lists them. I followed the links in the HTML: /admin/ redirect -> /login form (field names from `<input name=…>`), admin index links, the list page's "Add" link, the add form's `<input name>`s, the row's link to /admin/blog/Post/1/, and the "Delete" link and its confirm form's hidden `id`. All discoverable by curl + reading HTML; no JS needed.

No failures in §5.

Unclear/guess spots in §5:
- Tutorial says "Sign in at http://localhost:8000/admin/" but does not say the login URL/field names or that forms are plain POSTs; for a curl user it's fine because the HTML is simple, but an agent has to scrape.
- Tutorial does not mention that there is no CSRF token and that cross-origin POSTs are refused by header (Origin/Sec-Fetch-Site); I only learned by probing. An agent writing its own forms later would want to know.
- `go run .` in the background spawns a child binary in the build cache; stopping it means finding that child (killing `go run` by name pattern is fiddly). Not Maago's fault.
- The server prints nothing on start (no "listening on :18002" line) and no request log, so a background start gives no signal it is ready; I had to poll with curl.
