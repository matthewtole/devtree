# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`devtree` is a Go TUI that controls which git worktree your long-running dev servers
run in. Each configured service owns one tmux window; devtree starts/stops/restarts
the service command there and can re-point it at a different worktree of the same
repo. tmux holds the state — devtree is a stateless controller over it.

## Commands

```sh
go build ./...                                    # build everything
go build -o devtree ./cmd/devtree                 # build the binary (gitignored)
go build -ldflags "-X main.Version=x.y.z" ./cmd/devtree   # stamp the version
go test ./...                                     # all tests
go test ./internal/tmux -run TestHasSession -v    # single test
go vet ./...
```

Runtime deps are external binaries, not Go packages: `tmux` and `git` must be on PATH.

Manual runs need a real config at `$XDG_CONFIG_HOME/devtree/config.toml` (or
`~/.config/devtree/config.toml`). `devtree config check` validates it without
launching the TUI, which is the fastest way to sanity-check config changes.

## Architecture

Four internal packages, each with a single job:

- **`internal/config`** — TOML config (`[[service]]` blocks: `name`, `repo`, `command`).
  `Parse` rejects unknown keys via `meta.Undecoded()`, so adding a config field means
  adding it to the struct or parsing starts failing. `LoadServices` deliberately skips
  validation (used by `service add` to check for duplicate names against a possibly
  half-written file).
- **`internal/git`** — `ListWorktrees` shells out to `git worktree list --porcelain`
  and parses it. `Service` wraps that in a TTL cache keyed by repo path, caching errors
  too so a broken repo isn't retried every poll. All worktree lookups go through
  `Service`; nothing should call `ListWorktrees` directly.
- **`internal/tmux`** — thin wrapper over the `tmux` CLI. `Client.Socket` selects the
  server (`-L`); empty means the user's default server. Tests set a unique socket so
  they never touch the real one.
- **`internal/tui`** — the bubbletea app. `model.go` is the root model + poll logic,
  `commands.go` holds the `tea.Cmd` side effects, `picker.go` is the worktree overlay,
  `styles.go` the lipgloss palette.

### The tmux contract

This is the load-bearing design decision. devtree stores no state of its own:

- Session name is the constant `tui.SessionName` (`"devtree"`); window name == service name.
- The active worktree lives in the tmux **window user option** `@worktree`, read back
  each poll via `GetUserOption`. That's why a service's worktree survives a devtree
  restart, and why a worktree selection made before the window exists is held only in
  the model until `cmdStart` writes it.
- Status is inferred, not tracked: no window → `statusAbsent`; `pane_current_command`
  is a known shell name → `statusIdle`; anything else → `statusRunning`.
- Log output is `capture-pane -e` (ANSI preserved) on every poll.

### Poll loop

`cmdTick` fires a `tickMsg` every 500ms; `cmdPoll` runs the whole tmux/git inspection
off the UI goroutine over a *copy* of the rows and returns a `stateMsg` with fresh rows,
which replaces `m.rows` wholesale. Consequences to respect:

- `tickMsg` and `stateMsg` are handled **before** the picker branch in `Update` so
  polling continues while the overlay is open.
- Anything written into `m.rows` outside a poll (e.g. a pending worktree choice) must
  either be re-derived by `cmdPoll` or explicitly preserved there — otherwise the next
  poll overwrites it 500ms later.
- Poll interval and `gitCacheTTL` are independent knobs; git work is deduped by the
  cache, not the tick.

### Start/stop mechanics

`cmdStart`/`cmdRestart` stop the process with two spaced `C-c` sends, then
`cd '<worktree>' && clear && <command>`. `cmdSwitch` instead uses `respawn-pane -k`,
which kills the process outright rather than hoping it honours SIGINT — prefer that
when correctness matters more than a graceful shutdown. Worktree paths go through
`shellEscape` before being interpolated into the `cd`.

`cmdAttach` must use `tea.ExecProcess`: `switch-client`/`attach-session` inspect the
caller's controlling terminal, and the normal buffered `run()` path makes tmux report
"not a terminal".

### Rendering

The main view is a hand-built two-column layout — `buildLeftPanel` and
`buildRightPanel` each return exactly `contentH` strings that are zipped together with
`divider`. Both must return full-height slices or the zip misaligns. Because lines are
composed manually rather than by lipgloss containers:

- Measure and pad with `lipgloss.Width` / `padToWidth`, truncate with `ansi.Truncate` —
  raw `len()` on a styled string is wrong.
- Avoid nesting lipgloss renders inside a highlighted row; the inner `\x1b[0m` punches
  gaps in the background colour. The selected-row code sidesteps this by keeping the
  status dot outside the highlight.
- Captured log lines get a per-line `\x1b[0m` in `syncViewport` so colour can't bleed.
- The viewport's keymap is stripped down to pgup/pgdn (`viewportKeyMap`) so j/k stay
  with the service list. Tmux windows are resized to the panel width on
  `WindowSizeMsg` so `capture-pane` wraps at the width we actually render.

## Testing

`internal/tmux` tests drive a real tmux server on a per-test `-L` socket and skip if
tmux isn't installed; cleanup kills the server and removes the socket file. `internal/tui`
tests exercise the model directly — feed a `stateMsg` to fake a poll result (see
`withState`) rather than trying to reach tmux, and assert on `View()` with `ansi.Strip`.
