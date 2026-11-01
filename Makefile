# Versões fixadas. Confirmadas nas páginas de releases em 30/10/2026.
GO_MIN                := 1.26
OAPI_CODEGEN_VERSION  := v2.8.1
KIN_OPENAPI_VERSION   := v0.152.0
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION := v1.9.0
GITLEAKS_VERSION      := v8.30.1
ACTIONLINT_VERSION    := v1.7.12
YQ_VERSION            := v4.54.1

# Imagens por digest (a tag fica só como lembrete da versão).
VACUUM_IMAGE            := ghcr.io/daveshanley/vacuum@sha256:5d600e8df2a9c1b110fb5683c61c9e80e0c0574a37c118d3b1ff1868710050a5 # v0.36.0, amd64+arm64
OASDIFF_IMAGE           := docker.io/tufin/oasdiff@sha256:82b1840a77335562bb11ce95f77ad31745a0c6f3a770929333ed1c5ef466e400 # v1.35.0, amd64+arm64
PRISM_IMAGE             := docker.io/stoplight/prism@sha256:3f6d29e31bfe0b99587f0f6f79c423858dd6a2ea7e1e4658273ed930a78a9acf # `prism --version` 5.15.10 (a tag 5.16.0 não existe), só amd64
REDOCLY_IMAGE           := docker.io/redocly/cli@sha256:d39a229ebd8250fea62f44c377ddb04797e8a155140eda49db17f1caaa9334ff # 2.72.1, amd64+arm64
PANDOC_IMAGE            := docker.io/pandoc/core@sha256:c7127705b9d84c1b4acbbe411a86e3a6d67ac57870654dc378b6fe636ab747df # 3.8 (pandoc 3.8.3), amd64+arm64
OPENAPI_GENERATOR_IMAGE := docker.io/openapitools/openapi-generator-cli@sha256:a304ddf1e2e5f24f68fa3153568d6174cea4959d09aa8e3db6d526fd0782326d # v7.26.0, amd64+arm64

# info.version da spec: vira o rótulo da imagem do mock (ARG SPEC_VERSION).
SPEC_VERSION := $(shell sed -n 's/^  version:[[:space:]]*\([0-9][^[:space:]]*\).*/\1/p' openapi/abertura.yaml | head -1)
export SPEC_VERSION

# O go run perde o código de saída do programa (devolve sempre 1), então o
# gitleaks é instalado na versão fixada em out/bin e chamado direto.
GITLEAKS := $(CURDIR)/out/bin/gitleaks
export GITLEAKS

$(GITLEAKS): Makefile
	GOBIN=$(CURDIR)/out/bin go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

.PHONY: segredos spec-lint spec-exemplos testa-regras spec-diff docs mock mock-smoke mock-imagem go-tidy go-gen go-test go-race go-lint go-contrato go-vuln go-deps checa-workflows testa-quebra-spec quebra-spec testa-checa-tag checa-tag testa-checa-marca checa-marca hooks ci

segredos: $(GITLEAKS) ## prova a guarda de segredos e varre o repositório
	scripts/testa-segredos.sh
	$(GITLEAKS) dir --config .gitleaks.toml --no-banner --exit-code 42 .
	@if git rev-parse --verify -q HEAD >/dev/null; then \
		$(GITLEAKS) git --config .gitleaks.toml --no-banner --exit-code 42 .; \
	else echo "sem commits ainda: varredura do histórico pulada"; fi

spec-lint: ## lint da spec com as regras próprias (openapi/vacuum.yaml)
	docker run --rm -v "$(CURDIR)":/work -w /work $(VACUUM_IMAGE) lint -r openapi/vacuum.yaml -d -b -q --no-clip openapi/abertura.yaml

spec-exemplos: ## confere as regras de exemplo inventado que o vacuum não expressa
	YQ_VERSION=$(YQ_VERSION) scripts/confere-exemplos.sh

testa-regras: ## prova que cada regra própria do lint reprova a sua spec mínima
	VACUUM_IMAGE=$(VACUUM_IMAGE) scripts/testa-regras.sh

spec-diff: ## compara a spec com a última tag openapi/v* (oasdiff); sem tag, pula
	OASDIFF_IMAGE=$(OASDIFF_IMAGE) scripts/oasdiff.sh
	OASDIFF_IMAGE=$(OASDIFF_IMAGE) scripts/testa-oasdiff.sh

docs: ## gera a página Redoc (out/docs/index.html) e o catálogo de erros (out/docs/erros/index.html), não versionados
	mkdir -p out/docs
	docker run --rm -v "$(CURDIR)":/work -w /work $(REDOCLY_IMAGE) build-docs openapi/abertura.yaml -o out/docs/index.html --inlineBundle --template docs/template.hbs --config docs/redocly.yaml
	@grep -q "descrição não oficial" out/docs/index.html && echo "faixa 'descrição não oficial' presente em out/docs/index.html"
	PANDOC_IMAGE=$(PANDOC_IMAGE) scripts/gera-erros.sh
	scripts/testa-confere-erros.sh
	scripts/confere-erros.sh

