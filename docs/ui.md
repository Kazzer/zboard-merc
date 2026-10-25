# The web UI

The agent serves a small web UI on its control address (`127.0.0.1:47319` unless `control:` in `config.yaml`
says otherwise). Open it with

```
zboard ui
```

which prints the link and hands it to the default browser. The link carries the control
token, which the page turns into a cookie; the token changes at every start of the agent, so
after a restart open it with `zboard ui` again. Nothing but the loopback address is served,
and nothing can be changed from another site: the API refuses foreign Host and Origin headers,
the cookie is HttpOnly and SameSite=Strict, and every change needs a header no cross-site
form can send.

## What it does

- **Status bar:** the two extra LEDs as the agent drives them, the active Mod with a
  selector to pin one (a matching trigger still wins; "automatic" unpins), the foreground
  process, every flag the configuration or a Mod names as a checkbox, and "Reload files", which re-reads `config.yaml` and the
  Mods the way `zboard reload` does.
- **Mods:** one entry per file in `mods/`, with its triggers. A Mod's name is its file name
  without `.yaml` (lower case; letters, digits, `.`, `_` and `-`); "New Mod" asks for it, and
  changing the name in the form renames the file. A `name:` key in a file is ignored.
  "Duplicate…" in the editor copies the file, comments included, under a new name.
- **Editor, Form tab:** name, table (the stand-in choice says whether `driver/umdf` is running,
  on Windows), triggers, the Mod's own LED meaning, the base layer's
  bindings and each layer's. A binding is a source key (the pad legends as `pad.<legend>`,
  the numpad as `num.<legend>`, `hotkey.1` to `hotkey.3`, `backlight.intensity`,
  `backlight.colour`, or any key name; `docs/keys.md` lists them all), what it does (a key, a chord, a macro with its
  steps, mode and hold, a chat line, a launch, a layer while held, a Mod switch, disable, or only a
  flag toggle), and optionally a flag the press also toggles. Save validates the whole Mod
  through the same parser the agent uses and writes nothing when it fails; the error names
  the binding. Saving the form rewrites the file in the file's own key order, in block style with
  one item per line, so lines can be moved or copied between files, without comments.
- **Editor, YAML tab:** the file as it is, comments included. Saving here keeps them.
- **Settings:** the default Mod, the device, the control address (applies at the next
  start), the tap hold (how long a macro's tap stays pressed unless the macro or the step
  says, `hold:` beside `macro:` or beside `tap:`) and the two LED defaults. Saving rewrites `config.yaml` without its comments.

Everything the UI writes, the agent applies at once; `zboard` and a text editor remain
equally valid ways to change the same files.

Macro steps reorder with the arrows beside each step or by dragging a step's handle onto
the step whose place it should take.

## Recording a macro

In a macro binding, "Record…" starts the recorder: the Action LED blinks, and every key you
press on the keyboard, in whatever window has the focus, becomes a step. A press followed by
its release becomes one tap, a press with other keys in between stays a press and a release,
and the time between keys becomes a wait, to the millisecond. Keys keep working while it
records, so give the focus to a harmless window (a text editor) first, then come back and
press "Stop recording". The steps replace the binding's steps; edit them and save. The same
runs over the API: `POST /record/start`, `GET /record`, `POST /record/stop`.

## The tray icon and the Z key

With a desktop session the agent also shows a tray icon (`tray: auto|on|off` in
`config.yaml`): a keyboard with the two LEDs as the agent drives them. Its menu pins a Mod or
returns to automatic, flips the flags, opens this UI, reloads the files and quits the agent.
Mods with triggers stay out of the menu, since they come and go by themselves, unless the
Mod says `menu: true`; a Mod without triggers is listed unless it says `menu: false`. The
form has the checkbox.
On Linux the icon needs a tray host (KDE has one; GNOME needs the AppIndicator extension).

A binding of `ui` opens this UI; the example Mods put it on the Z key, which opened
ZEngine. The form offers it as "Open this UI".

## API

The UI uses the control API, which the CLI uses too. With the token as `Authorization:
Bearer <token>`: `GET /status`, `POST /mod?name=`, `POST /flag?name=&on=`, `POST /reload`,
`GET /keys`, `GET /config`, `PUT /config` (JSON), `GET /mods`, `POST /mods` (JSON `{document}`
or `{yaml}`), `GET /mods/{name}`, `PUT /mods/{name}`, `DELETE /mods/{name}`, `GET /record`,
`POST /record/start`, `POST /record/stop`.
