// Package orders is the example app of M3's code generation: the
// SearchOrders example of DECISIONS and a query of each other shape. Its
// Amount is money (decimal=2), and a Parcel has a UUID and JSON (D-158).
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
	maago.Model `maago:"table=orders,ordering=(CustomerID)"`

	ID         int64
	CustomerID *int64 `maago:"fk=Customer"`
	Status     string `maago:"max=20,default='new'"`
	Amount     int64  `maago:"decimal=2"`
	Priority   *int32 `maago:"default=010"`
	CreatedAt  time.Time
	UpdatedAt  time.Time `maago:"auto=update"`
}

// ParcelMeta is what a Parcel keeps as JSON.
type ParcelMeta struct {
	Carrier string `json:"carrier"`
	Grams   int    `json:"grams"`
}

type Parcel struct {
	maago.Model `maago:"table=parcels"`

	ID      int64
	OrderID int64      `maago:"fk=Order"`
	Key     maago.UUID `maago:"unique"`
	Meta    maago.JSON[ParcelMeta]
	Tags    *maago.JSON[[]string]
	Ref     *maago.UUID
}
