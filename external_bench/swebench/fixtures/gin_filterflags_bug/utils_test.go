package gin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterFlags(t *testing.T) {
	result := filterFlags("text/html ")
	assert.Equal(t, "text/html", result)

	result = filterFlags("text/html;")
	assert.Equal(t, "text/html", result)
}
