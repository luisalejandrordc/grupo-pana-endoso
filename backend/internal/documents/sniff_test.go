package documents

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipWith(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("<x/>")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSniffRecognizesPDFAndExcelByContent(t *testing.T) {
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF")
	if d, ok := sniff(pdf); !ok || d.Format != FormatPDF || d.Mime != mimePDF || d.Extension != "pdf" {
		t.Errorf("pdf: %+v %v", d, ok)
	}

	xlsx := zipWith(t, "[Content_Types].xml", "xl/workbook.xml", "xl/worksheets/sheet1.xml")
	if d, ok := sniff(xlsx); !ok || d.Format != FormatExcel || d.Mime != mimeXLSX || d.Extension != "xlsx" {
		t.Errorf("xlsx: %+v %v", d, ok)
	}

	xls := append(append([]byte{}, oleHeader...), make([]byte, 600)...)
	xls = append(xls, utf16le("Workbook")...)
	if d, ok := sniff(xls); !ok || d.Format != FormatExcel || d.Mime != mimeXLS || d.Extension != "xls" {
		t.Errorf("xls: %+v %v", d, ok)
	}
}

func TestSniffRefusesEverythingElse(t *testing.T) {
	word := append(append([]byte{}, oleHeader...), []byte(strings.Repeat("W\x00o\x00r\x00d\x00D\x00o\x00c\x00", 20))...)
	cases := map[string][]byte{
		"empty":                {},
		"a renamed executable": []byte("MZ\x90\x00\x03\x00\x00\x00 This program cannot be run in DOS mode"),
		"plain text":           []byte("%PDF is not at the start\n%PDF-1.4"),
		"a word document":      word,
		"a docx":               zipWith(t, "[Content_Types].xml", "word/document.xml"),
		"a zip of anything":    zipWith(t, "readme.txt"),
		"a broken zip":         append([]byte("PK\x03\x04"), []byte("not really a zip")...),
		"an image":             []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
	}
	for name, data := range cases {
		if d, ok := sniff(data); ok {
			t.Errorf("%s was accepted as %+v", name, d)
		}
	}
}

func TestCleanFileName(t *testing.T) {
	cases := map[string]string{
		"Cotización final.pdf":       "Cotización final.pdf",
		`C:\Users\ana\Desktop\a.pdf`: "a.pdf",
		"../../etc/passwd":           "passwd",
		"  espacios.pdf  ":           "espacios.pdf",
		"con\x00nulo\n.pdf":          "connulo.pdf",
		"":                           "documento",
		"..":                         "documento",
		"/":                          "documento",
		"   ":                        "documento",
		strings.Repeat("ñ", 400):     strings.Repeat("ñ", 150),
	}
	for in, want := range cases {
		if got := cleanFileName(in); got != want {
			t.Errorf("cleanFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
