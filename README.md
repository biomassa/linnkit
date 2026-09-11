# linnkit

linnkit is a terminal app for the Roger Linn LinnStrument. It reads Scala (`.scl`) scale files. It shows the contents of a scale. It makes a row layout and a light pattern for the scale. It sends them to the LinnStrument.

linnkit works with all types of scale: just intonation, equal divisions (EDO), non-octave scales and irregular scales.

linnkit does not write `.scl` files. It writes `.kbm` files only for synths that need them.

## Requirements

- macOS.
- A LinnStrument (full size, 200 pads). linnkit was tested with firmware 2.3.4.
- A USB connection to the LinnStrument. The MIDI port name must contain `LinnStrument MIDI`.
- To build: Go 1.27 or later and the Xcode command-line tools (the MIDI driver uses cgo).
- A terminal window of 160 × 50 characters or larger.

## Install

1. Clone the repository.
2. Go to the repository folder.
3. Build the app:

   ```
   make build
   ```

4. The app is `bin/linnkit`.

To run the tests, use `make check`.

## Scale folders

linnkit reads `.scl` files from these folders, if they exist:

- `SCL/` in the repository
- `~/SCL`
- the folders that you add with `linnkit folders add DIR`

To see the folders, use `linnkit folders`. To remove a folder, use `linnkit folders remove DIR`.

To use other folders for one session, use `linnkit tui --scales DIR1,DIR2`.

## Start the dashboard

1. Connect the LinnStrument.
2. Touch a pad to wake the LinnStrument. linnkit gets no answer from a LinnStrument that sleeps.
3. Make sure that the LinnStrument shows its normal play surface, not a settings screen.
4. Run `bin/linnkit`.

The dashboard has these panes:

| Pane | Contents |
|---|---|
| SCALES | The list of scale files, with size and type |
| SCALE | The scale summary and the table of degrees |
| LAYOUT | Row offsets for the scale, best first |
| LIGHTS | The light scheme |
| SYNTH | The synth and its pitch-bend range |
| GRID | A preview of the 25 × 8 pads |
| SEND | The light slot and the send options |

Use Tab, or the Left and Right arrows, to go to the next pane. Use the Up and Down arrows to move in a pane. Press `?` to see all keys.

## Keys

| Key | Action |
|---|---|
| ↑ ↓ | Move in the pane |
| Tab, ← → | Go to the next or the previous pane |
| Enter | Load the selected scale |
| `/` | Filter the scale list by name or description |
| `[` `]` | Move the bottom-left note down or up (LAYOUT pane) |
| `{` `}` | Move the root note down or up (LAYOUT pane) |
| `<` `>` | Change the prime limit (LIGHTS pane) or the synth bend range (SYNTH pane) |
| `0` `1` `2` | Select the custom light slot |
| `l` | Send the row layout: on or off |
| `c` | Configure MIDI: on or off |
| `f` | Send the factory 12-TET layout: on or off |
| `s` | Send to the LinnStrument (linnkit asks first) |
| `p` | Open the presets |
| `e` | Export to Madrona Labs synths |
| `b` | Back up the LinnStrument settings |
| `r` | Restore the latest backup |
| `m` | Show the interval matrix |
| `t` | Show the full table of degrees |
| `q` | Quit |

## Send a scale to the LinnStrument

1. In SCALES, select a scale. Press Enter.
2. In LAYOUT, select a row offset.
3. In LIGHTS, select a light scheme.
4. In SYNTH, select your synth.
5. In SEND, select a light slot (`0`, `1` or `2`).
6. Press `s`.
7. Read the summary. Press `y` to send, or `n` to cancel.

linnkit then does these steps:

1. It reads all LinnStrument settings and keeps a backup in `~/.config/linnkit/backups`.
2. It sends the MIDI configuration, if "configure MIDI" is on.
3. It sends the row layout, if "send row layout" is on.
4. It paints the light pattern into the light slot and saves the slot.
5. It reads the settings again and compares them with the values that it sent.

**Caution:** A send replaces the light pattern in the slot that you select. Slot 2 is the default.

To go back to the settings before the send, press `r`. Then press `y`.

### Light schemes

| Scheme | Colors |
|---|---|
| Just-interval families | One color for each prime limit (3, 5, 7, 11, 13) |
| Note names | The root, naturals, sharps and flats in different colors |
| MOS inside the scale | The notes of a smaller moment-of-symmetry scale |
| Root only | The root only |

The LinnStrument has 10 colors. Each LED is either on or off for red, green and blue. White, orange, lime and pink are mixes of two colors. Thus, white looks light cyan and pink looks salmon on the pads. The grid preview shows these mixes.

### Factory 12-TET layout

Press `f` to send the layout that the LinnStrument had from the factory:

- Rows are a fourth (5 semitones) apart.
- The bottom-left pad is F#1 (MIDI 30).
- C is cyan. The other natural notes are green.

This send does not change the custom light slots. The LinnStrument does not keep these settings after power-off until you save them. To save them, push and release a control button (for example, Preset) one time after the send.

