package backend

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestStubApplications_StoresFileAttachments(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	apps := newStubApplications(logger)

	file := schemes.FileAttachment{
		Filename: "test.docx",
		Size:     4096,
		Payload:  schemes.FileAttachmentPayload{Token: "token-123"},
	}

	rawItems := []json.RawMessage{json.RawMessage(mustMarshal(t, file))}
	payloadBytes, err := json.Marshal(rawItems)
	require.NoError(t, err)

	err = apps.SubmitApplication(context.Background(), 123, RoleStudent, ApplicationTypeAcademicLeave, map[string]string{
		"supporting_files": string(payloadBytes),
	})
	require.NoError(t, err)

	stored := apps.StoredFiles(123)
	require.NotNil(t, stored)
	files, ok := stored["supporting_files"]
	require.True(t, ok)
	require.Len(t, files, 1)
	require.Equal(t, "test.docx", files[0].Filename)
	require.Equal(t, int64(4096), files[0].Size)
	require.Equal(t, "token-123", files[0].Payload.Token)
}

func mustMarshal(t testing.TB, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
