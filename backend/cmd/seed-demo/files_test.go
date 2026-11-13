package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestSimplePDFIsWellFormed(t *testing.T) {
	data := simplePDF("Cotización (final)", []string{"Cliente: Embajada de Japón", "Monto: 35 000,00 \\ USD"})
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) || !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Fatal("not a PDF")
	}
	// Every cross-reference offset must point at its object.
	text := string(data)
	start, _ := strconv.Atoi(regexp.MustCompile(`startxref\n(\d+)`).FindStringSubmatch(text)[1])
	if !strings.HasPrefix(text[start:], "xref\n0 6\n") {
		t.Fatalf("startxref does not point at the table: %q", text[start:start+12])
	}
	entries := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllStringSubmatch(text, -1)
	if len(entries) != 5 {
		t.Fatalf("entries = %d", len(entries))
	}
	for i, e := range entries {
		offset, _ := strconv.Atoi(e[1])
		if !strings.HasPrefix(text[offset:], fmt.Sprintf("%d 0 obj", i+1)) {
			t.Errorf("object %d is not at offset %d", i+1, offset)
		}
	}
	// The stream length is the real length.
	m := regexp.MustCompile(`/Length (\d+) >>\nstream\n`).FindStringSubmatch(text)
	length, _ := strconv.Atoi(m[1])
	body := text[strings.Index(text, "stream\n")+7:]
	if end := strings.Index(body, "endstream"); end != length {
		t.Errorf("stream length %d, declared %d", end, length)
	}
	if !strings.Contains(text, `\(final\)`) {
		t.Error("parentheses in the text must be escaped")
	}
}

func TestSimpleXLSXHasTheParts(t *testing.T) {
	data := simpleXLSX("Cotización", [][]any{{"Concepto", "Monto"}, {"Unidad", 30000.5}, {"Total", 35000}})
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range r.File {
		found[f.Name] = true
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		_ = rc.Close()
		var root struct{ XMLName xml.Name }
		if err := xml.Unmarshal(content, &root); err != nil {
			t.Errorf("%s is not well-formed XML: %v", f.Name, err)
		}
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml"} {
		if !found[name] {
			t.Errorf("missing %s", name)
		}
	}
}

func TestCarPNGDecodes(t *testing.T) {
	img, format, err := image.Decode(bytes.NewReader(carPNG(bodyColor("Blanco"))))
	if err != nil || format != "png" {
		t.Fatalf("decode: %v %s", err, format)
	}
	if b := img.Bounds(); b.Dx() != 960 || b.Dy() != 540 {
		t.Errorf("size = %v", b)
	}
}

func TestEveryDemoColorHasAPaint(t *testing.T) {
	if bodyColor("Negro") == bodyColor("Blanco") || bodyColor("Gris Plata") == bodyColor("Rojo") {
		t.Error("different colors must be painted differently")
	}
}
