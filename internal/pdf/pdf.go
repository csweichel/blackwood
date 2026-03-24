package pdf

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// AttachmentResolver returns the raw bytes and content type for an attachment ID.
// Returns an error if the attachment cannot be found.
type AttachmentResolver func(id string) (data []byte, contentType string, err error)

// FileResolver returns the raw bytes and content type for a filename
// relative to the note's day directory.
type FileResolver func(filename string) (data []byte, contentType string, err error)

// Options configures PDF generation.
type Options struct {
	// Date is the YYYY-MM-DD date string shown as the document title.
	Date string
	// ResolveAttachment resolves inline attachment images by DB ID. May be nil.
	ResolveAttachment AttachmentResolver
	// ResolveFile resolves inline attachment images by relative filename. May be nil.
	ResolveFile FileResolver
}

const (
	pageW      = 210.0 // A4 width mm
	marginL    = 20.0
	marginR    = 20.0
	marginT    = 20.0
	marginB    = 20.0
	contentW   = pageW - marginL - marginR
	lineHeight = 5.0

	fontFamily = "Helvetica"
)

// Generate converts markdown content to a PDF document written to w.
func Generate(w io.Writer, markdown string, opts Options) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginL, marginT, marginR)
	pdf.SetAutoPageBreak(true, marginB)
	pdf.AddPage()

	// Title: formatted date
	title := formatDateTitle(opts.Date)
	pdf.SetFont(fontFamily, "B", 20)
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(contentW, 10, title, "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// Thin rule under title
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetLineWidth(0.3)
	y := pdf.GetY()
	pdf.Line(marginL, y, pageW-marginR, y)
	pdf.Ln(6)

	// Parse markdown AST
	source := []byte(markdown)
	md := goldmark.New()
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	r := &renderer{
		pdf:    pdf,
		source: source,
		opts:   opts,
	}
	r.render(doc)

	if pdf.Err() {
		return fmt.Errorf("pdf generation: %s", pdf.Error())
	}

	return pdf.Output(w)
}

type renderer struct {
	pdf    *fpdf.Fpdf
	source []byte
	opts   Options

	bold      int
	italic    int
	inCode    bool
	listDepth int
	listItem  int // counter per depth for ordered lists
}

func (r *renderer) render(n ast.Node) {
	r.walkNode(n)
}

func (r *renderer) walkNode(n ast.Node) {
	skipChildren := r.enterNode(n)
	if !skipChildren {
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			r.walkNode(child)
		}
	}
	r.exitNode(n)
}

