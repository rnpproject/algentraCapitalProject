// Package textutil contains text normalization helpers used for Python parity.
package textutil

import "golang.org/x/text/cases"

func Fold(value string) string {
	return cases.Fold().String(value)
}

func EqualFold(left, right string) bool {
	return Fold(left) == Fold(right)
}
