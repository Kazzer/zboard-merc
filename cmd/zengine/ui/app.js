'use strict';

const $ = (sel, root = document) => root.querySelector(sel);

function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (k === 'class') node.className = v;
    else if (k === 'text') node.textContent = v;
    else if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
    else if (v !== false && v != null) node.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat()) {
    if (c == null) continue;
    node.append(c.nodeType ? c : document.createTextNode(String(c)));
  }
  return node;
}

async function api(method, path, body, form) {
  const headers = { 'X-Requested-With': 'zboard' };
  let payload;
  if (body !== undefined) {
    headers['Content-Type'] = form ? 'application/x-www-form-urlencoded' : 'application/json';
    payload = form ? new URLSearchParams(body).toString() : JSON.stringify(body);
  }
  const r = await fetch(path, { method, headers, body: payload, credentials: 'same-origin' });
  const text = await r.text();
  if (!r.ok) throw new Error(text.trim() || r.statusText);
  return text ? JSON.parse(text) : null;
}

const state = { status: null, keys: { sources: [], keys: [] }, mods: [], current: null, view: null };

function notice(text, error) {
  const n = $('#notice');
  n.textContent = text;
  n.className = error ? 'error' : '';
  if (!error && text) setTimeout(() => { if (n.textContent === text) n.textContent = ''; }, 4000);
}
const fail = (err) => notice(err.message || String(err), true);


function setStatus(st) { state.status = st; renderStatus(); }

function renderStatus() {
  const st = state.status;
  if (!st) return;
  $('#led-enabled').classList.toggle('on', !!(st.leds && st.leds.enabled));
  $('#led-action').classList.toggle('on', !!(st.leds && st.leds.action));
  const pin = $('#pin');
  const opts = ['', ...st.mods];
  if (pin.dataset.opts !== opts.join('|')) {
    pin.replaceChildren(...opts.map(m => el('option', { value: m, text: m || 'automatic' })));
    pin.dataset.opts = opts.join('|');
  }
  pin.value = st.pinned || '';
  $('#process').textContent = (st.attached === false ? 'keyboard not found · ' : '') + (st.active ? 'active: ' + st.active : 'bare keyboard') + (st.process ? ' · ' + st.process : '');
  $('#process').classList.toggle('error', st.attached === false);
  const names = Object.keys(st.flags || {}).sort();
  $('#flags').replaceChildren(...names.map(name => el('label', {},
    el('input', { type: 'checkbox', checked: st.flags[name], onchange: e => api('POST', '/flag', { name, on: e.target.checked ? '1' : '0' }, true).then(setStatus).catch(fail) }),
    name)));
  $('#flagnames').replaceChildren(...names.map(n => el('option', { value: n })));
  $('#mods').replaceChildren(...state.mods.map(m => el('li', {},
    el('a', { href: '#mod/' + encodeURIComponent(m.name), class: state.view === 'mod' && state.current && state.current.name === m.name ? 'active' : '' },
      m.name, ' ', el('span', { class: 'muted', text: m.name === st.active ? '● active' : m.triggers.join(', ') })))));
  $('#settings-link').className = state.view === 'settings' ? 'active' : '';
}

async function refreshStatus() { setStatus(await api('GET', '/status')); }
async function refreshMods() { state.mods = await api('GET', '/mods'); renderStatus(); }

async function openRoute() {
  const h = decodeURIComponent(location.hash.slice(1));
  if (h === 'settings') {
    state.view = 'settings';
    state.current = null;
    await renderSettings().catch(fail);
  } else if (h.startsWith('mod/')) {
    state.view = 'mod';
    await openMod(h.slice(4));
  } else {
    state.view = null;
    state.current = null;
    $('#main').replaceChildren(el('p', { class: 'muted', text: 'Pick a Mod on the left, or Settings.' }));
  }
  renderStatus();
}


