package embedded

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_Placeholder(t *testing.T) {
	assert.ErrorIs(t, validate([]byte(`{}`)), ErrPlaceholderCredentials)
}

func TestValidate_MissingPrivateKey(t *testing.T) {
	assert.ErrorIs(t, validate([]byte(`{"client_email":"x@y.iam.gserviceaccount.com"}`)), ErrPlaceholderCredentials)
}

func TestValidate_MissingClientEmail(t *testing.T) {
	assert.ErrorIs(t, validate([]byte(`{"private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`)), ErrPlaceholderCredentials)
}

func TestValidate_Valid(t *testing.T) {
	data := []byte(`{"type":"service_account","client_email":"x@y.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`)
	assert.NoError(t, validate(data))
}

func TestValidate_InvalidJSON(t *testing.T) {
	err := validate([]byte(`not json`))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrPlaceholderCredentials))
}

// 確保版控中的內嵌憑證為佔位,真實憑證不會被誤 commit。
func TestEmbeddedIsPlaceholderInRepo(t *testing.T) {
	assert.ErrorIs(t, Validate(), ErrPlaceholderCredentials)
}
