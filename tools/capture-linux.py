#!/usr/bin/env python3
"""Capture everything an Ideazon keyboard (Zboard, MERC, Fang, MERC Stealth; USB 1038:xxxx) exposes on Linux.

The LED trials and the table walk write feature report 2 to the keyboard.
Pressed keys are swallowed for the duration of each prompt so they are not received by the system.
"""
import argparse
import collections.abc
import fcntl
import glob
import json
import os
import pathlib
import select
import struct
import sys
import tarfile
import threading
import time
import typing

CHORDS: list[tuple[str, str]] = [
    ('chord-wa', 'hold pad W, then press pad A, then release both'),
    ('chord-sd', 'hold pad S, then press pad D, then release both'),
    ('chord-5', 'hold pad Shift, pad Ctrl, pad Space and pad W, then press pad 1, then release all'),
    ('chord-7', 'press and hold seven pad keys at once (e.g. 1 2 3 4 5 6 7), then release all'),
    ('chord-cross', 'hold main-keyboard A, then press pad W, then release both'),
    ('hold-w', 'hold pad W for three seconds, then release'),
]
EV_KEY: int = 1
EV_LED: int = 0x11
EV_MSC: int = 4
EV_SYN: int = 0
FIRST_EVENT_TIMEOUT_S: float = 20.0
IDLE_S: float = 1.0
INPUT_EVENT: struct.Struct = struct.Struct('llHHi')
IOC_READ: int = 2
IOC_WRITE: int = 1
LED_NAMES: dict[int, str] = {
    0: 'numlock',
    1: 'capslock',
    2: 'scrolllock',
    3: 'compose',
    4: 'kana',
}
MAIN_REF: list[str] = [
    'A',
    'Enter',
    'Left Shift',
    'Left Ctrl',
    'Space',
    'F1',
    'Esc',
    'Up arrow',
    'Caps Lock',
    'Left Windows key',
]
MODELS: dict[int, str] = {
    0x0100: 'Zboard',
    0x0210: 'Merc',
    0x0310: 'Fang',
    0x0510: 'Merc Stealth',
}
NUMPAD: list[str] = [
    'Scroll Lock',
    'Pause',
    '/',
    '7',
    '8',
    '9',
    '*',
    '4',
    '5',
    '6',
    '-',
    '1',
    '2',
    '3',
    '+',
    '0',
    '.',
    'Enter',
]
PAD_KEYS: int = 34
SETTLE_S: float = 0.6
SETTLE_TIMEOUT_S: float = 15.0
TOP_ROW: list[str] = [
    'Z key (opens Z Engine)',
    'Volume up',
    'Volume down',
    'Mute',
    'Previous track',
    'Play/Pause',
    'Stop',
    'Next track',
    'Hotkey I',
    'Hotkey II',
    'Hotkey III',
    'backlight colour key',
    'backlight intensity key',
]
VID: int = 0x1038

ioc: collections.abc.Callable[[int, str, int, int], int] = (
    lambda direction, typ, nr, size: (direction << 30) | (size << 16) | (ord(typ) << 8) | nr
)
EVIOCGID: int = ioc(IOC_READ, 'E', 0x02, 8)
EVIOCGRAB: int = ioc(IOC_WRITE, 'E', 0x90, 4)
HIDIOCGRAWINFO: int = ioc(IOC_READ, 'H', 0x03, 8)
eviocgkey: collections.abc.Callable[[int], int] = lambda n: ioc(IOC_READ, 'E', 0x18, n)
eviocgled: collections.abc.Callable[[int], int] = lambda n: ioc(IOC_READ, 'E', 0x19, n)
eviocgname: collections.abc.Callable[[int], int] = lambda n: ioc(IOC_READ, 'E', 0x06, n)
hidiocgfeature: collections.abc.Callable[[int], int] = lambda n: ioc(IOC_WRITE | IOC_READ, 'H', 0x07, n)
hidiocginput: collections.abc.Callable[[int], int] = lambda n: ioc(IOC_WRITE | IOC_READ, 'H', 0x0A, n)
hidiocsfeature: collections.abc.Callable[[int], int] = lambda n: ioc(IOC_WRITE | IOC_READ, 'H', 0x06, n)

