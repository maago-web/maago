package web

import (
	"fmt"
	"go/token"
	"go/types"
	"html/template"
	"slices"
	"text/template/parse"
	"unicode"
	"unicode/utf8"

	"github.com/maago-web/maago/internal/diag"
)

// val is what a template expression is, as far as the walk knows.
type val struct {
	t      types.Type // nil: no known type
	label  string     // how errors name it: PostPage.Post
	konst  string     // a constant's kind: "string", "int", "float", "bool", "nil"
	nilDot bool       // the dot of a {{template}} called without a value
	bad    bool       // an error was reported on the way here: stay quiet
	addr   bool       // addressable as text/template reaches it: past a pointer, an element of a slice
}

// walker walks one page's template set with the page type as the dot,
// as text/template would execute it (D-42).
type walker struct {
	pkg    *types.Package
	set    *template.Template
	routes map[string][]param
	pos    func(file string, offset int) token.Position
	done   map[string]bool // (template, dot type) walked
	file   string          // the file of the template walked now
	ds     []diag.Diagnostic
}

type scope struct {
	vars map[string]val
	up   *scope
}

func (s *scope) push() *scope { return &scope{map[string]val{}, s} }

func (s *scope) lookup(name string) (val, *scope) {
	for ; s != nil; s = s.up {
		if v, ok := s.vars[name]; ok {
			return v, s
		}
	}
	return val{}, nil
}

func (w *walker) errAt(n parse.Node, extra int, msg, fix string) {
	w.ds = append(w.ds, diag.New(w.pos(w.file, int(n.Position())+extra), diag.OneLine(msg), fix))
}

// template walks the template name with dot, once for each dot type. Its
// $ is dot: the value the call passed.
func (w *walker) template(name string, dot val, at parse.Node) {
	t := w.set.Lookup(name)
	if t == nil || t.Tree == nil {
		if at != nil {
			w.errAt(at, 0, fmt.Sprintf("no template %q", name), "define it with {{define \""+name+"\"}}, or name one the set has")
		}
		return
	}
	key := name + "|" + typeKey(dot)
	if w.done[key] {
		return
	}
	w.done[key] = true
	file := w.file
	w.file = t.Tree.ParseName
	w.list(t.Tree.Root, dot, &scope{vars: map[string]val{"$": dot}})
	w.file = file
}

func typeKey(v val) string {
	switch {
	case v.nilDot:
		return "nil"
	case v.t == nil:
		return "?"
	}
	if v.addr {
		return "&" + types.TypeString(v.t, nil)
	}
	return types.TypeString(v.t, nil)
}

func (w *walker) list(l *parse.ListNode, dot val, sc *scope) {
	if l == nil {
		return
	}
	for _, n := range l.Nodes {
		switch n := n.(type) {
		case *parse.ActionNode:
			w.pipe(n.Pipe, dot, sc)
		case *parse.IfNode:
			inner := sc.push()
			w.pipe(n.Pipe, dot, inner)
			w.list(n.List, dot, inner.push())
			w.list(n.ElseList, dot, inner.push())
		case *parse.WithNode:
			inner := sc.push()
			v := w.pipe(n.Pipe, dot, inner)
			w.list(n.List, v, inner.push())
			w.list(n.ElseList, dot, inner.push())
		case *parse.RangeNode:
			w.rangeNode(n, dot, sc)
		case *parse.TemplateNode:
			arg := val{nilDot: true}
			if n.Pipe != nil {
				arg = w.pipe(n.Pipe, dot, sc.push())
			}
			w.template(n.Name, arg, n)
		}
	}
}

// rangeNode walks a range: its body with the element as the dot, and its
// variables as text/template sets them: one is the element, two are the
// key or index and the element.
func (w *walker) rangeNode(n *parse.RangeNode, dot val, sc *scope) {
	inner := sc.push()
	v := w.pipe(n.Pipe, dot, inner) // declares the variables as v: set again below
	decl := n.Pipe.Decl
	key, elem := val{label: "a key of " + v.label, bad: v.bad}, val{label: "an element of " + v.label, bad: v.bad}
	if v.t != nil && types.IsInterface(deref(v.t)) {
		w.errAt(n, 0, fmt.Sprintf("cannot check what range ranges over: %s is an interface, whose value has no type maago can know", v.label),
			"give the page a field of a concrete type")
		key.bad, elem.bad = true, true
	} else if v.t != nil {
		k, e, more := rangeTypes(v.t, len(decl))
		if e == nil {
			fix := "range over a slice, an array, a map, a channel, an int or an iterator"
			if more != "" {
				fix = "range over it with one variable"
			}
			w.errAt(n, 0, fmt.Sprintf("cannot range over %s of type %s%s", v.label, types.TypeString(v.t, types.RelativeTo(w.pkg)), more), fix)
			key.bad, elem.bad = true, true
		}
		key.t, elem.t = k, e
		switch deref(v.t).Underlying().(type) {
		case *types.Slice:
			elem.addr = true
		case *types.Array:
			_, isPtr := v.t.Underlying().(*types.Pointer)
			elem.addr = v.addr || isPtr
		}
	} else if !v.bad && v.konst == "" {
		w.errAt(n, 0, "cannot check what range ranges over: its type is not known here", "compute it into a field of the page struct")
		key.bad, elem.bad = true, true
	}
	switch len(decl) {
	case 1:
		inner.vars[decl[0].Ident[0]] = elem
	case 2:
		inner.vars[decl[0].Ident[0]], inner.vars[decl[1].Ident[0]] = key, elem
	}
	w.list(n.List, elem, inner.push())
	w.list(n.ElseList, dot, inner.push())
}

