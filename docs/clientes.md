# Guia de clientes

Este guia mostra como subir o mock da spec, como gerar um cliente em Python,
TypeScript ou Java a partir de `openapi/abertura.yaml`, o que esses clientes
gerados **não** fazem e quais regras de repetição (retry) quem escreve um
cliente próprio deve seguir.

> **Descrição não oficial.** Este projeto é independente: não é afiliado,
> patrocinado nem endossado pela CCEE. A spec descreve o que a API devolveu em
> testes no sandbox e não substitui a documentação do operador. Tudo o que está
> aqui roda contra o mock local, com dados inventados.

Para ler a spec no navegador, rode `make docs` e abra `out/docs/index.html`.

## O que você precisa

- Docker (o mock e o gerador rodam em contêiner; nada é instalado na máquina).
- Para o cliente Python: Python 3.10 ou mais novo.
- Um terminal na raiz do repositório.

## 1. Subir o mock

```bash
docker compose -f mock/compose.yaml up --build
```

O mock é o [Prism](https://stoplight.io/open-source/prism) servindo os exemplos
da spec em `http://localhost:4010`. Para parar:
`docker compose -f mock/compose.yaml down`. Sem Compose na linha de comando,
`make mock` sobe em segundo plano e espera o mock ficar saudável.

O Prism fixado só existe para amd64. Num Mac com Apple Silicon ele roda por
emulação (`platform: linux/amd64` no `compose.yaml`) e a subida leva alguns
segundos a mais; num computador amd64 roda nativo.

### O que o mock responde

Cada operação devolve o **primeiro exemplo** da resposta 200. Para escolher
outra resposta ou outro exemplo, mande o cabeçalho `Prefer`:

| Você quer | Cabeçalho |
|---|---|
| a segunda página de migrações (sem cursor) | `Prefer: example=pagina-2` |
| um 403 `ERR_CREDENCIAL_INVALIDA` | `Prefer: code=403` |
| um 400 `ERR_MIGRACAO_SOLICITADA` ao criar migração | `Prefer: code=400, example=migracao-solicitada` |

Todo exemplo é inventado: UC `000000000000001`, códigos na faixa `900000`,
e-mails em `example.org`. O nome de cada exemplo começa com "Inventado:" na spec.

### Onde o mock difere da API real

O mock valida a requisição contra a spec, e isso é útil (pega erro de cliente
antes de tocar o sandbox), mas o formato do erro **não é o da API real**:

- **Requisição inválida** (falta `codigoAgente`, enum errado, corpo incompleto):
  o Prism responde `400` e devolve como corpo o *primeiro exemplo de 400 da
  operação*. Esse corpo é genérico e **não descreve o problema**: na listagem
  de migrações, por exemplo, ele sempre diz `ERR_PERFIL_VAREJISTA_OBRIGATORIO`,
  mesmo que o campo que falta seja outro. O motivo real vem no cabeçalho de
  resposta `sl-violations`, uma lista em JSON:

  ```text
  HTTP/1.1 400 Bad Request
  sl-violations: [{"location":["request","header"],"severity":"Error","code":"required","message":"Request header must have required property 'codigoagente'"}]
  Content-type: application/json

  {"timestamp":"2027-01-10T10:00:00.000-03:00","status":400,"error":"ERR_PERFIL_VAREJISTA_OBRIGATORIO","message":"Inventado: informe o perfil do varejista"}
  ```

  Os nomes de cabeçalho e de campo aparecem em minúsculas nessas mensagens
  (`codigoagente`). Se o seu cliente "recebeu um 400 que não faz sentido",
  olhe `sl-violations` primeiro.

- **Sem token**: o Prism responde `401` com `Content-Type: application/problem+json`
  e `WWW-Authenticate: OAuth2`. A API real devolve `403` com
  `ERR_CREDENCIAL_INVALIDA` (nunca 401). Qualquer valor de `Authorization:
  Bearer ...` serve para o mock, mas o cabeçalho precisa existir.

- O mock não limita taxa, não expira token e não guarda estado: criar a mesma
  migração duas vezes devolve 200 duas vezes. Para ver o 400 de duplicata, peça
  o exemplo com `Prefer`.

## 2. Gerar um cliente

O gerador é o [OpenAPI Generator](https://openapi-generator.tech) 7.26.0, em
contêiner e fixado por digest. Gere **sempre fora do repositório** (os clientes
gerados não são versionados). Do diretório raiz:

```bash
GERADOR=docker.io/openapitools/openapi-generator-cli@sha256:a304ddf1e2e5f24f68fa3153568d6174cea4959d09aa8e3db6d526fd0782326d  # v7.26.0
SAIDA="$(mktemp -d)"
gera() { # gerador, pasta de saída, propriedades
  docker run --rm -u "$(id -u):$(id -g)" \
    -v "$PWD/openapi":/spec:ro -v "$SAIDA":/out "$GERADOR" \
    generate -i /spec/abertura.yaml -g "$1" -o "/out/$2" --additional-properties="$3"
}
```

**Python:**

```bash
gera python python packageName=abertura_cliente,projectName=abertura-cliente
```

**TypeScript** (`fetch`, sem dependências em tempo de execução):

```bash
gera typescript-fetch ts npmName=abertura-cliente,supportsES6=true
```

**Java** (cliente nativo do JDK 11+, sem dependências além do Jackson):

```bash
gera java java library=native,groupId=example.abertura,artifactId=abertura-cliente,invokerPackage=example.abertura.cliente,apiPackage=example.abertura.cliente.api,modelPackage=example.abertura.cliente.modelo
```

Em 30/10/2026 os três geraram sem erro, com 2 avisos do gerador
("Failed to get the schema name" e "Ignoring complex example on request body")
que não afetam o código. Só o cliente Python foi instalado e executado; o
TypeScript e o Java foram apenas gerados, **não compilados**.

Dois detalhes dos clientes gerados:

- As tags da spec não têm acento (`Autenticacao`, `Migracoes`, `Medicoes`,
  `Solicitacoes`); o nome acentuado vai em `x-displayName`, que a página usa no
  menu. Por isso as classes saem como `AutenticacaoApi`, `MigracoesApi`,
  `MedicoesApi` e `SolicitacoesApi`. Os nomes dos métodos vêm do `operationId`
  (`listar_migracoes` em Python, `listarMigracoes` em TypeScript e Java).
- Como as datas de resposta ficam sem `format` na spec, elas chegam como `str`:
  convertê-las é com você (ver "O que os clientes gerados não fazem").

## 3. Python: do token à lista de migrações

Com o mock no ar e o cliente gerado em `$SAIDA/python`:

```bash
python3 -m venv "$SAIDA/venv"
"$SAIDA/venv/bin/pip" install "$SAIDA/python"
```

Salve como `lista.py` e rode com `"$SAIDA/venv/bin/python" lista.py`:

```python
import abertura_cliente as ac
from abertura_cliente.api import AutenticacaoApi, MigracoesApi

HOST = "http://localhost:4010"  # o mock; no sandbox, o host está em `servers` da spec

# 1. Token. O mock aceita qualquer par; as credenciais abaixo são inventadas.
# Com credenciais de verdade, leia-as do ambiente: nunca as escreva no código.
ID = "id-inventado"
SEGREDO = "segredo-inventado"
with ac.ApiClient(ac.Configuration(host=HOST)) as cliente:
    token = AutenticacaoApi(cliente).obter_token(client_id=ID, client_secret=SEGREDO)
print("token expira em", token.expires_in, "s")

# 2. Uma página de migrações. O perfil é exigido no cabeçalho E na consulta.
cfg = ac.Configuration(host=HOST, access_token=token.access_token)
with ac.ApiClient(cfg) as cliente:
    pagina = MigracoesApi(cliente).listar_migracoes(
        codigo_agente=900002,
        codigo_perfil=900003,
        codigo_perfil_varejista=900003,
    )
for m in pagina.migracao:
    print(m.id_migracao, m.codigo_unidade_consumidora, m.status_migracao)
print("próxima página:", pagina.index_proxima_pagina)
```

Saída esperada:

```text
token expira em 300 s
00000000-0000-4000-8000-000000000001 000000000000001 CRIADA
próxima página: cursor-inventado-2
```

Para ver a última página e um erro, passe o `Prefer` por `_headers`:

```python
from abertura_cliente.exceptions import ApiException

api = MigracoesApi(cliente)
ultima = api.listar_migracoes(900002, 900003, 900003, _headers={"Prefer": "example=pagina-2"})
print(ultima.index_proxima_pagina)  # None

try:
    api.listar_migracoes(900002, 900003, 900003, _headers={"Prefer": "code=403"})
except ApiException as erro:
    print(erro.status, erro.body)  # 403 {"...","error":"ERR_CREDENCIAL_INVALIDA",...}
```

Medido em 30/10/2026 num Mac com Apple Silicon, com as imagens já baixadas
(mock subindo por emulação): 6 s para construir e subir o mock, 1 s para gerar
o cliente, 2 s para o `venv` e o `pip`, 10 s no total até a lista aparecer. Na
primeira vez, some o tempo de baixar as imagens do Prism e do gerador.

## 4. O que os clientes gerados não fazem

Um cliente gerado é só a camada de HTTP e de tipos. Faltam, e ficam por sua
conta:

1. **Renovar o token.** O token vale cerca de 300 segundos e não há token de
   renovação (`refresh_expires_in` é 0). Peça outro antes de vencer; uma margem
   de 30 segundos funciona. O cliente gerado guarda o `access_token` que você
   entregar e o usa até a API recusar.
2. **Seguir o cursor.** A listagem devolve `indexProximaPagina`; enquanto vier
   preenchido, repita a chamada com ele no parâmetro `indexProximaPagina`. O
   cliente gerado lê uma página por chamada. Proteja-se contra cursor repetido
   (devolver o mesmo cursor duas vezes é laço infinito). Nenhuma resposta
   observada no sandbox trouxe cursor preenchido, então a regra vem da
   estrutura do envelope e tem confiança baixa.
3. **Tratar escrita de resultado incerto.** Num `POST` ou `PUT`, se a conexão
   cai depois de a requisição ser enviada, ou o tempo esgota, ou o corpo da
   resposta vem ilegível, você **não sabe** se a escrita aconteceu. Repetir às
   cegas pode duplicar. Confira antes: liste as migrações da UC, ou repita e
   trate `400 ERR_MIGRACAO_SOLICITADA` como sinal de que a primeira tentativa
   valeu (o sandbox recusa a migração duplicada). O cliente gerado só lança
   exceção; ele não distingue "não saiu" de "saiu e não sei".
4. **Aceitar os três formatos de mês.** O mês de referência aparece como
   `AAAA-MM` (filtros e corpos de envio), como data e hora com deslocamento
   `-03:00` sempre no dia 1 (listagem) e, no detalhe, em `dataReferencia`, cujo
   formato exato não foi guardado. A spec deixa as datas de resposta como texto
   para o cliente gerado não quebrar; interpretar cada formato é com você.
5. **Códigos como número ou como texto.** Códigos de agente, perfil e
   concessionária são inteiros no sandbox e texto na documentação pública; os
   campos `consumo` e `codigoAgenteConcessionaria` de uma medição vêm sem tipo
   fixo na spec. Aceite as duas formas.
6. **Limitar a taxa e repetir com espera.** Veja a próxima seção.

## 5. Regras de repetição para quem escreve o cliente

Estas regras são as do SDK Go do projeto. Servem de ponto de partida para um
cliente próprio, em qualquer linguagem. As partes sobre 429 e 5xx são
**hipótese**: nenhuma dessas respostas foi vista no sandbox. O 403 e a
duplicata são fato observado.

### Quando repetir

- **Falha antes de escrever** (DNS, conexão recusada, erro de TLS, nada saiu):
  repita, em qualquer método. Nada chegou ao servidor.
- **GET**: repita em erro de transporte, tempo esgotado, 429 e qualquer 5xx
  menos o 501. A leitura é segura de repetir.
- **POST e PUT** depois de o corpo ter saído: erro de transporte, tempo
  esgotado e qualquer 5xx menos o 501 são **resultado incerto** (seção 4, item 3).
  Não repita sozinho.
- **429 em POST e PUT**: repita só se a resposta traz o cabeçalho
  `x-amzn-ErrorType` (a recusa veio do gateway, antes do backend). Sem ele,
  trate como limite de taxa sem repetir.
- **2xx**: sucesso. Em POST e PUT, corpo ilegível numa resposta 2xx é incerto.
- **400, 404 e outros 4xx**: não repita. Corrija a requisição.
- **Cancelamento ou prazo do chamador**: pare e devolva o erro do contexto. Num
  POST ou PUT que já escreveu, é incerto.

### O 403 `ERR_CREDENCIAL_INVALIDA`

A API real devolve **403, não 401**, para token inválido ou vencido e também
para agente ou perfil que não pertencem à credencial. Só a `message` diferencia
os casos. Renove o token e repita **uma vez**, em qualquer método, quando valem
as duas condições:

1. a resposta veio do autorizador do gateway (traz `x-amzn-ErrorType`): o
   backend não chegou a processar nada, nem num POST;
2. o token rejeitado **já tinha sido aceito** antes por uma resposta sem essa
   marca. Um token que funcionava e parou venceu ou foi revogado.

Sem a condição 2, o erro é de agente, perfil ou credencial: repetir só gasta
requisição. Sem a condição 1, o 403 é do backend e não se repete. A repetição é
única por chamada.

### Quantas vezes, e esperando quanto

- No máximo **2 repetições** por chamada.
- Espera exponencial com *jitter* cheio (sorteio entre zero e o teto): base de
  500 ms e teto de 8 s. Em 429, base de 2 s e teto de 30 s.
- Se a resposta trouxer `Retry-After`, ele manda, limitado a 30 s.
- Toda espera tem de respeitar o prazo do chamador.
- **Nunca envie `Idempotency-Key`**: a API não o conhece, e alguns clientes
  HTTP reenviam POST sozinhos quando ele está presente.

### Prazos e limites

- 30 s por tentativa; 10 s para o token. O gateway corta perto de 30 s.
- A API responde `Connection: close`: cada requisição abre uma conexão TLS nova.
  Conte com essa latência.
- Limite de taxa a respeitar do seu lado: 900 requisições a cada 5 minutos no
  geral (1.000 por IP, no máximo) e 9.000 a cada 5 minutos nas medições. É por
  processo; vários processos somam.
- Hipótese não verificada: a janela de 5 minutos por IP lembra a regra de taxa
  de um firewall de aplicação, que bloqueia com 403. Um 403 sem código `ERR_*`
  **não** se repete.

## 6. Problem Details em um cliente Python

A API devolve erros com quatro campos (`timestamp`, `status`, `error`,
`message`) e a spec descreve exatamente isso. O SDK Go entrega o mesmo erro
como Problem Details da RFC 9457; num cliente Python, a conversão cabe numa
função, apoiada no [catálogo de erros](erros.md). Salve como `problema.py`,
suba o mock (`make mock`) e rode `python3 -I problema.py`:

```python
import json
import re
import urllib.error
import urllib.request

CATALOGO = "https://marcobuarque.github.io/abertura-mercado-energia/erros"

# Slug (âncora de docs/erros.md) e título por código da API. Código fora da
# tabela cai em "codigo-desconhecido"; sem código, o status decide, como no SDK.
CODIGOS = {
    "ERR_CREDENCIAL_INVALIDA": ("credencial-invalida-autorizador", "Credencial recusada pelo autorizador"),
    "ERR_VALIDACAO_CAMPOS": ("validacao-campos", "Campo inválido na requisição"),
    "ERR_PERFIL_VAREJISTA_OBRIGATORIO": ("perfil-varejista-obrigatorio", "Perfil de varejista obrigatório"),
    "ERR_MIGRACAO_SOLICITADA": ("migracao-solicitada", "Migração já solicitada"),
}
SEM_CODIGO = {
    404: ("nao-encontrado", "Recurso não encontrado"),
    429: ("limite-de-requisicoes", "Limite de requisições excedido"),
    502: ("falha-do-gateway", "Falha no gateway da API"),
    504: ("falha-do-gateway", "Falha no gateway da API"),
}


def para_problema(status, corpo, cabecalhos, operacao):
    """Converte o erro de quatro campos da API em Problem Details (RFC 9457)."""
    try:
        codigo = json.loads(corpo).get("error", "")
    except (ValueError, AttributeError):
        codigo = ""
    if not re.fullmatch(r"ERR_[A-Z0-9_]{1,80}", str(codigo)):
        codigo = ""  # texto livre do servidor não vai para o Problem
    if codigo in CODIGOS:
        slug, titulo = CODIGOS[codigo]
        if codigo == "ERR_CREDENCIAL_INVALIDA" and not cabecalhos.get("x-amzn-ErrorType"):
            slug, titulo = "credencial-invalida-backend", "Credencial recusada pelo backend"
    elif codigo:
        slug, titulo = "codigo-desconhecido", "Código de erro desconhecido do SDK"
    elif status in SEM_CODIGO:
        slug, titulo = SEM_CODIGO[status]
    elif status >= 500:
        slug, titulo = "erro-interno", "Erro interno da API"
    else:
        slug, titulo = "recusa-sem-codigo", "Recusa sem código da API"
    problema = {
        "type": f"{CATALOGO}#{slug}",
        "title": titulo,
        "status": status,
        # Nunca copie a "message" da API: ela pode citar UC, documento ou e-mail.
        "detail": f"Operação {operacao}: veja o catálogo.",
        "operation": operacao,
    }
    if codigo:
        problema["code"] = codigo
    return problema


pedido = urllib.request.Request(
    "http://localhost:4010/v1.1/varejista/migracoes",
    data=b"{}",
    method="POST",
    headers={
        "Authorization": "Bearer token-inventado",
        "Content-Type": "application/json",
        "codigoAgente": "900002",
        "codigoPerfil": "900003",
        "Prefer": "code=400, example=migracao-solicitada",
    },
)
try:
    urllib.request.urlopen(pedido)
except urllib.error.HTTPError as erro:
    problema = para_problema(erro.code, erro.read(), erro.headers, "criarMigracao")
    print(json.dumps(problema, ensure_ascii=False, indent=2))
```

Saída esperada (rodada contra o mock em 30/10/2026):

```json
{
  "type": "https://marcobuarque.github.io/abertura-mercado-energia/erros#migracao-solicitada",
  "title": "Migração já solicitada",
  "status": 400,
  "detail": "Operação criarMigracao: veja o catálogo.",
  "operation": "criarMigracao",
  "code": "ERR_MIGRACAO_SOLICITADA"
}
```

Três cuidados, os mesmos do SDK: o `detail` é texto seu e **não** copia a
`message` da API (ela pode citar UC, documento ou e-mail); o `code` só entra se
tiver a forma `ERR_` e maiúsculas; e o 403 `ERR_CREDENCIAL_INVALIDA` tem duas
entradas no catálogo, conforme venha ou não com o cabeçalho `x-amzn-ErrorType`.
Para devolver o resultado a quem chama o seu serviço, use o tipo de mídia
`application/problem+json`.
