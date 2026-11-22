package signals

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"rsc.io/qr"
)

const (
	// QR login needs no code at all: the runner shows a QR code, and the owner scans it with the
	// Telegram app on a phone that is already signed in (Settings > Devices > Link Desktop Device).
	// Telegram replaces the code every 30 seconds or so; the picture is rewritten each time.
	TELEGRAM_QR_IMAGE     = "telegram-login-qr.png"
	TELEGRAM_QR_PAGE      = "telegram-login-qr.html"
	TELEGRAM_QR_PERMS     = 0o600
	TELEGRAM_QR_SCALE     = 8 // pixels per QR module
	TELEGRAM_QR_SHOWN     = "Telegram QR code ready (%s). On your phone: Telegram > Settings > Devices > Link Desktop Device, then scan it."
	TELEGRAM_QR_PAGE_HTML = `<!DOCTYPE html><html><head><meta charset="utf-8"><meta http-equiv="refresh" content="3">
<title>Telegram login</title><style>body{margin:0;display:grid;place-items:center;min-height:100vh;background:#fff;font-family:sans-serif}
img{width:min(80vmin,420px);image-rendering:pixelated}p{max-width:30rem;text-align:center;color:#222}</style></head>
<body><div><img src="` + TELEGRAM_QR_IMAGE + `?%d" alt="Telegram login QR code">
<p>On your phone: Telegram, Settings, Devices, <b>Link Desktop Device</b>, then scan this code. It refreshes on its own.</p></div></body></html>`
)

// TelegramQRLogin signs the account in by QR code, with no login code, and saves the session for the
// runner. The code picture and a page that shows it are written to dir.
func TelegramQRLogin(ctx context.Context, data, dir, passwordFile string, output io.Writer) error {
	config := telegramConfigFromEnv(data)
	if config.id <= 0 || config.hash == "" {
		return errors.New(TELEGRAM_LOGIN_ENV)
	}
	ctx, cancel := context.WithTimeout(ctx, TELEGRAM_LOGIN_TIMEOUT)
	defer cancel()
	disk := telegramSession{path: config.path}
	memory := &session.StorageMemory{}
	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)
	client := telegram.NewClient(config.id, config.hash, telegram.Options{SessionStorage: memory, UpdateHandler: dispatcher})
	password := func(ctx context.Context) (string, error) {
		return telegramLoginFiles{passwordFile: passwordFile, output: output}.Password(ctx)
	}
	shown := 0
	show := func(_ context.Context, token qrlogin.Token) error {
		shown++
		return writeTelegramQR(dir, token, shown, output)
	}
	var loginErr error
	err := client.Run(ctx, func(connected context.Context) error {
		qr := qrlogin.NewQR(client.API(), config.id, config.hash, qrlogin.Options{Migrate: client.MigrateTo, Password: password})
		_, loginErr = qr.Auth(connected, loggedIn, show)
		return loginErr
	})
	switch {
	case errors.Is(loginErr, errTelegramLoginPassword), errors.Is(loginErr, errTelegramLoginFile), errors.Is(loginErr, errTelegramLoginWait):
		return loginErr
	case err != nil:
		if rpc, ok := tgerr.As(err); ok {
			return fmt.Errorf(TELEGRAM_LOGIN_RPC, rpc.Code)
		}
		return errors.New(TELEGRAM_LOGIN_FAILED)
	}
	contents, err := memory.Bytes(nil)
	if err != nil || disk.StoreSession(ctx, contents) != nil {
		return errors.New(TELEGRAM_SESSION_WRITE)
	}
	fmt.Fprintln(output, TELEGRAM_LOGIN_SAVED)
	return nil
}

// writeTelegramQR writes the current code as a picture and the page that shows it.
func writeTelegramQR(dir string, token qrlogin.Token, version int, output io.Writer) error {
	code, err := qr.Encode(token.URL(), qr.M)
	if err != nil {
		return err
	}
	code.Scale = TELEGRAM_QR_SCALE
	image, err := os.OpenFile(filepath.Join(dir, TELEGRAM_QR_IMAGE), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, TELEGRAM_QR_PERMS)
	if err != nil {
		return err
	}
	err = png.Encode(image, code.Image())
	if closeErr := image.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	page := filepath.Join(dir, TELEGRAM_QR_PAGE)
	if err := os.WriteFile(page, []byte(fmt.Sprintf(TELEGRAM_QR_PAGE_HTML, version)), TELEGRAM_QR_PERMS); err != nil {
		return err
	}
	fmt.Fprintf(output, TELEGRAM_QR_SHOWN+"\n", page)
	return nil
}
