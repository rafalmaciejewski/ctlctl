package shell

import (
	"slices"
	"testing"
)

func TestFields(t *testing.T) {
	got := Fields(`kubectl get pods -o "json path" --template='{{.x}}' | grep x`)
	want := []string{"kubectl", "get", "pods", "-o", "json path", "--template={{.x}}", "grep", "x"}
	if !slices.Equal(got, want) {
		t.Errorf("Fields = %q, want %q", got, want)
	}
}

type fakeResolver struct{}

func (fakeResolver) Subcommand(path []string, word string) bool {
	return len(path) == 1 && path[0] == "kubectl" && word == "get"
}

func (fakeResolver) FlagTakesValue(_ []string, flag string) bool { return flag == "-n" }

func TestHighlight(t *testing.T) {
	s := `FOO=1 kubectl get pods -n kube-system --all=true "$HOME" 2>&1 | less # done`
	cls := Classes(s, fakeResolver{})
	at := func(word string) Class {
		for i := 0; i+len(word) <= len(s); i++ {
			if s[i:i+len(word)] == word {
				return cls[i]
			}
		}
		t.Fatalf("%q not found", word)
		return 0
	}
	cases := []struct {
		word string
		want Class
	}{
		{"FOO", ClassAssign},
		{"kubectl", ClassProgram},
		{"get", ClassSubcommand},
		{"pods", ClassArg},
		{"-n", ClassFlag},
		{"kube-system", ClassValue},
		{"--all", ClassFlag},
		{"true", ClassValue},
		{`"$HOME"`, ClassString},
		{"$HOME", ClassVariable},
		{"2>&1", ClassOperator},
		{"| less", ClassOperator},
		{"less", ClassProgram},
		{"# done", ClassComment},
	}
	for _, c := range cases {
		if got := at(c.word); got != c.want {
			t.Errorf("%q: class %d, want %d", c.word, got, c.want)
		}
	}
}

func TestUnterminatedQuote(t *testing.T) {
	items := Split(`echo "abc`)
	if len(items) != 2 || items[1].Word.Value != "abc" {
		t.Errorf("Split = %+v", items)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{"abc": "abc", "a b": "'a b'", "it's": `'it'\''s'`, "": "''"} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}
