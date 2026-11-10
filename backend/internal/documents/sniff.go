package documents

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MIME types the documents table accepts.
const (
	mimePDF  = "application/pdf"
	mimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimeXLS  = "application/vnd.ms-excel"
)

// detected is what a file turned out to be, judged by its content and never by its name or declared type.
type detected struct {
	Mime      string
	Format    string // FormatPDF or FormatExcel
	Extension string
}

var (
	oleHeader = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	zipHeader = []byte("PK\x03\x04")
	// "Workbook" and "Book" in UTF-16, the names of the stream that makes an OLE file an Excel workbook.
	workbookName = utf16le("Workbook")
	bookName     = utf16le("Book")
)

func utf16le(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, c := range s {
		out = append(out, byte(c), 0)
	}
	return out
}

// sniff reports what the data really is: a PDF, an .xlsx workbook or an .xls workbook. Anything else,
// including a renamed executable or a Word file, reports false.
func sniff(data []byte) (detected, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return detected{Mime: mimePDF, Format: FormatPDF, Extension: "pdf"}, true
	case bytes.HasPrefix(data, zipHeader) && isXLSX(data):
		return detected{Mime: mimeXLSX, Format: FormatExcel, Extension: "xlsx"}, true
	case bytes.HasPrefix(data, oleHeader) && (bytes.Contains(data, workbookName) || bytes.Contains(data, bookName)):
		return detected{Mime: mimeXLS, Format: FormatExcel, Extension: "xls"}, true
	}
	return detected{}, false
}

// isXLSX opens the zip and looks for the parts every Excel workbook has.
func isXLSX(data []byte) bool {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	var contentTypes, workbook bool
	for _, f := range reader.File {
		switch f.Name {
		case "[Content_Types].xml":
			contentTypes = true
		case "xl/workbook.xml":
			workbook = true
		}
	}
	return contentTypes && workbook
}

// cleanFileName keeps only the base name the browser sent, drops control characters and limits the length.
func cleanFileName(raw string) string {
	name := strings.ReplaceAll(raw, `\`, "/")
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	name = strings.TrimSpace(b.String())
	if name == "." || name == ".." || name == "/" || name == "" {
		return "documento"
	}
	for utf8.RuneCountInString(name) > 150 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}
