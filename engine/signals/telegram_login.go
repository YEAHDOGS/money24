package signals

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	TELEGRAM_LOGIN_POLL      = time.Second
	TELEGRAM_LOGIN_TIMEOUT   = 10 * time.Minute
	TELEGRAM_LOGIN_BYTES     = 4096
	TELEGRAM_LOGIN_ENV       = "Telegram login requires TELEGRAM_API_ID, TELEGRAM_API_HASH and TELEGRAM_PHONE"
	TELEGRAM_LOGIN_CODE      = "Telegram login requires --telegram-code-file"
	TELEGRAM_CODE_PROMPT     = "Telegram sent a login code. Write it to the file specified by --telegram-code-file."
	TELEGRAM_PASSWORD_PROMPT = "Telegram requires a two-step password. Write it to the file specified by --telegram-password-file."
	TELEGRAM_LOGIN_PASSWORD  = "Telegram requires --telegram-password-file for two-step verification"
	TELEGRAM_LOGIN_FILE      = "could not read the Telegram login input file"
	TELEGRAM_LOGIN_WAIT      = "Telegram login stopped while waiting for the input file"
	TELEGRAM_LOGIN_FAILED    = "Telegram login failed (check your API credentials and login inputs)"
	TELEGRAM_LOGIN_RPC       = "Telegram login failed (RPC %d)"
	TELEGRAM_LOGIN_SAVED     = "Telegram session saved."
	TELEGRAM_SESSION_WRITE   = "could not save the private Telegram session"
	TELEGRAM_SESSION_TEMP    = ".telegram.session-*"
	TELEGRAM_UTF8_BOM        = "\ufeff"

	// Where Telegram says it sent the login code. It chooses; the client cannot.
	TELEGRAM_CODE_UNKNOWN     = "Telegram did not say where it sent the code."
	TELEGRAM_CODE_APP         = "The code was sent inside the Telegram app, as a message from \"Telegram\" (blue check) on your other signed-in devices."
	TELEGRAM_CODE_SMS         = "The code was sent by SMS."
	TELEGRAM_CODE_CALL        = "The code comes by phone call."
	TELEGRAM_CODE_EMAIL       = "The code was sent to your login email (%s)."
	TELEGRAM_CODE_SETUP_EMAIL = "No code was sent: Telegram requires a login email to be set up first, in the Telegram app under Settings > Privacy and Security."
	TELEGRAM_CODE_FRAGMENT    = "The code was sent through Fragment, for anonymous numbers: %s"
	TELEGRAM_CODE_RESEND      = " A new code can be requested after %d seconds."
)

// telegramCodeWhere says where Telegram chose to deliver the login code, so the owner knows where to look.
func telegramCodeWhere(sent *tg.AuthSentCode) string {
	if sent == nil {
		return TELEGRAM_CODE_UNKNOWN
	}
	where := TELEGRAM_CODE_UNKNOWN
	switch t := sent.Type.(type) {
	case *tg.AuthSentCodeTypeApp:
		where = TELEGRAM_CODE_APP
	case *tg.AuthSentCodeTypeSMS, *tg.AuthSentCodeTypeSMSWord, *tg.AuthSentCodeTypeSMSPhrase, *tg.AuthSentCodeTypeFirebaseSMS:
		where = TELEGRAM_CODE_SMS
	case *tg.AuthSentCodeTypeCall, *tg.AuthSentCodeTypeFlashCall, *tg.AuthSentCodeTypeMissedCall:
		where = TELEGRAM_CODE_CALL
	case *tg.AuthSentCodeTypeEmailCode:
		where = fmt.Sprintf(TELEGRAM_CODE_EMAIL, t.EmailPattern)
	case *tg.AuthSentCodeTypeSetUpEmailRequired:
		where = TELEGRAM_CODE_SETUP_EMAIL
	case *tg.AuthSentCodeTypeFragmentSMS:
		where = fmt.Sprintf(TELEGRAM_CODE_FRAGMENT, t.URL)
	}
	if timeout, ok := sent.GetTimeout(); ok {
		where += fmt.Sprintf(TELEGRAM_CODE_RESEND, timeout)
	}
	return where
}

