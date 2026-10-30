package abertura

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// respostaDe monta uma resposta com status e cabeçalhos para os testes.
func respostaDe(status int, cabecalhos map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range cabecalhos {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h}
}

func TestDecodificarErro_WhenFormatoReal_PreencheCampos(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       int
		cabecalhos   map[string]string
		corpo        string
		wantCode     string
		wantMessage  string
		wantRequest  string
		wantGateway  string
		wantRawBytes int
	}{
		{
			name:        "backend 400 validação de campos, compacto e com milissegundos",
			status:      400,
			cabecalhos:  map[string]string{"x-amzn-RequestId": "req-inventado-1"},
			corpo:       `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_VALIDACAO_CAMPOS","message":"campo inventado inválido"}`,
			wantCode:    "ERR_VALIDACAO_CAMPOS",
			wantMessage: "campo inventado inválido",
			wantRequest: "req-inventado-1",
		},
		{
			name:        "backend 400 perfil obrigatório",
			status:      400,
			corpo:       `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_PERFIL_VAREJISTA_OBRIGATORIO","message":"perfil inventado ausente"}`,
			wantCode:    "ERR_PERFIL_VAREJISTA_OBRIGATORIO",
			wantMessage: "perfil inventado ausente",
		},
		{
			name:        "backend 400 migração já solicitada",
			status:      400,
			corpo:       `{"timestamp":"2026-01-02T03:04:05.678-03:00","status":400,"error":"ERR_MIGRACAO_SOLICITADA","message":"migração inventada repetida"}`,
			wantCode:    "ERR_MIGRACAO_SOLICITADA",
			wantMessage: "migração inventada repetida",
		},
		{
			name:   "autorizador 403 com espaços, sem milissegundos e com tipo do gateway",
			status: 403,
			cabecalhos: map[string]string{
				"x-amzn-RequestId": "req-inventado-2",
				"x-amzn-ErrorType": "AccessDeniedException",
			},
			corpo:       `{"timestamp": "2026-01-02T03:04:05-03:00", "status": 403, "error": "ERR_CREDENCIAL_INVALIDA", "message": "token inventado inválido"}`,
			wantCode:    "ERR_CREDENCIAL_INVALIDA",
			wantMessage: "token inventado inválido",
			wantRequest: "req-inventado-2",
			wantGateway: "AccessDeniedException",
		},
		{
			name:        "JSON com campos a mais é tolerado",
			status:      400,
			corpo:       `{"error":"ERR_VALIDACAO_CAMPOS","message":"m","content":{"a":1},"extra":[1,2]}`,
			wantCode:    "ERR_VALIDACAO_CAMPOS",
			wantMessage: "m",
		},
		{
			name:        "campo error que não é texto vira código vazio",
			status:      400,
			corpo:       `{"error":{"x":1},"message":"m"}`,
			wantCode:    "",
			wantMessage: "m",
		},
		{
			name:   "502 com texto puro fica sem código nem mensagem",
			status: 502,
			corpo:  "bad gateway inventado",
		},
		{
			name:   "corpo vazio",
			status: 500,
		},
		{
			name:         "corpo de 10 KiB guarda só 4 KiB",
			status:       500,
			corpo:        strings.Repeat("x", 10*1024),
			wantRawBytes: 4096,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := decodificarErro("listarMigracoes", respostaDe(tc.status, tc.cabecalhos), []byte(tc.corpo))

			if got.Operation != "listarMigracoes" {
				t.Errorf("Operation = %q", got.Operation)
			}
			if got.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, quer %d", got.StatusCode, tc.status)
			}
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, quer %q", got.Code, tc.wantCode)
			}
			if got.Message != tc.wantMessage {
				t.Errorf("Message = %q, quer %q", got.Message, tc.wantMessage)
			}
			if got.RequestID != tc.wantRequest {
				t.Errorf("RequestID = %q, quer %q", got.RequestID, tc.wantRequest)
			}
			if got.GatewayErrorType != tc.wantGateway {
				t.Errorf("GatewayErrorType = %q, quer %q", got.GatewayErrorType, tc.wantGateway)
			}
			if tc.wantRawBytes != 0 && len(got.RawBody()) != tc.wantRawBytes {
				t.Errorf("len(RawBody) = %d, quer %d", len(got.RawBody()), tc.wantRawBytes)
			}
			if tc.wantRawBytes == 0 && string(got.RawBody()) != tc.corpo {
				t.Errorf("RawBody = %q, quer o corpo original", got.RawBody())
			}
		})
	}
}

