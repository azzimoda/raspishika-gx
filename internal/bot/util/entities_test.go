package botutil

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestEntitiesToHTML_NoEntities(t *testing.T) {
	got := EntitiesToHTML("Hello <world> & friends", nil)
	want := "Hello &lt;world&gt; &amp; friends"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEntitiesToHTML_Bold(t *testing.T) {
	entities := []models.MessageEntity{
		{Type: models.MessageEntityTypeBold, Offset: 0, Length: 5},
	}
	got := EntitiesToHTML("Hello", entities)
	want := "<b>Hello</b>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEntitiesToHTML_BoldAndItalicNested(t *testing.T) {
	entities := []models.MessageEntity{
		{Type: models.MessageEntityTypeBold, Offset: 0, Length: 10},
		{Type: models.MessageEntityTypeItalic, Offset: 2, Length: 6},
	}
	got := EntitiesToHTML("abcdefghij", entities)
	want := "<b>ab<i>cdefgh</i>ij</b>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEntitiesToHTML_Link(t *testing.T) {
	entities := []models.MessageEntity{
		{Type: models.MessageEntityTypeTextLink, Offset: 0, Length: 4, URL: "https://example.com?a=1&b=2"},
	}
	got := EntitiesToHTML("site", entities)
	want := `<a href="https://example.com?a=1&amp;b=2">site</a>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEntitiesToHTML_RawHTMLNotInjected(t *testing.T) {
	// Raw HTML inside the text must be escaped, not passed through.
	entities := []models.MessageEntity{
		{Type: models.MessageEntityTypeBold, Offset: 0, Length: 18},
	}
	got := EntitiesToHTML("<script>x</script>", entities)
	want := "<b>&lt;script&gt;x&lt;/script&gt;</b>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEntitiesToHTML_SameOffsetNesting(t *testing.T) {
	// Bold and italic applied to the exact same range; tags must nest LIFO.
	entities := []models.MessageEntity{
		{Type: models.MessageEntityTypeBold, Offset: 0, Length: 2},
		{Type: models.MessageEntityTypeItalic, Offset: 0, Length: 2},
	}
	got := EntitiesToHTML("hi", entities)
	// Both orderings yield valid containment; assert balanced and both present.
	if got != "<b><i>hi</i></b>" && got != "<i><b>hi</b></i>" {
		t.Fatalf("got %q, want LIFO-nested bold+italic", got)
	}
}

func TestEntitiesToHTML_OutOfRangeIgnored(t *testing.T) {
	entities := []models.MessageEntity{
		{Type: models.MessageEntityTypeBold, Offset: 0, Length: 100}, // exceeds text length
	}
	got := EntitiesToHTML("Hi", entities)
	want := "Hi"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
