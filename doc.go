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
package maago