// enterNode processes a node on entry. Returns true if children should be skipped.
func (r *renderer) enterNode(n ast.Node) bool {
	switch n.Kind() {
	case ast.KindHeading:
		heading := n.(*ast.Heading)
		r.pdf.Ln(4)
		size := headingSize(heading.Level)
		r.pdf.SetFont(fontFamily, "B", size)
		r.pdf.SetTextColor(30, 30, 30)

	case ast.KindParagraph:
		r.setBodyFont()

	case ast.KindTextBlock:
		r.setBodyFont()

	case ast.KindText:
		t := n.(*ast.Text)
		content := string(t.Segment.Value(r.source))
		r.writeText(content)
		if t.HardLineBreak() || t.SoftLineBreak() {
			r.pdf.Ln(lineHeight)
		}

	case ast.KindString:
		s := n.(*ast.String)
		r.writeText(string(s.Value))

	case ast.KindEmphasis:
		em := n.(*ast.Emphasis)
		if em.Level >= 2 {
			r.bold++
		} else {
			r.italic++
		}
		r.applyStyle()

	case ast.KindCodeSpan:
		r.inCode = true
		r.pdf.SetFont("Courier", "", 9)
		r.pdf.SetFillColor(240, 240, 240)

	case ast.KindFencedCodeBlock, ast.KindCodeBlock:
		r.pdf.Ln(2)
		r.pdf.SetFont("Courier", "", 8)
		r.pdf.SetFillColor(245, 245, 245)
		r.pdf.SetTextColor(50, 50, 50)

		// Collect all lines from the code block
		var code strings.Builder
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			code.Write(seg.Value(r.source))
		}
		codeText := strings.TrimRight(code.String(), "\n")

		// Draw background and text
		x := r.pdf.GetX()
		r.pdf.SetX(x + 2)
		for _, line := range strings.Split(codeText, "\n") {
			if line == "" {
				line = " "
			}
			r.pdf.CellFormat(contentW-4, lineHeight, line, "", 1, "L", true, 0, "")
		}
		r.pdf.Ln(2)
		r.setBodyFont()
		return true

	case ast.KindList:
		r.listDepth++
		r.listItem = 0

	case ast.KindListItem:
		r.listItem++
		list := n.Parent().(*ast.List)
		indent := float64(r.listDepth-1) * 6.0
		r.pdf.SetX(marginL + indent + 4)

		if list.IsOrdered() {
			bullet := fmt.Sprintf("%d. ", r.listItem)
			r.pdf.SetFont(fontFamily, "", 10)
			r.pdf.Write(lineHeight, bullet)
		} else {
			r.pdf.SetFont(fontFamily, "", 10)
			r.pdf.Write(lineHeight, "• ")
		}

	case ast.KindThematicBreak:
		r.pdf.Ln(3)
		y := r.pdf.GetY()
		r.pdf.SetDrawColor(200, 200, 200)
		r.pdf.SetLineWidth(0.2)
		r.pdf.Line(marginL, y, pageW-marginR, y)
		r.pdf.Ln(3)

	case ast.KindLink:
		link := n.(*ast.Link)
		r.pdf.SetTextColor(0, 0, 180)
		linkText := extractText(n, r.source)
		r.pdf.WriteLinkString(lineHeight, linkText, string(link.Destination))
		r.pdf.SetTextColor(30, 30, 30)
		return true

	case ast.KindAutoLink:
		link := n.(*ast.AutoLink)
		url := string(link.URL(r.source))
		r.pdf.SetTextColor(0, 0, 180)
		r.pdf.WriteLinkString(lineHeight, url, url)
		r.pdf.SetTextColor(30, 30, 30)
		return true

	case ast.KindImage:
		img := n.(*ast.Image)
		r.handleImage(string(img.Destination))
		return true

	case ast.KindHTMLBlock, ast.KindRawHTML:
		r.handleRawHTML(n)
		return true

	case ast.KindBlockquote:
		r.pdf.Ln(2)
		r.pdf.SetX(marginL + 6)
		r.pdf.SetTextColor(100, 100, 100)
		r.pdf.SetFont(fontFamily, "I", 10)
	}
	return false
}

func (r *renderer) exitNode(n ast.Node) {
	switch n.Kind() {
	case ast.KindHeading:
		r.pdf.Ln(lineHeight + 2)
		r.setBodyFont()

	case ast.KindParagraph:
		r.pdf.Ln(lineHeight + 1)

	case ast.KindEmphasis:
		em := n.(*ast.Emphasis)
		if em.Level >= 2 {
			r.bold--
		} else {
			r.italic--
		}
		r.applyStyle()

	case ast.KindCodeSpan:
		r.inCode = false
		r.setBodyFont()

	case ast.KindList:
		r.listDepth--
		if r.listDepth == 0 {
			r.pdf.Ln(2)
		}

	case ast.KindListItem:
		r.pdf.Ln(lineHeight)

	case ast.KindBlockquote:
		r.pdf.Ln(lineHeight)
		r.pdf.SetX(marginL)
		r.setBodyFont()

	case ast.KindLink:
		// Children already handled in enterNode; skip them.
	}
}

func (r *renderer) writeText(s string) {
	if r.inCode {
		// Inline code: draw with background
		r.pdf.CellFormat(r.pdf.GetStringWidth(s)+2, lineHeight, s, "", 0, "L", true, 0, "")
	} else {
		r.pdf.Write(lineHeight, s)
	}
}

func (r *renderer) setBodyFont() {
	r.pdf.SetFont(fontFamily, r.styleStr(), 10)
	r.pdf.SetTextColor(30, 30, 30)
}

