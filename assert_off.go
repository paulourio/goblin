//go:build !goblin_assert

package goblin

// debugUnreachable compiles to a no-op outside goblin_assert builds.
func debugUnreachable(_ string, _ ...any) {}