const KINDS = ['tap', 'chord', 'macro', 'chat', 'launch', 'layer', 'mod', 'ui', 'disable', 'toggle'];
const KIND_LABELS = { tap: 'Key', chord: 'Chord', macro: 'Macro', chat: 'Chat line', launch: 'Launch', layer: 'Layer while held', mod: 'Switch Mod', ui: 'Open this UI', disable: 'Disable', toggle: 'Toggle flag only' };

const stepFrom = (s) => { const k = (s && Object.keys(s)[0]) || 'tap'; return { kind: k, value: s ? String(s[k]) : '' }; };

const recording = { row: null, count: 0, timer: null };

async function toggleRecord(row, redraw) {
  try {
    if (recording.row) {
      const r = await api('POST', '/record/stop', {});
      const target = recording.row;
      clearInterval(recording.timer);
      recording.row = null;
      target.steps = (r.steps || []).map(stepFrom);
      notice(target.steps.length + ' step(s) recorded');
    } else {
      await api('POST', '/record/start', {});
      recording.row = row;
      recording.count = 0;
      recording.timer = setInterval(() => api('GET', '/record').then(r => {
        if (!r.recording) { clearInterval(recording.timer); recording.row = null; redraw(); return; }
        if (r.steps.length !== recording.count) { recording.count = r.steps.length; redraw(); }
      }).catch(() => {}), 500);
    }
    redraw();
  } catch (e) { fail(e); }
}

function fromAction(a) {
  const b = { kind: 'tap', target: '', chord: [], steps: [], mode: 'once', hold: '', chat: { open: '', text: '', send: '' }, launch: '', layer: '', mod: '', toggle: '' };
  if (a == null) return b;
  if (typeof a !== 'object') {
    const s = String(a);
    if (s.toLowerCase() === 'disable') b.kind = 'disable';
    else if (s.toLowerCase() === 'ui') b.kind = 'ui';
    else b.target = s;
    return b;
  }
  b.toggle = a.toggle == null ? '' : String(a.toggle);
  if (a.tap != null) { b.kind = 'tap'; b.target = String(a.tap); }
  else if (Array.isArray(a.chord)) { b.kind = 'chord'; b.chord = a.chord.map(String); }
  else if (Array.isArray(a.macro)) {
    b.kind = 'macro';
    b.steps = a.macro.map(stepFrom);
    b.mode = a.mode ? String(a.mode) : 'once';
    b.hold = a.hold !== undefined ? String(a.hold) : '';
  }
  else if (a.chat) { b.kind = 'chat'; b.chat = { open: String(a.chat.open || ''), text: String(a.chat.text || ''), send: String(a.chat.send || '') }; }
  else if (a.launch != null) { b.kind = 'launch'; b.launch = String(a.launch); }
  else if (a.layer != null) { b.kind = 'layer'; b.layer = String(a.layer); }
  else if (a.mod != null) { b.kind = 'mod'; b.mod = String(a.mod); }
  else if (b.toggle) b.kind = 'toggle';
  return b;
}

function toAction(b) {
  const withToggle = (o) => { if (b.toggle) o.toggle = b.toggle; return o; };
  switch (b.kind) {
    case 'disable': return 'disable';
    case 'ui': return 'ui';
    case 'toggle': return { toggle: b.toggle };
    case 'tap': return b.toggle ? { tap: b.target, toggle: b.toggle } : b.target;
    case 'chord': return withToggle({ chord: b.chord.filter(Boolean) });
    case 'macro': {
      const o = { macro: b.steps.map(s => ({ [s.kind]: s.value })) };
      if (b.mode && b.mode !== 'once') o.mode = b.mode;
      if (b.hold) o.hold = b.hold;
      return withToggle(o);
    }
    case 'chat': return withToggle({ chat: { open: b.chat.open, text: b.chat.text, send: b.chat.send } });
    case 'launch': return withToggle({ launch: b.launch });
    case 'layer': return withToggle({ layer: b.layer });
    case 'mod': return withToggle({ mod: b.mod });
  }
  return b.target;
}

