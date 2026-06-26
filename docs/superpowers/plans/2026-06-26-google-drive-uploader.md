# Google Drive Uploader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 Go 寫一個 CLI 工具，build 後以 `gdrive-upload [filename]` 把檔案(300–500MB log)上傳到 build 時固定的 Google Drive 資料夾(同名覆蓋)。

**Architecture:** 三層 — `internal/embedded`(go:embed 內嵌 SA 憑證 + 驗證)、`internal/uploader`(Drive client、find→create/update、resumable 上傳、配額錯誤處理)、`cmd/gdrive-upload`(CLI、slog、ldflags 注入 folderID/subject)。Makefile 於 build 時注入真實憑證並在結束後還原佔位檔。

**Tech Stack:** Go 1.22、`google.golang.org/api/drive/v3`、`golang.org/x/oauth2/google`、`log/slog`、`testing` + `testify`。

## Global Constraints

- Go module path: `github.com/gilbertchiao/google-drive-uploader`，Go 1.22。
- 註解、文件用繁體中文，技術術語用英文。
- 顯式錯誤處理，向上以 `fmt.Errorf("...: %w", err)` 包裝。
- 日誌一律用標準庫 `log/slog`(輸出 stderr)。
- OAuth scope 用 `drive.file`(最小權限)。
- resumable upload chunk size 固定 16MB。
- `internal/embedded/credentials.json` 永遠以佔位 `{}` 進版控；真實憑證僅 build 時暫存、結束即還原。
- 每階段為獨立分支 → PR → Copilot review → 處理 comment → merge → 進下一階段。

## 分支與 PR 對應

| Stage | 分支 | 內容 |
| --- | --- | --- |
| 0 | `main`(直接 commit) | 設計文件、實作計劃、scaffolding |
| 1 | `feat/embedded-credentials` | `internal/embedded` 套件 + 測試 |
| 2 | `feat/uploader` | `internal/uploader` 套件 + 測試 |
| 3 | `feat/cli-and-makefile` | `cmd/gdrive-upload` + Makefile + README |

---

## Stage 0：Scaffolding(直接 commit 到 main)

**Files:**
- Create: `go.mod`、`.gitignore`、`README.md`、`docs/superpowers/specs/...`、`docs/superpowers/plans/...`

- [ ] `go mod init github.com/gilbertchiao/google-drive-uploader`
- [ ] 寫 `.gitignore`(見下)、`README.md` skeleton
- [ ] commit「chore: 初始化專案與設計文件」並 push 到 main

`.gitignore`:
```gitignore
# Binaries
/bin/
gdrive-upload

# Working/Temporary directories
work/
temp/
tmp/

# Go
*.test
*.out
*.prof

# 注意:internal/embedded/credentials.json 以佔位 {} 進版控,不可在此忽略。
# 真實 Service Account 憑證請勿 commit(Makefile build 後會自動還原佔位)。
```

---

## Stage 1：internal/embedded

**Files:**
- Create: `internal/embedded/embedded.go`
- Create: `internal/embedded/credentials.json`(內容 `{}`)
- Test: `internal/embedded/embedded_test.go`

**Interfaces:**
- Produces: `embedded.CredentialsJSON() []byte`、`embedded.Validate() error`、`embedded.ErrPlaceholderCredentials error`

- [ ] **Step 1: 寫 `internal/embedded/credentials.json`**
```json
{}
```

- [ ] **Step 2: 寫失敗測試 `embedded_test.go`**
```go
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

func TestValidate_Valid(t *testing.T) {
	data := []byte(`{"type":"service_account","client_email":"x@y.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`)
	assert.NoError(t, validate(data))
}

func TestValidate_InvalidJSON(t *testing.T) {
	err := validate([]byte(`not json`))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrPlaceholderCredentials))
}

func TestEmbeddedIsPlaceholderInRepo(t *testing.T) {
	assert.ErrorIs(t, Validate(), ErrPlaceholderCredentials)
}
```

- [ ] **Step 3: 跑測試確認失敗**：`go test ./internal/embedded/`(預期編譯失敗:未定義)

- [ ] **Step 4: 寫實作 `embedded.go`**
```go
// Package embedded 提供 build 時內嵌的 Service Account 憑證。
package embedded

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

//go:embed credentials.json
var credentialsJSON []byte

// ErrPlaceholderCredentials 表示內嵌的是佔位憑證,尚未以 make build 注入真實憑證。
var ErrPlaceholderCredentials = errors.New("內嵌的 credentials 為佔位內容,請用 make build 注入真實 Service Account 憑證")

// CredentialsJSON 回傳內嵌的憑證 bytes。
func CredentialsJSON() []byte { return credentialsJSON }

// Validate 檢查內嵌憑證是否為有效的 Service Account 憑證(而非佔位 {})。
func Validate() error { return validate(credentialsJSON) }

