package rtcore

import "net/http"

// App is maago.App.
type App struct {
	name   string
	routes []Route
	static *StaticFiles
}

// NewApp is the App of the app named name, with its static files or nil.
func NewApp(name string, static *StaticFiles, routes ...Route) App {
	return App{name, routes, static}
}

// Parts are a's name, routes and static files, for Handler.
func Parts(a App) (string, []Route, *StaticFiles) { return a.name, a.routes, a.static }

// Route has rt.AppRoute's fields, in order, so each converts to the other.
type Route struct {
	Method, Path, Name              string
	API, LoginRequired, User, Files bool
	Serve                           func(w http.ResponseWriter, r *http.Request) error
}
