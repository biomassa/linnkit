# linnkit: development plan

Draft 1, 2026-09-11. A terminal app for mapping Scala scales onto the LinnStrument.

## Goal (v1)

1. Pick an `.scl` file from `SCL/` or from extra folders you add.
2. Show what's in the scale, in readable form.
3. Suggest LinnStrument layouts and light schemes that fit the scale, with options you control: colors per pitch class, row offset in steps, and so on.
4. Never write `.scl` files. Write `.kbm` files only when a synth needs one.
5. Send the layout and lights to the LinnStrument, and store them in a light slot and a preset you pick.

It must handle any Scala file: just intonation, EDO, non-EDO, irregular.

Tier 2, after v1: control most LinnStrument settings.

## Decided

| Topic | Decision |
|---|---|
| Stack | Go 1.27 (installed), Bubble Tea + Bubbles + Lip Gloss |
| MIDI | gomidi v2 + `rtmididrv` (RtMidi through C bindings; the Xcode command-line tools are present) |
| Project | `~/code/linnkit`, git, `SCL/` subfolder inside |
| Scale folders | the project `SCL/` and `~/SCL`, each when present; `--scales` overrides both; more folders configurable in M5 |
| Scala parsing | our own parser: lenient, keeps exact ratios, reports problems |
| Presets | app presets (JSON on the Mac, pushed on demand) + guided save into a device memory |
| Reference | the Python scripts in Dropbox/Linnstrument; their outputs become test cases |

## Sources

- `reference/linnstrument-facts.md`: verified firmware, device and synth facts with file:line. The `device` package's parameter table and send and readback code are built from it and from `refs/linnstrument-firmware`.
- `reference/LinnStrumentReference.md`: manual digest, full NRPN list, troubleshooting.
- `reference/EDO-howto.md`, `reference/31EDO/`: bend math, Aalto and Pigments setup.
- `reference/linnstrument_edo*.py`: working message sequences. Their outputs become Go test cases.
- `reference/tui-mockup.py`: the grid style for the TUI.
- These are gitignored, local only.

## Hard constraints (firmware 2.3.4, checked in source)

- Pad note = row start + (col − 1). Columns are always +1 MIDI note. Notes outside 0–127 are clamped, not skipped.
- Row starts can be anything: Guitar mode, NRPN 227 = 13, rows via NRPN 263–270.
- Light slots:
  - 3 custom slots. CC20/21/22 paint pads, CC23 saves.
  - A slot is shown with NRPN 247 = 9 + n.
  - Patterns can't be read back, so the app keeps its own copy.
- Preset memories (6):
  - Can be loaded over MIDI (NRPN 243), but only saved by holding the pad.
  - Loading doesn't refresh the custom lights; the app re-sends NRPN 247 afterwards.
- Settings changed by NRPN reach flash only when `storeSettings()` runs. CC23 triggers it.
- There's no NRPN for the main-channel on/off flag.
- In MPE state the Bend Range is sent to the synth (RPN 0).
- Slides are exact only when all steps are equal (one pad = 1/BendRange of full bend).
- NRPN 299 reads any setting back. `midi.md` has some values wrong; the source is authoritative.

## Scala files seen

- 5,114 unique files in the five12 archive, plus 10 in `SCL/`.
- Sizes: median 12, 99% ≤ 80, max 579. 17 files have more than 128 notes.
- Periods: 4,348 octave, 90 tritave, 676 other (stretched octaves or non-octave periods).
- 202 equal, 605 with exactly two step sizes, 182 not ascending or with a degree ≤ 0.
- `er301/scala/scl` holds 4,802 empty (0-byte) files. Handle cleanly.
- Your `SCL/`:
  - 22edo and 31-edo: equal.
  - Seven JI files: near-equal or irregular, 7–17 notes.
  - ji_9: two step sizes but effectively equal.
  - ji_8coh and ji_9coh: every step a different size.

