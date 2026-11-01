# Catálogo de erros

> **Descrição não oficial.** Este projeto é independente: não é afiliado, patrocinado nem endossado pelo operador da API. O catálogo descreve o que a API devolveu em testes no sandbox e o que o SDK Go faz com cada erro. Os exemplos são inventados.

Esta é a fonte didática de cada erro que você pode ver ao usar a API, o SDK Go (`go/`) ou um cliente gerado a partir de `openapi/abertura.yaml`. Cada entrada tem uma âncora estável, como `#migracao-solicitada`; o campo `type` do Problem Details do SDK aponta para ela.

## Dois formatos

A API devolve erros com **quatro campos**: `timestamp`, `status`, `error` (um código textual como `ERR_MIGRACAO_SOLICITADA`) e `message` (texto em português, para humanos), com `Content-Type: application/json`. A spec descreve esse formato como ele é, e o mock o repete; por isso ela não declara `application/problem+json` em nenhuma resposta.

O SDK Go entrega o mesmo erro como **Problem Details** da [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) (`application/problem+json`): `type` aponta para uma entrada deste catálogo, `title` e `detail` são texto nosso, e `code`, `operation`, `requestId`, `retryable` e `uncertain` são extensões. O `detail` nunca repete a `message` da API, que pode citar unidade consumidora, documento ou e-mail; ela continua em `APIError.Message` para quem quiser usá-la.

Como a API devolve (valores inventados):

```json
{"timestamp":"2027-01-10T10:00:00.000-03:00","status":400,"error":"ERR_MIGRACAO_SOLICITADA","message":"Inventado: já existe migração ativa para a unidade consumidora"}
```

Como o SDK entrega, com `abertura.ProblemFrom(err)`:

```json
{"type":"https://marcobuarque.github.io/abertura-mercado-energia/erros#migracao-solicitada","title":"Migração já solicitada","status":400,"detail":"Operação criarMigracao: Já existe migração ativa para a mesma unidade consumidora e concessionária. Se a criação anterior ficou incerta, ela provavelmente valeu: confira.","code":"ERR_MIGRACAO_SOLICITADA","operation":"criarMigracao","requestId":"req-inventado-0001","retryable":false,"uncertain":false}
```

Esta página é publicada no GitHub Pages a cada tag final `openapi/vX.Y.Z`, só com o repositório público (`make docs` gera `out/docs/erros/index.html`); até a primeira publicação o endereço de `type` não responde, e este arquivo é a fonte. Para quem não usa Go, a seção "Problem Details em um cliente Python" de [`clientes.md`](clientes.md) mostra a conversão.

## Índice

