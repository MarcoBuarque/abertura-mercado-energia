package abertura

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MarcoBuarque/abertura-mercado-energia/go/internal/gen"
)

// fusoBrasilia é o -03:00 fixo da API. Fixo e sem tzdata: as datas da API
// sempre vêm nesse deslocamento.
var fusoBrasilia = time.FixedZone("-03:00", -3*60*60)

const (
	layoutDia     = "2006-01-02"
	layoutInstant = "2006-01-02T15:04:05-07:00"
	// layoutSemDeslocamento é a data e hora sem deslocamento. Como em
	// ParseMes, o Go lê também a fração de segundo mesmo sem ela no layout.
	layoutSemDeslocamento = "2006-01-02T15:04:05"
)

// erroCampo descreve um campo ilegível sem repetir o valor recebido, que pode
// ser dado da unidade consumidora.
func erroCampo(campo string) error {
	return respostaIlegivel(fmt.Errorf("abertura: o campo %q da resposta tem um valor ilegível", campo))
}

// dataDe lê uma data só de dia ("2027-01-15"), uma data e hora sem
// deslocamento ("2027-01-15T10:20:30", com ou sem fração de segundo) ou um
// instante RFC 3339. As duas primeiras valem em -03:00 e o instante é levado
// para -03:00. O formato sem deslocamento é tolerância sem ocorrência observada;
// o sandbox devolveu RFC 3339 com deslocamento ou Z (D21). Texto vazio devolve o tempo zero.
func dataDe(campo, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.ParseInLocation(layoutDia, s, fusoBrasilia); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(layoutSemDeslocamento, s, fusoBrasilia); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(fusoBrasilia), nil
	}
	return time.Time{}, erroCampo(campo)
}

// mesDe lê o mês de referência nos três formatos da API. Vazio = ausente.
func mesDe(campo, s string) (Mes, error) {
	if strings.TrimSpace(s) == "" {
		return Mes{}, nil
	}
	m, err := ParseMes(s)
	if err != nil {
		return Mes{}, erroCampo(campo)
	}
	return m, nil
}

// decimalCampo lê um decimal do modelo gerado.
func decimalCampo(campo string, n json.Number) (Decimal, error) {
	d, err := decimalDe(n)
	if err != nil {
		return Decimal{}, erroCampo(campo)
	}
	return d, nil
}

// documentoDe monta o Documento de um par tipo e número da resposta.
func documentoDe(tipo, numero string) Documento {
	if tipo == "" && numero == "" {
		return Documento{}
	}
	return Documento{tipo: TipoDocumento(strings.ToUpper(strings.TrimSpace(tipo))), numero: limparDocumento(numero)}
}

// camposMigracao são os campos que a lista e o detalhe têm em comum, já
// separados do formato de cada um. O mês e a supridora entram com o nome do
// campo de origem, para a mensagem de erro citar o certo.
type camposMigracao struct {
	id, uc                                       string
	concessionaria, supridora                    int
	varejista, perfil                            int
	tipoDocumento, numeroDocumento, email        string
	campoMes, mes                                string
	status, submercado                           string
	valorDHC, valorMUSD                          json.Number
	pagamentoMulta                               string
	dataDenuncia, dataSolicitacao, dataValidacao string
	justificativa, comentario, tipoConectado     string
	idCelebracaoCER, indicadorProcessoSiga       string
}

func (c camposMigracao) converter() (Migracao, error) {
	m := Migracao{
		ID: c.id, UC: c.uc,
		Concessionaria: c.concessionaria, Supridora: c.supridora,
		Varejista: c.varejista, Perfil: c.perfil,
		Documento:      documentoDe(c.tipoDocumento, c.numeroDocumento),
		Email:          c.email,
		Status:         StatusMigracao(c.status),
		Submercado:     c.submercado,
		PagamentoMulta: SimNao(c.pagamentoMulta),
		Justificativa:  c.justificativa, Comentario: c.comentario, TipoConectado: c.tipoConectado,
		IDCelebracaoCER: c.idCelebracaoCER, IndicadorProcessoSiga: c.indicadorProcessoSiga,
	}
	var err error
	if m.Mes, err = mesDe(c.campoMes, c.mes); err != nil {
		return Migracao{}, err
	}
	if m.ValorDHC, err = decimalCampo("valorDHC", c.valorDHC); err != nil {
		return Migracao{}, err
	}
	if m.ValorMUSD, err = decimalCampo("valorMusd", c.valorMUSD); err != nil {
		return Migracao{}, err
	}
	if m.DataDenuncia, err = dataDe("dataDenuncia", c.dataDenuncia); err != nil {
		return Migracao{}, err
	}
	if m.DataSolicitacao, err = dataDe("dataSolicitacao", c.dataSolicitacao); err != nil {
		return Migracao{}, err
	}
	if m.DataValidacao, err = dataDe("dataValidacao", c.dataValidacao); err != nil {
		return Migracao{}, err
	}
	return m, nil
}

