package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// readToken is the single place a token enters the CLI: --token-file, then
// stdin when it is not a terminal, then a hidden prompt when it is. The
// token is never accepted as a flag value.
func readToken(ctx *Context) (string, error) {
	if path := ctx.Flags.TokenFile; path != "" {
		if path == "-" {
			return readTokenFromStdin(ctx)
		}
		return config.ReadTokenFile(path)
	}
	if !ctx.StdinIsTTY {
		return readTokenFromStdin(ctx)
	}
	return promptHidden(ctx, "Figma personal access token: ")
}

func readTokenFromStdin(ctx *Context) (string, error) {
	raw, err := io.ReadAll(ctx.Stdin)
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "reading token from stdin: "+err.Error())
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", figctl.New(figctl.CodeUsage, "no token provided on stdin").
			WithHint("Pipe the token on stdin (printf '%%s' \"$TOKEN\" | figctl auth login NAME) or pass --token-file PATH.")
	}
	return token, nil
}

func promptHidden(ctx *Context, prompt string) (string, error) {
	f, ok := ctx.Stdin.(*os.File)
	if !ok {
		return "", figctl.New(figctl.CodeInternal, "stdin is not a terminal file")
	}
	fmt.Fprint(ctx.Stderr, prompt)
	raw, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(ctx.Stderr)
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "reading token: "+err.Error())
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", figctl.New(figctl.CodeUsage, "no token entered").
			WithHint("Create a personal access token in Figma settings and paste it at the prompt.")
	}
	return token, nil
}

// confirm asks a yes/no question on a terminal. Without a terminal it
// requires --yes and never blocks.
func confirm(ctx *Context, question string) error {
	if ctx.Flags.Yes {
		return nil
	}
	if !ctx.StdinIsTTY {
		return figctl.Newf(figctl.CodeUsage, "confirmation required: %s", question).
			WithHint("Pass --yes to confirm without a prompt.")
	}
	fmt.Fprintf(ctx.Stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(ctx.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return figctl.Wrap(figctl.CodeInternal, err, "reading confirmation: "+err.Error())
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return figctl.New(figctl.CodeUsage, "cancelled").WithHint("Answer y to confirm, or pass --yes.")
}
