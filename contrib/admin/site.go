package admin

import (
	"cmp"
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/rt"
)

// What Can is asked to allow: the words of the Action of the
// copied auth's Permission rows.
const (
	View   = "view"
	Add    = "add"
	Change = "change"
	Delete = "delete"
)

// Config is what main gives the admin.
type Config struct {
	// Models are each app's Admin: [][]admin.Model{board.Admin, auth.Admin}.
	Models [][]Model

	// Can tells whether the user of ctx may do action to the model named
	// model of the app named app: the copied auth's Can. ctx holds
	// the user (maago.UserFrom) and the database (maago.DBFrom). A model
	// whose view it refuses is not in the index, and its pages are 403.
	// Its error, such as the database's, answers 500.
	Can func(ctx context.Context, app, model, action string) (bool, error)
}

// pageSize is the rows of a page of a list.
const pageSize = 50

//go:embed templates admin.css
var files embed.FS

var pages = rt.Pages(files, template.FuncMap{"cell": cell, "css": css}, "index.html", "list.html", "form.html", "delete.html")

// App is the admin's routes, under Prefix, for main's Config.Apps: each
// requires login. It panics on a Config that cannot work, as main
// makes it once at start.
func App(cfg Config) maago.App {
	if cfg.Can == nil {
		panic("admin: Config.Can is nil; set it to the copied auth's Can")
	}
	s := &site{can: cfg.Can, byKey: map[string]*Model{}, heads: map[*Model][]string{}}
	for _, ms := range cfg.Models {
		for i := range ms {
			m := &ms[i]
			if s.byKey[m.App+"."+m.Name] != nil {
				panic(fmt.Sprintf("admin: model %s.%s is in Config.Models twice", m.App, m.Name))
			}
			s.byKey[m.App+"."+m.Name] = m
			s.models = append(s.models, m)
			for _, name := range slices.Concat(m.Search, filterFields(m.Filters)) {
				if !slices.ContainsFunc(m.Fields, func(f Field) bool { return f.Name == name }) {
					panic(fmt.Sprintf("admin: %s.%s searches or filters by %s, which is not among its Fields; run maago gen", m.App, m.Name, name))
				}
			}
			for _, name := range m.List {
				i := slices.IndexFunc(m.Fields, func(f Field) bool { return f.Name == name })
				if i < 0 {
					panic(fmt.Sprintf("admin: %s.%s lists %s, which is not among its Fields; run maago gen", m.App, m.Name, name))
				}
				s.heads[m] = append(s.heads[m], m.Fields[i].Label)
			}
		}
	}
	if len(s.models) == 0 {
		panic("admin: Config.Models has no model; add each app's Admin, such as board.Admin (a model with admin=hidden is not there)")
	}
	route := func(method, path, name string, serve func(http.ResponseWriter, *http.Request) error) rt.AppRoute {
		return rt.AppRoute{Method: method, Path: Prefix + path, Name: name, LoginRequired: true, Serve: serve}
	}
	return rt.NewApp("admin", nil,
		route("GET", "{$}", "admin_index", s.index),
		route("GET", "{app}/{model}/{$}", "admin_list", s.list),
		route("GET", "{app}/{model}/delete/{$}", "admin_delete", s.delete), // the literals are more specific than {id}
		route("POST", "{app}/{model}/delete/{$}", "admin_delete_submit", s.delete),
		route("GET", "{app}/{model}/add/{$}", "admin_add", s.add),
		route("POST", "{app}/{model}/add/{$}", "admin_add_submit", s.add),
		route("GET", "{app}/{model}/{id}/{$}", "admin_row", s.row),
		route("POST", "{app}/{model}/{id}/{$}", "admin_row_submit", s.row))
}

func filterFields(fs []Filter) []string {
	var names []string
	for _, f := range fs {
		names = append(names, f.Field)
	}
	return names
}

type site struct {
	can    func(ctx context.Context, app, model, action string) (bool, error)
	models []*Model            // in the order of Config.Models
	byKey  map[string]*Model   // by "app.Model"
	heads  map[*Model][]string // the labels of each model's List
}

// home is the index's path, under maago.Config.Prefix.
func home() string { return maago.Prefix() + Prefix }

func listURL(m *Model) string { return home() + m.App + "/" + m.Name + "/" }

func rowURL(m *Model, id int64) string { return listURL(m) + strconv.FormatInt(id, 10) + "/" }

type link struct{ Name, URL string }

