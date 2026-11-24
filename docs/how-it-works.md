# How it works

## Finding tools

There's no built-in list of tools. Every time it starts, ctlctl scans the
directories on your `PATH` for files whose names end with `ctl` and that you
can run. It asks the operating system, so the file's owner, group and ACLs
count: a root-only tool isn't listed for other users, and broken symlinks
are skipped. When a name appears in several directories, the first one wins,
as it would in your shell. Tools you name on the command line or include in
the [config file](configuration.md) are added to the list.

## Reading help

ctlctl reads a tool's help the first time you select it, by running
`<tool> --help`, or `<tool> -h` when `--help` gets an "illegal option" error.
It runs `<tool> help` only when the output offers it. For tools whose help is
only a short usage message, it adds the man page. For tools that list their
global flags separately, such as `kubectl options`, it shows those flags under
every subcommand.

Help formats differ between tools, so ctlctl reads them by their layout:
section headers end with a colon or are written in capitals, entries are
indented, and names are separated from descriptions by two or more spaces.
That covers Cobra (kubectl, doctl), urfave/cli (crictl), clap, argparse,
Swift ArgumentParser (devicectl), systemd tools, Go's `flag` package and man
pages.

Tools without a commands section get their subcommands from:

- **Usage lines** like `cpuctl { offline | online } <cpu>`, `[ list ]` or
  `<status|enable|disable>`. Placeholders such as `command` or `[options]`
  are skipped, and so is an optional choice after the flags, which picks an
  argument rather than a subcommand.
- **Man page lists**, when the synopsis says the tool takes a command
  (`apachectl command`). These are terms like `start` or `stop, graceful-stop`
  with their description indented below or beside them.

## Subcommands are never run by accident

Some tools ignore `--help` after a subcommand and run the subcommand:
`cpuctl list --help` lists CPUs. So ctlctl asks for a subcommand's help only
in a way the tool's own help documents:

- `<tool> help <subcommand>`, when the tool lists a `help` subcommand;
- `<tool> <subcommand> --help`, when it documents a `--help` flag or says
  `<command> --help` works;
- `<tool> <subcommand> -h`, when it documents `-h` as help.

When none of these is documented, or the subcommand was read from a usage
line or a man page, ctlctl runs nothing. It shows the tool's help instead,
with the subcommand's line highlighted. It does the same when a tool prints
its general help for a subcommand, as `systemctl start --help` does.

## Running help commands safely

Help commands run in their own session without a terminal, with a timeout
(5 seconds by default), and with colors and pagers turned off. A tool that
waits for input can't take over the screen. Each command's help is read once
per session and kept in memory.