// rangeTypes are the key and element of ranging over t with vars
// variables, as text/template ranges: through pointers; an int or an
// iter.Seq takes one variable; an iter.Seq2 with fewer than two gives its
// key as the element. No element: text/template would fail, why in
// more.
func rangeTypes(t types.Type, vars int) (key, elem types.Type, more string) {
	t = deref(t)
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return types.Typ[types.Int], u.Elem(), ""
	case *types.Array:
		return types.Typ[types.Int], u.Elem(), ""
	case *types.Map:
		return u.Key(), u.Elem(), ""
	case *types.Chan:
		return types.Typ[types.Int], u.Elem(), ""
	case *types.Basic:
		if u.Info()&types.IsInteger != 0 {
			return oneVar(nil, t, vars)
		}
	case *types.Signature: // iter.Seq[V], iter.Seq2[K, V]
		if u.Params().Len() == 1 && u.Results().Len() == 0 {
			if y, isSig := u.Params().At(0).Type().Underlying().(*types.Signature); isSig && y.Results().Len() == 1 {
				switch y.Params().Len() {
				case 1:
					return oneVar(nil, y.Params().At(0).Type(), vars)
				case 2:
					if vars < 2 {
						return nil, y.Params().At(0).Type(), ""
					}
					return y.Params().At(0).Type(), y.Params().At(1).Type(), ""
				}
			}
		}
	}
	return nil, nil, ""
}

func oneVar(key, elem types.Type, vars int) (types.Type, types.Type, string) {
	if vars > 1 {
		return nil, nil, ", with two variables"
	}
	return key, elem, ""
}

// deref follows pointers, as text/template does to reach fields.
func deref(t types.Type) types.Type {
	for {
		p, ok := t.Underlying().(*types.Pointer)
		if !ok {
			return t
		}
		t = p.Elem()
	}
}

// pipe walks a pipeline: each command's value goes into the next as its
// last argument. Its variables are declared or assigned in sc.
func (w *walker) pipe(p *parse.PipeNode, dot val, sc *scope) val {
	var v val
	for i, c := range p.Cmds {
		var piped *val
		if i > 0 {
			piped = &v
		}
		v = w.cmd(c, dot, sc, piped)
	}
	for _, d := range p.Decl {
		name := d.Ident[0]
		if _, s := sc.lookup(name); p.IsAssign && s != nil {
			s.vars[name] = v
		} else {
			sc.vars[name] = v
		}
	}
	return v
}

// cmd walks one command.
func (w *walker) cmd(c *parse.CommandNode, dot val, sc *scope, piped *val) val {
	args := c.Args
	if id, ok := args[0].(*parse.IdentifierNode); ok {
		return w.call(id, args[1:], dot, sc, piped)
	}
	if len(args) > 1 || piped != nil {
		w.errAt(c, 0, "method called with arguments, which maago cannot check",
			"compute the value into a field of the page struct, or into a method without arguments")
		return val{bad: true}
	}
	return w.term(args[0], dot, sc)
}

