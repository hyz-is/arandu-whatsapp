package main

import (
	"fmt"
	"os"
	"path/filepath"

	webhookdocs "github.com/hyz-is/arandu-whatsapp/internal/webhook/docs"
)

func main() {
	doc := webhookdocs.Build()
	if err := webhookdocs.ValidateDocument(doc); err != nil {
		exit(err)
	}
	markdown, err := webhookdocs.Markdown(doc)
	if err != nil {
		exit(err)
	}
	if err := os.MkdirAll("docs", 0o755); err != nil {
		exit(err)
	}
	if err := os.WriteFile(filepath.Join("docs", "webhooks.md"), []byte(markdown), 0o644); err != nil {
		exit(err)
	}
}

func exit(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "webhook-docs: %v\n", err)
	os.Exit(1)
}
