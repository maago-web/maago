package web

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/diag"
)

// funcNames are the functions templates may call beside text/template's
// own: a closed list, enforced by parsing with only these.
var funcNames = template.FuncMap{
	"url":  func(string, ...any) (string, error) { return "", nil },
	"date": func(string, any) (string, error) { return "", nil },
}

// page is an HTML route's page, as checkTemplates sees it.
type page struct {
	route *Route
	typ   *types.Named
	label string // the view's position, for errors about its template
}

// checkTemplates checks the templates of one app (D-42): the files, then
// each page's set, walked with the page type as its dot, then executed
// once on nil to let html/template find what it cannot escape. The walk
// comes first, since executing rewrites the trees.
func checkTemplates(fset *token.FileSet, root, dir string, pkg *types.Package, pages []page, routes map[string][]param, viewPos map[*Route]token.Position) []diag.Diagnostic {
	tdir := path.Join(dir, "templates")
	fsys := os.DirFS(filepath.Join(root, filepath.FromSlash(dir)))
	entries, _ := fs.ReadDir(fsys, "templates")
	srcs := map[string][]byte{} // file name → content, for positions
	names := map[string]bool{}
	for _, p := range pages {
		names[p.route.view.template] = true
	}
	var ds []diag.Diagnostic
	for _, e := range entries {
		rel := path.Join(tdir, e.Name())
		switch {
		case e.IsDir():
			ds = append(ds, diag.New(token.Position{Filename: rel}, "directory in templates/", "keep the templates in templates/ itself"))
		case !strings.HasSuffix(e.Name(), ".html"):
		case len(pages) == 0:
			ds = append(ds, diag.New(token.Position{Filename: rel}, "template of an app without HTML pages", "remove it; only a view that returns a page renders a template"))
		case e.Name() != "base.html" && !names[e.Name()]:
			ds = append(ds, diag.New(token.Position{Filename: rel}, "template "+e.Name()+" is no page's and not base.html",
				"name it after a page type (PostPage → post_page.html), or move what it holds into a {{define}} of base.html"))
		default:
			srcs[e.Name()], _ = fs.ReadFile(fsys, "templates/"+e.Name())
		}
	}
	_, layout := srcs["base.html"]
	entry := func(file string) string {
		if layout {
			return "base.html"
		}
		return file
	}
	var files []string // the page files, each once
	byFile := map[string][]page{}
	for _, p := range pages {
		file := p.route.view.template
		if _, ok := srcs[file]; !ok {
			ds = append(ds, diag.New(viewPos[p.route], fmt.Sprintf("page %s has no template %s", p.label, path.Join(tdir, file)),
				"create it; a page type's template is its name in snake_case (D-116)"))
			continue
		}
		if byFile[file] == nil {
			files = append(files, file)
		}
		byFile[file] = append(byFile[file], p)
	}
	pos := func(file string, offset int) token.Position {
		src := srcs[file]
		line := 1 + strings.Count(string(src[:offset]), "\n")
		col := offset - strings.LastIndexByte(string(src[:offset]), '\n')
		return token.Position{Filename: path.Join(tdir, file), Line: line, Column: col}
	}
	for _, file := range files {
		parsed, err := maago.ParsePages(fsys, funcNames, file)
		if err != nil {
			ds = append(ds, templateDiag(tdir, err))
			continue
		}
		set := parsed[file]
		where := whereIs(set) // before executing: a failed escape drops the trees
		tds := topLevel(set, file, layout, pos)
		typ := byFile[file][0].typ // a file is one page type's
		dot := val{t: typ, label: typ.Obj().Name()}
		w := &walker{pkg: pkg, set: set, routes: routes, pos: pos, done: map[string]bool{}}
		w.template(entry(file), dot, nil)
		var blocks []string
		for _, p := range byFile[file] {
			for _, o := range p.route.Options {
				switch {
				case o.Name != "Block" || slices.Contains(blocks, o.Block):
				case set.Lookup(o.Block) == nil:
					tds = append(tds, diag.New(fset.Position(o.Pos), fmt.Sprintf("template %s has no block %q", path.Join(tdir, file), o.Block),
						"name a {{block}} or {{define}} of the page or of base.html"))
				default:
					blocks = append(blocks, o.Block)
					w.template(o.Block, dot, nil) // rendered with the page, as the route renders it
				}
			}
		}
		if tds = append(tds, w.ds...); len(tds) > 0 {
			ds = append(ds, tds...)
			continue
		}
		if err := set.ExecuteTemplate(io.Discard, entry(file), nil); escapeError(err) {
			ds = append(ds, escapeDiag(where, err, pos, ""))
			continue
		}
		for _, b := range blocks {
			// Sent alone, it must escape alone: in a new set, since the page's
			// execution escaped it already, without the end check.
			fresh, _ := maago.ParsePages(fsys, funcNames, file)
			if err := fresh[file].ExecuteTemplate(io.Discard, b, nil); escapeError(err) {
				ds = append(ds, escapeDiag(where, err, pos, "block "+b+", sent alone, "))
			}
		}
	}
	return ds
}

