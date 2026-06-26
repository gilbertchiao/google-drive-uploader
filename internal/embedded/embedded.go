// Package embedded 提供 build 時內嵌的 Service Account 憑證。
//
// 版控中的 credentials.json 永遠是佔位內容 {};真實憑證由 Makefile 於 build 時
// 暫時複製進來,build 結束後立即還原為佔位,避免真實憑證被 commit。
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

// CredentialsJSON 回傳內嵌的 Service Account 憑證 bytes。
func CredentialsJSON() []byte { return credentialsJSON }

// Validate 檢查內嵌憑證是否為有效的 Service Account 憑證(而非佔位 {})。
func Validate() error { return validate(credentialsJSON) }

// validate 將實際檢查邏輯獨立出來,讓測試能傳入任意內容驗證。
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
