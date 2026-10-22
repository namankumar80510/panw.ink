package seed

import "strings"

// sentences is a small pool of original synthetic prose, written for these
// fixtures. Chapters are assembled deterministically from it.
var sentences = []string{
	"The lamps along the canal had been lit for an hour, and still the water refused to reflect them.",
	"She counted the steps to the archive door twice, because the first number had seemed wrong.",
	"Somewhere above, a bell rang once and then thought better of it.",
	"“You’re late,” said the keeper, without looking up from a ledger that had no ink in it.",
	"Dust lay on the shelves in the patient way of things that expect to outlive their owners.",
	"The book was warmer than it should have been, as if someone had only just put it down.",
	"Rain moved across the roofs like a rumour, quick and impossible to pin to any one place.",
	"He had promised himself he would not open the drawer, and he kept that promise for almost a minute.",
	"Every map in the building agreed on the corridors and disagreed, quietly, about the doors.",
	"There are debts you pay in coin, the old woman had said, and debts that pay themselves in you.",
	"The clock in the hall had stopped at seven, which was either a warning or a habit.",
	"Outside, the city went on with its business, unaware that one of its streets had gone missing.",
	"Mira pressed her palm flat against the wood and felt, faintly, a second heartbeat answer.",
	"Nobody had used the stairs to the third floor in twenty years, yet the banister was polished.",
	"The letter was addressed to her in her own handwriting, dated a week from now.",
	"It is easier to lock a room than to persuade a room to stay locked.",
	"The candle leaned toward the window, though there was no draught to make it.",
	"She wrote the date in the margin, crossed it out, and wrote it again more carefully.",
	"For a moment the corridor was longer than the house that contained it.",
	"“Some things are catalogued,” the keeper said, “and some things catalogue you.”",
	"The ink had dried in the shape of a word she did not yet know.",
	"Far below, the drowned bells of the old quarter began, very softly, to ring the hour.",
	"He laughed, and the laugh came back to him from the wrong end of the room.",
	"By morning the footprints in the dust pointed in both directions at once.",
}

// Prose returns deterministic Markdown: a short epigraph, then n paragraphs
// of four to six sentences, with a scene break in the middle.
func Prose(_ string, n, salt int) string {
	var b strings.Builder
	b.WriteString("*" + sentences[(salt*7)%len(sentences)] + "*\n\n")
	// Strides coprime with the pool size visit every sentence before repeating.
	strides := []int{5, 7, 11, 13, 17, 19}
	stride := strides[salt%len(strides)]
	k := salt * 5
	for p := 0; p < n; p++ {
		if p == n/2 && n > 3 {
			b.WriteString("---\n\n")
		}
		count := 4 + (p+salt)%3
		for s := 0; s < count; s++ {
			if s > 0 {
				b.WriteString(" ")
			}
			b.WriteString(sentences[k%len(sentences)])
			k += stride
		}
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
