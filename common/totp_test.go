package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateBackupCodeFormat(t *testing.T) {
	for range 500 {
		code, err := generateRandomBackupCode()
		require.NoError(t, err)
		require.Len(t, code, BackupCodeLength+1) // XXXX-XXXX
		require.Equal(t, "-", code[4:5])
		require.True(t, ValidateBackupCode(code))
		clean := strings.ReplaceAll(code, "-", "")
		for i := range clean {
			c := clean[i]
			ok := (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
			require.Truef(t, ok, "unexpected char %q in backup code", c)
		}
	}
}
