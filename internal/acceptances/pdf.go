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

	var imageID int
	if imageObject != nil {
		imageID = addObject(imageObject)
	}

	contentID := addObject(streamObject([]byte(content)))

	pageResources := fmt.Sprintf("<< /Font << /F1 %d 0 R >>", fontID)
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
	lines := make([]string, 0, 32)
	lines = append(lines, "Tecora Work Acceptance")
	lines = append(lines, "")
	lines = append(lines, "Acceptance ID: "+record.ID)
	lines = append(lines, "Work Order ID: "+record.WorkOrderID)
	lines = append(lines, "Customer Name: "+record.CustomerName)
	lines = append(lines, "Customer Email: "+record.CustomerEmail)
	lines = append(lines, "Service Date: "+record.ServiceDate)
	lines = append(lines, "Service Expiration Date: "+record.ServiceExpirationDate)
	lines = append(lines, "Service Type: "+record.ServiceType)
	lines = append(lines, "Approved: "+yesNo(record.Approved))
	lines = append(lines, "Signed By Technician ID: "+record.SignedByTechnicianID)
	lines = append(lines, "Signed At (UTC): "+record.SignedAt.UTC().Format(time.RFC3339))
	lines = append(lines, "")
	lines = append(lines, "Products:")
	for _, product := range record.Products {
		lines = append(lines, "  - "+product)
	}
	lines = append(lines, "")
	lines = append(lines, "Notes:")
	if strings.TrimSpace(record.Notes) == "" {
		lines = append(lines, "  (none)")
	} else {
		for _, wrapped := range wrapText(record.Notes, 78) {
			lines = append(lines, "  "+wrapped)
		}
	}

	var content strings.Builder
	y := 760
	for i, line := range lines {
		fontSize := 12
		if i == 0 {
			fontSize = 18
		}
		if line == "" {
			y -= 8
			continue
		}
		fmt.Fprintf(&content, "BT /F1 %d Tf 50 %d Td (%s) Tj ET\n", fontSize, y, escapePDFText(line))
		y -= 16
	}

	imageObject, imageWidth, imageHeight, err := signatureImageObject(record.SignatureImageBase64)
	if err != nil {
		return "", nil, err
	}

	if imageObject != nil {
		content.WriteString(fmt.Sprintf("BT /F1 12 Tf 50 130 Td (%s) Tj ET\n", escapePDFText("Signature:")))

		const maxWidth = 220.0
		const maxHeight = 80.0
		drawWidth := float64(imageWidth)
		drawHeight := float64(imageHeight)
		scale := min(maxWidth/drawWidth, maxHeight/drawHeight)
		if scale < 1 {
			drawWidth *= scale
			drawHeight *= scale
		}
		content.WriteString(fmt.Sprintf("q %.2f 0 0 %.2f 50 30 cm /Im1 Do Q\n", drawWidth, drawHeight))
	}

	return content.String(), imageObject, nil
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
		case r < 32 || r > 126:
			out.WriteByte('?')
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
