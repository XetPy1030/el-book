#!/bin/sh
# el-book: Wi-Fi без Nickel. Оболочка запускает скрипт как «sh wifi.sh <команда>»:
#   up    — питание чипа, модуль драйвера, интерфейс и wpa_supplicant с сетями Nickel;
#   dhcp  — получить адрес (wpa_supplicant уже подключился к сети);
#   down  — отпустить адрес и выключить всё, что включил up.
#
# WIFI_MODULE, INTERFACE и PLATFORM оболочка получает от скрипта запуска, а тот — от Nickel.
# Последовательность повторяет факты из скриптов KOReader enable-wifi.sh, obtain-ip.sh и
# disable-wifi.sh (см. docs/explanation/Жизненный цикл приложения на Kobo.md); код не перенесён.
# Конфиг сетей Nickel только читается: скрипт его не меняет.
#
# Коды выхода: 0 — готово, 2 — неизвестная команда, 3 — модуль не поддерживается,
# 4 — модуль не загрузился, 5 — интерфейс не поднялся, 6 — wpa_supplicant не запустился,
# 7 — адрес не получен.

WPA_CONF="/etc/wpa_supplicant/wpa_supplicant.conf"
WPA_CTRL="/var/run/wpa_supplicant"
KMOD_PATH="/drivers/${PLATFORM}/wifi"
# Где ядро перечисляет модули; тест подменяет файл.
PROC_MODULES="${PROC_MODULES:-/proc/modules}"
# insmod, ifconfig и dhcpcd лежат в /sbin, которого может не быть в PATH у NickelMenu.
PATH="${PATH}:/sbin:/usr/sbin"

log() {
    echo "wifi.sh: $*" >&2
}

loaded() {
    grep -q "^$1 " "${PROC_MODULES}"
}

check_env() {
    if [ -z "${WIFI_MODULE}" ] || [ -z "${INTERFACE}" ] || [ -z "${PLATFORM}" ]; then
        log "нет WIFI_MODULE, INTERFACE или PLATFORM"
        exit 3
    fi
    case "${WIFI_MODULE}" in
        moal | wlan_drv_gen4m)
            # Этим чипам питание включают отдельным ioctl или через wmt — не поддержано.
            log "модуль ${WIFI_MODULE} не поддерживается"
            exit 3
            ;;
    esac
}

up() {
    check_env
    if ! loaded sdio_wifi_pwr && [ -e "${KMOD_PATH}/sdio_wifi_pwr.ko" ]; then
        insmod "${KMOD_PATH}/sdio_wifi_pwr.ko" || exit 4
    fi
    usleep 250000
    if ! loaded "${WIFI_MODULE}"; then
        ko="${KMOD_PATH}/${WIFI_MODULE}.ko"
        [ -e "${ko}" ] || ko="/drivers/${PLATFORM}/${WIFI_MODULE}.ko"
        if [ ! -e "${ko}" ]; then
            log "нет файла модуля ${WIFI_MODULE}"
            exit 4
        fi
        insmod "${ko}" || exit 4
        # После загрузки модуля Nickel ждёт; без паузы интерфейс не поднимается.
        sleep 1
    fi
    ifconfig "${INTERFACE}" up || exit 5
    if [ "${WIFI_MODULE}" = "dhd" ]; then
        wlarm_le -i "${INTERFACE}" up
    fi
    if ! pkill -0 wpa_supplicant; then
        wpa_supplicant -D wext -s -i "${INTERFACE}" -c "${WPA_CONF}" -C "${WPA_CTRL}" -B || exit 6
    fi
    n=0
    while [ ! -e "${WPA_CTRL}/${INTERFACE}" ] && [ "${n}" -lt 20 ]; do
        usleep 250000
        n=$((n + 1))
    done
    [ -e "${WPA_CTRL}/${INTERFACE}" ] || exit 6
}

release() {
    if [ -x /sbin/dhcpcd ]; then
        dhcpcd -d -k "${INTERFACE}" >/dev/null 2>&1
    fi
    killall -q -TERM udhcpc default.script
}

dhcp() {
    check_env
    release
    # Nickel получает адрес dhcpcd; если его нет — udhcpc из busybox.
    if [ -x /sbin/dhcpcd ]; then
        dhcpcd -d -t 30 -w "${INTERFACE}" || exit 7
    else
        udhcpc -S -i "${INTERFACE}" -s /etc/udhcpc.d/default.script -t 15 -T 2 -n -q || exit 7
    fi
}

down() {
    check_env
    release
    wpa_cli -i "${INTERFACE}" terminate >/dev/null 2>&1
    if [ "${WIFI_MODULE}" = "dhd" ]; then
        wlarm_le -i "${INTERFACE}" down
    fi
    ifconfig "${INTERFACE}" down
    if loaded "${WIFI_MODULE}"; then
        usleep 250000
        rmmod "${WIFI_MODULE}"
    fi
    if loaded sdio_wifi_pwr; then
        usleep 250000
        rmmod sdio_wifi_pwr
    fi
}

case "$1" in
    up) up ;;
    dhcp) dhcp ;;
    down) down ;;
    *)
        log "команда: up, dhcp или down"
        exit 2
        ;;
esac
