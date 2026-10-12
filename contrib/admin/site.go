package admin

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/maago-web/maago"
)

// What Can is asked to allow (D-133): the words of the Action of the
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
	// model of the app named app (D-133): the copied auth's Can. ctx holds
	// the user (maago.UserFrom) and the database (maago.DBFrom). A model
	// whose view it refuses is not in the index, and its pages are 403.
	Can func(ctx context.Context, app, model, action string) bool
}

// pageSize is the rows of a page of a list (D-134).
const pageSize = 50

//go:embed templates admin.css
var files embed.FS

var pages = maago.Pages(files, template.FuncMap{"cell": cell, "css": css}, "index.html", "list.html")

// App is the admin's routes, under Prefix, for main's Config.Apps: each
// requires login (D-121). It panics on a Config that cannot work, as main
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
	return maago.NewApp("admin",
		maago.AppRoute{Method: "GET", Path: Prefix + "{$}", Name: "admin_index", LoginRequired: true, Serve: s.index},
		maago.AppRoute{Method: "GET", Path: Prefix + "{app}/{model}/{$}", Name: "admin_list", LoginRequired: true, Serve: s.list})
}

type site struct {
	can    func(ctx context.Context, app, model, action string) bool
	models []*Model            // in the order of Config.Models
	byKey  map[string]*Model   // by "app.Model"
	heads  map[*Model][]string // the labels of each model's List
}

// home is the index's path, under maago.Config.Prefix (D-122).
func home() string { return maago.Prefix() + Prefix }

func listURL(m *Model) string { return home() + m.App + "/" + m.Name + "/" }

type link struct{ Name, URL string }

// index lists the models the user may view, by app; none is 403 (D-131).
func (s *site) index(w http.ResponseWriter, r *http.Request) error {
	type app struct {
		Name   string
		Models []link
	}
	var apps []app
	for _, m := range s.models {
		if !s.can(r.Context(), m.App, m.Name, View) {
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
	return maago.Render(w, pages, "index.html", "", map[string]any{"Home": home(), "Apps": apps})
}

// list is a page of a model's rows. Can comes first, so a user it refuses
// learns nothing of which models there are. A page past the last is
// empty (D-134), and no rows are read for it.
func (s *site) list(w http.ResponseWriter, r *http.Request) error {
	ctx, app, name := r.Context(), r.PathValue("app"), r.PathValue("model")
	if !s.can(ctx, app, name, View) {
		return maago.Status(http.StatusForbidden, fmt.Errorf("admin: the user may not view %s.%s", app, name))
	}
	m := s.byKey[app+"."+name]
	if m == nil {
		return maago.Status(http.StatusNotFound, fmt.Errorf("admin: no model %s.%s", app, name))
	}
	q, err := maago.ParseQuery(r)
	if err != nil {
		return err
	}
	page, _, err := maago.Query(q, "page", func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }, false)
	if err != nil {
		return err
	}
	page = max(page, 1)
	db := maago.DBFrom(ctx)
	n, err := m.Count(ctx, db, Query{})
	if err != nil {
		return err
	}
	last := max(n/pageSize+min(n%pageSize, 1), 1) // no overflow near the largest count
	var rows []Row
	if page <= last {
		if rows, err = m.Rows(ctx, db, Query{Limit: pageSize, Offset: int(page-1) * pageSize}); err != nil {
			return err
		}
	}
	at := func(p int64) string {
		if p < 1 || p > last {
			return ""
		}
		return listURL(m) + "?page=" + strconv.FormatInt(p, 10)
	}
	return maago.Render(w, pages, "list.html", "", map[string]any{
		"Home": home(), "App": m.App, "Name": m.Name, "Heads": s.heads[m], "Rows": rows,
		"Count": n, "Page": page, "Pages": last, "Prev": at(min(page, last+1) - 1), "Next": at(page + 1),
	})
}

// cell is a cell's text; html/template escapes it. The types are the
// columns' (D-77), so another is a bug of maago.
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
		return maago.DateFunc("2006-01-02 15:04", v)
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
