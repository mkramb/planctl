package plan_test

import (
	"strings"
	"testing"

	"github.com/mkramb/planctl/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlug(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ title, slug string }{
		{"Add SSO", "add-sso"},
		{"  Add---SSO!  ", "add-sso"},
		{"OAuth 2.0 / Login", "oauth-2-0-login"},
		{"Prijava Članov", "prijava-članov"},
		{"登录功能", "登录功能"},
	} {
		t.Run(tt.title, func(t *testing.T) {
			slug, err := plan.Slug(tt.title)
			require.NoError(t, err)
			assert.Equal(t, tt.slug, slug)
		})
	}
	for _, title := range []string{"", "!!!", "  ", "CON", "nul", "COM1", "lpt9", strings.Repeat("a", 101)} {
		_, err := plan.Slug(title)
		var failure *plan.Error
		require.ErrorAs(t, err, &failure, "title: %s", title)
		assert.Equal(t, "invalid_title", failure.Code)
	}
}
