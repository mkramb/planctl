package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithCauseSurfacesUnderlyingError(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "message", withCause("message", nil))
	assert.Equal(t, "message: boom", withCause("message", errors.New("boom")))
}
