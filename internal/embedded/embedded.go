// Package embedded 提供 build 時內嵌的 Service Account 憑證。
//
// 版控中的 credentials.json 永遠是佔位內容 {};真實憑證由 build 流程於編譯時
// 暫時注入,build 結束後立即還原為佔位,避免真實憑證被 commit。
package embedded

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

//go:embed credentials.json
var credentialsJSON []byte

// ErrInvalidCredentials 表示內嵌憑證無效:為佔位 {} 或缺少必要欄位
// (client_email / private_key),亦即尚未於 build 時注入有效的 Service Account 憑證。
var ErrInvalidCredentials = errors.New("內嵌的 credentials 無效(為佔位 {} 或缺少 client_email/private_key),請於 build 時注入有效的 Service Account 憑證")

// CredentialsJSON 回傳內嵌憑證的複本,避免呼叫端就地修改影響內部狀態。
func CredentialsJSON() []byte {
	out := make([]byte, len(credentialsJSON))
	copy(out, credentialsJSON)
	return out
}

// Validate 檢查內嵌憑證是否為有效的 Service Account 憑證(而非佔位或缺欄位)。
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
		return ErrInvalidCredentials
	}
	return nil
}
