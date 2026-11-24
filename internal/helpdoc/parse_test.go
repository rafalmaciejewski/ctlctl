package helpdoc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func load(t *testing.T, name string, path ...string) *Doc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return Parse(string(b), path)
}

func commandNames(d *Doc) []string {
	var out []string
	for _, c := range d.Commands {
		out = append(out, c.Name)
	}
	return out
}

func wantCommands(t *testing.T, d *Doc, want ...string) {
	t.Helper()
	got := commandNames(d)
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("missing command %q in %v", w, got)
		}
	}
}

func wantFlag(t *testing.T, d *Doc, name, arg string) *Flag {
	t.Helper()
	f := d.FindFlag(name)
	if f == nil {
		var all [][]string
		for _, f := range d.Flags {
			all = append(all, f.Names)
		}
		t.Fatalf("missing flag %q in %v", name, all)
	}
	if f.Arg != arg {
		t.Errorf("flag %s: arg = %q, want %q", name, f.Arg, arg)
	}
	return f
}

func TestKubectlRoot(t *testing.T) {
	d := load(t, "kubectl", "kubectl")
	wantCommands(t, d, "create", "get", "describe", "config", "version", "api-resources", "port-forward")
	if n := len(d.Commands); n != 42 {
		t.Errorf("got %d commands, want 42: %v", n, commandNames(d))
	}
	c := d.FindCommand("get")
	if c.Desc != "Display one or many resources" {
		t.Errorf("get desc = %q", c.Desc)
	}
	if len(d.Flags) != 0 {
		t.Errorf("unexpected flags: %v", d.Flags)
	}
}

func TestKubectlGet(t *testing.T) {
	d := load(t, "kubectl_get", "kubectl", "get")
	if len(d.Commands) != 0 {
		t.Errorf("kubectl get has no subcommands, got %v", commandNames(d))
	}
	f := wantFlag(t, d, "--all-namespaces", "")
	if !slices.Equal(f.Names, []string{"-A", "--all-namespaces"}) || f.Default != "false" {
		t.Errorf("all-namespaces = %+v", f)
	}
	if !strings.HasPrefix(f.Desc, "If present, list the requested object(s)") {
		t.Errorf("desc = %q", f.Desc)
	}
	f = wantFlag(t, d, "-o", "value")
	if f.Preferred() != "--output" {
		t.Errorf("preferred = %q", f.Preferred())
	}
	wantFlag(t, d, "--chunk-size", "value")
	wantFlag(t, d, "-f", "value")
	if len(d.Usage) == 0 || !strings.HasPrefix(d.Usage[0], "kubectl get [(-o|--output=)") {
		t.Errorf("usage = %q", d.Usage)
	}

	// Examples are clickable and highlighted.
	var ex *Line
	for i := range d.Lines {
		if d.Lines[i].Text == "  kubectl get pods -o wide" {
			ex = &d.Lines[i]
		}
	}
	if ex == nil {
		t.Fatal("example line not found")
	}
	if len(ex.Targets) != 1 || ex.Targets[0].Target != (Target{Kind: TargetExample, Value: "kubectl get pods -o wide"}) {
		t.Errorf("example targets = %+v", ex.Targets)
	}
	wantStyle(t, ex, "kubectl", KindProgram)
	wantStyle(t, ex, "get", KindCommand)
	wantStyle(t, ex, "-o", KindFlag)
	wantStyle(t, ex, "wide", KindArg)
}

func wantStyle(t *testing.T, l *Line, word string, k Kind) {
	t.Helper()
	i := strings.Index(l.Text, word)
	if i < 0 {
		t.Fatalf("%q not in %q", word, l.Text)
	}
	start := runeOff(l.Text, i)
	for _, s := range l.Styles {
		if s.Start <= start && start < s.End {
			if s.Kind != k {
				t.Errorf("%q in %q: kind %d, want %d", word, l.Text, s.Kind, k)
			}
			return
		}
	}
	t.Errorf("%q in %q: no style, want %d", word, l.Text, k)
}

func TestKubectlConfig(t *testing.T) {
	d := load(t, "kubectl_config", "kubectl", "config")
	wantCommands(t, d, "current-context", "use-context", "view", "set-credentials")
	if len(d.Commands) != 15 {
		t.Errorf("got %v", commandNames(d))
	}
	// The --kubeconfig mention in prose is styled even though it is global.
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "If the --kubeconfig flag") {
			wantStyle(t, &l, "--kubeconfig", KindFlag)
		}
	}
}

