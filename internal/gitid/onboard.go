package gitid

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type DetectedIdentity struct {
	GitName  string
	GitEmail string
	SSHKey   string
}

func DetectGlobalIdentity() (DetectedIdentity, error) {
	return DetectedIdentity{
		GitName:  gitConfigGet("user.name"),
		GitEmail: gitConfigGet("user.email"),
		SSHKey:   detectSSHKeyFromGit(),
	}, nil
}

type ProjectSetupResult struct {
	ProjectPath string
	UserAlias   string
	UserCreated bool
	Applied     bool
	PathMapped  bool
}

func CandidateSSHKeys() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var keys []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".pub") {
			continue
		}
		switch name {
		case "config", "known_hosts", "authorized_keys":
			continue
		}
		if !strings.HasPrefix(name, "id_") {
			continue
		}
		keys = append(keys, filepath.Join(sshDir, name))
	}
	return keys, nil
}

func RunOnboarding(cfg *Config, in io.Reader, out io.Writer) (string, bool, error) {
	identity, err := DetectGlobalIdentity()
	if err != nil {
		return "", false, err
	}
	candidates, err := CandidateSSHKeys()
	if err != nil {
		return "", false, err
	}

	fmt.Fprintln(out, "Onboarding from your existing Git setup")
	if identity.GitName != "" {
		fmt.Fprintf(out, "  git user.name:  %s\n", identity.GitName)
	}
	if identity.GitEmail != "" {
		fmt.Fprintf(out, "  git user.email: %s\n", identity.GitEmail)
	}
	if identity.SSHKey != "" {
		fmt.Fprintf(out, "  ssh key:        %s\n", ShellPath(identity.SSHKey))
	}
	if identity.GitName == "" && identity.GitEmail == "" && identity.SSHKey == "" {
		fmt.Fprintln(out, "  no global Git identity was detected")
	}

	reader := bufio.NewReader(in)
	create, err := promptYesNo(reader, out, "Create a go-git user from this information?", true)
	if err != nil || !create {
		return "", false, err
	}

	alias, err := promptCreateUser(cfg, reader, out, identity, candidates)
	if err != nil {
		return "", false, err
	}
	fmt.Fprintf(out, "saved user %s\n", alias)
	return alias, true, nil
}

func RunProjectInit(cfg *Config, projectPath string, in io.Reader, out io.Writer) (ProjectSetupResult, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return ProjectSetupResult{}, err
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		if root, rootErr := gitRoot(abs); rootErr == nil {
			abs = root
		} else {
			return ProjectSetupResult{}, fmt.Errorf("%s does not look like a git repository", projectPath)
		}
	}

	reader := bufio.NewReader(in)
	result := ProjectSetupResult{ProjectPath: abs}
	fmt.Fprintf(out, "Project setup for %s\n", ShellPath(abs))

	alias, created, err := promptSelectOrCreateUser(cfg, reader, out)
	if err != nil {
		return ProjectSetupResult{}, err
	}
	if alias == "" {
		return ProjectSetupResult{}, nil
	}
	result.UserAlias = alias
	result.UserCreated = created

	if apply, err := promptYesNo(reader, out, "Apply this user to the repository now?", cfg.Settings.ApplyUserOnProject); err != nil {
		return ProjectSetupResult{}, err
	} else if apply {
		if err := ApplyToRepo(abs, cfg.Users[alias]); err != nil {
			return ProjectSetupResult{}, err
		}
		result.Applied = true
		fmt.Fprintf(out, "applied %s to %s\n", alias, ShellPath(abs))
	}

	if mapPath, err := promptYesNo(reader, out, "Add a path rule for this project?", cfg.Settings.MapPathOnProject); err != nil {
		return ProjectSetupResult{}, err
	} else if mapPath {
		if err := AddPath(cfg, abs, alias); err != nil {
			return ProjectSetupResult{}, err
		}
		result.PathMapped = true
		fmt.Fprintf(out, "mapped %s -> %s\n", ShellPath(abs), alias)
	}

	return result, nil
}

func gitConfigGet(key string) string {
	cmd := exec.Command("git", "config", "--global", "--get", key)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectSSHKeyFromGit() string {
	sshCommand := gitConfigGet("core.sshCommand")
	if sshCommand == "" {
		return ""
	}
	fields := strings.Fields(sshCommand)
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] != "-i" {
			continue
		}
		key, err := expandPath(strings.Trim(fields[i+1], `"'`))
		if err != nil {
			return strings.Trim(fields[i+1], `"'`)
		}
		return key
	}
	return ""
}