// index lists the models the user may view, by app; none is 403.
func (s *site) index(w http.ResponseWriter, r *http.Request) error {
	type app struct {
		Name   string
		Models []link
	}
	var apps []app
	for _, m := range s.models {
		ok, err := s.can(r.Context(), m.App, m.Name, View)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if len(apps) == 0 || apps[len(apps)-1].Name != m.App {
			apps = append(apps, app{Name: m.App})
		}
		a := &apps[len(apps)-1]
		a.Models = append(a.Models, link{m.Name, listURL(m)})
	}
	if apps == nil {
		return maago.Status(http.StatusForbidden, fmt.Errorf("admin: the user may view no model"))
	}
	return rt.Render(w, pages, "index.html", "", map[string]any{"Home": home(), "Apps": apps})
}

// list is a page of a model's rows. Can comes first, so a user it refuses
// learns nothing of which models there are. A page past the last is
// empty, and no rows are read for it.
func (s *site) list(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	m, err := s.model(r, View)
	if err != nil {
		return err
	}
	q, err := rt.ParseQuery(r)
	if err != nil {
		return err
	}
	page, _, err := rt.Query(q, "page", func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }, false)
	if err != nil {
		return err
	}
	page = max(page, 1)
	text := func(s string) (string, error) { return s, nil }
	query := Query{Filters: map[string]string{}}
	if query.Search, _, err = rt.Query(q, "q", text, false); err != nil {
		return err
	}
	query.Search = strings.TrimSpace(query.Search)
	for _, f := range m.Filters {
		if query.Filters[f.Key], _, err = rt.Query(q, f.Key, text, false); err != nil {
			return err
		}
	}
	db := maago.DBFrom(ctx)
	n, err := m.Count(ctx, db, query) // first: a bad filter value is a 400
	if err != nil {
		return err
	}
	last := max(n/pageSize+min(n%pageSize, 1), 1) // no overflow near the largest count
	var rows []Row
	if page <= last {
		query.Limit, query.Offset = pageSize, int(page-1)*pageSize
		if rows, err = m.Rows(ctx, db, query); err != nil {
			return err
		}
	}
	type rowView struct {
		ID    int64
		URL   string
		Cells []any
	}
	views := make([]rowView, len(rows))
	for i, row := range rows {
		views[i] = rowView{row.ID, rowURL(m, row.ID), row.Cells}
	}
	deleteURL := ""
	if ok, err := s.can(ctx, m.App, m.Name, Delete); err != nil {
		return err
	} else if ok {
		deleteURL = listURL(m) + "delete/"
	}
	addURL := ""
	if m.Add {
		ok, err := s.can(ctx, m.App, m.Name, Add)
		if err != nil {
			return err
		}
		if ok {
			addURL = listURL(m) + "add/"
		}
	}
	// The conditions in force, which the links of pages keep; a link of a
	// filter, or the search, starts again at page 1.
	kept := url.Values{}
	if query.Search != "" {
		kept.Set("q", query.Search)
	}
	for k, v := range query.Filters {
		if v != "" {
			kept.Set(k, v)
		}
	}
	link := func(vals url.Values) string {
		if len(vals) == 0 {
			return listURL(m)
		}
		return listURL(m) + "?" + vals.Encode()
	}
	at := func(p int64) string {
		if p < 1 || p > last {
			return ""
		}
		vals := maps.Clone(kept)
		vals.Set("page", strconv.FormatInt(p, 10))
		return link(vals)
	}
	type option struct {
		Text, URL string
		On        bool
	}
	type filterView struct {
		Key, Label, Value string
		Options           []option // choices and bools: links; an fk: none, a number in the search form
	}
	var filters []filterView
	var hidden []filterView // the filters the search form keeps, as they are links
	for _, f := range m.Filters {
		v := filterView{Key: f.Key, Label: m.label(f.Field), Value: query.Filters[f.Key]}
		values := f.Choices
		if f.Kind == "bool" {
			values = []string{"yes", "no"}
		}
		if f.Kind != "fk" {
			for _, c := range append([]string{""}, values...) {
				vals := maps.Clone(kept)
				vals.Del(f.Key)
				text := cmp.Or(c, "All")
				if c != "" {
					vals.Set(f.Key, c)
				}
				v.Options = append(v.Options, option{text, link(vals), c == v.Value})
			}
			if v.Value != "" {
				hidden = append(hidden, v)
			}
		}
		filters = append(filters, v)
	}
	var search []string
	for _, name := range m.Search {
		search = append(search, m.label(name))
	}
	return rt.Render(w, pages, "list.html", "", map[string]any{
		"Home": home(), "App": m.App, "Name": m.Name, "Heads": s.heads[m], "Rows": views, "Add": addURL, "Delete": deleteURL,
		"Search": strings.Join(search, ", "), "Q": query.Search, "Filters": filters, "Hidden": hidden, "Action": listURL(m),
		"Box":   search != nil || slices.ContainsFunc(m.Filters, func(f Filter) bool { return f.Kind == "fk" }),
		"Count": n, "Page": page, "Pages": last, "Prev": at(min(page, last+1) - 1), "Next": at(page + 1),
	})
}