// term walks one operand.
func (w *walker) term(n parse.Node, dot val, sc *scope) val {
	switch n := n.(type) {
	case *parse.DotNode:
		return dot
	case *parse.FieldNode:
		first := 0 // the parser places a node of two or more fields at its second
		if len(n.Ident) > 1 {
			first = -len(n.Ident[0]) - 1
		}
		return w.chain(dot, n.Ident, n, first)
	case *parse.VariableNode:
		base, _ := sc.lookup(n.Ident[0])
		return w.chain(base, n.Ident[1:], n, 0)
	case *parse.ChainNode:
		base := w.term(n.Node, dot, sc)
		return w.chain(base, n.Field, n, 0)
	case *parse.PipeNode:
		return w.pipe(n, dot, sc.push())
	case *parse.IdentifierNode:
		return w.call(n, nil, dot, sc, nil)
	case *parse.StringNode:
		return val{t: types.Typ[types.String], konst: "string", label: n.Quoted}
	case *parse.NumberNode:
		if n.IsInt {
			return val{t: types.Typ[types.Int], konst: "int", label: n.Text}
		}
		return val{t: types.Typ[types.Float64], konst: "float", label: n.Text}
	case *parse.BoolNode:
		return val{t: types.Typ[types.Bool], konst: "bool"}
	case *parse.NilNode:
		return val{konst: "nil"}
	}
	return val{}
}

// chain walks .A.B from base. The first ident's dot is at offset first
// from the node's position.
func (w *walker) chain(base val, idents []string, n parse.Node, first int) val {
	off := first
	for _, name := range idents {
		at := off + 1 // past the dot
		off += len(name) + 1
		switch {
		case base.bad:
			return base
		case base.nilDot:
			w.errAt(n, at, fmt.Sprintf("cannot read .%s: {{template}} called this template without a value, so its dot is nil", name),
				"pass the value: {{template \"name\" .}}")
			return val{bad: true}
		case base.t == nil:
			w.errAt(n, at, fmt.Sprintf("cannot check .%s: %s has no type maago can know", name, orLabel(base.label)),
				"compute the value into a field of the page struct")
			return val{bad: true}
		}
		next, msg, fix := w.field(base, name)
		if msg != "" {
			w.errAt(n, at, msg, fix)
			return val{bad: true}
		}
		base = next
	}
	return base
}

func orLabel(l string) string {
	if l == "" {
		return "the value before it"
	}
	return l
}

// field is base.name: a field, a method without arguments, or the element
// of a map with string keys.
func (w *walker) field(base val, name string) (val, string, string) {
	t := base.t
	label := base.label + "." + name
	u := deref(t).Underlying()
	if m, ok := u.(*types.Map); ok {
		if !types.Identical(m.Key(), types.Typ[types.String]) { // text/template looks up a string, which a named key type does not take
			return val{}, fmt.Sprintf("cannot read .%s of %s: its keys are %s, not string", name, base.label, types.TypeString(m.Key(), types.RelativeTo(w.pkg))), "give the map string keys"
		}
		return val{t: m.Elem(), label: label}, "", ""
	}
	if _, ok := u.(*types.Interface); ok {
		return val{}, fmt.Sprintf("cannot check .%s: %s is an interface, whose value has no type maago can know", name, base.label),
			"give the page a field of a concrete type"
	}
	obj, _, _ := types.LookupFieldOrMethod(t, base.addr, w.pkg, name) // the adapter passes the page by value: not addressable
	if obj == nil {                                                   // past pointers to pointers, text/template finds fields only
		if f, _, _ := types.LookupFieldOrMethod(deref(t), false, w.pkg, name); f != nil {
			if _, isVar := f.(*types.Var); isVar {
				obj = f
			}
		}
	}
	if obj == nil {
		fix := "check the name"
		if m := diag.Suggest(name, members(t)); m != "" {
			fix = fmt.Sprintf("did you mean %q?", m)
		}
		if _, isPtr := t.Underlying().(*types.Pointer); !isPtr {
			ptr := types.NewPointer(t)
			if o, _, _ := types.LookupFieldOrMethod(ptr, false, w.pkg, name); o != nil {
				return val{}, fmt.Sprintf("method %s of %s has a pointer receiver, which templates cannot call here: the value is the page, a map element or a result, not addressable", name, base.label),
					"give " + name + " a value receiver"
			}
		}
		return val{}, fmt.Sprintf("%s has no field or method %q", base.label, name), fix
	}
	if !obj.Exported() {
		return val{}, fmt.Sprintf("%s is not exported, so templates cannot read it", label), exportFix(name)
	}
	switch o := obj.(type) {
	case *types.Var:
		_, isPtr := t.Underlying().(*types.Pointer)
		return val{t: o.Type(), label: label, addr: base.addr || isPtr}, "", ""
	case *types.Func:
		sig := o.Type().(*types.Signature)
		rs := sig.Results()
		switch {
		case sig.Params().Len() > 0:
			return val{}, fmt.Sprintf("method %s takes arguments, which maago cannot check", label), "compute the value into a field of the page struct, or a method without arguments"
		case rs.Len() == 1, rs.Len() == 2 && types.Identical(rs.At(1).Type(), types.Universe.Lookup("error").Type()):
			return val{t: rs.At(0).Type(), label: label}, "", ""
		}
		return val{}, fmt.Sprintf("method %s returns what templates cannot use", label), "return a value, or a value and an error"
	}
	return val{}, label + " is not a field or a method", "check the name"
}

