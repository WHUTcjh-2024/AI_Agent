package admin

import "embed"

//go:embed web/*.html web/*.css web/*.js
var Assets embed.FS