// label is the label of m's field named name, which App checked is there.
func (m *Model) label(name string) string {
	i := slices.IndexFunc(m.Fields, func(f Field) bool { return f.Name == name })
	return m.Fields[i].Label
}

// model is the model of r's path, if Can lets the user do action to it.
// Can comes first, so a user it refuses learns nothing of which models
// there are: 403 whether the model is there or not, then 404.
func (s *site) model(r *http.Request, action string) (*Model, error) {
	app, name := r.PathValue("app"), r.PathValue("model")
	ok, err := s.can(r.Context(), app, name, action)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, maago.Status(http.StatusForbidden, fmt.Errorf("admin: the user may not %s %s.%s", action, app, name))
	}
	m := s.byKey[app+"."+name]
	if m == nil {
		return nil, maago.Status(http.StatusNotFound, fmt.Errorf("admin: no model %s.%s", app, name))
	}
	return m, nil
}

// add is the form of a new row; a valid POST makes it and goes to the
// list.
func (s *site) add(w http.ResponseWriter, r *http.Request) error {
	m, err := s.model(r, Add)
	if err != nil {
		return err
	}
	if !m.Add {
		return maago.Status(http.StatusNotFound, fmt.Errorf("admin: %s.%s has no adding", m.App, m.Name))
	}
	return s.form(w, r, m, 0, true)
}

// row is the form of the row of the path's id: to read with View, to
// save with Change. Without Change, or with nothing to change, it is
// drawn to read only.
func (s *site) row(w http.ResponseWriter, r *http.Request) error {
	action := View
	if r.Method == http.MethodPost {
		action = Change
	}
	m, err := s.model(r, action)
	if err != nil {
		return err
	}
	id, err := rt.Path(r, "id", func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) })
	if err != nil {
		return err
	}
	save := m.Edit && action == Change
	if m.Edit && action == View {
		if save, err = s.can(r.Context(), m.App, m.Name, Change); err != nil {
			return err
		}
	}
	if action == Change && !m.Edit {
		return maago.Status(http.StatusForbidden, fmt.Errorf("admin: %s.%s has nothing to change", m.App, m.Name))
	}
	return s.form(w, r, m, id, save)
}

// form reads, saves and draws m's form of the row with id, or of a new
// row when id is 0.
func (s *site) form(w http.ResponseWriter, r *http.Request, m *Model, id int64, save bool) error {
	ctx := r.Context()
	vals, err := rt.FormValues(r)
	if err != nil {
		return err
	}
	for k, vs := range vals { // no input of the admin's sends a key twice: the value would be unclear
		if len(vs) > 1 {
			return maago.Status(http.StatusBadRequest, fmt.Errorf("form key %q is given %d times", k, len(vs)))
		}
	}
	f, err := m.Form(ctx, maago.DBFrom(ctx), id, vals)
	conflict := errors.Is(err, maago.ErrConflict) // drawn again, the inputs as given
	if err != nil && !conflict {
		return err
	}
	if f.Saved {
		http.Redirect(w, r, listURL(m), http.StatusSeeOther)
		return nil
	}
	type inputView struct {
		Input
		Link string // the row an fk field's number is of, when the user may view it
	}
	inputs := make([]inputView, len(f.Inputs))
	for i, in := range f.Inputs {
		inputs[i].Input = in
		target := s.byKey[in.FK]
		n, err := strconv.ParseInt(in.Raw, 10, 64)
		if target == nil || err != nil || in.Error != "" {
			continue
		}
		ok, err := s.can(ctx, target.App, target.Name, View)
		if err != nil {
			return err
		}
		if ok {
			inputs[i].Link = rowURL(target, n)
		}
	}
	title, deleteURL := "Add "+m.Name, ""
	if id != 0 {
		title = m.Name + " " + strconv.FormatInt(id, 10)
		if ok, err := s.can(ctx, m.App, m.Name, Delete); err != nil {
			return err
		} else if ok {
			deleteURL = listURL(m) + "delete/?id=" + strconv.FormatInt(id, 10)
		}
	}
	version := ""
	if !f.Version.IsZero() {
		version = strconv.FormatInt(f.Version.UnixMicro(), 10)
	}
	if conflict {
		w = &conflicted{ResponseWriter: w}
	}
	return rt.Render(w, pages, "form.html", "", map[string]any{
		"Home": home(), "App": m.App, "Name": m.Name, "List": listURL(m), "Title": title, "Inputs": inputs, "Save": save, "Delete": deleteURL,
		"Version": version, "Conflict": conflict,
	})
}

