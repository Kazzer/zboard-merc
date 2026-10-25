package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

var String = strings.TrimSpace(raw)
