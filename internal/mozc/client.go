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
)

const maxResponseBytes = 4 << 20

var ipcKey = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Client talks to a dedicated Linux mozc_server profile. Each call opens a new
// Unix connection; the server's wire framing uses EOF in each direction.
type Client struct {
	Profile string
	Timeout time.Duration
}

func (c *Client) call(ctx context.Context, input []byte) (output, error) {
	path, err := os.ReadFile(filepath.Join(c.Profile, ".session.ipc"))
	if err != nil {
		return output{}, fmt.Errorf("read Mozc IPC discovery: %w", err)
	}
	fields, err := decode(path)
	if err != nil {
		return output{}, fmt.Errorf("decode Mozc IPC discovery: %w", err)
	}
	var key string
	for _, f := range fields {
		if f.number == 1 {
			key = string(f.data)
		}
	}
	if !ipcKey.MatchString(key) {
		return output{}, errors.New("invalid Mozc IPC discovery key")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", "@tmp/.mozc."+key+".session")
	if err != nil {
		return output{}, fmt.Errorf("connect to Mozc: %w", err)
	}
	defer func() { _ = conn.Close() }()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return output{}, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if _, err := io.Copy(conn, bytes.NewReader(input)); err != nil {
		return output{}, fmt.Errorf("send Mozc request: %w", err)
	}
	if err := conn.(*net.UnixConn).CloseWrite(); err != nil {
		return output{}, err
	}
	data, err := io.ReadAll(io.LimitReader(conn, maxResponseBytes+1))
	if err != nil {
		return output{}, fmt.Errorf("receive Mozc response: %w", err)
	}
	if len(data) == 0 || len(data) > maxResponseBytes {
		return output{}, errors.New("invalid Mozc response size")
	}
	return decodeOutput(data)
}

// Configure applies stateless conversion settings only to the dedicated profile.
// Do not point this client at a user's desktop IME profile.
func (c *Client) Configure(ctx context.Context) error {
	// Incognito + NO_HISTORY disable use/update of personal history. MSIME fixes
	// the key bindings used for focus and resize commands across installations.
	config := number(nil, 20, 1)
	config = number(config, 41, 2)
	config = number(config, 50, 2)
	input := number(nil, 1, 7) // SET_CONFIG
	out, err := c.call(ctx, message(input, 5, config))
	if err != nil {
		return err
	}
	fields, err := decode(out.config)
	if err != nil {
		return err
	}
	settings := make(map[int]uint64)
	for _, f := range fields {
		settings[int(f.number)] = f.value
	}
	if settings[20] != 1 || settings[41] != 2 || settings[50] != 2 {
		return errors.New("mozc did not enable incognito, NO_HISTORY and MSIME key bindings")
	}
	return nil
}

type session struct {
	client *Client
	id     uint64
}

func (c *Client) create(ctx context.Context) (*session, error) {
	out, err := c.call(ctx, number(nil, 1, 1)) // CREATE_SESSION
	if err != nil {
		return nil, err
	}
	if out.id == 0 {
		return nil, errors.New("mozc returned an empty session ID")
	}
	return &session{client: c, id: out.id}, nil
}

func (s *session) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = s.client.call(ctx, number(number(nil, 1, 2), 2, s.id)) // DELETE_SESSION
}

func (s *session) command(ctx context.Context, kind uint64, args []byte) (output, error) {
	input := number(number(nil, 1, 5), 2, s.id) // SEND_COMMAND
	return s.client.call(ctx, message(input, 4, append(number(nil, 1, kind), args...)))
}

func (s *session) key(ctx context.Context, key []byte) (output, error) {
	input := number(number(nil, 1, 3), 2, s.id) // SEND_KEY
	input = number(input, 14, 0)                // request_suggestion=false
	return s.client.call(ctx, message(input, 3, key))
}

func (s *session) begin(ctx context.Context, reading string) (output, error) {
	if _, err := s.command(ctx, 22, number(nil, 3, 1)); err != nil { // TURN_ON_IME, HIRAGANA
		return output{}, err
	}
	// AS_IS preserves punctuation, spaces, mixed scripts and literal input.
	if _, err := s.key(ctx, number(message(nil, 5, []byte(reading)), 6, 1)); err != nil {
		return output{}, err
	}
	return s.key(ctx, number(nil, 3, 4)) // SPACE starts conversion
}
