#!/bin/sh
# el-book: запуск оболочки вместо Nickel.
#
# NickelMenu запускает скрипт пунктом из scripts/device/nm/el-book:
#   cmd_spawn :quiet:exec /bin/sh /mnt/onboard/.adds/el-book/launch.sh
#
# Скрипт останавливает Nickel, запускает el-book и по коду выхода выключает книгу (10),
# перезагружает её (11) или возвращает Nickel (всё остальное, в том числе падение).
# Последовательность остановки и возврата Nickel повторяет известные факты о KOReader
# и Plato (см. docs/explanation/Жизненный цикл приложения на Kobo.md).

EB_DIR="/mnt/onboard/.adds/el-book"
EB_BIN="${EB_DIR}/el-book"
EB_LOGS="${EB_DIR}/logs"
EB_LOG="${EB_LOGS}/launch.log"
EB_LOCK="/tmp/el-book.lock"

# Работаем из копии в /tmp: sh читает скрипт по частям, а файл на /mnt/onboard
# могут заменить, пока он выполняется.
if [ "${EB_FROM_TMP}" != "1" ]; then
    cp -f "$0" /tmp/el-book-launch.sh || exit 1
    EB_FROM_TMP=1 exec /bin/sh /tmp/el-book-launch.sh "$@"
fi

mkdir -p "${EB_LOGS}"
if [ -f "${EB_LOG}" ] && [ "$(wc -c <"${EB_LOG}")" -gt 262144 ]; then
    mv -f "${EB_LOG}" "${EB_LOG}.1"
fi

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >>"${EB_LOG}"
}

# Второй запуск, пока работает первый, ничего не делает.
if ! mkdir "${EB_LOCK}" 2>/dev/null; then
    log "уже запущен, второй запуск пропущен"
    exit 0
fi

if [ ! -f "${EB_BIN}" ]; then
    log "нет ${EB_BIN}, Nickel не останавливаем"
    rmdir "${EB_LOCK}"
    exit 1
fi

# Берёт переменные окружения у процесса $1, если они ещё не заданы.
siphon() {
    pid="$(pidof -s "$1")" || return 0
    shift
    for var in "$@"; do
        eval "cur=\${${var}}"
        [ -n "${cur}" ] && continue
        val="$(tr '\0' '\n' <"/proc/${pid}/environ" | sed -n "s/^${var}=//p" | head -n 1)"
        [ -n "${val}" ] && export "${var}=${val}"
    done
}

stop_nickel() {
    log "останавливаем Nickel"
    sync
    killall -q -TERM nickel hindenburg sickel fickel strickel fontickel adobehost foxitpdf iink \
        dhcpcd-dbus dhcpcd bluealsa bluetoothd fmon nanoclock.lua
    n=0
    while pkill -0 nickel 2>/dev/null && [ "${n}" -lt 16 ]; do
        usleep 250000
        n=$((n + 1))
    done
}

# Wi-Fi гасится перед работой и перед возвратом Nickel: со включённым модулем сон
# ненадёжен, а Nickel ждёт, что Wi-Fi выключен. Модули, которым нужно отдельное
# управление питанием (moal, wlan_drv_gen4m), скрипт не трогает.
wifi_off() {
    [ -n "${WIFI_MODULE}" ] || return 0
    grep -q "^${WIFI_MODULE} " /proc/modules || return 0
    case "${WIFI_MODULE}" in
        moal | wlan_drv_gen4m)
            log "Wi-Fi ${WIFI_MODULE} включён, но выключать такой модуль скрипт не умеет"
            return 0
            ;;
    esac
    log "выключаем Wi-Fi ${WIFI_MODULE}"
    killall -q -TERM udhcpc default.script dhcpcd
    if [ -n "${INTERFACE}" ]; then
        wpa_cli -i "${INTERFACE}" terminate >/dev/null 2>&1
        [ "${WIFI_MODULE}" = "dhd" ] && wlarm_le -i "${INTERFACE}" down
        ifconfig "${INTERFACE}" down
    fi
    usleep 250000
    rmmod "${WIFI_MODULE}"
    if grep -q "^sdio_wifi_pwr " /proc/modules; then
        usleep 250000
        rmmod sdio_wifi_pwr
    fi
}

CPUFREQ="/sys/devices/system/cpu/cpu0/cpufreq"
[ -d "/sys/devices/system/cpu/cpufreq/policy0" ] && CPUFREQ="/sys/devices/system/cpu/cpufreq/policy0"
ORIG_GOVERNOR=""

# Отзывчивый регулятор частоты процессора: interactive, иначе ondemand.
set_governor() {
    [ -w "${CPUFREQ}/scaling_governor" ] || return 0
    cur="$(cat "${CPUFREQ}/scaling_governor")"
    for gov in interactive ondemand; do
        [ "${cur}" = "${gov}" ] && return 0
        if grep -q "${gov}" "${CPUFREQ}/scaling_available_governors"; then
            ORIG_GOVERNOR="${cur}"
            echo "${gov}" >"${CPUFREQ}/scaling_governor"
            log "регулятор частоты: ${cur} → ${gov}"
            return 0
        fi
    done
}

restore_governor() {
    [ -n "${ORIG_GOVERNOR}" ] && echo "${ORIG_GOVERNOR}" >"${CPUFREQ}/scaling_governor"
    ORIG_GOVERNOR=""
}

start_nickel() {
    log "возвращаем Nickel"
    restore_governor
    wifi_off
    cd / || true
    export LD_LIBRARY_PATH="/usr/local/Kobo"
    /etc/init.d/on-animator.sh &
    rm -f /tmp/nickel-hardware-status
    mkfifo /tmp/nickel-hardware-status
    sync
    /usr/local/Kobo/hindenburg &
    LIBC_FATAL_STDERR_=1 /usr/local/Kobo/nickel -platform kobo -skipFontLoad &
    [ "${PLATFORM}" != "freescale" ] && udevadm trigger &
}

siphon nickel PRODUCT PLATFORM WIFI_MODULE INTERFACE LANG NICKEL_HOME DBUS_SESSION_BUS_ADDRESS
siphon udevd PRODUCT PLATFORM
log "запуск: PRODUCT=${PRODUCT} PLATFORM=${PLATFORM} WIFI_MODULE=${WIFI_MODULE}"

stop_nickel
wifi_off
set_governor

"${EB_BIN}" -root "${EB_DIR}" -log "${EB_LOGS}/el-book.log" >>"${EB_LOGS}/el-book.out" 2>&1
code=$?
log "el-book завершился с кодом ${code}"
rmdir "${EB_LOCK}"

case "${code}" in
    10)
        restore_governor
        log "выключение"
        sync
        poweroff -f
        ;;
    11)
        restore_governor
        log "перезагрузка"
        sync
        reboot
        ;;
    *)
        start_nickel
        ;;
esac
