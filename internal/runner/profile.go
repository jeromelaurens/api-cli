package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

const (
	keyringService = "api-cli"
	envProfiles    = "API_PROFILES"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Catalog struct {
	SchemaVersion int                `json:"schemaVersion"`
	Profiles      map[string]Profile `json:"profiles"`
}

type Profile struct {
	Service  string     `json:"service"`
	BaseURL  string     `json:"baseURL"`
	ReadOnly bool       `json:"readOnly"`
	Auth     AuthConfig `json:"auth"`
}

type AuthConfig struct {
	Scheme             string `json:"scheme"`
	Header             string `json:"header,omitempty"`
	Param              string `json:"param,omitempty"`
	Credential         string `json:"credential,omitempty"`
	UsernameCredential string `json:"usernameCredential,omitempty"`
	PasswordCredential string `json:"passwordCredential,omitempty"`
}

func ResolveProfilesPath(explicit string, env map[string]string, home string) string {
	if explicit != "" {
		return explicit
	}
	if env != nil {
		if value := env[envProfiles]; value != "" {
			return value
		}
	}
	if home == "" {
		if detected, err := os.UserHomeDir(); err == nil {
			home = detected
		}
	}
	return filepath.Join(home, ".api", "profiles.json")
}

func LoadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, wrapError(CodeProfileCatalogMissing, err, "profile catalog not found at %s", path)
		}
		return nil, wrapError(CodeProfileCatalogInvalid, err, "cannot read profile catalog at %s", path)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, wrapError(CodeProfileCatalogInvalid, err, "profile catalog at %s is not valid JSON", path)
	}
	if err := ValidateCatalog(catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func ValidateCatalog(catalog Catalog) error {
	if catalog.SchemaVersion != 1 {
		return codedError(CodeProfileCatalogInvalid, "profile catalog schemaVersion must be 1")
	}
	if catalog.Profiles == nil {
		return codedError(CodeProfileCatalogInvalid, "profile catalog profiles must be an object")
	}
	for name, profile := range catalog.Profiles {
		if !validName(name) {
			return codedError(CodeProfileCatalogInvalid, "profile name %q must match [a-zA-Z0-9._-]+", name)
		}
		if profile.Service == "" {
			return codedError(CodeProfileCatalogInvalid, "profile %s service is required", name)
		}
		if err := validateBaseURL(name, profile.BaseURL); err != nil {
			return err
		}
		if err := validateAuth(name, profile.Auth); err != nil {
			return err
		}
	}
	return nil
}

func sortedProfileNames(catalog *Catalog) []string {
	if catalog == nil {
		return nil
	}
	names := make([]string, 0, len(catalog.Profiles))
	for name := range catalog.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func requiredCredentials(profile Profile) []string {
	switch profile.Auth.Scheme {
	case "bearer", "header", "query":
		return []string{profile.Auth.Credential}
	case "basic":
		return []string{profile.Auth.UsernameCredential, profile.Auth.PasswordCredential}
	default:
		return nil
	}
}

func validName(name string) bool {
	return namePattern.MatchString(name)
}

func validateBaseURL(profileName, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return codedError(CodeProfileCatalogInvalid, "profile %s baseURL must be absolute", profileName)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return codedError(CodeProfileCatalogInvalid, "profile %s baseURL must use http or https", profileName)
	}
	return nil
}

func validateAuth(profileName string, auth AuthConfig) error {
	switch auth.Scheme {
	case "bearer":
		return validateCredential(profileName, auth.Credential)
	case "header":
		if auth.Header == "" {
			return codedError(CodeProfileCatalogInvalid, "profile %s auth.header is required", profileName)
		}
		if !validHeaderName(auth.Header) {
			return codedError(CodeProfileCatalogInvalid, "profile %s auth.header is invalid", profileName)
		}
		return validateCredential(profileName, auth.Credential)
	case "query":
		if auth.Param == "" {
			return codedError(CodeProfileCatalogInvalid, "profile %s auth.param is required", profileName)
		}
		return validateCredential(profileName, auth.Credential)
	case "basic":
		if err := validateCredential(profileName, auth.UsernameCredential); err != nil {
			return err
		}
		return validateCredential(profileName, auth.PasswordCredential)
	default:
		return codedError(CodeProfileCatalogInvalid, "profile %s auth.scheme is not supported: %s", profileName, auth.Scheme)
	}
}

func validateCredential(profileName, credential string) error {
	if !validName(credential) {
		return codedError(CodeProfileCatalogInvalid, "profile %s credential %q must match [a-zA-Z0-9._-]+", profileName, credential)
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

func lookupProfile(catalog *Catalog, name string) (Profile, error) {
	if catalog == nil {
		return Profile{}, codedError(CodeProfileNotFound, "profile %s not found", name)
	}
	profile, ok := catalog.Profiles[name]
	if !ok {
		return Profile{}, codedError(CodeProfileNotFound, "profile %s not found", name)
	}
	return profile, nil
}

func credentialKey(profile, credential string) string {
	return fmt.Sprintf("%s/%s", profile, credential)
}
