// Command cast-director runs a simple script in a real terminal and saves it
// as an asciinema .cast file (asciicast v2). The speed can change in the
// middle of a recording, so slow parts can be stored faster.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Step is one line of a script.
type Step struct {
	Line int
	Cmd  string
	Arg  string
	N    float64 // for speed, sleep and maxidle
}

// parse reads a script. It checks every line before anything runs.
func parse(name string, r io.Reader) ([]Step, error) {
	var steps []Step
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(stripComment(sc.Text()))
		if line == "" {
			continue
		}
		cmd, arg, _ := strings.Cut(line, " ")
		arg = strings.TrimSpace(arg)
		s := Step{Line: n, Cmd: cmd, Arg: arg}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", name, n, fmt.Sprintf(format, a...))
		}
		switch cmd {
		case "run", "type", "wait":
			if arg == "" {
				return nil, fail("%s needs text", cmd)
			}
		case "key":
			if arg == "" {
				return nil, fail("key needs a key name")
			}
			if _, err := keys(arg); err != nil {
				return nil, fail("%v", err)
			}
		case "enter", "prompt", "hide", "show":
			if arg != "" {
				return nil, fail("%s takes no argument", cmd)
			}
		case "speed", "sleep", "maxidle":
			v, err := strconv.ParseFloat(arg, 64)
			if err != nil || v < 0 || (cmd == "speed" && v == 0) {
				return nil, fail("bad number %q", arg)
			}
			s.N = v
		default:
			return nil, fail("unknown command %q", cmd)
		}
		steps = append(steps, s)
	}
	return steps, sc.Err()
}

// stripComment removes a # comment. # starts a comment at the start of a
// line or after a space, so text like "a#b" is kept.
func stripComment(line string) string {
	for i, c := range line {
		if c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}

// specialKeys maps key names to what a terminal sends for them.
var specialKeys = map[string]string{
	"enter":     "\r",
	"tab":       "\t",
	"space":     " ",
	"esc":       "\x1b",
	"backspace": "\x7f",
	"delete":    "\x1b[3~",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"right":     "\x1b[C",
	"left":      "\x1b[D",
	"home":      "\x1b[H",
	"end":       "\x1b[F",
	"pageup":    "\x1b[5~",
	"pagedown":  "\x1b[6~",
}

// keys turns a list like "up ctrl+c enter" into what a terminal sends.
func keys(list string) ([]string, error) {
	var out []string
	for _, name := range strings.Fields(strings.ToLower(list)) {
		if seq, ok := specialKeys[name]; ok {
			out = append(out, seq)
		} else if c, ok := strings.CutPrefix(name, "ctrl+"); ok && len(c) == 1 && c[0] >= 'a' && c[0] <= 'z' {
			out = append(out, string(rune(c[0]-'a'+1)))
		} else {
			return nil, fmt.Errorf("unknown key %q", name)
		}
	}
	return out, nil
}

// play runs the steps on the recorder.
func play(rec *Recorder, steps []Step, timeout time.Duration) (Step, error) {
	for _, s := range steps {
		var err error
		switch s.Cmd {
		case "run":
			if err = rec.Type(s.Arg); err == nil {
				err = rec.Enter()
			}
		case "type":
			err = rec.Type(s.Arg)
		case "enter":
			err = rec.Enter()
		case "key":
			seqs, _ := keys(s.Arg) // checked by parse
			err = rec.Keys(seqs)
		case "wait":
			err = rec.Wait(s.Arg, timeout)
		case "prompt":
			err = rec.Wait("$ ", timeout)
		case "hide":
			rec.SetHidden(true)
		case "show":
			rec.SetHidden(false)
		case "speed":
			rec.SetSpeed(s.N)
		case "maxidle":
			rec.SetMaxIdle(s.N)
		case "sleep":
			rec.Sleep(time.Duration(s.N * float64(time.Second)))
		}
		if err != nil {
			return s, err
		}
	}
	return Step{}, nil
}

func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("cast-director", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output file (default: script name with .cast)")
	width := fs.Int("width", 100, "terminal width")
	height := fs.Int("height", 30, "terminal height")
	timeout := fs.Float64("timeout", 60, "real seconds that wait and prompt wait")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: cast-director [-o out.cast] [-width 100] [-height 30] [-timeout 60] script.txt")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	script := fs.Arg(0)
	if *out == "" {
		*out = strings.TrimSuffix(script, filepath.Ext(script)) + ".cast"
	}

	f, err := os.Open(script)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	steps, err := parse(script, f)
	f.Close()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	rec, err := NewRecorder(*out, *width, *height)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	wait := time.Duration(*timeout * float64(time.Second))
	code := 0
	if err := rec.Wait("$ ", wait); err != nil {
		fmt.Fprintf(stderr, "%s: shell did not start: %v\n", script, err)
		code = 1
	} else if s, err := play(rec, steps, wait); err != nil {
		fmt.Fprintf(stderr, "%s:%d: %s: %v\n", script, s.Line, s.Cmd, err)
		code = 1
	}
	// Save the .cast even after an error.
	if err := rec.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintln(stderr, err)
		code = 1
	}
	return code
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}
