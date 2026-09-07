package messenger

import (
	"html"
	"regexp"
)

var (
	lineTags    = regexp.MustCompile(`(?i)<br\s*/?>|</(?:p|div)>`)
	strikeOpen  = regexp.MustCompile(`(?i)<(?:s|strike|del)>`)
	strikeClose = regexp.MustCompile(`(?i)</(?:s|strike|del)>`)
	links       = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	formatTags  = regexp.MustCompile(`(?i)</?(?:b|strong|i|em|u|ins|code|pre|p|div|span)(?:\s[^>]*)?>`)
)

// PlainText converts Telegram HTML to readable plain text for platforms that
// do not support rich markup. Entities are decoded last so escaped literal
// angle brackets are not mistaken for tags.
func PlainText(value string) string {
	value = links.ReplaceAllString(value, "$2 ($1)")
	value = lineTags.ReplaceAllString(value, "\n")
	value = strikeOpen.ReplaceAllString(value, "[ранее: ")
	value = strikeClose.ReplaceAllString(value, "]")
	return html.UnescapeString(formatTags.ReplaceAllString(value, ""))
}
