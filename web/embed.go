// Package web embeds the static operations console so the API can serve it same-origin.
package web

import "embed"

//go:embed index.html styles.css app.js
var Assets embed.FS
