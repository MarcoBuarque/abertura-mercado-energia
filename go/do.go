package abertura

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// operacaoDo é o operationId das chamadas feitas por Client.Do.
const operacaoDo = "do"

// Do faz uma chamada qualquer à API, por uma rota que o SDK ainda não cobre.
//
// path é relativo ao APIURL do ambiente, começa com "/" e já vem escapado; a
// query, se houver, vai depois do "?". body é codificado como JSON (nil não
// envia corpo). out recebe a resposta 2xx decodificada; pode ser um ponteiro
// para struct, ou um *json.RawMessage para o corpo cru, ou nil para ignorá-la.
// Corpo de resposta vazio deixa out intocado.
//
// Do passa pelo mesmo laço dos métodos tipados: token, cabeçalhos,
// limitador, retries de leitura, erros ([*APIError]) e log. Escrita só repete
// nas duas exceções da tabela de retries: nada foi enviado, ou o autorizador
// recusou antes do backend. Uma escrita sem resultado conhecido, ou com
// resposta 2xx que não decodifica em out, devolve [*UncertainResultError] com
// Key nil: o SDK não sabe como conferi-la. A rota entra no log sem a query.
//
// Não há validação contra a spec nem garantia de compatibilidade entre
// versões.
func (c *Client) Do(ctx context.Context, method, path string, body, out any, opts ...CallOption) error {
	op, err := operacaoLivre(method, path, body)
	if err != nil {
		return err
	}
	_, corpo, err := c.enviar(ctx, op, novoCallConfig(opts...))
	if err != nil {
		return err
	}
	if err := decodificarSaida(corpo, out); err != nil {
		err = fmt.Errorf("%s: %w", operacaoDo, err)
		if !ehLeitura(op.metodo) {
			// A escrita foi aceita, mas não sabemos o que ela devolveu.
			return &UncertainResultError{Operation: operacaoDo, Err: err}
		}
		return err
	}
	return nil
}

// operacaoLivre monta a operação de um Do. Os erros nunca citam o caminho nem
// a query, que podem trazer dados da unidade consumidora.
func operacaoLivre(method, path string, body any) (operacao, error) {
	metodo := strings.ToUpper(strings.TrimSpace(method))
	if metodo == "" {
		return operacao{}, argumentoInvalido("%s: método vazio", operacaoDo)
	}
	if !strings.HasPrefix(path, "/") {
		return operacao{}, argumentoInvalido("%s: o caminho precisa começar com \"/\"", operacaoDo)
	}
	rota, bruta, _ := strings.Cut(path, "?")
	query, err := url.ParseQuery(bruta)
	if err != nil {
		return operacao{}, argumentoInvalido("%s: query inválida", operacaoDo)
	}
	return operacao{
		id:      operacaoDo,
		metodo:  metodo,
		modelo:  rota,
		caminho: rota,
		query:   query,
		corpo:   body,
		grupo:   grupoDaRota(rota),
	}, nil
}

// grupoDaRota manda para o balde de medições as rotas que têm o segmento
// "medicoes", que têm limite próprio em produção.
func grupoDaRota(rota string) grupoLimite {
	for seg := range strings.SplitSeq(rota, "/") {
		if seg == "medicoes" {
			return grupoMedicoes
		}
	}
	return grupoGeral
}

// decodificarSaida preenche out com o corpo de uma resposta 2xx.
func decodificarSaida(corpo []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(corpo)) == 0 {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok && raw != nil {
		*raw = append((*raw)[:0], corpo...)
		return nil
	}
	err := json.Unmarshal(corpo, out)
	var sintaxe *json.SyntaxError
	if errors.As(err, &sintaxe) {
		// A mensagem do encoding/json cita um trecho do corpo.
		return respostaIlegivel(fmt.Errorf("a resposta não é JSON válido (byte %d)", sintaxe.Offset))
	}
	if err != nil {
		return respostaIlegivel(fmt.Errorf("decodificar a resposta: %w", err))
	}
	return nil
}
