# Usage

## Starting ctlctl

```sh
ctlctl                    # every *ctl tool you can run
ctlctl kubectl            # open kubectl right away
ctlctl kubectl config     # ...and go straight to a subcommand
ctlctl docker             # any tool works when you name it
```

| Flag            | Meaning                                                         |
| --------------- | --------------------------------------------------------------- |
| `-p`, `--print` | Print the command instead of running it (see below)             |
| `-l`, `--list`  | List the tools ctlctl finds, with their paths, and exit         |
| `-v`, `--version` | Print the version and exit                                    |
| `-h`, `--help`  | Show help and exit                                              |

## The screen

- **Tools** (left): the tools, and their subcommands once you open them. `▸`
  marks something you can open, and `·` a command without subcommands.
- **Help** (right): the help of the selected command, highlighted. When a
  subcommand has no help of its own, this shows the tool's help with the
  subcommand's line highlighted. While a command's help loads, the pane keeps
  the previous help, and a spinner shows after the command's name in the
  tree and in the pane's title.
- **Command** (bottom): the command that will run. It follows the selection
  in the tree, so the tool and subcommands are already filled in. What you
  type is kept per subcommand while you move around the tree.

## Mouse

- Click a tool or subcommand to select it, and double-click it or click its
  `▸` to open it.
- Click a **flag** in the help to add it to the command. If the flag takes a
  value, the cursor waits right after it.
- Click a **subcommand** in the help to open it in the tree.
- Click an **example** in the help to copy it to the command line.
- Scroll any pane with the wheel, and click in the command line to move the
  cursor.

## Keys

| Where      | Key                       | Action                                                   |
| ---------- | ------------------------- | -------------------------------------------------------- |
| Everywhere | `tab` / `shift+tab`       | Move focus between panes                                 |
|            | `?`                       | Show all keys                                            |
|            | `ctrl+c`                  | Quit without running anything                            |
| Tools      | `↑` `↓` / `k` `j`         | Move                                                     |
|            | `enter`                   | Open the subcommands, or edit the command if it has none |
|            | `→` / `←`                 | Expand / collapse, or go to the parent                   |
|            | `space`                   | Expand or collapse                                       |
|            | `e`                       | Edit the command for the selected node                   |
|            | `/`                       | Filter by name                                           |
|            | `q`                       | Quit                                                     |
| Help       | `↑` `↓` `pgup` `pgdn`     | Scroll                                                   |
|            | `n` / `N`                 | Select the next / previous flag, subcommand or example   |
|            | `enter`                   | Use the selected item                                    |
| Command    | `enter`                   | Run the command and exit                                 |
|            | `alt+enter` or `ctrl+o`   | Run the command and come back to ctlctl                  |
|            | `tab` or `→`              | Accept the suggested flag or subcommand                  |
|            | `↑` `↓`                   | Commands you ran before for this subcommand              |
|            | `ctrl+a` / `ctrl+e`       | Start / end of the line                                  |
|            | `ctrl+w` / `ctrl+u` / `ctrl+k` | Delete a word / to the start / to the end           |
|            | `esc`                     | Back to the tools                                        |

## Running commands

The command line understands shell syntax. Pipes, redirections, quotes and
variables are highlighted and work as they would in your shell, because
ctlctl runs the command with `$SHELL -c` (`cmd /C` on Windows). You can set
another shell in the [config file](configuration.md).

Enter replaces ctlctl with the command, so it gets your terminal and its exit
status becomes ctlctl's. Alt+Enter runs the command, waits for you to press
Enter, and then brings ctlctl back.

Commands you run are saved in `~/.local/state/ctlctl/history`. Press `↑` in
the command line to go back through the ones for the selected subcommand.

## Shell integration

`ctlctl --print` prints the command instead of running it, and exits with
status 130 if you quit without choosing one. Bind it to a key, and the command
lands on your shell's command line, so it ends up in your shell history too.

zsh (`~/.zshrc`):

```zsh
ctlctl-widget() {
  local cmd
  cmd="$(ctlctl --print)" && BUFFER=$cmd && CURSOR=${#BUFFER}
  zle reset-prompt
}
zle -N ctlctl-widget
bindkey '^G' ctlctl-widget
```

bash (`~/.bashrc`):

```bash
ctlctl_widget() {
  local cmd
  cmd="$(ctlctl --print)" && READLINE_LINE=$cmd && READLINE_POINT=${#cmd}
}
bind -x '"\C-g": ctlctl_widget'
```

fish (`~/.config/fish/config.fish`):

```fish
function ctlctl_widget
  set -l cmd (ctlctl --print); and commandline -r -- $cmd
  commandline -f repaint
end
bind \cg ctlctl_widget
```
