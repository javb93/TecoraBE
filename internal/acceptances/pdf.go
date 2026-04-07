package acceptances

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"time"
)

type Renderer struct{}

const (
	pdfPageWidth     = 612.0
	pdfPageHeight    = 792.0
	pdfPageMargin    = 44.0
	pdfSectionGap    = 18.0
	pdfContentWidth  = pdfPageWidth - (pdfPageMargin * 2)
	pdfFooterHeight  = 132.0
	pdfSignatureArea = 208.0
)

func NewPDFRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(record Record) (PDFDocument, error) {
	content, imageObject, err := buildPDFContent(record)
	if err != nil {
		return PDFDocument{}, err
	}

	objects := make([][]byte, 0, 6)
	addObject := func(data []byte) int {
		objects = append(objects, data)
		return len(objects)
	}

	fontID := addObject([]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	boldFontID := addObject([]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>"))

	var imageID int
	if imageObject != nil {
		imageID = addObject(imageObject)
	}

	contentID := addObject(streamObject([]byte(content)))

	pageResources := fmt.Sprintf("<< /Font << /F1 %d 0 R /F2 %d 0 R >>", fontID, boldFontID)
	if imageID != 0 {
		pageResources += fmt.Sprintf(" /XObject << /Im1 %d 0 R >>", imageID)
	}
	pageResources += " >>"

	pageID := addObject([]byte(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources %s /Contents %d 0 R >>", len(objects)+2, pageResources, contentID)))
	pagesID := addObject([]byte(fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageID)))
	catalogID := addObject([]byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID)))

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", i+1)
		out.Write(object)
		out.WriteString("\nendobj\n")
	}

	xrefOffset := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(objects)+1)
	out.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF", len(objects)+1, catalogID, xrefOffset)

	return PDFDocument{
		Bytes:       out.Bytes(),
		ContentType: "application/pdf",
	}, nil
}

func buildPDFContent(record Record) (string, []byte, error) {
	imageObject, imageWidth, imageHeight, err := signatureImageObject(record.SignatureImageBase64)
	if err != nil {
		return "", nil, err
	}

	var content strings.Builder

	writeFillColor(&content, 1, 1, 1)
	drawFilledRect(&content, 0, 0, pdfPageWidth, pdfPageHeight)

	headerBottom := pdfPageHeight - 126
	writeFillColor(&content, 0.09, 0.21, 0.34)
	drawFilledRect(&content, pdfPageMargin, headerBottom, pdfContentWidth, 82)
	writeText(&content, "/F1", 10, 0.85, 0.91, 0.95, pdfPageMargin+24, headerBottom+57, "WORK ACCEPTANCE")
	writeText(&content, "/F2", 26, 1, 1, 1, pdfPageMargin+24, headerBottom+30, record.WorkOrderID)
	writeText(&content, "/F1", 11, 0.85, 0.91, 0.95, pdfPageMargin+24, headerBottom+12, "Acceptance ID "+record.ID)

	statusText := "Pending approval"
	statusR, statusG, statusB := 0.64, 0.16, 0.16
	if record.Approved {
		statusText = "Approved"
		statusR, statusG, statusB = 0.13, 0.44, 0.24
	}
	drawRoundedBadge(&content, pdfPageMargin+pdfContentWidth-132, headerBottom+21, 92, 30, statusR, statusG, statusB)
	writeText(&content, "/F2", 11, 1, 1, 1, pdfPageMargin+pdfContentWidth-106, headerBottom+31, strings.ToUpper(statusText))

	y := headerBottom - 18.0
	y = drawSectionHeader(&content, y, "Customer Information")
	y = drawDetailBox(&content, y, []detailRow{
		{Label: "Name", Value: defaultValue(record.CustomerName)},
		{Label: "Email", Value: defaultValue(record.CustomerEmail)},
	})

	y -= pdfSectionGap
	y = drawSectionHeader(&content, y, "Job Information")
	expirationValue := highlightValue(record.ServiceExpirationDate)
	y = drawDetailBox(&content, y, []detailRow{
		{Label: "Service Date", Value: defaultValue(record.ServiceDate)},
		{Label: "Expiration Date", Value: expirationValue, Highlight: true},
		{Label: "Service Type", Value: defaultValue(record.ServiceType)},
		{Label: "Technician", Value: defaultValue(record.SignedByTechnicianID)},
		{Label: "Signed At (UTC)", Value: record.SignedAt.UTC().Format(time.RFC3339)},
	})

	y -= pdfSectionGap
	y = drawSectionHeader(&content, y, "Included Products")
	y = drawListBox(&content, y, sanitizedProducts(record.Products))

	y -= pdfSectionGap
	y = drawSectionHeader(&content, y, "Notes")
	y = drawParagraphBox(&content, y, defaultNotes(record.Notes))

	drawSignatureFooter(&content, imageWidth, imageHeight)
	if imageObject != nil {
		const maxWidth = 170.0
		const maxHeight = 54.0
		drawWidth := float64(imageWidth)
		drawHeight := float64(imageHeight)
		scale := min(maxWidth/drawWidth, maxHeight/drawHeight)
		if scale < 1 {
			drawWidth *= scale
			drawHeight *= scale
		}
		x := pdfPageWidth - pdfPageMargin - drawWidth - 18
		y := 48.0
		content.WriteString(fmt.Sprintf("q %.2f 0 0 %.2f %.2f %.2f cm /Im1 Do Q\n", drawWidth, drawHeight, x, y))
	}

	return content.String(), imageObject, nil
}

