// Package render turns entry Markdown (the single source of truth) into
// sanitized HTML at write time (IMPLEMENTATION.md §1, §11.1 step 5, §13.2
// item 4). Goldmark never emits raw HTML (no WithUnsafe), and bluemonday then
// enforces an allowlist: prose elements only, links limited to
// http/https/mailto with rel="nofollow noopener noreferrer", no images,
// scripts, frames, styles or event handlers.
package render

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// MaxMarkdownBytes bounds one entry body (§5.2: 2 MB UTF-8).
const MaxMarkdownBytes = 2 << 20

// WordsPerMinute is the reading speed for estimates (§11.2).
const WordsPerMinute = 230

var md = goldmark.New(
	goldmark.WithExtensions(extension.Strikethrough, extension.Typographer),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

var policy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "hr", "em", "strong", "del", "s", "blockquote",
		"ul", "ol", "li", "h2", "h3", "h4", "h5", "h6", "code", "pre", "sup", "sub")
	p.AllowAttrs("start").Matching(regexp.MustCompile(`^[0-9]{1,6}$`)).OnElements("ol")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(false)
	return p
}()

// Result is a rendered revision.
type Result struct {
	HTML        string
	PlainText   string
	WordCount   int
	ReadSeconds int
	Checksum    []byte // SHA-256 of the Markdown
}

// Markdown renders and sanitizes src. An "# H1" in a body is demoted to h2,
// since the page title is the entry's h1.
func Markdown(src string) (*Result, error) {
	if len(src) > MaxMarkdownBytes {
		return nil, fmt.Errorf("render: body exceeds %d bytes", MaxMarkdownBytes)
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	out := strings.ReplaceAll(buf.String(), "<h1", "<h2")
	out = strings.ReplaceAll(out, "</h1>", "</h2>")
	safe := policy.Sanitize(out)
	plain := PlainText(safe)
	words := CountWords(plain)
	sum := sha256.Sum256([]byte(src))
	return &Result{HTML: safe, PlainText: plain, WordCount: words, ReadSeconds: ReadSeconds(words), Checksum: sum[:]}, nil
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// PlainText strips tags from sanitized HTML and unescapes entities.
func PlainText(safeHTML string) string {
	s := tagRE.ReplaceAllString(strings.NewReplacer("</p>", "\n\n", "<br/>", "\n", "<br>", "\n").Replace(safeHTML), " ")
	return strings.TrimSpace(html.UnescapeString(s))
}

// ReadSeconds estimates reading time: ceil(words / 230) minutes, minimum one
// minute (§11.2).
func ReadSeconds(words int) int {
	mins := (words + WordsPerMinute - 1) / WordsPerMinute
	return max(mins, 1) * 60
}

// CountWords counts Unicode words: runs of letters/digits/marks, joined by
// apostrophes, hyphens or periods between word characters (so "don't",
// "well-known", "3.14" and "naïve" are one word each). Ideographs (CJK) count
// one word per character, since those scripts do not separate words with
// spaces. Bytes and characters are never used as words.
func CountWords(s string) int {
	n := 0
	in := false
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			n++
			in = false
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
			if !in {
				n++
				in = true
			}
		case in && isJoiner(r) && i+1 < len(runes) && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])):
			// stay inside the word
		default:
			in = false
		}
	}
	return n
}

func isJoiner(r rune) bool {
	return r == '\'' || r == '’' || r == '-' || r == '‐' || r == '.' || r == '_'
}
