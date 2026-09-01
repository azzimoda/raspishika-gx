package botutil

import (
	"html"
	"sort"
	"strings"

	"github.com/go-telegram/bot/models"
)

// EntitiesToHTML converts a message text together with its entities back into
// an HTML string suitable for sending with models.ParseModeHTML. It preserves
// the formatting Telegram has already applied (bold, italic, underline,
// strikethrough, spoiler, blockquote, code, pre and links).
func EntitiesToHTML(text string, entities []models.MessageEntity) string {
	if len(entities) == 0 {
		return html.EscapeString(text)
	}

	type span struct {
		open, close string
	}
	// tagAt[i] collects the tags that open/close at byte offset i.
	opens := make(map[int][]string)
	closes := make(map[int][]string)

	for _, e := range entities {
		o, c := entityTags(e)
		if o == "" {
			continue
		}
		// Guard against out-of-range offsets.
		end := e.Offset + e.Length
		if e.Offset < 0 || end > len(text) {
			continue
		}
		opens[e.Offset] = append(opens[e.Offset], o)
		closes[end] = append(closes[end], c)
	}
	if len(opens) == 0 {
		return html.EscapeString(text)
	}

	offsets := make(map[int]struct{}, len(opens)+len(closes))
	for off := range opens {
		offsets[off] = struct{}{}
	}
	for off := range closes {
		offsets[off] = struct{}{}
	}
	sorted := make([]int, 0, len(offsets))
	for off := range offsets {
		sorted = append(sorted, off)
	}
	sort.Ints(sorted)

	var b strings.Builder
	prev := 0
	for _, off := range sorted {
		// Emit the text segment up to this offset.
		if off > prev {
			b.WriteString(html.EscapeString(text[prev:off]))
		}
		// Close tags first (reverse the open order to keep nesting correct).
		for i := len(closes[off]) - 1; i >= 0; i-- {
			b.WriteString(closes[off][i])
		}
		for _, tag := range opens[off] {
			b.WriteString(tag)
		}
		prev = off
	}
	if prev < len(text) {
		b.WriteString(html.EscapeString(text[prev:]))
	}
	return b.String()
}

// entityTags returns the opening and closing HTML tag pair for an entity, or
// ("", "") if the entity has no HTML representation.
func entityTags(e models.MessageEntity) (open, close string) {
	switch e.Type {
	case models.MessageEntityTypeBold:
		return "<b>", "</b>"
	case models.MessageEntityTypeItalic:
		return "<i>", "</i>"
	case models.MessageEntityTypeUnderline:
		return "<u>", "</u>"
	case models.MessageEntityTypeStrikethrough:
		return "<s>", "</s>"
	case models.MessageEntityTypeSpoiler:
		return `<span class="tg-spoiler">`, "</span>"
	case models.MessageEntityTypeCode:
		return "<code>", "</code>"
	case models.MessageEntityTypePre:
		return "<pre>", "</pre>"
	case models.MessageEntityTypeBlockquote:
		return "<blockquote>", "</blockquote>"
	case models.MessageEntityTypeTextLink:
		url := html.EscapeString(e.URL)
		return `<a href="` + url + `">`, "</a>"
	default:
		return "", ""
	}
}
