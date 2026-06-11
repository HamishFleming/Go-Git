package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"git-id/internal/gitid"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return nil
	}

	switch args[0] {
	case "init":
		cfg := gitid.DefaultConfig()
		if existing, err := gitid.Load(); err == nil && (len(existing.Users) > 0 || len(existing.Paths) > 0) {
			cfg = existing
		}
		if err := gitid.Save(cfg); err != nil {
			return err
		}
		path, _ := gitid.ConfigPath()
		fmt.Println("created", path)
		return nil
	case "user":
		return userCmd(args[1:])
	case "path":
		return pathCmd(args[1:])
	case "list":
		return listCmd()
	case "resolve":
		path := "."
		if len(args) > 1 {
			path = args[1]
		}
		return resolveCmd(path)
	case "apply":
		return applyCmd()
	case "use":
		return useCmd(args[1:])
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func userCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing user subcommand")
	}
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("git-id user add", flag.ContinueOnError)
		gitName := fs.String("git-name", "", "Git user.name")
		gitEmail := fs.String("git-email", "", "Git user.email")
		sshKey := fs.String("ssh-key", "", "SSH private key path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: git-id user add <alias> --git-name <name> --git-email <email> --ssh-key <path>")
		}
		if err := gitid.AddUser(&cfg, fs.Arg(0), gitid.User{GitName: *gitName, GitEmail: *gitEmail, SSHKey: *sshKey}); err != nil {
			return err
		}
		if err := gitid.Save(cfg); err != nil {
			return err
		}
		fmt.Println("saved user", fs.Arg(0))
		return nil
	case "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: git-id user rm <alias>")
		}
		if err := gitid.RemoveUser(&cfg, args[1]); err != nil {
			return err
		}
		if err := gitid.Save(cfg); err != nil {
			return err
		}
		fmt.Println("removed user", args[1])
		return nil
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func pathCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing path subcommand")
	}
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		if len(args) != 3 {
			return fmt.Errorf("usage: git-id path add <path> <user-alias>")
		}
		if err := gitid.AddPath(&cfg, args[1], args[2]); err != nil {
			return err
		}
		if err := gitid.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("mapped %s -> %s\n", args[1], args[2])
		return nil
	case "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: git-id path rm <path>")
		}
		if err := gitid.RemovePath(&cfg, args[1]); err != nil {
			return err
		}
		if err := gitid.Save(cfg); err != nil {
			return err
		}
		fmt.Println("removed path", args[1])
		return nil
	default:
		return fmt.Errorf("unknown path subcommand %q", args[0])
	}
}

func listCmd() error {
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERS")
	fmt.Fprintln(w, "ALIAS\tNAME\tEMAIL\tSSH KEY")
	for _, alias := range gitid.SortedUsers(cfg) {
		u := cfg.Users[alias]
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", alias, u.GitName, u.GitEmail, gitid.ShellPath(u.SSHKey))
	}
	fmt.Fprintln(w, "\nPATH RULES")
	fmt.Fprintln(w, "PATH\tUSER")
	for _, r := range cfg.Paths {
		fmt.Fprintf(w, "%s\t%s\n", gitid.ShellPath(r.Path), r.User)
	}
	return w.Flush()
}

func resolveCmd(path string) error {
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	r, u, ok, err := gitid.Resolve(cfg, path)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no matching path rule for %s", path)
	}
	fmt.Printf("path: %s\nuser: %s\nname: %s\nemail: %s\nssh_key: %s\n", gitid.ShellPath(r.Path), r.User, u.GitName, u.GitEmail, gitid.ShellPath(u.SSHKey))
	return nil
}

func applyCmd() error {
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	block, err := gitid.GenerateIncludes(cfg)
	if err != nil {
		return err
	}
	fmt.Println("Generated include files. Add this block to ~/.gitconfig:")
	fmt.Println()
	fmt.Print(block)
	return nil
}

func useCmd(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: git-id use <user-alias> [repo-path]")
	}
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	u, ok := cfg.Users[args[0]]
	if !ok {
		return fmt.Errorf("unknown user %q", args[0])
	}
	repo := "."
	if len(args) == 2 {
		repo = args[1]
	}
	if err := gitid.ApplyToRepo(repo, u); err != nil {
		return err
	}
	fmt.Printf("applied %s to %s\n", args[0], repo)
	return nil
}

func usage() {
	fmt.Print(`git-id manages Git identities and SSH keys per repo/path.

Commands:
  git-id init
  git-id user add <alias> --git-name <name> --git-email <email> --ssh-key <path>
  git-id user rm <alias>
  git-id path add <path> <user-alias>
  git-id path rm <path>
  git-id list
  git-id resolve [path]
  git-id apply
  git-id use <user-alias> [repo-path]

Environment:
  GIT_ID_CONFIG_DIR  Override config directory, default ~/.config/git-id
`)
}