const parseLED = (v) => { v = (v || 'mod').toLowerCase(); return v.startsWith('flag:') ? { kind: 'flag', flag: v.slice(5) } : { kind: v, flag: '' }; };

function docToForm(d) {
  const rows = (keys) => (keys || []).map(k => Object.assign(fromAction(k.action), { source: k.source }));
  return {
    name: d.name || '', table: d.table || 'legend', triggers: (d.triggers || []).join(', '), menu: d.menu,
    leds: d.leds ? { enabled: d.leds.enabled || 'mod', action: d.leds.action || 'mod' } : null,
    keys: rows(d.keys), layers: (d.layers || []).map(l => ({ name: l.name, keys: rows(l.keys) })),
  };
}

function formToDoc(f) {
  const keys = (rows) => rows.map(r => ({ source: r.source, action: toAction(r) }));
  return {
    name: f.name.trim(), table: f.table,
    triggers: f.triggers.split(',').map(s => s.trim()).filter(Boolean),
    menu: f.menu == null ? undefined : f.menu,
    leds: f.leds ? { enabled: f.leds.enabled, action: f.leds.action } : null,
    keys: keys(f.keys), layers: f.layers.map(l => ({ name: l.name.trim(), keys: keys(l.keys) })),
  };
}


async function openMod(name) {
  try {
    const m = await api('GET', '/mods/' + encodeURIComponent(name));
    state.current = { name, file: m.file, yaml: m.yaml, doc: docToForm(m.document), tab: 'form' };
    renderEditor();
  } catch (e) { fail(e); }
}

function ledSource(value, onchange) {
  const v = parseLED(value);
  const flag = el('input', { list: 'flagnames', value: v.flag, placeholder: 'flag name', style: v.kind === 'flag' ? '' : 'display:none', onchange: () => onchange(get()) });
  const sel = el('select', { onchange: () => { flag.style.display = sel.value === 'flag' ? '' : 'none'; onchange(get()); } },
    ...['mod', 'on', 'off', 'flag'].map(k => el('option', { value: k, selected: v.kind === k, text: k === 'mod' ? 'mod (ZEngine meaning)' : k === 'flag' ? 'flag…' : k })));
  const get = () => sel.value === 'flag' ? 'flag:' + flag.value.trim() : sel.value;
  return { node: el('span', { class: 'params' }, sel, flag), get };
}

function keyInput(value, onchange, extra) {
  return el('input', Object.assign({ list: 'keynames', value, placeholder: 'key name', onchange: e => onchange(e.target.value.trim()) }, extra || {}));
}