mock: ## constrói e sobe o mock Prism em http://localhost:4010 (docker compose -f mock/compose.yaml down para parar)
	docker compose -f mock/compose.yaml up -d --build --wait --wait-timeout 180

mock-smoke: ## sobe o mock, chama as 10 operações e derruba o mock
	scripts/mock-smoke.sh

# Arquivos gerados por `go generate`. A deriva é medida só neles: mudança em
# qualquer outro arquivo de go/ (código à mão, testes, go.mod) não reprova.
GERADOS := go/internal/gen/gen.go go/internal/spec/abertura.yaml

go-tidy: ## falha se go.mod ou go.sum mudariam com `go mod tidy`
	cd go && go mod tidy -diff

go-gen: ## regenera os modelos e a cópia da spec e falha se os arquivos gerados mudarem (deriva)
	@set -e; mkdir -p out; for f in $(GERADOS); do echo "$$f $$(git hash-object $$f)"; done > out/.go-gen-antes; \
	(cd go && go generate ./...); \
	deriva=0; while read -r f h; do \
		if [ "$$(git hash-object $$f)" != "$$h" ]; then echo "go-gen: deriva em $$f (diferia do que o gerador produz)" >&2; deriva=1; fi; \
	done < out/.go-gen-antes; \
	[ "$$deriva" = 0 ] || exit 1; \
	git diff --exit-code -- $(GERADOS) || { echo "go-gen: arquivos gerados diferem do índice; faça git add" >&2; exit 1; }

go-test:
	cd go && go build ./... && go vet ./... && go test ./...

go-race: ## testes com -race e cobertura (mínimo de 80% no pacote raiz); perfil em out/go-cobertura.out
	scripts/go-cobertura.sh 80

go-lint:
	cd go && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

go-contrato: ## contrato do sandbox: vet, lint e os testes sem rede, com -tags sandbox (nunca fala com a API)
	cd go && go vet -tags sandbox ./contrato/...
	cd go && go test -tags sandbox ./contrato/... -run 'TestOrcamento|TestNomesDeCampos|TestDocumentoDoAmbiente|TestEscolha' -count=1
	cd go && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --build-tags sandbox ./...

go-vuln:
	cd go && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

CHECA_DEPS ?= scripts/checa-deps.sh
go-deps: ## confere as dependências do módulo Go (scripts/checa-deps.sh)
	$(CHECA_DEPS)

checa-workflows: ## actionlint, actions por SHA, imagens por digest, permissions e alvos do Makefile
	ACTIONLINT_VERSION=$(ACTIONLINT_VERSION) scripts/checa-workflows.sh

testa-quebra-spec: ## prova a leitura da seção Quebras do CHANGELOG (tabela de casos sintéticos)
	scripts/testa-quebra-spec.sh

quebra-spec: ## imprime 1 se a seção Quebras de [Não lançado] está preenchida, 0 se não
	@scripts/quebra-spec.sh

testa-checa-tag: ## prova as checagens de tag (tabela de casos, sem criar tag)
	scripts/testa-checa-tag.sh

# make checa-tag TIPO=openapi TAG=openapi/v0.1.0-rc.1
checa-tag: ## confere a tag de release contra info.version, version.Version e CHANGELOG
	scripts/checa-tag.sh "$(TIPO)" "$(TAG)"

MOCK_IMAGEM ?= abertura-mercado-energia-mock
MOCK_TAG    ?= local
mock-imagem: ## constrói a imagem do mock (linux/amd64, como o Prism fixado) com o nome $(MOCK_IMAGEM):$(MOCK_TAG)
	docker build --platform linux/amd64 --build-arg SPEC_VERSION=$(SPEC_VERSION) -f mock/Dockerfile -t $(MOCK_IMAGEM):$(MOCK_TAG) .

testa-checa-marca: ## prova a checagem da marca (tabela de casos em repositórios temporários)
	scripts/testa-checa-marca.sh

checa-marca: ## o nome do operador só no aviso de não afiliação (scripts/checa-marca.sh)
	scripts/checa-marca.sh

hooks: $(GITLEAKS) ## instala o gancho de pre-commit (funciona em worktree)
	@dir="$$(git rev-parse --git-path hooks)" && mkdir -p "$$dir" && \
		install -m 755 scripts/pre-commit "$$dir/pre-commit" && \
		echo "gancho instalado em $$dir/pre-commit"

ci: segredos testa-checa-marca checa-marca checa-workflows testa-checa-tag testa-quebra-spec testa-regras spec-lint spec-exemplos spec-diff docs mock mock-smoke go-tidy go-gen go-test go-race go-lint go-contrato go-vuln go-deps