func suggestAlias(email string) string {
	if email == "" {
		return "default"
	}
	local := strings.SplitN(email, "@", 2)[0]
	local = strings.TrimSpace(local)
	if local == "" {
		return "default"
	}
	local = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, local)
	local = strings.Trim(local, "-_")
	if local == "" {
		return "default"
	}
	return local
}

func promptSelectOrCreateUser(cfg *Config, reader *bufio.Reader, out io.Writer) (string, bool, error) {
	aliases := SortedUsers(*cfg)
	if len(aliases) > 0 {
		fmt.Fprintln(out, "Available users:")
		for i, alias := range aliases {
			u := cfg.Users[alias]
			fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, alias, u.GitEmail)
		}
		fmt.Fprintln(out, "  0. create a new user")
		choice, err := promptValue(reader, out, "Select a user", "1")
		if err != nil {
			return "", false, err
		}
		if n, convErr := strconv.Atoi(choice); convErr == nil {
			if n >= 1 && n <= len(aliases) {
				return aliases[n-1], false, nil
			}
			if n == 0 {
				alias, err := runNestedUserOnboarding(cfg, reader, out)
				return alias, alias != "", err
			}
		}
		if _, ok := cfg.Users[choice]; ok {
			return choice, false, nil
		}
		return "", false, fmt.Errorf("unknown user selection %q", choice)
	}

	fmt.Fprintln(out, "No go-git users exist yet.")
	alias, err := runNestedUserOnboarding(cfg, reader, out)
	return alias, alias != "", err
}

func runNestedUserOnboarding(cfg *Config, reader *bufio.Reader, out io.Writer) (string, error) {
	identity, err := DetectGlobalIdentity()
	if err != nil {
		return "", err
	}
	candidates, err := CandidateSSHKeys()
	if err != nil {
		return "", err
	}
	fmt.Fprintln(out, "Creating a new user from your existing Git setup")
	return promptCreateUser(cfg, reader, out, identity, candidates)
}

func promptCreateUser(cfg *Config, reader *bufio.Reader, out io.Writer, identity DetectedIdentity, candidates []string) (string, error) {
	suggestedAlias := suggestAlias(identity.GitEmail)
	alias, err := promptValue(reader, out, "Alias", suggestedAlias)
	if err != nil {
		return "", err
	}
	if alias == "" {
		return "", fmt.Errorf("alias is required")
	}
	if _, exists := cfg.Users[alias]; exists {
		overwrite, err := promptYesNo(reader, out, fmt.Sprintf("User %q already exists. Overwrite it?", alias), false)
		if err != nil || !overwrite {
			return "", err
		}
	}

	gitName, err := promptValue(reader, out, "Git name", identity.GitName)
	if err != nil {
		return "", err
	}
	gitEmail, err := promptValue(reader, out, "Git email", identity.GitEmail)
	if err != nil {
		return "", err
	}
	sshKey, err := promptSSHKey(reader, out, identity.SSHKey, candidates)
	if err != nil {
		return "", err
	}

	if err := AddUser(cfg, alias, User{
		GitName:  gitName,
		GitEmail: gitEmail,
		SSHKey:   sshKey,
	}); err != nil {
		return "", err
	}
	return alias, nil
}

func promptValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	text, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return defaultValue, nil
	}
	return text, nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, question string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(out, "%s %s: ", question, suffix)
	text, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return defaultYes, nil
	}
	return text == "y" || text == "yes", nil
}

func promptSSHKey(reader *bufio.Reader, out io.Writer, detected string, candidates []string) (string, error) {
	if detected == "" && len(candidates) > 0 {
		fmt.Fprintln(out, "Detected SSH keys:")
		for i, candidate := range candidates {
			fmt.Fprintf(out, "  %d. %s\n", i+1, ShellPath(candidate))
		}
	}

	prompt := "SSH key path"
	if detected == "" && len(candidates) > 0 {
		prompt += " (enter a number or path)"
	}

	value, err := promptValue(reader, out, prompt, detected)
	if err != nil {
		return "", err
	}
	if value == "" && len(candidates) > 0 {
		return "", fmt.Errorf("ssh key path is required")
	}
	if n, convErr := strconv.Atoi(value); convErr == nil && n >= 1 && n <= len(candidates) {
		return candidates[n-1], nil
	}
	return value, nil
}
