package abertura

import (
	"fmt"
	"log/slog"
	"time"
)

// StatusMigracao é o status de uma migração. O conjunto é aberto: a API pode
// devolver valores novos, e eles passam sem erro; use [StatusMigracao.Conhecido].
type StatusMigracao string

// Status de migração publicados pela API.
const (
	StatusCriada               StatusMigracao = "CRIADA"
	StatusConcluida            StatusMigracao = "CONCLUIDA"
	StatusReprovada            StatusMigracao = "REPROVADA"
	StatusEncerrada            StatusMigracao = "ENCERRADA"
	StatusCancelada            StatusMigracao = "CANCELADA"
	StatusExpirada             StatusMigracao = "EXPIRADA"
	StatusAprovadaComPendencia StatusMigracao = "APROVADA_COM_PENDENCIA"
)

// Conhecido informa se o status é um dos publicados nesta versão do SDK.
func (s StatusMigracao) Conhecido() bool {
	switch s {
	case StatusCriada, StatusConcluida, StatusReprovada, StatusEncerrada,
		StatusCancelada, StatusExpirada, StatusAprovadaComPendencia:
		return true
	}
	return false
}

// SituacaoMedicao é a situação de uma medição. Conjunto aberto, como [StatusMigracao].
type SituacaoMedicao string

// Situações de medição publicadas (ver a spec). A documentação pública cita as
// duas; só CONSISTIDA foi vista no sandbox.
const (
	SituacaoConsistida SituacaoMedicao = "CONSISTIDA"
	SituacaoRejeitada  SituacaoMedicao = "REJEITADA"
)

// Conhecido informa se a situação é uma das publicadas nesta versão do SDK.
func (s SituacaoMedicao) Conhecido() bool {
	return s == SituacaoConsistida || s == SituacaoRejeitada
}

// SimNao é um campo "SIM" ou "NAO" da API. Conjunto aberto; vazio é ausente.
type SimNao string

// Valores de [SimNao].
const (
	Sim SimNao = "SIM"
	Nao SimNao = "NAO"
)

// Migracao é uma migração de unidade consumidora para o mercado livre, já
// normalizada entre os formatos de lista e de detalhe da API. Campos que a API
// não enviou ficam no valor zero (0, vazio ou ausente).
//
// fmt (%v, %+v, %#v, %s) e slog mostram só o id, o status e o mês; os campos
// continuam exportados e intactos para quem grava no próprio banco. json.Marshal
// leva os campos em claro, UC e e-mail inclusive: o JSON é para guardar, nunca
// para log. Com o handler JSON do slog, logue o id e não a lista inteira, porque
// uma lista passa por json.Marshal.
type Migracao struct {
	// ID é o identificador da migração.
	ID string
	// UC é o código da unidade consumidora. Dado pessoal.
	UC string
	// Concessionaria e Supridora são códigos de agente; 0 = ausente.
	Concessionaria, Supridora int
	// Varejista é o código do agente varejista e Perfil, o do perfil.
	Varejista, Perfil int
	// Documento é o documento da unidade consumidora. Dado pessoal, sempre mascarado ao imprimir.
	Documento Documento
	// Email é o e-mail da unidade consumidora. Dado pessoal.
	Email string
	// Mes é o mês de referência (mesReferencia na lista, dataReferencia no detalhe).
	Mes Mes
	// Status é o status da migração.
	Status StatusMigracao
	// Submercado é o submercado informado.
	Submercado string
	// ValorDHC e ValorMUSD são os valores de DHC e MUSD.
	ValorDHC, ValorMUSD Decimal
	// PagamentoMulta indica se há pagamento de multa.
	PagamentoMulta SimNao
	// DataDenuncia, DataSolicitacao e DataValidacao são datas do processo.
	DataDenuncia, DataSolicitacao, DataValidacao time.Time
	// Justificativa e Comentario são textos livres; TipoConectado é o tipo de conexão.
	Justificativa, Comentario, TipoConectado string
	// IDCelebracaoCER e IndicadorProcessoSiga são identificadores auxiliares.
	IDCelebracaoCER, IndicadorProcessoSiga string
}

// LogValue implementa [slog.LogValuer]. Registra só o id, o status e o mês:
// logar a migração inteira não vaza dado pessoal.
func (m Migracao) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", m.ID),
		slog.String("status", string(m.Status)),
		slog.String("mes", m.Mes.String()),
	)
}

