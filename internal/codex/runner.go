package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var errOutputTooLarge = errors.New("codex output exceeded limit")

// RunRequest describes a single Codex CLI invocation.
type RunRequest struct {
	Path           string
	Args           []string
	Dir            string
	Stdin          string
	Env            []string
	MaxOutputBytes int64
}

// RunResult contains captured process output.
type RunResult struct {
	Stdout string
	Stderr string
}

// Runner executes Codex. Tests provide fakes for deterministic behavior.
type Runner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// CLIRunner executes the local codex CLI.
type CLIRunner struct{}

func (CLIRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	cmd := exec.CommandContext(ctx, req.Path, req.Args...)
	cmd.Dir = req.Dir
	cmd.Env = req.Env
	cmd.Stdin = strings.NewReader(req.Stdin)

	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	stdout := &limitedBuffer{limit: limit}
	stderr := &limitedBuffer{limit: limit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result := RunResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if errors.Is(stdout.err, errOutputTooLarge) || errors.Is(stderr.err, errOutputTooLarge) {
		return result, errOutputTooLarge
	}
	if err != nil {
		return result, fmt.Errorf("run codex: %w", err)
	}
	return result, nil
}

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int64
	err   error
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	if int64(b.buf.Len()+len(p)) > b.limit {
		remaining := int(b.limit) - b.buf.Len()
		if remaining > 0 {
			_, _ = b.buf.Write(p[:remaining])
		}
		b.err = errOutputTooLarge
		return 0, b.err
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

func safeEnv() []string {
	allowExact := map[string]bool{
		"PATH":            true,
		"HOME":            true,
		"CODEX_HOME":      true,
		"XDG_CONFIG_HOME": true,
		"XDG_DATA_HOME":   true,
		"XDG_CACHE_HOME":  true,
		"TMPDIR":          true,
		"TEMP":            true,
		"TMP":             true,
		"SSL_CERT_FILE":   true,
		"SSL_CERT_DIR":    true,
		"LANG":            true,
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if allowExact[k] || strings.HasPrefix(k, "LC_") {
			env = append(env, kv)
		}
	}
	return env
}
