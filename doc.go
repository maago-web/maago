// Package maago is the runtime library that Maago applications and
// generated code import.
//
// # Work after the response
//
// maago has no task queue (D-72, D-118). A view that must do something
// after it answers, such as calling a webhook, starts a goroutine with a
// context that the end of the request does not cancel:
//
//	go notify(context.WithoutCancel(ctx), post)
//
// The goroutine is the app's: the server does not wait for it when it
// stops, and Handler does not catch its panics or hear its errors, so it
// recovers and reports them itself.
//
// # Static files, uploads and media: what v0 does not do
//
//   - Static files are not compressed: a proxy in front does that (D-137).
//     A file of static/ changes its hashed name with its content, after
//     go build: the files are embedded.
//   - maago tailwind finds the classes written as they are in the files it
//     scans; a class a template builds, such as "bg-" + .Color, is not
//     found. maago gen and check do not see a static/tailwind.css older
//     than the templates: run maago tailwind again after changing classes
//     (D-138).
//   - A form field takes one file, from an HTML form: an API route takes
//     none. The kinds accept= names are png, jpeg, gif, webp, pdf and zip
//     (FileKinds), as the content's first bytes tell; zip includes .docx
//     and .xlsx. A File lives as long as the request (D-139).
//   - Media stays in Config.MediaDir, on this machine: no other storage,
//     no image processing (D-72). A file no row points at stays, nothing
//     sweeps it: its row deleted in the admin, a view that did not call
//     DeleteMedia, or a request that stopped between SaveMedia and writing
//     the row. No route serves the directory: the app's view checks who
//     asks, then calls ServeMedia, which always sends a download (D-140).
package maago
