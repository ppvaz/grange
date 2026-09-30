package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Ask runs r with a prompt that asks for an answer, and returns the answer.
// The agent writes it to a file, which every backend can do, so no CLI's
// output format has to be parsed.
func Ask(ctx context.Context, r Run) (string, error) {
	tmp, err := os.MkdirTemp("", "grange-answer-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	answer := filepath.Join(tmp, "answer.md")

	r.Inv.Prompt += fmt.Sprintf("\n\nWrite your final answer, and nothing else, to the file %s. Don't create or modify any other file.\n", answer)
	if err := Exec(ctx, r); err != nil {
		return "", fmt.Errorf("%s: %w", r.Spec, err)
	}
	out, err := os.ReadFile(answer)
	if err != nil {
		return "", fmt.Errorf("%s finished without writing an answer to %s", r.Spec, answer)
	}
	return string(out), nil
}