## Architecture

```
cmd/linnkit/          main
internal/scala/       parse .scl/.kbm (warnings, exact ratios); write .kbm
internal/theory/      cents/ratios, classification (equal, near-equal, MOS, irregular),
                      JI analysis (prime/odd limit, nearest ratios), interval matrix,
                      MOS detection and generators
internal/layout/      grid model (25x8 pads + control column), candidate generators, scoring
internal/lights/      schemes -> 25x8 colors, per-degree palettes
internal/device/      ports, NRPN/RPN/CC encoding, parameter table, readback,
                      backup/restore, send layout/lights, preset load + slot refresh
internal/store/       app config (scale roots), app presets, per-scale settings (JSON)
internal/tui/         screens
testdata/             golden grids, fake-port message logs
```

Rules:
- `scala`, `theory`, `layout` and `lights` have no terminal or MIDI dependencies. They're tested without hardware.
- `device` is tested against a fake port that records messages. Real-device checks follow a written checklist.

## UI decisions (M4, 2026-09-11)

- One full-screen dashboard rather than tabs:
  - Panes: Scales, Scale, Layout, Lights, Grid, Send.
  - Overlays: interval matrix, full degree table, palette editor, send confirmation. Big views scroll or open as overlays, so nothing is cut.
  - Minimum 160×50; smaller windows get an "enlarge" message.
- Keys: arrows to move within a pane, Tab or Left/Right to switch panes, Enter to select. Letters run commands and are always listed in the footer.
- Build a working prototype first, then adjust.

## Screens (v1)

1. **Picker:** scale folders, search by name, description, size and class, with a one-line summary per file.
2. **Scale:** the description (contents: D1).
3. **Layout:** ranked candidates, a grid preview in the style of `tui-mockup.py`, and options: row offset, low note, root MIDI note, and bend range (for unequal steps, choices with their effect; D7).
4. **Lights:** scheme and palette editor, previewed on the grid.
5. **Send:**
   - or send the factory 12-TET layout and note lights instead of the scale (no light slot touched)
   - pick a light slot (0–2)
   - push the layout and settings
   - read back and show differences
   - save as an app preset
   - guided save to a device memory ("hold pad N")
6. **Synth** (a pane under LIGHTS):
   - Pick a synth profile: Aalto/Kaivo, Pigments, Surge XT, Plasmonic, Cypher2, Ableton built-ins, Bitwig built-ins/Grid, legacy non-MPE, Generic.
   - Set the synth's per-note bend range S among the values it allows.
   - The pane shows the LinnStrument Bend Range B = 100·S / target step (the step for equal scales, the average step otherwise), cents per pad, the error, and the largest step error.
   - "Configure MIDI" (on by default) sends B. Legacy synths get One Channel mode.
   - Each profile shows its tuning method and whether its values are verified.
7. **Device:** connection, read-back status.
7. **Export to Madrona Labs plugins:**
   - Copy the selected `.scl` files unchanged into the Madrona Labs Scales folder, in a `linnkit/` subfolder so they don't mix with the synths' own scales.
   - Write a matching `.kbm` next to each one: degree 0 = root MIDI note (D4), every degree listed. Aalto ignores a size-0 map and falls back to A4.
   - Target folder: `~/Music/Madrona Labs/Scales/linnkit/` (D9; briefly the top level, back to the subfolder 2026-09-11). This is the folder Aalto 1.9.5 reads; `~/Library/Audio/Presets/Madrona Labs/Scales` is not used. Same-named files there are replaced, and the overlay lists them before `y`.

## Milestones

| | Scope | Done when |
|---|---|---|
| M0 | Repo, `go.mod`, Makefile, lint, test harness | `make test` passes |
| M1 | `scala` + `theory` | all 5,124 files parse without crashing; a classification report matches expectations; the SCL files are described correctly |
| M2 | `layout` + `lights` + a CLI that prints text grids | the 31-EDO grids match the Python scripts exactly |
| M3 | `device` | fake-port logs match the Python message sequences; send and readback work on the device |
| M4 | TUI: picker → scale → layout → lights → send | full flow works on the device |
| M5 | App presets, guided save, `.kbm` export, polish | presets round-trip; `.kbm` loads in Aalto |

