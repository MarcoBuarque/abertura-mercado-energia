# abertura-mercado-energia

[English version](README.md)

Especificação OpenAPI e SDK em Go para as APIs de varejo da abertura do mercado de energia.

> Projeto independente e não oficial. Não é afiliado, patrocinado nem endossado pela Câmara de Comercialização de Energia Elétrica (CCEE). A descrição da API deriva da documentação pública e de testes no ambiente sandbox.

Status: **0.x**. A spec e o SDK ainda podem mudar de forma incompatível entre versões menores. Cada afirmação da spec tem fonte e grau de confiança em [`openapi/SOURCES.md`](openapi/SOURCES.md) (em inglês).

> Esta é a tradução do [README em inglês](README.md), que é a versão de referência. Os nomes de código (`abertura.NewClient`, `Migrations.List` etc.) são os mesmos nas duas versões: desde a v0.2 toda a API é em inglês.

## Spec

[`openapi/abertura.yaml`](openapi/abertura.yaml) descreve 10 operações: token, migrações, medições e solicitações. Ela registra o que a API devolve de fato, inclusive onde isso difere da documentação pública (as divergências estão numeradas em `SOURCES.md`).

- **Ler no navegador:** a [página Redoc](https://marcobuarque.github.io/abertura-mercado-energia/) é publicada a cada tag final `openapi/v*`; `make docs` a gera localmente em `out/docs/index.html`.
- **Testar sem credenciais:** `make mock` sobe um mock em `http://localhost:4010` com os exemplos da spec, todos inventados. Para parar: `docker compose -f mock/compose.yaml down`.
- **Gerar um cliente** em Python, TypeScript ou Java, e o que ele não faz sozinho (regras de repetição, resultado incerto): [`docs/clients.md`](docs/clients.md).
- **Erros:** o [catálogo de erros](https://marcobuarque.github.io/abertura-mercado-energia/errors/) didático, com o que fazer e quando repetir, está em [`docs/errors.md`](docs/errors.md). O SDK Go entrega cada erro também como Problem Details (RFC 9457).

## SDK Go

Requer Go 1.26 ou mais novo.

```bash
go get github.com/MarcoBuarque/abertura-mercado-energia/abertura@v0.2.0
```

```go
import "github.com/MarcoBuarque/abertura-mercado-energia/abertura"

client, err := abertura.NewClient(abertura.Sandbox(),
	abertura.WithCredentials(clientID, clientSecret),
	abertura.WithAgent(agent, profile))
if err != nil { log.Fatal(err) }
for m, err := range client.Migrations.List(ctx, abertura.MigrationFilter{
	From: abertura.NewYearMonth(2027, time.January),
	To:   abertura.NewYearMonth(2027, time.December)}) {
	if err != nil { log.Fatal(err) }
	fmt.Println(m.ID, m.Status, m.Month)
}
```

O ambiente é sempre explícito: nada aponta para produção sem pedido. O SDK não lê variáveis de ambiente por conta própria.

Contra o mock (`make mock`), o ambiente é um `Environment` próprio, e qualquer credencial serve:

```go
mock := abertura.Environment{Name: "mock", APIURL: "http://localhost:4010",
	TokenURL: "http://localhost:4010/sso/oauth/token"}
client, err := abertura.NewClient(mock,
	abertura.WithCredentials("invented-id", "invented-secret"),
	abertura.WithAgent(900002, 900003))
if err != nil { log.Fatal(err) }
page, err := client.Migrations.ListPage(ctx, abertura.MigrationFilter{
	From: abertura.NewYearMonth(2027, time.January),
	To:   abertura.NewYearMonth(2027, time.March)}, "")
if err != nil { log.Fatal(err) }
for _, m := range page.Items {
	fmt.Println(m.ID, m.Status, m.Month) // 00000000-0000-4000-8000-000000000001 CRIADA 2027-03
}
```

O mock devolve sempre a mesma página, com o mesmo cursor. Por isso, contra ele, use `ListPage`: `List` entrega a primeira página uma vez e para com `ErrRepeatedCursor` quando a segunda aponta para o mesmo cursor, sem repetir os itens; é a guarda contra cursor que não avança.

O que ele faz por você:

- **Token e limites:** busca e renova o token, e limita as requisições por processo.
- **Paginação:** `List` percorre as páginas; `ListPage` devolve o cursor para retomar depois.
- **Formatos tolerados:** três formatos de mês, detalhe em lista ou objeto, consumo como texto ou número, campos e status novos.
- **Escrita sem repetição:** `Create` e `Edit` nunca repetem sozinhos. Se o resultado é incerto, o erro é um `*UncertainResultError` com a chave natural, e `FindCreated` ou `CheckEdited` dizem o que aconteceu.
- **Dado pessoal:** `Document` aparece mascarado em `fmt`, `slog` e JSON; o log do SDK nunca traz unidade consumidora, documento, e-mail nem token.
- **Rotas ainda sem método:** `Client.Do` usa o mesmo laço de token, limites e erros.

Todos os exemplos rodam sem rede (`go test -run Example ./...`, em `abertura/`).

### Cobertura

A v0.2 cobre a v1.1 das APIs de varejo: migrações (listar, obter, criar, editar), medições (listar) e, pelo `Do`, solicitações. As unidades consumidoras dos grupos B1 a B4 dependem da v2 da API, que a spec ainda não descreve.

## Documentação

- [Página Redoc](https://marcobuarque.github.io/abertura-mercado-energia/) e [catálogo de erros](https://marcobuarque.github.io/abertura-mercado-energia/errors/) (o endereço antigo `/erros/` redireciona para ele).
- [`docs/clients.md`](docs/clients.md): geração de clientes e regras de repetição.
- [`openapi/SOURCES.md`](openapi/SOURCES.md): fontes, confiança e divergências.
- Changelogs: [`openapi/CHANGELOG.md`](openapi/CHANGELOG.md) e [`abertura/CHANGELOG.md`](abertura/CHANGELOG.md).
- [`SECURITY.md`](SECURITY.md): como reportar uma vulnerabilidade. Nunca envie credenciais.

## Releases

Dois artefatos independentes, cada um com o seu prefixo de tag:

- `openapi/vX.Y.Z` (e `openapi/vX.Y.Z-rc.N`): a spec; publica o release do GitHub com o arquivo da spec, o site no Pages e a imagem do mock.
- `abertura/vX.Y.Z` (e `abertura/vX.Y.Z-rc.N`): o módulo Go `github.com/MarcoBuarque/abertura-mercado-energia/abertura`.

Tags puras `vX.Y.Z` não são usadas. As checagens de tag estão em `scripts/check-tag.sh`.

## Desenvolvimento

`make ci` roda os mesmos passos da integração contínua: guarda de segredos, lint da spec, mock, geração, testes, lint do Go, `govulncheck` e checagem das dependências.

O teste de contrato (`go test -tags sandbox ./contract/...`, em `abertura/`) é manual: usa credenciais do sandbox, limita-se a 90 requisições e só escreve com `ABERTURA_CONTRATO_ESCRITA=1`, `ABERTURA_UC` e `ABERTURA_DOCUMENTO` (CPF ou CNPJ válido, só do ambiente). Os detalhes estão no cabeçalho de `abertura/contract/contract_test.go`.

## Contato

Dúvidas e problemas de uso: abra uma issue neste repositório. Outros assuntos: contato@quantogasta.com.br.

## Licença

Apache-2.0. Veja [LICENSE](LICENSE) e [NOTICE](NOTICE).
