# Fontes e divergências da spec

Projeto independente e não oficial. Este arquivo diz de onde vem cada afirmação de `abertura.yaml` e onde a documentação pública e o comportamento observado divergem. Nada aqui reproduz texto da documentação do operador: só nomes de campo, rotas, códigos `ERR_*` e descrições escritas por nós.

## Fontes

| Fonte | O que dá | Data e hora |
|---|---|---|
| Coleção pública do Postman "Abertura de Mercado" (<https://documenter.getpostman.com/view/30322966/2sAXqqchhJ>), publicada em 16/05/2025 | Rotas, nomes de campo, enums e descrições curtas | extraída em 30/10/2026 às 10:52 (-03:00) |
| Teste manual no ambiente sandbox, com 38 requisições | Formato real de respostas e erros, tipos, nulos, validações e regras de autenticação | 30/10/2026, a partir das 11:56 (-03:00) |

Regra de precedência: **o que se observou no sandbox ganha da coleção**. O que só a coleção mostra leva `x-confianca: baixa` (ou `media`, quando a operação inteira não foi exercitada).

Os dados do sandbox são fictícios e compartilhados. Nenhum valor observado (UC, códigos de agente e de perfil, documentos, identificadores) foi copiado para os exemplos: todos são inventados, com UC de zeros à frente, códigos na faixa 900000 e e-mails em `example.org`.

## Operações

| `operationId` | Método e caminho | `x-confianca` | `x-verificado-em` | Divergências |
|---|---|---|---|---|
| `obterToken` | `POST /sso/oauth/token` | alta | 2026-10-30 | D15 |
| `listarMigracoes` | `GET /v1.1/varejista/migracoes` | alta | 2026-10-30 | D2, D8, D10, D14 |
| `criarMigracao` | `POST /v1.1/varejista/migracoes` | alta | 2026-10-30 | D2, D10, D12, D14 |
| `obterMigracao` | `GET /v1.1/varejista/migracoes/{idMigracao}` | alta | 2026-10-30 | D2, D10, D14 |
| `editarMigracao` | `PUT /v1/varejista/migracoes/{idMigracao}` | media | n/a | D10, D14 |
| `listarMedicoes` | `GET /v1/varejista/consumo/medicoes` | media | n/a | D12, D14, D16 |
| `listarSolicitacoes` | `GET /v1/solicitacoes` | alta | 2026-10-30 | D3, D5, D11, D14 |
| `solicitarTrocaRepresentante` | `POST /v1/solicitacoes/representante` | media | n/a | D4, D5, D13, D14 |
| `solicitarSuspensaoFornecimento` | `POST /v1/solicitacoes/suspensao-fornecimento-varejista` | media | n/a | D13, D14 |
| `decidirSolicitacao` | `POST /v1/solicitacoes/{idSolicitacaoAlteracao}` | media | n/a | D14 |

Toda operação declara as respostas 400, 403, 429 e 500. As duas últimas nunca foram observadas e têm `x-confianca: baixa`.

## Divergências

Cada item diz o que a coleção afirma, o que se observou e o que a spec adota.

**D1. Host dos exemplos.** Alguns exemplos da coleção apontam para um host de homologação interno do operador, e não para o sandbox nem para a produção. A spec ignora esse host e declara só o sandbox, o mock local e a produção em `servers`. Fonte: coleção, exemplo do PUT v2 da concessionária.

**D2. Três formatos de mês.** O mês de referência aparece como `AAAA-MM` (filtros e corpos de envio), como data e hora com deslocamento -03:00 sempre no dia 1 (listagem v1.1) e, no detalhe, como `dataReferencia`, cujo formato exato o teste não guardou. A spec valida `AAAA-MM` onde o servidor o aceita (envio e filtros) e deixa as datas de resposta sem `format`, para clientes gerados não quebrarem. Na listagem de solicitações, o texto da coleção dá `MM-AAAA` nos filtros de mês e `DD-MM-AAAA` nas datas de solicitação, enquanto a URL de exemplo da mesma requisição usa `AAAA-MM` e `AAAA-MM-DD`; a spec segue a URL. No sandbox, os filtros de mês das migrações em `AAAA-MM` responderam 200, mas o registro do teste não diz se os itens devolvidos respeitavam a faixa, e os filtros de solicitações não foram testados. Fonte: coleção e teste no sandbox.

**D3. Envelope da lista de solicitações.** Corrigida. A coleção sugere uma lista pura; o sandbox devolve um objeto com a lista em `solicitacao` (singular) e `indexProximaPagina`. A spec adota o objeto. Fonte: teste no sandbox.

**D4. Falta de `id` na mensagem do receptor.** A estrutura da mensagem de notificação documentada para o receptor não traz um identificador, o que dificulta a deduplicação. Não afeta as 10 operações desta versão; fica para a v0.2. Fonte: documentação do receptor.

**D5. Aprovação automática em 90 dias.** A coleção diz que a troca de representante pedida com mais de 90 dias de antecedência (resilição) é aprovada automaticamente, e com mais de 15 dias no caso de resolução. O Procedimento de Comercialização trata a falta de resposta como transferência compulsória no mês pretendido. A spec registra as duas leituras na descrição da operação e não escolhe uma. Fonte: coleção e Procedimento de Comercialização, item 3.37.3.

**D6. Formato de erro.** Corrigida. A coleção mostra dois formatos, um com chaves de nome genérico e outro com `timestamp`, `status`, `error`, `message` e `content`. O sandbox devolve, nos dois casos (backend e autorizador), só `timestamp`, `status`, `error` e `message`, sem `content`. O backend usa timestamp com milissegundos; o autorizador, sem milissegundos e com espaços depois de `:`. A spec tem um schema `Erro` com os quatro campos; `error` é um código textual estável. Fonte: teste no sandbox.

**D7. Cabeçalho `codigoAgente`.** A coleção o descreve como código da concessionária, mas o varejista envia o próprio código de agente, validado contra o token. A spec descreve como "código do agente que faz a chamada". Fonte: coleção e teste no sandbox.

**D8. Perfil obrigatório também na consulta.** A coleção lista o `codigoPerfilVarejista` da listagem de migrações sem marcá-lo como obrigatório. Sem ele, o sandbox responde 400 `ERR_PERFIL_VAREJISTA_OBRIGATORIO`, mesmo com o cabeçalho `codigoPerfil`. A spec marca o parâmetro como `required`. Fonte: teste no sandbox.

**D9. 403 no lugar de 401.** Token inválido, agente ou perfil que não pertencem à credencial voltam 403 `ERR_CREDENCIAL_INVALIDA`, com o cabeçalho `x-amzn-ErrorType: AccessDeniedException`. Nenhum 401 foi visto e não há `www-authenticate`. Os casos só se distinguem pela `message`. A spec declara 403 em toda operação e não declara 401. Fonte: teste no sandbox.

**D10. Lista × detalhe.** Os schemas diferem: a listagem usa `mesReferencia`, `codigoAgenteSupridora` e `idCelebracaoCER`; o detalhe usa `dataReferencia` e `codigoSupridora`, acrescenta `tipoConectado` e não traz `idCelebracaoCER`. O GET por identificador devolve uma lista de um item. A spec tem dois schemas, `MigracaoResumo` e `MigracaoDetalhe`. Fonte: teste no sandbox.

**D11. Obrigatoriedade de filtros de solicitações.** `tipoSolicitacao` é obrigatório de fato (a coleção o mostra opcional) e `situacaoAlteracao`, marcado como obrigatório na coleção, é opcional. O endpoint de contratos, fora desta spec, também exigiu `codigoAgenteConcessionaria` no sandbox; nas medições a spec o marca como obrigatório pela coleção, sem teste com itens. A spec segue o observado. Fonte: teste no sandbox e coleção.

**D12. UC de 15 dígitos.** A v1.1 de migrações exige UC numérica de 15 dígitos com dígito verificador (algoritmo não documentado; um valor inventado foi recusado). As medições da v1 aceitaram UC de 10 dígitos. A spec aplica `pattern` só no corpo da v1.1 e deixa a UC das medições sem `pattern`. Fonte: teste no sandbox e coleção.

**D13. Nomes no corpo e na resposta das solicitações.** Os corpos de troca de representante e de suspensão usam `codigoConcessionaria` e `codigoVarejistaNovo`; a resposta usa `codigoAgenteConcessionaria` e `codigoAgenteVarejistaNovo`. A spec mantém cada nome onde ele aparece. Fonte: coleção; resposta confirmada no sandbox.

**D14. Código como string ou inteiro.** Nos exemplos da coleção, códigos de agente, perfil e concessionária aparecem como string; no sandbox, são inteiros. A spec usa inteiro e deixa os dois campos de `Medicao` (`consumo` e `codigoAgenteConcessionaria`), nunca observados, sem tipo fixo. Os exemplos `pagina-1` (string) e `pagina-2` (número) mostram as duas formas. Fonte: coleção e teste no sandbox.

**D15. Validade do token.** A coleção mostra `expires_in` 298 em um exemplo; o sandbox devolveu 300, com `refresh_expires_in` 0 (sem token de renovação). Um token de 312 segundos ainda foi aceito, de modo que o código de token vencido não foi observado. A spec registra 300 e recomenda renovar antes do vencimento. Fonte: teste no sandbox.

**D16. Unidade de `consumo`.** Nem a coleção nem o sandbox (que devolveu medições vazias) declaram a unidade. A spec não assume nenhuma e marca o schema `Medicao` com `x-confianca: baixa`. Fonte: coleção e teste no sandbox.

## Notas sobre os enums

- `statusMigracao` e as demais listas de valores de resposta usam `x-extensible-enum`: a lista é a conhecida e pode crescer. Em parâmetros de consulta e corpos de envio, `enum` é fechado.
- `tipoSolicitacao`: os valores vêm da coleção e têm o prefixo `ALTERACAO_ALFA_VAREJISTA_`, `ALTERACAO_RETORNO_CATIVO_` ou `SUSPENSAO_FORNECIMENTO_`. Dos dez, só `SUSPENSAO_FORNECIMENTO_*` apareceu no sandbox.
- `situacaoMedicao`: `CONSISTIDA` e `REJEITADA`, só pela coleção.
