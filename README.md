# devtree

Control which git worktree your dev servers run in.

If you juggle several worktrees of the same repo — a feature branch, a review checkout,
`main` — you end up with the same problem every time: your dev server is running in the
*wrong one*. `devtree` is a small terminal UI that keeps each service in a tmux window
and lets you point it at any worktree of its repo without hunting for the right pane.

```
  devtree

  services                          │ web  web-checkout
  ────────────────────────────────  │ ────────────────────────────────
  backend                        ●  │  VITE v5.4.2  ready in 412 ms
    main                            │
▸ web                            ●  │  ➜  Local:   http://localhost:5173/
    feature/checkout                │  ➜  Network: use --host to expose
  storybook                      ◌  │
    —                               │

  ↑/k  ↓/j  navigate    ↵/w  switch worktree    s  start    x  stop    r  restart    a  attach    q  quit
```

## Requirements

- **tmux** — devtree drives it; it is not optional
- **git** — for listing worktrees
- **Go 1.26+** to build from source

## Install

```sh
go install github.com/matthewtole/devtree/cmd/devtree@latest
```

Or from a clone:

```sh
go build -o devtree ./cmd/devtree
```

## Configure

devtree reads `~/.config/devtree/config.toml` (or `$XDG_CONFIG_HOME/devtree/config.toml`).
Each service is one `[[service]]` block:

```toml
[[service]]
name    = "web"
repo    = "/Users/you/code/myapp"
command = "npm run dev"

[[service]]
name    = "backend"
repo    = "/Users/you/code/myapp-api"
command = "cargo watch -x run"
```

- `name` — unique; also the tmux window name
- `repo` — absolute path to the repo (any worktree of it will do)
- `command` — what to run, executed in the selected worktree

Add one interactively with `devtree service add`, or check what you've written with
`devtree config check`.

## Use

Run `devtree` with no arguments to open the TUI.

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | move between services |
| `↵` / `w` | switch the selected service to another worktree |
| `s` | start |
| `x` | stop |
| `r` | restart in the same worktree |
| `a` | attach to the service's tmux window |
| `pgup` / `pgdn` | scroll the log panel |
| `q` | quit |

The right panel tails the selected service's output live and follows the bottom until
you scroll up.

Status is shown next to each service name:

| | |
| --- | --- |
| ● | running |
| ○ | idle — the window exists, sitting at a shell prompt |
| ◌ | not started — no tmux window yet |

Switching worktrees on a running service kills the process, `cd`s to the new worktree,
and restarts the command. On a service that hasn't started yet, it just records your
choice for the next `s`.

### Other commands

```
devtree              launch the TUI
devtree service add  add a new service (interactive)
devtree config check validate the config file and print parsed services
devtree help         show usage
```

## How it works

devtree keeps no state of its own — tmux is the source of truth.

Every service gets a window named after it inside a single tmux session called
`devtree`. The worktree a service is currently running in is stored on that window as
the tmux user option `@worktree`, so your setup survives quitting devtree, and running
`devtree` again just reads back whatever is already there.

Status is inferred rather than tracked: if the window's foreground process is a shell,
the service is idle; if it's anything else, it's running. Log output comes from
`capture-pane` on a twice-a-second poll.

Because it's all just tmux, you can `tmux attach -t devtree` at any point and work with
the windows directly — devtree will pick up whatever it finds on the next poll.

## Development

```sh
go test ./...        # tmux tests are skipped if tmux isn't installed
go vet ./...
go build ./...
```

The tmux tests spin up their own server on a per-test socket, so they never touch your
real session.

## License

MIT — see [LICENSE](LICENSE).