// where tells the file of a template, and of a node of the set: an escape
// error names the template executed, not the one that holds its node.
type where struct {
	names map[string]string
	nodes map[nodeKey]string // "": in two files
}

// nodeKey finds a node again in html/template's copies of a tree.
type nodeKey struct {
	typ parse.NodeType
	pos parse.Pos
}

func whereIs(set *template.Template) where {
	w := where{map[string]string{}, map[nodeKey]string{}}
	var list func(file string, l *parse.ListNode)
	list = func(file string, l *parse.ListNode) {
		if l == nil {
			return
		}
		for _, n := range l.Nodes {
			k := nodeKey{n.Type(), n.Position()}
			if f, ok := w.nodes[k]; ok && f != file {
				w.nodes[k] = ""
			} else {
				w.nodes[k] = file
			}
			switch n := n.(type) {
			case *parse.IfNode:
				list(file, n.List)
				list(file, n.ElseList)
			case *parse.RangeNode:
				list(file, n.List)
				list(file, n.ElseList)
			case *parse.WithNode:
				list(file, n.List)
				list(file, n.ElseList)
			}
		}
	}
	for _, t := range set.Templates() {
		if t.Tree != nil {
			w.names[t.Name()] = t.Tree.ParseName
			list(t.Tree.ParseName, t.Tree.Root)
		}
	}
	return w
}

func blockOf(r *Route) string {
	for _, o := range r.Options {
		if o.Name == "Block" {
			return o.Block
		}
	}
	return ""
}

// topLevel checks what the page file holds outside its {{define}}s: with
// base.html it is never rendered, so it must be blank; without, it is the
// page, so it must be there.
func topLevel(set *template.Template, file string, layout bool, pos func(string, int) token.Position) []diag.Diagnostic {
	t := set.Lookup(file)
	blank := true
	var first parse.Node
	if t != nil && t.Tree != nil {
		for _, n := range t.Tree.Root.Nodes {
			if tn, ok := n.(*parse.TextNode); ok && strings.TrimSpace(string(tn.Text)) == "" {
				continue
			}
			blank, first = false, n
			break
		}
	}
	switch {
	case layout && !blank:
		return []diag.Diagnostic{diag.New(pos(file, int(first.Position())), "content outside {{define}} is never rendered: pages render through base.html",
			"put it in a {{define}} of a block that base.html renders, such as {{define \"content\"}}")}
	case !layout && blank:
		return []diag.Diagnostic{diag.New(token.Position{Filename: pos(file, 0).Filename}, "page renders nothing: without base.html the file itself is the page",
			"write the page outside the {{define}}s, or add templates/base.html")}
	}
	return nil
}

// templateError is html/template's "template: NAME:LINE: message".
var templateError = regexp.MustCompile(`^template: ([^:]+):(\d+): (.*)$`)

// templateDiag places a parse error of a set.
func templateDiag(tdir string, err error) diag.Diagnostic {
	pos := token.Position{Filename: tdir}
	msg := err.Error()
	if m := templateError.FindStringSubmatch(msg); m != nil {
		pos.Filename = path.Join(tdir, m[1])
		pos.Line, _ = strconv.Atoi(m[2])
		msg = m[3]
	}
	return diag.New(pos, diag.OneLine(msg), "fix the template; it is Go's html/template")
}

// escapeError reports whether err is html/template's own: what it cannot
// escape. Executing on nil fails anyway; that is not an escape error.
func escapeError(err error) bool {
	var e *template.Error
	return errors.As(err, &e) && e.ErrorCode != template.ErrNoSuchTemplate
}

// escapeDiag places an escape error: at its node if one file has it, else
// in the file of the template it names.
func escapeDiag(w where, err error, pos func(string, int) token.Position, what string) diag.Diagnostic {
	var e *template.Error
	errors.As(err, &e)
	at := token.Position{Filename: pos(w.names[e.Name], 0).Filename}
	if e.Node != nil {
		if f := w.nodes[nodeKey{e.Node.Type(), e.Node.Position()}]; f != "" {
			at = pos(f, int(e.Node.Position()))
		}
	}
	msg := e.Description
	if msg == "" {
		msg = e.Error()
	}
	msg, _, _ = strings.Cut(msg, ": {state") // html/template's context, as Go values
	return diag.New(at, what+diag.OneLine(msg), "see html/template's escaping rules: an action cannot sit where the HTML cannot hold it")
}