func validate(data []byte) error {
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &sa); err != nil {
		return fmt.Errorf("解析內嵌 credentials 失敗: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return ErrPlaceholderCredentials
	}
	return nil
}
```

- [ ] **Step 5: `go mod tidy` 取得 testify**
- [ ] **Step 6: 跑測試確認通過**：`go test ./internal/embedded/`(預期 PASS)
- [ ] **Step 7: commit**：`feat(embedded): 內嵌 Service Account 憑證與驗證`

---

## Stage 2：internal/uploader

**Files:**
- Create: `internal/uploader/uploader.go`
- Test: `internal/uploader/uploader_test.go`

**Interfaces:**
- Consumes: 無(憑證以 `[]byte` 由呼叫端傳入)
- Produces:
  - `uploader.Config{ CredentialsJSON []byte; FolderID string; Subject string }`
  - `uploader.New(ctx context.Context, cfg Config, log *slog.Logger) (*Uploader, error)`
  - `(*Uploader).Upload(ctx context.Context, localPath, remoteName string) (*drive.File, error)`

- [ ] **Step 1: 寫測試 `uploader_test.go`**(white-box,測 orchestration)
```go
package uploader

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

type fakeAPI struct {
	findResult            []*drive.File
	findErr               error
	createCalled          bool
	updateCalled          bool
	createdName, createdFolder, updatedID string
}

func (f *fakeAPI) findByName(_ context.Context, _, _ string) ([]*drive.File, error) {
	return f.findResult, f.findErr
}
func (f *fakeAPI) create(_ context.Context, name, folderID string, _ io.Reader, _ int64) (*drive.File, error) {
	f.createCalled = true
	f.createdName = name
	f.createdFolder = folderID
	return &drive.File{Id: "new-id", Name: name}, nil
}
func (f *fakeAPI) update(_ context.Context, fileID string, _ io.Reader, _ int64) (*drive.File, error) {
	f.updateCalled = true
	f.updatedID = fileID
	return &drive.File{Id: fileID, Name: "x"}, nil
}

func newTestUploader(api driveAPI) *Uploader {
	return &Uploader{api: api, folderID: "FID", log: slog.New(slog.NewTextHandler(io.Discard, nil))}
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
	_, err := u.Upload(context.Background(), tempFile(t, "app.log"), "app.log")
	require.NoError(t, err)
	assert.True(t, api.createCalled)
	assert.False(t, api.updateCalled)
	assert.Equal(t, "FID", api.createdFolder)
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

func TestEscapeQueryValue(t *testing.T) {
	assert.Equal(t, `a\'b`, escapeQueryValue(`a'b`))
	assert.Equal(t, `a\\b`, escapeQueryValue(`a\b`))
}

func TestWrapQuotaError(t *testing.T) {
	gErr := &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "storageQuotaExceeded"}}}
	out := wrapQuotaError(gErr)
	assert.Contains(t, out.Error(), "Shared Drive")
}
```

- [ ] **Step 2: 跑測試確認失敗**：`go test ./internal/uploader/`(預期編譯失敗)

- [ ] **Step 3: 寫實作 `uploader.go`**
```go
// Package uploader 負責把本地檔案上傳到固定的 Google Drive 資料夾。
package uploader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const chunkSize = 16 * 1024 * 1024 // 16MB,resumable upload 每塊大小

// Config 為建立 Uploader 所需的設定。
type Config struct {
	CredentialsJSON []byte // Service Account 憑證(JSON bytes)
	FolderID        string // 目標資料夾 ID
	Subject         string // 選填:domain-wide delegation 要模擬的使用者 email
}

// driveAPI 抽象出實際用到的 Drive 操作,方便以 fake 測試 orchestration。
type driveAPI interface {
	findByName(ctx context.Context, name, folderID string) ([]*drive.File, error)
	create(ctx context.Context, name, folderID string, media io.Reader, size int64) (*drive.File, error)
	update(ctx context.Context, fileID string, media io.Reader, size int64) (*drive.File, error)
}

// Uploader 把檔案上傳到固定資料夾。
type Uploader struct {
	api      driveAPI
	folderID string
	log      *slog.Logger
}

