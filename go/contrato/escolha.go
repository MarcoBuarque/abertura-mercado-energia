//go:build sandbox

package contrato

import (
	"errors"
	"fmt"
	"time"

	abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"
)

// ehAtiva diz se a migração ainda conta como ativa. Cancelada, encerrada,
// expirada e reprovada não contam; um status novo ou vazio conta (a API pode
// criar valores).
func ehAtiva(m abertura.Migracao) bool {
	switch m.Status {
	case abertura.StatusCancelada, abertura.StatusEncerrada, abertura.StatusExpirada, abertura.StatusReprovada:
		return false
	}
	return true
}

// escolherAtiva devolve a primeira migração ativa da lista.
func escolherAtiva(achadas []abertura.Migracao) (abertura.Migracao, bool) {
	for _, m := range achadas {
		if ehAtiva(m) {
			return m, true
		}
	}
	return abertura.Migracao{}, false
}

// daUC filtra as migrações da UC e da concessionária (0 não filtra a
// concessionária). Uma lista por faixa ampla pode trazer outras UCs se o filtro
// do servidor não valer.
func daUC(lista []abertura.Migracao, uc string, concessionaria int) []abertura.Migracao {
	var out []abertura.Migracao
	for _, m := range lista {
		if m.UC == uc && (concessionaria == 0 || m.Concessionaria == concessionaria) {
			out = append(out, m)
		}
	}
	return out
}

// mesesFaixaInicial é a largura da primeira faixa tentada. Em 01/11/2026 o
// sandbox aceitou 12 meses e recusou 24 (teto entre 12 e 24, ver FONTES D22).
const mesesFaixaInicial = 12

// faixaInicial devolve a primeira faixa a tentar: mesesFaixaInicial meses
// terminando em hoje.
func faixaInicial(hoje abertura.Mes) (de, ate abertura.Mes) {
	return somarMeses(hoje, -(mesesFaixaInicial - 1)), hoje
}

// maxTentativasFaixa é o teto de requisições do passo que procura a maior faixa
// de meses que a API aceita: a faixa inicial e até seis reduções pela metade
// (a partir de 12 meses bastam quatro: 12, 6, 3 e 1).
const maxTentativasFaixa = 7

// metadeDaFaixa devolve a metade da faixa de n meses, com mínimo de um mês.
func metadeDaFaixa(n int) int {
	return max(1, n/2)
}

// mesesNaFaixa conta os meses de de a ate, os dois incluídos.
func mesesNaFaixa(de, ate abertura.Mes) int {
	return (ate.Ano*12 + int(ate.Mes)) - (de.Ano*12 + int(de.Mes)) + 1
}

// somarMeses soma n meses (n pode ser negativo) a m.
func somarMeses(m abertura.Mes, n int) abertura.Mes {
	total := m.Ano*12 + int(m.Mes) - 1 + n
	return abertura.MesDe(total/12, time.Month(total%12+1))
}

// faixaCentrada devolve a faixa de n meses centrada em centro: com n par, um
// mês a mais antes que depois. Com n = 1 é o próprio mês.
func faixaCentrada(centro abertura.Mes, n int) (de, ate abertura.Mes) {
	de = somarMeses(centro, -(n / 2))
	return de, somarMeses(de, n-1)
}

// faixaAceita é o resultado de [buscarFaixaAceita].
type faixaAceita struct {
	// De, Ate e Meses descrevem a faixa que a API aceitou.
	De, Ate abertura.Mes
	Meses   int
	// Recusada é o tamanho, em meses, da última faixa recusada por ser larga
	// demais; 0 se a primeira foi aceita.
	Recusada int
	// Pagina é a primeira página devolvida para a faixa aceita.
	Pagina abertura.Pagina[abertura.Migracao]
}

// buscarFaixaAceita pede a primeira página de de0 a ate0 e, enquanto a API
// responde ErrFaixaDeMesesLargaDemais, reduz a faixa pela metade, sempre
// centrada em centro, até uma página vir ou a faixa chegar a um mês. No máximo
// [maxTentativasFaixa] requisições. Outro erro encerra a busca.
func buscarFaixaAceita(de0, ate0, centro abertura.Mes, tenta func(de, ate abertura.Mes) (abertura.Pagina[abertura.Migracao], error)) (faixaAceita, error) {
	de, ate := de0, ate0
	recusada := 0
	for tentativa := 1; ; tentativa++ {
		n := mesesNaFaixa(de, ate)
		pag, err := tenta(de, ate)
		if err == nil {
			return faixaAceita{De: de, Ate: ate, Meses: n, Recusada: recusada, Pagina: pag}, nil
		}
		if !errors.Is(err, abertura.ErrFaixaDeMesesLargaDemais) || n == 1 || tentativa >= maxTentativasFaixa {
			return faixaAceita{}, fmt.Errorf("faixa de %d meses: %w", n, err)
		}
		recusada = n
		de, ate = faixaCentrada(centro, metadeDaFaixa(n))
	}
}
