// Package gen contém os modelos gerados a partir da descrição OpenAPI.
//
// O arquivo gen.go é gerado e não deve ser editado à mão: ajuste o overlay
// (go/overlay.yaml) ou a configuração (config.yaml) e rode go generate.
// O pacote usa só a biblioteca padrão.
package gen

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.1 -config config.yaml ../spec/abertura.yaml
