#!/bin/sh
# Проверка wifi.sh в контейнере busybox: вместо insmod, rmmod, ifconfig, wpa_supplicant,
# dhcpcd и прочих — заглушки, которые записывают свои вызовы в /tmp/calls; загруженные
# модули — в файле /tmp/modules вместо /proc/modules. Запуск: make test-scripts.
# Скрипт создаёт файлы в /drivers, /sbin, /var/run и /stubs, поэтому только для одноразового контейнера.

set -u
SRC="$(dirname "$0")/.."
KMOD="/drivers/mx6sll-ntx/wifi"
MODULE=8192es
failed=0

stub() {
    printf '#!/bin/sh\necho "%s $*" >>/tmp/calls\n%s\n' "$(basename "$1")" "${2:-}" >"$1"
    chmod +x "$1"
}

# setup готовит книгу: $1 — загруженные модули через пробел, $2 — есть ли dhcpcd (yes/no).
setup() {
    rm -rf /stubs /var/run/wpa_supplicant /drivers /sbin/dhcpcd /tmp/calls /tmp/modules /tmp/wpa_running
    mkdir -p /stubs /sbin "${KMOD}" /var/run/wpa_supplicant
    : >/tmp/calls
    : >/tmp/modules
    for m in $1; do
        echo "${m} 16384 0 - Live 0x00000000" >>/tmp/modules
    done
    : >"${KMOD}/sdio_wifi_pwr.ko"
    : >"${KMOD}/8192es.ko"
    # Тела заглушек раскрываются при их вызове, а не здесь.
    # shellcheck disable=SC2016
    stub /stubs/insmod 'echo "$(basename "$1" .ko) 16384 0 - Live 0x00000000" >>/tmp/modules'
    # shellcheck disable=SC2016
    stub /stubs/rmmod 'grep -v "^$1 " /tmp/modules >/tmp/modules.new; mv /tmp/modules.new /tmp/modules'
    stub /stubs/ifconfig
    stub /stubs/wlarm_le
    stub /stubs/wpa_supplicant ': >/var/run/wpa_supplicant/eth0; : >/tmp/wpa_running'
    stub /stubs/wpa_cli 'rm -f /var/run/wpa_supplicant/eth0 /tmp/wpa_running'
    stub /stubs/pkill '[ -e /tmp/wpa_running ]'
    stub /stubs/killall
    stub /stubs/udhcpc
    if [ "$2" = yes ]; then
        stub /sbin/dhcpcd
    fi
}

# wifi запускает скрипт как оболочка: sh wifi.sh <команда>.
wifi() {
    env PATH="/stubs:${PATH}" PROC_MODULES=/tmp/modules PLATFORM=mx6sll-ntx \
        WIFI_MODULE="${MODULE}" INTERFACE=eth0 sh "${SRC}/wifi.sh" "$@" 2>/dev/null
}

check() {
    if grep -q -- "$2" /tmp/calls; then
        [ "$1" = has ] && return 0
    else
        [ "$1" = no ] && return 0
    fi
    echo "НЕ ПРОШЛО: $3: ожидалось $1 «$2»; вызовы:"
    sed 's/^/    /' /tmp/calls
    failed=1
}

code() {
    if [ "$1" -ne "$2" ]; then
        echo "НЕ ПРОШЛО: $3: код $1, ожидался $2"
        failed=1
    fi
}

# Включение с нуля: питание, модуль, интерфейс, wpa_supplicant с конфигом Nickel.
setup "" yes
wifi up
code $? 0 "up с нуля"
check has "insmod ${KMOD}/sdio_wifi_pwr.ko" "up с нуля"
check has "insmod ${KMOD}/8192es.ko" "up с нуля"
check has "ifconfig eth0 up" "up с нуля"
check has "wpa_supplicant -D wext -s -i eth0 -c /etc/wpa_supplicant/wpa_supplicant.conf -C /var/run/wpa_supplicant -B" "up с нуля"
check no "wlarm_le" "up с нуля"

# Повторное включение ничего не загружает второй раз.
: >/tmp/calls
wifi up
code $? 0 "повторный up"
check no "insmod" "повторный up"
check no "wpa_supplicant -D" "повторный up"

# Адрес: dhcpcd, как у Nickel; без dhcpcd — udhcpc.
wifi dhcp
code $? 0 "dhcp"
check has "dhcpcd -d -t 30 -w eth0" "dhcp через dhcpcd"
setup "sdio_wifi_pwr 8192es" no
wifi dhcp
code $? 0 "dhcp без dhcpcd"
check has "udhcpc -S -i eth0" "dhcp через udhcpc"

# Выключение: адрес, wpa_supplicant, интерфейс, модуль, питание — всё, что включил up.
setup "" yes
wifi up
: >/tmp/calls
wifi down
code $? 0 "down"
check has "dhcpcd -d -k eth0" "down"
check has "wpa_cli -i eth0 terminate" "down"
check has "ifconfig eth0 down" "down"
check has "rmmod 8192es" "down"
check has "rmmod sdio_wifi_pwr" "down"
if [ -s /tmp/modules ]; then
    echo "НЕ ПРОШЛО: после down остались модули: $(cat /tmp/modules)"
    failed=1
fi

# Ошибки: нет переменных, неподдержанный модуль, нет файла модуля, нет сокета, нет команды.
setup "" yes
MODULE=""
wifi up
code $? 3 "без WIFI_MODULE"
MODULE=moal
wifi up
code $? 3 "модуль moal"
check no "insmod" "неподдержанный модуль"
MODULE=8192es
rm -f "${KMOD}/8192es.ko"
wifi up
code $? 4 "нет файла модуля"
setup "" yes
stub /stubs/wpa_supplicant
wifi up
code $? 6 "wpa_supplicant без сокета"
wifi restart
code $? 2 "неизвестная команда"

if [ "${failed}" -ne 0 ]; then
    echo "wifi.sh: есть ошибки"
    exit 1
fi
echo "wifi.sh: все проверки прошли"
