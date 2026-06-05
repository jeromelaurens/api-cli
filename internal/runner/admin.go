package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

func (c *CLI) runProfile(args []string) int {
	if hasHelp(args) {
		fmt.Fprint(c.Out, HelpText("profile"))
		return 0
	}
	jsonMode, args := consumeBoolFlag(args, "--json")
	if len(args) == 0 {
		return c.writeAdminError(codedError(CodeInvalidArgs, "profile subcommand is required"), jsonMode)
	}

	switch args[0] {
	case "path":
		if len(args) != 1 {
			return c.writeAdminError(codedError(CodeInvalidArgs, "profile path does not accept arguments"), jsonMode)
		}
		_, err := os.Stat(c.ProfilesPath)
		exists := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return c.writeAdminError(wrapError(CodeProfileCatalogInvalid, err, "cannot stat profile catalog at %s", c.ProfilesPath), jsonMode)
		}
		if jsonMode {
			return c.writeAdminJSON(map[string]any{"path": c.ProfilesPath, "exists": exists})
		}
		fmt.Fprintf(c.Out, "Profiles catalog: %s\nExists: %t\n", c.ProfilesPath, exists)
		return 0
	case "list":
		if len(args) != 1 {
			return c.writeAdminError(codedError(CodeInvalidArgs, "profile list does not accept arguments"), jsonMode)
		}
		catalog, err := LoadCatalog(c.ProfilesPath)
		if err != nil {
			if codeOf(err) == CodeProfileCatalogMissing {
				if jsonMode {
					return c.writeAdminJSON(map[string]any{"profiles": []string{}, "catalogExists": false})
				}
				fmt.Fprintf(c.Out, "Profiles catalog: %s\nProfiles: (none)\n", c.ProfilesPath)
				return 0
			}
			return c.writeAdminError(err, jsonMode)
		}
		names := sortedProfileNames(catalog)
		if jsonMode {
			return c.writeAdminJSON(map[string]any{"profiles": names, "catalogExists": true})
		}
		fmt.Fprintf(c.Out, "Profiles catalog: %s\nProfiles:\n", c.ProfilesPath)
		for _, name := range names {
			fmt.Fprintf(c.Out, "  %s\n", name)
		}
		return 0
	case "show":
		if len(args) != 2 {
			return c.writeAdminError(codedError(CodeInvalidArgs, "usage: api profile show <profile>"), jsonMode)
		}
		catalog, err := LoadCatalog(c.ProfilesPath)
		if err != nil {
			if codeOf(err) == CodeProfileCatalogMissing {
				err = codedError(CodeProfileNotFound, "profile %s not found", args[1])
			}
			return c.writeAdminError(err, jsonMode)
		}
		profile, err := lookupProfile(catalog, args[1])
		if err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		if jsonMode {
			return c.writeAdminJSON(profile)
		}
		fmt.Fprintf(c.Out, "Profile: %s\nService: %s\nBase URL: %s\nRead only: %t\nAuth scheme: %s\n", args[1], profile.Service, profile.BaseURL, profile.ReadOnly, profile.Auth.Scheme)
		return 0
	case "create":
		return c.runProfileCreate(args[1:], jsonMode)
	default:
		return c.writeAdminError(codedError(CodeInvalidArgs, "unknown profile subcommand: %s", args[0]), jsonMode)
	}
}

type profileCreateOptions struct {
	Name               string
	Service            string
	BaseURL            string
	ReadOnly           bool
	AuthScheme         string
	Header             string
	Param              string
	Credential         string
	UsernameCredential string
	PasswordCredential string
}

func (c *CLI) runProfileCreate(args []string, jsonMode bool) int {
	opts, err := parseProfileCreateArgs(args)
	if err != nil {
		return c.writeAdminError(err, jsonMode)
	}

	profile, err := c.buildProfileFromCreateOptions(opts)
	if err != nil {
		return c.writeAdminError(err, jsonMode)
	}
	catalog, _, err := LoadCatalogOrEmpty(c.ProfilesPath)
	if err != nil {
		return c.writeAdminError(err, jsonMode)
	}
	if _, exists := catalog.Profiles[opts.Name]; exists {
		return c.writeAdminError(codedError(CodeProfileCatalogInvalid, "profile %s already exists", opts.Name), jsonMode)
	}
	catalog.Profiles[opts.Name] = profile
	if err := SaveCatalog(c.ProfilesPath, *catalog); err != nil {
		return c.writeAdminError(err, jsonMode)
	}

	commands := secretCommandsForProfile(opts.Name, profile)
	if jsonMode {
		return c.writeAdminJSON(map[string]any{
			"profile":        opts.Name,
			"path":           c.ProfilesPath,
			"created":        true,
			"secretCommands": commands,
		})
	}
	fmt.Fprintf(c.Out, "Created profile %s in %s\n", opts.Name, c.ProfilesPath)
	if len(commands) > 0 {
		fmt.Fprintln(c.Out, "Next credential commands:")
		for _, command := range commands {
			fmt.Fprintf(c.Out, "  %s\n", command)
		}
	}
	return 0
}

