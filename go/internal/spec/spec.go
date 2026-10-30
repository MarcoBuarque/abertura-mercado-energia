// Package spec embute uma cópia da descrição OpenAPI para uso em testes.
//
// A cópia em abertura.yaml é gerada a partir de ../../../openapi/abertura.yaml
// por go generate e não deve ser editada à mão. Nenhum código de produção do
// SDK importa este pacote.
package spec

import _ "embed"

//go:generate cp ../../../openapi/abertura.yaml abertura.yaml

// Abertura contém o YAML da descrição OpenAPI.
//
//go:embed abertura.yaml
var Abertura []byte
