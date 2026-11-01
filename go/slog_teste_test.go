package abertura

import (
	"bytes"
	"log/slog"
)

// slogJSONSemTempo registra v com o handler JSON do slog e devolve a linha,
// sem o carimbo de tempo: os microssegundos podem conter trechos de dígitos
// que os testes de máscara proíbem na saída (ex.: "...30.701113-03:00").
func slogJSONSemTempo(chave string, v any) string {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}
	slog.New(slog.NewJSONHandler(&buf, opts)).Info("teste", chave, v)
	return buf.String()
}
