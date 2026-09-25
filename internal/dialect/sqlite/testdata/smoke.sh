#!/bin/sh
# Applies example.sql to an in-memory SQLite and checks that the schema
# accepts what it should and refuses what it should, for the reason it
# should. Run from anywhere: sh smoke.sh (needs the sqlite3 command).
set -u
cd "$(dirname "$0")" || exit 1
fail=0
run() { # run <want: ok, or the expected error text> <description> <sql>
	out=$(sqlite3 -bail :memory: ".read example.sql" "PRAGMA foreign_keys = ON;" "$3" 2>&1)
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
post="INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't', 's', x'00');"

run ok    "schema applies"                 "SELECT 1;"
run ok    "NULL id is generated"           "$author SELECT CASE WHEN id = 1 THEN 1 ELSE abs(-9223372036854775808) END FROM people;"
run ok    "defaults fill in"               "$author $post SELECT CASE WHEN status = 'draft' AND public = 1 AND views = 0 AND score = 1.5 AND (SELECT admin FROM people) = 0 THEN 1 ELSE abs(-9223372036854775808) END FROM blog_post;"
run "FOREIGN KEY constraint failed" "FK to a missing author"         "$post"
run "CHECK constraint failed: status" "choice outside the list"        "$author INSERT INTO blog_post (author_id, title, slug, body, status) VALUES (1, 't', 's', x'00', 'x');"
run "CHECK constraint failed: length(\"title\") <= 200" "title over 200 characters"      "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, printf('%.201c', 'a'), 's', x'00');"
run ok    "200 Korean characters fit"      "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, replace(printf('%.200c', 'x'), 'x', '가'), 's', x'00');"
run "cannot store TEXT value in INTEGER column blog_post.views" "text in an integer (STRICT)"    "$author INSERT INTO blog_post (author_id, title, slug, body, views) VALUES (1, 't', 's', x'00', 'abc');"
run "CHECK constraint failed: public" "bool other than 0 or 1"         "$author INSERT INTO blog_post (author_id, title, slug, body, public) VALUES (1, 't', 's', x'00', 2);"
run ok    "NULL passes CHECK"              "$author INSERT INTO blog_post (author_id, title, slug, body, score) VALUES (1, 't', 's', x'00', NULL);"
run "UNIQUE constraint failed" "duplicate slug"                 "$author $post INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't2', 's', x'00');"
run "FOREIGN KEY constraint failed" "delete a referenced author"     "$author $post DELETE FROM people;"
run "NOT NULL constraint failed: blog_post.title" "NULL into a NOT NULL column" "$author INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, NULL, 's', x'00');"
run ok    "ids are not reused"             "$author INSERT INTO people (name, editor_id, code) VALUES ('b', 2, 'c2'); DELETE FROM people WHERE id = 2; INSERT INTO people (name, editor_id, code) VALUES ('c', 3, 'c3'); SELECT CASE WHEN max(id) = 3 THEN 1 ELSE abs(-9223372036854775808) END FROM people;"
exit $fail