Input: typing.TypedDict = typing.TypedDict(
    'Input',
    {
        'capabilities': dict[str, str],
        'name': str,
        'phys': str,
    },
)

Entry: typing.TypedDict = typing.TypedDict(
    'Entry',
    {
        'descriptor_hex': str,
        'driver': str,
        'events': list[str],
        'hidraw': list[str],
        'iface': str,
        'inputs': dict[str, Input],
        'led_brightness': dict[str, str],
        'leds': list[str],
        'path': str,
        'report_lengths': dict[str, int],
        'reports': dict[str, str],
        'uevent': str,
    },
)

HID: typing.TypedDict = typing.TypedDict(
    'HID',
    {
        'descriptor': bytes,
        'driver': str,
        'events': list[str],
        'hidraw': list[str],
        'iface': str,
        'leds': list[str],
        'path': str,
    },
)

LEDTrial: typing.TypedDict = typing.TypedDict(
    'LEDTrial',
    {
        'expected': str,
        'method': str,
        'node': str,
        'payload': list[int],
        'seen': str,
        'status': str,
    },
)

State: typing.TypedDict = typing.TypedDict(
    'State',
    {
        'done': typing.NotRequired[list[str]],
        'order': typing.NotRequired[list[str]],
    },
)


def read_text(path: str | os.PathLike[str]) -> str:
    try:
        with open(path, 'r', errors='replace') as f:
            return f.read().strip()
    except OSError as e:
        return f'<{e}>'


def read_bytes(path: str | os.PathLike[str]) -> bytes:
    try:
        with open(path, 'rb') as f:
            return f.read()
    except OSError:
        return b''


def parse_descriptor(desc: bytes) -> list[tuple[str, int, int, int, list[tuple[int, int] | tuple[int, int, int]], int]]:
    """Minimal HID item walk: returns [(kind, report_id, bits, usage_page, usages)] per main item."""
    out: list[tuple[str, int, int, int, list[tuple[int, int] | tuple[int, int, int]], int]] = []
    i: int = 0
    page: int = 0
    rid: int = 0
    size: int = 0
    count: int = 0
    usages: list[tuple[int, int] | tuple[int, int, int]] = []
    umin: int | None = None
    while i < len(desc):
        b: int = desc[i]
        i += 1
        n: int
        if b == 0xFE:
            if i >= len(desc):
                break
            n = desc[i]
            i += 2 + n
            continue
        n = (0, 1, 2, 4)[b & 3]
        data: int = int.from_bytes(desc[i:i+n], 'little')
        i += n
        tag: int = b & 0xFC
        match tag:
            case 0x04:
                page = data
            case 0x08:
                usages.append((page, data) if n <= 2 else (data >> 16, data & 0xFFFF))
            case 0x18:
                umin = data
            case 0x28:
                if umin is not None:
                    usages.append((page, umin, data))
                    umin = None
            case 0x74:
                size = data
            case 0x80 | 0x90 | 0xB0:
                kind: str = {
                    0x80: 'input',
                    0x90: 'output',
                    0xB0: 'feature',
                }[tag]
                out.append((kind, rid, size * count, page, usages, data))
                usages = []
            case 0x84:
                rid = data
            case 0x94:
                count = data
            case 0xA0, 0xC0:
                usages = []
    return out