func TestDecodificarErro_WhenRespostaNula_NaoPanica(t *testing.T) {
	t.Parallel()

	got := decodificarErro("do", nil, []byte(`{"error":"ERR_X","message":"m"}`))
	if got.StatusCode != 0 || got.Code != "ERR_X" {
		t.Errorf("got = %+v", got)
	}
}

func TestAPIError_RawBody_DevolveCopia(t *testing.T) {
	t.Parallel()

	e := decodificarErro("do", respostaDe(500, nil), []byte("abc"))
	b := e.RawBody()
	b[0] = 'z'
	if string(e.RawBody()) != "abc" {
		t.Errorf("RawBody mudou por alteração externa: %q", e.RawBody())
	}
}

func TestAPIError_Error_WhenFormatado_TemOperacaoStatusECodigo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         *APIError
		wantContem  []string
		wantAusente []string
		wantMaxLen  int
	}{
		{
			name:       "operação, status e código",
			err:        &APIError{Operation: "criarMigracao", StatusCode: 400, Code: "ERR_MIGRACAO_SOLICITADA", Message: "já existe"},
			wantContem: []string{"criarMigracao", "400", "ERR_MIGRACAO_SOLICITADA", "já existe"},
		},
		{
			name:       "sem código nem mensagem",
			err:        &APIError{Operation: "do", StatusCode: 502},
			wantContem: []string{"do", "502"},
		},
		{
			name:        "mensagem longa é cortada em 200 caracteres",
			err:         &APIError{Operation: "do", StatusCode: 400, Code: "ERR_X", Message: strings.Repeat("ã", 500)},
			wantContem:  []string{strings.Repeat("ã", 200)},
			wantAusente: []string{strings.Repeat("ã", 201)},
		},
		{
			name:        "não traz corpo nem cabeçalhos",
			err:         &APIError{Operation: "do", StatusCode: 403, Code: "ERR_CREDENCIAL_INVALIDA", RequestID: "req-inventado", GatewayErrorType: "AccessDeniedException", rawBody: []byte("corpo-inventado")},
			wantContem:  []string{"403"},
			wantAusente: []string{"corpo-inventado", "AccessDeniedException"},
		},
		{
			name:        "quebras de linha na mensagem viram espaço",
			err:         &APIError{Operation: "do", StatusCode: 400, Message: "linha1\nlinha2\r\x00fim"},
			wantContem:  []string{"linha1 linha2"},
			wantAusente: []string{"\n", "\r", "\x00"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.err.Error()
			for _, c := range tc.wantContem {
				if !strings.Contains(s, c) {
					t.Errorf("Error() = %q, falta %q", s, c)
				}
			}
			for _, a := range tc.wantAusente {
				if strings.Contains(s, a) {
					t.Errorf("Error() = %q, não deveria conter %q", s, a)
				}
			}
			for _, verbo := range []string{"%v", "%+v", "%s", "%#v"} {
				if out := fmt.Sprintf(verbo, tc.err); strings.Contains(out, "corpo-inventado") {
					t.Errorf("%s vazou o corpo: %q", verbo, out)
				}
			}
		})
	}
}

