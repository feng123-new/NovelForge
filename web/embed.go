package web

import "embed"

// Assets contains the compiled NovelForge web workspace.
//
//go:embed dist/*
var Assets embed.FS
