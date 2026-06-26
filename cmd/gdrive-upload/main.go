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

// 以 -ldflags "-X main.folderID=... -X main.subject=... -X main.version=..." 於 build 時注入。
var (
	folderID string
	subject  string
	version  = "v1.0.0"
)

// appName 為顯示用的程式名稱。
const appName = "gdrive-upload"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "錯誤:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		name        string
		verbose     bool
		showVersion bool
	)
	flag.StringVar(&name, "name", "", "上傳到 Drive 後的檔名(預設取本地檔名)")
	flag.BoolVar(&verbose, "verbose", false, "顯示 debug 等級日誌")
	flag.BoolVar(&showVersion, "version", false, "顯示版本號並結束")
	flag.BoolVar(&showVersion, "v", false, "顯示版本號並結束(等同 --version)")
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		fmt.Printf("%s %s\n", appName, version)
		return nil
	}

	if flag.NArg() != 1 {
		flag.Usage()
		return fmt.Errorf("需要剛好一個檔案路徑參數,收到 %d 個", flag.NArg())
	}
	localPath := flag.Arg(0)

	level := slog.LevelInfo
	if verbose {
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
	f, err := up.Upload(ctx, localPath, name)
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
