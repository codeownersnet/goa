package agent

type StreamingMode string

const (
	StreamingModeNone StreamingMode = "none"
	StreamingModeSSE  StreamingMode = "sse"
)

type RunConfig struct {
	StreamingMode StreamingMode
}
