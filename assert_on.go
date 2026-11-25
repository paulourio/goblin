//go:build goblin_assert

package goblin

import "fmt"

// debugUnreachable panics in goblin_assert builds when an unreachable solver state is reached.
func debugUnreachable(format string, args ...any) {
	panic("goblin unreachable invariant violated: " + fmt.Sprintf(format, args...))
}
