package maago

// Model marks a struct as a Maago model. Embed it, and put the
// model's options in its tag:
//
//	type Post struct {
//		maago.Model `maago:"table=posts"`
//
//		ID          int64      // the primary key
//		Title       string     `maago:"max=200"`
//		PublishedAt *time.Time // a pointer allows NULL
//	}
type Model struct{}
