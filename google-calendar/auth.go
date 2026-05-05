package gcal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const tokenDir = "tokens"

// tokenPath mengembalikan path file token berdasarkan email.
func tokenPath(email string) string {
	safe := ""
	for _, c := range email {
		if c == '@' || c == '.' {
			safe += "_"
		} else {
			safe += string(c)
		}
	}
	return filepath.Join(tokenDir, "gcal_"+safe+".json")
}

// LoadOAuthConfig membaca oauth2_client.json (Desktop app credentials).
func LoadOAuthConfig(clientSecretPath string) (*oauth2.Config, error) {
	b, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca %s: %w\n"+
			"Download dari: console.cloud.google.com → APIs & Services → Credentials → Create Credentials → OAuth client ID → Desktop app",
			clientSecretPath, err)
	}
	cfg, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("gagal parse oauth config: %w", err)
	}
	return cfg, nil
}

// AuthorizeUser membuka browser untuk otorisasi satu user dan menyimpan token.
func AuthorizeUser(cfg *oauth2.Config, email string) error {
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return err
	}

	// Pakai local redirect dengan port acak
	cfg.RedirectURL = "http://localhost:8085"

	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\n[%s] Buka URL berikut di browser:\n\n%s\n\n", email, authURL)
	openBrowser(authURL)

	// Tangkap code via local HTTP server
	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8085"}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code != "" {
			fmt.Fprintf(w, "<h2>Berhasil! Silakan tutup tab ini.</h2>")
			codeCh <- code
		}
	})
	go func() { _ = srv.ListenAndServe() }()

	fmt.Println("Menunggu otorisasi di browser...")
	code := <-codeCh
	_ = srv.Shutdown(context.Background())

	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("gagal exchange token: %w", err)
	}

	path := tokenPath(email)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("gagal menyimpan token: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(tok); err != nil {
		return err
	}

	fmt.Printf("[%s] Token tersimpan di %s\n", email, path)
	return nil
}

// GetCalendarService membuat calendar.Service menggunakan token yang tersimpan.
func GetCalendarService(cfg *oauth2.Config, email string) (*calendar.Service, error) {
	path := tokenPath(email)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("token untuk %s tidak ditemukan (%s).\nJalankan: go run main.go gcal-auth %s", email, path, email)
	}
	defer f.Close()

	tok := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(tok); err != nil {
		return nil, fmt.Errorf("gagal decode token %s: %w", path, err)
	}

	ts := cfg.TokenSource(context.Background(), tok)

	// Simpan token baru jika sudah di-refresh otomatis
	newTok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("token expired untuk %s, jalankan ulang: go run main.go gcal-auth %s", email, email)
	}
	if newTok.AccessToken != tok.AccessToken {
		saveToken(path, newTok)
	}

	return calendar.NewService(context.Background(), option.WithTokenSource(ts))
}

func saveToken(path string, tok *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(tok)
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	default:
		cmd = "start"
	}
	_ = exec.Command(cmd, url).Start()
}
