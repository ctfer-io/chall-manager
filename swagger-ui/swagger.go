// Just to enable embed go module to work :)

package swagger

import (
	"embed"
)

//go:embed *
var Content embed.FS
