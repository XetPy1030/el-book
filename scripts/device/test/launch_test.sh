#!/bin/sh
# Проверка launch.sh в контейнере busybox: вместо Nickel, el-book, poweroff и reboot —
# заглушки, которые записывают свои вызовы в /tmp/calls. Запуск: make test-scripts.
# Скрипт создаёт файлы в /mnt, /usr/local, /etc и /stubs, поэтому только для одноразового контейнера.

set -u
SRC="$(dirname "$0")/.."
EB_DIR="/mnt/onboard/.adds/el-book"
failed=0

stub() {
    printf '#!/bin/sh\necho "%s $*" >>/tmp/calls\n' "$(basename "$1")" >"$1"
    chmod +x "$1"
}

setup() {
    rm -rf /mnt/onboard /usr/local/Kobo /stubs /tmp/el-book.lock /tmp/el-book-launch.sh /tmp/calls /tmp/nickel-hardware-status
    mkdir -p "${EB_DIR}" /usr/local/Kobo /etc/init.d /stubs
    cp "${SRC}/launch.sh" "${EB_DIR}/launch.sh"
    for s in /usr/local/Kobo/nickel /usr/local/Kobo/hindenburg /etc/init.d/on-animator.sh \
        /stubs/poweroff /stubs/reboot /stubs/udevadm; do
        stub "$s"
    done
    : >/tmp/calls
    if [ -n "$1" ]; then
        printf '#!/bin/sh\necho "el-book $*" >>/tmp/calls\nexit %s\n' "$1" >"${EB_DIR}/el-book"
        chmod +x "${EB_DIR}/el-book"
    fi
}

launch() {
    env PATH="/stubs:${PATH}" PLATFORM="${PLATFORM:-mx6sll-ntx}" PRODUCT=frost \
        sh "${EB_DIR}/launch.sh"
    rc=$?
    sleep 1 # фоновые nickel и hindenburg
    return "${rc}"
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

# Выход в Nickel и падение: Nickel возвращается, FIFO пересоздан, блокировка снята.
for code in 0 70 2; do
    setup "${code}"
    launch
    check has "el-book -root ${EB_DIR} -log ${EB_DIR}/logs/el-book.log" "код ${code}"
    check has "on-animator.sh" "код ${code}"
    check has "hindenburg" "код ${code}"
    check has "nickel -platform kobo -skipFontLoad" "код ${code}"
    check has "udevadm trigger" "код ${code}"
    check no "poweroff" "код ${code}"
    [ -p /tmp/nickel-hardware-status ] || { echo "НЕ ПРОШЛО: код ${code}: нет FIFO"; failed=1; }
    [ -d /tmp/el-book.lock ] && { echo "НЕ ПРОШЛО: код ${code}: блокировка осталась"; failed=1; }
    [ -f /tmp/el-book-launch.sh ] || { echo "НЕ ПРОШЛО: код ${code}: нет копии в /tmp"; failed=1; }
done

# Выключение и перезагрузка: Nickel не запускается.
setup 10
launch
check has "poweroff -f" "код 10"
check no "nickel -platform" "код 10"
setup 11
launch
check has "reboot" "код 11"
check no "nickel -platform" "код 11"

# Нет оболочки: Nickel не трогаем вовсе.
setup ""
launch && { echo "НЕ ПРОШЛО: без оболочки скрипт вернул 0"; failed=1; }
check no "nickel" "без оболочки"
grep -q "Nickel не останавливаем" "${EB_DIR}/logs/launch.log" || { echo "НЕ ПРОШЛО: без оболочки нет записи в журнале"; failed=1; }

# Повторный запуск при живой блокировке ничего не делает.
setup 0
mkdir /tmp/el-book.lock
launch
check no "el-book -root" "второй запуск"

# На платформе freescale udevadm не нужен.
setup 0
PLATFORM=freescale launch
check no "udevadm" "freescale"

if [ "${failed}" -ne 0 ]; then
    echo "launch.sh: есть ошибки"
    exit 1
fi
echo "launch.sh: все проверки прошли"
