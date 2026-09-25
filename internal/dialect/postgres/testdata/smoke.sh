#!/bin/sh
# Applies example.sql to a throwaway PostgreSQL and checks that the schema
# accepts what it should and refuses what it should, for the reason it
# should. The cluster lives in a temporary directory, listens on a unix
# socket only, and is removed at the end; no other server is touched.
# Run from anywhere: sh smoke.sh (needs PostgreSQL's server binaries,
# found through pg_ctl on PATH).
set -u
cd "$(dirname "$0")" || exit 1

pgctl=$(command -v pg_ctl) || { echo "pg_ctl not found on PATH"; exit 1; }
target=$(readlink "$pgctl") || target=$pgctl
case "$target" in /*) ;; *) target=$(dirname "$pgctl")/$target ;; esac # a relative link is relative to its own directory
bindir=$(dirname "$target")
for b in initdb pg_ctl postgres psql; do
	[ -x "$bindir/$b" ] || { echo "$bindir/$b not found"; exit 1; }
done

tmp=$(mktemp -d) || exit 1
cleanup() {
	"$bindir/pg_ctl" -D "$tmp/data" -m immediate stop >/dev/null 2>&1
	rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

"$bindir/initdb" -D "$tmp/data" -U maago --auth=trust >"$tmp/initdb.log" 2>&1 || { cat "$tmp/initdb.log"; exit 1; }
"$bindir/pg_ctl" -D "$tmp/data" -w -l "$tmp/server.log" -o "-c listen_addresses='' -k '$tmp'" start >/dev/null 2>&1 || { cat "$tmp/server.log"; exit 1; }

fail=0
run() { # run <want: ok, or the expected error text> <description> <sql>
	# Each case is one transaction, rolled back, so cases do not mix.
	out=$(printf 'BEGIN;\n%s\n%s\nROLLBACK;\n' "$(cat example.sql)" "$3" |
		"$bindir/psql" -h "$tmp" -U maago -d postgres -X -q -v ON_ERROR_STOP=1 2>&1)
	status=$?
	case "$1" in
	ok) [ $status -eq 0 ] ;;
	*) [ $status -ne 0 ] && case "$out" in *"$1"*) true ;; *) false ;; esac ;;
	esac
	if [ $? -eq 0 ]; then
		echo "ok:   $2"
	else
		echo "FAIL: $2 (want $1): $out"
		fail=1
	fi
}
author="INSERT INTO people (name, editor_id, code) VALUES ('a', 1, 'c1');"
post="INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't', 's', '\\x00');"
assert() { echo "SELECT 1 / (CASE WHEN $1 THEN 1 ELSE 0 END);"; } # fails with division by zero

run ok "schema applies"            "SELECT 1;"
run ok "id is generated"           "$author $(assert "(SELECT id FROM people) = 1")"
run ok "defaults fill in"          "$author $post $(assert "(SELECT status = 'draft' AND public AND views = 0 AND score = 1.5 FROM blog_post) AND (SELECT NOT admin AND \"order\" = 7 FROM people)")"
run 'violates foreign key constraint "blog_post_author_id_fkey"' "FK to a missing author" "$post"
run 'violates check constraint "blog_post_status_check"'         "choice outside the list" "$author INSERT INTO blog_post (author_id, title, slug, body, status) VALUES (1, 't', 's', '\\x00', 'x');"
run 'value too long for type character varying(200)'              "title over 200 characters" "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, repeat('a', 201), 's', '\\x00');"
run ok                                                             "200 Korean characters fit" "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, repeat('가', 200), 's', '\\x00');"
run 'invalid input syntax for type integer'                       "text in an integer" "$author INSERT INTO blog_post (author_id, title, slug, body, views) VALUES (1, 't', 's', '\\x00', 'abc');"
run 'is of type boolean but expression is of type integer'        "integer in a bool" "$author INSERT INTO blog_post (author_id, title, slug, body, public) VALUES (1, 't', 's', '\\x00', 2);"
run ok                                                             "NULL where allowed" "$author INSERT INTO blog_post (author_id, title, slug, body, score) VALUES (1, 't', 's', '\\x00', NULL);"
run 'null value in column "title"'                                "NULL into a NOT NULL column" "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, NULL, 's', '\\x00');"
run 'violates unique constraint "blog_post_slug_key"'             "duplicate slug" "$author $post INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't2', 's', '\\x00');"
run 'violates foreign key constraint "blog_post_author_id_fkey"' "delete a referenced author" "$author $post DELETE FROM people;"
run ok "composite unique and indexes exist" "$(assert "(SELECT count(*) FROM pg_constraint WHERE conname = 'blog_post_author_id_slug_key') = 1 AND (SELECT count(*) FROM pg_indexes WHERE indexname IN ('blog_post_status_idx', 'blog_post_author_id_idx', 'people_parent_id_idx', 'people_reviewer_id_idx', 'people_code_idx')) = 5")"
run ok "an explicit id is accepted (BY DEFAULT)" "INSERT INTO people (id, name, editor_id, code) VALUES (10, 'a', 10, 'c1');"
run ok "ids are not reused" "$author INSERT INTO people (name, editor_id, code) VALUES ('b', 2, 'c2'); DELETE FROM people WHERE id = 2; INSERT INTO people (name, editor_id, code) VALUES ('c', 3, 'c3'); $(assert "(SELECT max(id) FROM people) = 3")"
exit $fail
