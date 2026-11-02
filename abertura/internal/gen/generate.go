// Package gen contains the models generated from the OpenAPI description.
//
// The gen.go file is generated and must not be edited by hand: adjust the
// overlay (overlay.yaml in the module root) or the configuration (config.yaml)
// and run go generate. The package uses only the standard library.
package gen

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.1 -config config.yaml ../spec/abertura.yaml
