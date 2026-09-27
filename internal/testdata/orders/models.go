// Package orders is the example app of M3's code generation: the
// SearchOrders example of DECISIONS and a query of each other shape.
package orders

import (
	"time"

	"github.com/maago-web/maago"
)

type Customer struct {
	maago.Model `maago:"table=customers"`

	ID   int64
	Name string `maago:"max=100"`
}

type Order struct {
	maago.Model `maago:"table=orders"`

	ID         int64
	CustomerID *int64 `maago:"fk=Customer"`
	Status     string `maago:"max=20,default='new'"`
	Amount     int64
	Priority   *int32 `maago:"default=010"`
	CreatedAt  time.Time
	UpdatedAt  time.Time `maago:"auto=update"`
}
