package render

import (
	"strings"
	"testing"
)

func TestSanitizesHostileMarkdown(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"<script>alert(1)</script>",
		"<iframe src=\"https://evil.example\"></iframe>",
		"<img src=x onerror=alert(1)>",
		"[click](javascript:alert(1)) [data](data:text/html;base64,PHNjcmlwdD4=) [ok](https://example.com)",
		"![pixel](https://tracker.example/p.gif)",
		"<style>body{display:none}</style><div onclick=\"x()\">raw</div>",
		"*emphasis* and **strong** — “quotes”",
	}, "\n\n")
	r, err := Markdown(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "<iframe", "<img", "onerror", "javascript:", "data:text", "<style", "onclick", "<h1", "tracker.example"} {
		if strings.Contains(strings.ToLower(r.HTML), bad) {
			t.Errorf("output contains %q:\n%s", bad, r.HTML)
		}
	}
	for _, good := range []string{"<h2", "<em>emphasis</em>", "<strong>strong</strong>", `href="https://example.com"`, `rel="nofollow noreferrer"`} {
		if !strings.Contains(r.HTML, good) {
			t.Errorf("output lacks %q:\n%s", good, r.HTML)
		}
	}
}

func TestCountWords(t *testing.T) {
	cases := map[string]int{
		"":                         0,
		"Hello, world!":            2,
		"Don't stop—believing.":    3,
		"It’s a well-known fact.":  4,
		"Pi is 3.14, not 22/7.":    6,
		"naïve café résumé":        3,
		"  spaced\n\nout\twords  ": 3,
		"— … !!! ---":              0,
		"東京に行きます":                  7,
		"Ελληνικά και русский текст": 4,
		"end-of-line - dash":              2,
		"emoji 🎉 doesn't count as a word": 6,
	}
	for in, want := range cases {
		if got := CountWords(in); got != want {
			t.Errorf("CountWords(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestReadSeconds(t *testing.T) {
	for words, want := range map[int]int{0: 60, 1: 60, 230: 60, 231: 120, 2300: 600} {
		if got := ReadSeconds(words); got != want {
			t.Errorf("ReadSeconds(%d) = %d, want %d", words, got, want)
		}
	}
}

func TestSizeLimit(t *testing.T) {
	if _, err := Markdown(strings.Repeat("a", MaxMarkdownBytes+1)); err == nil {
		t.Fatal("oversized body must be rejected")
	}
}