// conflicted is w for a form whose row changed since it was opened: a 409.
type conflicted struct {
	http.ResponseWriter
	wrote bool
}

func (w *conflicted) Write(p []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		w.WriteHeader(http.StatusConflict)
	}
	return w.ResponseWriter.Write(p)
}

// delete deletes the rows of the ids, all of them or none: a GET
// shows each row, and what refers to it; a POST counts again, and
// deletes them, in the transaction it counted in, only if nothing refers
// to any and all are there, else shows the same page.
func (s *site) delete(w http.ResponseWriter, r *http.Request) error {
	m, err := s.model(r, Delete)
	if err != nil {
		return err
	}
	ids, err := deleteIDs(r)
	if errors.Is(err, errNoRows) && r.Method == http.MethodGet { // Delete selected with nothing selected
		http.Redirect(w, r, listURL(m), http.StatusSeeOther)
		return nil
	}
	if err != nil {
		return err
	}
	ctx := r.Context()
	var db maago.DB = maago.DBFrom(ctx)
	var tx *sql.Tx
	if r.Method == http.MethodPost { // count and delete in one transaction
		if tx, err = maago.DBFrom(ctx).BeginTx(ctx, nil); err != nil {
			return err
		}
		defer tx.Rollback()
		db = tx
	}
	type rowView struct {
		ID      int64
		URL     string
		Missing bool
		Refs    []Ref // those with rows
	}
	var rows []rowView
	ok := true
	for _, id := range ids {
		v := rowView{ID: id, URL: rowURL(m, id)}
		refs, err := m.Refs(ctx, db, id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			v.Missing = true
		case err != nil:
			return err
		}
		for _, ref := range refs {
			if ref.N > 0 {
				v.Refs = append(v.Refs, ref)
			}
		}
		ok = ok && !v.Missing && v.Refs == nil
		rows = append(rows, v)
	}
	if tx != nil && ok {
		for _, id := range ids {
			if err := m.Delete(ctx, tx, id); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		http.Redirect(w, r, listURL(m), http.StatusSeeOther)
		return nil
	}
	return rt.Render(w, pages, "delete.html", "", map[string]any{
		"Home": home(), "App": m.App, "Name": m.Name, "List": listURL(m), "Rows": rows, "OK": ok,
	})
}

var errNoRows = errors.New("admin: no row to delete")

// deleteIDs are the ids to delete: the query's of a GET, the form's of a
// POST, each once. None, one that is not a number, or more than a page
// of the list holds, is a 400.
func deleteIDs(r *http.Request) ([]int64, error) {
	var vals url.Values
	var err error
	if r.Method == http.MethodPost {
		vals, err = rt.FormValues(r)
	} else {
		vals, err = rt.ParseQuery(r)
	}
	if err != nil {
		return nil, err
	}
	raw := vals["id"]
	switch {
	case len(raw) == 0:
		return nil, maago.Status(http.StatusBadRequest, errNoRows)
	case len(raw) > pageSize:
		return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("admin: %d rows to delete; a page of the list holds %d", len(raw), pageSize))
	}
	var ids []int64
	for _, s := range raw {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("admin: id %q: %w", s, err))
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// cell is a cell's text; html/template escapes it. The types are the
// columns', so another is a bug of maago.
func cell(v any) (string, error) {
	switch v := v.(type) {
	case string:
		return v, nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		if v {
			return "yes", nil
		}
		return "no", nil
	case time.Time:
		return rt.DateFunc("2006-01-02 15:04", v)
	case []byte:
		return fmt.Sprintf("(%d bytes)", len(v)), nil
	case *string:
		return ptr(v)
	case *int64:
		return ptr(v)
	case *int32:
		return ptr(v)
	case *float64:
		return ptr(v)
	case *bool:
		return ptr(v)
	case *time.Time:
		return ptr(v)
	case *[]byte:
		return ptr(v)
	}
	return "", fmt.Errorf("admin: a cell of type %T; this is a bug in maago", v)
}

// ptr is the cell of *p, or nothing for nil.
func ptr[T any](p *T) (string, error) {
	if p == nil {
		return "", nil
	}
	return cell(*p)
}

var style = func() template.CSS {
	b, err := files.ReadFile("admin.css")
	if err != nil {
		panic(err)
	}
	return template.CSS(b)
}()

func css() template.CSS { return style }
