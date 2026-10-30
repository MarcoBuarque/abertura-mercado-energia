package abertura

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// TipoDocumento é o tipo do documento de uma pessoa: CPF ou CNPJ.
type TipoDocumento string

// Tipos de documento conhecidos.
const (
	TipoCPF  TipoDocumento = "CPF"
	TipoCNPJ TipoDocumento = "CNPJ"
)

// Documento é um CPF ou CNPJ. É dado pessoal: nenhuma forma de impressão
// (fmt, slog, JSON) mostra o número. Só [Documento.Numero] o devolve em claro,
// por chamada explícita.
//
// O SDK não valida dígito verificador: o servidor é a fonte da regra, e o CNPJ
// passou a aceitar letras.
type Documento struct {
	tipo   TipoDocumento
	numero string
}

// DocumentoCPF monta um [Documento] CPF. Tira a pontuação e não valida dígito.
func DocumentoCPF(numero string) Documento {
	return Documento{tipo: TipoCPF, numero: limparDocumento(numero)}
}

// DocumentoCNPJ monta um [Documento] CNPJ. Tira a pontuação e não valida dígito.
func DocumentoCNPJ(numero string) Documento {
	return Documento{tipo: TipoCNPJ, numero: limparDocumento(numero)}
}

// limparDocumento mantém só letras e dígitos ASCII, em maiúsculas.
func limparDocumento(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		}
	}
	return b.String()
}

// Tipo devolve o tipo do documento (vazio no valor zero).
func (d Documento) Tipo() TipoDocumento {
	return d.tipo
}

// Numero devolve o número sem pontuação, em claro. Use só onde o número é
// realmente necessário, como no corpo de um envio; nunca em log.
func (d Documento) Numero() string {
	return d.numero
}

// IsZero informa se o documento está ausente.
func (d Documento) IsZero() bool {
	return d.tipo == "" && d.numero == ""
}

// String devolve o documento mascarado, como "CNPJ ************11": o tipo e
// os dois últimos caracteres. Número com dois caracteres ou menos sai todo
// mascarado. O documento ausente vira texto vazio.
func (d Documento) String() string {
	if d.IsZero() {
		return ""
	}
	n := len(d.numero)
	visivel := ""
	if n > 2 {
		visivel = d.numero[n-2:]
		n -= 2
	}
	return string(d.tipo) + " " + strings.Repeat("*", n) + visivel
}

// GoString implementa [fmt.GoStringer] com o texto mascarado de [Documento.String].
func (d Documento) GoString() string {
	return d.String()
}

// Format implementa [fmt.Formatter]: qualquer verbo (%v, %+v, %#v, %s, %d,
// %x) imprime só o texto mascarado.
func (d Documento) Format(f fmt.State, _ rune) {
	if _, err := io.WriteString(f, d.String()); err != nil {
		// fmt.State não tem como devolver erro; a escrita falhou no destino.
		return
	}
}

// LogValue implementa [slog.LogValuer] com o texto mascarado.
func (d Documento) LogValue() slog.Value {
	return slog.StringValue(d.String())
}

// MarshalText implementa [encoding.TextMarshaler] com o texto mascarado.
// Por isso o JSON de um [Documento] nunca traz o número. O envio à API usa
// [Documento.Numero] no tipo gerado interno.
func (d Documento) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}
