package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/tg"
)

func TestMessageEntities(t *testing.T) {
	t.Run("url", func(t *testing.T) {
		ents := messageEntities("see https://t.me/fzfkek now")
		require.Len(t, ents, 1)
		u := ents[0].(*tg.MessageEntityURL)
		require.Equal(t, 4, u.Offset)
		require.Equal(t, len("https://t.me/fzfkek"), u.Length)
	})

	t.Run("trailing punctuation trimmed", func(t *testing.T) {
		ents := messageEntities("go to https://example.com).")
		require.Len(t, ents, 1)
		u := ents[0].(*tg.MessageEntityURL)
		require.Equal(t, len("https://example.com"), u.Length)
	})

	t.Run("mention and hashtag", func(t *testing.T) {
		ents := messageEntities("@ada uses #golang")
		require.Len(t, ents, 2)
		require.IsType(t, &tg.MessageEntityMention{}, ents[0])
		require.IsType(t, &tg.MessageEntityHashtag{}, ents[1])
	})

	t.Run("utf16 offsets", func(t *testing.T) {
		// The emoji is one rune but two UTF-16 code units; the URL offset must
		// account for that.
		ents := messageEntities("😀 https://x.com")
		require.Len(t, ents, 1)
		require.Equal(t, 3, ents[0].(*tg.MessageEntityURL).Offset) // 2 (emoji) + 1 (space)
	})

	t.Run("plain text has no entities", func(t *testing.T) {
		require.Empty(t, messageEntities("just some words"))
	})
}
