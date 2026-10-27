package logos

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
)

const (
	contentTypeSVG = "image/svg+xml"
	svgSniffWindow = 1024
)

var sniffedImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

var svgPrologue = regexp.MustCompile(`(?s)^\x{feff}?\s*(<\?xml[^>]*\?>\s*)?(<!--.*?-->\s*|<!doctype[^>]*>\s*)*<svg[\s>]`)

func sniffImage(body []byte) (string, bool) {
	detected := http.DetectContentType(body)
	if sniffedImageTypes[detected] {
		return detected, true
	}
	if isSVG(body, detected) {
		return contentTypeSVG, true
	}
	return "", false
}

func isSVG(body []byte, detected string) bool {
	if !strings.HasPrefix(detected, "text/xml") && !strings.HasPrefix(detected, "text/plain") {
		return false
	}
	head := body[:min(len(body), svgSniffWindow)]
	return svgPrologue.Match(bytes.ToLower(head))
}