// migracaoDeResumo converte uma migração no formato da listagem (v1.1).
func migracaoDeResumo(r gen.MigracaoResumo) (Migracao, error) {
	return camposMigracao{
		id: r.IdMigracao, uc: r.CodigoUnidadeConsumidora,
		concessionaria: r.CodigoAgenteConcessionaria, supridora: r.CodigoAgenteSupridora,
		varejista: r.CodigoAgenteVarejista, perfil: r.CodigoPerfilVarejista,
		tipoDocumento: r.TipoDocumento, numeroDocumento: r.NumeroDocumento, email: r.EmailUnidadeConsumidora,
		campoMes: "mesReferencia", mes: r.MesReferencia,
		status: r.StatusMigracao, submercado: r.Submercado,
		valorDHC: r.ValorDHC, valorMUSD: r.ValorMusd, pagamentoMulta: r.PagamentoMulta,
		dataDenuncia: r.DataDenuncia, dataSolicitacao: r.DataSolicitacao, dataValidacao: r.DataValidacao,
		justificativa: r.Justificativa, comentario: r.Comentario,
		idCelebracaoCER: r.IdCelebracaoCER, indicadorProcessoSiga: r.IndicadorProcessoSiga,
	}.converter()
}

// migracaoDeDetalhe converte uma migração no formato do detalhe (POST, GET por
// id e PUT).
func migracaoDeDetalhe(d gen.MigracaoDetalhe) (Migracao, error) {
	return camposMigracao{
		id: d.IdMigracao, uc: d.CodigoUnidadeConsumidora,
		concessionaria: d.CodigoAgenteConcessionaria, supridora: d.CodigoSupridora,
		varejista: d.CodigoAgenteVarejista, perfil: d.CodigoPerfilVarejista,
		tipoDocumento: d.TipoDocumento, numeroDocumento: d.NumeroDocumento, email: d.EmailUnidadeConsumidora,
		campoMes: "dataReferencia", mes: d.DataReferencia,
		status: d.StatusMigracao, submercado: d.Submercado,
		valorDHC: d.ValorDHC, valorMUSD: d.ValorMusd, pagamentoMulta: d.PagamentoMulta,
		dataDenuncia: d.DataDenuncia, dataSolicitacao: d.DataSolicitacao, dataValidacao: d.DataValidacao,
		justificativa: d.Justificativa, comentario: d.Comentario, tipoConectado: d.TipoConectado,
		indicadorProcessoSiga: d.IndicadorProcessoSiga,
	}.converter()
}

// medicaoDe converte uma medição.
func medicaoDe(m gen.Medicao) (Medicao, error) {
	out := Medicao{
		ID: m.IdentificadorMedicaoConsumo, UC: m.CodigoUnidadeConsumidora,
		TipoConsumo: m.TipoConsumo, Situacao: SituacaoMedicao(m.SituacaoMedicao),
	}
	var err error
	if s := strings.TrimSpace(m.CodigoAgenteConcessionaria.String()); s != "" {
		if out.Concessionaria, err = strconv.Atoi(s); err != nil {
			return Medicao{}, erroCampo("codigoAgenteConcessionaria")
		}
	}
	if out.Consumo, err = decimalCampo("consumo", m.Consumo); err != nil {
		return Medicao{}, err
	}
	if out.Dia, err = dataDe("diaReferencia", m.DiaReferencia); err != nil {
		return Medicao{}, err
	}
	if out.Referencia, err = dataDe("dataReferenciaConsumo", m.DataReferenciaConsumo); err != nil {
		return Medicao{}, err
	}
	if out.AtualizadaEm, err = dataDe("dataAtualizacao", m.DataAtualizacao); err != nil {
		return Medicao{}, err
	}
	return out, nil
}

// diaDe escreve uma data só de dia, em -03:00. O tempo zero vira texto vazio.
func diaDe(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(fusoBrasilia).Format(layoutDia)
}

// instanteDe escreve um instante com deslocamento -03:00.
func instanteDe(t time.Time) string {
	return t.In(fusoBrasilia).Format(layoutInstant)
}

// novaMigracaoGen converte o pedido público no corpo do POST. O Documento
// serializa mascarado, então o corpo leva Numero(), possível porque o tipo
// gerado é interno.
func novaMigracaoGen(n NovaMigracao, agente, perfil int) gen.NovaMigracao {
	return gen.NovaMigracao{
		CodigoAgenteConcessionaria: n.Concessionaria,
		CodigoAgenteVarejista:      agente,
		CodigoPerfilVarejista:      perfil,
		CodigoUnidadeConsumidora:   n.UC,
		Comentario:                 n.Comentario,
		DataDenuncia:               diaDe(n.DataDenuncia),
		EmailUnidadeConsumidora:    n.Email,
		MesReferencia:              n.Mes.String(),
		NumeroDocumento:            n.Documento.Numero(),
		TipoDocumento:              gen.NovaMigracaoTipoDocumento(n.Documento.Tipo()),
	}
}

// edicaoGen converte a edição pública no corpo do PUT.
func edicaoGen(e EdicaoMigracao) gen.EdicaoMigracao {
	return gen.EdicaoMigracao{
		CodigoPerfilVarejista:   e.Perfil,
		EmailUnidadeConsumidora: e.Email,
		MesReferencia:           e.Mes.String(),
		NumeroDocumento:         e.Documento.Numero(),
		StatusMigracao:          string(e.Status),
		TipoDocumento:           gen.EdicaoMigracaoTipoDocumento(e.Documento.Tipo()),
	}
}
