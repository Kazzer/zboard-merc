# ZEngine 2.5.0.30 feature checklist

What the original did, from its help file, its restored C# sources and its data model, and what
the replacement does with each item. Status: **parity** (same behaviour), **extend** (same plus
more), **drop** (not carried over, with the reason).

## Device handling

| ZEngine | Replacement | Status |
|---|---|---|
| Kernel filter translates stand-in usages through registry tables; user mode uploads the tables per Mod | Translation in the agent from the captured usage table; no kernel code on Linux; on Windows the UMDF driver restores the eleven keys kbdhid drops | parity |
| Driver switches a Merc Stealth to "matrix table 2" at start-up (`02 0B 01`) | Sent when a table-2 Mod activates; `02 0B 00` restores the power-on table without a replug | parity |
| Extra LEDs via feature report 2 bits 0 (Enabled) and 1 (Action) | Same report, both OSes, user mode | parity |
| Lock-LED state selects the layout (Num Lock layer through `LayoutTable`) | Num Lock layer; Caps/Scroll as layer selectors optional | parity |
| Keys "registered as hotkeys" so the driver reports press/release to user mode | Every event reaches the agent anyway | n/a |
| Tray icon green/yellow/red by device state | Tray icon: keyboard body red while the keyboard is missing, plus the two extra LEDs; the menu names the active Mod | parity |

## Mods

| ZEngine | Replacement | Status |
|---|---|---|
| Category > Game > Mod ("Configuration") tree, Ideazon Mods and Custom Mods | Flat list of Mods, each with a name and the programs that trigger it | extend (simpler) |
| Layers per Mod: Primary plus fixed extra layers inheriting the Primary; cannot add or rename | Layers: any number, named, inherit a base layer | extend |
| Per key: Key Text, Key Tooltip, Action String, font, colour | Per key: the action; no labels, fonts or colours (no printed layout image) | drop fonts/colours |
| Action string syntax: key names, `,` sequence, `+` chord, `WATnn` wait (ms, 2 digits), press-only / release-only markers, "Record Delays" | Same ideas, readable YAML: `press`, `release`, `tap`, `wait` with any duration, chords, sequences; a recorder with real timestamps | extend |
| Chat Mode: Chat Command, Chat Text, Send Command | `chat` action: open key, text, send key | parity |
| Key restore / delete to original, cut/copy/paste between keys | Editor operations | parity |
| Import/export `.zmd` | YAML files are the exchange format; no `.zmd` import | drop |
| Printing the Mod image, skins, key images | None | drop |
| Online Mod and device updates (community.zboard.com, dead) | None | drop |

## Triggers and the Enabled LED

| ZEngine | Replacement | Status |
|---|---|---|
| Timer polls the foreground window's process name; a matching `Exe` row activates that Mod, otherwise the out-of-game Mod | The same poll, every 300 ms, of the foreground window's process name (X11 on Linux) | parity |
| Mod with no trigger: always enabled, LED always on | Same | parity |
| Enabled LED on while the active Mod's trigger matches; off on the Windows Default Mod | Same | parity |
| Special case: Scroll Lock forced on for `wow.exe` on some devices | None | drop |
| Action LED "reserved" | Assignable: recording, macro running, Mod indicator, manual | extend |

## Hotkeys and fixed keys

| ZEngine | Replacement | Status |
|---|---|---|
| I/II/III launch an exe, document, folder or URL; hotkey "profiles" (sets) | Same targets per Mod; launched with the agent's privileges | parity |
| Z key opens ZEngine, not programmable | Opens the UI by default; rebindable | extend |
| Seven media keys hard-wired | Pass through; rebindable | extend |
| Numpad shortcuts with Num Lock off (Cut, Copy, Paste, Find, Show Desk, My Computer, Restore, Maximize, Close, Mark Left/Right); digits with Num Lock on | Same defaults, every one rebindable | extend |

## Settings and other

| ZEngine | Replacement | Status |
|---|---|---|
| Language selection, version display | Version display; English only | drop languages |
| `ZE_CLEAN_CONTENT` / `ZE_CLEAN_UNINSTALL` repair tools | Not needed: plain files | n/a |
| Jet/Access database for all content | YAML files | n/a |
