# go git

`go git` is a small Go CLI for people who work across multiple Git identities.

It helps you keep personal, client, and employer repositories separate by mapping directory paths to the correct:

- `user.name`
- `user.email`
- SSH private key via `core.sshCommand`

If you have ever committed to the right repo with the wrong email address, or pushed with the wrong SSH key loaded, this tool exists to remove that friction.

## Why This Exists

Most developers who juggle more than one Git identity end up with some mix of:

- shell aliases
- manual `git config` edits
- SSH agent juggling
- half-remembered per-repo setup steps

That works until it does not.

`go git` gives you a simple, explicit model:

1. Define named identities.
2. Map repo roots or directory trees to those identities.
3. Generate Git `includeIf` config once.
4. Let Git automatically pick the right identity for repos under those paths.

It also supports a one-off mode for directly applying an identity to an existing repository.

## Features

- Simple JSON-backed configuration
- Path-based identity resolution
- Git `includeIf` config generation
- Per-identity SSH key selection
- Direct application to a single repository
- Small codebase with no external dependencies

## Installation

### Build from source

```bash
go build -o go-git ./cmd/go-git
```

### Run without installing

```bash
go run ./cmd/go-git --help
```

## Quick Start

Initialize config:

```bash
go-git init
```

Or start with guided onboarding from your existing global Git config:

```bash
go-git onboard
```

Add identities:

```bash
go-git user add personal \
  --git-name "Hamish Fleming" \
  --git-email "hamish@example.com" \
  --ssh-key ~/.ssh/id_ed25519_personal

go-git user add work \
  --git-name "Hamish Fleming" \
  --git-email "hamish@company.com" \
  --ssh-key ~/.ssh/id_ed25519_work
```

Map directories to those identities:

```bash
go-git path add ~/code/personal personal
go-git path add ~/work work
```

Generate include files and print the `~/.gitconfig` block you should add:

```bash
go-git apply
```

Example output:

```gitconfig
[includeIf "gitdir:~/code/personal/**"]
    path = ~/.config/go-git/includes/personal.gitconfig

[includeIf "gitdir:~/work/**"]
    path = ~/.config/go-git/includes/work.gitconfig
```

Once that block is added to `~/.gitconfig`, any repository under those paths will automatically use the matching identity and SSH key.

## One-Off Repo Setup

If you want to apply an identity directly to the current repository instead of using path rules:

```bash
go-git use work
```

Or target a specific repository path:

```bash
go-git use personal ~/code/personal/my-project
```

This writes the selected identity into that repository's local `.git/config`.

## How It Works

Configuration is stored in:

```text
~/.config/go-git/config.json
```

Generated include files are written to:

```text
~/.config/go-git/includes/
```

Each identity gets its own generated `.gitconfig` file. Path rules are matched by directory prefix, with more specific paths taking priority.

## CLI Reference

```text
go-git init
go-git onboard
go-git user add <alias> --git-name <name> --git-email <email> --ssh-key <path>
go-git user rm <alias>
go-git path add <path> <user-alias>
go-git path rm <path>
go-git list
go-git resolve [path]
go-git apply
go-git use <user-alias> [repo-path]
```

## Example Workflow

```bash
go-git init

go-git user add oss \
  --git-name "Jane Developer" \
  --git-email "jane@users.noreply.github.com" \
  --ssh-key ~/.ssh/id_ed25519_oss

go-git user add client \
  --git-name "Jane Developer" \
  --git-email "jane@client.com" \
  --ssh-key ~/.ssh/id_ed25519_client

go-git path add ~/src/open-source oss
go-git path add ~/src/client-work client

go-git apply
go-git resolve ~/src/open-source/go-git
```

## Environment

- `GIT_ID_CONFIG_DIR`: override the default config directory

## Contributing

Contributions are welcome, especially around:

- better onboarding and packaging
- tests for path matching and config generation
- cross-platform edge cases
- quality-of-life improvements for day-to-day Git workflows

If you want to contribute, start by opening an issue or sending a focused PR with a clear use case.

## Project Scope

`go git` is intentionally narrow. It is not trying to be a full Git wrapper, credential manager, or SSH agent replacement. The goal is to stay small, understandable, and reliable for one job: using the right Git identity in the right place.
