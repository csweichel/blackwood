package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/csweichel/blackwood/internal/config"
)

// Runner executes a sequence of hooks for each detected file.
type Runner struct {
	hooks []config.Hook
}

// New creates a Runner with the given hook definitions.
func New(hooks []config.Hook) *Runner {
	return &Runner{hooks: hooks}
}

// Run executes each hook sequentially for filePath. It sets the
// BLACKWOOD_FILE environment variable and appends filePath as the last
// argument. Execution stops on the first hook failure.
func (r *Runner) Run(ctx context.Context, filePath string) error {
	for i, h := range r.hooks {
		args := append(h.Args, filePath)
		cmd := exec.CommandContext(ctx, h.Command, args...)
		cmd.Env = append(cmd.Environ(), "BLACKWOOD_FILE="+filePath)

		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			slog.Info("hook output",
				slog.Int("index", i),
				slog.String("command", h.Command),
				slog.String("output", string(out)),
			)
		}
		if err != nil {
			return fmt.Errorf("hook[%d] %s failed: %w", i, h.Command, err)
		}
	}
	return nil
}