type detailRow struct {
	Label     string
	Value     string
	Highlight bool
}

func drawSectionHeader(content *strings.Builder, y float64, title string) float64 {
	writeText(content, "/F2", 13, 0.09, 0.21, 0.34, pdfPageMargin, y, title)
	drawLine(content, pdfPageMargin, y-6, pdfPageMargin+pdfContentWidth, y-6, 1.2, 0.77, 0.82, 0.87)
	return y - 16
}

func drawDetailBox(content *strings.Builder, top float64, rows []detailRow) float64 {
	const rowHeight = 28.0
	height := (float64(len(rows)) * rowHeight) + 18
	bottom := top - height

	writeFillColor(content, 0.97, 0.98, 0.99)
	drawFilledRect(content, pdfPageMargin, bottom, pdfContentWidth, height)
	drawStrokeRect(content, pdfPageMargin, bottom, pdfContentWidth, height, 0.8, 0.85, 0.88, 0.91)

	currentY := top - 20.0
	for i, row := range rows {
		if i > 0 {
			drawLine(content, pdfPageMargin+18, currentY+10, pdfPageMargin+pdfContentWidth-18, currentY+10, 0.6, 0.88, 0.9, 0.93)
		}
		writeText(content, "/F1", 9, 0.38, 0.45, 0.53, pdfPageMargin+18, currentY+8, strings.ToUpper(row.Label))
		if row.Highlight {
			writeFillColor(content, 1, 0.95, 0.86)
			drawFilledRect(content, pdfPageMargin+166, currentY-3, 162, 18)
		}
		writeText(content, "/F2", 12, 0.11, 0.13, 0.16, pdfPageMargin+104, currentY+4, row.Value)
		currentY -= rowHeight
	}

	return bottom
}

func drawListBox(content *strings.Builder, top float64, items []string) float64 {
	lines := make([]string, 0, len(items)*2)
	for _, item := range items {
		wrapped := wrapText(item, 52)
		if len(wrapped) == 0 {
			continue
		}
		lines = append(lines, "• "+wrapped[0])
		for _, continuation := range wrapped[1:] {
			lines = append(lines, "  "+continuation)
		}
	}
	if len(lines) == 0 {
		lines = []string{"• None listed"}
	}

	const lineHeight = 16.0
	height := (float64(len(lines)) * lineHeight) + 20
	bottom := top - height

	writeFillColor(content, 0.99, 0.99, 1)
	drawFilledRect(content, pdfPageMargin, bottom, pdfContentWidth, height)
	drawStrokeRect(content, pdfPageMargin, bottom, pdfContentWidth, height, 0.8, 0.85, 0.88, 0.91)

	currentY := top - 18.0
	for _, line := range lines {
		writeText(content, "/F1", 11, 0.17, 0.2, 0.25, pdfPageMargin+18, currentY, line)
		currentY -= lineHeight
	}

	return bottom
}

func drawParagraphBox(content *strings.Builder, top float64, notes string) float64 {
	lines := wrapText(notes, 72)
	if len(lines) == 0 {
		lines = []string{"No additional notes provided."}
	}

	const lineHeight = 15.0
	height := (float64(len(lines)) * lineHeight) + 22
	minBottom := pdfFooterHeight + 18
	bottom := top - height
	if bottom < minBottom {
		bottom = minBottom
		height = top - bottom
	}

	writeFillColor(content, 1, 1, 1)
	drawFilledRect(content, pdfPageMargin, bottom, pdfContentWidth, height)
	drawStrokeRect(content, pdfPageMargin, bottom, pdfContentWidth, height, 0.8, 0.85, 0.88, 0.91)

	currentY := top - 18.0
	for _, line := range lines {
		if currentY < bottom+14 {
			break
		}
		writeText(content, "/F1", 10.5, 0.2, 0.23, 0.27, pdfPageMargin+18, currentY, line)
		currentY -= lineHeight
	}

	return bottom
}

