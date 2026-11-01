# Changelog da spec

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Versões da spec usam as tags `openapi/vX.Y.Z`.

Esta é uma descrição **não oficial**, escrita à mão. O projeto é independente e não é afiliado, patrocinado nem endossado pela CCEE. A spec descreve o que a API devolveu em testes no sandbox, e não substitui a documentação do operador.

## [Não lançado]

### Quebras

Nenhuma.

## [0.1.0] - 2026-11-01

Primeira versão. Dez operações, de quatro grupos. A coluna de confiança é o `x-confianca` da operação: `alta` quer dizer que o comportamento foi conferido no sandbox (30/10/2026 e cinco rodadas em 01/11/2026); `media`, que a operação veio só da coleção pública do Postman e não foi exercitada (ou foi, mas sem itens na resposta). Pré-versões: `openapi/v0.1.0-rc.1`, `-rc.2` e `-rc.3`; as mudanças entre elas estão em "Alterado".

| `operationId` | Método e caminho | Confiança |
|---|---|---|
| `obterToken` | `POST /sso/oauth/token` | alta (formulário só com `client_id` e `client_secret`) |
| `listarMigracoes` | `GET /v1.1/varejista/migracoes` | alta (o cursor de página, baixa) |
| `criarMigracao` | `POST /v1.1/varejista/migracoes` | alta |
| `obterMigracao` | `GET /v1.1/varejista/migracoes/{idMigracao}` | alta (uma entrada por mês de referência) |
| `editarMigracao` | `PUT /v1/varejista/migracoes/{idMigracao}` | media |
| `listarMedicoes` | `GET /v1/varejista/consumo/medicoes` | media (lista vazia no sandbox; o item da lista, baixa) |
| `listarSolicitacoes` | `GET /v1/solicitacoes` | alta |
| `solicitarTrocaRepresentante` | `POST /v1/solicitacoes/representante` | media |
| `solicitarSuspensaoFornecimento` | `POST /v1/solicitacoes/suspensao-fornecimento-varejista` | media |
| `decidirSolicitacao` | `POST /v1/solicitacoes/{idSolicitacaoAlteracao}` | media |

### Adicionado

- As dez operações acima, com exemplos inventados (UC com zeros à frente, códigos na faixa 900000, e-mails em `example.org`).
- Respostas 400, 403, 429 e 500 em toda operação. Só o 400 e o 403 foram vistos no sandbox; 429 e 500 têm `x-confianca: baixa`.
- Extensões `x-confianca`, `x-verificado-em`, `x-fonte`, `x-divergencias` e `x-extensible-enum`.
- Vinte e três divergências entre a documentação pública e o observado (D1 a D23), numeradas em `FONTES.md`, e a seção "Rodada do contrato de 2026-11-01" com as cinco rodadas contra o sandbox.
- Schema `ErroOAuth2` (`error`, `error_description`) para o 400 de `obterToken`, que não usa o `Erro` (D17).
- Códigos `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` e `ERR_MES_REFERENCIA_INICIAL_DIFERENCA` em `Erro.error`, com as descrições e os links do catálogo (D18, D19).

### Alterado

Mudanças entre as pré-versões (rc.1 a rc.3), registradas aqui porque as pré-versões não contam como versão publicada.

- `TokenRequisicao` sem `grant_type`: o formulário com ele foi recusado com 400 em 01/11/2026 (D17). O 400 de `obterToken` passa a usar `ErroOAuth2`.
- `mesReferenciaInicial` e `mesReferenciaFinal` de `listarMigracoes` passam a `required` (D18); o teto da faixa fica entre 12 e 24 meses, provavelmente 12 (D19, D22). A faixa de 12 meses voltou 71 e depois 92 itens numa só página, sem cursor. A listagem de solicitações mantém os filtros opcionais.
- `obterMigracao`: a resposta é uma lista com uma entrada por mês de referência da migração; em 01/11/2026 vieram duas, diferentes só em `dataReferencia` (D23). A API não diz qual mês é o vigente.
- `MigracaoResumo.mesReferencia`, `MigracaoDetalhe.dataReferencia`, `dataSolicitacao`, `dataValidacao` e `dataDenuncia` (lista e detalhe) ganham `format: date-time` e o formato observado (D21); `mesReferencia` e `dataReferencia` com `x-confianca: alta`. `Erro.timestamp` descreve a fração de comprimento variável; `Token.expires_in` confirmado com três dígitos.
- `x-verificado-em` passa a 2026-11-01 em `obterToken`, `listarMigracoes`, `obterMigracao` e `listarMedicoes` (esta última segue com lista vazia e confiança `media`).
- Descrições do schema `Erro` (um código por linha) e das respostas 400, 403, 429 e 500 apontam para o catálogo de erros (`docs/erros.md`). Só texto: formato e tipos não mudam.

### Corrigido

- A spec dizia que `grant_type` era opcional no token e que os filtros de mês de migrações eram opcionais; o sandbox mostrou o contrário (D17, D18).

### Limites conhecidos

- O cursor de página (`indexProximaPagina`) nunca foi visto com valor; o tamanho da página é de ao menos 92 itens (D22).
- O teto exato da faixa de meses entre 12 e 24 não foi medido (D22).
- Qual mês do detalhe de uma migração é o vigente não está documentado (D23).
- As escritas só foram verificadas em 30/10/2026; as rodadas de 01/11/2026 foram só de leitura.
- Enums de resposta são abertos (`x-extensible-enum`); enums de parâmetro e de corpo de envio são fechados.
- `consumo` e `codigoAgenteConcessionaria` de `Medicao` não têm tipo fixo, e a unidade de `consumo` é desconhecida.
- A resposta de erro de limite de taxa (429) e de falha interna (500) é hipótese.

### Quebras

Nenhuma. As quebras entre as pré-versões (rc.1 para rc.2: o 400 do token sem `message`, e os filtros de mês de `listarMigracoes` obrigatórios) estão em "Alterado" e não contam contra uma versão publicada. Daqui em diante, uma quebra em 0.x é aceita quando registrada em `### Quebras` de `[Não lançado]`; o rótulo `quebra-spec` do PR a declara e o CI confere a declaração com essa seção (`scripts/quebra-spec.sh`).

[0.1.0]: https://github.com/MarcoBuarque/abertura-mercado-energia/releases/tag/openapi/v0.1.0
[Não lançado]: https://github.com/MarcoBuarque/abertura-mercado-energia/compare/openapi/v0.1.0...HEAD
