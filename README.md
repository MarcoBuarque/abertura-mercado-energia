# abertura-mercado-energia

Especificação OpenAPI e SDK em Go para as APIs de varejo da abertura do mercado de energia.

> Projeto independente e não oficial. Não é afiliado, patrocinado nem endossado pela Câmara de Comercialização de Energia Elétrica (CCEE). A descrição da API deriva da documentação pública e de testes no ambiente sandbox.

Status: **0.x**. A spec e o SDK ainda podem mudar de forma incompatível entre versões menores. Cada afirmação da spec tem fonte e grau de confiança em [`openapi/FONTES.md`](openapi/FONTES.md).

## Spec

[`openapi/abertura.yaml`](openapi/abertura.yaml) descreve 10 operações: token, migrações, medições e solicitações. Ela registra o que a API devolve de fato, inclusive onde isso difere da documentação pública (as divergências estão numeradas em `FONTES.md`).

- **Ler no navegador:** `make docs` gera `out/docs/index.html`.
- **Testar sem credenciais:** `make mock` sobe um mock em `http://localhost:4010` com os exemplos da spec, todos inventados. Para parar: `docker compose -f mock/compose.yaml down`.
- **Gerar um cliente** em Python, TypeScript ou Java, e o que ele não faz sozinho (regras de repetição, resultado incerto): [`docs/clientes.md`](docs/clientes.md).
- **Erros:** o catálogo didático, com o que fazer e quando repetir, está em [`docs/erros.md`](docs/erros.md). O SDK Go entrega cada erro também como Problem Details (RFC 9457).

## SDK Go

Requer Go 1.26 ou mais novo.

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

O ambiente é sempre explícito: nada aponta para produção sem pedido. O SDK não lê variáveis de ambiente por conta própria.

O que ele faz por você:

- **Token e limites:** busca e renova o token, e limita as requisições por processo.
- **Paginação:** `Listar` percorre as páginas; `ListarPagina` devolve o cursor para retomar depois.
- **Formatos tolerados:** três formatos de mês, detalhe em lista ou objeto, consumo como texto ou número, campos e status novos.
- **Escrita sem repetição:** `Criar` e `Editar` nunca repetem sozinhos. Se o resultado é incerto, o erro é um `*UncertainResultError` com a chave natural, e `ConferirCriacao` ou `ConferirEdicao` dizem o que aconteceu.
- **Dado pessoal:** `Documento` aparece mascarado em `fmt`, `slog` e JSON; o log do SDK nunca traz UC, documento, e-mail nem token.
- **Rotas ainda sem método:** `Client.Do` usa o mesmo laço de token, limites e erros.

Todos os exemplos rodam sem rede (`go test -run Example ./...`, em `go/`).

### Cobertura

A v0.1 cobre a v1.1 das APIs de varejo: migrações (listar, obter, criar, editar), medições (listar) e, pelo `Do`, solicitações. As unidades consumidoras dos grupos B1 a B4 dependem da v2 da API, que a spec ainda não descreve.

## Desenvolvimento

`make ci` roda os mesmos passos da integração contínua: guarda de segredos, lint da spec, mock, geração, testes, lint do Go, `govulncheck` e checagem das dependências.

O teste de contrato (`go test -tags sandbox ./contrato/...`, em `go/`) é manual: usa credenciais do sandbox, limita-se a 90 requisições e só escreve com `ABERTURA_CONTRATO_ESCRITA=1`. Os detalhes estão no cabeçalho de `go/contrato/contrato_test.go`.

## Contato

[seu e-mail]

## Licença

Apache-2.0. Veja [LICENSE](LICENSE) e [NOTICE](NOTICE).