func TestLaunchctl(t *testing.T) {
	d := load(t, "launchctl", "launchctl")
	wantCommands(t, d, "bootstrap", "bootout", "kickstart", "print-disabled", "help", "version")
	if len(d.Commands) != 45 {
		t.Errorf("got %d: %v", len(d.Commands), commandNames(d))
	}
	if c := d.FindCommand("load"); !strings.HasPrefix(c.Desc, "Recommended alternatives") {
		t.Errorf("load desc = %q", c.Desc)
	}
}

func TestDevicectl(t *testing.T) {
	d := load(t, "devicectl", "devicectl")
	if got := commandNames(d); !slices.Equal(got, []string{"device", "diagnose", "list", "manage"}) {
		t.Errorf("commands = %v", got)
	}
	f := wantFlag(t, d, "--timeout", "<seconds>")
	if !strings.HasPrefix(f.Desc, "The overall command timeout") {
		t.Errorf("timeout desc = %q", f.Desc)
	}
	f = wantFlag(t, d, "--json-output", "<path>")
	if !strings.Contains(f.Desc, "An optional path") {
		t.Errorf("json-output desc = %q", f.Desc)
	}
	wantFlag(t, d, "-v", "")
	if c := d.FindCommand("diagnose"); c.Desc != "Gather diagnostic information for debugging or filing bug reports." {
		t.Errorf("diagnose desc = %q", c.Desc)
	}
}

func TestGit(t *testing.T) {
	d := load(t, "git", "git")
	wantCommands(t, d, "clone", "init", "add", "commit", "push", "status")
	for _, bad := range []string{"start", "work", "grow", "collaborate", "examine"} {
		if d.FindCommand(bad) != nil {
			t.Errorf("%q is a group title, not a command", bad)
		}
	}
	if len(d.Usage) < 6 {
		t.Errorf("usage lines = %d", len(d.Usage))
	}
}

func TestBrctl(t *testing.T) {
	d := load(t, "brctl", "brctl")
	wantCommands(t, d, "diagnose", "log", "dump")
	wantFlag(t, d, "--help", "")
	wantFlag(t, d, "--doc", "<document-path>")
	if c := d.FindCommand("diagnose"); c.Desc != "diagnose and collect logs" {
		t.Errorf("diagnose desc = %q", c.Desc)
	}
}

func TestCupsctl(t *testing.T) {
	d := load(t, "cupsctl", "cupsctl")
	wantFlag(t, d, "-E", "")
	wantFlag(t, d, "-h", "server[:port]")
	f := wantFlag(t, d, "--debug-logging", "")
	if !slices.Contains(f.Names, "--no-debug-logging") {
		t.Errorf("names = %v", f.Names)
	}
}

func TestUsageDerivedCommands(t *testing.T) {
	d := load(t, "systemextensionsctl", "systemextensionsctl")
	if got := commandNames(d); !slices.Equal(got, []string{"developer", "list", "reset", "uninstall", "gc"}) {
		t.Errorf("commands = %v", got)
	}
	d = load(t, "spctl", "spctl")
	wantCommands(t, d, "developer-mode", "kext-consent")
	d = load(t, "pfctl", "pfctl")
	if len(d.Commands) != 0 {
		t.Errorf("pfctl has no subcommands, got %v", commandNames(d))
	}
}

func TestSystemctl(t *testing.T) {
	d := load(t, "systemctl", "systemctl")
	wantCommands(t, d, "list-units", "start", "stop", "set-property", "help", "is-system-running", "switch-root")
	if c := d.FindCommand("list-sockets"); c.Desc != "List socket units currently in memory, ordered by address" {
		t.Errorf("list-sockets desc = %q", c.Desc)
	}
	if c := d.FindCommand("start"); c.Args != "UNIT..." {
		t.Errorf("start args = %q", c.Args)
	}
	wantFlag(t, d, "--type", "TYPE")
	wantFlag(t, d, "-H", "[USER@]HOST")
	f := wantFlag(t, d, "--all", "")
	if !strings.HasSuffix(f.Desc, "use 'list-unit-files' instead.") {
		t.Errorf("all desc = %q", f.Desc)
	}
	wantFlag(t, d, "--no-pager", "")
}

func TestUrfave(t *testing.T) {
	d := load(t, "urfave", "crictl")
	wantCommands(t, d, "attach", "create", "exec", "version", "help")
	if c := d.FindCommand("h"); c == nil || c.Name != "help" {
		t.Errorf("alias h = %+v", c)
	}
	f := wantFlag(t, d, "--config", "value")
	if !slices.Equal(f.Names, []string{"--config", "-c"}) {
		t.Errorf("names = %v", f.Names)
	}
	wantFlag(t, d, "-D", "")
}

