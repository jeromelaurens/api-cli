package runner

import (
	"context"
	"fmt"
	"io"
	"os"
)

type CLI struct {
	In              io.Reader
	Out             io.Writer
	Err             io.Writer
	Env             map[string]string
	HomeDir         string
	ProfilesPath    string
	CredentialStore CredentialStore
	HTTPClient      HTTPDoer
}

func NewCLI() *CLI {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		for i, r := range entry {
			if r == '=' {
				env[entry[:i]] = entry[i+1:]
				break
			}
		}
	}
	home, _ := os.UserHomeDir()
	return &CLI{
		In:              os.Stdin,
		Out:             os.Stdout,
		Err:             os.Stderr,
		Env:             env,
		HomeDir:         home,
		CredentialStore: KeyringCredentialStore{},
		HTTPClient:      defaultHTTPClient(),
	}
}

func (c *CLI) Run(ctx context.Context, args []string) int {
	if c.In == nil {
		c.In = io.Reader(os.Stdin)
	}
	if c.Out == nil {
		c.Out = io.Writer(os.Stdout)
	}
	if c.Err == nil {
		c.Err = io.Writer(os.Stderr)
	}
	if c.CredentialStore == nil {
		c.CredentialStore = KeyringCredentialStore{}
	}
	if c.HTTPClient == nil {
		c.HTTPClient = defaultHTTPClient()
	}

	global, rest, err := parseGlobalArgs(args)
	if err != nil {
		return c.writeAdminError(err, false)
	}
	c.ProfilesPath = ResolveProfilesPath(global.ProfilesPath, c.Env, c.HomeDir)

	if global.Help || len(rest) == 0 {
		fmt.Fprint(c.Out, HelpText(""))
		return 0
	}
	if rest[0] == "help" {
		topic := ""
		if len(rest) > 1 {
			topic = rest[1]
		}
		fmt.Fprint(c.Out, HelpText(topic))
		return 0
	}

	switch rest[0] {
	case "request":
		return c.runRequest(ctx, rest[1:])
	case "profile":
		return c.runProfile(rest[1:])
	case "secret":
		return c.runSecret(rest[1:])
	default:
		return c.writeAdminError(codedError(CodeInvalidArgs, "unknown command: %s", rest[0]), false)
	}
}

type globalArgs struct {
	ProfilesPath string
	Help         bool
}

func parseGlobalArgs(args []string) (globalArgs, []string, error) {
	var global globalArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			global.Help = true
		case arg == "--profiles":
			if i+1 >= len(args) {
				return global, nil, codedError(CodeInvalidArgs, "--profiles requires a path")
			}
			i++
			global.ProfilesPath = args[i]
		case len(arg) > len("--profiles=") && arg[:len("--profiles=")] == "--profiles=":
			global.ProfilesPath = arg[len("--profiles="):]
		default:
			return global, args[i:], nil
		}
	}
	return global, nil, nil
}
