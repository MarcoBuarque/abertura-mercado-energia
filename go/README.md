# abertura (SDK Go)

Cliente Go para as APIs de varejo da abertura do mercado de energia.

> Projeto independente e não oficial. Não é afiliado, patrocinado nem endossado pela Câmara de Comercialização de Energia Elétrica (CCEE). A descrição da API deriva da documentação pública e de testes no ambiente sandbox.

Requer Go 1.26 ou mais novo. Status 0.x: a interface ainda pode mudar entre versões menores.

```bash
go get github.com/MarcoBuarque/abertura-mercado-energia/go
```

```go
import abertura "github.com/MarcoBuarque/abertura-mercado-energia/go"

c, err := abertura.NewClient(abertura.Sandbox,
	abertura.WithCredentials(clientID, clientSecret),
	abertura.WithAgent(agente, perfil))
if err != nil { log.Fatal(err) }
for m, err := range c.Migracoes.Listar(ctx, abertura.FiltroMigracoes{
	De: abertura.MesDe(2027, 1), Ate: abertura.MesDe(2027, 12)}) {
	if err != nil { log.Fatal(err) }
	fmt.Println(m.ID, m.Status, m.Mes)
}
```

Métodos: `Migracoes` (`Listar`, `ListarPagina`, `Obter`, `Criar`, `Editar`, `ConferirCriacao`, `ConferirEdicao`), `Medicoes` (`Listar`, `ListarPagina`) e `Client.Do` para rotas ainda sem método. Os exemplos executáveis estão no `godoc`.

Escrita com resultado incerto, dado pessoal, cobertura da API e a especificação OpenAPI: veja o [README da raiz](../README.md).
