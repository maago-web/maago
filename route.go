package maago

// Routes is an app's route table: var Routes = maago.Routes{…} (D-115).
// maago gen reads the literal and checks it; the value does nothing at
// run time. The code maago generates from it serves the requests.
type Routes []Route

// Route is one line of Routes. Made outside the Routes literal, it does
// nothing.
type Route struct{}

// RouteOption is what a route may add after its view: LoginRequired, API.
type RouteOption int

const (
	// LoginRequired lets only a signed-in user reach the route.
	LoginRequired RouteOption = iota + 1
	// API makes the route answer JSON, and its errors RFC 9457 problems.
	API
)

// GET is a route for GET (and HEAD) requests to path, named name, served
// by view: a function of the app's package whose signature maago gen
// checks (D-116), since views have more than one shape.
func GET(path, name string, view any, opts ...RouteOption) Route { return Route{} }

// POST is GET for POST requests.
func POST(path, name string, view any, opts ...RouteOption) Route { return Route{} }

// PUT is GET for PUT requests.
func PUT(path, name string, view any, opts ...RouteOption) Route { return Route{} }

// PATCH is GET for PATCH requests.
func PATCH(path, name string, view any, opts ...RouteOption) Route { return Route{} }

// DELETE is GET for DELETE requests.
func DELETE(path, name string, view any, opts ...RouteOption) Route { return Route{} }
