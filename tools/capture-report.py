#!/usr/bin/env python3
"""Render a capture directory from capture-linux.py as a per-prompt table (Markdown to stdout)."""
import itertools
import json
import pathlib
import re
import sys
import typing

CODES: dict[int, str] = {
    int(m.group(3), 0): f'{m.group(1)}{m.group(2)}'
    for path in (pathlib.Path('/usr/include/linux/input-event-codes.h'),)
    if path.exists()
    for m in re.finditer(r'^#define (KEY_|BTN_)(\w+)\s+(0x[0-9a-fA-F]+|\d+)', path.read_text(), re.MULTILINE)
}

Event: typing.TypedDict = typing.TypedDict(
    'Event',
    {
        'code': typing.NotRequired[int],
        'ext': typing.NotRequired[bool],
        'legend': typing.NotRequired[str],
        'prompt': typing.NotRequired[str],
        'raw': typing.NotRequired[str],
        'scan': typing.NotRequired[int],
        'src': typing.NotRequired[str],
        'text': typing.NotRequired[str],
        'type': typing.NotRequired[int],
        'up': typing.NotRequired[bool],
        'usages': typing.NotRequired[list[str] | None],
        'value': typing.NotRequired[int],
        'vk': typing.NotRequired[int],
    },
)

Segment: typing.TypedDict = typing.TypedDict(
    'Segment',
    {
        'keys': list[tuple[str, int, str]],
        'other': list[tuple[int, int, int]],
        'raw': dict[str, list[str]],
        'scan': list[str],
        'text': str,
    },
)


def keyname(code: int) -> str:
    return CODES.get(code, str(code))


def usage(v: int) -> str:
    page: int = v >> 16
    u: int = v & 0xFFFF
    return {
        1: f'gd:{u:02X}',
        7: f'kbd:{u:02X}',
        12: f'cons:{u:03X}',
    }.get(page, f'{page:X}:{u:X}')


def main(d: pathlib.Path) -> None:
    events: list[Event] = [json.loads(l) for l in (d / 'events.jsonl').read_text().splitlines()]
    legends: dict[str, str] = {e['legend']: e['text'] for e in events if 'legend' in e}
    segments: dict[str, Segment] = {}
    cur: str | None = None
    for e in events:
        if 'prompt' in e:
            cur = e['prompt']
            segments.pop(cur, None)
            segments[cur] = {'keys': [], 'other': [], 'raw': {}, 'scan': [], 'text': e['text']}
        elif 'prompt_end' in e:
            cur = None
        elif cur and 'raw' in e:
            segments[cur]['raw'].setdefault(e['src'], []).append(e['raw'])
            for u in e.get('usages') or []:
                segments[cur]['scan'].append(u)
        elif cur and e.get('src') == 'llhook':
            segments[cur]['keys'].append((
                f'vk{e['vk']:02X}/sc{e['scan']:02X}{'E' * bool(e.get('ext'))}',
                int(not e['up']),
                'llhook',
            ))
        elif cur and 'type' in e:
            if e['type'] == 4 and e['code'] == 4:
                segments[cur]['scan'].append(usage(e['value']))
            elif e['type'] == 1:
                segments[cur]['keys'].append((keyname(e['code']), e['value'], e['src']))
            else:
                segments[cur]['other'].append((e['type'], e['code'], e['value']))
    sources: list[str] = list(dict.fromkeys(
        src
        for s in segments.values()
        for src in s['raw']
    ))
    print(
        '| prompt | legend / key | '
        f'{' | '.join(f'{src} reports' for src in sources)}'
        ' | HID usages | keys (evdev press=1 release=0 repeat=2; Windows vk/scan ↓=1 ↑=0) |'
    )
    print(
        '|---|---|'
        f'{'---|' * len(sources)}'
        '---|---|'
    )
    for pid, s in segments.items():
        reports: str = ' | '.join(
            ' '.join(r for r, _ in itertools.groupby(s['raw'].get(src, [])))
            for src in sources
        )
        scans: str = ' '.join(dict.fromkeys(s['scan']))
        keys: str = ' '.join(f'{k}={v}' for k, v, _ in s['keys'])
        label: str = legends.get(pid) or re.sub(r'.*\[(.*)\].*', r'\1', s['text'])
        print(f'| {pid} | {label} | {reports} | {scans} | {keys} |')
    others: list[Event] = [
        e
        for e in events
        if 'grab_error' in e
        or 'led_state' in e
        or 'led_restore_error' in e
        or 'matrix_table' in e
    ]
    print('\nOther records:')
    for e in others:
        print(' ', {k: v for k, v in e.items() if k != 't'})


if __name__ == '__main__':
    main(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else '.'))
