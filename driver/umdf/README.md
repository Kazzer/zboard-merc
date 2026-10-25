# zboardhid: user-mode HID minidriver for the Zboard MERC keyboards

In the stand-in matrix table, eleven keys of the MERC send HID usages 0x74 to 0x7E, which Windows' `kbdhid.sys` drops.
This driver takes the place of `hidusb.sys` on the keyboard interface and rewrites those eleven usages into ones
`kbdhid` maps; the agent maps them back to the original. Everything else passes through, so the HID class driver,
`kbdhid`, and everything above see an ordinary keyboard, and the logon screen and UAC keep working.

The lock LEDs are also managed by the driver as noted below.

## Build

Visual Studio 2026:
* Desktop development with C++
* Windows Driver Kit
* C++ Spectre-mitigated libraries for x64/x86 (Latest MSVC)

Developer Command Prompt:
```
winget install --id Microsoft.NuGet --source winget
nuget restore packages.config -PackagesDirectory packages
msbuild ZboardHid.vcxproj /p:Configuration=Release /p:Platform=x64 /p:ZboardBuild=0
```

Note: increment ZboardBuild for new builds on the same day

Errors:
| Code / Message | Meaning |
|---|---|
| "Cannot open include file: 'wudfwdm.h'" | packages are not restored or the wrong version is present |
| MSB8040 | Spectre-mitigated libraries are missing |
| "toolset WindowsUserModeDriver10.0 not found" | Windows Driver Kit is not installed |

## Sign for this machine

Visual Studio build will create a certificate by default, which can be installed, but it's better to create a
self-signed certificate which can sign the driver and be used on subsequent builds.

Once, in an administrator PowerShell, make a code-signing certificate and trust it:

```
$cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject "CN=zboard local driver" -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddYears(5)
Export-Certificate -Cert $cert -FilePath zboard.cer | Out-Null
Import-Certificate -FilePath zboard.cer -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
Import-Certificate -FilePath zboard.cer -CertStoreLocation Cert:\LocalMachine\TrustedPublisher | Out-Null
$cert.Thumbprint
```

After every build, in the package directory (a Developer Command Prompt has `signtool` and `Inf2Cat` on the path):

```
signtool sign /fd sha256 /sha1 <thumbprint> zboardhid.dll
signtool sign /fd sha256 /sha1 <thumbprint> ZboardHid.cat
```

## Install and remove

Install:
```
pnputil /add-driver ZboardHid.inf /install
```

Windows picks this driver for the keyboard interface because its hardware id match beats the generic HID keyboard match.
If it asks whether to trust the publisher, that is the certificate above. Device Manager then shows "Zboard MERC" or
"Zboard MERC Stealth" under Human Interface Devices, with Aether Software as the driver provider, the version from
`VERSION`, and the HID Keyboard Device under Keyboards as before.

If the driver is not selected, in Device Manager, Update driver, Browse to the package directory.

Remove
```
pnputil /enum-drivers | Select-String 'zboardhid.inf' -Context 1,7
pnputil /delete-driver oemNN.inf /uninstall
```

## The lock LEDs

The driver cannot receive Windows' LED commands directly, and user-mode software cannot communicate with it through the
usual device interfaces. So the driver controls the LEDs itself, using two different sources:

* While the agent is running, it tells the driver which LEDs to turn on through a named pipe. It sends a bitmask
  representing Num Lock (1), Caps Lock (2), and Scroll Lock (4), whenever Windows' lock state changes and every five
  seconds. Only one client can connect at a time, and only a process of an interactively logged-on user can connect: the
  pipe's access list grants write access to the Interactive group and nobody else. The worst a client can do is change
  the three LEDs, or hold the connection so that the agent cannot. DebugView should show `agent connected`.
* While the agent is not running, the driver controls the LEDs itself. It turns Num Lock on at startup and toggles each
  lock LED whenever its corresponding key is pressed. This keeps the keyboard usable at the Windows logon screen,
  provided Windows’ initial lock state matches the driver's assumptions.

Limitations:

* If Num Lock is off in Windows when the keyboard is connected and the agent isn't running, pressing Num Lock twice is
  required to synchronise it.
* With the agent running, the first numpad press corrects Windows' state, and the LED follows it.
* The agent is still required for modifier keys, switching key tables, extra LEDs, and the other features previously
  handled by ZEngine.
* If the LED state is not synchronised, pressing a pad number (1 through 10) should synchronise the LED state

## Watching it work

DebugView will capture the following trace events with `Capture Global Win32`; filter on `zboardhid*`:
* device starts (interface, ids, descriptor sizes)
* LED writes (with a five second interval)
* a control transfer the keyboard refused or did not answer
* a missing interrupt pipe
* a report write without a buffer

A status of `0x0` is a success, `0xc0000185` or `0xc000009d` are refused or did not answer.

## When it does not start

* Code 10 or 31: the UMDF host could not start the driver. Details are in the Event Viewer under
  Application and Service Logs > Microsoft > Windows > DriverFramework-UserMode > Operational.
* Code 52: the package signature was not accepted. Check both certificate stores above
* Keyboard doesn't respond: aforementioned Event Viewer log shows whether `EvtDevicePrepareHardware` failed
