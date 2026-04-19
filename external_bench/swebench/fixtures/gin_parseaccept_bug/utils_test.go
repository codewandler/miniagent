package gin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAccept(t *testing.T) {
	parts := parseAccept("text/html , application/xhtml+xml,application/xml;q=0.9,  */* ;q=0.8")
	assert.Len(t, parts, 4)
	assert.Equal(t, "text/html", parts[0])
	assert.Equal(t, "application/xhtml+xml", parts[1])
	assert.Equal(t, "application/xml", parts[2])
	assert.Equal(t, "*/*", parts[3])
}
