package docs

import "embed"

//go:embed openapi.yaml swagger.html
var Content embed.FS
