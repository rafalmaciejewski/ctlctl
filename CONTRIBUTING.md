# Contributing to ctlctl

## Development

You need Go 1.27 or later.

```sh
go run .                 # start ctlctl
go run . kubectl         # open a tool right away
go test ./...            # run the tests
go vet ./...
gofmt -l .               # list files that need formatting
```

The code is in `internal/`:

| Package    | What it does                                                           |
| ---------- | ---------------------------------------------------------------------- |
| `discover` | Finds the `*ctl` executables on `PATH`                                 |
| `loader`   | Runs a tool to get its help, isolated from the terminal, with a timeout |
| `helpdoc`  | Parses help text into commands, flags and highlighted, clickable lines |
| `shell`    | Splits and highlights command lines                                    |
| `tui`      | The Bubble Tea interface                                               |
| `history`  | The history of commands run                                            |
| `runner`   | Runs the final command in your shell                                   |
| `config`   | The optional TOML config file                                          |

### Help formats

`internal/helpdoc/testdata` holds real and representative help output. When
ctlctl misreads a tool, save its output there:

```sh
NO_COLOR=1 sometool --help > internal/helpdoc/testdata/sometool.txt 2>&1
```

Then add a test to `internal/helpdoc/parse_test.go` that checks the commands
and flags you expect. `TestStylesAreSortedAndDisjoint` checks every fixture
automatically.

### Interface tests

`internal/tui/model_test.go` drives the model with key presses and mouse
clicks, serves help from the fixtures instead of running tools, and checks
the rendered screen.

## Releases

GitHub Actions runs the tests on Linux, macOS and Windows, and builds every
platform with GoReleaser, on pull requests to `main` and on pushes to `main`.

After a push to `main` passes, the release job runs
[svu](https://github.com/caarlos0/svu) to work out the next version from the
commits since the last `vX.Y.Z` tag. If there is one, the job pushes the tag,
and [GoReleaser](https://goreleaser.com) builds the binaries and publishes a
GitHub Release with the archives, `checksums.txt` and generated notes.
`install.sh` and `install.ps1` download from the latest release.

Use a Conventional Commit message for the commit that lands on `main`. With
squash merges, use the PR title as the commit subject.

| Commit example                                                   | Release    |
| ---------------------------------------------------------------- | ---------- |
| `fix: read flags from clap 4 help`                               | Patch      |
| `feat: filter the help pane`                                     | Minor      |
| `feat!: drop the --list flag`, or a `BREAKING CHANGE:` footer    | Major      |
| Only `docs:`, `test:`, `ci:`, or `chore:` commits                | No release |

The first release is `v0.1.0`. To preview the next version without
releasing, run the workflow from the Actions tab with "Only print the next
version" checked.

Dependabot opens `fix(deps)` pull requests for Go modules, which publish a
patch because they are compiled into the binary, and `ci(deps)` pull
requests for workflow actions, which publish nothing.

To try the release build locally:

```sh
goreleaser release --snapshot --clean
ls dist/
```
