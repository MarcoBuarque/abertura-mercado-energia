// Package spec embeds a copy of the OpenAPI description for use in tests.
//
// The copy in abertura.yaml is generated from ../../../openapi/abertura.yaml
// by go generate and must not be edited by hand. No production code of the SDK
// imports this package.
package spec

import _ "embed"

//go:generate cp ../../../openapi/abertura.yaml abertura.yaml

// Abertura holds the YAML of the OpenAPI description.
//
//go:embed abertura.yaml
var Abertura []byte
