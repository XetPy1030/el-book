#!/bin/sh
# Сбор данных об устройстве для снимка в документации. Только читает систему.
# Запуск на книге: sh /mnt/onboard/.adds/el-book/probe.sh (пункт NickelMenu из scripts/device/nm/el-book).
# Результат: /mnt/onboard/.adds/el-book/probe/probe-<дата>.txt. Серийный номер и MAC-адреса маскируются,
# пароли Wi-Fi не читаются.

BASE=/mnt/onboard/.adds/el-book
OUT_DIR="$BASE/probe"
mkdir -p "$OUT_DIR" || exit 1
STAMP=$(date +%Y%m%d-%H%M%S)
RAW="/tmp/el-book-probe-$STAMP.txt"
OUT="$OUT_DIR/probe-$STAMP.txt"

section() {
	printf '\n===== %s\n' "$1"
}

# Печатает «путь: содержимое» для каждого существующего файла.
show_files() {
	for f in "$@"; do
		[ -e "$f" ] && printf '%s: %s\n' "$f" "$(cat "$f" 2>&1)"
	done
}

{
	section "probe"
	echo "date: $(date '+%Y-%m-%d %H:%M:%S %Z')"
	echo "TZ: ${TZ:-не задан}"
	ls -l /etc/localtime 2>&1

	section "kobo version (серийный номер замаскирован)"
	sed 's/^\(N[0-9][0-9][0-9]\)[^,]*/\1*********/' /mnt/onboard/.kobo/version

	section "kernel"
	uname -a
	cat /proc/cmdline

	section "cpu"
	grep -v -i '^serial' /proc/cpuinfo
	show_files \
		/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor \
		/sys/devices/system/cpu/cpu0/cpufreq/scaling_available_governors \
		/sys/devices/system/cpu/cpu0/cpufreq/scaling_available_frequencies \
		/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq

	section "memory"
	head -n 8 /proc/meminfo

	section "mounts"
	cat /proc/mounts

	section "disk"
	df -h

	section "nickel env"
	NICKEL_PID=$(pidof -s nickel)
	if [ -n "$NICKEL_PID" ]; then
		tr '\0' '\n' <"/proc/$NICKEL_PID/environ" |
			grep -E '^(PLATFORM|PRODUCT|MODEL_NUMBER|WIFI_MODULE|INTERFACE|LANG|NICKEL_HOME|QT_QPA_PLATFORM)='
		PLATFORM=$(tr '\0' '\n' <"/proc/$NICKEL_PID/environ" | sed -n 's/^PLATFORM=//p')
	else
		echo "nickel не запущен"
	fi

	section "processes"
	ps

	section "kernel modules"
	cat /proc/modules

	section "input devices"
	cat /proc/bus/input/devices

	section "framebuffer"
	show_files \
		/sys/class/graphics/fb0/name \
		/sys/class/graphics/fb0/bits_per_pixel \
		/sys/class/graphics/fb0/rotate \
		/sys/class/graphics/fb0/virtual_size \
		/sys/class/graphics/fb0/stride \
		/sys/class/graphics/fb0/modes

	section "fbink -e"
	FBINK=/mnt/onboard/.adds/koreader/fbink
	if [ -e "$FBINK" ]; then
		"$FBINK" -e 2>&1 | tr ';' '\n'
	else
		echo "fbink не найден"
	fi

	section "backlight"
	for d in /sys/class/backlight/*; do
		show_files "$d/brightness" "$d/actual_brightness" "$d/max_brightness" "$d/color"
	done

	section "power supply"
	for d in /sys/class/power_supply/*; do
		echo "[$d]"
		cat "$d/uevent" 2>&1
	done

	section "power"
	ls /sys/power
	show_files /sys/power/state

	section "leds"
	ls /sys/class/leds 2>&1

	section "wifi"
	ls /drivers 2>&1
	[ -n "$PLATFORM" ] && ls -la "/drivers/$PLATFORM/wifi" 2>&1
	if [ -e /etc/wpa_supplicant/wpa_supplicant.conf ]; then
		echo "сохранённых сетей: $(grep -c '^network=' /etc/wpa_supplicant/wpa_supplicant.conf)"
	fi

	section "usb gadget"
	[ -n "$PLATFORM" ] && ls -la "/drivers/$PLATFORM" "/drivers/$PLATFORM/usb/gadget" 2>&1

	section "libc"
	ls -l /lib/libc.so* /lib/libc-* /lib/ld-linux* 2>&1

	section "busybox"
	busybox 2>&1 | head -n 1
	busybox --list 2>&1 | tr '\n' ' '
	echo

	section "tools"
	for t in dropbear ssh scp sftp-server rsync wget curl ntpd udhcpc wpa_cli insmod evtest; do
		printf '%s: %s\n' "$t" "$(command -v "$t" || echo -)"
	done

	section "nickelmenu"
	ls -la /usr/local/Kobo/imageformats 2>&1
	ls -la /mnt/onboard/.adds/nm 2>&1

	section "kfmon"
	ls -la /usr/local/kfmon /usr/local/kfmon/bin 2>&1

	section "udev fifo"
	ls -la /tmp/nickel-hardware-status 2>&1

	section "fb sysfs"
	show_files /sys/class/graphics/fb0/rotate /sys/class/graphics/fb0/bits_per_pixel \
		/sys/class/graphics/fb0/virtual_size /sys/class/graphics/fb0/stride

	section "dmesg (фильтр)"
	dmesg | grep -i -E 'PM:|suspend|resume|epdc|cyttsp|gsensor|accel|wifi|sdio|usb|mmc' | tail -n 100

	section "adds"
	ls -la /mnt/onboard/.adds

	section "uptime"
	uptime
} >"$RAW" 2>&1

# Маскируем MAC-адреса перед записью на пользовательский раздел.
HEX='[0-9a-fA-F][0-9a-fA-F]'
sed "s/$HEX:$HEX:$HEX:$HEX:$HEX:$HEX/xx:xx:xx:xx:xx:xx/g" "$RAW" >"$OUT"
rm -f "$RAW"
sync

echo "Готово: .adds/el-book/probe/$(basename "$OUT")"
echo "Строк: $(wc -l <"$OUT")"
