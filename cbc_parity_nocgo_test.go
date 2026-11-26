//go:build !cgo || !cbc

package goblin_test

import (
	"testing"

	gb "github.com/paulourio/goblin"
)

func compareWithOptionalCBC(_ testing.TB, _ string, _ *gb.CompiledProblem, _ gb.Result) {}
