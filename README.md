# cast-director

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
| `wait TEXT` | Record until TEXT appears (plain text, not a regex) |
| `prompt` | Wait for the shell prompt (`$ `) |
| `speed N` | Save what follows N times faster (`1` = normal, `0.5` = slower) |
| `sleep N` | Keep recording for N seconds (decimals OK) |

`wait` and `prompt` only look at output that came after the last match, so each `prompt` waits for a new prompt.
`speed` applies to everything after it, also `sleep`: `sleep 2` at `speed 4` is saved as half a second.

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
