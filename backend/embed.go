package main

import "embed"

//go:embed frontend/*
var frontendFS embed.FS
