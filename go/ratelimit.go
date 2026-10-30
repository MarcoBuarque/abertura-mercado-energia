package abertura

import "context"

// grupoLimite escolhe o balde do limitador local.
type grupoLimite int

const (
	// grupoGeral vale para tudo, menos medições.
	grupoGeral grupoLimite = iota
	// grupoMedicoes é o balde de medições (limite maior em produção).
	grupoMedicoes
)

// limitadores guarda um balde por grupo.
type limitadores struct{}

func (limitadores) esperar(ctx context.Context, _ grupoLimite) error { return ctx.Err() }