def report_lengths(desc: bytes) -> dict[tuple[str, int], int]:
    """{(kind, report_id): bytes} from the main items."""
    bits: dict[tuple[str, int], int] = {}
    for kind, rid, nbits, _, _, _ in parse_descriptor(desc):
        bits[(kind, rid)] = bits.get((kind, rid), 0) + nbits
    return {k: (v + 7) // 8 for k, v in bits.items()}


class Device:
    """Everything sysfs says about one 1038:0510 and its HID children."""

    def __init__(self, usb_path: str, pid: int):
        self.usb_path: str = usb_path
        self.pid: int = pid
        self.model: str = MODELS.get(pid, f'unknown Ideazon device {pid:04x}')
        self.hids: list[HID] = []
        for hid in sorted(pathlib.Path(usb_path).glob(f'*/[0-9A-F]*:{VID:04X}:{pid:04X}.*')):
            iface: str = read_text(hid.parent / 'bInterfaceNumber')
            desc: bytes = read_bytes(hid / 'report_descriptor')
            hidraw: list[str] = [p.name for p in (hid / 'hidraw').glob('hidraw*')]
            events: list[str] = []
            leds: list[str] = []
            for inp in (hid / 'input').glob('input*'):
                events += [p.name for p in inp.glob('event*')]
                leds += sorted(glob.glob(f'/sys/class/leds/{inp.name}::*'))
            self.hids.append({
                'descriptor': desc,
                'driver': (hid / 'driver').resolve().name,
                'events': sorted(events),
                'hidraw': hidraw,
                'iface': iface,
                'leds': leds,
                'path': str(hid),
            })


def find_devices(pid: int | None=None) -> list[Device]:
    found: list[Device] = []
    for dev in glob.glob('/sys/bus/usb/devices/*'):
        if read_text(pathlib.Path(dev) / 'idVendor') != f'{VID:04x}':
            continue
        product: str = read_text(pathlib.Path(dev) / 'idProduct')
        if product in [f'{p:04x}' for p in (MODELS if pid is None else [pid])]:
            found.append(Device(dev, int(product, 16)))
    return found


class Recorder:
    """Reads every event node and hidraw node of the device into a JSONL log."""

    def __init__(self, dev: Device, log: typing.TextIO):
        self.fds: dict[int, str] = {}
        for h in dev.hids:
            for ev in h['events']:
                self.fds[os.open(f'/dev/input/{ev}', os.O_RDWR | os.O_NONBLOCK)] = ev
            for raw in h['hidraw']:
                self.fds[os.open(f'/dev/{raw}', os.O_RDONLY | os.O_NONBLOCK)] = raw
        self.log: typing.TextIO = log
        self.lock: threading.Lock = threading.Lock()
        self.last_event: float = 0.0
        self.count: int = 0
        self.grabbed: bool = False
        self.stop: bool = False
        self.thread: threading.Thread = threading.Thread(daemon=True, target=self._loop)
        self.thread.start()

    def mark(self, **fields: object) -> None:
        fields.setdefault('t', time.monotonic())
        with self.lock:
            self.log.write(json.dumps(fields) + '\n')
            self.log.flush()

    def _loop(self) -> None:
        while not self.stop:
            ready: list[int]
            ready, _, _ = select.select(list(self.fds), [], [], 0.2)
            for fd in ready:
                try:
                    data: bytes = os.read(fd, 4096)
                except BlockingIOError:
                    continue
                except OSError as e:
                    self.mark(error=str(e), src=self.fds[fd])
                    continue
                t: float = time.monotonic()
                src: str = self.fds[fd]
                if src.startswith('event'):
                    for off in range(0, len(data) - INPUT_EVENT.size + 1, INPUT_EVENT.size):
                        typ: int
                        code: int
                        value: int
                        _, _, typ, code, value = INPUT_EVENT.unpack_from(data, off)
                        if typ == EV_SYN:
                            continue
                        self.mark(code=code, src=src, t=t, type=typ, value=value)
                else:
                    self.mark(raw=data.hex(), src=src, t=t)
                self.last_event = t
                self.count += 1

    def grab(self, on: bool) -> None:
        if on == self.grabbed:
            return
        self.grabbed = on
        for fd, name in self.fds.items():
            if name.startswith('event'):
                try:
                    fcntl.ioctl(fd, EVIOCGRAB, 1 if on else 0)
                except OSError as e:
                    self.mark(grab_error=str(e), src=name)

    def keys_down(self) -> int:
        """How many keys the device reports held right now (EVIOCGKEY over every event node)."""
        held: int = 0
        for fd, name in self.fds.items():
            if name.startswith('event'):
                buf: bytearray = bytearray(96)
                try:
                    fcntl.ioctl(fd, eviocgkey(len(buf)), buf)
                    held += sum(b.bit_count() for b in buf)
                except OSError:
                    pass
        return held

    def settle(self) -> None:
        """Wait until nothing is held and nothing has arrived for SETTLE_S, so the Enter that answered
        the previous question cannot be grabbed mid-press and leak into the next prompt."""
        deadline: float = time.monotonic() + SETTLE_TIMEOUT_S
        while time.monotonic() < deadline:
            if self.keys_down() == 0 and time.monotonic() - self.last_event > SETTLE_S:
                return
            time.sleep(0.02)
        self.mark(settle_timeout=self.keys_down())
        print(f'        (a key still looks held after {SETTLE_TIMEOUT_S:.0f} s; carrying on)')

    def wait_for_activity(self, grab: bool=True) -> int:
        """Wait for the first event (or timeout), then for IDLE_S of silence. Returns the event count."""
        start_count: int = self.count
        deadline: float = time.monotonic() + FIRST_EVENT_TIMEOUT_S
        if grab:
            self.grab(True)
        try:
            while self.count == start_count and time.monotonic() < deadline:
                time.sleep(0.02)
            while self.count != start_count and time.monotonic() - self.last_event < IDLE_S:
                time.sleep(0.02)
        finally:
            self.grab(False)
        return self.count - start_count

    def led_state(self) -> dict[str, dict[str, bool]]:
        state: dict[str, dict[str, bool]] = {}
        for fd, name in self.fds.items():
            if name.startswith('event'):
                buf: bytearray = bytearray(8)
                try:
                    fcntl.ioctl(fd, eviocgled(8), buf)
                    state[name] = {LED_NAMES.get(i, str(i)): bool(buf[i // 8] >> (i % 8) & 1) for i in range(16)}
                except OSError:
                    pass
        return state

    def close(self) -> None:
        self.stop = True
        self.thread.join(1)
        for fd in self.fds:
            os.close(fd)


def prompts(prefix: str='', main: bool=True) -> list[tuple[str, str, bool]]:
    """Stable ids so a capture can be resumed; prefix marks a repeat walk after the matrix-table switch."""
    out: list[tuple[str, str, bool]] = [
        (f'main-{i:02d}', f'main keyboard: press and release [{k}]', False)
        for i, k in enumerate(MAIN_REF)
        if main
    ]
    out += [
        (
            f'pad-{i:02d}',
            (
                f'gaming pad: press and release pad key #{i} of {PAD_KEYS} '
                '(reading order: top row left to right, then the next row)'
            ),
            True,
        )
        for i in range(1, PAD_KEYS + 1)
    ]
    for state in ('off', 'on'):
        out.append((f'numlock-{state}', f'press Num Lock until its LED is {state.upper()} (not grabbed)', False))
        out += [
            (f'np-{state}-{i:02d}', f'numpad (Num Lock {state}): press and release [{k}]', False)
            for i, k in enumerate(NUMPAD)
        ]
    out += [(f'top-{i:02d}', f'top row: press and release [{k}]', False) for i, k in enumerate(TOP_ROW)]
    out += [(cid, text, False) for cid, text in CHORDS]
    return [(prefix + pid, text, legend) for pid, text, legend in out]


def ask(text: str) -> str:
    try:
        return input(text).strip()
    except EOFError:
        return ''


def read_reports(h: HID) -> dict[str, str]:
    """GET_REPORT for every declared feature report and the current input report, read only."""
    reports: dict[str, str] = {}
    lengths: dict[tuple[str, int], int] = report_lengths(h['descriptor'])
    for raw in h['hidraw']:
        fd: int = os.open(f'/dev/{raw}', os.O_RDWR)
        try:
            for (kind, rid), n in sorted(lengths.items()):
                if kind == 'output':
                    continue
                buf: bytearray = bytearray(n + 1)
                buf[0] = rid
                req: int = hidiocgfeature(n + 1) if kind == 'feature' else hidiocginput(n + 1)
                try:
                    got: int = fcntl.ioctl(fd, req, buf)
                    reports[f'{raw} {kind}:{rid}'] = bytes(buf[:got]).hex()
                except OSError as e:
                    reports[f'{raw} {kind}:{rid}'] = str(e)
        finally:
            os.close(fd)
    return reports


def static_capture(dev: Device, out: pathlib.Path) -> None:
    hids: list[Entry] = []
    info: dict[str, str | list[Entry]] = {
        'hids': hids,
        'model': dev.model,
        'os_release': read_text('/etc/os-release'),
        'pid': f'{dev.pid:04x}',
        'python': sys.version,
        'uname': ' '.join(os.uname()),
        'usb_path': dev.usb_path,
    }
    for name in ('manufacturer', 'product', 'serial', 'bcdDevice', 'speed', 'bMaxPower', 'bNumInterfaces', 'version'):
        info[name] = read_text(f'{dev.usb_path}/{name}')
    with open(out / 'usb-descriptors.bin', 'wb') as f:
        f.write(read_bytes(f'{dev.usb_path}/descriptors'))
    for h in dev.hids:
        with open(out / f'report-descriptor-if{h['iface']}.bin', 'wb') as f:
            f.write(h['descriptor'])
        entry: Entry = {
            'descriptor_hex': h['descriptor'].hex(),
            'driver': h['driver'],
            'events': h['events'],
            'hidraw': h['hidraw'],
            'iface': h['iface'],
            'inputs': {
                os.path.basename(inp): {
                    'capabilities': {
                        os.path.basename(c): read_text(c)
                        for c in glob.glob(f'{inp}/capabilities/*')
                    },
                    'name': read_text(f'{inp}/name'),
                    'phys': read_text(f'{inp}/phys'),
                }
                for inp in glob.glob(f'{h['path']}/input/input*')
            },
            'led_brightness': {l: read_text(f'{l}/brightness') for l in h['leds']},
            'leds': h['leds'],
            'path': h['path'],
            'report_lengths': {f'{k[0]}:{k[1]}': v for k, v in report_lengths(h['descriptor']).items()},
            'reports': read_reports(h),
            'uevent': read_text(f'{h['path']}/uevent'),
        }
        hids.append(entry)
    with open(f'{out}/static.json', 'w') as f:
        json.dump(info, f, indent=2)
    print(
        f'static capture: {dev.model} ({VID:04x}:{dev.pid:04x}), {len(dev.hids)} HID interfaces, '
        f'{', '.join(
            f'if{h['iface']}={len(h['descriptor'])}B desc, events {h['events']}, hidraw {h['hidraw']}, '
            f'leds {[os.path.basename(l) for l in h['leds']]}'
            for h in dev.hids
        )}'
    )


def key_walk(rec: Recorder, out: pathlib.Path, state: State, prefix: str='', main: bool=True) -> None:
    done: set[str] = set(state.get('done', []))
    print(
        '\nKEY WALK. For each prompt, press the key once. Keys are grabbed while a prompt is open, so they do\n'
        'not reach your desktop. A prompt starts once every key is released and ends one second after the\n'
        f'last event, or after {FIRST_EVENT_TIMEOUT_S:.0f} s with no event. Ctrl+C stops; rerun with --resume\n'
        'to continue, adding --redo N (or --redo id,id) to repeat the last N prompts (or those ids).\n'
    )
    for pid, text, wants_legend in prompts(prefix, main):
        if pid in done:
            continue
        print(f'[{pid}] {text}')
        rec.settle()
        rec.mark(prompt=pid, text=text)
        n: int = rec.wait_for_activity(grab='numlock-' not in pid)
        rec.mark(events=n, prompt_end=pid)
        print(f'        {n} events' if n else '        no events')
        if wants_legend:
            legend: str = ask('        legend(s) printed on that key (Enter to skip): ')
            rec.mark(legend=pid, text=legend)
        if 'numlock-' in pid:
            rec.mark(led_state=rec.led_state())
        done.add(pid)
        state.setdefault('order', []).append(pid)
        state['done'] = [d for d in state['order'] if d in done]
        with open(f'{out}/state.json', 'w') as f:
            json.dump(state, f)


def led_report(enabled: bool, action: bool) -> list[int]:
    """Feature report 2 with the Enabled and Action LED bits."""
    return [0x02, enabled | action << 1, 0x00]


def table_report(table: int) -> list[int]:
    """Feature report 2 selecting matrix table 1 (the power-on legend table) or 2 (the stand-in table)."""
    return [0x02, 0x0B, table - 1]


def led_trials(dev: Device, out: pathlib.Path) -> None:
    print(
        '\nLED TRIALS. Each step sends one value of feature report 2; answer with the LED you see:\n'
        '  e=Enabled  a=Action  n/c/s=Num/Caps/Scroll  o=other  x=nothing\n'
    )
    if ask('Run the LED trials? [y/N] ').lower() != 'y':
        return
    results: list[LEDTrial] = []
    for raw in feature_nodes(dev):
        for enabled, action, expected in (
            (True, False, 'Enabled'),
            (False, True, 'Action'),
            (True, True, 'Enabled and Action'),
        ):
            set_feature(raw, led_report(False, False))
            payload: list[int] = led_report(enabled, action)
            status: str = set_feature(raw, payload)
            seen: str = (
                ask(f'  {raw} feature [{' '.join(f'{b:02X}' for b in payload)}] (expected {expected}): which LED is lit? ')
                if status == 'ok'
                else status
            )
            results.append({
                'expected': expected,
                'method': 'feature2',
                'node': raw,
                'payload': payload,
                'seen': seen,
                'status': status,
            })
        set_feature(raw, led_report(False, False))
    with open(f'{out}/led-trials.json', 'w') as f:
        json.dump(results, f, indent=2)


def feature_nodes(dev: Device) -> list[str]:
    """hidraw nodes whose descriptor declares feature report 2, else every hidraw node (the driver aimed at interface 1)."""
    declared: list[str] = [
        raw
        for h in dev.hids
        for raw in h['hidraw']
        if ('feature', 2) in report_lengths(h['descriptor'])
    ]
    return declared or [raw for h in dev.hids for raw in h['hidraw']]


def set_feature(raw: str, payload: collections.abc.Sequence[int]) -> str:
    fd: int = os.open(f'/dev/{raw}', os.O_RDWR)
    try:
        fcntl.ioctl(fd, hidiocsfeature(len(payload)), bytearray(payload))
        return 'ok'
    except OSError as e:
        return str(e)
    finally:
        os.close(fd)


def matrix_walk(dev: Device, rec: Recorder, out: pathlib.Path, state: State) -> None:
    print(
        '\nSTAND-IN TABLE. Feature report 2 selects the firmware\'s key matrix: table 1 is the power-on legend\n'
        'table, table 2 the stand-in table ZEngine\'s driver selected at start-up, where the pad sends usages of\n'
        'its own. The same report brings table 1 back; so does a replug.'
    )
    if ask('Switch to table 2 and walk the pad, numpad, top row and chords again? [y/N] ').lower() != 'y':
        return
    send_table(dev, rec, 2)
    key_walk(rec, out, state, main=False, prefix='m2-')
    send_table(dev, rec, 1)
    pid: str = 'm1-pad-01'
    text: str = 'pad key #1 after switching back to table 1'
    print(f'[{pid}] {text}')
    rec.settle()
    rec.mark(prompt=pid, text=text)
    n: int = rec.wait_for_activity()
    rec.mark(events=n, prompt_end=pid)
    print(f'        {n} events')


def send_table(dev: Device, rec: Recorder, table: int) -> None:
    payload: list[int] = table_report(table)
    for raw in feature_nodes(dev):
        status: str = set_feature(raw, payload)
        rec.mark(feature=' '.join(f'{b:02X}' for b in payload), matrix_table=table, node=raw, status=status)
        print(f'  {raw}: sent {' '.join(f'{b:02X}' for b in payload)} (table {table}): {status}')


def main() -> None:
    ap: argparse.ArgumentParser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument(
        '--out',
        help='output directory (default ~/capture-<model>)',
        type=pathlib.Path,
    )
    ap.add_argument(
        '--pid',
        help='USB product id to capture when several Ideazon devices are attached, e.g. 0510',
    )
    ap.add_argument(
        '--resume',
        action='store_true',
        help='continue a walk that was interrupted',
    )
    ap.add_argument(
        '--redo',
        help='with --resume: a number of most recent prompts to repeat, or prompt ids separated by commas',
    )
    ap.add_argument(
        '--skip-walk',
        action='store_true',
    )
    ap.add_argument(
        '--skip-leds',
        action='store_true',
    )
    ap.add_argument(
        '--list-prompts',
        action='store_true',
        help='print the walk plan and exit',
    )
    args: argparse.Namespace = ap.parse_args()

    if args.list_prompts:
        for pid, text, legend in prompts():
            print(f'{pid}|{text}{'| legend' if legend else ''}')
        return

    if os.geteuid() != 0:
        sys.exit('run as root (sudo); hidraw and /dev/input need it')

    devs: list[Device] = find_devices(int(args.pid, 16) if args.pid else None)
    if not devs:
        sys.exit(f'no Ideazon USB device ({VID:04x}:{' / '.join(f'{p:04x}' for p in MODELS)}) found')
    if len(devs) > 1:
        sys.exit(f'several Ideazon devices attached: {', '.join(f'{d.model} {d.pid:04x}' for d in devs)}; pass --pid')
    dev: Device = devs[0]
    out: pathlib.Path = args.out or (pathlib.Path('~') / f'capture-{dev.model.lower().replace(' ', '-')}').expanduser()
    out.mkdir(exist_ok=True, parents=True)
    state: State = {}
    if args.resume and (out / 'state.json').exists():
        with open(out / 'state.json') as f:
            state = json.load(f)
        if args.redo:
            done: list[str] = state.get('done', [])
            dropped: list[str] = (
                done[-int(args.redo):]
                if args.redo.isdigit()
                else [d for d in done if d in args.redo.split(',')]
            )
            state['done'] = [d for d in done if d not in dropped]
            state['order'] = [d for d in state.get('order', done) if d not in dropped]
            print(f'repeating: {' '.join(dropped)}')
    static_capture(dev, out)
    with open(out / 'events.jsonl', 'a') as log:
        rec: Recorder = Recorder(dev, log)
        rec.mark(led_state=rec.led_state(), redo=args.redo, session_start=time.time())
        try:
            if not args.skip_walk:
                key_walk(rec, out, state)
            if not args.skip_leds:
                led_trials(dev, out)
            if not args.skip_walk:
                matrix_walk(dev, rec, out, state)
        except KeyboardInterrupt:
            print('\ninterrupted; rerun with --resume to continue the walk')
        finally:
            rec.mark(session_end=time.time())
            rec.close()
            tar_path: pathlib.Path = pathlib.Path(f'{out}.tar.gz')
            with tarfile.open(tar_path, 'w:gz') as tar:
                tar.add(out, arcname=out.name)
            print(f'\nwrote {tar_path}; copy that file back')


if __name__ == '__main__':
    main()
