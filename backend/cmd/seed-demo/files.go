package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sort"
	"strings"
)

// latin1 turns text into the single-byte encoding the PDF font uses; anything outside it becomes "?".
func latin1(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 256 {
			out = append(out, byte(r))
		} else {
			out = append(out, '?')
		}
	}
	return out
}

func pdfEscape(b []byte) []byte {
	var out bytes.Buffer
	for _, c := range b {
		if c == '\\' || c == '(' || c == ')' {
			out.WriteByte('\\')
		}
		out.WriteByte(c)
	}
	return out.Bytes()
}

// simplePDF builds a valid one-page PDF with a title and lines of text, so a downloaded demo document opens in any viewer.
func simplePDF(title string, lines []string) []byte {
	var content bytes.Buffer
	text := func(size, y int, s string) {
		fmt.Fprintf(&content, "BT /F1 %d Tf 56 %d Td (", size, y)
		content.Write(pdfEscape(latin1(s)))
		content.WriteString(") Tj ET\n")
	}
	text(18, 780, title)
	y := 748
	for _, line := range lines {
		text(11, y, line)
		y -= 18
	}

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>"),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"),
		[]byte(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String())),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", i+1)
		out.Write(object)
		out.WriteString("\nendobj\n")
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// columnName gives A, B, C... for a zero-based column.
func columnName(i int) string { return string(rune('A' + i)) }

// simpleXLSX builds a valid workbook with one sheet. A cell is a string or a number.
func simpleXLSX(sheet string, rows [][]any) []byte {
	var data bytes.Buffer
	data.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&data, `<row r="%d">`, r+1)
		for c, cell := range row {
			ref := fmt.Sprintf("%s%d", columnName(c), r+1)
			switch v := cell.(type) {
			case string:
				fmt.Fprintf(&data, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, xmlText(v))
			case int:
				fmt.Fprintf(&data, `<c r="%s"><v>%d</v></c>`, ref, v)
			case float64:
				fmt.Fprintf(&data, `<c r="%s"><v>%.2f</v></c>`, ref, v)
			}
		}
		data.WriteString(`</row>`)
	}
	data.WriteString(`</sheetData></worksheet>`)

	parts := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + xmlText(sheet) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   data.String(),
	}
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names) // [Content_Types].xml sorts first
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, name := range names {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(parts[name]))
	}
	_ = w.Close()
	return out.Bytes()
}

// --- pictures of a vehicle, drawn in code ---

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, r int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx, dy := 0, 0
			if x < x0+r {
				dx = x0 + r - x
			} else if x >= x1-r {
				dx = x - (x1 - r - 1)
			}
			if y < y0+r {
				dy = y0 + r - y
			} else if y >= y1-r {
				dy = y - (y1 - r - 1)
			}
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func fillPolygon(img *image.RGBA, points [][2]int, c color.RGBA) {
	minY, maxY := points[0][1], points[0][1]
	for _, p := range points {
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	for y := minY; y <= maxY; y++ {
		var xs []int
		for i := range points {
			a, b := points[i], points[(i+1)%len(points)]
			if (a[1] <= y && b[1] > y) || (b[1] <= y && a[1] > y) {
				xs = append(xs, a[0]+(y-a[1])*(b[0]-a[0])/(b[1]-a[1]))
			}
		}
		sort.Ints(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			for x := xs[i]; x <= xs[i+1]; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// carPNG draws a simple side view of a pickup in the given body color: enough for the catalog and the sale
// screens to look alive without shipping photographs.
func carPNG(body color.RGBA) []byte {
	const w, h = 960, 540
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		shade := uint8(247 - y/14)
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{shade, shade, shade, 255})
		}
	}
	window := color.RGBA{198, 218, 232, 255}
	dark := color.RGBA{34, 34, 38, 255}
	rim := color.RGBA{186, 188, 192, 255}
	ground := color.RGBA{205, 205, 205, 255}

	fillRoundedRect(img, 110, 408, 850, 432, 12, ground)
	fillRoundedRect(img, 120, 268, 840, 394, 42, body)
	fillPolygon(img, [][2]int{{300, 270}, {364, 170}, {630, 170}, {736, 270}}, body)
	fillPolygon(img, [][2]int{{332, 264}, {376, 190}, {470, 190}, {470, 264}}, window)
	fillPolygon(img, [][2]int{{490, 264}, {490, 190}, {620, 190}, {700, 264}}, window)
	fillRect(img, 120, 360, 840, 372, color.RGBA{body.R / 2, body.G / 2, body.B / 2, 255})
	fillRect(img, 800, 296, 836, 312, color.RGBA{255, 240, 190, 255})
	fillRect(img, 124, 296, 148, 316, color.RGBA{200, 50, 50, 255})
	for _, cx := range []int{270, 690} {
		fillCircle(img, cx, 394, 64, dark)
		fillCircle(img, cx, 394, 32, rim)
		fillCircle(img, cx, 394, 10, dark)
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

// bodyColor returns a paint color by name, for the catalog's colors.
func bodyColor(name string) color.RGBA {
	switch strings.ToLower(name) {
	case "blanco", "blanco perla":
		return color.RGBA{238, 238, 236, 255}
	case "negro":
		return color.RGBA{28, 28, 32, 255}
	case "gris plata", "plata":
		return color.RGBA{168, 172, 178, 255}
	case "rojo":
		return color.RGBA{178, 24, 32, 255}
	case "azul":
		return color.RGBA{30, 70, 140, 255}
	}
	return color.RGBA{120, 120, 120, 255}
}
