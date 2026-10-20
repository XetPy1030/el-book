# Устройство Kobo Forma — снимок 19.10.2026

**Тип:** справочник · **Аудитория:** разработчики платформы и приложений, ИИ-агенты · **Статус документа:** снимок · **Снято:** 19.10.2026

**Когда читать:** нужен точный параметр железа или прошивки книги владельца — путь в sysfs, код кнопки, режим обновления экрана, расположение файлов.

---

Снимок собран из бэкапа корня пользовательского раздела книги (`backup/` в корне репозитория,
в git не попадает) и из исходников проектов, которые на ней уже работают. Источник у каждой
строки свой — он указан в колонке «Откуда». Чего в бэкапе нет, собрано в конце, в разделе
[«Что не удалось определить»](#что-не-удалось-определить) — это закрывает [сбор данных с устройства](../guides/Сбор%20данных%20с%20устройства.md).

Снимок не обновляют: свежие данные оформляются новым снимком, а этот уезжает в `docs/archive/`.

## Модель и прошивка

| Параметр | Значение | Откуда |
|---|---|---|
| Модель | Kobo Forma, 8 ГБ | `backup/driveinfo.calibre` (`device_name`); номер модели ниже |
| Номер модели | 377 (у Forma 32 ГБ — 380) | `backup/.kobo/version`, шестое поле |
| Кодовое имя (`PRODUCT`) | `frost` | журнал KFMon, `backup/.adds/kfmon/log/kfmon_dump.log` |
| Поколение платформы | Mark 7 | [FBInk → `DEVICE_KOBO_FORMA`](https://github.com/NiLuJe/FBInk/blob/886f25f13368859ad8a899b88d04c26e19cda32e/fbink_device_id.c) |
| `PLATFORM` | `mx6sll-ntx` | журнал KFMon |
| Прошивка (Nickel) | 4.38.23171 | `backup/.kobo/version`, третье поле |
| Ядро | Linux 4.1.15 | `backup/.kobo/version`, второе поле; журнал KFMon |
| Процессор | NXP i.MX 6SLL, одно ядро Cortex-A9, около 1 ГГц | `PLATFORM`; частота — [MobileRead Wiki](https://wiki.mobileread.com/wiki/Kobo_Forma) |
| ОЗУ | 512 МБ | [MobileRead Wiki](https://wiki.mobileread.com/wiki/Kobo_Forma), на устройстве не проверено |
| Регион Wi-Fi | `RU` | `backup/.kobo/Kobo/Kobo eReader.conf` → `WifiRegulatoryDomain` |
| Стилус | не поддерживается: экран ёмкостный, только палец | спецификация модели |

Сторонние бинарники на книге — 32-битные ARM EABI5 с динамическим glibc (`file` по бинарникам
KOReader и Plato в бэкапе). Статический Go-бинарник под `GOARCH=arm GOARM=7` собирается на
хосте без cgo — проверено 19.10.2026 с Go 1.25.2.

## Экран

| Параметр | Значение | Откуда |
|---|---|---|
| Разрешение | 1440 × 1920, портрет | журнал KOReader `backup/.adds/koreader/crash.log`; журнал KFMon |
| Плотность | 300 dpi | [KOReader → `KoboFrost`](https://github.com/koreader/koreader/blob/v2025.04/frontend/device/kobo/device.lua) (`display_dpi`) |
| Фреймбуфер | `/dev/fb0`, драйвер `mxc_epdc_fb` | журнал KFMon |
| Формат под Nickel | 32 bpp, BGRA, строка 5760 байт | журнал KFMon |
| Формат в 16 bpp | BGR565, строка 2880 байт | журнал KFMon (после смены глубины сторонним приложением) |
| Объём памяти фреймбуфера | 11 796 480 байт (виртуальная высота больше видимой) | журнал KFMon |
| Поворот, читаемый в портрете | 3 (`FB_ROTATE_CCW`), канонический — 0 | журнал KFMon |
| Квирк поворота | нечётные значения при записи инвертированы (`NTX_ROTA_ODD_INVERTED`) | [FBInk → `DEVICE_KOBO_FORMA`](https://github.com/NiLuJe/FBInk/blob/886f25f13368859ad8a899b88d04c26e19cda32e/fbink_device_id.c) |
| Карта поворотов (канон → запись) | UR → 3, CW → 2, UD → 1, CCW → 0 | там же, `rotationMap` |
| Квирк поворота по Plato | читается `n`, записывать `(4 − n) mod 4`; зеркалирование тача `(2, −1)` | [Plato → `transformed_rotation()`, `mirroring_scheme()`](https://github.com/baskerville/plato/blob/7da89f296e58c7426692abb6fda16431b9631e60/crates/core/src/device.rs) |

### Обновление e-ink (Mark 7)

| Что | Значение | Откуда |
|---|---|---|
| ioctl обновления | `MXCFB_SEND_UPDATE_V2`, структура `mxcfb_update_data_v2` | [FBInk → `refresh_kobo_mk7()`](https://github.com/NiLuJe/FBInk/blob/886f25f13368859ad8a899b88d04c26e19cda32e/fbink.c) |
| ioctl ожидания | `MXCFB_WAIT_FOR_UPDATE_COMPLETE_V3`, структура `mxcfb_update_marker_data` | [FBInk → `wait_for_complete_kobo_mk7()`](https://github.com/NiLuJe/FBInk/blob/886f25f13368859ad8a899b88d04c26e19cda32e/fbink.c) |
| Режим обновления | `UPDATE_MODE_PARTIAL` = 0, `UPDATE_MODE_FULL` = 1 | [FBInk → `eink/mxcfb-kobo.h`](https://github.com/NiLuJe/FBInk/blob/886f25f13368859ad8a899b88d04c26e19cda32e/eink/mxcfb-kobo.h) |
| Waveform-режимы NTX | INIT 0, DU 1, GC16 2, GC4 3, A2 4, GL16 5, GLR16 (REAGL) 6, GLD16 7, AUTO 257 | там же, `NTX_WFM_MODE_*`, `WAVEFORM_MODE_AUTO` |
| Известные квирки Mk7 | драйвер может игнорировать флаги и `dither_mode`; `FORCE_MONOCHROME` вне A2/DU провоцирует сбой; сдвиг невыровненных частичных обновлений при аппаратном дизеринге | комментарии в [FBInk → `refresh_kobo_mk7()`](https://github.com/NiLuJe/FBInk/blob/886f25f13368859ad8a899b88d04c26e19cda32e/fbink.c) |

Что эти режимы значат и как их выбирать — в [«Экран e-ink и ввод на Kobo»](../explanation/Экран%20e-ink%20и%20ввод%20на%20Kobo.md).

## Ввод

| Устройство | Имя | Что даёт | Откуда |
|---|---|---|---|
| `/dev/input/event0` | `gpio-keys` | кнопка питания, датчик обложки, кнопки листания, события поворота | `backup/.adds/koreader/crash.log` (сканирование FBInk) |
| `/dev/input/event1` | `cyttsp5_mt` | ёмкостный тачскрин, multitouch protocol B | там же; [Plato → `TouchProto::MultiB`](https://github.com/baskerville/plato/blob/7da89f296e58c7426692abb6fda16431b9631e60/crates/core/src/device.rs) |
| `/dev/input/event2` | `FreescaleAccelerometer` | ориентация устройства | там же |

| Код `EV_KEY` | Значение | Откуда |
|---|---|---|
| 116 | кнопка питания | [KOReader → `event_map`](https://github.com/koreader/koreader/blob/v2025.04/frontend/device/kobo/device.lua) |
| 59 | обложка (датчик Холла) | там же |
| 193 | кнопка листания «назад» (`RPgBack`) | там же |
| 194 | кнопка листания «вперёд» (`RPgFwd`) | там же |

Тач на Forma KOReader обрабатывает в режиме `touch_snow_protocol` (особенности
`ABS_MT_TRACKING_ID`). Ориентацию акселерометр сообщает событием `EV_MSC`/`MSC_RAW` со значениями
0x17 (портрет, вниз), 0x18 (портрет), 0x19 (альбом, вправо), 0x1a (альбом, влево) —
[KOReader → `gyroTranslation()`](https://github.com/koreader/koreader/blob/v2025.04/frontend/device/kobo/device.lua).

## Подсветка, питание, события железа

| Что | Значение | Откуда |
|---|---|---|
| Яркость | `/sys/class/backlight/mxc_msp430.0/brightness` | [KOReader → `KoboFrost`](https://github.com/koreader/koreader/blob/v2025.04/frontend/device/kobo/device.lua) |
| Теплота | `/sys/class/backlight/tlc5947_bl/color`, шкала 0–10, инвертирована | там же |
| Светодиод зарядки | управляем (`canToggleChargingLED`) | там же |
| Сон | `1` в `/sys/power/state-extended` (гасит тач), `sync`, `mem` в `/sys/power/state`; после пробуждения — `0` в `state-extended` | `backup/.adds/plato/scripts/suspend.sh`, `resume.sh` |
| События USB и сети | FIFO `/tmp/nickel-hardware-status`, строки `usb plug add`/`remove`, `usb ac add`/`remove`, `network bound …`; пишет udev | [Plato → `parse_usb_events()`](https://github.com/baskerville/plato/blob/7da89f296e58c7426692abb6fda16431b9631e60/crates/core/src/input.rs) |
| USB mass storage | раздел `/dev/mmcblk0p3`, на время USB размонтируется | `backup/.adds/plato/scripts/usb-enable.sh` |
| Wi-Fi | модуль из `WIFI_MODULE`, интерфейс из `INTERFACE` (переменные окружения Nickel); `wpa_supplicant` с `/etc/wpa_supplicant/wpa_supplicant.conf`, `udhcpc` | `backup/.adds/plato/scripts/wifi-enable.sh` |
| Настройки Nickel | яркость 1, автотеплота включена, 24-часовые часы, язык `en` | `backup/.kobo/Kobo/Kobo eReader.conf` → `[PowerOptions]`, `[ApplicationPreferences]` |

## Файлы на пользовательском разделе

Пользовательский раздел смонтирован в `/mnt/onboard` и виден компьютеру по USB.

| Путь | Что там |
|---|---|
| `/mnt/onboard/.kobo/` | данные Nickel: база `KoboReader.sqlite`, конфиги `Kobo/*.conf`, папка установки `KoboRoot.tgz` |
| `/mnt/onboard/.adds/` | сторонние приложения и их конфиги |
| `/mnt/onboard/.adds/nm/` | конфиги NickelMenu: `_umr_conf` (пункт UltimateMangaReader), `kfmon` (генератор пунктов KFMon) |
| `/mnt/onboard/.adds/kfmon/config/` | KFMon: иконки `koreader.png`, `icons/plato.png`, `vlasovsoft.png`, `kfmon.png` → скрипты запуска |
| `/mnt/onboard/Сборник Книг/` | книги владельца (в бэкап не вошли) |

**Секреты.** В бэкапе есть данные, которые нельзя публиковать и коммитить: токен и логин
Dropbox, идентификаторы Google Drive (`.kobo/Kobo/Kobo eReader.conf`), идентификатор аналитики
и история чтения (`.kobo/Kobo/Analytics.conf`), база библиотеки (`.kobo/KoboReader.sqlite`).
Правило обращения — в [«Правилах работы с устройством»](../rules/Правила%20работы%20с%20устройством.md).

## Установленные моды

| Мод | Версия | Как запускается | Откуда |
|---|---|---|---|
| KOReader | v2025.04 | KFMon (`koreader.png`) | `backup/.adds/koreader/version.log` |
| Plato | не определена | KFMon (`icons/plato.png`) | `backup/.adds/plato/` |
| vlasovsoft (набор Qt-приложений) | 2022.5.19 | KFMon (`vlasovsoft.png`) | `backup/.adds/vlasovsoft/VERSION` |
| UltimateMangaReader + Qt 5.15 | не определена | NickelMenu, `cmd_spawn` | `backup/.adds/nm/_umr_conf` |
| KFMon | не определена | демон, следит за открытием PNG-иконок | `backup/.adds/kfmon/` |
| NickelMenu | не определена (сама библиотека живёт в системном разделе, в бэкап не входит) | меню Nickel | `backup/.adds/nm/` |

## Что не удалось определить

Из бэкапа нельзя узнать — снимается [сбором данных с устройства](../guides/Сбор%20данных%20с%20устройства.md):

- точный объём ОЗУ и свободной памяти, частоты и губернатор процессора;
- файловая система и параметры монтирования `/mnt/onboard`;
- значения `WIFI_MODULE`, `INTERFACE`, `PLATFORM`, `PRODUCT` у текущего Nickel;
- путь батареи в `/sys/class/power_supply/` и диапазоны подсветки (`max_brightness`);
- версии NickelMenu и KFMon, версия glibc и набор апплетов busybox;
- параметры `ABS_MT_*` тачскрина (диапазоны координат) из `/proc/bus/input/devices`.
