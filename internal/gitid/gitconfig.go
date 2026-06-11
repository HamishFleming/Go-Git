package gitid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func GenerateIncludes(cfg Config) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	includesDir := filepath.Join(dir, "includes")
	if err := os.MkdirAll(includesDir, 0o700); err != nil {
		return "", err
	}

	for alias, u := range cfg.Users {
		file := filepath.Join(includesDir, alias+".gitconfig")
		body := fmt.Sprintf("[user]\n\tname = %s\n\temail = %s\n[core]\n\tsshCommand = ssh -i %s -o IdentitiesOnly=yes\n", escapeGitValue(u.GitName), escapeGitValue(u.GitEmail), escapeSSHCommandPath(u.SSHKey))
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			return "", err
		}
	}

	var b strings.Builder
	paths := append([]PathRule(nil), cfg.Paths...)
	sort.Slice(paths, func(i, j int) bool { return paths[i].Path < paths[j].Path })
	for _, r := range paths {
		if _, ok := cfg.Users[r.User]; !ok {
			continue
		}
		gitdir := ShellPath(filepath.Clean(r.Path))
		if !strings.HasSuffix(gitdir, "/") {
			gitdir += "/"
		}
		includePath := ShellPath(filepath.Join(includesDir, r.User+".gitconfig"))
		fmt.Fprintf(&b, "[includeIf \"gitdir:%s**\"]\n\tpath = %s\n\n", gitdir, includePath)
	}
	return b.String(), nil
}

func ApplyToRepo(repoPath string, u User) error {
	if repoPath == "" {
		repoPath = "."
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		if _, rootErr := gitRoot(abs); rootErr == nil {
			abs, _ = gitRoot(abs)
		} else {
			return fmt.Errorf("%s does not look like a git repository", repoPath)
		}
	}
	commands := [][]string{
		{"git", "-C", abs, "config", "user.name", u.GitName},
		{"git", "-C", abs, "config", "user.email", u.GitEmail},
		{"git", "-C", abs, "config", "core.sshCommand", fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes", u.SSHKey)},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func gitRoot(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func escapeGitValue(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

func escapeSSHCommandPath(p string) string {
	// Git stores this as a single string command. Quote only if needed.
	if strings.ContainsAny(p, " \t\n\"") {
		return "\"" + strings.ReplaceAll(p, "\"", "\\\"") + "\""
	}
	return p
}