function paramsFor(row, redraw) {
  switch (row.kind) {
    case 'tap': return [keyInput(row.target, v => { row.target = v; })];
    case 'chord': return [el('input', { class: 'wide', value: row.chord.join(', '), placeholder: 'left-ctrl, s',
      onchange: e => { row.chord = e.target.value.split(/[,\s]+/).map(s => s.trim()).filter(Boolean); } })];
    case 'macro': {
      let dragFrom = -1;
      const steps = el('div', { class: 'steps' }, ...row.steps.map((s, i) => el('div', { class: 'step',
        ondragover: e => { e.preventDefault(); e.currentTarget.classList.add('over'); },
        ondragleave: e => e.currentTarget.classList.remove('over'),
        ondrop: e => { e.preventDefault(); if (dragFrom < 0 || dragFrom === i) return; const [moved] = row.steps.splice(dragFrom, 1); row.steps.splice(i, 0, moved); dragFrom = -1; redraw(); } },
        el('span', { class: 'handle', draggable: true, title: 'drag to reorder', ondragstart: e => { dragFrom = i; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(i)); } }, '⋮⋮'),
        el('select', { onchange: e => { s.kind = e.target.value; if (s.kind === 'wait' && !/\d/.test(s.value)) s.value = '100ms'; redraw(); } },
          ...['tap', 'press', 'release', 'wait'].map(k => el('option', { value: k, selected: s.kind === k, text: k }))),
        s.kind === 'wait'
          ? el('input', { value: s.value, placeholder: '200ms', onchange: e => { s.value = e.target.value.trim(); } })
          : keyInput(s.value, v => { s.value = v; }),
        el('button', { type: 'button', class: 'small', title: 'move up', disabled: i === 0, onclick: () => { row.steps.splice(i - 1, 2, row.steps[i], row.steps[i - 1]); redraw(); } }, '↑'),
        el('button', { type: 'button', class: 'small', title: 'move down', disabled: i === row.steps.length - 1, onclick: () => { row.steps.splice(i, 2, row.steps[i + 1], row.steps[i]); redraw(); } }, '↓'),
        el('button', { type: 'button', class: 'small danger', onclick: () => { row.steps.splice(i, 1); redraw(); } }, '✕'))));
      const recLabel = recording.row === row ? 'Stop recording (' + recording.count + ' so far)' : 'Record…';
      const add = el('div', { class: 'row' },
        el('button', { type: 'button', class: 'small', onclick: () => { row.steps.push({ kind: 'tap', value: '' }); redraw(); } }, '+ key'),
        el('button', { type: 'button', class: 'small', onclick: () => { row.steps.push({ kind: 'wait', value: '100ms' }); redraw(); } }, '+ wait'),
        el('button', { type: 'button', class: recording.row === row ? 'small primary' : 'small', title: 'replace the steps with what you type on the keyboard until you stop; the Action LED blinks meanwhile', onclick: () => toggleRecord(row, redraw) }, recLabel),
        el('label', {}, 'mode ', el('select', { onchange: e => { row.mode = e.target.value; } },
          ...[['once', 'once per press'], ['repeat', 'repeat while held'], ['toggle', 'toggle on and off']].map(([k, t]) => el('option', { value: k, selected: row.mode === k, text: t })))),
        el('label', {}, 'hold ', el('input', { class: 'short', value: row.hold, placeholder: '40ms', title: 'how long each tap stays pressed (a game polling once per frame misses shorter ones); 0 for none', onchange: e => { row.hold = e.target.value.trim(); } })));
      return [el('div', {}, steps, add)];
    }
    case 'chat': return [
      el('label', {}, 'open ', keyInput(row.chat.open, v => { row.chat.open = v; })),
      el('label', {}, 'text ', el('input', { class: 'wide', value: row.chat.text, onchange: e => { row.chat.text = e.target.value; } })),
      el('label', {}, 'send ', keyInput(row.chat.send, v => { row.chat.send = v; }))];
    case 'launch': return [el('input', { class: 'wide', value: row.launch, placeholder: 'program, document, folder or URL', onchange: e => { row.launch = e.target.value.trim(); } })];
    case 'layer': return [el('input', { value: row.layer, placeholder: 'layer name', onchange: e => { row.layer = e.target.value.trim(); } })];
    case 'mod': return [el('input', { value: row.mod, placeholder: 'next, previous or a Mod name', onchange: e => { row.mod = e.target.value.trim(); } })];
    case 'toggle': return [el('input', { list: 'flagnames', value: row.toggle, placeholder: 'flag name', onchange: e => { row.toggle = e.target.value.trim(); } })];
  }
  return [];
}

const liveRows = new WeakMap();

