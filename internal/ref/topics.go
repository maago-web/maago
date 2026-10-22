package ref

// Topics are the topics of maago doc, in the order a project meets them.
var Topics = []*Topic{
	{
		Name:  Main,
		Title: "main.go",
		Summary: `main.go puts the project together: it migrates the database, opens it, gives
each app's App to maago.Handler with the project's settings, and serves the
handler with maago.Serve.`,
		Text: `A main:

	//go:embed migrations
	var migrations embed.FS // the migrations, in the binary

	func main() {
		ctx := context.Background()
		if _, err := maago.Migrate(ctx, maago.DBConfig{}, migrations); err != nil {
			log.Fatal(err)
		}
		db, err := maago.OpenDB(ctx, maago.DBConfig{})
		if err != nil {
			log.Fatal(err)
		}
		h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{blog.App}})
		if err != nil {
			log.Fatal(err)
		}
		if err := maago.Serve(ctx, ":8000", h); err != nil {
			log.Fatal(err)
		}
	}

main imports the database driver, _ "modernc.org/sqlite" or
_ "github.com/jackc/pgx/v5/stdlib": maago does not. maago.DBConfig{} is
MAAGO_DATABASE_URL, from the environment or .env in the working directory:
sqlite:app.db or postgres://….

  - Migrate applies the migrations the database has not, as maago migrate
    does, and refuses a changed or missing one. It comes before OpenDB, which
    needs the SQLite file to exist. Two copies starting at once apply each
    file once.
  - Handler refuses a Config it cannot serve, such as: no DB, no Apps, an app twice,
    routes that take the user without Config.Auth, a form with a file
    without Config.MediaDir.
  - Config: Apps (each app's generated App, and admin.App), Auth and
    LoginURL (maago doc auth), Origins (other origins to trust, for
    cross-origin POSTs and CORS), OnError (hears every 5xx), Messages (the
    forms' errors; maago.DefaultMessages has the English ones), TimeZone
    (where people's times are; nil is UTC), Prefix (the path the site is
    under, such as /app), MaxBody (0 is 10MB), MediaDir.
  - Serve stops on SIGINT or SIGTERM: no new connections, the running
    requests finish within 30 seconds. TLS is a proxy's in front, or an
    http.Server of main's.

Settings of an app, such as a webhook's URL, are the app's own package
variables, which main fills in before Handler. A test of the whole site
calls the function that makes the handler, as main does.`,
		API: []string{"Handler", "Config", "App", "DBConfig", "OpenDB", "Migrate", "Serve", "Messages", "DefaultMessages", "Prefix"},
	},
	{
		Name:  Model,
		Title: "Models",
		Summary: `A model is a struct of an app's package that embeds maago.Model. Its fields are
the table's columns, and every setting is in the maago:"…" tags. maago
makemigrations writes the migrations from the models, and maago gen writes
GetX, InsertX, UpdateX and DeleteX for each.`,
		Text: `A model:

	type Post struct {
		maago.Model ` + "`" + `maago:"table=posts,ordering=-PublishedAt,index=(Status,PublishedAt)"` + "`" + `

		ID          int64      // an int64 field named ID is the primary key
		AuthorID    int64      ` + "`" + `maago:"fk=auth.User,label='Author'"` + "`" + `
		Title       string     ` + "`" + `maago:"max=200,label='Title'"` + "`" + `
		Status      string     ` + "`" + `maago:"choices=draft/published,default='draft'"` + "`" + `
		PublishedAt *time.Time // a pointer allows NULL
		UpdatedAt   time.Time  ` + "`" + `maago:"auto=update"` + "`" + `
	}

The tag is a list split by commas. A key stands alone (unique), takes a
bare value (max=200), a list split by / (choices=draft/published), text in
single quotes (label='Title'; '' for a quote), or fields in parentheses
(unique=(AuthorID,Slug)). Settings of the whole model go on the embedded
maago.Model. A field of another type, unexported or embedded, is not a
column and may have no tag.

A relation is an ID field with fk=Model, not an object: JOIN in SQL when
you need the other row. choices are values the program writes, not words
for people: lowercase, split by /, none twice; the form shows them as they
are.

maago gen writes models_gen.go with, for a model Post:

	func GetPost(ctx context.Context, db maago.DB, id int64) (Post, error)
	func InsertPost(ctx context.Context, db maago.DB, in NewPostInput) (Post, error)
	func UpdatePost(ctx context.Context, db maago.DB, p *Post) error
	func DeletePost(ctx context.Context, db maago.DB, id int64) error

A missing row is sql.ErrNoRows (a view's 404). NewPostInput has the fields
without a default as they are and those with one as maago.Opt (maago.Set(v)
gives one). UpdatePost writes every column but auto=create ones; with an
auto=update field it writes only the row it read: a row changed since is
maago.ErrConflict, a 409. Do not name your own functions or types after
these. maago doc query has the queries beyond them, and the read policies:
the GetX of a model with them takes the user and the policy's params,
and GetXIgnoringPolicies reads any row.`,
		Lists: []*List{FieldTypes, FieldTags, ModelTags},
		API:   []string{"Model", "Opt", "Set", "Ptr", "ErrConflict"},
	},
	{
		Name:  Query,
		Title: "Queries",
		Summary: `A query is a file of SQL, queries/name.sql in an app, with a header that names
the Go function, its result and its parameters. maago gen prepares every
form of it on a practice database built from the migrations, and writes
queries_gen.go.`,
		Text: `A query file:

	-- name: SearchPosts :many PostSummary
	-- param: q?      string
	-- param: status any/draft/published
	-- param: ids     []int64
	SELECT id, title, published_at
	FROM posts
	WHERE {
	  {q}       AND title LIKE '%' || :q || '%'
	  {status=draft|status=published} AND status = :status
	  {ids}     AND id IN (:ids)
	}
	ORDER BY published_at DESC

It becomes

	type SearchPostsParams struct {
		Q      *string
		Status string // any, draft or published
		IDs    []int64
	}
	func SearchPosts(ctx context.Context, db maago.DB, p SearchPostsParams) ([]PostSummary, error)

With no params there is no p. db is a maago.DB or a maago.Tx, as the view
has it.

The six rules (a diagnostic's "rule N" is one of these):

 1. One query to a file. The header names it, its result and its params;
    ? marks a value that may be missing.
 2. Values are :name. A slice spreads into a placeholder for each element.
    Table and column names are never bound. $1, ? and @x are errors.
 3. A {condition} at the start of a line keeps the line only when it holds:
    name (given: a bool true, a slice not empty), !name, name=value (choices
    only),
    joined by only & or only |. No parentheses: compute anything more in Go
    and pass a bool.
 4. Inside WHERE { } and SET { }, the leading AND or OR and the trailing
    comma are tidied, and an empty WHERE drops; an empty SET is an error.
    They belong to the statement, not to a subquery or EXISTS ( … ).
 5. maago gen prepares every combination. Errors: an empty WHERE in a
    statement other than SELECT, a missing param used where no condition
    says it is given, result columns other than the declared ones.
 6. A query that reads the table of a model with read policies
    (policies.sql of its app) says so where it reads it: -- policy: name
    alias in the header and name(alias) as an AND term of that SELECT's
    WHERE, or -- policy: none alias why. The tables a table function reads
    are not seen.

Read policies are written once, in policies.sql beside the app's queries/
(its header lines are in the list below). Each is one condition, without
{conditions}, on the rows of a model:

	-- name: visible Post p
	-- get: GetPost
	-- The posts the user may read: those published by now, and their own.
	-- param: now time.Time
	p.status = 'published' AND (p.published_at IS NULL OR p.published_at <= :now)
	  OR p.author_id = :viewer_id

:viewer_<column> is a column of the user the views take (u *auth.User);
for nobody, a nil user, each is NULL, which WHERE drops. A query reads
the table under it where it reads it: -- policy: visible p in the header,
visible(p) an AND term of the WHERE of the SELECT that reads p (or of
the ON of the JOIN that brings p in, but not a RIGHT or FULL one), never
on a {condition} line. A table without an alias is its own alias. The
policy's params join the query's Params; the query may declare one too,
of the same type. A query of another app may not read the table. With a
policy in the project, a FROM maago gen cannot read, such as a join in
parentheses, is an error.

A query that uses a policy takes the user, as the views do:

	func SearchPosts(ctx context.Context, u *auth.User, db maago.DB, p SearchPostsParams) ([]PostSummary, error)

The GetX that -- get: names applies its policy, its params after the id
in the order declared, and GetXIgnoringPolicies reads any row, for the
admin and the tests that read back what they wrote:

	func GetPost(ctx context.Context, u *auth.User, db maago.DB, id int64, now time.Time) (Post, error)
	func GetPostIgnoringPolicies(ctx context.Context, db maago.DB, id int64) (Post, error)

A row the policy hides is sql.ErrNoRows to that user: a 404, not a 403.
Writes are no policy's: UpdatePost and DeletePost write any row, so the
view checks who may write. maago gen refuses a policy that neither a
query nor -- get: uses, and policies without a view that takes the user.

The result after the kind is one of three:

  - A model's name (:many Post): the columns must be exactly the model's,
    so SELECT * or all of them. A half-filled Post saved back would erase
    data.
  - A new name (:many PostSummary): maago gen writes a struct of the
    columns' shape. A column whose type or NULL the database cannot tell, an
    expression such as count(*) or the outer side of a LEFT JOIN, is
    declared with -- column: name[?] type.
  - A type, for one column: :one bool, :many int64, string, int32, float64,
    time.Time. It may not be NULL: wrap an aggregate in COALESCE, as in
    SELECT COALESCE(max(views), 0).

A model with an auto=update field is compared by UpdateX (maago doc model),
so a query that UPDATEs its table sets that field too, or an UpdateX that
read the row before would undo the change:

	-- param: now time.Time
	UPDATE posts SET views = views + 1, updated_at = :now WHERE id = :id

A param's type is a field type, a slice of one ([]int64), or choices
(latest/amount: lowercase snake_case, split by /). ? goes on no bool, slice
or choices: a nil or empty slice is none, and choices name their own none
(any/draft/published). Names: the query and a new result
start with an uppercase letter; params and columns are lowercase
snake_case.`,
		Lists: []*List{QueryKeys, QueryKinds, PolicyKeys},
		API:   []string{"DB", "ErrManyRows", "ErrEmptySlice"},
	},
	{
		Name:  View,
		Title: "Views and routes",
		Summary: `A view is a func of an app that takes what it uses and returns a page. The
app's var Routes lists each route's method, path, name and view, and main
puts each app's generated App in maago.Config.Apps.`,
		Text: `The routes, in urls.go of the app:

	var Routes = maago.Routes{
		maago.GET("/{$}", "post_list", PostList), // {$}: the path alone
		maago.GET("/post/{id}", "post_detail", PostDetail),
		maago.GET("/post/new", "post_new", NewPost),
		maago.POST("/post/new", "post_create", CreatePost),
		maago.GET("/api/posts", "api_posts", APIPosts, maago.API),
	}

Each route has its own name, so a GET and a POST of one path are two
names; templates and the generated URL functions (PostDetailURL(id)) use
them. A path that ends in / needs {$}. maago gen tells main what to add:
each app with routes goes in maago.Config.Apps as app.App.

A view's signature says what it uses, in this order:

	func V(ctx context.Context, [u U | u *U], [db maago.DB | tx maago.Tx], in In) (Out, error)

  - u U: the route requires a signed-in user (U is what Config.Auth
    returns, such as auth.User): a browser's GET goes to Config.LoginURL
    with next, any other request is a 401. u *U lets anybody in, nil when
    nobody is signed in. Without u the view does not know the user.
  - db maago.DB reads. tx maago.Tx writes: the view runs in the request's
    transaction, committed when it returns no error, rolled back on an error
    or a panic. A GET's view cannot take maago.Tx; a writing route's view
    takes maago.Tx, or maago.DB with maago.NoTx on its writing routes.
  - In is struct{} or a struct of the package whose fields say where each
    value comes from. It is always last.
  - Out is a page type (rendered by its template), maago.Result[Page] for a
    view that may redirect, or on an API route any type, sent as JSON.

	func PostDetail(ctx context.Context, db maago.DB, in PostDetailIn) (PostDetailPage, error)
	func CreatePost(ctx context.Context, u auth.User, tx maago.Tx, in NewPostIn) (maago.Result[PostFormPage], error)

maago.Show(page) renders the page (and commits); maago.SeeOther[Page](url)
commits and sends a 303. An error rolls back: sql.ErrNoRows is a 404,
maago.Status(code, err) is that code, maago.ErrConflict a 409, a form's
error on an API route a 422, any other a 500 that Config.OnError hears.
maago.AfterCommit(ctx, f) runs f after the commit, before the response,
and never after a rollback; a write that must not be lost is no job for it.

A view that answers with something other than a page, such as a file, is a
func(w http.ResponseWriter, r *http.Request). It reads the database and
the user itself, maago.DBFrom(r.Context()) and maago.UserFrom[U](r.Context()),
and answers a missing sign-in itself. A typed view that calls either is an
error; maago gen sees the calls in the view's own body, not in the
functions it calls.

An app whose views take the user has a permission table in its tests,
or maago gen refuses it: maagotest.Permissions(t, apps, roles, rows...),
a row for each route and a status for each role in each row
(maagotest.Skip leaves a role out of a row). A role signed in is
maagotest.As(h, session), the session the copied auth's
auth.SignIn(ctx, db, u) starts. The admin's routes are in the table
only if its App is in apps; a row's URL is its route's own, from the
URL function. go doc github.com/maago-web/maago/maagotest has Row's Body
and ContentType.

maago.NoTx is not in the fix of the error about a writing view that takes
maago.DB: its only use is a write that must stay when the view then fails,
and that is a decision, not a repair.`,
		Lists: []*List{Methods, RouteOptions, InTags},
		API:   []string{"Routes", "Route", "RouteOption", "GET", "POST", "PUT", "PATCH", "DELETE", "API", "NoTx", "Block", "Result", "Show", "SeeOther", "Status", "Tx", "WithTx", "AfterCommit", "DBFrom", "WithDB"},
	},
	{
		Name:  Form,
		Title: "Forms",
		Summary: `A form is a struct of an app that embeds maago.Form and has maago.Field[T]
fields. A view takes it as a field of its In tagged maago:"form"; maago gen
writes the code that reads and checks each field.`,
		Text: `A form:

	type PostForm struct {
		maago.Form ` + "`" + `maago:"model=Post"` + "`" + `
		Title      maago.Field[string]
		Status     maago.Field[string]
		Score      maago.Field[int64] ` + "`" + `maago:"min_value=1,max_value=5,label='Score'"` + "`" + `
	}

	type NewPostIn struct {
		Form PostForm ` + "`" + `maago:"form"` + "`" + `
	}

model=Post takes each field named after a field of the model with its
rules: required unless the model field is a pointer, max, choices, label,
help. Such a field may add optional and nothing else. The form key is the field's name in
snake_case.

A GET view and a POST view take the same In: the GET draws the empty form,
the POST reads it. In the POST view, in.Form.Valid() tells whether every
field read and passed its rules and Validate added no error:

	func CreatePost(ctx context.Context, tx maago.Tx, in NewPostIn) (maago.Result[PostFormPage], error) {
		f := in.Form
		if !f.Valid() {
			return maago.Show(PostFormPage{Form: f}), nil // 200, with the errors
		}
		…
		return maago.SeeOther[PostFormPage](PostListURL()), nil
	}

A maago.File field is read from a multipart form (enctype="multipart/form-data"),
and needs maago.Config.MediaDir; maago.SaveMedia keeps the file. A field
takes one file, from an HTML form: an API route takes none. Its kind is
what the content's first bytes say, not its name; zip includes .docx and
.xlsx. The File lives as long as the request.

Media stays in Config.MediaDir, on this machine. A file no row points at
stays: delete it with maago.DeleteMedia when you delete the row (the admin
does not); one saved by a request that failed before writing its row stays
too. No route serves the directory: a view checks who asks, then
calls maago.ServeMedia, which sends a download.

A field has Value, Raw (what was sent, to draw back), Error, and Name,
Label, Help and Choices for the template. An empty value, one that does
not parse, and one out of range each have their own message
(Config.Messages).

A check across fields is a method:

	func (f *PostForm) Validate() {
		if … {
			f.Title.Error = "…" // or f.AddError("…") for the form as a whole
		}
	}

It has a pointer receiver, takes and returns nothing, and runs after every
field was read. A check that needs the database goes in the view: set the
field's Error and maago.Show the page again; an API view returns
maago.Invalid(&f.Title, "…") or maago.InvalidForm("…"), a 422.

A value taken already needs no read before the write: a unique
constraint refuses it, and the form's Fail, which maago gen writes, shows
the error:

	_, err := InsertPost(ctx, tx, NewPostInput{Title: f.Title.Value})
	if f.Fail(err) {
		return maago.Show(PostFormPage{f}), nil
	}

Fail takes the *maago.UniqueError of a generated Insert or Update and
puts Config.Messages.Taken on the one field of the form the constraint is
over (the form's field of the model field's name), else on the form; the
transaction goes on. Any other error, nil too, it leaves: false. A model
with one unique constraint has every refusal on it, even a unique index
a migration made by hand. An API view has no page to draw: it takes the
error with errors.As and returns maago.Invalid or maago.InvalidForm.`,
		Lists: []*List{FormTypes, FormTags},
		API:   []string{"Form", "Field", "Invalid", "InvalidForm", "UniqueError", "File", "Media", "SaveMedia", "OpenMedia", "DeleteMedia", "ServeMedia", "WithMediaDir"},
	},
	{
		Name:  Template,
		Title: "Templates and static files",
		Summary: `A page type's template is templates/<its name in snake_case>.html in its app,
html/template filling the app's templates/base.html. maago gen checks every
field, route name and static file a template names.`,
		Text: `The page type a view returns names its template: PostListPage is
templates/post_list_page.html. The generated code embeds the app's
templates/*.html. The frame is templates/base.html, which has a block the
pages fill:

	<!-- templates/base.html -->
	<!doctype html>
	<title>{{block "title" .}}Blog{{end}}</title>
	<main>{{block "content" .}}{{end}}</main>

	<!-- templates/post_list_page.html -->
	{{define "content"}}
	  {{range .Posts}}
	    <a href="{{url "post_detail" .ID}}">{{.Title}}</a> {{date "2006-01-02" .PublishedAt}}
	  {{end}}
	{{end}}

maago gen checks each {{.Field}} against the page type, each route name and
its arguments, and each static file. What it cannot know, such as what a
function returns, it reports: compute it into a field of the page.

Static files are an app's static/ directory: {{static "blog/site.css"}}
gives its URL under /static/blog/, with the content's hash in the name so
browsers may keep it, and they are embedded: a change shows after go
build. They are not compressed: a proxy in front does that. No route may
be under /static/. maago tailwind writes static/tailwind.css from the
app's tailwind.css. It finds the classes written as they are, not one a
template builds such as "bg-" + .Color. Run it again after changing
classes: maago gen and check do not see a static/tailwind.css older than
the templates.`,
		Lists: []*List{TemplateFuncs},
	},
	{
		Name:  Auth,
		Title: "Signing in",
		Summary: `maago add auth copies the accounts app into the project, yours from then on:
the User model, the pages to sign up, sign in and out, verify, reset and
change a password, API tokens, and Current, which main gives
maago.Config.Auth.`,
		Text: `After maago add auth, main has

	auth.SiteURL = "https://example.com" // scheme and host: the mails' links, and the cookie's Secure
	h, err := maago.Handler(maago.Config{
		DB:       db,
		Apps:     []maago.App{auth.App, blog.App},
		Auth:     auth.Current, // the user of the request's session or API token
		LoginURL: "/login",
	})

and a view takes the user as u auth.User (sign-in required) or
u *auth.User (nil for nobody); maago doc view has the rest. The pages are
/login, /logout (a POST), /signup, /verify, /password, /password/reset,
/tokens and /account/close, with their confirm and resend pages. A GET of a page that requires a sign-in goes to
/login?next=<path>, and the sign-in goes back to next, on this site only.
The form fields are email and password.

  - The session cookie is Secure unless SiteURL is http://: http://localhost
    in development, https in production.
  - There is no CSRF token: a POST from another origin is a 403 (the
    browser's Origin or Sec-Fetch-Site header), unless Config.Origins
    trusts it.
  - An API client sends Authorization: Bearer <token>, a token made at
    /tokens; a route whose view takes u answers it without a user with
    401 and WWW-Authenticate: Bearer.
  - Sign-in and mail attempts are limited by email and by IP
    (AttemptLimit, AttemptLimitIP); behind a proxy the IP is the proxy's
    unless main's middleware sets it.
  - authlib.Mail sends the mails; authlib.ConsoleMail logs them.
  - auth.CreateSuperuser(ctx, db, email, password) makes a staff account
    that may sign in at once, for main's -createsuperuser.
  - auth.RequireStaff(u) is a 403 unless u is staff.

The security parts, such as password hashing, are in the library
(github.com/maago-web/maago/contrib/auth): go get -u brings their fixes.`,
		API: []string{"UserFrom", "WithUser", "SetCookie", "RemoteAddrFrom"},
	},
	{
		Name:  Admin,
		Title: "The admin",
		Summary: `The admin lists, adds, changes and deletes the rows of each model, through
code maago gen writes from the models (admin_gen.go). main gives
admin.App(admin.Config{…}) to maago.Config.Apps, and the copied auth's Can
decides who may do what.`,
		Text: `In main, after maago add auth:

	adminApp := admin.App(admin.Config{
		Models: [][]admin.Model{blog.Admin, auth.Admin}, // each app with models
		Can:    auth.Can,                                // who may do what to which model
	})
	h, err := maago.Handler(maago.Config{Apps: []maago.App{auth.App, blog.App, adminApp}, …})

admin is github.com/maago-web/maago/contrib/admin; maago gen writes each
app's Admin once a package imports it. Only a staff user gets in. The
model's tags shape it (maago doc model): admin=hidden on the model or a
field, admin=readonly on a field, admin_list, admin_search, admin_filter
and ordering. A model has no add page if a field the form leaves out (an
admin=hidden or admin=readonly one) has no default.

The URLs, under /admin/:

	/admin/                          the models
	/admin/<app>/<model>/            the list: ?q= searches, ?<field>=<value> filters, ?page=
	/admin/<app>/<model>/add/        a new row
	/admin/<app>/<model>/<id>/       a row, to change
	/admin/<app>/<model>/delete/     a GET with ?id=1&id=2 asks; a POST with the ids in its body deletes

<app> is the package's name and <model> the model's name, as written
(/admin/blog/Post/). A form field's and a filter's key is the field's name
in snake_case (author_id); an fk takes the row's number, and a time is
2006-01-02T15:04:05 in Config.TimeZone. A
row saved meanwhile by someone else is a 409 when the model has an
auto=update field. The admin saves through the model form's rules only: the
work of the app's views does not happen. It reads every row, read
policies or not: its form reads a row with GetXIgnoringPolicies.`,
	},
}
