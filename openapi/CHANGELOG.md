# Changelog da spec

Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Versões da spec usam as tags `openapi/vX.Y.Z`.

Esta é uma descrição **não oficial**, escrita à mão. O projeto é independente e não é afiliado, patrocinado nem endossado pela CCEE. A spec descreve o que a API devolveu em testes no sandbox, e não substitui a documentação do operador.

## [Não lançado]

### Alterado

- Descrições do schema `Erro` (um código por linha) e das respostas 400, 403, 429 e 500 apontam para o catálogo de erros (`docs/erros.md`). Só texto: formato e tipos não mudam.

## [0.1.0]

Primeira versão. Dez operações, de quatro grupos. A coluna de confiança é o `x-confianca` da operação: `alta` quer dizer que o comportamento foi conferido no sandbox em 30/10/2026; `media`, que a operação veio só da coleção pública do Postman e não foi exercitada (ou foi, mas sem itens na resposta).

| `operationId` | Método e caminho | Confiança |
|---|---|---|
| `obterToken` | `POST /sso/oauth/token` | alta |
| `listarMigracoes` | `GET /v1.1/varejista/migracoes` | alta (o cursor de página, baixa) |
| `criarMigracao` | `POST /v1.1/varejista/migracoes` | alta |
| `obterMigracao` | `GET /v1.1/varejista/migracoes/{idMigracao}` | alta |
| `editarMigracao` | `PUT /v1/varejista/migracoes/{idMigracao}` | media |
| `listarMedicoes` | `GET /v1/varejista/consumo/medicoes` | media (o item da lista, baixa) |
| `listarSolicitacoes` | `GET /v1/solicitacoes` | alta |
| `solicitarTrocaRepresentante` | `POST /v1/solicitacoes/representante` | media |
| `solicitarSuspensaoFornecimento` | `POST /v1/solicitacoes/suspensao-fornecimento-varejista` | media |
| `decidirSolicitacao` | `POST /v1/solicitacoes/{idSolicitacaoAlteracao}` | media |

### Adicionado

- As dez operações acima, com exemplos inventados (UC com zeros à frente, códigos na faixa 900000, e-mails em `example.org`).
- Respostas 400, 403, 429 e 500 em toda operação. Só o 400 e o 403 foram vistos no sandbox; 429 e 500 têm `x-confianca: baixa`.
- Extensões `x-confianca`, `x-verificado-em`, `x-fonte`, `x-divergencias` e `x-extensible-enum`.
- Dezesseis divergências entre a documentação pública e o observado, numeradas em `FONTES.md`.

### Limites conhecidos

- Datas de resposta ficam sem `format`: o mês de referência aparece em três formatos e o teste não guardou todos os valores.
- Enums de resposta são abertos (`x-extensible-enum`); enums de parâmetro e de corpo de envio são fechados.
- `consumo` e `codigoAgenteConcessionaria` de `Medicao` não têm tipo fixo, e a unidade de `consumo` é desconhecida.
- A resposta de erro de limite de taxa (429) e de falha interna (500) é hipótese.

### Quebras

Nenhuma. A seção só existirá em versões 0.x futuras que quebrem clientes, com o rótulo `quebra-spec` (ver `scripts/oasdiff.sh`).

[Não lançado]: openapi/v0.1.0...HEAD
