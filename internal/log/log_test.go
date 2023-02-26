package log

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestParser(t *testing.T) {
	require.Equal(t,
		logrus.Fields(map[string]any{
			"error": "err",
		}),
		parse("error", "err"),
	)

}
