# cast-director

[![CI](https://github.com/unsuman/cast-director/actions/workflows/ci.yml/badge.svg)](https://github.com/unsuman/cast-director/actions/workflows/ci.yml)

**Script your terminal demos. Speed up the boring parts.**

`cast-director` runs a small script in a real terminal and saves it as an [asciinema](https://asciinema.org) `.cast` file.

The special part: **the speed can change in the middle of a recording, when some text appears.**
Waiting 40 seconds for a VM to boot? Save it 4× faster until `READY` shows up, then go back to normal speed.

## Example

`demo.txt`:

```
run limactl start
speed 4
wait READY. Run
speed 1
prompt
run lima uname -a
prompt
sleep 3
```

```bash
cast-director demo.txt     # writes demo.cast
asciinema play demo.cast   # watch it
agg demo.cast demo.gif     # or make a GIF
```

The timing is saved inside the `.cast` file, so it plays the same everywhere: on a website with [asciinema-player](https://docs.asciinema.org/manual/player/), in a terminal with `asciinema play`, or as a GIF with [agg](https://github.com/asciinema/agg).

See [`examples/lima.txt`](examples/lima.txt) and the recording it made, [`examples/lima.cast`](examples/lima.cast).

## Install

```bash
go install github.com/unsuman/cast-director@latest
```

Works on macOS and Linux.

## Commands

One command per line. `#` is a comment (at the start of a line or after a space).

| Command | What it does |
|---|---|
| `run TEXT` | Type TEXT, then press Enter |
| `type TEXT` | Type TEXT, no Enter |
| `enter` | Press Enter |
| `key KEYS` | Press special keys, e.g. `key up enter` or `key ctrl+c` (list below) |
| `wait TEXT` | Record until TEXT appears (plain text, not a regex) |
| `prompt` | Wait for the shell prompt (`$ `) |
| `speed N` | Save what follows N times faster (`1` = normal, `0.5` = slower) |
| `maxidle N` | Save every pause after this as at most N seconds (`0` = no limit) |
| `typespeed MS` | Type about one key every MS milliseconds (default `70`, `0` = instant) |
| `sleep N` | Keep recording for N seconds (decimals OK) |
| `hide` | Stop recording: setup steps still run, but are not saved |
| `show` | Start recording again |

`wait` and `prompt` only look at output that came after the last match, so each `prompt` waits for a new prompt.

`key` takes one or more key names separated by spaces: `enter`, `tab`, `space`, `esc`, `backspace`, `delete`, `up`, `down`, `left`, `right`, `home`, `end`, `pageup`, `pagedown`, and `ctrl+a` to `ctrl+z`. Use it to stop a program (`key ctrl+c`), go through shell history (`key up`), or show tab completion (`type cat READ` then `key tab`).
`speed` applies to everything after it, also `sleep`: `sleep 2` at `speed 4` is saved as half a second.

`maxidle` works like `asciinema rec -i`: no screen stays still for longer than N seconds in the `.cast`, so you don't need to know in advance where the slow parts are. It is applied after `speed` and also shortens `sleep`.

`typespeed` sets how fast `run`, `type` and `key` press keys. Each pause is random, between half and one and a half times MS, so typing looks human.

Use `hide` and `show` for setup the viewer should not see. Time spent hidden is left out, and `wait` and `prompt` still work. End the hidden part at a prompt, so the screen still looks right afterwards:

```
hide
run cd $(mktemp -d) && git init -q
prompt
show
run git status
```

The script runs in `bash --norc --noprofile` with the prompt `$ `, so every recording looks the same.

## Options

```
cast-director [-o out.cast] [-width 100] [-height 30] [-timeout 60] script.txt
```

`-o` defaults to the script name with `.cast` (`demo.txt` → `demo.cast`).
`-timeout` is how many real seconds `wait` and `prompt` wait before giving up.

The whole script is checked before anything runs, so a typo stops it right away:
`demo.txt:5: unknown command "foo"`.
If a step fails while recording, `cast-director` prints the line, exits with code 1, and still saves the `.cast` up to that point.

## Why not...

- **`asciinema rec`**: you type by hand, and you can't speed up only one part.
- **[VHS](https://github.com/charmbracelet/vhs)**: great, but it makes GIF/MP4, not `.cast`, and has one speed for the whole video.

## License

Apache-2.0