// New 依 Config 建立連到 Google Drive 的 Uploader。
func New(ctx context.Context, cfg Config, log *slog.Logger) (*Uploader, error) {
	if cfg.FolderID == "" {
		return nil, errors.New("folderID 為空,請於 build 時以 -ldflags 注入")
	}
	svc, err := newDriveService(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	return &Uploader{api: svc, folderID: cfg.FolderID, log: log}, nil
}

// Upload 把 localPath 的檔案以 remoteName 上傳到設定的資料夾;同名則覆蓋。
func (u *Uploader) Upload(ctx context.Context, localPath, remoteName string) (*drive.File, error) {
	if remoteName == "" {
		remoteName = filepath.Base(localPath)
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("讀取本地檔案資訊失敗: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s 是目錄,只能上傳檔案", localPath)
	}

	existing, err := u.api.findByName(ctx, remoteName, u.folderID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("開啟本地檔案失敗: %w", err)
	}
	defer f.Close()

	var result *drive.File
	if len(existing) == 0 {
		u.log.Info("資料夾內無同名檔案,建立新檔", "name", remoteName)
		result, err = u.api.create(ctx, remoteName, u.folderID, f, info.Size())
	} else {
		if len(existing) > 1 {
			u.log.Warn("資料夾內有多個同名檔案,將更新第一筆", "name", remoteName, "count", len(existing))
		}
		u.log.Info("覆蓋既有檔案", "name", remoteName, "id", existing[0].Id)
		result, err = u.api.update(ctx, existing[0].Id, f, info.Size())
	}
	if err != nil {
		return nil, wrapQuotaError(err)
	}
	return result, nil
}

// driveService 是 driveAPI 的實際實作,包住官方 *drive.Service。
type driveService struct {
	svc *drive.Service
	log *slog.Logger
}

func newDriveService(ctx context.Context, cfg Config, log *slog.Logger) (*driveService, error) {
	jwtCfg, err := google.JWTConfigFromJSON(cfg.CredentialsJSON, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("解析 Service Account 憑證失敗: %w", err)
	}
	if cfg.Subject != "" {
		jwtCfg.Subject = cfg.Subject
	}
	svc, err := drive.NewService(ctx, option.WithHTTPClient(jwtCfg.Client(ctx)))
	if err != nil {
		return nil, fmt.Errorf("建立 Drive service 失敗: %w", err)
	}
	return &driveService{svc: svc, log: log}, nil
}

func (d *driveService) findByName(ctx context.Context, name, folderID string) ([]*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false",
		escapeQueryValue(name), escapeQueryValue(folderID))
	var files []*drive.File
	err := d.svc.Files.List().
		Q(q).
		Fields("nextPageToken, files(id, name)").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		PageSize(100).
		Pages(ctx, func(page *drive.FileList) error {
			files = append(files, page.Files...)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("查詢同名檔案失敗: %w", err)
	}
	return files, nil
}

func (d *driveService) create(ctx context.Context, name, folderID string, media io.Reader, size int64) (*drive.File, error) {
	created, err := d.svc.Files.Create(&drive.File{Name: name, Parents: []string{folderID}}).
		Media(media, googleapi.ChunkSize(chunkSize)).
		SupportsAllDrives(true).
		ProgressUpdater(d.progress(size)).
		Fields("id, name").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("建立檔案失敗: %w", err)
	}
	return created, nil
}

func (d *driveService) update(ctx context.Context, fileID string, media io.Reader, size int64) (*drive.File, error) {
	// 更新時不可帶 Parents,否則 API 報錯;傳空 File 只更新內容。
	updated, err := d.svc.Files.Update(fileID, &drive.File{}).
		Media(media, googleapi.ChunkSize(chunkSize)).
		SupportsAllDrives(true).
		ProgressUpdater(d.progress(size)).
		Fields("id, name").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("更新檔案內容失敗: %w", err)
	}
	return updated, nil
}

func (d *driveService) progress(total int64) googleapi.ProgressUpdater {
	return func(current, _ int64) {
		if total > 0 {
			d.log.Info("上傳進度", "bytes", current, "total", total,
				"percent", fmt.Sprintf("%.1f%%", float64(current)/float64(total)*100))
			return
		}
		d.log.Info("上傳進度", "bytes", current)
	}
}

// escapeQueryValue 跳脫 Drive query 中以單引號包覆的字串值。
func escapeQueryValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// wrapQuotaError 偵測 storageQuotaExceeded 並補上可行動的指引。
func wrapQuotaError(err error) error {
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		for _, e := range gErr.Errors {
			if e.Reason == "storageQuotaExceeded" {
				return fmt.Errorf("%w\n提示: Service Account 在 My Drive 沒有儲存配額,大檔上傳會失敗。"+
					"請改用共用雲端硬碟(Shared Drive)的 folderID,或於 build 時以 SUBJECT 設定 domain-wide delegation。", err)
			}
		}
	}
	return err
}
```

- [ ] **Step 4: `go mod tidy`** 取得 google.golang.org/api、golang.org/x/oauth2
- [ ] **Step 5: 跑測試確認通過**：`go test ./internal/uploader/`(預期 PASS)
- [ ] **Step 6: commit**：`feat(uploader): Drive 上傳(同名覆蓋、resumable、配額錯誤指引)`

---

## Stage 3：cmd CLI + Makefile + README

**Files:**
- Create: `cmd/gdrive-upload/main.go`
- Create: `Makefile`
- Modify: `README.md`(完成內容)

**Interfaces:**
- Consumes: `embedded.Validate`、`embedded.CredentialsJSON`、`uploader.New`、`uploader.Config`、`(*Uploader).Upload`
- build 變數:`var folderID string`、`var subject string`(package main),以 `-ldflags -X main.folderID/-X main.subject` 注入。

- [ ] **Step 1: 寫 `cmd/gdrive-upload/main.go`**
```go
// Command gdrive-upload 把指定檔案上傳到 build 時固定的 Google Drive 資料夾。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/gilbertchiao/google-drive-uploader/internal/embedded"
	"github.com/gilbertchiao/google-drive-uploader/internal/uploader"
)