## Pitch bend and slides

A slide across one pad must change the pitch by one scale step. For this, the LinnStrument Bend Range (B) and the synth per-note bend range (S) must agree:

    one pad of slide = 100 × S / B cents

In SYNTH, select your synth and set S to the value in the synth. linnkit calculates B. With "configure MIDI" on, the send sets B on the LinnStrument.

Example: for 31-EDO with the synth at 12 semitones, B is 31.

For scales with unequal steps, one pad of slide is the average step. The SYNTH pane shows the largest error.

### Synth profiles

| Profile | Tuning | Bend range |
|---|---|---|
| Aalto / Kaivo | `.scl` + `.kbm` in `~/Music/Madrona Labs/Scales` | 12, 24, 48 or 96 |
| Pigments | `.scl` (set Reference Note C3 = MIDI 60 when you load it) | 2–96 |
| Surge XT | `.scl` + `.kbm`, or MTS-ESP | 1–96 |
| Plasmonic | MTS-ESP | 1–96 |
| Cypher2 | `.tun` | 48 |
| Ableton Live built-ins | Live 12 Tuning System | 48 |
| Live tuning + MPE plugin | Live 12 Tuning System | 48 |
| Bitwig built-ins / Grid | Micro-pitch device (12 notes or fewer) | 1–96 |
| Legacy (non-MPE) | In the synth | 1–24, One Channel mode |
| Generic MPE | In the synth | 1–96 |

The SYNTH pane shows if linnkit checked the profile on a real synth.

### Ableton Live 12

Use this procedure for synths that have no tuning function, for example Expressive E Noisy 2:

1. Load the `.scl` file from the Tunings section of the Live browser.
2. In the plugin, set MPE to on.
3. In the plugin, set the per-note pitch bend range to 48.
4. On the track, make sure that "Bypass Tuning" is off.
5. In linnkit, select "Live tuning + MPE plugin". The send sets B to 48.

With a tuning, Live measures pitch bend in scale steps. Thus, B = 48 and S = 48 give one scale step for each pad.

## Presets and saved settings

linnkit keeps the settings of each scale: row offset, bottom-left note, root, reference frequency, light scheme and synth. When you load the scale again, linnkit uses these settings.

A preset keeps a scale, its settings and a light slot under a name.

1. Press `p`.
2. Press `a` to save the current scale as a preset. Type a name. Press Enter.
3. To load a preset, select it and press Enter. A load does not send to the LinnStrument. Press `s` to send.
4. To delete a preset, select it and press `d`. Then press `y`.

linnkit keeps its data in `~/.config/linnkit`. To use a different folder, set `LINNKIT_CONFIG_DIR`.

## Export to Madrona Labs synths

Aalto and Kaivo read scales from `~/Music/Madrona Labs/Scales`.

1. Load a scale.
2. Press `e`.
3. Set the root with `{` and `}`. Press `h` to type a reference frequency, or `z` for the 12-TET frequency of the root.
4. Press `a` to export all scales in the list, not only the current scale.
5. Press `y` to write the files.

linnkit copies each `.scl` file with no changes. It writes a `.kbm` file with the same name. The `.kbm` file puts degree 0 on the root note and lists all degrees (Aalto needs all degrees).

Aalto reads some `.scl` files differently from the Scala standard. For example, Aalto reads a line as cents if the line contains a period, also in a comment. linnkit does not export these files. The export screen shows the reason.

In Aalto, select the scale from the scale menu in the KEY module.

You can also export from the command line:

    bin/linnkit export SCL/31-edo.scl

Use `-n` to see the files before linnkit writes them.

## Command-line tools

| Command | Action |
|---|---|
| `linnkit describe [--matrix] FILE...` | Show the contents of scale files |
| `linnkit grid [flags] FILE` | Show the grid for a scale as text |
| `linnkit device ports` | Show the MIDI ports |
| `linnkit device read [NUM...]` | Read LinnStrument settings |
| `linnkit device backup FILE` | Save all settings to a file |
| `linnkit device restore FILE` | Write the settings from a file |
| `linnkit send --slot N [--layout] [--configure] FILE` | Send a scale without the dashboard |
| `linnkit export [-n] FILE...` | Copy scales and `.kbm` files to Madrona Labs |
| `linnkit folders [add\|remove DIR]` | Show, add or remove scale folders |

## Troubleshooting

| Problem | Cause and remedy |
|---|---|
| "the LinnStrument did not answer" | The LinnStrument sleeps. Touch a pad and try again. |
| The lights do not change | The LinnStrument shows a settings screen. Go to the play surface and send again. |
| The lights are old after you load a LinnStrument preset | The LinnStrument does not update custom lights on preset load. Select the light slot again, or send again. |
| Slides do not stop on the next pad | B and S do not agree, or the synth does not use the scale. Check the SYNTH pane and the tuning in the synth. |
| In Aalto, degree 0 is on A4 | The `.kbm` file is missing or empty. Export the scale again with `e`. |
| "linnkit needs at least 160x50" | Make the terminal window larger, or make the font smaller. |
