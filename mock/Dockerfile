# Local mock of the spec, with Prism. Build context: the repository root.
#
# The image is pinned by digest. Prism tag 5.16.0 does not exist on Docker Hub;
# the digest below reports `prism --version` 5.15.10 and only has an amd64
# build (on a Mac with Apple Silicon it runs under emulation; compose.yaml pins
# the platform). The same digest is in PRISM_IMAGE in the Makefile;
# scripts/mock-smoke.sh checks both.
ARG PRISM_IMAGE=docker.io/stoplight/prism@sha256:3f6d29e31bfe0b99587f0f6f79c423858dd6a2ea7e1e4658273ed930a78a9acf
FROM ${PRISM_IMAGE}

# Spec version (info.version). The Makefile and compose fill it from
# openapi/abertura.yaml; scripts/mock-smoke.sh checks the image label.
ARG SPEC_VERSION=unknown

LABEL org.opencontainers.image.title="Mock of the Market Opening API (unofficial description)" \
      org.opencontainers.image.description="Prism serving the spec's invented examples. Independent project, not affiliated with the operator." \
      org.opencontainers.image.source="https://github.com/MarcoBuarque/abertura-mercado-energia" \
      org.opencontainers.image.licenses="Apache-2.0" \
      io.github.abertura.spec-version="${SPEC_VERSION}" \
      io.github.abertura.prism-version="5.15.10"

COPY openapi/abertura.yaml /spec/abertura.yaml

EXPOSE 4010
CMD ["mock", "--host", "0.0.0.0", "--port", "4010", "--multiprocess=false", "--errors", "/spec/abertura.yaml"]
