package messenger

import "testing"

func TestPlainText(t *testing.T) {
	value := `<b>Пара &amp; группа</b><br><s>101</s> <i>202</i> &lt;literal&gt; <a href="https://example.org?a=1&amp;b=2">Источник</a>`
	want := "Пара & группа\n[ранее: 101] 202 <literal> Источник (https://example.org?a=1&b=2)"
	if got := PlainText(value); got != want {
		t.Fatalf("PlainText = %q, want %q", got, want)
	}
}