func (r *renderer) applyStyle() {
	if r.inCode {
		return
	}
	r.pdf.SetFont(fontFamily, r.styleStr(), 0)
}

func (r *renderer) styleStr() string {
	s := ""
	if r.bold > 0 {
		s += "B"
	}
	if r.italic > 0 {
		s += "I"
	}
	return s
}

var attachmentRe = regexp.MustCompile(`/api/attachments/([a-zA-Z0-9_-]+)`)

func (r *renderer) handleImage(src string) {
	var data []byte
	var contentType string
	var id string

	// Try old-style /api/attachments/{id} first.
	if m := attachmentRe.FindStringSubmatch(src); m != nil && r.opts.ResolveAttachment != nil {
		id = m[1]
		var err error
		data, contentType, err = r.opts.ResolveAttachment(id)
		if err != nil {
			return
		}
	} else if r.opts.ResolveFile != nil && !strings.HasPrefix(src, "/") && !strings.HasPrefix(src, "http") {
		// Relative filename — resolve from day directory.
		id = src
		var err error
		data, contentType, err = r.opts.ResolveFile(src)
		if err != nil {
			return
		}
	} else {
		return
	}

	var imgType string
	switch {
	case strings.Contains(contentType, "png"):
		imgType = "PNG"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		imgType = "JPEG"
	case strings.Contains(contentType, "gif"):
		imgType = "GIF"
	default:
		return // unsupported image type
	}

	r.pdf.Ln(2)
	name := fmt.Sprintf("att_%s", id)
	reader := bytes.NewReader(data)
	opt := fpdf.ImageOptions{ImageType: imgType, ReadDpi: true}
	r.pdf.RegisterImageOptionsReader(name, opt, reader)

	// Scale to fit content width, max 100mm tall
	maxW := contentW - 10
	maxH := 100.0
	info := r.pdf.GetImageInfo(name)
	if info == nil {
		return
	}
	w := info.Width()
	h := info.Height()
	if w > maxW {
		ratio := maxW / w
		w = maxW
		h = h * ratio
	}
	if h > maxH {
		ratio := maxH / h
		h = maxH
		w = w * ratio
	}

	r.pdf.ImageOptions(name, marginL+2, r.pdf.GetY(), w, h, false, opt, 0, "")
	r.pdf.Ln(h + 2)
}

var audioTagRe = regexp.MustCompile(`<audio[^>]*>.*?</audio>`)

func (r *renderer) handleRawHTML(n ast.Node) {
	var raw string
	if n.Kind() == ast.KindHTMLBlock {
		lines := n.Lines()
		var buf strings.Builder
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			buf.Write(seg.Value(r.source))
		}
		raw = buf.String()
	} else {
		// RawHTML inline
		raw = string(n.(*ast.RawHTML).Segments.Value(r.source))
	}

	// For audio tags, show "[Voice memo]" placeholder
	if audioTagRe.MatchString(raw) {
		r.pdf.SetFont(fontFamily, "I", 9)
		r.pdf.SetTextColor(120, 120, 120)
		r.pdf.Write(lineHeight, "[Voice memo]")
		r.setBodyFont()
		return
	}

	// For other HTML, try to render as plain text (strip tags)
	stripped := stripHTMLTags(raw)
	stripped = strings.TrimSpace(stripped)
	if stripped != "" {
		r.pdf.Write(lineHeight, stripped)
	}
}

func headingSize(level int) float64 {
	switch level {
	case 1:
		return 18
	case 2:
		return 15
	case 3:
		return 13
	case 4:
		return 12
	default:
		return 11
	}
}

func formatDateTitle(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("Monday, January 2, 2006")
}

func extractText(n ast.Node, source []byte) string {
	var buf strings.Builder
	collectText(n, source, &buf)
	return buf.String()
}

func collectText(n ast.Node, source []byte, buf *strings.Builder) {
	if n.Kind() == ast.KindText {
		t := n.(*ast.Text)
		buf.Write(t.Segment.Value(source))
	}
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		collectText(child, source, buf)
	}
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	return htmlTagRe.ReplaceAllString(s, "")
}