// TelegramLogin authorizes an existing account, then saves its session for the runner.
func TelegramLogin(ctx context.Context, data, codeFile, passwordFile string, output io.Writer) error {
	config := telegramConfigFromEnv(data)
	phone := strings.TrimSpace(os.Getenv(TELEGRAM_PHONE_ENV))
	if config.id <= 0 || config.hash == "" || phone == "" {
		return errors.New(TELEGRAM_LOGIN_ENV)
	}
	if codeFile == "" {
		return errors.New(TELEGRAM_LOGIN_CODE)
	}
	ctx, cancel := context.WithTimeout(ctx, TELEGRAM_LOGIN_TIMEOUT)
	defer cancel()
	disk := telegramSession{path: config.path}
	memory := &session.StorageMemory{}
	existing, err := disk.LoadSession(ctx)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return errors.New(TELEGRAM_LOGIN_FAILED)
	}
	if err == nil {
		if err := memory.StoreSession(ctx, existing); err != nil {
			return errors.New(TELEGRAM_LOGIN_FAILED)
		}
	}
	code := auth.CodeAuthenticatorFunc(func(ctx context.Context, sent *tg.AuthSentCode) (string, error) {
		fmt.Fprintln(output, TELEGRAM_CODE_PROMPT)
		fmt.Fprintln(output, telegramCodeWhere(sent))
		return readTelegramCredential(ctx, codeFile)
	})
	user := telegramLoginFiles{UserAuthenticator: auth.CodeOnly(phone, code), passwordFile: passwordFile, output: output}
	client := config.client(memory, &telegramFloodGate{busy: make(chan struct{}, 1)})
	var loginErr error
	err = client.Run(ctx, func(connected context.Context) error {
		loginErr = client.Auth().IfNecessary(connected, auth.NewFlow(user, auth.SendCodeOptions{}))
		return loginErr
	})
	switch {
	case errors.Is(loginErr, errTelegramLoginPassword):
		return errTelegramLoginPassword
	case errors.Is(loginErr, errTelegramLoginFile):
		return errTelegramLoginFile
	case errors.Is(loginErr, errTelegramLoginWait):
		return errTelegramLoginWait
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

var (
	errTelegramLoginPassword = errors.New(TELEGRAM_LOGIN_PASSWORD)
	errTelegramLoginFile     = errors.New(TELEGRAM_LOGIN_FILE)
	errTelegramLoginWait     = errors.New(TELEGRAM_LOGIN_WAIT)
)

type telegramLoginFiles struct {
	auth.UserAuthenticator
	passwordFile string
	output       io.Writer
}

func (a telegramLoginFiles) Password(ctx context.Context) (string, error) {
	if a.passwordFile == "" {
		return "", errTelegramLoginPassword
	}
	fmt.Fprintln(a.output, TELEGRAM_PASSWORD_PROMPT)
	return readTelegramCredential(ctx, a.passwordFile)
}

func readTelegramCredential(ctx context.Context, path string) (string, error) {
	for ctx.Err() == nil {
		info, err := os.Stat(path)
		switch {
		case err == nil:
			if !info.Mode().IsRegular() || info.Size() > TELEGRAM_LOGIN_BYTES {
				return "", errTelegramLoginFile
			}
			contents, err := os.ReadFile(path)
			if err != nil || len(contents) > TELEGRAM_LOGIN_BYTES {
				return "", errTelegramLoginFile
			}
			input := strings.TrimRight(strings.TrimPrefix(string(contents), TELEGRAM_UTF8_BOM), "\r\n")
			if strings.TrimSpace(input) != "" {
				return input, nil
			}
		case !errors.Is(err, os.ErrNotExist):
			return "", errTelegramLoginFile
		}
		if waitForReader(ctx, TELEGRAM_LOGIN_POLL) != nil {
			break
		}
	}
	return "", errTelegramLoginWait
}

type telegramSession struct {
	path string
}

func (s telegramSession) LoadSession(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, session.ErrNotFound
	}
	return data, err
}

func (s telegramSession) StoreSession(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), TELEGRAM_SESSION_TEMP)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = protectTelegramSession(tmp)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