func drawSignatureFooter(content *strings.Builder, imageWidth, imageHeight int) {
	footerY := 34.0
	leftWidth := pdfContentWidth - pdfSignatureArea - 12

	drawLine(content, pdfPageMargin, pdfFooterHeight, pdfPageMargin+pdfContentWidth, pdfFooterHeight, 1.0, 0.84, 0.87, 0.9)
	writeText(content, "/F2", 12, 0.09, 0.21, 0.34, pdfPageMargin, footerY+62, "Acceptance Confirmation")
	writeText(content, "/F1", 10, 0.36, 0.42, 0.49, pdfPageMargin, footerY+44, "Customer and service details above were acknowledged at the time of signature.")
	writeText(content, "/F1", 10, 0.36, 0.42, 0.49, pdfPageMargin, footerY+28, "This document is intended to be clear, readable, and ready for sharing or archiving.")

	sigX := pdfPageMargin + leftWidth + 12
	writeFillColor(content, 0.97, 0.98, 0.99)
	drawFilledRect(content, sigX, footerY, pdfSignatureArea, 82)
	drawStrokeRect(content, sigX, footerY, pdfSignatureArea, 82, 0.8, 0.85, 0.88, 0.91)
	writeText(content, "/F1", 9, 0.38, 0.45, 0.53, sigX+16, footerY+64, "SIGNATURE")
	if imageWidth == 0 || imageHeight == 0 {
		writeText(content, "/F1", 10, 0.45, 0.48, 0.52, sigX+16, footerY+38, "Signature unavailable")
	}
	drawLine(content, sigX+16, footerY+18, sigX+pdfSignatureArea-16, footerY+18, 0.8, 0.74, 0.78, 0.83)
}

func drawFilledRect(content *strings.Builder, x, y, width, height float64) {
	content.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re f\n", x, y, width, height))
}

func drawStrokeRect(content *strings.Builder, x, y, width, height, lineWidth, r, g, b float64) {
	content.WriteString(fmt.Sprintf("%.2f %.2f %.2f RG %.2f w %.2f %.2f %.2f %.2f re S\n", r, g, b, lineWidth, x, y, width, height))
}

func drawLine(content *strings.Builder, x1, y1, x2, y2, lineWidth, r, g, b float64) {
	content.WriteString(fmt.Sprintf("%.2f %.2f %.2f RG %.2f w %.2f %.2f m %.2f %.2f l S\n", r, g, b, lineWidth, x1, y1, x2, y2))
}

func writeText(content *strings.Builder, font string, size, r, g, b, x, y float64, text string) {
	content.WriteString(fmt.Sprintf("BT %.2f %.2f %.2f rg %s %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", r, g, b, font, size, x, y, escapePDFText(text)))
}

func writeFillColor(content *strings.Builder, r, g, b float64) {
	content.WriteString(fmt.Sprintf("%.2f %.2f %.2f rg\n", r, g, b))
}

func drawRoundedBadge(content *strings.Builder, x, y, width, height, r, g, b float64) {
	writeFillColor(content, r, g, b)
	drawFilledRect(content, x, y, width, height)
}

func signatureImageObject(raw string) ([]byte, int, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, 0, 0, ErrInvalidInput
	}

	if strings.HasPrefix(raw, "data:") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) != 2 {
			return nil, 0, 0, ErrInvalidInput
		}
		raw = parts[1]
	}

	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, 0, 0, ErrInvalidInput
	}

	img, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return nil, 0, 0, ErrInvalidInput
	}

	bounds := img.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, bounds, img, bounds.Min, draw.Over)

	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, canvas, &jpeg.Options{Quality: 85}); err != nil {
		return nil, 0, 0, err
	}

	imageObject := []byte(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", bounds.Dx(), bounds.Dy(), jpegBytes.Len()))
	imageObject = append(imageObject, jpegBytes.Bytes()...)
	imageObject = append(imageObject, []byte("\nendstream")...)

	return imageObject, bounds.Dx(), bounds.Dy(), nil
}

func streamObject(content []byte) []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "<< /Length %d >>\nstream\n", len(content))
	out.Write(content)
	out.WriteString("endstream")
	return out.Bytes()
}

func escapePDFText(input string) string {
	input = strings.TrimSpace(input)
	var out strings.Builder
	for _, r := range input {
		switch {
		case r == '\\' || r == '(' || r == ')':
			out.WriteByte('\\')
			out.WriteRune(r)
		case r == '\n' || r == '\r' || r == '\t':
			out.WriteByte(' ')
		case r == '•':
			out.WriteString("\\225")
		case r < 32 || r > 126:
			out.WriteByte('-')
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func wrapText(input string, maxLen int) []string {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return nil
	}

	lines := make([]string, 0, len(fields))
	current := fields[0]
	for _, field := range fields[1:] {
		if len(current)+1+len(field) <= maxLen {
			current += " " + field
			continue
		}
		lines = append(lines, current)
		current = field
	}
	lines = append(lines, current)
	return lines
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func defaultValue(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "Not provided"
	}
	return trimmed
}

func defaultNotes(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "No additional notes provided."
	}
	return trimmed
}

func sanitizedProducts(products []string) []string {
	out := make([]string, 0, len(products))
	for _, product := range products {
		if trimmed := strings.TrimSpace(product); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func highlightValue(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "Not provided"
	}
	return "Expires: " + trimmed
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func init() {
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("jpg", "jpg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "gif", gif.Decode, gif.DecodeConfig)
}
