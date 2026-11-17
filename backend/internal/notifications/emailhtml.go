package notifications

import (
	"fmt"
	"html"
	"strings"
	"unicode"
)

// The look of every email. Email clients are far less forgiving than browsers, so the layout follows the
// usual rules: nested tables for structure, every style inline, system fonts, no images (many clients block
// them until the reader allows it, and a localhost address would never load), and a "bulletproof" button made
// of a table cell so it renders even where CSS backgrounds on links do not. Colors come from the app's brand.
const (
	colorRed      = "#f11119"
	colorRedDark  = "#c90e16"
	colorRedSoft  = "#fff0f0"
	colorInk      = "#202020"
	colorDark     = "#191919"
	colorMuted    = "#777777"
	colorLine     = "#e7e7e7"
	colorSoft     = "#f8f8f8"
	colorPage     = "#f1f1f1"
	colorBarEmpty = "#e3e3e3"

	fontStack = "-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"
)

// fact is one labelled line of a details table.
type fact struct{ label, value string }

// person is a contact card: the advisor in the customer's emails.
type person struct{ name, email, phone string }

// button is the call to action.
type button struct{ label, url string }

// logoWidth and logoHeight are the size the header logo is drawn at (the file is 866x288, so it stays sharp on
// high-density screens).
const (
	logoWidth  = 150
	logoHeight = 50
)

// content is everything one email says, independent of how it is drawn. build turns it into both versions.
type content struct {
	logoURL        string // public address of the header logo. Empty draws the brand name as text instead.
	preheader      string // the preview line some clients show next to the subject
	eyebrow        string // a short label above the headline
	title          string
	greeting       string
	paragraphs     []string
	facts          []fact
	contact        *person
	progress       []milestone // the purchase milestones; drawn as a progress bar when present
	button         *button
	showLink       bool // prints the button's address under it, for readers whose client breaks buttons
	notes          []string
	toCustomer     bool // picks the footer wording for customers.
	replyToAdvisor bool // sends replies to the sale's advisor
}

// withLogo sets the header logo address.
func (c content) withLogo(url string) content {
	c.logoURL = url
	return c
}

// brandHTML is the left side of the header: the logo when there is an address for it, else the brand name as
// text. The image carries its size and an alt text drawn in the header's colors, so a client that blocks
// images (or a logo that fails to load) still shows the name instead of an empty box.
func (c content) brandHTML() string {
	if c.logoURL != "" {
		return fmt.Sprintf(`<img src="%s" width="%d" height="%d" alt="EndosoFlow" style="display:block;border:0;outline:none;width:%dpx;height:%dpx;font-family:%s;font-size:22px;font-weight:800;color:%s;line-height:%dpx;">`,
			esc(c.logoURL), logoWidth, logoHeight, logoWidth, logoHeight, fontStack, colorInk, logoHeight)
	}
	return fmt.Sprintf(`<span style="font-family:%s;font-size:22px;font-weight:800;letter-spacing:-0.3px;color:%s;">Endoso<span style="color:%s;">Flow</span></span>`, fontStack, colorInk, colorRed)
}

// build renders both versions of the email.
func (c content) build(subject string) rendered {
	return rendered{
		Subject:        subject,
		Text:           c.text(),
		HTML:           c.html(subject),
		ReplyToAdvisor: c.replyToAdvisor,
	}
}

func esc(s string) string { return html.EscapeString(s) }

