package theme

import (
	"embed"
)

//go:embed defaults/*.theme
var EmbeddedThemes embed.FS
