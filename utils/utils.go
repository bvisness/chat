package utils

import (
	"fmt"
)

// We have this because otherwise passing a nil *SomeError through Must or
// Must1 will result in a non-nil interface value and a spurious panic.
type comparableError interface {
	comparable
	error
}

// Panics if the provided value is falsy (so, zero). This works for booleans
// but also normal values, through the magic of generics.
func Assert[T comparable](value T, msg ...any) {
	var zero T
	if value == zero {
		finalMsg := ""
		for i, arg := range msg {
			if i > 0 {
				finalMsg += " "
			}
			finalMsg += fmt.Sprintf("%v", arg)
		}
		panic(finalMsg)
	}
}

// Takes an (error) return and panics if there is an error.
// Helps avoid `if err != nil` in scripts.
func Must[E comparableError](err E) {
	var zero E
	if err != zero {
		panic(err)
	}
}

// Takes a (something, error) return and panics if there is an error.
// Helps avoid `if err != nil` in scripts.
func Must1[T any, E comparableError](v T, err E) T {
	var zero E
	if err != zero {
		panic(err)
	}
	return v
}

// Takes a (something, something, error) return and panics if there is an
// error. Helps avoid `if err != nil` in scripts.
func Must2[T1 any, T2 any, E comparableError](v1 T1, v2 T2, err E) (T1, T2) {
	var zero E
	if err != zero {
		panic(err)
	}
	return v1, v2
}
