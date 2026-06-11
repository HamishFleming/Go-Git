package gitid

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Users    map[string]User `json:"users"`
	Paths    []PathRule      `json:"paths"`
	Settings Settings        `json:"settings"`
}

type User struct {
	GitName  string `json:"git_name"`
	GitEmail string `json:"git_email"`
	SSHKey   string `json:"ssh_key"`
}

type PathRule struct {
	Path      string `json:"path"`
	User      string `json:"user"`
	RemoteURL string `json:"remote_url,omitempty"`
}

type Settings struct {
	AutoOnboardOnInit  bool `json:"auto_onboard_on_init"`
	ApplyUserOnProject bool `json:"apply_user_on_project_init"`
	MapPathOnProject   bool `json:"map_path_on_project_init"`
}

func DefaultConfig() Config {
	return Config{
		Users:    map[string]User{},
		Paths:    []PathRule{},
		Settings: DefaultSettings(),
	}
}

func DefaultSettings() Settings {
	return Settings{
		AutoOnboardOnInit:  true,
		ApplyUserOnProject: true,
		MapPathOnProject:   true,
	}
}

func ConfigDir() (string, error) {
	if v := os.Getenv("GIT_ID_CONFIG_DIR"); v != "" {
		return expandPath(v)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "go-git"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return DefaultConfig(), err
	}
	if cfg.Users == nil {
		cfg.Users = map[string]User{}
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func AddUser(cfg *Config, alias string, u User) error {
	if alias == "" || strings.ContainsAny(alias, `/\\`) {
		return fmt.Errorf("invalid user alias %q", alias)
	}
	if u.GitName == "" || u.GitEmail == "" || u.SSHKey == "" {
		return fmt.Errorf("git name, git email and ssh key are required")
	}
	p, err := expandPath(u.SSHKey)
	if err != nil {
		return err
	}
	u.SSHKey = p
	cfg.Users[alias] = u
	return nil
}

func RemoveUser(cfg *Config, alias string) error {
	if _, ok := cfg.Users[alias]; !ok {
		return fmt.Errorf("unknown user %q", alias)
	}
	delete(cfg.Users, alias)
	out := cfg.Paths[:0]
	for _, r := range cfg.Paths {
		if r.User != alias {
			out = append(out, r)
		}
	}
	cfg.Paths = out
	return nil
}

func AddPath(cfg *Config, path, alias string) error {
	if _, ok := cfg.Users[alias]; !ok {
		return fmt.Errorf("unknown user %q", alias)
	}
	p, err := expandPath(path)
	if err != nil {
		return err
	}
	p, err = filepath.Abs(p)
	if err != nil {
		return err
	}
	p = filepath.Clean(p)
	remoteURL := RepoRemoteURL(p)
	for i := range cfg.Paths {
		if samePath(cfg.Paths[i].Path, p) {
			cfg.Paths[i].User = alias
			cfg.Paths[i].RemoteURL = remoteURL
			return nil
		}
	}
	cfg.Paths = append(cfg.Paths, PathRule{Path: p, User: alias, RemoteURL: remoteURL})
	sortPathRules(cfg.Paths)
	return nil
}

func RemovePath(cfg *Config, path string) error {
	p, err := expandPath(path)
	if err != nil {
		return err
	}
	p, _ = filepath.Abs(p)
	p = filepath.Clean(p)
	out := cfg.Paths[:0]
	removed := false
	for _, r := range cfg.Paths {
		if samePath(r.Path, p) {
			removed = true
			continue
		}
		out = append(out, r)
	}
	cfg.Paths = out
	if !removed {
		return fmt.Errorf("path rule not found: %s", p)
	}
	return nil
}

func Resolve(cfg Config, path string) (PathRule, User, bool, error) {
	p, err := expandPath(path)
	if err != nil {
		return PathRule{}, User{}, false, err
	}
	p, err = filepath.Abs(p)
	if err != nil {
		return PathRule{}, User{}, false, err
	}
	p = filepath.Clean(p)
	sortPathRules(cfg.Paths)
	for _, r := range cfg.Paths {
		rp := filepath.Clean(r.Path)
		if p == rp || strings.HasPrefix(p+string(os.PathSeparator), rp+string(os.PathSeparator)) {
			u, ok := cfg.Users[r.User]
			return r, u, ok, nil
		}
	}
	return PathRule{}, User{}, false, nil
}

func SortedUsers(cfg Config) []string {
	keys := make([]string, 0, len(cfg.Users))
	for k := range cfg.Users {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func IsEmptyConfig(cfg Config) bool {
	return len(cfg.Users) == 0 && len(cfg.Paths) == 0
}

func sortPathRules(paths []PathRule) {
	sort.Slice(paths, func(i, j int) bool {
		// Most specific paths first.
		if len(paths[i].Path) != len(paths[j].Path) {
			return len(paths[i].Path) > len(paths[j].Path)
		}
		return paths[i].Path < paths[j].Path
	})
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func expandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

func ShellPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if rel, relErr := filepath.Rel(home, p); relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}