| Entrada | Código da API | Status | Repetir um GET | Repetir uma escrita |
|---|---|---|---|---|
| [Credencial recusada pelo autorizador](#credencial-invalida-autorizador) | `ERR_CREDENCIAL_INVALIDA` | 403 | não (o SDK renova o token sozinho quando cabe) | idem |
| [Credencial recusada pelo backend](#credencial-invalida-backend) | `ERR_CREDENCIAL_INVALIDA` | 403 | não | não |
| [Campo inválido na requisição](#validacao-campos) | `ERR_VALIDACAO_CAMPOS` | 400 | não | não |
| [Perfil de varejista obrigatório](#perfil-varejista-obrigatorio) | `ERR_PERFIL_VAREJISTA_OBRIGATORIO` | 400 | não | não |
| [Migração já solicitada](#migracao-solicitada) | `ERR_MIGRACAO_SOLICITADA` | 400 | não | não (confira) |
| [Faixa de meses obrigatória](#faixa-de-meses-obrigatoria) | `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO` | 400 | não | não |
| [Faixa de meses larga demais](#faixa-de-meses-larga-demais) | `ERR_MES_REFERENCIA_INICIAL_DIFERENCA` | 400 | não | não |
| [Código de erro desconhecido do SDK](#codigo-desconhecido) | `ERR_*` novo | qualquer | não | não |
| [Recusa sem código da API](#recusa-sem-codigo) | nenhum | 4xx | não | não |
| [Recurso não encontrado](#nao-encontrado) | nenhum | 404 | não | não |
| [Limite de requisições excedido](#limite-de-requisicoes) | nenhum | 429 | sim, com espera | só com a marca do gateway |
| [Erro interno da API](#erro-interno) | nenhum | 500, 503 e outros 5xx, menos 501 | sim | incerto: confira |
| [Falha no gateway da API](#falha-do-gateway) | nenhum | 502, 504 | sim | incerto: confira |
| [Falha de rede](#erro-de-transporte) | nenhum | sem resposta | sim | sim se nada saiu; senão incerto |
| [Tempo esgotado](#tempo-esgotado) | nenhum | sem resposta | sim | sim se nada saiu; senão incerto |
| [Chamada cancelada](#chamada-cancelada) | nenhum | sem resposta | só você decide | incerto se já saiu |
| [Resultado incerto da escrita](#resultado-incerto) | depende da causa | depende da causa | não se aplica | não: confira |
| [Resposta ilegível](#resposta-ilegivel) | nenhum | 2xx | sim | incerto: confira |
| [Cursor de paginação repetido](#cursor-repetido) | nenhum | sem erro HTTP | não | não se aplica |
| [Falha ao obter o token](#falha-ao-obter-token) | pode vir do endpoint | do endpoint | você pode, em 429 e 5xx (o SDK não repete) | idem |
| [Argumento inválido](#argumento-invalido) | nenhum | sem requisição | não | não |
| [Erro fora do catálogo](#fora-do-catalogo) | nenhum | sem resposta | depende | depende |

"Repetir" segue a tabela de retries do SDK (a mesma regra de [`clientes.md`](clientes.md), seção 5): o SDK repete sozinho até 2 vezes o que está marcado como seguro, e mostra o erro só depois. `retryable` no Problem diz se repetir **agora** é seguro; `uncertain` diz que a escrita pode ter sido aplicada.

Confiança: **visto no sandbox** quer dizer observado em 30/10/2026 (quatro códigos) ou em 01/11/2026 (os dois de faixa de meses); **baixa** quer dizer não observado (hipótese a partir da estrutura do gateway ou da coleção pública); **do SDK** quer dizer comportamento do próprio SDK, coberto por testes locais. A coleção pública do Postman não cita nenhum código `ERR_*` além dos seis vistos no sandbox (conferido em 30/10/2026).

---

<a id="credencial-invalida-autorizador"></a>
## Credencial recusada pelo autorizador

- **Código da API:** `ERR_CREDENCIAL_INVALIDA`.
- **Status:** 403 (a API nunca devolve 401), com o cabeçalho `x-amzn-ErrorType`.
- **Quando acontece:** o autorizador do API Gateway recusou o token (por exemplo, "não é um token JWT válido") ou a combinação de `codigoAgente` e `codigoPerfil` que não pertence à credencial. Só a `message` distingue os casos. Um token vencido não foi observado: o sandbox aceitou um token 12 s depois do `expires_in`.
- **O que fazer:** se o token funcionava e parou, o SDK o renova e repete **uma vez**, sozinho, em qualquer método (o gateway recusou antes do backend, então nada foi processado, nem num POST). Se o erro chega até você, o problema é agente, perfil ou credencial: confira `WithAgent` e `WithCredentials` e o ambiente. Repetir o mesmo pedido só gasta requisição.
- **Repetir:** GET e escrita, não, depois da renovação automática. A renovação vale só quando o token já tinha sido aceito antes por uma resposta sem a marca do autorizador; sem isso, o erro é de configuração. Com `WithTokenSource`, o SDK não renova (o cache é seu).
- **Sentinela Go:** `ErrCredencialInvalida` (casa pelo código, nos dois 403).
- **Confiança:** visto no sandbox.

<a id="credencial-invalida-backend"></a>
## Credencial recusada pelo backend

- **Código da API:** `ERR_CREDENCIAL_INVALIDA`.
- **Status:** 403, **sem** `x-amzn-ErrorType`.
- **Quando acontece:** o código vem do backend e não do autorizador. Não foi observado; o SDK o separa do caso anterior porque a regra de renovação só vale para a recusa do autorizador, que acontece antes do backend. Sem essa marca, o SDK não renova o token nem repete.
- **O que fazer:** confira credencial, agente, perfil e ambiente. Não repita às cegas.
- **Repetir:** GET e escrita, não.
- **Sentinela Go:** `ErrCredencialInvalida`.
- **Confiança:** baixa.

<a id="validacao-campos"></a>
## Campo inválido na requisição

- **Código da API:** `ERR_VALIDACAO_CAMPOS`.
- **Status:** 400.
- **Quando acontece:** um campo do corpo ou da consulta foi recusado. No sandbox, a UC com letras, com menos de 15 dígitos ou com dígito verificador errado deu este código; os outros campos (mês, documento, e-mail) não foram testados um a um. A API valida **um campo por vez** e para na primeira falha: corrigir um pode revelar o próximo.
- **O que fazer:** leia a `message` (em `APIError.Message`), corrija o campo e envie de novo. Num cliente gerado, mande o mês como `AAAA-MM`.
- **Repetir:** GET e escrita, não, sem corrigir antes.
- **Sentinela Go:** `ErrValidacaoCampos`.
- **Confiança:** visto no sandbox.

<a id="perfil-varejista-obrigatorio"></a>
## Perfil de varejista obrigatório

- **Código da API:** `ERR_PERFIL_VAREJISTA_OBRIGATORIO`.
- **Status:** 400.
- **Quando acontece:** a listagem de migrações foi pedida sem o perfil na consulta. O perfil é exigido no cabeçalho **e** na consulta.
- **O que fazer:** informe o perfil. O SDK já o envia nos dois lugares; num cliente próprio, acrescente `codigoPerfilVarejista` à consulta (o cabeçalho continua `codigoPerfil`).
- **Repetir:** GET e escrita, não, sem corrigir antes.
- **Sentinela Go:** `ErrPerfilVarejistaObrigatorio`.
- **Confiança:** visto no sandbox.

<a id="migracao-solicitada"></a>
## Migração já solicitada

- **Código da API:** `ERR_MIGRACAO_SOLICITADA`.
- **Status:** 400.
- **Quando acontece:** um POST de migração para uma UC e uma concessionária que já têm migração **ativa**. Migração encerrada ou cancelada não foi testada.
- **O que fazer:** se foi você mesmo que repetiu depois de um [resultado incerto](#resultado-incerto), a primeira criação provavelmente valeu: liste a UC com `ConferirCriacao` para ver o que existe. Não é um erro de dado: é a defesa do servidor contra duplicata.
- **Repetir:** GET não se aplica; escrita, não (confira antes).
- **Sentinela Go:** `ErrMigracaoSolicitada`.
- **Confiança:** visto no sandbox.

<a id="faixa-de-meses-obrigatoria"></a>
## Faixa de meses obrigatória

- **Código da API:** `ERR_MES_REFERENCIA_INICIAL_OBRIGATORIO`.
- **Status:** 400.
- **Quando acontece:** a listagem de migrações foi pedida sem `mesReferenciaInicial` e `mesReferenciaFinal` (`AAAA-MM`). A API exige os dois; visto no sandbox em 01/11/2026.
- **O que fazer:** informe a faixa. O SDK valida `De` e `Ate` antes de enviar e devolve `ErrArgumentoInvalido` sem fazer requisição; só `Client.Do` chega a este código. Num cliente próprio, acrescente os dois meses à consulta.
- **Repetir:** GET e escrita, não, sem corrigir antes.
- **Sentinela Go:** `ErrFaixaDeMesesObrigatoria`.
- **Confiança:** visto no sandbox.

<a id="faixa-de-meses-larga-demais"></a>
## Faixa de meses larga demais

- **Código da API:** `ERR_MES_REFERENCIA_INICIAL_DIFERENCA`.
- **Status:** 400.
- **Quando acontece:** a distância entre o mês inicial e o final passou do limite da API. O limite não foi publicado nem medido: uma faixa de 96 meses foi recusada e uma de um mês foi aceita (sandbox, 01/11/2026).
- **O que fazer:** estreite a faixa ou divida a consulta em partes menores, uma por vez.
- **Repetir:** GET e escrita, não, sem estreitar a faixa antes.
- **Sentinela Go:** `ErrFaixaDeMesesLargaDemais`.
- **Confiança:** visto no sandbox (o limite é desconhecido).

<a id="codigo-desconhecido"></a>
## Código de erro desconhecido do SDK

- **Código da API:** um `ERR_*` que o SDK ainda não conhece.
- **Status:** o que a API devolveu.
- **Quando acontece:** a API ganhou um código novo. O SDK não quebra: o erro continua um `*APIError` com `Code`, `Message` e `StatusCode`, e o Problem usa esta entrada, com o código em `code`. Códigos fora da forma `ERR_` seguida de maiúsculas, dígitos e sublinhados são descartados do Problem e caem em [recusa sem código](#recusa-sem-codigo).
- **O que fazer:** trate pelo `status` e pelo `code`; a `message` está em `APIError.Message`. Abra uma issue para o código entrar no catálogo.
- **Repetir:** segue o status: 429 e 5xx como nas entradas deles; o resto, não.
- **Sentinela Go:** nenhuma; compare `APIError.Code`.
- **Confiança:** do SDK.

<a id="recusa-sem-codigo"></a>
## Recusa sem código da API

- **Código da API:** nenhum. O corpo não é JSON, ou o campo `error` não tem a forma de um código.
- **Status:** 4xx que não seja 404 nem 429 (por exemplo, um 403 sem `ERR_*`), ou um 3xx, que o SDK não segue.
- **Quando acontece:** uma recusa de um componente que não fala o formato da API. Hipótese não verificada: um limite de taxa por IP, aplicado por um firewall de aplicação, pode responder 403.
- **O que fazer:** use o `status` e o `requestId` para investigar com o operador. Não repita: um 403 sem código `ERR_*` não é repetido pelo SDK.
- **Repetir:** GET e escrita, não.
- **Sentinela Go:** nenhuma.
- **Confiança:** baixa.

<a id="nao-encontrado"></a>
## Recurso não encontrado

- **Código da API:** nenhum.
- **Status:** 404. O SDK também devolve este erro, sem requisição extra, quando uma consulta por id volta com lista vazia (`Obter`).
- **Quando acontece:** o id da migração não existe para este agente e perfil.
- **O que fazer:** confira o id e se o agente e o perfil são os da criação.
- **Repetir:** GET e escrita, não.
- **Sentinela Go:** `ErrNotFound`.
- **Confiança:** baixa para o 404 da API (não observado); do SDK para a lista vazia.

<a id="limite-de-requisicoes"></a>
## Limite de requisições excedido

- **Código da API:** nenhum.
- **Status:** 429, com ou sem a marca `x-amzn-ErrorType` do gateway.
- **Quando acontece:** o gateway limitou a taxa. O limite conhecido é de 1.000 requisições por IP a cada 5 minutos e, em produção, de 10.000 por agente nas medições; o limitador local do SDK usa 900 e 9.000, com folga. Nunca foi observado.
- **O que fazer:** espere. O SDK repete leituras sozinho, com espera exponencial; `Retry-After`, se vier, manda (teto de 30 s). Se chegou até você, reduza o ritmo.
- **Repetir:** GET, sim, com espera. Escrita: só se a resposta traz a marca do gateway (ele limitou antes do backend); sem a marca, não.
- **Sentinela Go:** `ErrRateLimited`.
- **Confiança:** baixa.

<a id="erro-interno"></a>
## Erro interno da API

- **Código da API:** nenhum conhecido.
- **Status:** 500, 503 e qualquer outro 5xx, menos o 501. O 501 (não implementado) é definitivo: o SDK não repete e não trata a escrita como incerta.
- **Quando acontece:** o backend falhou. Nunca foi observado no sandbox.
- **O que fazer:** em leitura, o SDK já tentou de novo; tente mais tarde. Numa **escrita**, o resultado é incerto: o backend pode ter aplicado a mudança antes de falhar. Siga o passo a passo de [resultado incerto](#resultado-incerto).
- **Repetir:** GET, sim. POST e PUT, não: confira antes. No 501, nem GET.
- **Sentinela Go:** nenhuma.
- **Confiança:** baixa.

<a id="falha-do-gateway"></a>
## Falha no gateway da API

- **Código da API:** nenhum.
- **Status:** 502 e 504. Não observados; o corpo pode não seguir o formato de quatro campos (até texto puro), e o SDK aceita qualquer corpo.
- **Quando acontece:** o gateway não conseguiu falar com o backend, ou o backend demorou mais que o corte do gateway (perto de 30 s).
- **O que fazer:** igual ao [erro interno](#erro-interno): leitura, tente mais tarde; escrita, confira antes de repetir.
- **Repetir:** GET, sim. POST e PUT, não.
- **Sentinela Go:** nenhuma.
- **Confiança:** baixa.

<a id="erro-de-transporte"></a>
## Falha de rede

- **Código da API:** nenhum.
- **Status:** nenhum (não houve resposta).
- **Quando acontece:** DNS, conexão recusada, TLS (por exemplo, certificado não confiável) ou conexão fechada sem resposta (`EOF`). O erro sai **sem a URL**, que traria a UC e o mês na consulta.
- **O que fazer:** confira a rede e o ambiente. O SDK já repetiu até 2 vezes quando era seguro.
- **Repetir:** GET, sim. Escrita: sim se a falha foi antes de escrever (nada saiu); se a falha foi depois, o resultado é [incerto](#resultado-incerto).
- **Sentinela Go:** nenhuma; a causa (`x509.UnknownAuthorityError`, `io.EOF`, `net.Error`) continua acessível por `errors.As` e `errors.Is`.
- **Confiança:** do SDK.

<a id="tempo-esgotado"></a>
## Tempo esgotado

- **Código da API:** nenhum.
- **Status:** nenhum.
- **Quando acontece:** a tentativa passou de 30 s (ou do `WithTimeout`), o prazo do `context` venceu, ou o limitador local não liberou a vez a tempo. Casa com `context.DeadlineExceeded` por `errors.Is`.
- **O que fazer:** aumente o prazo se a operação for pesada; reduza o ritmo se foi o limitador local.
- **Repetir:** GET, sim, quando foi o prazo da tentativa; se foi o prazo do seu `context`, o SDK para e a decisão é sua. Escrita enviada, não: [incerto](#resultado-incerto).
- **Sentinela Go:** `context.DeadlineExceeded`.
- **Confiança:** do SDK.

<a id="chamada-cancelada"></a>
## Chamada cancelada

- **Código da API:** nenhum.
- **Status:** nenhum.
- **Quando acontece:** o `context` da chamada foi cancelado. Casa com `context.Canceled`.
- **O que fazer:** nada: foi você quem desistiu. Numa escrita que já saiu, o erro é [incerto](#resultado-incerto).
- **Repetir:** o SDK nunca repete depois de um cancelamento; a decisão é sua.
- **Sentinela Go:** `context.Canceled`.
- **Confiança:** do SDK.

<a id="resultado-incerto"></a>
## Resultado incerto da escrita

- **Código da API:** o da causa, se houver.
- **Status:** o da causa (por exemplo 502), ou nenhum.
- **Quando acontece:** num `Criar` ou `Editar` (ou num `Do` de escrita), a requisição saiu, mas a resposta não diz se foi aplicada: erro de rede depois do envio, tempo esgotado, 5xx, ou 2xx com corpo ilegível. O erro é um `*UncertainResultError`, com a causa em `Err` e a chave natural em `Key`.
- **O que fazer, passo a passo:**
  1. Não repita a escrita às cegas.
  2. Pegue o erro: `var inc *abertura.UncertainResultError; errors.As(err, &inc)`. A chave fica em `inc.Key`, do tipo `ChaveNatural`; converta para o tipo concreto com `k, ok := inc.Key.(abertura.ChaveMigracao)` (ou `abertura.ChaveEdicao`).
  3. Para uma criação, chame `ConferirCriacao(ctx, k)` com a `ChaveMigracao`; lista vazia quer dizer "não criada", e então você pode repetir `Criar`. Achou alguma: confira `Status` e `DataSolicitacao`, porque uma migração anterior da mesma UC e mês (cancelada, por exemplo) também volta.
  4. Para uma edição, chame `ConferirEdicao(ctx, k)` com a `ChaveEdicao`; o segundo retorno diz se a migração já tem os campos desejados.
  5. Só repita a escrita se a conferência disser que ela não foi aplicada. Se você repetir `Criar` com a migração ativa, recebe [`ERR_MIGRACAO_SOLICITADA`](#migracao-solicitada), que indica que a primeira provavelmente valeu: confira de novo.
  6. Em `Do`, `Key` vem vazia: confira por conta própria.
- **Repetir:** não, até conferir. No Problem: `uncertain` verdadeiro e `retryable` falso.
- **Sentinela Go:** nenhuma; use `errors.As` com `*UncertainResultError`. A `Key` nunca vai ao Problem, porque pode conter a UC.
- **Confiança:** do SDK; a duplicata foi vista no sandbox.

<a id="resposta-ilegivel"></a>
## Resposta ilegível

- **Código da API:** nenhum.
- **Status:** 2xx (a API respondeu com sucesso); o Problem não repete o status, porque o erro é do corpo.
- **Quando acontece:** a conexão caiu no meio do corpo, o corpo não é JSON válido, ou um campo tem um valor que o SDK não entende (uma data ou um número fora do formato). A mensagem do erro não repete o valor recebido, que pode ser dado da unidade consumidora.
- **O que fazer:** numa leitura, o SDK já repetiu quando o corpo foi interrompido; se persiste, abra uma issue com o `requestId`. Numa **escrita**, a API pode ter aplicado a mudança: o erro vem como [resultado incerto](#resultado-incerto), com esta entrada na causa.
- **Repetir:** GET, sim. POST e PUT, não: confira antes.
- **Sentinela Go:** nenhuma.
- **Confiança:** do SDK.

<a id="cursor-repetido"></a>
## Cursor de paginação repetido

- **Código da API:** nenhum.
- **Status:** nenhum erro HTTP; a API respondeu 200.
- **Quando acontece:** a API devolveu como próxima página um cursor já usado. A paginação nunca foi vista funcionando no sandbox; sem a guarda, um cursor fixo giraria para sempre. O iterador `Listar` encerra com este erro: se a página aponta para o cursor que acabou de usar, os itens dela não são entregues (repetiriam a anterior); se aponta para um cursor visto antes, os itens dela são entregues e o iterador para sem buscar de novo.
- **O que fazer:** use `ListarPagina` para controlar o cursor e abra uma issue com o `request_id` da última página, que sai no log do SDK (`WithLogger`); o erro não o traz.
- **Repetir:** não.
- **Sentinela Go:** `ErrCursorRepetido`.
- **Confiança:** do SDK.

<a id="falha-ao-obter-token"></a>
## Falha ao obter o token

- **Código da API:** o do endpoint de token, se vier um.
- **Status:** o do endpoint de token (`operation` vale `obterToken`).
- **Quando acontece:** o pedido de token foi recusado ou falhou: `client_id` ou `client_secret` errados, ambiente trocado, endpoint fora do ar. Erros de rede ao buscar o token aparecem como [falha de rede](#erro-de-transporte) ou [tempo esgotado](#tempo-esgotado). Isso inclui uma resposta do endpoint sem `access_token` e um erro da fonte de `WithTokenSource`. O erro nunca traz o formulário nem o corpo.
- **O que fazer:** confira as credenciais e o ambiente (`Sandbox` ou `Production`).
- **Repetir:** o SDK não repete o pedido de token sozinho. Buscar token não tem efeito colateral, então 429 e 5xx são seguros de repetir (`retryable` verdadeiro); 4xx, não.
- **Sentinela Go:** nenhuma específica; `ErrCredencialInvalida` casa se o endpoint devolver esse código.
- **Confiança:** baixa.

<a id="argumento-invalido"></a>
## Argumento inválido

- **Código da API:** nenhum.
- **Status:** nenhum: o SDK recusou antes de enviar.
- **Quando acontece:** falta algo que a API exige (o filtro de medições sem UC, concessionária, início ou fim), o id está vazio, a função de edição é nula, o caminho de `Do` não começa com `/`, o corpo não vira JSON, ou a configuração do `NewClient` é inválida (credencial ou agente faltando, URL do ambiente, prazo, limites). A mensagem do erro não repete a UC nem o caminho.
- **O que fazer:** corrija a chamada. Nada foi enviado nem gasto do limite de taxa.
- **Repetir:** não, sem corrigir.
- **Sentinela Go:** `ErrArgumentoInvalido`.
- **Confiança:** do SDK.

<a id="fora-do-catalogo"></a>
## Erro fora do catálogo

- **Código da API:** nenhum.
- **Status:** nenhum.
- **Quando acontece:** `ProblemFrom` recebeu um erro que não vem do SDK, ou que o SDK não classificou. O Problem usa esta entrada (`type` termina em `#fora-do-catalogo`, nunca `about:blank`) e **não** copia o texto do erro. O `about:blank` da RFC 9457 fica para quem monta um Problem à mão: o JSON do SDK, nesse caso, usa como `title` a frase padrão do status.
- **O que fazer:** trate o erro original; ele continua em suas mãos.
- **Repetir:** depende; o Problem diz `retryable` falso por prudência.
- **Sentinela Go:** nenhuma.
- **Confiança:** do SDK.