func parseProfileCreateArgs(args []string) (profileCreateOptions, error) {
	var opts profileCreateOptions
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--service":
			value, next, err := requireFlagValue(args, i, "--service")
			if err != nil {
				return opts, err
			}
			opts.Service = value
			i = next
		case strings.HasPrefix(arg, "--service="):
			opts.Service = strings.TrimPrefix(arg, "--service=")
		case arg == "--base-url":
			value, next, err := requireFlagValue(args, i, "--base-url")
			if err != nil {
				return opts, err
			}
			opts.BaseURL = value
			i = next
		case strings.HasPrefix(arg, "--base-url="):
			opts.BaseURL = strings.TrimPrefix(arg, "--base-url=")
		case arg == "--auth":
			value, next, err := requireFlagValue(args, i, "--auth")
			if err != nil {
				return opts, err
			}
			opts.AuthScheme = value
			i = next
		case strings.HasPrefix(arg, "--auth="):
			opts.AuthScheme = strings.TrimPrefix(arg, "--auth=")
		case arg == "--header":
			value, next, err := requireFlagValue(args, i, "--header")
			if err != nil {
				return opts, err
			}
			opts.Header = value
			i = next
		case strings.HasPrefix(arg, "--header="):
			opts.Header = strings.TrimPrefix(arg, "--header=")
		case arg == "--param":
			value, next, err := requireFlagValue(args, i, "--param")
			if err != nil {
				return opts, err
			}
			opts.Param = value
			i = next
		case strings.HasPrefix(arg, "--param="):
			opts.Param = strings.TrimPrefix(arg, "--param=")
		case arg == "--credential":
			value, next, err := requireFlagValue(args, i, "--credential")
			if err != nil {
				return opts, err
			}
			opts.Credential = value
			i = next
		case strings.HasPrefix(arg, "--credential="):
			opts.Credential = strings.TrimPrefix(arg, "--credential=")
		case arg == "--username-credential":
			value, next, err := requireFlagValue(args, i, "--username-credential")
			if err != nil {
				return opts, err
			}
			opts.UsernameCredential = value
			i = next
		case strings.HasPrefix(arg, "--username-credential="):
			opts.UsernameCredential = strings.TrimPrefix(arg, "--username-credential=")
		case arg == "--password-credential":
			value, next, err := requireFlagValue(args, i, "--password-credential")
			if err != nil {
				return opts, err
			}
			opts.PasswordCredential = value
			i = next
		case strings.HasPrefix(arg, "--password-credential="):
			opts.PasswordCredential = strings.TrimPrefix(arg, "--password-credential=")
		case arg == "--read-only":
			opts.ReadOnly = true
		case strings.HasPrefix(arg, "-"):
			return opts, codedError(CodeInvalidArgs, "unknown profile create flag: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return opts, codedError(CodeInvalidArgs, "usage: api profile create <profile>")
	}
	if !validName(positionals[0]) {
		return opts, codedError(CodeProfileCatalogInvalid, "profile name %q must match [a-zA-Z0-9._-]+", positionals[0])
	}
	opts.Name = positionals[0]
	return opts, nil
}