M5 order (2026-09-11):
1. Store. Done:
   - `~/.config/linnkit` holds config (extra scale folders via `linnkit folders`, last synth), per-scale settings (saved on every change), and named presets (`p` overlay).
   - Factory 12-TET send (`f`): rows +5 from F#1, note-light pattern 0, no light slot touched.
2. Madrona export + `.kbm`, root and reference editing (D4, D9). Done:
   - `{ }` move the root; the reference Hz is set in the export overlay (`e`); both are saved per scale.
   - Export writes the loaded scale or every listed scale.
   - Files that madronalib would read differently from linnkit are skipped, with the reason.
   - Not yet checked in Aalto itself.
3. Palette editor: an overlay listing degrees; ↑↓ pick a degree, ←→ cycle the 10 colors, 0 turns it off; starts from the current scheme.
4. Guided save to a device memory.
5. Tuning relay (added 2026-09-11, key `R`). Done:
   - LinnStrument USB in → nearest 12-TET note + pitch bend, one output channel per note → a DIN port.
   - Targets: Kurzweil K2600 (Multi mode, channels 1–16), Mutant Brain (channels 1–4, notes 24–120), generic.
   - Bend range 24 by default; RPN 0 is sent on start.
   - Slides are turned into scale positions, so they land on every degree.
   - Channels are allocated by quietest channel first; the oldest note is stolen when all are busy.
   - The relay reads the LinnStrument Bend Range on start. The synth profile "linnkit relay" sets it to 24.
   - Not yet run with the real K2600 or Mutant Brain.
| Tier 2 | Most LinnStrument settings | tbd |

## Testing

- Unit tests for the math (cents, ratios, limits, MOS detection).
- Golden files for grids and light patterns.
- Corpus test: parse every `.scl` on disk and report warnings.
- Device: fake-port message logs; a manual checklist on the real device (readback after every send).

## Decisions (2026-09-11)

- **D1 Scale description:** all four views.
  - Degree table: cents, ratio, step, nearest 12-TET ± cents.
  - Structure summary: size, period, step sizes, class (equal, near-equal, MOS with its LLsLs pattern, irregular).
  - JI analysis: prime and odd limit; nearest simple ratios for cents scales.
  - Interval matrix.
- **D2 Layout generators in v1:**
  - uniform row offset aimed at target intervals
  - generator-based for MOS scales
  - no overlap

  Per-row manual rows come later.
- **D3 Light schemes in v1:**
  - pitch-class palette (a color per degree, saved per scale)
  - just-interval families
  - MOS step pattern
  - root and period only
- **D4 Root mapping:** degree 0 = MIDI 60 = 261.63 Hz by default. The root note and reference frequency can be edited per scale. The app writes a matching `.kbm` when a synth needs one.
- **D5 App data:** `~/.config/linnkit` (config, app presets, per-scale settings).
- **D6 Edge cases:**
  - Non-octave periods: supported; layouts and lights work per period.
  - Non-ascending scales: keep the file's order and show a warning.
  - More than 128 notes: described only; layouts limited to what fits in MIDI 0–127.
- **D7 Bend range for unequal steps:** your choice. The app shows the options (average step, smallest step, the Quantize and Quant Hold settings) and their effect; it doesn't pick one.
- **D8 `SCL/`:** committed; the files are test fixtures.
- **D9 Madrona Labs export:** `~/Music/Madrona Labs/Scales/linnkit/`, with a `.kbm` per scale listing every degree.
- **Note-name lights:** added as a user-selectable scheme (`names`), a port of `linnstrument_edo.py`.
