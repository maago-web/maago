package maago

import "github.com/maago-web/maago/internal/rtcore"

// What generated code makes an App with: rt's, which this package's tests
// cannot import.
type AppRoute = rtcore.Route

func NewApp(name string, routes ...AppRoute) App { return rtcore.NewApp(name, nil, routes...) }
