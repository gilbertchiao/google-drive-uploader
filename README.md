# google-drive-uploader

用 Go 寫的 CLI 工具:build 後以 `gdrive-upload [檔案]` 把檔案上傳到 **build 時固定** 的 Google Drive 資料夾(同名覆蓋)。

特點:

- Service Account credentials 與目標 folderID 於 **build 時內嵌** 進 binary,部署時只需單一執行檔。
- 同名檔案自動 **覆蓋**(更新內容),不會在資料夾內累積重複檔。
- 大檔走 **resumable upload**(16MB 分塊),並回報進度。
- 標準庫 `log/slog` 結構化日誌、明確錯誤處理。

---

## ⚠️ 重要限制:Service Account 儲存配額

自 2021 年起,Service Account 在 Google Drive **沒有自己的儲存配額**。把大檔上傳到「使用者分享給 SA 的
我的雲端硬碟(My Drive)資料夾」時,檔案歸 SA 所有並計入 SA 配額,300–500MB 的檔案 **很可能回傳
`storageQuotaExceeded` 而失敗**。這是 Google 的政策限制,非程式可繞過。

穩定可行的做法(擇一):

1. **共用雲端硬碟(Shared Drive / Team Drive)** — 儲存空間由組織池化,最適合大檔。把 SA 加入該 Shared Drive
   為成員,folderID 用 Shared Drive 內的資料夾即可(本工具已帶 `supportsAllDrives`)。需 Google Workspace。
2. **Domain-wide delegation** — SA 模擬一位 Workspace 使用者上傳,檔案歸該使用者、計入其配額。
   build 時以 `SUBJECT=user@your-domain.com` 指定要模擬的使用者。需 Workspace 且管理員於後台授權 SA 的
   client ID 對應 `https://www.googleapis.com/auth/drive.file` scope。

若使用 **個人 Google 帳號**,上述兩者皆不適用,純 SA 上傳大檔到個人 My Drive 基本上會失敗。

---

## 事前準備

### 1. 建立 Service Account 與 credentials.json

1. 在 [Google Cloud Console](https://console.cloud.google.com/) 建立(或選擇)專案。
2. 啟用 **Google Drive API**。
3. 建立 **Service Account**,並產生 JSON 金鑰,下載為 `credentials.json`。
4. 視情境授權(擇一):
   - **My Drive 資料夾**:把目標資料夾「共用」給 SA 的 email(`xxx@xxx.iam.gserviceaccount.com`),權限給「編輯者」。
   - **Shared Drive**:把 SA 加入 Shared Drive 成員(內容管理員以上)。
   - **Delegation**:於 Workspace 管理後台設定 domain-wide delegation。

### 2. 取得 folderID

開啟目標資料夾,網址列 `https://drive.google.com/drive/folders/<這段就是 folderID>`。

### 3. Go toolchain

相依套件 `google.golang.org/api` 要求 Go ≥ 1.25.8。若系統 Go 較舊,Go 預設的 `GOTOOLCHAIN=auto`
會在 build 時自動下載對應 toolchain(需網路)。**Makefile 以 `CGO_ENABLED=0` 產生靜態、單一可執行檔,
複製到目標主機即可執行,不需 Go toolchain 或任何 runtime。**

---

## Build

```bash
make build CREDENTIALS=/path/to/credentials.json FOLDER_ID=<folderID>
```

加 delegation:

```bash
make build CREDENTIALS=/path/to/credentials.json FOLDER_ID=<folderID> SUBJECT=user@your-domain.com
```

產出於 `bin/gdrive-upload`。版本號預設為 `v1.0.0`,可加 `VERSION=vX.Y.Z` 覆寫(會編進 binary,執行 `gdrive-upload --version` 可查看)。

> 安全性:版控中的 `internal/embedded/credentials.json` 永遠是佔位 `{}`;`make build` 只在編譯當下暫時放入真實憑證,
> 結束後(含失敗)立即還原佔位,避免真實憑證被 commit。請勿將真實 `credentials.json` 加入版控。

把 `bin/gdrive-upload` 複製到目標主機(例如 `/usr/local/bin/`)即可使用。

---

## 使用

```bash
gdrive-upload [選項] <本地檔案路徑>
```

選項:

- `-name <檔名>`:上傳到 Drive 後的檔名(預設取本地檔名)。
- `--verbose`:顯示 debug 等級日誌。
- `-v`、`--version`:顯示版本號並結束。

成功時 stdout 會印出該檔案的 Drive file ID。

### 範例

```bash
# 上傳 app.log(遠端檔名 app.log,若已存在則覆蓋)
gdrive-upload /var/log/myapp/app.log

# 指定遠端檔名
gdrive-upload -name myapp-latest.log /var/log/myapp/app.log

# 觀察上傳進度
gdrive-upload --verbose /var/log/myapp/app.log

# 顯示版本
gdrive-upload --version
```

### 搭配 cron(每天上傳一次)

```cron
# 每天 03:00 上傳昨日 log
0 3 * * * /usr/local/bin/gdrive-upload /var/log/myapp/app.log >> /var/log/gdrive-upload.log 2>&1
```

---

## 疑難排解

- **`storageQuotaExceeded`**:見上方「Service Account 儲存配額」。改用 Shared Drive 的 folderID,或設定 delegation。
- **`File not found` / 查無資料夾**:確認 folderID 正確,且該資料夾已共用給 SA(或 SA 已加入 Shared Drive)。
- **`內嵌的 credentials 無效`**:binary 不是用 `make build` 注入真實憑證編譯的(內嵌仍為佔位)。請重新 build。

---

## 開發

```bash
make test   # 跑單元測試
make vet    # go vet
make fmt    # gofmt -w
make clean  # 清掉 bin/ 並還原佔位憑證
```

設計與分階段實作計劃見 `docs/superpowers/`。
