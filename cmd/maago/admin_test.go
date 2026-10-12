package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
)

// maago gen writes admin_gen.go only when a file of the project, not a
// test, imports contrib/admin, and removes it when none does; the admin
// needs each app's Admin, and no route of the project's own under /admin/
// (D-131).
func TestAdminGen(t *testing.T) {
	root := webProject(t)
	gen := filepath.Join(root, "site", "admin_gen.go")
	main := readFile(t, filepath.Join(root, "main.go"))
	exists := func() bool {
		_, err := os.Stat(gen)
		return err == nil
	}
	mustGen := func() {
		t.Helper()
		if _, ds := build.Gen(root); ds != nil {
			t.Fatalf("maago gen:\n%s", text(ds))
		}
	}
	withAdmin := func(use string) string {
		return strings.Replace(main, `"github.com/maago-web/maago"`, `"github.com/maago-web/maago"
	"github.com/maago-web/maago/contrib/admin"`, 1) + use
	}

	mustGen()
	if exists() {
		t.Error("admin_gen.go without contrib/admin")
	}
	writeFile(t, filepath.Join(root, "site", "admin_test.go"), "package site\n\nimport _ \"github.com/maago-web/maago/contrib/admin\"\n")
	mustGen()
	if exists() {
		t.Error("admin_gen.go when only a test imports contrib/admin")
	}

	writeFile(t, filepath.Join(root, "main.go"), withAdmin("\nvar _ []admin.Model = site.Admin\n"))
	mustGen()
	if !exists() {
		t.Fatal("no admin_gen.go when main imports contrib/admin")
	}
	if ds := build.Check(root); ds != nil {
		t.Fatalf("maago check after gen:\n%s", text(ds))
	}

	writeFile(t, filepath.Join(root, "main.go"), withAdmin("\nvar _ admin.Query\n"))
	want := "app site has models, but no package uses site.Admin"
	if ds := build.Check(root); !strings.Contains(text(ds), want) {
		t.Errorf("check without site.Admin:\n%s\nwant %q", text(ds), want)
	}

	urls := filepath.Join(root, "site", "urls.go")
	writeFile(t, filepath.Join(root, "main.go"), withAdmin("\nvar _ []admin.Model = site.Admin\n"))
	list := readFile(t, urls)
	for _, p := range []string{"/admin/list", "/admin"} {
		writeFile(t, urls, strings.Replace(list, `maago.GET("/list", `, `maago.GET("`+p+`", `, 1))
		want = "route " + p + " is under /admin/, which the admin serves"
		if ds := build.Check(root); !strings.Contains(text(ds), want) {
			t.Errorf("check with a route at %s:\n%s\nwant %q", p, text(ds), want)
		}
	}
	writeFile(t, urls, list)

	// Admin is the admin's name only in a project that uses the admin.
	clash := filepath.Join(root, "site", "clash.go")
	writeFile(t, clash, "package site\n\nvar Admin = 1\n")
	want = "the admin would declare Admin, which site/clash.go:3 declares already"
	if ds := build.Check(root); !strings.Contains(text(ds), want) {
		t.Errorf("check with Admin declared:\n%s\nwant %q", text(ds), want)
	}
	writeFile(t, filepath.Join(root, "main.go"), main)
	if ds := build.Check(root); strings.Contains(text(ds), "declare Admin") {
		t.Errorf("Admin clashes without the admin:\n%s", text(ds))
	}
	os.Remove(clash)

	want = "site/admin_gen.go: generated file is no longer generated"
	if ds := build.Check(root); !strings.Contains(text(ds), want) {
		t.Errorf("check after the import is gone:\n%s\nwant %q", text(ds), want)
	}
	mustGen()
	if exists() {
		t.Error("gen left admin_gen.go after the import is gone")
	}
}
