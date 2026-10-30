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

// SituacaoConsistida é a situação de medição com constante nesta versão. A
// documentação pública cita também REJEITADA (ver a spec); nenhuma das duas
// foi observada no sandbox.
const SituacaoConsistida SituacaoMedicao = "CONSISTIDA"

// Conhecido informa se a situação é uma das publicadas nesta versão do SDK.
func (s SituacaoMedicao) Conhecido() bool {
	return s == SituacaoConsistida
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

// Medicao é uma medição de consumo de uma unidade consumidora.
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

// FiltroMigracoes filtra a listagem de migrações. Zero em um campo = sem filtro.
type FiltroMigracoes struct {
	// De e Ate delimitam a faixa de meses de referência.
	De, Ate Mes
	// UC filtra por unidade consumidora.
	UC string
	// Status filtra por status da migração.
	Status StatusMigracao
	// Perfil é o código do perfil; 0 usa o perfil do cliente.
	Perfil int
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

// EdicaoMigracao reúne os campos editáveis de uma migração. Só os campos que
// a spec descreve para o PUT.
type EdicaoMigracao struct {
	Mes       Mes
	Perfil    int
	Documento Documento
	Email     string
	Status    StatusMigracao
}

// ChaveMigracao identifica uma migração criada pelos seus dados: unidade
// consumidora e concessionária com migração ativa. Serve para conferir, depois
// de um resultado incerto, se a criação aconteceu.
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
