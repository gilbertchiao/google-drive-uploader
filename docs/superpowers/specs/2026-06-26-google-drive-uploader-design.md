# Google Drive Uploader 設計文件

> 狀態:已核准（2026-06-26）

## 1. 目標

提供一個用 Go 撰寫的 CLI 工具，build 之後能以

```
gdrive-upload [-name 遠端檔名] [-v] <本地檔案路徑>
```

的方式，把指定檔案上傳到「build 時就已固定」的某個 Google Drive 資料夾。

主要使用情境：在 Ubuntu 24.04 上把 log 檔（單檔 300MB ~ 500MB）上傳到 Google Drive。

## 2. 關鍵需求與決策

| 項目 | 決策 |
| --- | --- |
| 認證方式 | Service Account（SA），credentials.json 於 build 時內嵌進 binary |
| 目標資料夾 | folderID 於 build 時內嵌進 binary |
| 目標硬碟類型 | 我的雲端硬碟（My Drive）資料夾 |
| 同名檔案處理 | 覆蓋（先 find by name，有則 update、無則 create） |
| 內嵌機制 | `go:embed`（credentials）+ `-ldflags -X`（folderID / subject / version）+ Makefile |
| 大檔上傳 | 官方 client 的 resumable upload，chunk size 16MB，回報進度 |
| 權限 scope | `drive.file`（最小權限，只管自己建立的檔案） |

## 3. 重大限制：Service Account 儲存配額（務必理解）

自 2021 年起，Service Account 在 Google Drive **沒有自己的儲存配額**。

當 SA 上傳檔案到「使用者分享給它的 My Drive 資料夾」時，檔案的擁有者會是 SA 而非該使用者，
上傳會嘗試計入 SA 的配額，對 300–500MB 的大檔**很可能直接回傳 `storageQuotaExceeded` 而失敗**。
這是 Google 的政策限制，並非程式寫法可繞過。

能穩定運作的替代路徑：

1. **共用雲端硬碟（Shared Drive / Team Drive）**：儲存空間由組織池化，最適合大檔（需 Google Workspace）。
2. **Domain-wide delegation**：SA 模擬一位 Workspace 使用者上傳，檔案歸該使用者、計入其配額（需 Workspace 且管理員授權）。

若使用的是**個人 Google 帳號**，上述兩者皆不適用，純 SA 上傳大檔到個人 My Drive 基本上會失敗。

### 本設計的因應方式

- 仍依需求實作 My Drive + SA 路徑。
- 內建一個 build 時可選的 `SUBJECT`（要模擬的使用者 email）以支援 domain-wide delegation；
  未指定時走純 SA。
- 執行期偵測 `storageQuotaExceeded`，印出清楚的指引（建議改用 Shared Drive 或設定 delegation）。

如此現在可用，未來要切換到 Shared Drive / delegation 只需調整 build 參數，程式碼不必改。

## 4. 技術選型

- 語言：Go（單一 binary、零 runtime 依賴，適合在 Ubuntu 上直接放著跑）。
- 套件：官方 `google.golang.org/api/drive/v3` + `golang.org/x/oauth2/google`，內建 resumable upload，最穩。
- 日誌：標準庫 `log/slog`，輸出 stderr。
- 測試：標準 `testing` + `testify`。

## 5. 專案結構（Standard Go Project Layout）

```
.
├── cmd/gdrive-upload/main.go        # 進入點:參數解析、slog 設定、build 變數(folderID/subject)
├── internal/
│   ├── embedded/
│   │   ├── embedded.go              # //go:embed credentials.json + 驗證 helper
│   │   └── credentials.json         # 佔位 {},已進版控;build 時暫時替換成真實憑證
│   └── uploader/
│       ├── uploader.go              # Drive client、find/create/update、resumable、進度
│       └── uploader_test.go         # 以 fake interface 測決策邏輯
├── docs/superpowers/                # 設計文件與實作計劃
├── Makefile
├── go.mod
├── .gitignore
└── README.md
```

## 6. Build 時注入機制

`credentials.json` 永遠以**佔位內容 `{}`** 進版控（讓 `go build`／CI 可正常編譯），真實憑證只在 build 當下暫時存在：

```
make build CREDENTIALS=/path/to/creds.json FOLDER_ID=xxx [SUBJECT=user@domain]
```

Makefile `build` 流程：

1. 驗證 `CREDENTIALS` 檔案存在、`FOLDER_ID` 非空。
2. 複製真實 creds → `internal/embedded/credentials.json`。
3. `CGO_ENABLED=0 go build -ldflags "-X main.folderID=$(FOLDER_ID) -X main.subject=$(SUBJECT) -X main.version=$(VERSION)" -o bin/gdrive-upload ./cmd/gdrive-upload`（`VERSION` 預設 `v1.0.0`）。
4. 無論成功與否，皆還原 `internal/embedded/credentials.json` 為佔位 `{}`（避免誤 commit 真實憑證）。

- `credentials`（多行 JSON）→ 用 `go:embed`。
- `folderID` / `subject` / `version`（單行字串）→ 用 `-ldflags -X`。

## 7. 執行行為

1. 解析參數：取得本地檔案路徑；`-name` 可覆寫遠端檔名（預設 `filepath.Base()`）；`--verbose` 開 Debug log；`-v`/`--version` 顯示版本號（預設 `v1.0.0`，可於 build 時以 `-ldflags -X main.version` 覆寫）並結束。
2. 啟動前驗證：
   - 本地檔案存在且可讀。
   - 內嵌 credentials 非佔位（含 `client_email` 與 `private_key`）。
   - `folderID` 非空。
3. 建立 Drive client：以 JWT 設定載入 SA；若 `subject` 非空則設定 delegation。
4. 覆蓋邏輯：
   - 以 `name = '<遠端檔名>' and '<folderID>' in parents and trashed = false` 查詢。
   - 找到 → `files.Update` 更新第一筆內容（多筆則更新第一筆並 warn）。
   - 沒找到 → `files.Create`，parents 設為 folderID。
5. 上傳：`Media()` 搭配 `googleapi.ChunkSize(16MB)`（resumable）；`ProgressUpdater` 用 slog 回報已傳位元組／百分比。
6. 完成印出檔案 ID 與名稱；結束碼 0。任一步失敗 → 包裝錯誤、印出、結束碼非 0。

## 8. 錯誤處理與日誌

- 顯式檢查 error，向上以 `fmt.Errorf("...: %w", err)` 包裝保留錯誤鏈。
- 特別偵測 `*googleapi.Error` 中 `storageQuotaExceeded`，印出 Shared Drive / delegation 指引。
- 日誌用 `log/slog`（TextHandler，stderr）；`--verbose` 切 `LevelDebug`，否則 `LevelInfo`。

## 9. 測試策略

把 Drive 操作抽成 interface，讓 orchestration 邏輯可在無真實憑證下測試：

- `findByName` / `create` / `update` 抽成 interface，uploader 依賴此 interface。
- 測試項目：
  - create vs update 決策（fake 回傳「找到／找不到」）。
  - 檔名中含單引號時的 query 跳脫。
  - 遠端檔名推導（未給 `-name` 時取 base name）。
  - 參數／前置驗證（缺檔案、缺 folderID、佔位 credentials）。

## 10. 非目標（YAGNI）

- 不做壓縮（依需求「固定上傳檔案」原樣上傳）。
- 不做多檔／資料夾遞迴上傳。
- 不做設定檔（folderID/credentials 皆 build 時固定）。
- 不內建排程（交給 cron / systemd timer）。
