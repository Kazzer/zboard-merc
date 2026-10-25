#!/usr/bin/env bash
set -eu

sudo sh <<ROOT
rm -f /etc/udev/rules.d/70-zboard.rules /etc/udev/hwdb.d/70-zboard.hwdb /etc/modules-load.d/zboard-uinput.conf
systemd-hwdb update
udevadm control --reload
udevadm trigger --subsystem-match=input --subsystem-match=hidraw --action=add
ROOT

systemctl --user disable --now zengine
rm -f ~/.local/bin/zengine ~/.local/bin/zboard ~/.config/systemd/user/zengine.service
if [ "${1:-}" == "--purge" ]
then
    rm -rf ~/.config/zboard
fi