function bindingRow(row, list, rerender) {
  const tr = el('tr');
  liveRows.set(row, tr);
  const redraw = () => (liveRows.get(row) || tr).replaceWith(bindingRow(row, list, rerender));
  const extraToggle = row.kind !== 'disable' && row.kind !== 'ui' && row.kind !== 'toggle';
  tr.append(
    el('td', {}, el('input', { class: 'source', list: 'sources', value: row.source, placeholder: 'pad.1', onchange: e => { row.source = e.target.value.trim(); } })),
    el('td', {}, el('select', { onchange: e => { row.kind = e.target.value; redraw(); } },
      ...KINDS.map(k => el('option', { value: k, selected: row.kind === k, text: KIND_LABELS[k] })))),
    el('td', {}, el('div', { class: 'params' }, ...paramsFor(row, redraw))),
    el('td', {}, extraToggle ? el('input', { list: 'flagnames', value: row.toggle, placeholder: 'also toggle…', title: 'a flag flipped on every press, as well as the action', onchange: e => { row.toggle = e.target.value.trim(); } }) : ''),
    el('td', {}, el('button', { type: 'button', class: 'small danger', title: 'remove', onclick: () => { list.splice(list.indexOf(row), 1); rerender(); } }, '✕')));
  return tr;
}

function bindingTable(list, rerender) {
  const table = el('table', { class: 'bindings' },
    el('thead', {}, el('tr', {}, el('th', { text: 'Key' }), el('th', { text: 'Does' }), el('th', { text: '' }), el('th', { text: 'Flag' }), el('th', { text: '' }))),
    el('tbody', {}, ...list.map(r => bindingRow(r, list, rerender))));
  const add = el('button', { type: 'button', class: 'small', onclick: () => { list.push(Object.assign(fromAction(''), { source: '' })); rerender(); } }, '+ binding');
  return el('div', {}, table, el('p', {}, add));
}

function renderEditor() {
  const cur = state.current;
  const f = cur.doc;
  const main = $('#main');
  const tabs = el('div', { class: 'tabs' },
    el('button', { type: 'button', class: cur.tab === 'form' ? 'active' : '', onclick: () => { cur.tab = 'form'; renderEditor(); } }, 'Form'),
    el('button', { type: 'button', class: cur.tab === 'yaml' ? 'active' : '', onclick: () => { cur.tab = 'yaml'; renderEditor(); } }, 'YAML'),
    el('span', { class: 'muted', text: cur.file }));
  if (cur.tab === 'yaml') {
    const ta = el('textarea', { spellcheck: 'false' }, cur.yaml);
    main.replaceChildren(el('h2', { text: cur.name }), tabs, ta,
      el('p', { class: 'hint', text: 'The file as it is, comments included. Saving here keeps them; saving the form rewrites the file without comments.' }),
      el('div', { class: 'actions' }, el('button', { type: 'button', class: 'primary', onclick: () => saveMod({ yaml: ta.value }) }, 'Save YAML')));
    return;
  }
  const rerender = () => renderEditor();
  const ledBox = el('div', { class: 'row' });
  const ledsOn = el('input', { type: 'checkbox', checked: !!f.leds, onchange: e => { f.leds = e.target.checked ? { enabled: 'mod', action: 'mod' } : null; rerender(); } });
  ledBox.append(el('label', {}, ledsOn, ' this Mod sets the two LEDs itself while active'));
  if (f.leds) {
    ledBox.append(el('label', {}, 'Enabled ', ledSource(f.leds.enabled, v => { f.leds.enabled = v; }).node),
      el('label', {}, 'Action ', ledSource(f.leds.action, v => { f.leds.action = v; }).node));
  }
  const driver = state.status ? state.status.driver : '';
  const standin = 'standin (pad keys distinct' + (driver === 'absent' ? '; requires driver, not running' : driver === 'connected' ? '; requires driver, running' : '') + ')';
  const head = el('fieldset', {}, el('legend', { text: 'Mod' }),
    el('div', { class: 'row' },
      el('label', {}, 'Name ', el('input', { value: f.name, title: 'the file name without .yaml; changing it renames the file', onchange: e => { f.name = e.target.value; } })),
      el('label', {}, 'Table ', el('select', { onchange: e => { f.table = e.target.value; } },
        el('option', { value: 'legend', selected: f.table !== 'standin', text: 'legend (power-on table)' }),
        el('option', { value: 'standin', selected: f.table === 'standin', text: standin }))),
      el('label', {}, 'Triggers ', el('input', { class: 'wide', value: f.triggers, placeholder: 'game.exe, other.exe', title: 'executables whose window in front activates this Mod; empty = never by itself', onchange: e => { f.triggers = e.target.value; } }))),
    el('div', { class: 'row' }, el('label', {},
      el('input', { type: 'checkbox', checked: f.menu == null ? !f.triggers.trim() : f.menu, onchange: e => { f.menu = e.target.checked; } }),
      ' in the tray menu'), el('span', { class: 'muted', text: 'unless set, Mods with triggers stay out of the menu' })),
    ledBox);
  const keys = el('fieldset', {}, el('legend', { text: 'Keys' }), bindingTable(f.keys, rerender));
  const layers = el('fieldset', {}, el('legend', { text: 'Layers' }),
    el('p', { class: 'hint', text: 'num-lock, caps-lock and scroll-lock follow the lock keys; any other name is held through a "Layer while held" binding.' }),
    ...f.layers.map(l => el('div', {},
      el('div', { class: 'layer-head' }, el('label', {}, 'Layer ', el('input', { value: l.name, onchange: e => { l.name = e.target.value; } })),
        el('button', { type: 'button', class: 'small danger', onclick: () => { f.layers.splice(f.layers.indexOf(l), 1); rerender(); } }, 'remove layer')),
      bindingTable(l.keys, rerender))),
    el('button', { type: 'button', class: 'small', onclick: () => { f.layers.push({ name: 'layer-' + (f.layers.length + 1), keys: [] }); rerender(); } }, '+ layer'));
  const actions = el('div', { class: 'actions' },
    el('button', { type: 'button', class: 'primary', onclick: () => saveMod({ document: formToDoc(f) }) }, 'Save'),
    el('button', { type: 'button', onclick: () => api('POST', '/mod', { name: cur.name }, true).then(setStatus).catch(fail) }, 'Activate now'),
    el('button', { type: 'button', onclick: duplicateMod }, 'Duplicate…'),
    el('button', { type: 'button', class: 'danger', onclick: deleteMod }, 'Delete Mod'));
  main.replaceChildren(el('h2', { text: cur.name }), tabs, head, keys, layers, actions);
}