func TestArgparse(t *testing.T) {
	d := load(t, "argparse", "supervisorctl.py")
	if got := commandNames(d); !slices.Equal(got, []string{"status", "start", "stop"}) {
		t.Errorf("commands = %v", got)
	}
	f := wantFlag(t, d, "--configuration", "FILE")
	if f.Desc != "configuration file path (default: /etc/supervisord.conf)" {
		t.Errorf("desc = %q", f.Desc)
	}
}

func TestClapAndCobra(t *testing.T) {
	d := load(t, "clap", "thingctl")
	if got := commandNames(d); !slices.Equal(got, []string{"list", "apply", "help"}) {
		t.Errorf("clap commands = %v", got)
	}
	wantFlag(t, d, "--config", "<FILE>")
	wantFlag(t, d, "--version", "")

	d = load(t, "cobra", "doctl")
	if got := commandNames(d); !slices.Equal(got, []string{"account", "apps", "compute", "help"}) {
		t.Errorf("cobra commands = %v", got)
	}
	wantFlag(t, d, "--access-token", "string")
	wantFlag(t, d, "--trace", "")
}

func TestGoFlag(t *testing.T) {
	d := load(t, "goflag", "serverctl")
	f := wantFlag(t, d, "-addr", "string")
	if f.Desc != `listen address (default ":8080")` {
		t.Errorf("desc = %q", f.Desc)
	}
	wantFlag(t, d, "-debug", "")
	wantFlag(t, d, "-workers", "int")
}

func TestManPages(t *testing.T) {
	d := load(t, "man", "git", "commit")
	wantFlag(t, d, "--all", "")
	wantFlag(t, d, "--message", "<msg>")

	d = load(t, "overstrike", "pfctl")
	if d.Lines[0].Text != "NAME" {
		t.Errorf("overstrike not removed: %q", d.Lines[0].Text)
	}
	wantFlag(t, d, "-A", "")
}

func TestStylesAreSortedAndDisjoint(t *testing.T) {
	entries, _ := os.ReadDir("testdata")
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".txt")
		d := load(t, name, name)
		for _, l := range d.Lines {
			end := 0
			n := len([]rune(l.Text))
			for _, s := range l.Styles {
				if s.Start < end || s.End <= s.Start || s.End > n {
					t.Errorf("%s: bad span %+v in %q", name, s, l.Text)
				}
				end = s.End
			}
			for _, tg := range l.Targets {
				if tg.Start < 0 || tg.End > n || tg.End <= tg.Start {
					t.Errorf("%s: bad target %+v in %q", name, tg, l.Text)
				}
			}
		}
	}
}

func TestClean(t *testing.T) {
	got := Clean("\x1b[1mBold\x1b[0m\tx\r\n\n")
	if got != "Bold    x" {
		t.Errorf("Clean = %q", got)
	}
}

