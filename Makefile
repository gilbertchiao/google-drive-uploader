BINARY      := gdrive-upload
CMD_PKG     := ./cmd/gdrive-upload
EMBED_CREDS := internal/embedded/credentials.json

CREDENTIALS ?=
FOLDER_ID   ?=
SUBJECT     ?=
VERSION     ?= v1.0.0

.PHONY: build test fmt vet lint clean help

help:
	@echo "make build CREDENTIALS=path/to/credentials.json FOLDER_ID=xxx [SUBJECT=user@domain] [VERSION=vX.Y.Z]"
	@echo "  VERSION 預設 v1.0.0,會編進 binary(gdrive-upload --version 可查看)"
	@echo "make test | fmt | vet | lint | clean"

## build: 注入 credentials 與 folderID 後編譯,結束後還原佔位檔
build:
	@test -n "$(CREDENTIALS)" || { echo "錯誤: 請指定 CREDENTIALS=/path/to/credentials.json"; exit 1; }
	@test -f "$(CREDENTIALS)" || { echo "錯誤: 找不到憑證檔 $(CREDENTIALS)"; exit 1; }
	@test -n "$(FOLDER_ID)"   || { echo "錯誤: 請指定 FOLDER_ID=<Google Drive 資料夾 ID>"; exit 1; }
	@set -e; \
		trap 'printf "{}" > "$(EMBED_CREDS)"' EXIT; \
		cp "$(CREDENTIALS)" "$(EMBED_CREDS)"; \
		mkdir -p bin; \
		CGO_ENABLED=0 go build -ldflags "-X main.folderID=$(FOLDER_ID) -X main.subject=$(SUBJECT) -X main.version=$(VERSION)" -o bin/$(BINARY) $(CMD_PKG); \
		echo "已輸出 bin/$(BINARY)(靜態,CGO_ENABLED=0)"

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

## lint: 執行 golangci-lint(需先安裝,設定見 .golangci.yml)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "錯誤: 未安裝 golangci-lint"; \
		echo "  安裝方式請見 https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	}
	golangci-lint run ./...

clean:
	rm -rf bin
	@printf "{}" > "$(EMBED_CREDS)"
