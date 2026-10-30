# Mock local da spec, com o Prism. Contexto de build: a raiz do repositório.
#
# A imagem é fixada por digest. A tag 5.16.0 do Prism não existe no Docker Hub;
# o digest abaixo reporta `prism --version` 5.15.10 e só tem build amd64 (no Mac
# com Apple Silicon roda por emulação; o compose.yaml fixa a plataforma).
# O mesmo digest está em PRISM_IMAGE no Makefile; scripts/mock-smoke.sh confere.
ARG PRISM_IMAGE=docker.io/stoplight/prism@sha256:3f6d29e31bfe0b99587f0f6f79c423858dd6a2ea7e1e4658273ed930a78a9acf
FROM ${PRISM_IMAGE}

LABEL org.opencontainers.image.title="mock da API de Abertura de Mercado (descrição não oficial)" \
      org.opencontainers.image.description="Prism servindo os exemplos inventados da spec. Projeto independente, sem afiliação ao operador." \
      org.opencontainers.image.licenses="Apache-2.0" \
      io.github.abertura.spec-version="0.1.0" \
      io.github.abertura.prism-version="5.15.10"

COPY openapi/abertura.yaml /spec/abertura.yaml

EXPOSE 4010
CMD ["mock", "--host", "0.0.0.0", "--port", "4010", "--multiprocess=false", "--errors", "/spec/abertura.yaml"]
