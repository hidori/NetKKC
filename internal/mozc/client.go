package mozc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/hidori/NetKKC/internal/errs"
	"google.golang.org/protobuf/encoding/protowire"
)

const maxResponseBytes = 4 << 20

var ipcKey = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Client talks to a dedicated Linux mozc_server profile. Each call opens a new
// Unix connection; the server's wire framing uses EOF in each direction.
type Client struct {
	Profile string
	Timeout time.Duration
}

func (c *Client) call(ctx context.Context, input []byte) (commandOutput, error) {
	path, err := os.ReadFile(filepath.Join(c.Profile, ".session.ipc"))
	if err != nil {
		return commandOutput{}, fmt.Errorf("read Mozc IPC discovery: %w", err)
	}

	fields, err := decode(path)
	if err != nil {
		return commandOutput{}, fmt.Errorf("decode Mozc IPC discovery: %w", err)
	}

	var key string

	for _, f := range fields {
		if f.number == 1 {
			key = string(f.data)
		}
	}

	if !ipcKey.MatchString(key) {
		return commandOutput{}, errors.New("invalid Mozc IPC discovery key")
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", "@tmp/.mozc."+key+".session")
	if err != nil {
		return commandOutput{}, fmt.Errorf("connect to Mozc: %w", err)
	}

	defer func() { _ = conn.Close() }()

	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return commandOutput{}, errs.Wrap(err)
	}

	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	if _, err := io.Copy(conn, bytes.NewReader(input)); err != nil {
		return commandOutput{}, fmt.Errorf("send Mozc request: %w", err)
	}

	if err := conn.(*net.UnixConn).CloseWrite(); err != nil {
		return commandOutput{}, errs.Wrap(err)
	}

	data, err := io.ReadAll(io.LimitReader(conn, maxResponseBytes+1))
	if err != nil {
		return commandOutput{}, fmt.Errorf("receive Mozc response: %w", err)
	}

	if len(data) == 0 || len(data) > maxResponseBytes {
		return commandOutput{}, errors.New("invalid Mozc response size")
	}

	return decodeOutput(data)
}

// Configure applies stateless conversion settings only to the dedicated profile.
// Do not point this client at a user's desktop IME profile.
func (c *Client) Configure(ctx context.Context) error {
	// Incognito + NO_HISTORY disable use/update of personal history. MSIME fixes
	// the key bindings used for focus and resize commands across installations.
	config := number(nil, configIncognitoMode, 1)
	config = number(config, configSessionKeymap, sessionKeymapMSIME)
	config = number(config, configHistoryLearningLevel, historyNoHistory)
	input := number(nil, inputType, commandSetConfig)

	out, err := c.call(ctx, message(input, inputConfig, config))
	if err != nil {
		return err
	}

	fields, err := decode(out.config)
	if err != nil {
		return err
	}

	settings := make(map[protowire.Number]uint64)
	for _, f := range fields {
		settings[f.number] = f.value
	}

	if settings[configIncognitoMode] != 1 || settings[configSessionKeymap] != sessionKeymapMSIME ||
		settings[configHistoryLearningLevel] != historyNoHistory {
		return errors.New("mozc did not enable incognito, NO_HISTORY and MSIME key bindings")
	}

	return nil
}

type session struct {
	client *Client
	id     uint64
}

func (c *Client) create(ctx context.Context) (*session, error) {
	out, err := c.call(ctx, number(nil, inputType, commandCreateSession))
	if err != nil {
		return nil, err
	}

	if out.id == 0 {
		return nil, errors.New("mozc returned an empty session ID")
	}

	return &session{client: c, id: out.id}, nil
}

// close deletes the session even after ctx is canceled.
func (s *session) close(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()

	_, _ = s.client.call(ctx, number(number(nil, inputType, commandDeleteSession), inputID, s.id))
}

func (s *session) command(ctx context.Context, kind uint64, args []byte) (commandOutput, error) {
	input := number(number(nil, inputType, commandSendCommand), inputID, s.id)

	return s.client.call(ctx, message(input, inputCommand, append(number(nil, sessionCommandType, kind), args...)))
}

func (s *session) key(ctx context.Context, key []byte) (commandOutput, error) {
	input := number(number(nil, inputType, commandSendKey), inputID, s.id)
	input = number(input, inputRequestSuggestion, 0)

	return s.client.call(ctx, message(input, inputKey, key))
}

func (s *session) begin(ctx context.Context, reading string) (commandOutput, error) {
	if _, err := s.command(ctx, sessionCommandTurnOnIME, number(nil, sessionCommandCompositionMode, compositionModeHiragana)); err != nil {
		return commandOutput{}, err
	}
	// AS_IS preserves punctuation, spaces, mixed scripts and literal input.
	if _, err := s.key(ctx, number(message(nil, keyKeyString, []byte(reading)), keyInputStyle, inputStyleAsIs)); err != nil {
		return commandOutput{}, err
	}

	return s.key(ctx, number(nil, keySpecialKey, specialKeySpace)) // SPACE starts conversion
}
