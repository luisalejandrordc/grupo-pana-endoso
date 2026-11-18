// Command email-preview writes every email template, rendered with sample data, to a folder of HTML files so
// the designs can be opened in a browser. Nothing is sent and no database or configuration is needed.
//
//	go run ./cmd/email-preview -out tmp/email-previews
package main

import (
	"endosoflow/internal/notifications"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	out := flag.String("out", "tmp/email-previews", "folder to write the previews to")
	appURL := flag.String("base-url", "http://localhost:5173", "frontend address used in the sample links")
	portalURL := flag.String("portal-url", "http://localhost:5174", "frontend address used in the sample links")
	logo := flag.String("logo-url", "", "address of the header logo; by default the frontend's logo file is copied next to the previews")
	flag.Parse()
	if err := run(*out, *appURL, *portalURL, *logo); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// frontendLogo is the logo file the frontend serves, relative to the backend folder.
const frontendLogo = "../frontend/public/endosoflow-logo-landscape.png"

func run(dir, appURL, portalURL, logo string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if logo == "" {
		// Show the real logo offline: copy it beside the previews and point at it with a relative address.
		if data, err := os.ReadFile(frontendLogo); err == nil {
			if err := os.WriteFile(filepath.Join(dir, filepath.Base(frontendLogo)), data, 0o644); err != nil {
				return err
			}
			logo = filepath.Base(frontendLogo)
		}
	}
	var index strings.Builder
	index.WriteString(`<!DOCTYPE html><meta charset="utf-8"><title>Correos de EndosoFlow</title>` +
		`<body style="font-family:system-ui,sans-serif;max-width:640px;margin:40px auto;color:#202020"><h1>Correos de EndosoFlow</h1><ul>`)
	for _, p := range notifications.Previews(appURL, portalURL, logo) {
		if err := os.WriteFile(filepath.Join(dir, p.Name+".html"), []byte(p.HTML), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, p.Name+".txt"), []byte(p.Subject+"\n\n"+p.Text), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(&index, `<li style="margin:8px 0"><a href="%s.html">%s</a> <small style="color:#777">(%s) &middot; <a href="%s.txt">texto</a></small></li>`,
			p.Name, html.EscapeString(p.Subject), p.Name, p.Name)
	}
	index.WriteString(`</ul></body>`)
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(index.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("Previews written to", filepath.Join(dir, "index.html"))
	return nil
}
