package shared

import textclean "tool-gateway/internal/textclean"

func CleanVisibleOutput(text string, stripReferenceMarkers bool) string {
	if text == "" {
		return text
	}
	if stripReferenceMarkers {
		text = textclean.StripReferenceMarkers(text)
	}
	return sanitizeLeakedOutput(text)
}
