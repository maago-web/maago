#!/bin/sh
# steps.sh unpack DIR | pack DIR
#
# unpack turns the patches in steps/ into a git repository at DIR, one
# commit per step, ready to build against this checkout of maago, whose
# maago it builds into DIR/.git/maago. A step whose message says
# Maago-Run: maago add APP has it run first; one whose message says
# Maago-Database: postgres points .env at a local PostgreSQL database named
# board from there on (D-141). Change a step there with git
# rebase, then pack writes the commits back into steps/. Generated files,
# what maago add writes, .env, the database and go.work stay out.
set -eu
usage() {
	echo "usage: $0 unpack|pack DIR" >&2
	exit 2
}
[ $# -eq 2 ] || usage
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd -P)
dir=$2
case $1 in
unpack)
	[ ! -e "$dir" ] || { echo "$dir exists" >&2; exit 1; }
	trap 'rm -rf "$dir"' EXIT # a half-made DIR would block the next try
	git init -q "$dir"
	printf '*_gen.go\n/auth/\n.env\n*.db\ngo.work*\n' >>"$dir/.git/info/exclude"
	printf 'go 1.25.0\n\nuse .\n\nreplace github.com/maago-web/maago => %s\n' "$repo" >"$dir/go.work"
	printf 'MAAGO_DATABASE_URL=sqlite:board.db\n' >"$dir/.env"
	go build -C "$repo/cmd/maago" -o "$(cd "$dir" && pwd -P)/.git/maago" .
	for p in "$here"/steps/*.patch; do
		# Before maago add: it picks the test driver by .env (D-123).
		if [ -n "$(sed -n '/^diff --git /q; /^Maago-Database: postgres$/p' "$p")" ]; then
			printf 'MAAGO_DATABASE_URL=postgres://localhost:5432/board\n' >"$dir/.env"
		fi
		for app in $(sed -n '/^diff --git /q; s/^Maago-Run: maago add //p' "$p"); do
			(cd "$dir" && .git/maago add "$app" >/dev/null)
		done
		git -C "$dir" -c user.name=Maago -c user.email=maago@example.com am -q "$p"
	done
	trap - EXIT
	;;
pack)
	# The old patches go only once the new ones are written.
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	git -C "$dir" format-patch -q --root -N --zero-commit --no-signature --no-stat -o "$tmp"
	rm -f "$here"/steps/*.patch
	mv "$tmp"/*.patch "$here/steps/"
	;;
*) usage ;;
esac
