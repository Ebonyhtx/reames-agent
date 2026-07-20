package weixin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

type savedAccount struct {
	Token   string `json:"token"`
	BaseURL string `json:"base_url"`
	UserID  string `json:"user_id"`
	SavedAt string `json:"saved_at"`
}

type LoginResult struct {
	AccountID string
	Token     string
	BaseURL   string
	UserID    string
}

type LoginSession struct {
	SessionKey string
	QRCode     string
	QRCodeURL  string
	BaseURL    string
	StartedAt  time.Time
}

func weixinAccountDir(root string) string {
	return filepath.Join(root, "weixin", "accounts")
}

func savedAccountPath(accountID string) string {
	root := config.MemoryUserDir()
	stem := weixinAccountFileStem(accountID)
	if root == "" || stem == "" {
		return ""
	}
	return filepath.Join(weixinAccountDir(root), stem+".json")
}

// weixinAccountFileStem preserves ordinary iLink account IDs while mapping
// manually configured path separators, control characters and Windows device
// names to a stable non-secret digest. All account-owned files must use this
// helper so account_id can never escape the accounts directory.
func weixinAccountFileStem(accountID string) string {
	raw := accountID
	accountID = strings.TrimSpace(raw)
	if accountID == "" {
		return ""
	}
	if raw == accountID && safeWeixinAccountFileStem(accountID) {
		return accountID
	}
	sum := sha256.Sum256([]byte(raw))
	return "account-" + hex.EncodeToString(sum[:16])
}

