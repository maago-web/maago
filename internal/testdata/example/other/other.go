// Package other has a Model that is not maago.Model.
package other

import other "example.com/other"

type X struct {
	other.Model
	ID int64
}
