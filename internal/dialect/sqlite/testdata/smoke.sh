#!/bin/sh
# Applies example.sql to an in-memory SQLite and checks that the schema
# accepts what it should and refuses what it should. Run from this
# directory: sh smoke.sh (needs the sqlite3 command).
set -u
fail=0
run() { # run <want: ok|error> <description> <sql>
	out=$(sqlite3 -bail :memory: ".read example.sql" "PRAGMA foreign_keys = ON;" "$3" 2>&1)
	status=$?
	if { [ "$1" = ok ] && [ $status -ne 0 ]; } || { [ "$1" = error ] && [ $status -eq 0 ]; }; then
		echo "FAIL: $2 (want $1): $out"
		fail=1
	else
		echo "ok:   $2"
	fi
}
author="INSERT INTO people (name, editor_id, code) VALUES ('a', 1, 'c1');"
post="INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't', 's', x'00');"

run ok    "schema applies"                 "SELECT 1;"
run ok    "NULL id is generated"           "$author SELECT CASE WHEN id = 1 THEN 1 ELSE abs(-9223372036854775808) END FROM people;"
run ok    "defaults fill in"               "$author $post SELECT CASE WHEN status = 'draft' AND public = 1 AND views = 0 AND score = 1.5 THEN 1 ELSE abs(-9223372036854775808) END FROM blog_post;"
run error "FK to a missing author"         "$post"
run error "choice outside the list"        "$author INSERT INTO blog_post (author_id, title, slug, body, status) VALUES (1, 't', 's', x'00', 'x');"
run error "title over 200 characters"      "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, printf('%.201c', 'a'), 's', x'00');"
run ok    "200 Korean characters fit"      "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, replace(printf('%.200c', 'x'), 'x', '가'), 's', x'00');"
run error "text in an integer (STRICT)"    "$author INSERT INTO blog_post (author_id, title, slug, body, views) VALUES (1, 't', 's', x'00', 'abc');"
run error "bool other than 0 or 1"         "$author INSERT INTO blog_post (author_id, title, slug, body, public) VALUES (1, 't', 's', x'00', 2);"
run ok    "NULL passes CHECK"              "$author INSERT INTO blog_post (author_id, title, slug, body, score) VALUES (1, 't', 's', x'00', NULL);"
run error "duplicate slug"                 "$author $post INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't2', 's', x'00');"
run error "delete a referenced author"     "$author $post DELETE FROM people;"
run ok    "ids are not reused"             "$author INSERT INTO people (name, editor_id, code) VALUES ('b', 2, 'c2'); DELETE FROM people WHERE id = 2; INSERT INTO people (name, editor_id, code) VALUES ('c', 3, 'c3'); SELECT CASE WHEN max(id) = 3 THEN 1 ELSE abs(-9223372036854775808) END FROM people;"
exit $fail