func safeWeixinAccountFileStem(name string) bool {
	if len(name) > 128 || strings.Trim(name, ". ") != name {
		return false
	}
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(`/\\:<>"|?*`, r) {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		base == "CLOCK$" || base == "CONIN$" || base == "CONOUT$" {
		return false
	}
	return len(base) != 4 ||
		(!strings.HasPrefix(base, "COM") && !strings.HasPrefix(base, "LPT")) ||
		base[3] < '1' || base[3] > '9'
}

func loadSavedAccount(accountID string) (savedAccount, error) {
	path := savedAccountPath(accountID)
	if path == "" {
		return savedAccount{}, fmt.Errorf("reamesAgent user config dir is unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return savedAccount{}, err
	}
	var account savedAccount
	if err := json.Unmarshal(data, &account); err != nil {
		return savedAccount{}, err
	}
	return account, nil
}

func loadAnySavedAccount() (savedAccount, error) {
	root := config.MemoryUserDir()
	if root == "" {
		return savedAccount{}, fmt.Errorf("reamesAgent user config dir is unavailable")
	}
	entries, err := os.ReadDir(weixinAccountDir(root))
	if err != nil {
		return savedAccount{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") ||
			strings.HasSuffix(entry.Name(), ".context-tokens.json") ||
			strings.HasSuffix(entry.Name(), ".poll-state.json") {
			continue
		}
		accountID := strings.TrimSuffix(entry.Name(), ".json")
		account, err := loadSavedAccount(accountID)
		if err == nil && account.Token != "" {
			return account, nil
		}
	}
	return savedAccount{}, fmt.Errorf("no saved weixin account")
}

func HasSavedAccount(accountID string) bool {
	if accountID != "" {
		account, err := loadSavedAccount(accountID)
		return err == nil && account.Token != ""
	}
	account, err := loadSavedAccount("default")
	if err == nil && account.Token != "" {
		return true
	}
	account, err = loadAnySavedAccount()
	return err == nil && account.Token != ""
}

func saveAccount(accountID string, account savedAccount) error {
	path := savedAccountPath(accountID)
	if path == "" {
		return fmt.Errorf("reamesAgent user config dir is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: a truncated credentials file silently breaks login.
	return fileutil.AtomicWriteFile(path, data, 0o600)
}

func Login(ctx context.Context, out io.Writer, timeout time.Duration) (*LoginResult, error) {
	if timeout <= 0 {
		timeout = 8 * time.Minute
	}
	session, err := StartLogin(ctx)
	if err != nil {
		return nil, err
	}
	if out != nil {
		fmt.Fprintln(out, "请使用微信扫描以下二维码链接：")
		if session.QRCodeURL != "" {
			fmt.Fprintln(out, session.QRCodeURL)
		} else {
			fmt.Fprintln(out, session.QRCode)
		}
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
		result, status, err := PollLogin(ctx, session)
		if err != nil {
			if out != nil {
				fmt.Fprintf(out, "二维码状态查询失败: %v\n", err)
			}
			continue
		}
		if result != nil {
			return result, nil
		}
		if out != nil {
			switch status {
			case "wait", "", "<nil>":
				fmt.Fprint(out, ".")
			case "scaned":
				fmt.Fprintln(out, "\n已扫码，请在微信里确认...")
			default:
				fmt.Fprintf(out, "\n二维码状态: %s\n", status)
			}
		}
	}
	return nil, fmt.Errorf("weixin login timed out")
}

func StartLogin(ctx context.Context) (*LoginSession, error) {
	qrResp, err := ilinkGET(ctx, defaultWeixinAPI, getBotQRPath+"?bot_type=3")
	if err != nil {
		return nil, fmt.Errorf("fetch qr code: %w", err)
	}
	qrcode := fmt.Sprint(qrResp["qrcode"])
	qrcodeURL := fmt.Sprint(qrResp["qrcode_img_content"])
	if qrcode == "" || qrcode == "<nil>" {
		return nil, fmt.Errorf("weixin qr response missing qrcode")
	}
	if qrcodeURL == "<nil>" {
		qrcodeURL = ""
	}
	return &LoginSession{
		SessionKey: qrcode,
		QRCode:     qrcode,
		QRCodeURL:  qrcodeURL,
		BaseURL:    defaultWeixinAPI,
		StartedAt:  time.Now(),
	}, nil
}

func PollLogin(ctx context.Context, session *LoginSession) (*LoginResult, string, error) {
	if session == nil || session.QRCode == "" {
		return nil, "", fmt.Errorf("weixin login session is missing")
	}
	baseURL := session.BaseURL
	if baseURL == "" {
		baseURL = defaultWeixinAPI
	}
	statusResp, err := ilinkGET(ctx, baseURL, getQRStatusPath+"?qrcode="+session.QRCode)
	if err != nil {
		return nil, "", err
	}
	status := fmt.Sprint(statusResp["status"])
	switch status {
	case "wait", "", "<nil>":
		return nil, status, nil
	case "scaned":
		return nil, status, nil
	case "scaned_but_redirect":
		if host := fmt.Sprint(statusResp["redirect_host"]); host != "" && host != "<nil>" {
			session.BaseURL = "https://" + host
		}
		return nil, status, nil
	case "confirmed":
		accountID := fmt.Sprint(statusResp["ilink_bot_id"])
		token := fmt.Sprint(statusResp["bot_token"])
		userID := fmt.Sprint(statusResp["ilink_user_id"])
		respBaseURL := fmt.Sprint(statusResp["baseurl"])
		if respBaseURL == "" || respBaseURL == "<nil>" {
			respBaseURL = defaultWeixinAPI
		}
		if accountID == "" || accountID == "<nil>" || token == "" || token == "<nil>" {
			return nil, status, fmt.Errorf("weixin qr confirmed but credential payload is incomplete")
		}
		account := savedAccount{
			Token:   token,
			BaseURL: respBaseURL,
			UserID:  userID,
			SavedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := saveAccount(accountID, account); err != nil {
			return nil, status, err
		}
		if err := saveAccount("default", account); err != nil {
			return nil, status, err
		}
		return &LoginResult{AccountID: accountID, Token: token, BaseURL: respBaseURL, UserID: userID}, status, nil
	case "expired":
		return nil, status, fmt.Errorf("weixin qr code expired; rerun login")
	default:
		return nil, status, nil
	}
}
