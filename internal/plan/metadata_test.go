package plan_test

import (
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataRoundTripAndInvalidVersions(t *testing.T) {
	t.Parallel()
	want := plan.Metadata{Version: 1, ID: "add-sso", ImplementationRepository: "acme/payments"}
	body, err := want.Body()
	require.NoError(t, err)
	got, err := plan.ParseMetadata("Human introduction.\n" + body + "\nHuman notes.")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	got, err = plan.ParseMetadata(strings.ReplaceAll(body, "\n", "\r\n"))
	require.NoError(t, err)
	assert.Equal(t, want, got)
	for _, invalid := range []string{
		"no metadata", body + body, strings.TrimSuffix(body, "-->\n"),
		strings.Replace(body, "version: 1", "version: 2", 1),
		strings.Replace(body, "version: 1", "version: 1.5", 1),
		strings.Replace(body, "version: 1", "version: 1\nunknown: field", 1),
		strings.Replace(body, "id: add-sso", "id: ''", 1),
		strings.Replace(body, "-->", "---\nversion: 1\n-->", 1),
	} {
		_, err := plan.ParseMetadata(invalid)
		assert.Error(t, err, "body: %s", invalid)
	}
}
