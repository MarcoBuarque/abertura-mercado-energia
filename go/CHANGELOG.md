# Changelog do SDK Go

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Versões do SDK usam as tags `go/vX.Y.Z`.

## [Não lançado]

## [0.1.0]

Primeira versão. Segue a spec `openapi/v0.1.0`; as pré-versões `go/v0.1.0-rc.N` seguem a `openapi/v0.1.0-rc.N` correspondente. Requer Go 1.26 ou mais novo.

### Adicionado

- `NewClient` com o ambiente como primeiro argumento (`Sandbox`, `Production` ou um `Environment` próprio, como o do mock) e as opções `WithCredentials`, `WithTokenSource`, `WithAgent`, `WithTimeout`, `WithHTTPClient`, `WithRetries`, `WithRateLimits`, `WithLogger` e `WithUserAgent`.
- Token de client credentials com cache, renovado 30 s antes de vencer; num 403 `ERR_CREDENCIAL_INVALIDA` do autorizador com token já aceito, renova e repete uma vez.
- Limitador local por processo: 900 requisições a cada 5 minutos no sandbox; em produção, também 9.000 nas medições.
- `Migracoes`: `Listar`, `ListarPagina`, `Obter`, `Criar`, `Editar`, `ConferirCriacao` e `ConferirEdicao`. `Medicoes`: `Listar` e `ListarPagina`. `Client.Do` para as rotas sem método, como as de solicitações.
- Retries só em leitura, salvo quando nada saiu ou o autorizador recusou antes do backend. Escrita sem resultado conhecido devolve `*UncertainResultError` com a chave natural.
- Erros `*APIError` com `Code`, `Message`, `RequestID` e `GatewayErrorType`, e as sentinelas `ErrCredencialInvalida`, `ErrValidacaoCampos`, `ErrPerfilVarejistaObrigatorio`, `ErrMigracaoSolicitada`, `ErrNotFound`, `ErrRateLimited` e `ErrCursorRepetido`.
- Tipos `Migracao`, `Medicao`, `Mes` (três formatos de mês), `Documento` (mascarado em `fmt`, `slog` e JSON) e `Decimal` (exato, aceita número ou texto).
- Log opcional com `slog`, sem query, corpo, token, credenciais nem dado pessoal.
- `Problem`, `ProblemFrom`, `APIError.Problem`, `UncertainResultError.Problem`, `ProblemContentType` e `CatalogoErrosURL`: os erros do SDK no formato da RFC 9457, ligados ao catálogo `docs/erros.md`.
- Falhas de transporte (TLS não confiável, conexão fechada sem resposta, prazo da tentativa) e respostas 2xx ilegíveis (corpo interrompido, JSON inválido, campo ilegível) entram no catálogo: `ProblemFrom` as classifica como `#erro-de-transporte`, `#tempo-esgotado` ou `#resposta-ilegivel` (entrada nova), em vez de `about:blank`. O texto dos erros e `errors.As`/`errors.Is` sobre a causa não mudam.
- `ErrArgumentoInvalido`: casa com a validação local feita antes de enviar, o que inclui o corpo que não vira JSON, o caminho inválido e os erros de validação do `NewClient`. O texto dos erros não muda.
- `ProblemFrom` não devolve mais `about:blank`: o que não tem entrada própria usa `#fora-do-catalogo`. Respostas do token sem `access_token` e erros da fonte de `WithTokenSource` usam `#falha-ao-obter-token`. O JSON de um `Problem` montado à mão omite o `type` vazio e, com `about:blank`, usa a frase padrão do status como `title` (RFC 9457, 4.2.1).
- Chaves e erros não vazam dado pessoal por `fmt` nem `slog`: `ChaveMigracao` e `ChaveEdicao` ganham `LogValue` e `GoString`, sem UC nem e-mail; `APIError` e `UncertainResultError` ganham `GoString`, sem corpo nem chave. A `Message` da API continua em `Error()`. O `json.Marshal` das chaves leva os dados em claro, para guardar a conferência.
- O pedido de token nunca segue redirecionamento (um 307 reenviaria o `client_secret`), também com `WithHTTPClient`: o SDK usa uma cópia do seu cliente com `CheckRedirect` próprio. O `CheckRedirect` do seu cliente vale só para as chamadas à API.
- Qualquer 5xx, menos o 501, segue a linha do 500: leitura repete, escrita fica incerta (`*UncertainResultError`). O 501 é erro definitivo.
- `SituacaoRejeitada` (`REJEITADA`) entra entre as situações de medição conhecidas.
- Uma página que aponta para um cursor já visto encerra com `ErrCursorRepetido` sem entregar os itens dela; antes, contra o mock, a mesma página saía duas vezes.

### Limites conhecidos

- A paginação por `indexProximaPagina` nunca foi vista funcionando; um cursor repetido encerra a iteração com `ErrCursorRepetido`.
- Não se sabe se o PUT de edição é parcial ou total: `Editar` lê a migração e envia o objeto inteiro, sem proteção contra outra escrita no meio.
- A unidade de `Consumo` não é declarada pela API.
- O limitador vale por processo; vários processos no mesmo IP ou agente somam requisições sem que ele saiba.

[Não lançado]: go/v0.1.0...HEAD
