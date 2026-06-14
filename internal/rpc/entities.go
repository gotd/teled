package rpc

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gotd/td/tg"
)

// Telegram auto-detects message entities (clickable URLs, mentions, hashtags)
// from plain text and returns them so clients can highlight them. teled does the
// same so links like "https://t.me/..." render as links rather than plain text.
var (
	entityURLRe     = regexp.MustCompile(`(?i)(?:https?://|www\.)\S+`)
	entityMentionRe = regexp.MustCompile(`@[A-Za-z0-9_]{1,32}`)
	entityHashtagRe = regexp.MustCompile(`#[\p{L}0-9_]+`)
)

// urlTrailing is the set of characters trimmed from the end of a detected URL,
// so trailing sentence punctuation is not swallowed into the link.
const urlTrailing = ".,!?;:)]}'\"»…"

// messageEntities auto-detects URLs, mentions and hashtags in text and returns
// them as Telegram message entities. Offsets and lengths are in UTF-16 code
// units, as the MTProto protocol requires.
func messageEntities(text string) []tg.MessageEntityClass {
	type span struct {
		start, end int
		kind       string
	}

	var spans []span

	add := func(locs [][]int, kind string, trim bool) {
		for _, loc := range locs {
			s, e := loc[0], loc[1]
			if trim {
				for e > s {
					r, size := utf8.DecodeLastRuneInString(text[s:e])
					if !strings.ContainsRune(urlTrailing, r) {
						break
					}

					e -= size
				}
			}

			if e > s {
				spans = append(spans, span{s, e, kind})
			}
		}
	}

	add(entityURLRe.FindAllStringIndex(text, -1), "url", true)
	add(entityMentionRe.FindAllStringIndex(text, -1), "mention", false)
	add(entityHashtagRe.FindAllStringIndex(text, -1), "hashtag", false)

	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var (
		out     []tg.MessageEntityClass
		lastEnd int
	)

	for _, sp := range spans {
		// Skip spans overlapping an already-emitted one (first match wins).
		if sp.start < lastEnd {
			continue
		}

		offset := utf16Len(text[:sp.start])
		length := utf16Len(text[sp.start:sp.end])

		switch sp.kind {
		case "url":
			out = append(out, &tg.MessageEntityURL{Offset: offset, Length: length})
		case "mention":
			out = append(out, &tg.MessageEntityMention{Offset: offset, Length: length})
		case "hashtag":
			out = append(out, &tg.MessageEntityHashtag{Offset: offset, Length: length})
		}

		lastEnd = sp.end
	}

	return out
}

// utf16Len returns the number of UTF-16 code units in s. Telegram entity offsets
// and lengths are measured in UTF-16, not bytes or runes.
func utf16Len(s string) int {
	n := 0

	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}

	return n
}
