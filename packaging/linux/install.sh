#!/usr/bin/env bash
set -e

pushd "$(dirname "$0")" >/dev/null
    sudo sh <<ROOT
install -D -m 644 70-zboard.rules /etc/udev/rules.d/70-zboard.rules
install -D -m 644 70-zboard.hwdb /etc/udev/hwdb.d/70-zboard.hwdb
install -D -m 644 zboard-uinput.conf /etc/modules-load.d/zboard-uinput.conf
systemd-hwdb update
udevadm control --reload
udevadm trigger --subsystem-match=input --subsystem-match=hidraw --action=add
modprobe uinput
ROOT

    install -D -m 755 zengine ~/.local/bin/zengine
    install -D -m 755 zboard ~/.local/bin/zboard
    install -D -m 644 zengine.service ~/.config/systemd/user/zengine.service
    mkdir -p ~/.config/zboard/mods
    if [ "${1:-}" == "--examples" -o ! -e ~/.config/zboard/config.yaml ]
    then
        cp config.yaml ~/.config/zboard/
        cp mods/*.yaml ~/.config/zboard/mods/
    fi
    systemctl --user enable --now zengine
popd >/dev/null
