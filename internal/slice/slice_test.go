package slice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunk(t *testing.T) {
	require.Equal(t,
		Chunk([]string{"a", "b"}, 2),
		[][]string{{"a", "b"}},
	)
	require.Equal(t,
		Chunk([]string{"a", "b", "c"}, 2),
		[][]string{{"a", "b"}, {"c"}},
	)
	require.Equal(t,
		0,
		len(Chunk([]string(nil), 2)),
	)
}
