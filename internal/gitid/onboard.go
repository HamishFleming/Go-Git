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

func RunOnboarding(cfg *Config, in io.Reader, out io.Writer) (bool, error) {
	identity, err := DetectGlobalIdentity()
	if err != nil {
		return false, err
	}
	candidates, err := CandidateSSHKeys()
	if err != nil {
		return false, err
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
		return false, err
	}

	suggestedAlias := suggestAlias(identity.GitEmail)
	alias, err := promptValue(reader, out, "Alias", suggestedAlias)
	if err != nil {
		return false, err
	}
	if alias == "" {
		return false, fmt.Errorf("alias is required")
	}
	if _, exists := cfg.Users[alias]; exists {
		overwrite, err := promptYesNo(reader, out, fmt.Sprintf("User %q already exists. Overwrite it?", alias), false)
		if err != nil || !overwrite {
			return false, err
		}
	}

	gitName, err := promptValue(reader, out, "Git name", identity.GitName)
	if err != nil {
		return false, err
	}
	gitEmail, err := promptValue(reader, out, "Git email", identity.GitEmail)
	if err != nil {
		return false, err
	}
	sshKey, err := promptSSHKey(reader, out, identity.SSHKey, candidates)
	if err != nil {
		return false, err
	}

	if err := AddUser(cfg, alias, User{
		GitName:  gitName,
		GitEmail: gitEmail,
		SSHKey:   sshKey,
	}); err != nil {
		return false, err
	}

	fmt.Fprintf(out, "saved user %s\n", alias)
	return true, nil
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
