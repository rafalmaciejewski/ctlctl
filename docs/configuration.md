# Configuration

ctlctl works without a config file. To change which tools it lists, create
`~/.config/ctlctl/config.toml`:

```toml
# Executables on PATH whose names end with one of these are listed.
suffixes = ["ctl"]

# Extra tools to list, by name or path.
include = ["docker", "helm", "gh"]

# Tools to hide.
exclude = ["sysctl"]

# How long a tool may take to print its help.
help_timeout = "5s"

# The shell that runs the final command. Defaults to $SHELL.
# shell = "/bin/zsh"
```

| Setting        | Default       | Meaning                                                        |
| -------------- | ------------- | -------------------------------------------------------------- |
| `suffixes`     | `["ctl"]`     | List the executables whose names end with one of these         |
| `include`      | `[]`          | Also list these tools, by name or path                         |
| `exclude`      | `[]`          | Never list these tools                                         |
| `help_timeout` | `"5s"`        | Give up on a help command after this long                      |
| `shell`        | `$SHELL`      | Run commands with this shell (`cmd.exe` on Windows by default) |

Run `ctlctl --list` to check which tools ctlctl finds.

## File locations

| File    | Location                                                                   |
| ------- | -------------------------------------------------------------------------- |
| Config  | `$CTLCTL_CONFIG`, else `$XDG_CONFIG_HOME/ctlctl/config.toml`, else `~/.config/ctlctl/config.toml` (Windows: `%APPDATA%\ctlctl\config.toml`) |
| History | `$XDG_STATE_HOME/ctlctl/history`, else `~/.local/state/ctlctl/history` (Windows: `%LOCALAPPDATA%\ctlctl\history`) |