func exportFix(name string) string {
	r, n := utf8.DecodeRuneInString(name)
	if u := unicode.ToUpper(r); u != r {
		return "export it: " + string(u) + name[n:]
	}
	return "rename it to start with an upper-case letter"
}

// members are the exported fields and methods of t, for did you mean.
func members(t types.Type) []string {
	var names []string
	u := t.Underlying()
	if p, ok := u.(*types.Pointer); ok {
		u = p.Elem().Underlying()
	}
	if s, ok := u.(*types.Struct); ok {
		for i := range s.NumFields() {
			if f := s.Field(i); f.Exported() {
				names = append(names, f.Name())
			}
		}
	}
	ms := types.NewMethodSet(t)
	for i := range ms.Len() {
		if m := ms.At(i).Obj(); m.Exported() {
			names = append(names, m.Name())
		}
	}
	return names
}

// call walks a function call: its arguments always, and url's against
// the routes.
func (w *walker) call(id *parse.IdentifierNode, args []parse.Node, dot val, sc *scope, piped *val) val {
	var vs []val
	for _, a := range args {
		vs = append(vs, w.term(a, dot, sc))
	}
	if piped != nil {
		vs = append(vs, *piped)
	}
	switch id.Ident {
	case "eq", "ne", "lt", "le", "gt", "ge", "not":
		return val{t: types.Typ[types.Bool]}
	case "len":
		return val{t: types.Typ[types.Int]}
	case "print", "printf", "println", "html", "js", "urlquery":
		return val{t: types.Typ[types.String]}
	case "url":
		w.url(id, args, vs)
		return val{t: types.Typ[types.String]}
	}
	return val{label: "the result of " + id.Ident} // index, slice, call, and, or: no type known
}

// url checks {{url "name" args…}}: a route of the project, given a value
// of its parameter's type for each wildcard.
func (w *walker) url(id *parse.IdentifierNode, args []parse.Node, vs []val) {
	if len(args) == 0 {
		w.errAt(id, 0, "url takes the name of a route", `write {{url "post_detail" .ID}}`)
		return
	}
	lit, ok := args[0].(*parse.StringNode)
	if !ok {
		w.errAt(args[0], 0, "cannot check url: the route name is not a string literal", `write the name in quotes: {{url "post_detail" .ID}}`)
		return
	}
	params, ok := w.routes[lit.Text]
	if !ok {
		names := make([]string, 0, len(w.routes))
		for n := range w.routes {
			names = append(names, n)
		}
		slices.Sort(names)
		fix := "name a route of the project's route tables"
		if m := diag.Suggest(lit.Text, names); m != "" {
			fix = fmt.Sprintf("did you mean %q? routes are declared in the apps' var Routes", m)
		}
		w.errAt(lit, 0, fmt.Sprintf("unknown route name %q", lit.Text), fix)
		return
	}
	given := vs[1:]
	if len(given) != len(params) {
		w.errAt(id, 0, fmt.Sprintf("route %s has %d wildcards, but url got %d values", lit.Text, len(params), len(given)),
			"give one value for each {wildcard} of its path, in order")
		return
	}
	for i, p := range params {
		v := given[i]
		var node parse.Node = id
		if i+1 < len(args) {
			node = args[i+1]
		}
		switch {
		case v.bad:
		case v.t == nil && v.konst == "":
			w.errAt(node, 0, fmt.Sprintf("cannot check the value for {%s} of route %s: its type is not known here", p.name, lit.Text),
				"pass a field of the page struct")
		case !fits(v, p.typ):
			w.errAt(node, 0, fmt.Sprintf("{%s} of route %s is %s, not %s", p.name, lit.Text, p.typ, kindOf(v)),
				"pass a value of type "+p.typ)
		}
	}
}

// fits reports whether v can fill a wildcard of type typ: URLFunc takes a
// string, or an int, int32 or int64.
func fits(v val, typ string) bool {
	if v.konst != "" {
		return v.konst == "string" && typ == "string" || v.konst == "int" && typ != "string"
	}
	for _, k := range map[string][]types.BasicKind{"string": {types.String}, "int64": {types.Int, types.Int64, types.Int32}, "int32": {types.Int, types.Int32, types.Int64}}[typ] {
		if types.Identical(v.t, types.Typ[k]) {
			return true
		}
	}
	return false
}

func kindOf(v val) string {
	if v.konst != "" {
		return "a " + v.konst + " constant"
	}
	return types.TypeString(v.t, nil)
}
