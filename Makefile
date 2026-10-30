# Versões fixadas. Confirmadas nas páginas de releases em 30/10/2026.
GO_MIN                := 1.25
OAPI_CODEGEN_VERSION  := v2.8.1
KIN_OPENAPI_VERSION   := v0.152.0
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.1.4
GITLEAKS_VERSION      := v8.30.1
ACTIONLINT_VERSION    := v1.7.12

# Imagens por digest (a tag fica só como lembrete da versão).
VACUUM_IMAGE            := ghcr.io/daveshanley/vacuum@sha256:5d600e8df2a9c1b110fb5683c61c9e80e0c0574a37c118d3b1ff1868710050a5 # v0.36.0, amd64+arm64
OASDIFF_IMAGE           := docker.io/tufin/oasdiff@sha256:82b1840a77335562bb11ce95f77ad31745a0c6f3a770929333ed1c5ef466e400 # v1.35.0, amd64+arm64
PRISM_IMAGE             := docker.io/stoplight/prism@sha256:3f6d29e31bfe0b99587f0f6f79c423858dd6a2ea7e1e4658273ed930a78a9acf # tag 5 (5.16.0 não tem tag), só amd64
REDOCLY_IMAGE           := docker.io/redocly/cli@sha256:d39a229ebd8250fea62f44c377ddb04797e8a155140eda49db17f1caaa9334ff # 2.72.1, amd64+arm64
OPENAPI_GENERATOR_IMAGE := docker.io/openapitools/openapi-generator-cli@sha256:a304ddf1e2e5f24f68fa3153568d6174cea4959d09aa8e3db6d526fd0782326d # v7.26.0, amd64+arm64

# O go run perde o código de saída do programa (devolve sempre 1), então o
# gitleaks é instalado na versão fixada em out/bin e chamado direto.
GITLEAKS := $(CURDIR)/out/bin/gitleaks
export GITLEAKS

$(GITLEAKS): Makefile
	GOBIN=$(CURDIR)/out/bin go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

.PHONY: segredos spec-lint spec-diff docs mock mock-smoke go-gen go-test go-lint go-vuln hooks ci

segredos: $(GITLEAKS) ## prova a guarda de segredos e varre o repositório
	scripts/testa-segredos.sh
	$(GITLEAKS) dir --config .gitleaks.toml --no-banner --exit-code 42 .
	@if git rev-parse --verify -q HEAD >/dev/null; then \
		$(GITLEAKS) git --config .gitleaks.toml --no-banner --exit-code 42 .; \
	else echo "sem commits ainda: varredura do histórico pulada"; fi

spec-lint: ## pendente: task 02
	@echo "pendente: task 02"

spec-diff: ## pendente: task 03
	@echo "pendente: task 03"

docs: ## pendente: task 03
	@echo "pendente: task 03"

mock: ## pendente: task 03
	@echo "pendente: task 03"

mock-smoke: ## pendente: task 03
	@echo "pendente: task 03"

go-gen: ## pendente: task 07
	@echo "pendente: task 07"

go-test:
	cd go && go build ./... && go vet ./... && go test ./...

go-lint:
	cd go && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

go-vuln:
	cd go && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

hooks: $(GITLEAKS) ## instala o gancho de pre-commit
	install -m 755 scripts/pre-commit .git/hooks/pre-commit
	@echo "gancho instalado em .git/hooks/pre-commit"

ci: segredos spec-lint spec-diff docs mock mock-smoke go-gen go-test go-lint go-vuln
