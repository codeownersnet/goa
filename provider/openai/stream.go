package openai

import (
	"io"

	internal "github.com/codeownersnet/goa/provider/internal"
)

func newSSEScanner(r io.Reader) *internal.SSEScanner {
	return internal.NewSSEScanner(r)
}
