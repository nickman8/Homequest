// Package widget implements the Bot API extension absent from upstream's SDK.
package widget

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

func Validate(html []byte, state, request string) error {
	if len(html) > 32768 || len(state) > 1024 || len(request) > 64 {
		return errors.New("widget byte budget exceeded")
	}
	var b bytes.Buffer
	z, _ := zlib.NewWriterLevel(&b, 9)
	_, _ = z.Write(html)
	_ = z.Close()
	if b.Len() > 4096 {
		return errors.New("widget compressed budget exceeded")
	}
	return nil
}

type Client struct {
	token string
	http  *http.Client
}

func NewClient(token string, c *http.Client) *Client {
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{token: token, http: c}
}

// Call deliberately never returns the underlying network error, response body,
// or URL: all may contain bot credentials or private family data.
func (c *Client) Call(ctx context.Context, method string, payload any, result any) error {
	switch method {
	case "getFile", "getMe", "getWebhookInfo", "sendWidget", "editMessageWidgetData", "answerWidgetQuery", "getUpdates", "sendMessage", "sendPhoto", "answerCallbackQuery":
	default:
		return errors.New("unsupported Telegram method")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.New("invalid Telegram payload")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+c.token+"/"+method, bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid Telegram configuration")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("Telegram transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var body struct {
			Description string `json:"description"`
			Parameters  struct {
				RetryAfter int `json:"retry_after"`
			} `json:"parameters"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
		reason := ""
		if resp.StatusCode == 400 && strings.Contains(body.Description, "INPUT_MEDIA_WIDGET_NOT_ALLOWED") {
			reason = "widget_not_allowed"
		}
		return &APIError{Code: resp.StatusCode, RetryAfter: body.Parameters.RetryAfter, Reason: reason}
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope) != nil || !envelope.OK {
		return errors.New("invalid Telegram response")
	}
	if result != nil && json.Unmarshal(envelope.Result, result) != nil {
		return errors.New("invalid Telegram result")
	}
	return nil
}

type APIError struct {
	Reason     string
	Code       int
	RetryAfter int
}

func (e *APIError) Error() string { return fmt.Sprintf("Telegram HTTP %d", e.Code) }

// Voice downloads use a file path returned by Telegram for this bot. No caller
// supplied URL or redirect may carry the token to another host.
func (c *Client) Voice(ctx context.Context, fileID string) ([]byte, error) {
	if fileID == "" || len(fileID) > 512 {
		return nil, errors.New("invalid voice file ID")
	}
	var f struct {
		Path string `json:"file_path"`
		Size int    `json:"file_size"`
	}
	if e := c.Call(ctx, "getFile", map[string]any{"file_id": fileID}, &f); e != nil {
		return nil, e
	}
	if f.Size > 4<<20 || f.Path == "" || path.Clean(f.Path) != f.Path || strings.ContainsAny(f.Path, "?:#\\") || strings.HasPrefix(f.Path, "/") || strings.Contains(f.Path, "..") {
		return nil, errors.New("voice file unavailable")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://api.telegram.org/file/bot"+c.token+"/"+f.Path, nil)
	if e != nil {
		return nil, errors.New("voice unavailable")
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, e := client.Do(req)
	if e != nil {
		return nil, errors.New("voice transport unavailable")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, errors.New("voice download rejected")
	}
	raw, e := io.ReadAll(io.LimitReader(r.Body, (4<<20)+1))
	if e != nil || len(raw) > 4<<20 {
		return nil, errors.New("voice size limit")
	}
	return raw, nil
}
