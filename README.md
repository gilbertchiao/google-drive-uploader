# google-drive-uploader

用 Go 寫的 CLI 工具:build 後以 `gdrive-upload [檔案]` 把檔案上傳到 **build 時固定** 的 Google Drive 資料夾。
主要情境是在 Ubuntu 24.04 上把 log 檔(單檔 300–500MB)上傳到 Google Drive。

> 設計與實作細節見 `docs/superpowers/`。README 完整內容會在 CLI/Makefile 完成後補上。

## 重要限制(務必先讀)

Service Account 在「我的雲端硬碟(My Drive)」**沒有自己的儲存配額**。把大檔上傳到使用者分享給 SA 的 My Drive
資料夾時,檔案歸 SA 所有並計入 SA 配額,300–500MB 的檔案**很可能回 `storageQuotaExceeded` 失敗**。

穩定可行的做法:改用**共用雲端硬碟(Shared Drive)**,或設定 **domain-wide delegation**(build 時帶 `SUBJECT`)。
兩者皆需 Google Workspace。詳見設計文件。
