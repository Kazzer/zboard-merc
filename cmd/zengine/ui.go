package main

import (
	"embed"
	"io/fs"
)

//go:embed ui/index.html ui/app.js ui/style.css
var uiEmbed embed.FS

var uiFiles, _ = fs.Sub(uiEmbed, "ui")