func (c content) text() string {
	var b strings.Builder
	b.WriteString(c.greeting + "\n\n")
	for _, p := range c.paragraphs {
		b.WriteString(p + "\n\n")
	}
	if len(c.progress) > 0 {
		b.WriteString(progressTitle(c.progress) + "\n")
		for _, m := range c.progress {
			mark := map[string]string{"completed": "✓", "in_progress": "▶"}[m.State]
			if mark == "" {
				mark = "○"
			}
			fmt.Fprintf(&b, "  %s %d. %s\n", mark, m.Number, m.Name)
		}
		b.WriteString("\n")
	}
	wrote := false
	for _, f := range c.facts {
		if strings.TrimSpace(f.value) != "" {
			b.WriteString(f.label + ": " + f.value + "\n")
			wrote = true
		}
	}
	if p := c.contact; p != nil {
		b.WriteString("Su asesor: " + p.name + "\n")
		if p.email != "" {
			b.WriteString("Correo del asesor: " + p.email + "\n")
		}
		if p.phone != "" {
			b.WriteString("Teléfono del asesor: " + p.phone + "\n")
		}
		if w := whatsappURL(p.phone); w != "" {
			b.WriteString("WhatsApp del asesor: " + w + "\n")
		}
		wrote = true
	}
	if wrote {
		b.WriteString("\n")
	}
	if c.button != nil {
		b.WriteString(c.button.label + ": " + c.button.url + "\n\n")
	}
	for _, n := range c.notes {
		b.WriteString(n + "\n")
	}
	if len(c.notes) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("— " + brand + "\n")
	return b.String()
}

func (c content) html(subject string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="es"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">` +
		`<title>` + esc(subject) + `</title></head>`)
	fmt.Fprintf(&b, `<body style="margin:0;padding:0;background:%s;-webkit-text-size-adjust:100%%;">`, colorPage)

	// Hidden preview text. The trailing entities push the body text out of the preview line.
	fmt.Fprintf(&b, `<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:%s;font-size:1px;line-height:1px;">%s%s</div>`,
		colorPage, esc(c.preheader), strings.Repeat("&#847;&zwnj;&nbsp;", 24))

	fmt.Fprintf(&b, `<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" bgcolor="%s"><tr><td align="center" style="padding:28px 12px;">`, colorPage)
	b.WriteString(`<table role="presentation" width="600" cellspacing="0" cellpadding="0" border="0" style="width:600px;max-width:100%;">`)

	// Header band. It is white because the logo's black lettering needs a light background.
	fmt.Fprintf(&b, `<tr><td bgcolor="#ffffff" style="background:#ffffff;border:1px solid %s;border-bottom:0;border-radius:16px 16px 0 0;padding:18px 32px;">`+
		`<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0"><tr>`+
		`<td valign="middle">%s</td>`+
		`<td align="right" valign="middle" style="font-family:%s;font-size:11px;font-weight:700;letter-spacing:1.6px;color:#9a9a9a;text-transform:uppercase;">Grupo Pana</td>`+
		`</tr></table></td></tr>`, colorLine, c.brandHTML(), fontStack)
	// Red accent line.
	fmt.Fprintf(&b, `<tr><td height="4" bgcolor="%s" style="background:%s;height:4px;line-height:4px;font-size:0;">&nbsp;</td></tr>`, colorRed, colorRed)

	// Card.
	fmt.Fprintf(&b, `<tr><td bgcolor="#ffffff" style="background:#ffffff;padding:36px 32px 32px;border-left:1px solid %s;border-right:1px solid %s;font-family:%s;color:%s;">`,
		colorLine, colorLine, fontStack, colorInk)
	if c.eyebrow != "" {
		fmt.Fprintf(&b, `<div style="font-size:11px;font-weight:800;letter-spacing:1.6px;text-transform:uppercase;color:%s;margin:0 0 10px;">%s</div>`, colorRed, esc(c.eyebrow))
	}
	fmt.Fprintf(&b, `<h1 style="margin:0 0 22px;font-size:26px;line-height:1.2;font-weight:800;letter-spacing:-0.4px;color:%s;">%s</h1>`, colorInk, esc(c.title))
	fmt.Fprintf(&b, `<p style="margin:0 0 14px;font-size:16px;line-height:1.55;color:%s;">%s</p>`, colorInk, esc(c.greeting))
	for _, p := range c.paragraphs {
		fmt.Fprintf(&b, `<p style="margin:0 0 14px;font-size:15px;line-height:1.6;color:#404040;">%s</p>`, esc(p))
	}

	if len(c.progress) > 0 {
		b.WriteString(progressHTML(c.progress))
	}
	b.WriteString(factsHTML(c.facts))
	if c.contact != nil {
		b.WriteString(c.contact.html())
	}
	if c.button != nil {
		b.WriteString(c.button.html())
		if c.showLink {
			fmt.Fprintf(&b, `<p style="margin:0 0 6px;font-size:12px;line-height:1.5;color:%s;">Si el botón no funciona, copie y pegue esta dirección en su navegador:</p>`+
				`<p style="margin:0 0 18px;font-size:12px;line-height:1.5;word-break:break-all;"><a href="%s" style="color:%s;text-decoration:underline;">%s</a></p>`,
				colorMuted, esc(c.button.url), colorRedDark, esc(c.button.url))
		}
	}
	for _, n := range c.notes {
		fmt.Fprintf(&b, `<p style="margin:0 0 8px;font-size:13px;line-height:1.55;color:%s;">%s</p>`, colorMuted, esc(n))
	}
	b.WriteString(`</td></tr>`)

	// Footer.
	footer := "Recibe este mensaje porque tiene una cuenta en EndosoFlow."
	if c.toCustomer {
		footer = "Este es un mensaje automático de Grupo Pana."
		if c.replyToAdvisor {
			footer = "Puede responder a este correo: su mensaje llegará directamente a su asesor."
		}
	}
	fmt.Fprintf(&b, `<tr><td bgcolor="%s" style="background:%s;border:1px solid %s;border-top:0;border-radius:0 0 16px 16px;padding:20px 32px;font-family:%s;">`+
		`<p style="margin:0 0 4px;font-size:12px;line-height:1.5;color:%s;">%s</p>`+
		`<p style="margin:0;font-size:12px;line-height:1.5;color:#a0a0a0;">%s</p></td></tr>`,
		colorSoft, colorSoft, colorLine, fontStack, colorMuted, esc(footer), esc(brand))

	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}

