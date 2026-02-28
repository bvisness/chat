package chat

// ----------------------------------------------------------------------------
// "Static asserts" about the program

// NOTE(ben): This app is 64-bit only.
var _ int = 1 << 31

func init() {
	// TODO(ben): Test that the system is little-endian, since we pervasively rely on
	// that assumption.
}