// 以 -ldflags "-X main.folderID=... -X main.subject=..." 於 build 時注入。
var (
	folderID string
	subject  string
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "錯誤:", err)
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "", "上傳到 Drive 後的檔名(預設取本地檔名)")
	verbose := flag.Bool("v", false, "顯示 debug 等級日誌")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		return fmt.Errorf("需要剛好一個檔案路徑參數,收到 %d 個", flag.NArg())
	}
	localPath := flag.Arg(0)

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := embedded.Validate(); err != nil {
		return err
	}
	if _, err := os.Stat(localPath); err != nil {
		return fmt.Errorf("本地檔案無法存取: %w", err)
	}

	ctx := context.Background()
	up, err := uploader.New(ctx, uploader.Config{
		CredentialsJSON: embedded.CredentialsJSON(),
		FolderID:        folderID,
		Subject:         subject,
	}, log)
	if err != nil {
		return err
	}

	log.Info("開始上傳", "file", localPath, "folderID", folderID)
	f, err := up.Upload(ctx, localPath, *name)
	if err != nil {
		return err
	}
	log.Info("上傳完成", "id", f.Id, "name", f.Name)
	fmt.Println(f.Id)
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `用法: %s [選項] <本地檔案路徑>

把指定檔案上傳到 build 時固定的 Google Drive 資料夾(同名覆蓋)。

選項:
`, os.Args[0])
	flag.PrintDefaults()
}
```

- [ ] **Step 2: 寫 `Makefile`**
```makefile
BINARY      := gdrive-upload
CMD_PKG     := ./cmd/gdrive-upload
EMBED_CREDS := internal/embedded/credentials.json

CREDENTIALS ?=
FOLDER_ID   ?=
SUBJECT     ?=

.PHONY: build test fmt vet clean help

help:
	@echo "make build CREDENTIALS=path/to/creds.json FOLDER_ID=xxx [SUBJECT=user@domain]"
	@echo "make test | fmt | vet | clean"

build:
	@test -n "$(CREDENTIALS)" || { echo "錯誤: 請指定 CREDENTIALS=/path/to/credentials.json"; exit 1; }
	@test -f "$(CREDENTIALS)" || { echo "錯誤: 找不到憑證檔 $(CREDENTIALS)"; exit 1; }
	@test -n "$(FOLDER_ID)"   || { echo "錯誤: 請指定 FOLDER_ID=<Google Drive 資料夾 ID>"; exit 1; }
	@cp "$(CREDENTIALS)" "$(EMBED_CREDS)"
	@trap 'printf "{}" > "$(EMBED_CREDS)"' EXIT; \
		mkdir -p bin; \
		go build -ldflags "-X main.folderID=$(FOLDER_ID) -X main.subject=$(SUBJECT)" -o bin/$(BINARY) $(CMD_PKG)
	@echo "已輸出 bin/$(BINARY)"

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin
	@printf "{}" > "$(EMBED_CREDS)"
```

- [ ] **Step 3: 確認編譯**：`go build ./...`(此時內嵌為佔位 `{}`,可編譯)
- [ ] **Step 4: 確認全測試通過**：`go test ./...`
- [ ] **Step 5: 用假憑證驗證 Makefile 流程**：建臨時假 creds，`make build CREDENTIALS=... FOLDER_ID=test`，確認產出 `bin/gdrive-upload` 且 `internal/embedded/credentials.json` 已還原為 `{}`
- [ ] **Step 6: 完成 `README.md`**(安裝/build/使用/SA 配額限制說明/cron 範例)
- [ ] **Step 7: commit**：`feat(cli): CLI、Makefile 與 README`

## Self-Review

- Spec coverage:第 6 節 build 機制 → Stage 3 Makefile;第 7 節執行行為 → Stage 2/3;第 8 節錯誤處理 → `wrapQuotaError`、各 `%w` 包裝;第 9 節測試 → Stage 1/2 測試。涵蓋完整。
- Placeholder scan:各 step 皆有完整程式碼,無 TODO。
- Type consistency:`uploader.Config`、`New`、`Upload`、`driveAPI` 三方法簽章在 Stage 2 定義並於 Stage 3 使用,一致。
