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

// driveAPI 抽象出實際用到的 Drive 操作,方便以 fake 測試 orchestration 邏輯。
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

// progress 回傳一個會以 slog 回報上傳進度的 ProgressUpdater。
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

// escapeQueryValue 跳脫 Drive query 中以單引號包覆的字串值(反斜線與單引號)。
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