async function saveMod(body) {
  const cur = state.current;
  try {
    const m = await api('PUT', '/mods/' + encodeURIComponent(cur.name), Object.assign({ name: cur.doc.name }, body));
    const renamed = m.document.name !== cur.name;
    Object.assign(cur, { name: m.document.name, file: m.file, yaml: m.yaml, doc: docToForm(m.document) });
    notice('Saved ' + m.file);
    await refreshMods();
    await refreshStatus();
    if (renamed) location.hash = '#mod/' + encodeURIComponent(cur.name); else renderEditor();
  } catch (e) { fail(e); }
}

async function deleteMod() {
  const cur = state.current;
  if (!confirm('Delete the Mod "' + cur.name + '" and its file ' + cur.file + '?')) return;
  try {
    await api('DELETE', '/mods/' + encodeURIComponent(cur.name));
    notice('Deleted ' + cur.file);
    await refreshMods();
    await refreshStatus();
    location.hash = '';
  } catch (e) { fail(e); }
}

async function duplicateMod() {
  const cur = state.current;
  const name = prompt('Name of the copy (it becomes the file name)', cur.name + '-copy');
  if (!name || !name.trim()) return;
  try {
    const m = await api('POST', '/mods', { name: name.trim(), yaml: cur.yaml });
    await refreshMods();
    location.hash = '#mod/' + encodeURIComponent(m.document.name);
  } catch (e) { fail(e); }
}

