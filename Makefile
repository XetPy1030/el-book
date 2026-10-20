# Команды проекта. Порядок работы — docs/guides/Настройка окружения разработки.md.

# Откуда doccheck читает документацию: head (коммит), index (хук перед коммитом) или worktree.
DOCS_SRC ?= head

# Версия в бинарнике: тег или коммит; до первого коммита — дата.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev-$$(date +%Y%m%d))

# Сборка для книги: Kobo Forma — 32-битный ARMv7, без cgo.
KOBO_ENV := CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7
KOBO_LDFLAGS := -s -w -X main.version=$(VERSION)

# Куда macOS монтирует книгу по USB.
KOBO_VOLUME ?= /Volumes/KOBOeReader
# На macOS cp -X не тащит расширенные атрибуты исходника. Атрибут com.apple.provenance macOS
# всё равно ставит на каждый новый файл, и на FAT он ложится в соседний ._<имя>, поэтому
# deploy-usb после копирования удаляет ._-файлы своих файлов и своей папки.
CP := cp $(if $(filter Darwin,$(shell uname -s)),-X,)

.PHONY: check vet test archcheck docs-check hooks vet-kobo test-kobo build-kobo package deploy-usb \
	emu shots uitest uitest-update test-scripts

# Всё, что должно быть зелёным перед коммитом.
check: vet vet-kobo test archcheck docs-check

vet:
	go vet ./...

# Код под книгу (файлы *_linux_arm.go) компьютерный vet не видит.
vet-kobo:
	$(KOBO_ENV) go vet ./...

test:
	go test ./...

# Правила кода, которые проверяются по исходникам: cgo, слои, код под Kobo, go, запись файлов.
archcheck:
	go run ./tools/archcheck

docs-check:
	go run ./tools/doccheck -src=$(DOCS_SRC)

# Подключает хуки из .githooks: перед коммитом проверяется документация из индекса.
hooks:
	git config core.hooksPath .githooks

# Тесты слоя Kobo на 32-битном ARM: размеры структур ioctl как на книге. Нужен Docker.
test-kobo:
	$(KOBO_ENV) go test -c -o build/kobo/kobo.test ./sdk/device/kobo
	docker run --rm --platform linux/arm/v7 -v "$(CURDIR)/build/kobo:/w" busybox:1.36 /w/kobo.test

# Скрипты книги: launch.sh и wifi.sh с заглушками в контейнере busybox и shellcheck. Нужен Docker.
test-scripts:
	docker run --rm -v "$(CURDIR)/scripts/device:/src:ro" busybox:1.36 sh /src/test/launch_test.sh
	docker run --rm -v "$(CURDIR)/scripts/device:/src:ro" busybox:1.36 sh /src/test/wifi_test.sh
	docker run --rm -v "$(CURDIR)/scripts/device:/mnt:ro" koalaman/shellcheck:v0.10.0 -s sh \
		/mnt/launch.sh /mnt/probe.sh /mnt/wifi.sh /mnt/test/launch_test.sh /mnt/test/wifi_test.sh

# Оболочка для книги: build/kobo/el-book.
build-kobo:
	$(KOBO_ENV) go build -trimpath -ldflags "$(KOBO_LDFLAGS)" -o build/kobo/el-book ./cmd/el-book

# Архив для установки: dist/el-book-<версия>.zip, внутри папка .adds как в корне книги.
package: build-kobo
	rm -rf build/package
	mkdir -p build/package/.adds/el-book build/package/.adds/nm dist
	cp build/kobo/el-book scripts/device/launch.sh scripts/device/probe.sh scripts/device/wifi.sh build/package/.adds/el-book/
	cp scripts/device/nm/el-book build/package/.adds/nm/el-book
	rm -f dist/el-book-$(VERSION).zip
	cd build/package && zip -qr ../../dist/el-book-$(VERSION).zip .adds
	@echo dist/el-book-$(VERSION).zip

# Установка на книгу, подключённую по USB (в Nickel нажать «Connect»).
deploy-usb: build-kobo
	@test -d "$(KOBO_VOLUME)/.adds" || { echo "книга не подключена: нет $(KOBO_VOLUME)/.adds"; exit 1; }
	mkdir -p "$(KOBO_VOLUME)/.adds/el-book" "$(KOBO_VOLUME)/.adds/nm"
	$(CP) build/kobo/el-book scripts/device/launch.sh scripts/device/probe.sh scripts/device/wifi.sh "$(KOBO_VOLUME)/.adds/el-book/"
	$(CP) scripts/device/nm/el-book "$(KOBO_VOLUME)/.adds/nm/el-book"
	cd "$(KOBO_VOLUME)/.adds" && rm -f ._el-book el-book/._* nm/._el-book
	sync
	@echo "Готово: извлеките книгу и откройте NickelMenu → el-book."

# Эмулятор в браузере: http://localhost:8740.
emu:
	go run ./cmd/emulator

# Снимки экранов по сценарию smoke в build/shots.
shots:
	go run ./cmd/emulator -fresh -data build/shots-data -clock 2026-10-19T14:05 \
		-script emulator/scripts/smoke.txt -out build/shots

uitest:
	go test ./internal/uitest

# Перезаписать эталоны UI-тестов после осознанного изменения интерфейса.
uitest-update:
	go test ./internal/uitest -update
