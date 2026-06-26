package uploader

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

// fakeAPI 是 driveAPI 的測試替身,記錄被呼叫的方法與參數。
type fakeAPI struct {
	findResult []*drive.File
	findErr    error

	createCalled  bool
	updateCalled  bool
	createdName   string
	createdFolder string
	updatedID     string

	opErr error // create/update 要回傳的錯誤
}

func (f *fakeAPI) findByName(_ context.Context, _, _ string) ([]*drive.File, error) {
	return f.findResult, f.findErr
}

func (f *fakeAPI) create(_ context.Context, name, folderID string, _ io.Reader, _ int64) (*drive.File, error) {
	f.createCalled = true
	f.createdName = name
	f.createdFolder = folderID
	if f.opErr != nil {
		return nil, f.opErr
	}
	return &drive.File{Id: "new-id", Name: name}, nil
}

func (f *fakeAPI) update(_ context.Context, fileID string, _ io.Reader, _ int64) (*drive.File, error) {
	f.updateCalled = true
	f.updatedID = fileID
	if f.opErr != nil {
		return nil, f.opErr
	}
	return &drive.File{Id: fileID, Name: "x"}, nil
}

func newTestUploader(api driveAPI) *Uploader {
	return &Uploader{
		api:      api,
		folderID: "FID",
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func tempFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))
	return p
}

func TestUpload_CreatesWhenAbsent(t *testing.T) {
	api := &fakeAPI{findResult: nil}
	u := newTestUploader(api)

	got, err := u.Upload(context.Background(), tempFile(t, "app.log"), "app.log")

	require.NoError(t, err)
	assert.True(t, api.createCalled)
	assert.False(t, api.updateCalled)
	assert.Equal(t, "FID", api.createdFolder)
	assert.Equal(t, "new-id", got.Id)
}

func TestUpload_UpdatesWhenPresent(t *testing.T) {
	api := &fakeAPI{findResult: []*drive.File{{Id: "old-id", Name: "app.log"}}}
	u := newTestUploader(api)

	_, err := u.Upload(context.Background(), tempFile(t, "app.log"), "app.log")

	require.NoError(t, err)
	assert.True(t, api.updateCalled)
	assert.False(t, api.createCalled)
	assert.Equal(t, "old-id", api.updatedID)
}

func TestUpload_MultipleMatchesUpdatesFirst(t *testing.T) {
	api := &fakeAPI{findResult: []*drive.File{{Id: "first"}, {Id: "second"}}}
	u := newTestUploader(api)

	_, err := u.Upload(context.Background(), tempFile(t, "app.log"), "app.log")

	require.NoError(t, err)
	assert.Equal(t, "first", api.updatedID)
}

func TestUpload_DefaultRemoteName(t *testing.T) {
	api := &fakeAPI{}
	u := newTestUploader(api)

	_, err := u.Upload(context.Background(), tempFile(t, "server.log"), "")

	require.NoError(t, err)
	assert.Equal(t, "server.log", api.createdName)
}

func TestUpload_MissingFile(t *testing.T) {
	u := newTestUploader(&fakeAPI{})

	_, err := u.Upload(context.Background(), "/no/such/file.log", "x")

	require.Error(t, err)
}

func TestUpload_RejectsDirectory(t *testing.T) {
	u := newTestUploader(&fakeAPI{})

	_, err := u.Upload(context.Background(), t.TempDir(), "x")

	require.Error(t, err)
}

func TestUpload_FindErrorPropagates(t *testing.T) {
	api := &fakeAPI{findErr: assertErr("boom")}
	u := newTestUploader(api)

	_, err := u.Upload(context.Background(), tempFile(t, "app.log"), "app.log")

	require.Error(t, err)
	assert.False(t, api.createCalled)
	assert.False(t, api.updateCalled)
}

func TestEscapeQueryValue(t *testing.T) {
	assert.Equal(t, `a\'b`, escapeQueryValue(`a'b`))
	assert.Equal(t, `a\\b`, escapeQueryValue(`a\b`))
	assert.Equal(t, `plain.log`, escapeQueryValue(`plain.log`))
}

func TestWrapQuotaError(t *testing.T) {
	gErr := &googleapi.Error{
		Code:   403,
		Errors: []googleapi.ErrorItem{{Reason: "storageQuotaExceeded"}},
	}
	out := wrapQuotaError(gErr)
	assert.Contains(t, out.Error(), "Shared Drive")
}

func TestWrapQuotaError_OtherErrorUnchanged(t *testing.T) {
	in := assertErr("some other error")
	assert.Equal(t, in, wrapQuotaError(in))
}

// progress callback 頻繁觸發時應節流:每 10% 或完成才記一次,不應每次都記。
func TestProgress_Throttled(t *testing.T) {
	var buf bytes.Buffer
	d := &driveService{log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))}

	p := d.progress(100)
	for i := int64(0); i <= 100; i++ {
		p(i, 100)
	}

	lines := strings.Count(buf.String(), "上傳進度")
	assert.LessOrEqual(t, lines, 12, "101 次 callback 不應產生超過 ~12 條 log")
	assert.GreaterOrEqual(t, lines, 10)
}

// assertErr 是測試用的簡單 error。
type assertErr string

func (e assertErr) Error() string { return string(e) }
