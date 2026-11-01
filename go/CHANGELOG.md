# Changelog do SDK Go

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Versões do SDK usam as tags `go/vX.Y.Z`.

## [Não lançado]

## [0.1.0]

Primeira versão. Segue a spec `openapi/v0.1.0`; a pré-versão `go/v0.1.0-rc.1` segue a `openapi/v0.1.0-rc.2`; a `go/v0.1.0-rc.2` e a `go/v0.1.0-rc.3` seguem a `openapi/v0.1.0-rc.3` (a spec foi remarcada por causa dos achados do sandbox de 01/11/2026), e não a `openapi/v0.1.0-rc.1`. A `go/v0.1.0-rc.2` não teve release publicado: a CI da tag reprovou no `govulncheck` antigo (corrigido na rc.3, que também traz os achados da quarta rodada). Requer Go 1.26 ou mais novo.

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
- `Migracao` e `Medicao` ganham `String` e `GoString`, e `NovaMigracao`, `EdicaoMigracao`, `FiltroMigracoes` e `FiltroMedicoes` ganham `String`, `GoString` e `LogValue`, todos sem UC, e-mail, documento, comentário nem consumo: `%v`, `%+v`, `%#v`, `%s` e `slog` (inclusive um `[]Migracao` no handler de texto) mostram só id, status, mês, concessionária e afins. Os campos continuam exportados e o `json.Marshal` não muda, então ele leva os dados em claro; com o handler JSON do `slog`, logue o id e não a lista inteira.
- O pedido de token nunca segue redirecionamento (um 307 reenviaria o `client_secret`), também com `WithHTTPClient`: o SDK usa uma cópia do seu cliente com `CheckRedirect` próprio. O `CheckRedirect` do seu cliente vale só para as chamadas à API.
- O pedido de token leva só `client_id` e `client_secret` no formulário, sem `grant_type`: o endpoint recusou o `grant_type` com 400 em 01/11/2026 e aceitou o formulário reduzido. `expires_in` é lido como número ou texto, e o pedido de token não segue redirecionamento.
- A faixa de meses de `Migracoes.Listar` e `ListarPagina` é obrigatória: a API responde 400 `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` sem `mesReferenciaInicial` e `mesReferenciaFinal`. O SDK valida `FiltroMigracoes.De` e `Ate` antes de enviar: ausente ou `Ate` antes de `De` dá `ErrArgumentoInvalido`, sem requisição. `FiltroMedicoes` já exigia `Inicio` e `Fim`.
- `ErrFaixaDeMesesObrigatoria` (`ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO`, só alcançável por `Client.Do`) e `ErrFaixaDeMesesLargaDemais` (`ERR_MES_REFERENCIA_INICIAL_DIFERENCA`), com as entradas `#faixa-de-meses-obrigatoria` e `#faixa-de-meses-larga-demais` no catálogo. Os dois são 400 definitivos: nem leitura nem escrita repetem.
- `ParseMes` aceita também a data `2026-12-01` e a data e hora sem deslocamento `2026-12-01T00:00:00` (com ou sem fração de segundo). Os dois formatos são tolerância: o sandbox devolveu a `dataReferencia` como RFC 3339 com deslocamento (D21), e a falha do `Obter` tem outra causa (D23); ficam por serem inofensivos. Pelo mesmo motivo, os campos de dia ou de data e hora (`dataSolicitacao`, `dataValidacao`, `dataDenuncia`) também aceitam a data e hora sem deslocamento (`2026-12-01T00:00:00`, com ou sem fração de segundo), lida em -03:00 como a data só de dia.
- `Migracoes.Obter` escolhe o item pelo `IdMigracao` quando a API devolve mais de um (um casa: devolve; nenhum: `ErrNotFound`). Se vários casam, devolve a entrada quando todas são iguais depois de decodificadas e falha com resposta ilegível (`#resposta-ilegivel`) quando diferem. O sandbox devolveu duas entradas com o mesmo id em 01/11/2026 (D23).
- O teste de contrato (`go/contrato`) começa a busca da faixa em 12 meses e imprime a contagem de itens da raiz (`itens na raiz: N`) e, para o detalhe, se as entradas são iguais e os nomes dos campos que diferem e o texto fixo dos erros do próprio SDK, sem valores.
- Qualquer 5xx, menos o 501, segue a linha do 500: leitura repete, escrita fica incerta (`*UncertainResultError`). O 501 é erro definitivo.
- `SituacaoRejeitada` (`REJEITADA`) entra entre as situações de medição conhecidas.
- Uma página que aponta para o cursor que acabou de usar (o que o mock faz) encerra com `ErrCursorRepetido` sem entregar os itens dela, que repetiriam a página anterior. Uma página que aponta para um cursor visto antes (ciclo A, B, A) entrega os itens dela e só então devolve `ErrCursorRepetido`, sem buscar de novo.

### Limites conhecidos

- A distância máxima entre `De` e `Ate` fica entre 12 e 24 meses (provavelmente 12): 12 foram aceitos e 24, 48 e 96 recusados (`ERR_MES_REFERENCIA_INICIAL_DIFERENCA`) em 01/11/2026. Quem lista uma faixa ampla deve estar pronto para `ErrFaixaDeMesesLargaDemais` e dividir a consulta.
- O GET por id devolveu duas entradas com o mesmo `idMigracao` no sandbox (01/11/2026, D23). Entradas repetidas com conteúdo diferente fazem o `Obter` falhar com `#resposta-ilegivel` até se entender o comportamento da API (não se sabe qual é a vigente).
- A paginação por `indexProximaPagina` nunca foi vista funcionando: uma faixa de 12 meses trouxe 71 e depois 92 itens numa só página (tamanho de página de pelo menos 92) e nenhum cursor; um cursor repetido encerra a iteração com `ErrCursorRepetido`.
- Não se sabe se o PUT de edição é parcial ou total: `Editar` lê a migração e envia o objeto inteiro, sem proteção contra outra escrita no meio.
- A unidade de `Consumo` não é declarada pela API.
- O limitador vale por processo; vários processos no mesmo IP ou agente somam requisições sem que ele saiba.

[Não lançado]: go/v0.1.0...HEAD
