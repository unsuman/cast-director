package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	good := `# demo
run limactl start      # boot
speed 4

wait READY. Run
speed 0.5
prompt
type echo a#b
enter
sleep 3
`
	steps, err := parse("demo.txt", strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{Line: 2, Cmd: "run", Arg: "limactl start"},
		{Line: 3, Cmd: "speed", Arg: "4", N: 4},
		{Line: 5, Cmd: "wait", Arg: "READY. Run"},
		{Line: 6, Cmd: "speed", Arg: "0.5", N: 0.5},
		{Line: 7, Cmd: "prompt"},
		{Line: 8, Cmd: "type", Arg: "echo a#b"},
		{Line: 9, Cmd: "enter"},
		{Line: 10, Cmd: "sleep", Arg: "3", N: 3},
	}
	if len(steps) != len(want) {
		t.Fatalf("got %d steps, want %d: %+v", len(steps), len(want), steps)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Errorf("step %d: got %+v, want %+v", i, steps[i], want[i])
		}
	}

	bad := []struct{ script, err string }{
		{"run ls\n\n# x\n\nfoo bar\n", `demo.txt:5: unknown command "foo"`},
		{"run ls\nspeed fast\n", `demo.txt:2: bad number "fast"`},
		{"speed 0\n", `demo.txt:1: bad number "0"`},
		{"sleep -1\n", `demo.txt:1: bad number "-1"`},
		{"prompt\nwait\n", `demo.txt:2: wait needs text`},
		{"enter now\n", `demo.txt:1: enter takes no argument`},
	}
	for _, b := range bad {
		_, err := parse("demo.txt", strings.NewReader(b.script))
		if err == nil || err.Error() != b.err {
			t.Errorf("parse(%q): got %v, want %s", b.script, err, b.err)
		}
	}
}

type event struct {
	time float64
	text string
}

// readCast checks the .cast format and returns its events.
func readCast(t *testing.T, path string) []event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	if !sc.Scan() {
		t.Fatal("empty .cast")
	}
	var header map[string]any
	if err := json.Unmarshal(sc.Bytes(), &header); err != nil || header["version"] != 2.0 {
		t.Fatalf("bad header %s: %v", sc.Text(), err)
	}
	var events []event
	last := 0.0
	for sc.Scan() {
		var e []any
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("bad line %s: %v", sc.Text(), err)
		}
		tm, ok1 := e[0].(float64)
		code, ok2 := e[1].(string)
		text, ok3 := e[2].(string)
		if len(e) != 3 || !ok1 || !ok2 || !ok3 || code != "o" {
			t.Fatalf("bad event %s", sc.Text())
		}
		if tm < last {
			t.Fatalf("time goes down: %v after %v", tm, last)
		}
		last = tm
		events = append(events, event{tm, text})
	}
	return events
}

// find returns the time of the first event containing s.
func find(t *testing.T, events []event, s string) float64 {
	t.Helper()
	for _, e := range events {
		if strings.Contains(e.text, s) {
			return e.time
		}
	}
	t.Fatalf("%q not found in recording", s)
	return 0
}

func TestRecordSpeed(t *testing.T) {
	dir := t.TempDir()
	fake := `#!/bin/sh
for i in 1 2 3 4 5 6 7 8; do
	echo "boot step $i"
	sleep 1
done
echo "READY. Run lima to open the shell."
`
	if err := os.WriteFile(filepath.Join(dir, "limactl"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	script := filepath.Join(dir, "demo.txt")
	os.WriteFile(script, []byte("run limactl start\nspeed 4\nwait READY. Run\nspeed 1\nprompt\nsleep 2\n"), 0o644)
	var stderr strings.Builder
	if code := run([]string{script}, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}

	events := readCast(t, filepath.Join(dir, "demo.cast"))
	boot := find(t, events, "READY. Run") - find(t, events, "boot step 1")
	if boot > 2.5 || boot < 1.5 {
		t.Errorf("boot took %.2fs in the .cast, want about 2s (8s at speed 4)", boot)
	}
	n := len(events)
	if events[n-1].text != "" {
		t.Errorf("last event is %q, want empty", events[n-1].text)
	}
	if tail := events[n-1].time - events[n-2].time; tail < 1.9 || tail > 2.5 {
		t.Errorf("final sleep is %.2fs in the .cast, want about 2s", tail)
	}
}

func TestHide(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "demo.txt")
	os.WriteFile(script, []byte("hide\nrun sleep 2; echo sec''ret\nprompt\nshow\nrun echo visible\nprompt\n"), 0o644)
	var stderr strings.Builder
	if code := run([]string{script}, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	events := readCast(t, filepath.Join(dir, "demo.cast"))
	for _, e := range events {
		if strings.Contains(e.text, "secret") {
			t.Errorf("hidden output was recorded: %q", e.text)
		}
	}
	// Without hide, typing and sleep 2 would take over 3s.
	if at := find(t, events, "visible\r\n"); at > 2.5 {
		t.Errorf("visible output at %.2fs, want hidden time left out", at)
	}
}

func TestWaitTimeout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "demo.txt")
	os.WriteFile(script, []byte("run echo hello\nwait never shows up\n"), 0o644)
	out := filepath.Join(dir, "out.cast")
	var stderr strings.Builder
	code := run([]string{"-o", out, "-timeout", "1", script}, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := script + ":2: "; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("stderr %q, want prefix %q", stderr.String(), want)
	}
	// The .cast is still saved.
	find(t, readCast(t, out), "hello")
}