// String descreve a migração sem dado pessoal: só o id, o status e o mês. Vale
// para %v, %+v e %s, e portanto para um []Migracao impresso com fmt ou com o
// handler de texto do slog.
func (m Migracao) String() string {
	return fmt.Sprintf("Migracao{id=%s status=%s mes=%s}", m.ID, m.Status, m.Mes)
}

// GoString faz o %#v seguir o String, sem UC, e-mail nem documento.
func (m Migracao) GoString() string { return m.String() }

// Medicao é uma medição de consumo de uma unidade consumidora.
//
// fmt e slog mostram só o id e a situação; os campos continuam exportados.
// json.Marshal leva UC e consumo em claro (veja [Migracao]).
type Medicao struct {
	// ID é o identificador da medição (identificadorMedicaoConsumo).
	ID string
	// UC é o código da unidade consumidora. Dado pessoal.
	UC string
	// Concessionaria é o código do agente concessionária.
	Concessionaria int
	// Dia, Referencia e AtualizadaEm são o dia de referência, a data de
	// referência do consumo e a data da última atualização.
	Dia, Referencia, AtualizadaEm time.Time
	// Consumo é o consumo medido. A unidade não é declarada pela API.
	Consumo Decimal
	// TipoConsumo é o tipo de consumo informado.
	TipoConsumo string
	// Situacao é a situação da medição.
	Situacao SituacaoMedicao
}

// LogValue implementa [slog.LogValuer]. Registra só o id e a situação: nada de
// UC nem consumo.
func (m Medicao) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", m.ID),
		slog.String("situacao", string(m.Situacao)),
	)
}

// String descreve a medição sem dado pessoal: só o id e a situação.
func (m Medicao) String() string {
	return fmt.Sprintf("Medicao{id=%s situacao=%s}", m.ID, m.Situacao)
}

// GoString faz o %#v seguir o String, sem UC nem consumo.
func (m Medicao) GoString() string { return m.String() }

// FiltroMigracoes filtra a listagem de migrações. De e Ate são obrigatórios;
// zero nos demais campos = sem filtro.
type FiltroMigracoes struct {
	// De e Ate delimitam a faixa de meses de referência, e a API exige os dois.
	// Ate não pode ser anterior a De.
	De, Ate Mes
	// UC filtra por unidade consumidora.
	UC string
	// Status filtra por status da migração.
	Status StatusMigracao
	// Perfil é o código do perfil; 0 usa o perfil do cliente.
	Perfil int
}

// String descreve o filtro sem a UC: só a faixa de meses, o status e o perfil.
func (f FiltroMigracoes) String() string {
	return fmt.Sprintf("FiltroMigracoes{de=%s ate=%s status=%s perfil=%d}", f.De, f.Ate, f.Status, f.Perfil)
}

// GoString faz o %#v seguir o String, sem a UC.
func (f FiltroMigracoes) GoString() string { return f.String() }

// LogValue implementa [slog.LogValuer] sem a UC.
func (f FiltroMigracoes) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("de", f.De.String()),
		slog.String("ate", f.Ate.String()),
		slog.String("status", string(f.Status)),
		slog.Int("perfil", f.Perfil),
	)
}

// FiltroMedicoes filtra a listagem de medições.
type FiltroMedicoes struct {
	// UC e Concessionaria identificam a unidade consumidora.
	UC             string
	Concessionaria int
	// Inicio e Fim delimitam o período; são enviados em -03:00.
	Inicio, Fim time.Time
	// Situacao filtra por situação da medição.
	Situacao SituacaoMedicao
}

// String descreve o filtro sem a UC: só a concessionária, o período e a situação.
func (f FiltroMedicoes) String() string {
	return fmt.Sprintf("FiltroMedicoes{concessionaria=%d inicio=%s fim=%s situacao=%s}",
		f.Concessionaria, f.Inicio.Format(layoutDia), f.Fim.Format(layoutDia), f.Situacao)
}

// GoString faz o %#v seguir o String, sem a UC.
func (f FiltroMedicoes) GoString() string { return f.String() }

// LogValue implementa [slog.LogValuer] sem a UC.
func (f FiltroMedicoes) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("concessionaria", f.Concessionaria),
		slog.String("inicio", f.Inicio.Format(layoutDia)),
		slog.String("fim", f.Fim.Format(layoutDia)),
		slog.String("situacao", string(f.Situacao)),
	)
}

// NovaMigracao reúne os dados para criar uma migração. O código do varejista
// e o perfil vêm do cliente.
type NovaMigracao struct {
	UC             string
	Concessionaria int
	Documento      Documento
	Email          string
	Mes            Mes
	DataDenuncia   time.Time
	Comentario     string
}

