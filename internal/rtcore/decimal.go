package rtcore

import (
	"errors"
	"strconv"
	"strings"
)

// ErrPrecise is ParseDecimal's error for more digits after the point than
// it takes.
var ErrPrecise = errors.New("too many digits after the point")

// ParseDecimal reads s, digits with an optional - before and an optional
// point among them, as a whole number of 1/10^places: "12.3" with 2 is
// 1230. More digits after the point than places is ErrPrecise, and a
// result past int64 a *strconv.NumError of strconv.ErrRange. It reads the
// forms and the defaults of decimal= fields.
func ParseDecimal(s string, places int) (int64, error) {
	whole, frac, dotted := strings.Cut(s, ".")
	digits := strings.TrimPrefix(whole, "-")
	if digits == "" || dotted && frac == "" || strings.Trim(digits+frac, "0123456789") != "" {
		return 0, &strconv.NumError{Func: "ParseDecimal", Num: s, Err: strconv.ErrSyntax}
	}
	if len(frac) > places {
		return 0, ErrPrecise
	}
	n, err := strconv.ParseInt(whole+frac+strings.Repeat("0", places-len(frac)), 10, 64)
	if err != nil {
		return 0, &strconv.NumError{Func: "ParseDecimal", Num: s, Err: err.(*strconv.NumError).Err}
	}
	return n, nil
}

// FormatDecimal is n in units of 1/10^places as a decimal: 1234 with 2 is
// "12.34", -5 is "-0.05".
func FormatDecimal(n int64, places int) string {
	s := strconv.FormatInt(n, 10)
	sign, digits := "", s
	if n < 0 {
		sign, digits = "-", s[1:]
	}
	if len(digits) <= places {
		digits = strings.Repeat("0", places-len(digits)+1) + digits
	}
	return sign + digits[:len(digits)-places] + "." + digits[len(digits)-places:]
}