func TestUsageAlternatives(t *testing.T) {
	// cpuctl -h followed by its man page, as the loader combines them.
	d := load(t, "cpuctl", "cpuctl")
	if got := commandNames(d); !slices.Equal(got, []string{"list", "offline", "online"}) {
		t.Fatalf("commands = %v", got)
	}
	for _, c := range d.Commands {
		if !c.Fallback {
			t.Errorf("%s should be marked as inferred", c.Name)
		}
	}
	if c := d.FindCommand("list"); !strings.HasPrefix(c.Desc, "For each CPU in the system") {
		t.Errorf("list desc = %q", c.Desc)
	}
	if c := d.FindCommand("offline"); c.Desc != "Set the specified CPUs off line." {
		t.Errorf("offline desc = %q", c.Desc)
	}
	if d.FindCommand("command") != nil {
		t.Error(`"command" is a placeholder, not a subcommand`)
	}

	for in, want := range map[string][]string{
		" [ list ]":                       {"list"},
		" { offline | online } <cpu>":     {"offline", "online"},
		" start|stop|restart":             {"start", "stop", "restart"},
		" [-v] [-f file] {up,down} NAME":  {"up", "down"},
		" developer [on|off]":             {"developer"},
		" command [arguments]":            nil,
		" [ httpd-argument ]":             nil,
		" [flags] [options]":              nil,
		" <subcommand> ... | help":        nil,
		" [OPTIONS] <COMMAND>":            nil,
		" [-t table -T command [addr]]":   nil,
		" --assess [--type type] path":    nil,
		" [global options] command [opt]": nil,
	} {
		var got []string
		for _, a := range usageAlternatives(in) {
			got = append(got, a.word)
		}
		if !slices.Equal(got, want) {
			t.Errorf("usageAlternatives(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestManDefinitionList(t *testing.T) {
	// apachectl --help only prints errors; the commands are in its man page.
	d := load(t, "apachectl", "apachectl")
	want := []string{"start", "stop", "restart", "fullstatus", "status", "configtest", "startssl"}
	if got := commandNames(d); !slices.Equal(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	if c := d.FindCommand("graceful-stop"); c == nil || c.Name != "stop" {
		t.Errorf("graceful-stop should be an alias of stop: %+v", c)
	}
	if c := d.FindCommand("status"); !strings.HasPrefix(c.Desc, "Displays a brief status report.") {
		t.Errorf("status desc = %q", c.Desc)
	}
	if c := d.FindCommand("start"); c.Desc != "Loads the org.apache.httpd launchd job." {
		t.Errorf("start desc = %q", c.Desc)
	}
	// The entries are clickable.
	l := d.Lines[d.FindCommand("configtest").Line]
	if len(l.Targets) == 0 || l.Targets[0].Target.Value != "configtest" {
		t.Errorf("configtest targets = %+v", l.Targets)
	}
	// Prose in the same man page is not a list.
	for _, bad := range []string{"httpd", "taking", "details", "script", "appropriate"} {
		if d.FindCommand(bad) != nil {
			t.Errorf("%q is prose, not a command", bad)
		}
	}
}

func TestHelpMechanisms(t *testing.T) {
	cases := []struct {
		name                 string
		path                 []string
		flag, dashH, helpCmd bool
	}{
		{"kubectl", []string{"kubectl"}, true, false, false}, // Use "kubectl <command> --help"
		{"kubectl_config", []string{"kubectl", "config"}, true, false, false},
		{"cobra", []string{"doctl"}, true, true, true},
		{"launchctl", []string{"launchctl"}, false, false, true}, // only `launchctl help <sub>`
		{"git", []string{"git"}, true, false, false},             // usage: [-h | --help]
		{"systemctl", []string{"systemctl"}, true, true, true},
		{"cpuctl", []string{"cpuctl"}, false, false, false},
		{"apachectl", []string{"apachectl"}, false, false, false},
		{"systemextensionsctl", []string{"systemextensionsctl"}, false, false, false},
	}
	for _, c := range cases {
		d := load(t, c.name, c.path...)
		if d.HelpFlag() != c.flag || d.HelpDashH() != c.dashH || d.HelpCommand() != c.helpCmd {
			t.Errorf("%s: --help %v, -h %v, help command %v; want %v %v %v",
				c.name, d.HelpFlag(), d.HelpDashH(), d.HelpCommand(), c.flag, c.dashH, c.helpCmd)
		}
	}
}

func TestMoreBSDStyles(t *testing.T) {
	cases := []struct {
		fixture string
		want    []string
	}{
		// The man page synopsis names the commands; later lists of pipe
		// settings (bw, delay, ...) must not be added.
		{"dnctl", []string{"list", "show", "flush", "delete", "pipe", "queue"}},
		// Usage: rdma_ctl <status|enable|disable>
		{"rdma_ctl", []string{"status", "enable", "disable"}},
		// list | get   : List the principals ...
		{"ncctl", []string{"list", "unset", "set"}},
		// An indented "Available commands:" header and a bare list.
		{"xsanctl", []string{"help", "ping", "firstBoot", "mount", "unmount", "list"}},
		// where COMMAND includes: (tab-indented names)
		{"skywalkctl", []string{"channel", "flow", "flow-adv", "interface"}},
		// [-d] ... [codec|index|default] picks an argument, not a subcommand.
		{"codecctl", nil},
	}
	for _, c := range cases {
		d := load(t, c.fixture, c.fixture)
		got := commandNames(d)
		if c.want == nil {
			if len(got) != 0 {
				t.Errorf("%s: commands = %v, want none", c.fixture, got)
			}
			continue
		}
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("%s: missing %q in %v", c.fixture, w, got)
			}
		}
		if c.fixture != "xsanctl" && c.fixture != "skywalkctl" && len(got) != len(c.want) {
			t.Errorf("%s: commands = %v, want %v", c.fixture, got, c.want)
		}
	}

	d := load(t, "ncctl", "ncctl")
	if c := d.FindCommand("get"); c == nil || c.Name != "list" || !strings.HasPrefix(c.Desc, "List the principals") {
		t.Errorf("ncctl get = %+v", c)
	}
	d = load(t, "xsanctl", "xsanctl")
	if got := commandNames(d); got[3] != "mount" || len(got) < 15 {
		t.Errorf("xsanctl commands out of order or missing: %v", got)
	}
	if c := d.FindCommand("createSan"); c == nil || c.Args != "SAN_Name [createMaster options]" {
		t.Errorf("xsanctl createSan = %+v", c)
	}
	d = load(t, "dnctl", "dnctl")
	if c := d.FindCommand("flush"); c == nil || !c.Fallback {
		t.Errorf("dnctl flush = %+v", c)
	}
}
