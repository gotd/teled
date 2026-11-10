package rpc

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/tg"
)

// tdesktopSchema is the api.tl of the Telegram Desktop build teled is tested
// against (gotd/tdesktop, Telegram/SourceFiles/mtproto/scheme/api.tl). Refresh
// it together with the gotd/td bump when the client moves to a new layer.
const tdesktopSchema = "../../_testdata/tdesktop/api.tl"

var (
	tlConstructor = regexp.MustCompile(`^([a-zA-Z][\w.]*)#([0-9a-f]{1,8})\s`)
	tlLayer       = regexp.MustCompile(`^//\s*LAYER\s+(\d+)`)
)

// TestClientSchemaCompatible checks that every constructor teled can put on the
// wire decodes in the client: a constructor present in both schemas must have
// the same id. A stale gotd/td serializes old-layer constructors (e.g. userFull
// of layer 228), which the client rejects with CLIENT_RESPONSE_PARSE_FAILED.
func TestClientSchemaCompatible(t *testing.T) {
	f, err := os.Open(tdesktopSchema)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	client := map[string]uint32{}
	layer := 0

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if m := tlLayer.FindStringSubmatch(line); m != nil {
			layer, err = strconv.Atoi(m[1])
			require.NoError(t, err)

			continue
		}

		m := tlConstructor.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		id, err := strconv.ParseUint(m[2], 16, 32)
		require.NoError(t, err)

		client[m[1]] = uint32(id)
	}

	require.NoError(t, s.Err())
	require.NotEmpty(t, client)
	require.Equal(t, layer, tg.Layer, "gotd/td layer differs from the client's")

	var mismatched []string

	for id, name := range tg.TypesMap() {
		name, _, _ = strings.Cut(name, "#")

		want, ok := client[name]
		if ok && want != id {
			mismatched = append(mismatched, name+": teled #"+strconv.FormatUint(uint64(id), 16)+
				", client #"+strconv.FormatUint(uint64(want), 16))
		}
	}

	require.Empty(t, mismatched, "constructors that the client cannot decode")
}