async function newMod() {
  const name = prompt('Name of the new Mod (it becomes the file name: letters, digits, . _ -)');
  if (!name || !name.trim()) return;
  try {
    const m = await api('POST', '/mods', { name: name.trim(), document: { table: 'legend', triggers: [], keys: [], layers: [] } });
    await refreshMods();
    location.hash = '#mod/' + encodeURIComponent(m.document.name);
  } catch (e) { fail(e); }
}


async function renderSettings() {
  const cfg = await api('GET', '/config');
  const def = el('select', {}, el('option', { value: '', text: '(bare keyboard)' }),
    ...state.mods.map(m => el('option', { value: m.name, selected: m.name === cfg.default, text: m.name })));
  const device = el('input', { value: cfg.device || '', placeholder: 'detect' });
  const control = el('input', { value: cfg.control || '127.0.0.1:47319' });
  const tapHold = el('input', { class: 'short', value: cfg['tap-hold'] || '', placeholder: '40ms', title: 'how long a macro tap stays pressed unless the macro or the step says; a game polling once per frame misses shorter ones' });
  const en = ledSource(cfg.leds.enabled, () => {});
  const ac = ledSource(cfg.leds.action, () => {});
  const save = el('button', { type: 'button', class: 'primary', onclick: async () => {
    try {
      await api('PUT', '/config', { default: def.value, device: device.value.trim(), control: control.value.trim(), 'tap-hold': tapHold.value.trim(), leds: { enabled: en.get(), action: ac.get() } });
      notice('Saved config.yaml');
      await refreshStatus();
    } catch (e) { fail(e); }
  } }, 'Save');
  $('#main').replaceChildren(el('h2', { text: 'Settings' }),
    el('fieldset', {}, el('legend', { text: 'Agent' }),
      el('div', { class: 'row' }, el('label', {}, 'Default Mod ', def), el('span', { class: 'muted', text: 'when no trigger matches' })),
      el('div', { class: 'row' }, el('label', {}, 'Device ', device), el('span', { class: 'muted', text: 'USB product id: 0510 Merc Stealth, 0210 Merc, empty detects' })),
      el('div', { class: 'row' }, el('label', {}, 'Control address ', control), el('span', { class: 'muted', text: 'applies at the next start' })),
      el('div', { class: 'row' }, el('label', {}, 'Tap hold ', tapHold), el('span', { class: 'muted', text: 'how long a macro tap stays pressed; 40ms unless set, 0 for none' }))),
    el('fieldset', {}, el('legend', { text: 'Extra LEDs, unless the active Mod says otherwise' }),
      el('div', { class: 'row' }, el('label', {}, 'Enabled ', en.node)),
      el('div', { class: 'row' }, el('label', {}, 'Action ', ac.node)),
      el('p', { class: 'hint', text: 'mod: Enabled = a Mod is active, Action = a macro is running. flag: a named flag that toggle bindings flip.' })),
    el('p', { class: 'muted', text: 'Files: ' + cfg.dir + '. zengine ' + (state.status ? state.status.version : '') }),
    el('div', { class: 'actions' }, save));
}


document.body.append(el('datalist', { id: 'flagnames' }));
$('#pin').addEventListener('change', e => api('POST', '/mod', { name: e.target.value }, true).then(setStatus).catch(fail));
$('#reload').addEventListener('click', () => api('POST', '/reload', {}, true).then(st => { setStatus(st); return refreshMods(); }).then(() => notice('Files reloaded')).catch(fail));
$('#new-mod').addEventListener('click', newMod);
window.addEventListener('hashchange', openRoute);

(async () => {
  try {
    state.keys = await api('GET', '/keys');
    $('#sources').replaceChildren(...state.keys.sources.map(s => el('option', { value: s })));
    $('#keynames').replaceChildren(...state.keys.keys.map(s => el('option', { value: s })));
    await refreshMods();
    await refreshStatus();
    await openRoute();
  } catch (e) { fail(e); }
  setInterval(() => refreshStatus().catch(() => notice('zengine is not answering', true)), 2000);
})();
