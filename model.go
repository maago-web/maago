package maago

// Model marks a struct as a Maago model. Embed it, and put the
// model's options in its tag (D-13):
//
//	type Post struct {
//		maago.Model `maago:"table=posts"`
//
//		ID          int64      // the primary key (D-74)
//		Title       string     `maago:"max=200"`
//		PublishedAt *time.Time // a pointer allows NULL (D-75)
//	}
type Model struct{}
