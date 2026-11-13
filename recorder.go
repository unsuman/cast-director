package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
)

// Recorder runs bash in a PTY and writes everything it prints to an
// asciicast v2 file.
//
// The recording clock vt only moves forward: each output chunk adds
// realElapsed/speed to it. So with speed 4, eight real seconds become two
// seconds in the .cast.
type Recorder struct {
	ptmx *os.File
	cmd  *exec.Cmd
	file *os.File
	w    *bufio.Writer

	mu       sync.Mutex
	vt       float64   // recording time in seconds
	lastReal time.Time // real time that vt corresponds to
	speed    float64
	maxIdle  float64 // longest gap between events in the .cast, 0 = no limit
	cut      float64 // recording time removed by maxIdle
	lastOut  float64 // .cast time of the last event
	hidden   bool    // output is not recorded and time does not pass
	buf      string  // output not yet consumed by Wait
	closed   bool
	err      error // first write error

	changed chan struct{} // signalled when new output arrives
	done    chan struct{} // closed when the shell's output ends
}

// NewRecorder creates the .cast file, writes the header and starts bash.
func NewRecorder(out string, width, height int) (*Recorder, error) {
	f, err := os.Create(out)
	if err != nil {
		return nil, err
	}
	r := &Recorder{
		file:    f,
		w:       bufio.NewWriter(f),
		speed:   1,
		changed: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	header, _ := json.Marshal(struct {
		Version   int               `json:"version"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Timestamp int64             `json:"timestamp"`
		Env       map[string]string `json:"env"`
	}{2, width, height, time.Now().Unix(), map[string]string{"SHELL": "/bin/bash", "TERM": "xterm-256color"}})
	r.w.Write(append(header, '\n'))

	r.cmd = exec.Command("bash", "--norc", "--noprofile")
	r.cmd.Env = append(os.Environ(),
		"PS1=$ ",
		"PS2=> ",
		"PROMPT_COMMAND=",
		"HISTFILE=/dev/null",
		"TERM=xterm-256color",
		"BASH_SILENCE_DEPRECATION_WARNING=1", // macOS /bin/bash
	)
	r.ptmx, err = pty.StartWithSize(r.cmd, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
	if err != nil {
		f.Close()
		return nil, err
	}
	r.lastReal = time.Now()
	go r.read()
	return r, nil
}

// read stamps each chunk of output with its real arrival time and records it.
func (r *Recorder) read() {
	defer close(r.done)
	b := make([]byte, 32*1024)
	var pending []byte // an incomplete UTF-8 sequence from the last read
	for {
		n, err := r.ptmx.Read(b)
		if n > 0 {
			now := time.Now()
			data := append(pending, b[:n]...)
			data, pending = splitUTF8(data)
			if len(data) > 0 {
				r.output(now, string(data))
			}
		}
		if err != nil {
			return
		}
	}
}

func (r *Recorder) output(at time.Time, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.advance(at)
	if !r.hidden {
		r.event(text)
	}
	r.buf += text
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

// advance moves the recording clock to real time t at the current speed.
// While hidden, the time is dropped. The caller must hold r.mu.
func (r *Recorder) advance(t time.Time) {
	if d := t.Sub(r.lastReal); d > 0 {
		if !r.hidden {
			r.vt += d.Seconds() / r.speed
		}
		r.lastReal = t
	}
}

// event writes one "o" event at the current recording time, shortening the
// gap since the last event to maxIdle. The caller must hold r.mu.
func (r *Recorder) event(text string) {
	t := r.vt - r.cut
	if r.maxIdle > 0 && t-r.lastOut > r.maxIdle {
		r.cut += t - r.lastOut - r.maxIdle
		t = r.lastOut + r.maxIdle
	}
	r.lastOut = t
	line, _ := json.Marshal([]any{math.Round(t*1e6) / 1e6, "o", text})
	if _, err := r.w.Write(append(line, '\n')); err != nil && r.err == nil {
		r.err = err
	}
}

// SetSpeed stores everything from now on n times faster.
func (r *Recorder) SetSpeed(n float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advance(time.Now()) // time so far counts at the old speed
	r.speed = n
}

// SetMaxIdle limits every pause from now on to n seconds in the .cast.
// 0 removes the limit.
func (r *Recorder) SetMaxIdle(n float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxIdle = n
}

// SetHidden stops (true) or restarts (false) recording. Hidden output is
// still seen by Wait, and hidden time is left out of the .cast.
func (r *Recorder) SetHidden(h bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advance(time.Now())
	r.hidden = h
}

// Type writes text to the shell one character at a time, like a human.
func (r *Recorder) Type(text string) error {
	for _, c := range text {
		if _, err := r.ptmx.WriteString(string(c)); err != nil {
			return err
		}
		time.Sleep(time.Duration(40+rand.IntN(60)) * time.Millisecond)
	}
	return nil
}

// Enter presses Enter.
func (r *Recorder) Enter() error {
	_, err := r.ptmx.WriteString("\r")
	return err
}

// Wait records until text appears in output not yet matched by an earlier
// Wait. The timeout is real time.
func (r *Recorder) Wait(text string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		r.mu.Lock()
		i := strings.Index(r.buf, text)
		if i >= 0 {
			r.buf = r.buf[i+len(text):]
		}
		r.mu.Unlock()
		if i >= 0 {
			return nil
		}
		select {
		case <-r.changed:
		case <-r.done:
			return fmt.Errorf("shell exited before %q appeared", text)
		case <-timer.C:
			return fmt.Errorf("%q did not appear within %v", text, timeout)
		}
	}
}

// Sleep keeps recording for d of real time.
func (r *Recorder) Sleep(d time.Duration) {
	time.Sleep(d)
}

// Close stops the shell and finishes the .cast file. It writes one empty
// event at the current time, so a final sleep is kept.
func (r *Recorder) Close() error {
	r.mu.Lock()
	r.advance(time.Now())
	r.event("")
	r.closed = true
	err := r.err
	r.mu.Unlock()

	r.ptmx.Close()
	r.cmd.Process.Kill()
	r.cmd.Wait()

	return errors.Join(err, r.w.Flush(), r.file.Close())
}

// splitUTF8 splits b into complete UTF-8 text and an incomplete trailing
// sequence that should be kept for the next read.
func splitUTF8(b []byte) (complete, rest []byte) {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i], append([]byte(nil), b[i:]...)
			}
			break
		}
	}
	return b, nil
}
