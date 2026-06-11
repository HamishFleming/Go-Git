package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"go-git/internal/gitid"
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
		return initCmd(args[1:])
	case "user":
		return userCmd(args[1:])
	case "path":
		return pathCmd(args[1:])
	case "settings":
		return settingsCmd(args[1:])
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
	case "onboard":
		return onboardCmd()
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
		fs := flag.NewFlagSet("go-git user add", flag.ContinueOnError)
		gitName := fs.String("git-name", "", "Git user.name")
		gitEmail := fs.String("git-email", "", "Git user.email")
		sshKey := fs.String("ssh-key", "", "SSH private key path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: go-git user add <alias> --git-name <name> --git-email <email> --ssh-key <path>")
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
			return fmt.Errorf("usage: go-git user rm <alias>")
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

func initCmd(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: go-git init [repo-path]")
	}
	cfg := gitid.DefaultConfig()
	if existing, err := gitid.Load(); err == nil {
		cfg = existing
	}
	if err := gitid.Save(cfg); err != nil {
		return err
	}
	path, _ := gitid.ConfigPath()
	fmt.Println("initialized", path)

	if len(args) == 1 {
		result, err := gitid.RunProjectInit(&cfg, args[0], os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		if result.UserAlias == "" {
			fmt.Println("project setup cancelled")
			return nil
		}
		return gitid.Save(cfg)
	}

	if gitid.IsEmptyConfig(cfg) && cfg.Settings.AutoOnboardOnInit {
		alias, changed, err := gitid.RunOnboarding(&cfg, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		if changed {
			if err := gitid.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("onboarded %s\n", alias)
		}
	}
	return nil
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
			return fmt.Errorf("usage: go-git path add <path> <user-alias>")
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
			return fmt.Errorf("usage: go-git path rm <path>")
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

func settingsCmd(args []string) error {
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		return settingsListCmd(cfg)
	}
	if args[0] != "set" || len(args) != 3 {
		return fmt.Errorf("usage: go-git settings [list] | go-git settings set <key> <true|false>")
	}
	value, err := strconv.ParseBool(args[2])
	if err != nil {
		return fmt.Errorf("invalid boolean %q", args[2])
	}
	switch args[1] {
	case "auto_onboard_on_init":
		cfg.Settings.AutoOnboardOnInit = value
	case "apply_user_on_project_init":
		cfg.Settings.ApplyUserOnProject = value
	case "map_path_on_project_init":
		cfg.Settings.MapPathOnProject = value
	default:
		return fmt.Errorf("unknown setting %q", args[1])
	}
	if err := gitid.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("set %s=%t\n", args[1], value)
	return nil
}

func settingsListCmd(cfg gitid.Config) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE")
	fmt.Fprintf(w, "auto_onboard_on_init\t%t\n", cfg.Settings.AutoOnboardOnInit)
	fmt.Fprintf(w, "apply_user_on_project_init\t%t\n", cfg.Settings.ApplyUserOnProject)
	fmt.Fprintf(w, "map_path_on_project_init\t%t\n", cfg.Settings.MapPathOnProject)
	return w.Flush()
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
		return fmt.Errorf("usage: go-git use <user-alias> [repo-path]")
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

func onboardCmd() error {
	cfg, err := gitid.Load()
	if err != nil {
		return err
	}
	_, changed, err := gitid.RunOnboarding(&cfg, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("onboarding cancelled")
		return nil
	}
	return gitid.Save(cfg)
}

func usage() {
	fmt.Print(`go git manages Git identities and SSH keys per repo/path.

Commands:
  go-git init [repo-path]
  go-git onboard
  go-git settings [list]
  go-git settings set <key> <true|false>
  go-git user add <alias> --git-name <name> --git-email <email> --ssh-key <path>
  go-git user rm <alias>
  go-git path add <path> <user-alias>
  go-git path rm <path>
  go-git list
  go-git resolve [path]
  go-git apply
  go-git use <user-alias> [repo-path]

Environment:
  GIT_ID_CONFIG_DIR  Override config directory, default ~/.config/go-git
`)
}
