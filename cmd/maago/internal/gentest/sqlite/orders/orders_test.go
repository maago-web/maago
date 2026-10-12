package orders

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/cmd/maago/internal/gentest"
	"github.com/maago-web/maago/contrib/admin"
)

const fixture = "internal/testdata/orders"

var t0 = time.Date(2026, 9, 28, 9, 0, 0, 123_456_000, time.UTC)

func hour(n int) time.Time { return t0.Add(time.Duration(n) * time.Hour) }

func ptr[T any](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}

// show is o's columns, times as UTC to the microsecond.
func show(o Order) string {
	return fmt.Sprintf("%d %s %s %d %s %s %s", o.ID, ptr(o.CustomerID), o.Status, o.Amount, ptr(o.Priority),
		o.CreatedAt.UTC().Format(time.StampMicro), o.UpdatedAt.UTC().Format(time.StampMicro))
}

// seed inserts a customer with no orders, one with some, and six orders
// with distinct amounts and times.
func seed(t *testing.T, ctx context.Context, db maago.DB) []Order {
	t.Helper()
	if _, err := InsertCustomer(ctx, db, NewCustomerInput{Name: "lee"}); err != nil {
		t.Fatal(err)
	}
	kim, err := InsertCustomer(ctx, db, NewCustomerInput{Name: "kim"})
	if err != nil {
		t.Fatal(err)
	}
	var out []Order
	for i, o := range []struct {
		status string
		amount int64
	}{{"new", 50}, {"paid", 300}, {"deleted", 999}, {"paid", 10}, {"new", 200}, {"paid", 120}} {
		in := NewOrderInput{Status: maago.Set(o.status), Amount: o.amount, CreatedAt: hour(i + 1)}
		if i < 2 {
			in.CustomerID = &kim.ID
		}
		o, err := InsertOrder(ctx, db, in)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, o)
	}
	return out
}