func TestAPIError_Is_WhenSentinela_CasaPorCodigoOuStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"credencial inválida pelo código", &APIError{StatusCode: 403, Code: "ERR_CREDENCIAL_INVALIDA"}, ErrCredencialInvalida, true},
		{"validação pelo código", &APIError{StatusCode: 400, Code: "ERR_VALIDACAO_CAMPOS"}, ErrValidacaoCampos, true},
		{"perfil obrigatório pelo código", &APIError{StatusCode: 400, Code: "ERR_PERFIL_VAREJISTA_OBRIGATORIO"}, ErrPerfilVarejistaObrigatorio, true},
		{"migração solicitada pelo código", &APIError{StatusCode: 400, Code: "ERR_MIGRACAO_SOLICITADA"}, ErrMigracaoSolicitada, true},
		{"mesmo 400 com outro código não casa", &APIError{StatusCode: 400, Code: "ERR_VALIDACAO_CAMPOS"}, ErrMigracaoSolicitada, false},
		{"código ignora o status", &APIError{StatusCode: 500, Code: "ERR_MIGRACAO_SOLICITADA"}, ErrMigracaoSolicitada, true},
		{"403 sem código não é credencial inválida", &APIError{StatusCode: 403}, ErrCredencialInvalida, false},
		{"404 pelo status", &APIError{StatusCode: 404}, ErrNotFound, true},
		{"404 mesmo com código", &APIError{StatusCode: 404, Code: "ERR_X"}, ErrNotFound, true},
		{"429 pelo status", &APIError{StatusCode: 429}, ErrRateLimited, true},
		{"400 não é not found", &APIError{StatusCode: 400}, ErrNotFound, false},
		{"cursor repetido nunca casa com APIError", &APIError{StatusCode: 400}, ErrCursorRepetido, false},
		{"sentinela casa com ela mesma", ErrCursorRepetido, ErrCursorRepetido, true},
		{"através de fmt.Errorf %w", fmt.Errorf("contexto: %w", &APIError{StatusCode: 429}), ErrRateLimited, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := errors.Is(tc.err, tc.target); got != tc.want {
				t.Errorf("errors.Is = %v, quer %v", got, tc.want)
			}
		})
	}
}

func TestAPIError_As_WhenEmbrulhado_Funciona(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("camada: %w", &APIError{Operation: "do", StatusCode: 400})
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 400 {
		t.Fatalf("errors.As falhou: %v", err)
	}
}

type chaveFalsa struct{}

func (chaveFalsa) String() string { return "chave-inventada" }
func (chaveFalsa) chaveNatural()  {}

func TestUncertainResultError_Unwrap_WhenCausa_ExpoeCausaEChave(t *testing.T) {
	t.Parallel()

	causa := &APIError{Operation: "criarMigracao", StatusCode: 503}
	var err error = fmt.Errorf("camada: %w", &UncertainResultError{
		Operation: "criarMigracao",
		Key:       chaveFalsa{},
		Err:       causa,
	})

	var incerto *UncertainResultError
	if !errors.As(err, &incerto) {
		t.Fatal("errors.As não achou UncertainResultError")
	}
	if incerto.Key == nil || incerto.Key.String() != "chave-inventada" {
		t.Errorf("Key = %v", incerto.Key)
	}
	if !errors.Is(err, causa) {
		t.Error("errors.Is não chegou à causa")
	}
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 503 {
		t.Error("errors.As não chegou ao APIError da causa")
	}
}

func TestUncertainResultError_Error_WhenFormatado_NaoTrazChave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *UncertainResultError
		want []string
	}{
		{"com causa", &UncertainResultError{Operation: "criarMigracao", Key: chaveFalsa{}, Err: errors.New("timeout inventado")}, []string{"criarMigracao", "incerto", "timeout inventado"}},
		{"sem causa", &UncertainResultError{Operation: "do"}, []string{"do", "incerto"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.err.Error()
			for _, w := range tc.want {
				if !strings.Contains(s, w) {
					t.Errorf("Error() = %q, falta %q", s, w)
				}
			}
			if strings.Contains(s, "chave-inventada") {
				t.Errorf("Error() vazou a chave: %q", s)
			}
		})
	}
}

func TestErroSemURL_WhenURLError_RemoveURL(t *testing.T) {
	t.Parallel()

	causa := errors.New("conexão recusada inventada")
	err := erroSemURL(&url.Error{Op: "Get", URL: "https://exemplo.invalid/x?uc=000123", Err: causa})
	if strings.Contains(err.Error(), "uc=000") {
		t.Errorf("a URL vazou: %v", err)
	}
	if !errors.Is(err, causa) {
		t.Error("perdeu a causa")
	}
	if erroSemURL(nil) != nil {
		t.Error("nil deveria continuar nil")
	}
}
