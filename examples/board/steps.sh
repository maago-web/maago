#!/bin/sh
# steps.sh unpack DIR | pack DIR
#
# unpack turns the patches in steps/ into a git repository at DIR, one
# commit per step, ready to build against this checkout of maago. Change a
# step there with git rebase -i, then pack writes the commits back into
# steps/. Generated files, .env, the database and go.work stay out.
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
	git -C "$dir" -c user.name=Maago -c user.email=maago@example.com am -q "$here"/steps/*.patch
	printf '*_gen.go\n.env\n*.db\ngo.work*\n' >>"$dir/.git/info/exclude"
	printf 'go 1.25.0\n\nuse .\n\nreplace github.com/maago-web/maago => %s\n' "$repo" >"$dir/go.work"
	printf 'MAAGO_DATABASE_URL=sqlite:board.db\n' >"$dir/.env"
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
