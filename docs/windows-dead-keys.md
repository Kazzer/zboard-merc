# The eleven dead keys of table 2 on Windows

In the stand-in table (`02 0B 01`) the pad's LOAD, SAVE, Caps and P and the numpad's
`/ 7 + 2 3 0 .` register nothing on Windows: not in the agent, not in its debug log, not in
any hook. This note says why, what a driver could and could not do about it, and what to
try first.

## What is established

- **The keyboard sends them.** Those eleven keys send HID keyboard usages 0x74–0x7E
  (Execute, Help, Menu, Select, Stop, Again, Undo, Cut, Copy, Paste, Find). The Linux capture,
  shows every one arriving as an input event; Linux's generic HID driver passes every usage,
  and the Linux agent handles all eleven.
- **Windows drops them in `kbdhid.sys`**, the HID keyboard class driver. It turns usages into
  PS/2 scan codes with a fixed table, and that table has no entry for 0x74–0x7E, so the
  events end there: nothing reaches `kbdclass`, Raw Input, the low-level hook or any
  application.
- **ZEngine saw them because its driver replaced Windows'.** Ideazon's kernel drivers
  (`Merc1.sys`, `Merc164.sys`, `merc2.sys`, `merc264.sys`, by Protolog) bound to both
  interfaces in place of `hidusb` and `kbdhid` and did the translation in the kernel from
  registry tables. The repository `adamwestman/driver-zboard-merc` is that driver with an
  edited INF: the binaries date from 2007 (DriverVer 1.0.14.01, 2007-07-23) and carry no
  signature of their own, only a catalog of that era; its own README says driver signature
  enforcement must be disabled, and the "signed" variant it mentions needed the same. Its
  INF supplied the translation tables. On a machine that keeps Secure Boot on and test signing
  off it cannot load, and no edit of it changes that: Windows 10 and 11 load only kernel drivers
  signed by Microsoft.

## Linux

No driver is needed or would add anything. The kernel exposes every usage, the hwdb file
restores the legends without the agent, and the agent sees the original usage through
`MSC_SCAN`. The Linux side is complete as it is.

## Windows: the options

1. **A user-mode HID minidriver (UMDF 2) on WinUSB for the keyboard interface.** Reads the
   interrupt endpoint and rewrites 0x74-0x7E into eleven usages `kbdhid` maps which the keyboard
   doesn't send, then maps them back to the original codes in the agent, while the others
   pass through.
2. **A kernel filter driver** below `kbdhid` is the ideal solution, but kernel drivers need
   Microsoft's attestation signature, which needs an EV certificate and a Partner Center
   account.
3. **A hardware converter** (a small USB-host board rewriting the reports) needs no driver
   and would work at the logon screen too.

## The user-mode driver

`driver/umdf/` holds option 1, built and in use: `zboardhid.c` and `.h` (the minidriver, with
the rewrite table), `ZboardHid.inx` (its INF, bound to the keyboard interface `MI_00` of the
Stealth, `PID_0510`, and of the plain MERC, `PID_0210`), the Visual Studio project, and a
README with the build, signing, install, check and removal steps. It needs Visual Studio
2026 and the WDK from NuGet on Windows; it cannot be built from Linux.

The rewrite, which the agent reverses (`merc.WindowsDriverMap`, the key names `intl1` to
`intl6`, `lang3`, `lang4`, `kp-comma`, `f21`, `f24`):

| Dead usage | Key | Becomes | Scan code kbdhid gives it |
|---|---|---|---|
| 0x74 | pad P | 0x87 International 1 | 0x73 |
| 0x75 | numpad 0 | 0x88 International 2 | 0x70 |
| 0x76 | pad LOAD | 0x89 International 3 | 0x7D |
| 0x77 | pad Caps | 0x8A International 4 | 0x79 |
| 0x78 | numpad / | 0x8B International 5 | 0x7B |
| 0x79 | pad SAVE | 0x8C International 6 | 0x5C |
| 0x7A | numpad . | 0x92 Lang 3 | 0x78 |
| 0x7B | numpad 2 | 0x93 Lang 4 | 0x77 |
| 0x7C | numpad 3 | 0x70 F21 | 0x6C |
| 0x7D | numpad + | 0x73 F24 | 0x76 |
| 0x7E | numpad 7 | 0x85 Keypad Comma | 0x7E |