func TestCRUD(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	c, err := InsertCustomer(ctx, db, NewCustomerInput{Name: "kim"})
	if err != nil || c.ID != 1 {
		t.Fatalf("customer: %+v %v", c, err)
	}

	// Left unset, a field takes its default; default=010 is 10.
	before := time.Now().Truncate(time.Microsecond)
	o, err := InsertOrder(ctx, db, NewOrderInput{Amount: 100, CreatedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	if o.UpdatedAt.Before(before) || o.UpdatedAt.Nanosecond()%1000 != 0 {
		t.Errorf("UpdatedAt %v: not now to the microsecond", o.UpdatedAt)
	}
	got, err := GetOrder(ctx, db, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := "1 nil new 100 10 Sep 28 09:00:00.123456 "; !strings.HasPrefix(show(got), want) || show(got) != show(o) {
		t.Errorf("got %s\ninsert returned %s\nwant %s…", show(got), show(o), want)
	}

	// Set gives any value: a zero value, nil for a default that is not.
	o2, err := InsertOrder(ctx, db, NewOrderInput{CustomerID: &c.ID, Status: maago.Set(""), Amount: 7, Priority: maago.Set[*int32](nil), CreatedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := GetOrder(ctx, db, o2.ID); err != nil || !strings.HasPrefix(show(got), "2 1  7 nil ") {
		t.Errorf("with Set: %s %v", show(got), err)
	}

	// Update writes every column and the new UpdatedAt, then tells o.
	got.Amount, got.Status, got.UpdatedAt = 150, "paid", time.Time{}
	if err := UpdateOrder(ctx, db, &got); err != nil {
		t.Fatal(err)
	}
	after, _ := GetOrder(ctx, db, got.ID)
	if got.UpdatedAt.IsZero() || show(after) != show(got) || after.Amount != 150 {
		t.Errorf("after update: %s, o is %s", show(after), show(got))
	}

	// Nothing to update or delete: sql.ErrNoRows, and o is unchanged.
	missing := Order{ID: 99, CreatedAt: t0, UpdatedAt: t0}
	if err := UpdateOrder(ctx, db, &missing); !errors.Is(err, sql.ErrNoRows) || !missing.UpdatedAt.Equal(t0) {
		t.Errorf("update of a missing id: %v, UpdatedAt %v", err, missing.UpdatedAt)
	}
	if err := DeleteOrder(ctx, db, 99); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("delete of a missing id: %v", err)
	}
	if err := DeleteOrder(ctx, db, o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetOrder(ctx, db, o.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("get after delete: %v", err)
	}
}

// Every combination of SearchOrders runs, and gives what a filter in Go
// gives: the key picks the right SQL text.
func TestSearchOrders(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	all := seed(t, ctx, db)
	paid, from, to := "paid", hour(2), hour(5)
	n := 0
	for _, status := range []*string{nil, &paid} {
		for _, from := range []*time.Time{nil, &from} {
			for _, to := range []*time.Time{nil, &to} {
				for _, sort := range []SearchOrdersSort{SearchOrdersSortLatest, SearchOrdersSortAmount} {
					p := SearchOrdersParams{Status: status, From: from, To: to, Sort: sort, Limit: 3}
					var want []int64
					for _, o := range all {
						if status != nil && o.Status != *status || status == nil && o.Status == "deleted" ||
							from != nil && to != nil && (o.CreatedAt.Before(*from) || o.CreatedAt.After(*to)) {
							continue
						}
						want = append(want, o.ID)
					}
					slices.SortFunc(want, func(a, b int64) int {
						if sort == SearchOrdersSortLatest {
							return all[b-1].CreatedAt.Compare(all[a-1].CreatedAt)
						}
						return cmp.Compare(all[b-1].Amount, all[a-1].Amount)
					})
					want = want[:min(len(want), 3)]
					rows, err := SearchOrders(ctx, db, p)
					if err != nil {
						t.Fatal(err)
					}
					var got []int64
					for _, r := range rows {
						got = append(got, r.ID)
					}
					if !slices.Equal(got, want) {
						t.Errorf("status %v, from %v, to %v, sort %s: %v, want %v", status != nil, from != nil, to != nil, sort, got, want)
					}
					n++
				}
			}
		}
	}
	if n != 16 {
		t.Errorf("%d combinations", n)
	}
	rows, err := SearchOrders(ctx, db, SearchOrdersParams{Status: &paid, Sort: SearchOrdersSortAmount, Limit: 1})
	if err != nil || len(rows) != 1 || fmt.Sprint(rows[0].ID, rows[0].Status, rows[0].Amount) != "2paid300" || !rows[0].CreatedAt.Equal(hour(2)) {
		t.Errorf("row: %+v %v", rows, err)
	}
	if _, err := SearchOrders(ctx, db, SearchOrdersParams{Sort: "cheap"}); err == nil ||
		err.Error() != `SearchOrders: sort "cheap" is not one of latest, amount` {
		t.Errorf("unknown sort: %v", err)
	}
}

func TestOneAndExec(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	all := seed(t, ctx, db)
	// The columns come in another order than the fields.
	if o, err := OrderByAmount(ctx, db, OrderByAmountParams{Amount: 300}); err != nil || show(o) != show(all[1]) {
		t.Errorf("OrderByAmount: %s %v, want %s", show(o), err, show(all[1]))
	}
	if _, err := OrderByAmount(ctx, db, OrderByAmountParams{Amount: 1}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("no row: %v", err)
	}
	if _, err := InsertOrder(ctx, db, NewOrderInput{Amount: 300, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if _, err := OrderByAmount(ctx, db, OrderByAmountParams{Amount: 300}); !errors.Is(err, maago.ErrManyRows) {
		t.Errorf("two rows: %v", err)
	}

	if _, err := DeleteOrders(ctx, db, DeleteOrdersParams{}); !errors.Is(err, maago.ErrEmptySlice) || !strings.HasPrefix(err.Error(), "DeleteOrders: ids: ") {
		t.Errorf("no ids: %v", err)
	}
	if n, err := DeleteOrders(ctx, db, DeleteOrdersParams{IDs: []int64{3, 4, 99}}); n != 2 || err != nil {
		t.Errorf("delete: %d %v", n, err)
	}
	if n, err := DeleteOrders(ctx, db, DeleteOrdersParams{IDs: []int64{3}}); n != 0 || err != nil {
		t.Errorf("delete again: %d %v, want 0 rows and no error", n, err)
	}
}

// Each slice's placeholders follow the scalar and the slices before it:
// were any argument off, the rows would differ.
func TestOrdersMatching(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	seed(t, ctx, db)
	times := []time.Time{hour(2), hour(4), hour(6)}
	for _, c := range []struct {
		p    OrdersMatchingParams
		want string
	}{
		{OrdersMatchingParams{IDs: []int64{1, 2, 4, 6}, Times: times, Min: 100}, "[{2 300} {6 120}]"},
		{OrdersMatchingParams{IDs: []int64{1, 2, 4, 6}, Statuses: []string{"paid", "new"}, Times: times, Min: 100}, "[{2 300} {6 120}]"},
		{OrdersMatchingParams{IDs: []int64{1, 2, 4, 6}, Statuses: []string{"paid"}, Times: times, Min: 5}, "[{2 300} {4 10} {6 120}]"},
		{OrdersMatchingParams{IDs: []int64{6, 4}, Statuses: []string{"new"}, Times: times, Min: 5}, "[]"},
	} {
		rows, err := OrdersMatching(ctx, db, c.p)
		if got := fmt.Sprint(rows); err != nil || got != c.want {
			t.Errorf("%+v: %s %v, want %s", c.p, got, err, c.want)
		}
	}
	if _, err := OrdersMatching(ctx, db, OrdersMatchingParams{IDs: []int64{1}, Statuses: []string{"new"}}); !errors.Is(err, maago.ErrEmptySlice) ||
		!strings.HasPrefix(err.Error(), "OrdersMatching: times: ") {
		t.Errorf("no times: %v", err)
	}
}

// The NULL side of an outer join: a pointer is nil, a field declared
// without ? is a Scan error that names the query.
func TestOuterJoin(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	seed(t, ctx, db)
	rows, err := CustomerOrders(ctx, db)
	var got []string
	for _, r := range rows {
		amount := "nil"
		if r.Amount != nil {
			amount = fmt.Sprint(*r.Amount)
		}
		got = append(got, fmt.Sprint(r.ID, r.Name, amount))
	}
	if want := []string{"1leenil", "2kim50", "2kim300"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("CustomerOrders: %v %v, want %v", got, err, want)
	}
	if _, err := CustomerOrdersStrict(ctx, db); err == nil || !strings.HasPrefix(err.Error(), "CustomerOrdersStrict: ") {
		t.Errorf("CustomerOrdersStrict: %v, want a Scan error", err)
	}
}

// The functions run in a transaction too.
func TestTx(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := InsertCustomer(ctx, tx, NewCustomerInput{Name: "kim"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := GetCustomer(ctx, tx, c.ID); err != nil || got != c {
		t.Errorf("in the transaction: %+v %v", got, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := GetCustomer(ctx, db, c.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("after rollback: %v", err)
	}
}

// The admin's list of orders (D-132, D-134): by CustomerID, NULL last in
// both dialects, then the newest ID; an empty table counts 0.
func TestAdmin(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	order := Admin[1]
	if n, err := order.Count(ctx, db, admin.Query{}); err != nil || n != 0 {
		t.Errorf("count of none: %d %v", n, err)
	}
	var cs []int64
	for _, name := range []string{"lee", "kim"} {
		c, err := InsertCustomer(ctx, db, NewCustomerInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		cs = append(cs, c.ID)
	}
	var ids []int64
	for _, c := range []*int64{nil, &cs[1], &cs[1], &cs[0]} {
		o, err := InsertOrder(ctx, db, NewOrderInput{CustomerID: c, Amount: 1, CreatedAt: t0})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.ID)
	}
	rows, err := order.Rows(ctx, db, admin.Query{Limit: 10})
	var got []int64
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if want := []int64{ids[3], ids[2], ids[1], ids[0]}; err != nil || !slices.Equal(got, want) {
		t.Errorf("rows: %v %v, want %v", got, err, want)
	}
}

// The admin's form of a new order: the defaults drawn, a cleared pointer
// field is NULL and not its default, the auto time is set, and an fk
// number of no row is the field's error (D-132).
func TestAdminForm(t *testing.T) {
	ctx, db := gentest.Open(t, fixture)
	order := Admin[1]
	if !order.Add || !order.Edit {
		t.Fatalf("Add %t, Edit %t", order.Add, order.Edit)
	}
	f, err := order.Form(ctx, db, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	raws := map[string]string{}
	for _, in := range f.Inputs {
		raws[in.Name] = in.Raw
	}
	if want := map[string]string{"customer_id": "", "status": "new", "amount": "", "priority": "10", "created_at": ""}; !maps.Equal(raws, want) {
		t.Errorf("new order's inputs: %v, want %v", raws, want)
	}

	vals := url.Values{"customer_id": {"999"}, "status": {"paid"}, "amount": {"5"}, "priority": {""}, "created_at": {"2026-10-13T09:30"}}
	if f, err := order.Form(ctx, db, 0, vals); err != nil || f.Saved {
		t.Errorf("an fk of no row: %v %+v", err, f)
	}
	vals.Set("customer_id", "")
	if f, err := order.Form(ctx, db, 0, vals); err != nil || !f.Saved {
		t.Fatalf("add: %v %+v", err, f)
	}
	rows, err := order.Rows(ctx, db, admin.Query{Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows: %v %v", rows, err)
	}
	o, err := GetOrder(ctx, db, rows[0].ID)
	if err != nil || o.CustomerID != nil || o.Priority != nil || o.Status != "paid" || o.Amount != 5 || o.UpdatedAt.IsZero() ||
		!o.CreatedAt.Equal(time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("the new order: %s %v", show(o), err)
	}
}