func (c *CLI) buildProfileFromCreateOptions(opts profileCreateOptions) (Profile, error) {
	service, err := c.promptRequired("Service", firstNonEmpty(opts.Service, opts.Name))
	if err != nil {
		return Profile{}, err
	}
	baseURL, err := c.promptRequired("Base URL", opts.BaseURL)
	if err != nil {
		return Profile{}, err
	}
	scheme, err := c.promptRequired("Auth scheme", opts.AuthScheme)
	if err != nil {
		return Profile{}, err
	}
	auth, err := c.promptAuthConfig(strings.ToLower(scheme), opts)
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{Service: service, BaseURL: baseURL, ReadOnly: opts.ReadOnly, Auth: auth}
	if err := ValidateCatalog(Catalog{SchemaVersion: 1, Profiles: map[string]Profile{opts.Name: profile}}); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (c *CLI) promptAuthConfig(scheme string, opts profileCreateOptions) (AuthConfig, error) {
	switch scheme {
	case "bearer":
		credential, err := c.promptRequired("Credential name", opts.Credential)
		if err != nil {
			return AuthConfig{}, err
		}
		return AuthConfig{Scheme: "bearer", Credential: credential}, nil
	case "header":
		header, err := c.promptRequired("Header name", opts.Header)
		if err != nil {
			return AuthConfig{}, err
		}
		credential, err := c.promptRequired("Credential name", opts.Credential)
		if err != nil {
			return AuthConfig{}, err
		}
		return AuthConfig{Scheme: "header", Header: header, Credential: credential}, nil
	case "query":
		param, err := c.promptRequired("Query parameter", opts.Param)
		if err != nil {
			return AuthConfig{}, err
		}
		credential, err := c.promptRequired("Credential name", opts.Credential)
		if err != nil {
			return AuthConfig{}, err
		}
		return AuthConfig{Scheme: "query", Param: param, Credential: credential}, nil
	case "basic":
		username, err := c.promptRequired("Username credential name", opts.UsernameCredential)
		if err != nil {
			return AuthConfig{}, err
		}
		password, err := c.promptRequired("Password credential name", opts.PasswordCredential)
		if err != nil {
			return AuthConfig{}, err
		}
		return AuthConfig{Scheme: "basic", UsernameCredential: username, PasswordCredential: password}, nil
	default:
		return AuthConfig{}, codedError(CodeProfileCatalogInvalid, "auth.scheme is not supported: %s", scheme)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (c *CLI) promptRequired(label string, defaultValue string) (string, error) {
	value, err := c.readPromptLine(label, defaultValue)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", codedError(CodeInvalidArgs, "%s is required", label)
	}
	return value, nil
}

func (c *CLI) readPromptLine(label string, defaultValue string) (string, error) {
	if file, ok := c.In.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		if defaultValue == "" {
			fmt.Fprintf(c.Err, "%s: ", label)
		} else {
			fmt.Fprintf(c.Err, "%s [%s]: ", label, defaultValue)
		}
	}

	var builder strings.Builder
	buffer := make([]byte, 1)
	for {
		n, err := c.In.Read(buffer)
		if n > 0 {
			if buffer[0] == '\n' {
				break
			}
			builder.WriteByte(buffer[0])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", wrapError(CodeInvalidArgs, err, "cannot read %s", label)
		}
	}
	value := strings.TrimSpace(builder.String())
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func secretCommandsForProfile(profileName string, profile Profile) []string {
	credentials := sortedCredentials(profile)
	commands := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		commands = append(commands, fmt.Sprintf("api secret set %s %s", profileName, credential))
	}
	return commands
}

func (c *CLI) runSecret(args []string) int {
	if hasHelp(args) {
		fmt.Fprint(c.Out, HelpText("secret"))
		return 0
	}
	jsonMode, args := consumeBoolFlag(args, "--json")
	check, args := consumeBoolFlag(args, "--check")
	if len(args) == 0 {
		return c.writeAdminError(codedError(CodeInvalidArgs, "secret subcommand is required"), jsonMode)
	}

	switch args[0] {
	case "set":
		if len(args) != 3 {
			return c.writeAdminError(codedError(CodeInvalidArgs, "usage: api secret set <profile> <credential>"), jsonMode)
		}
		if err := c.validateCredentialRef(args[1], args[2]); err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		value, err := c.readCredentialValue()
		if err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		key := credentialKey(args[1], args[2])
		if err := c.CredentialStore.Set(key, value); err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		if jsonMode {
			return c.writeAdminJSON(map[string]any{"profile": args[1], "credential": args[2], "stored": true})
		}
		fmt.Fprintf(c.Out, "Stored credential %s\n", key)
		return 0
	case "list":
		if len(args) > 2 {
			return c.writeAdminError(codedError(CodeInvalidArgs, "usage: api secret list [profile]"), jsonMode)
		}
		profileFilter := ""
		if len(args) == 2 {
			profileFilter = args[1]
		}
		return c.runSecretList(profileFilter, check, jsonMode)
	case "delete":
		if len(args) != 3 {
			return c.writeAdminError(codedError(CodeInvalidArgs, "usage: api secret delete <profile> <credential>"), jsonMode)
		}
		if err := c.validateCredentialRef(args[1], args[2]); err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		key := credentialKey(args[1], args[2])
		if err := c.CredentialStore.Delete(key); err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		if jsonMode {
			return c.writeAdminJSON(map[string]any{"profile": args[1], "credential": args[2], "deleted": true})
		}
		fmt.Fprintf(c.Out, "Deleted credential %s\n", key)
		return 0
	default:
		return c.writeAdminError(codedError(CodeInvalidArgs, "unknown secret subcommand: %s", args[0]), jsonMode)
	}
}

func (c *CLI) runSecretList(profileFilter string, check bool, jsonMode bool) int {
	catalog, err := LoadCatalog(c.ProfilesPath)
	if err != nil {
		return c.writeAdminError(err, jsonMode)
	}
	names := sortedProfileNames(catalog)
	if profileFilter != "" {
		if _, err := lookupProfile(catalog, profileFilter); err != nil {
			return c.writeAdminError(err, jsonMode)
		}
		names = []string{profileFilter}
	}

	if jsonMode {
		if profileFilter != "" {
			payload, err := c.secretProfilePayload(catalog, profileFilter, check)
			if err != nil {
				return c.writeAdminError(err, jsonMode)
			}
			return c.writeAdminJSON(payload)
		}
		profiles := make([]secretProfilePayload, 0, len(names))
		for _, name := range names {
			payload, err := c.secretProfilePayload(catalog, name, check)
			if err != nil {
				return c.writeAdminError(err, jsonMode)
			}
			profiles = append(profiles, payload)
		}
		return c.writeAdminJSON(map[string]any{"profiles": profiles})
	}

	for _, name := range names {
		profile := catalog.Profiles[name]
		fmt.Fprintf(c.Out, "Profile: %s\n", name)
		for _, credential := range sortedCredentials(profile) {
			if check {
				present, err := c.credentialPresent(name, credential)
				if err != nil {
					return c.writeAdminError(err, jsonMode)
				}
				fmt.Fprintf(c.Out, "  %s: present=%t\n", credential, present)
			} else {
				fmt.Fprintf(c.Out, "  %s\n", credential)
			}
		}
	}
	return 0
}

type secretProfilePayload struct {
	Profile     string                     `json:"profile"`
	Credentials []secretCredentialListItem `json:"credentials"`
}

type secretCredentialListItem struct {
	Name    string `json:"name"`
	Present *bool  `json:"present,omitempty"`
}

func (c *CLI) secretProfilePayload(catalog *Catalog, profileName string, check bool) (secretProfilePayload, error) {
	profile := catalog.Profiles[profileName]
	credentials := sortedCredentials(profile)
	items := make([]secretCredentialListItem, 0, len(credentials))
	for _, credential := range credentials {
		item := secretCredentialListItem{Name: credential}
		if check {
			present, err := c.credentialPresent(profileName, credential)
			if err != nil {
				return secretProfilePayload{}, err
			}
			item.Present = &present
		}
		items = append(items, item)
	}
	return secretProfilePayload{Profile: profileName, Credentials: items}, nil
}

func (c *CLI) credentialPresent(profileName string, credential string) (bool, error) {
	_, err := c.CredentialStore.Get(credentialKey(profileName, credential))
	if err == nil {
		return true, nil
	}
	if codeOf(err) == CodeCredentialMissing {
		return false, nil
	}
	return false, err
}

func (c *CLI) validateCredentialRef(profileName string, credential string) error {
	catalog, err := LoadCatalog(c.ProfilesPath)
	if err != nil {
		return err
	}
	profile, err := lookupProfile(catalog, profileName)
	if err != nil {
		return err
	}
	for _, expected := range requiredCredentials(profile) {
		if credential == expected {
			return nil
		}
	}
	return codedError(CodeProfileCatalogInvalid, "credential %s is not declared by profile %s", credential, profileName)
}

func (c *CLI) readCredentialValue() (string, error) {
	if file, ok := c.In.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(c.Err, "Credential: ")
		value, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(c.Err)
		if err != nil {
			return "", wrapError(CodeInvalidArgs, err, "cannot read credential")
		}
		if len(value) == 0 {
			return "", codedError(CodeInvalidArgs, "credential value cannot be empty")
		}
		return string(value), nil
	}
	data, err := io.ReadAll(c.In)
	if err != nil {
		return "", wrapError(CodeInvalidArgs, err, "cannot read credential from stdin")
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return "", codedError(CodeInvalidArgs, "credential value cannot be empty")
	}
	return value, nil
}

func sortedCredentials(profile Profile) []string {
	credentials := append([]string(nil), requiredCredentials(profile)...)
	sort.Strings(credentials)
	return credentials
}

func (c *CLI) writeAdminError(err error, jsonMode bool) int {
	if jsonMode {
		writeJSON(c.Out, errorPayload(err))
		return 1
	}
	fmt.Fprintf(c.Err, "error: %s: %s\n", codeOf(err), errorMessage(err))
	return 1
}

func (c *CLI) writeAdminJSON(payload any) int {
	writeJSON(c.Out, payload)
	return 0
}

func writeJSON(out io.Writer, payload any) {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func errorPayload(err error) map[string]any {
	return map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    codeOf(err),
			"message": errorMessage(err),
		},
	}
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
		return e.Message
	}
	return err.Error()
}

func consumeBoolFlag(args []string, flag string) (bool, []string) {
	found := false
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == flag {
			found = true
			continue
		}
		rest = append(rest, arg)
	}
	return found, rest
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}
