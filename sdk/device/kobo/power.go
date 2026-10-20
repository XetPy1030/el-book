package kobo

import (
	"errors"
	"fmt"
	"log/slog"
	"syscall"
	"time"

	"el-book/sdk/device"
)

// power — сон по последовательности KOReader: state-extended=1 гасит тач и периферию,
// через 2 с sync и mem в /sys/power/state (запись возвращается после пробуждения), затем
// state-extended=0 и пауза 100 мс, пока периферия просыпается.
//
// Драйвер экрана не даёт уснуть сразу после обновления: mxc_epdc_fb_suspend в ядре Forma
// отвечает EPERM, пока панель под питанием, и ENOENT, пока после её выключения не прошло
// 900 тиков (9 с при HZ=100). Поэтому первая попытка ждёт settle от последнего обновления
// экрана, а такие отказы повторяются. KOReader по той же причине ждёт 15 с.
//
// Кнопка питания, открытая обложка или кабель компьютера (их присылает wake), пришедшие до
// засыпания, отменяют сон: Suspend возвращает device.ErrSuspendCancelled.
//
// Если после пробуждения за wakeWait не пришло законного события, пробуждение ложное
// (зарядка, сбой): книга засыпает снова, не перерисовывая экран; после maxSpurious ложных
// пробуждений подряд Suspend всё же возвращается, чтобы книга не уснула навсегда, если
// ввод сломался.
//
// wakeup_count не сверяется: KOReader отключил его, потому что на части книг после
// успешной сверки книга сразу просыпается.
type power struct {
	fs    sysWriter
	sync  func()
	now   func() time.Time
	shown func() time.Time // когда экран обновлялся последний раз
	wake  <-chan struct{}  // nil — без отмены и без защиты от ложных пробуждений
	log   *slog.Logger
	t     powerTiming
}

// powerTiming — паузы сна; тесты их укорачивают.
type powerTiming struct {
	settle    time.Duration // от обновления экрана до записи mem
	lead      time.Duration // state-extended=1 раньше mem
	retry     time.Duration // пауза после отказа ядра
	attempts  int           // попыток до ошибки
	afterWake time.Duration // периферия просыпается после state-extended=0
	wakeWait  time.Duration // сколько ждать законного события после пробуждения
}

func defaultTiming(settle time.Duration) powerTiming {
	if settle <= 0 {
		settle = defaultSuspendSettle
	}
	return powerTiming{
		settle: settle, lead: 2 * time.Second, retry: 2 * time.Second, attempts: 6,
		afterWake: 100 * time.Millisecond, wakeWait: 2 * time.Second,
	}
}

// sysWriter — запись в sysfs; тесты подменяют её, чтобы ядро «отказывало».
type sysWriter interface {
	write(p, value string) error
}

const (
	statePath         = "/sys/power/state"
	stateExtendedPath = "/sys/power/state-extended"
	maxSpurious       = 10
)

var errWokenBeforeSleep = fmt.Errorf("%w: книгу разбудили до сна", device.ErrSuspendCancelled)

func (p power) Suspend() error {
	p.drainWake()
	for spurious := 0; ; spurious++ {
		if err := p.enter(); err != nil {
			return err
		}
		if p.wake == nil || spurious >= maxSpurious || p.waitWake() {
			return nil
		}
		p.log.Info("ложное пробуждение, засыпаем снова", "подряд", spurious+1)
	}
}

// enter усыпляет книгу и возвращается после пробуждения, отмены или отказа ядра.
func (p power) enter() error {
	var err error
	for attempt := 1; attempt <= p.t.attempts; attempt++ {
		wait := p.t.settle - p.now().Sub(p.shown()) - p.t.lead
		if attempt > 1 {
			wait = max(wait, p.t.retry)
		}
		if !p.pause(wait) {
			return errWokenBeforeSleep
		}
		if err := p.fs.write(stateExtendedPath, "1"); err != nil {
			return fmt.Errorf("ядро не перевело периферию в сон: %w", err)
		}
		if !p.pause(p.t.lead) {
			p.wakeUp()
			return errWokenBeforeSleep
		}
		p.sync()
		before := p.now()
		err = p.fs.write(statePath, "mem")
		after := p.now()
		p.wakeUp()
		if err == nil {
			// Монотонные часы во сне стоят, настенные идут: разница — сколько книга спала.
			slept := after.Round(0).Sub(before.Round(0)) - after.Sub(before)
			p.log.Info("сон", "попытка", attempt, "спала", slept.Round(time.Second))
			return nil
		}
		if !kernelBusy(err) {
			break
		}
		p.log.Warn("ядро не усыпило книгу, повтор", "попытка", attempt, "err", err)
	}
	return fmt.Errorf("ядро отказалось усыпить устройство: %w", err)
}

// kernelBusy — отказ, после которого стоит попробовать ещё: панель под питанием или не
// остыла (EPERM, ENOENT, ESRCH у драйвера экрана), устройство занято или пришло событие
// пробуждения (EBUSY, EAGAIN).
func kernelBusy(err error) bool {
	for _, e := range []syscall.Errno{syscall.EPERM, syscall.ENOENT, syscall.ESRCH, syscall.EBUSY, syscall.EAGAIN} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// pause ждёт d; false — если за это время книгу разбудили.
func (p power) pause(d time.Duration) bool {
	if d <= 0 {
		select {
		case <-p.wake:
			return false
		default:
			return true
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.wake:
		return false
	case <-t.C:
		return true
	}
}

// drainWake забывает события пробуждения, пришедшие до сна: среди них нажатие, которым
// книгу усыпили.
func (p power) drainWake() {
	for {
		select {
		case <-p.wake:
		default:
			return
		}
	}
}

func (p power) waitWake() bool {
	t := time.NewTimer(p.t.wakeWait)
	defer t.Stop()
	select {
	case <-p.wake:
		return true
	case <-t.C:
		return false
	}
}

func (p power) wakeUp() {
	if err := p.fs.write(stateExtendedPath, "0"); err != nil {
		p.log.Warn("ядро не разбудило периферию", "err", err)
	}
	time.Sleep(p.t.afterWake)
}
