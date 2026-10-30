# Versões fixadas. Confirmadas nas páginas de releases em 30/10/2026.
GO_MIN                := 1.25
OAPI_CODEGEN_VERSION  := v2.8.1
KIN_OPENAPI_VERSION   := v0.152.0
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.1.4
GITLEAKS_VERSION      := v8.30.1
ACTIONLINT_VERSION    := v1.7.12
YQ_VERSION            := v4.54.1

# Imagens por digest (a tag fica só como lembrete da versão).
VACUUM_IMAGE            := ghcr.io/daveshanley/vacuum@sha256:5d600e8df2a9c1b110fb5683c61c9e80e0c0574a37c118d3b1ff1868710050a5 # v0.36.0, amd64+arm64
OASDIFF_IMAGE           := docker.io/tufin/oasdiff@sha256:82b1840a77335562bb11ce95f77ad31745a0c6f3a770929333ed1c5ef466e400 # v1.35.0, amd64+arm64
PRISM_IMAGE             := docker.io/stoplight/prism@sha256:3f6d29e31bfe0b99587f0f6f79c423858dd6a2ea7e1e4658273ed930a78a9acf # `prism --version` 5.15.10 (a tag 5.16.0 não existe), só amd64
REDOCLY_IMAGE           := docker.io/redocly/cli@sha256:d39a229ebd8250fea62f44c377ddb04797e8a155140eda49db17f1caaa9334ff # 2.72.1, amd64+arm64
OPENAPI_GENERATOR_IMAGE := docker.io/openapitools/openapi-generator-cli@sha256:a304ddf1e2e5f24f68fa3153568d6174cea4959d09aa8e3db6d526fd0782326d # v7.26.0, amd64+arm64

# O go run perde o código de saída do programa (devolve sempre 1), então o
# gitleaks é instalado na versão fixada em out/bin e chamado direto.
GITLEAKS := $(CURDIR)/out/bin/gitleaks
export GITLEAKS

$(GITLEAKS): Makefile
	GOBIN=$(CURDIR)/out/bin go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

.PHONY: segredos spec-lint spec-exemplos testa-regras spec-diff docs mock mock-smoke go-gen go-test go-lint go-vuln hooks ci

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

docs: ## gera a página Redoc em out/docs/index.html (não versionada)
	mkdir -p out/docs
	docker run --rm -v "$(CURDIR)":/work -w /work $(REDOCLY_IMAGE) build-docs openapi/abertura.yaml -o out/docs/index.html --inlineBundle --template docs/template.hbs --config docs/redocly.yaml
	@grep -q "descrição não oficial" out/docs/index.html && echo "faixa 'descrição não oficial' presente em out/docs/index.html"

mock: ## constrói e sobe o mock Prism em http://localhost:4010 (docker compose -f mock/compose.yaml down para parar)
	docker compose -f mock/compose.yaml up -d --build --wait --wait-timeout 180

mock-smoke: ## sobe o mock, chama as 10 operações e derruba o mock
	scripts/mock-smoke.sh

go-gen: ## regenera os modelos e a cópia da spec e falha se algo mudar (deriva)
	cd go && go generate ./...
	git diff --exit-code -- go

go-test:
	cd go && go build ./... && go vet ./... && go test ./...

go-lint:
	cd go && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

go-vuln:
	cd go && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

hooks: $(GITLEAKS) ## instala o gancho de pre-commit (funciona em worktree)
	@dir="$$(git rev-parse --git-path hooks)" && mkdir -p "$$dir" && \
		install -m 755 scripts/pre-commit "$$dir/pre-commit" && \
		echo "gancho instalado em $$dir/pre-commit"

ci: segredos testa-regras spec-lint spec-exemplos spec-diff docs mock mock-smoke go-gen go-test go-lint go-vuln