// String descreve a nova migração sem UC, documento, e-mail nem comentário:
// só a concessionária e o mês.
func (n NovaMigracao) String() string {
	return fmt.Sprintf("NovaMigracao{concessionaria=%d mes=%s}", n.Concessionaria, n.Mes)
}

// GoString faz o %#v seguir o String, sem dado pessoal.
func (n NovaMigracao) GoString() string { return n.String() }

// LogValue implementa [slog.LogValuer] sem dado pessoal.
func (n NovaMigracao) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("concessionaria", n.Concessionaria),
		slog.String("mes", n.Mes.String()),
	)
}

// EdicaoMigracao reúne os campos editáveis de uma migração. Só os campos que
// a spec descreve para o PUT.
type EdicaoMigracao struct {
	Mes       Mes
	Perfil    int
	Documento Documento
	Email     string
	Status    StatusMigracao
}

// String descreve a edição sem documento nem e-mail: só o mês, o perfil e o
// status.
func (e EdicaoMigracao) String() string {
	return fmt.Sprintf("EdicaoMigracao{mes=%s perfil=%d status=%s}", e.Mes, e.Perfil, e.Status)
}

// GoString faz o %#v seguir o String, sem dado pessoal.
func (e EdicaoMigracao) GoString() string { return e.String() }

// LogValue implementa [slog.LogValuer] sem dado pessoal.
func (e EdicaoMigracao) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("mes", e.Mes.String()),
		slog.Int("perfil", e.Perfil),
		slog.String("status", string(e.Status)),
	)
}

// ChaveMigracao identifica uma migração criada pelos seus dados: unidade
// consumidora e concessionária com migração ativa. Serve para conferir, depois
// de um resultado incerto, se a criação aconteceu.
//
// fmt e slog mostram a UC mascarada; json.Marshal, não: o JSON serve para
// guardar a chave e conferir depois, e leva a UC em claro. Guarde-o como o
// resto do dado pessoal e não o use em log.
type ChaveMigracao struct {
	UC             string
	Concessionaria int
	// Mes é o mês de referência; a listagem exige faixa de meses.
	Mes Mes
	// Perfil é o perfil da consulta; 0 = o do cliente.
	Perfil int
}

// String descreve a chave sem dado pessoal: a UC sai mascarada, só com os
// quatro últimos caracteres.
func (k ChaveMigracao) String() string {
	return fmt.Sprintf("migracao uc=%s concessionaria=%d mes=%s perfil=%d",
		mascarar(k.UC, 4), k.Concessionaria, k.Mes, k.Perfil)
}

// GoString faz o %#v seguir o String, sem a UC em claro.
func (k ChaveMigracao) GoString() string { return k.String() }

// LogValue implementa [slog.LogValuer] com o String mascarado: um slog
// registrando a chave não grava a UC.
func (k ChaveMigracao) LogValue() slog.Value { return slog.StringValue(k.String()) }

func (ChaveMigracao) chaveNatural() {}

// ChaveEdicao identifica uma edição pelo id da migração e pelos campos
// desejados. Serve para conferir, depois de um resultado incerto, se a edição
// aconteceu.
//
// fmt e slog mostram só o id e o que muda; json.Marshal leva o e-mail em claro
// e o documento mascarado, e o Unmarshal desse JSON falha no documento. Para
// guardar a chave, guarde os campos à parte (o documento por
// Desejado.Documento.Numero()), como dado pessoal.
type ChaveEdicao struct {
	ID       string
	Desejado EdicaoMigracao
}

// String descreve a chave sem documento nem e-mail: só o id e o que muda.
func (k ChaveEdicao) String() string {
	return fmt.Sprintf("edicao id=%s mes=%s perfil=%d status=%s",
		k.ID, k.Desejado.Mes, k.Desejado.Perfil, k.Desejado.Status)
}

// GoString faz o %#v seguir o String, sem documento nem e-mail.
func (k ChaveEdicao) GoString() string { return k.String() }

// LogValue implementa [slog.LogValuer] com o String, sem documento nem e-mail.
func (k ChaveEdicao) LogValue() slog.Value { return slog.StringValue(k.String()) }

func (ChaveEdicao) chaveNatural() {}

// mascarar troca todos os caracteres por '*', menos os n últimos. Texto de n
// caracteres ou menos sai todo mascarado.
func mascarar(s string, n int) string {
	if s == "" {
		return ""
	}
	if len(s) <= n {
		n = 0
	}
	out := make([]byte, len(s))
	for i := range out {
		out[i] = '*'
	}
	copy(out[len(s)-n:], s[len(s)-n:])
	return string(out)
}