// factsHTML draws the details table, skipping lines without a value. It returns "" when nothing is left.
func factsHTML(facts []fact) string {
	var rows strings.Builder
	n := 0
	for _, f := range facts {
		if strings.TrimSpace(f.value) == "" {
			continue
		}
		n++
		fmt.Fprintf(&rows, `<tr><td valign="top" width="38%%" style="padding:11px 12px 11px 0;font-size:13px;color:%s;border-top:1px solid %s;">%s</td>`+
			`<td valign="top" style="padding:11px 0;font-size:14px;font-weight:700;color:%s;border-top:1px solid %s;">%s</td></tr>`,
			colorMuted, colorLine, esc(f.label), colorInk, colorLine, esc(f.value))
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(`<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="margin:8px 0 22px;"><tr><td style="padding:0 0 4px;">`+
		`<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="border-bottom:1px solid %s;">%s</table></td></tr></table>`,
		colorLine, rows.String())
}

// html draws the advisor card.
func (p person) html() string {
	var lines strings.Builder
	if p.email != "" {
		fmt.Fprintf(&lines, `<div style="font-size:13px;line-height:1.6;"><a href="mailto:%s" style="color:%s;text-decoration:none;">%s</a></div>`, esc(p.email), colorRedDark, esc(p.email))
	}
	if p.phone != "" {
		fmt.Fprintf(&lines, `<div style="font-size:13px;line-height:1.6;"><a href="tel:%s" style="color:%s;text-decoration:none;">%s</a></div>`, esc(phoneDigits(p.phone)), colorInk, esc(p.phone))
	}
	if w := whatsappURL(p.phone); w != "" {
		fmt.Fprintf(&lines, `<div style="font-size:13px;line-height:1.6;margin-top:2px;"><a href="%s" target="_blank" style="color:#1a7f45;font-weight:700;text-decoration:none;">Escribir por WhatsApp &rarr;</a></div>`, esc(w))
	}
	return fmt.Sprintf(`<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="margin:6px 0 24px;background:%s;border:1px solid %s;border-radius:12px;">`+
		`<tr><td width="64" valign="middle" style="padding:16px 0 16px 16px;">`+
		`<table role="presentation" cellspacing="0" cellpadding="0" border="0"><tr><td width="44" height="44" align="center" valign="middle" bgcolor="%s" style="width:44px;height:44px;border-radius:22px;background:%s;color:#ffffff;font-family:%s;font-size:17px;font-weight:800;">%s</td></tr></table></td>`+
		`<td valign="middle" style="padding:16px 16px 16px 14px;font-family:%s;">`+
		`<div style="font-size:11px;font-weight:800;letter-spacing:1.2px;text-transform:uppercase;color:%s;">Su asesor</div>`+
		`<div style="font-size:16px;font-weight:800;color:%s;margin:2px 0 3px;">%s</div>%s</td></tr></table>`,
		colorSoft, colorLine, colorDark, colorDark, fontStack, esc(initials(p.name)), fontStack, colorMuted, colorInk, esc(p.name), lines.String())
}

// progressTitle is the heading of the progress bar: how many of the milestones are done.
func progressTitle(ms []milestone) string {
	done := 0
	for _, m := range ms {
		if m.State == "completed" {
			done++
		}
	}
	if done == len(ms) {
		return "Avance de su compra: compra completada"
	}
	return fmt.Sprintf("Avance de su compra: %d de %d etapas completadas", done, len(ms))
}

// progressHTML draws the purchase milestones: a bar for each one (red when completed, light red while it is the
// current one, grey while pending) with its name underneath.
func progressHTML(ms []milestone) string {
	width := 100 / len(ms)
	var bars, names strings.Builder
	for _, m := range ms {
		fill, weight, color := colorBarEmpty, "400", "#a8a8a8"
		switch m.State {
		case "completed":
			fill, color = colorRed, colorMuted
		case "in_progress":
			fill, weight, color = "#f6a5a8", "800", colorInk
		}
		fmt.Fprintf(&bars, `<td width="%d%%" style="padding:0 2px;"><div style="height:6px;line-height:6px;font-size:0;background:%s;border-radius:3px;">&nbsp;</div></td>`, width, fill)
		fmt.Fprintf(&names, `<td width="%d%%" valign="top" align="center" style="padding:7px 2px 0;font-size:10px;line-height:1.25;font-weight:%s;color:%s;">%s</td>`, width, weight, color, esc(m.Name))
	}
	return fmt.Sprintf(`<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="margin:10px 0 22px;background:%s;border:1px solid %s;border-radius:12px;"><tr><td style="padding:16px 12px 14px;">`+
		`<div style="font-family:%s;font-size:11px;font-weight:800;letter-spacing:1.2px;text-transform:uppercase;color:%s;margin:0 0 10px 2px;">%s</div>`+
		`<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="font-family:%s;"><tr>%s</tr><tr>%s</tr></table></td></tr></table>`,
		colorSoft, colorLine, fontStack, colorMuted, esc(progressTitle(ms)), fontStack, bars.String(), names.String())
}

// html draws the call to action as a table cell, which survives clients that ignore styles on links.
func (b button) html() string {
	return fmt.Sprintf(`<table role="presentation" cellspacing="0" cellpadding="0" border="0" style="margin:10px 0 20px;"><tr>`+
		`<td align="center" bgcolor="%s" style="background:%s;border-radius:10px;">`+
		`<a href="%s" target="_blank" style="display:inline-block;padding:14px 30px;font-family:%s;font-size:15px;font-weight:800;color:#ffffff;text-decoration:none;border-radius:10px;">%s</a>`+
		`</td></tr></table>`, colorRed, colorRed, esc(b.url), fontStack, esc(b.label))
}

// initials returns up to two capital letters for the avatar of a person.
func initials(name string) string {
	var out []rune
	for _, word := range strings.Fields(name) {
		if r := []rune(word)[0]; unicode.IsLetter(r) {
			out = append(out, unicode.ToUpper(r))
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "•"
	}
	return string(out)
}

// phoneDigits keeps what a tel: link may contain.
func phoneDigits(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if unicode.IsDigit(r) || r == '+' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// whatsappURL builds the chat address of a phone number, or "" when the number cannot be told apart from a local
// one. Peruvian mobile numbers (nine digits starting with 9) get the country code; a number that already starts
// with + keeps its own.
func whatsappURL(phone string) string {
	digits := strings.TrimPrefix(phoneDigits(phone), "+")
	switch {
	case strings.HasPrefix(strings.TrimSpace(phone), "+") && len(digits) >= 8:
		return "https://wa.me/" + digits
	case len(digits) == 9 && digits[0] == '9':
		return "https://wa.me/51" + digits
	}
	return ""
}
