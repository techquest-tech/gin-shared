package notify

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringSlice_ValueAndScan(t *testing.T) {
	in := StringSlice{"a", "b"}
	v, err := in.Value()
	require.NoError(t, err)

	var raw []string
	require.NoError(t, json.Unmarshal([]byte(v.(string)), &raw))
	require.Equal(t, []string{"a", "b"}, raw)

	var out StringSlice
	require.NoError(t, out.Scan(v))
	require.Equal(t, in, out)

	var out2 StringSlice
	require.NoError(t, out2.Scan([]byte(v.(string))))
	require.Equal(t, in, out2)
}

