package blog

import (
	m "github.com/maago-web/maago"
	t "time"
)

type Status string

type Post struct {
	m.Model `maago:"ordering=(-PublishedAt,Title),unique=(AuthorID,Slug),index=(Status),admin_list=(Title,Status,PublishedAt),admin_search=(Title,Slug),admin_filter=(Status,Public,AuthorID)"`

	ID           int64    `maago:"label='번호'"`
	AuthorID     int64    `maago:"fk=Author"`
	Title        string   `maago:"max=200,label='제목',help='Shown in lists'"`
	Slug         string   `maago:"max=100,unique,admin=readonly"`
	Status       string   `maago:"choices=draft/published,default='draft'"`
	Views, Likes int32    `maago:"default=0"`
	Score        *float64 `maago:"default=1.50"`
	Public       bool     `maago:"default=true"`
	Body         []byte
	PublishedAt  *t.Time
	Kind         Status
	Count        int
	cache        string
	Digest       [4]byte
	Audit
}

type Audit struct {
	CreatedBy string
}

type Author struct {
	m.Model `maago:"table=people,admin_filter=(ParentID)"`

	ID       int64
	Name     string
	ParentID *int64 `maago:"fk=Author"`
	// An FK with its own unique or index gets no second, automatic index.
	EditorID   int64  `maago:"fk=Author,unique"`
	ReviewerID *int64 `maago:"fk=Author,index"`
	// The same columns may be both unique and indexed.
	Code string `maago:"unique,index,admin=hidden"`
	// A reserved word as a column, and a default written unusually.
	Order int32 `maago:"default=007"`
	Admin bool  `maago:"default=false"`
	// Filled by the generated CRUD (D-102).
	CreatedAt t.Time `maago:"auto=create"`
	UpdatedAt t.Time `maago:"auto=update"`
}

type NotAModel struct {
	Name string
}
