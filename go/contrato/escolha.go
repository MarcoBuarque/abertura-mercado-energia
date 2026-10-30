//go:build sandbox

package contrato

import (
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
