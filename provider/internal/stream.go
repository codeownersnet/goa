package internal

const (
	DefaultMaxStreamBytes     = 8 << 20
	DefaultMaxStreamToolCalls = 128
	DefaultMaxResponseBytes   = 16 << 20
	DefaultMaxErrorBodyBytes  = 64 << 10
)

func StreamLimit(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
