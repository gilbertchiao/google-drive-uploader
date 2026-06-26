package embedded

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_Placeholder(t *testing.T) {
	assert.ErrorIs(t, validate([]byte(`{}`)), ErrInvalidCredentials)
}

func TestValidate_MissingPrivateKey(t *testing.T) {
	assert.ErrorIs(t, validate([]byte(`{"client_email":"x@y.iam.gserviceaccount.com"}`)), ErrInvalidCredentials)
}

func TestValidate_MissingClientEmail(t *testing.T) {
	assert.ErrorIs(t, validate([]byte(`{"private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`)), ErrInvalidCredentials)
}

func TestValidate_Valid(t *testing.T) {
	data := []byte(`{"type":"service_account","client_email":"x@y.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`)
	assert.NoError(t, validate(data))
}

func TestValidate_InvalidJSON(t *testing.T) {
	err := validate([]byte(`not json`))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrInvalidCredentials))
}

// 確保版控中的內嵌憑證為佔位,真實憑證不會被誤 commit。
func TestEmbeddedIsPlaceholderInRepo(t *testing.T) {
	assert.ErrorIs(t, Validate(), ErrInvalidCredentials)
}

// CredentialsJSON 應回傳複本,呼叫端竄改不可影響內部狀態或後續呼叫。
func TestCredentialsJSON_ReturnsCopy(t *testing.T) {
	first := CredentialsJSON()
	require.NotEmpty(t, first)
	for i := range first {
		first[i] = 'X'
	}
	second := CredentialsJSON()
	assert.False(t, bytes.Equal(first, second), "竄改回傳值不應影響後續呼叫")
	assert.Equal(t, []byte(`{}`), bytes.TrimSpace(second))
}
