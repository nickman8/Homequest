package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"
)

type FasterWhisper struct{ Python, Script, TempDir string }

func (s *FasterWhisper) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if len(audio) == 0 || len(audio) > 4<<20 {
		return "", errors.New("voice size limit")
	}
	f, e := os.CreateTemp(s.TempDir, "voice-*.ogg")
	if e != nil {
		return "", errors.New("STT temp unavailable")
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(audio); e != nil {
		f.Close()
		return "", errors.New("STT temp unavailable")
	}
	f.Close()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Python, s.Script, f.Name())
	var out bytes.Buffer
	cmd.Stdout = &out
	// stderr is intentionally discarded; no audio/paths enter application logs.
	if e = cmd.Run(); e != nil {
		return "", errors.New("STT unavailable")
	}
	if out.Len() > 8192 {
		return "", errors.New("STT response limit")
	}
	var result struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Text == "" {
		return "", errors.New("STT empty response")
	}
	return result.Text, nil
}
