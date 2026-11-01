package abertura

import (
	"bytes"
	"log/slog"
)

// slogJSONNoTime logs v with the slog JSON handler and returns the line
// without the timestamp: the microseconds may contain digit runs that the mask
// tests forbid in the output (for example "...30.701113-03:00").
func slogJSONNoTime(key string, v any) string {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}
	slog.New(slog.NewJSONHandler(&buf, opts)).Info("test", key, v)
	return buf.String()
}
