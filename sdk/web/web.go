// Package web — HTTP-клиент приложений. Перед запросом он просит платформу включить Wi-Fi,
// ограничивает время и размер ответа, кэширует ответы в кэше приложения и отдаёт сохранённый
// ответ, когда сети нет: на e-ink приложение показывает последние данные сразу, а свежие —
// когда придут. Ошибки переводятся в текст для пользователя (Explain).
//
// Методы блокируются: зовите их из фоновой работы (ctx.Go, kit.Load), не из цикла интерфейса.
package web

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"el-book/sdk/device"
)

// Connector включает сеть и ждёт подключения. Его даёт платформа: приложение Wi-Fi не видит.
type Connector func(ctx context.Context) error

// Cache — где клиент хранит ответы; это кэш приложения из sdk/storage.
type Cache interface {
	ReadCache(name string) ([]byte, time.Time, error)
	WriteCache(name string, data []byte) error
}

// Options — параметры клиента.
type Options struct {
	Connect   Connector         // nil — сеть считается доступной всегда
	Transport http.RoundTripper // nil — http.DefaultTransport; тесты подставляют свой
	Cache     Cache             // nil — без кэша
	UserAgent string
	Timeout   time.Duration // на весь запрос; 0 — 30 с
	MaxBody   int64         // предел ответа; 0 — 8 МБ
	Now       func() time.Time
}

// Client — HTTP-клиент приложения.
type Client struct {
	o    Options
	http *http.Client
}

// New создаёт клиент.
func New(o Options) *Client {
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MaxBody == 0 {
		o.MaxBody = 8 << 20
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	tr := o.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	return &Client{o: o, http: &http.Client{Transport: tr, Timeout: o.Timeout}}
}

// HTTPError — сервер ответил кодом ошибки.
type HTTPError struct {
	Status int
	URL    string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("%s: код ответа %d", e.URL, e.Status) }

// ErrTooLarge — ответ больше допустимого.
var ErrTooLarge = errors.New("ответ сервера слишком большой")

// Do выполняет запрос: включает сеть, ставит User-Agent, проверяет код ответа и читает тело
// не больше MaxBody.
func (c *Client) Do(req *http.Request) ([]byte, error) {
	if c.o.Connect != nil {
		if err := c.o.Connect(req.Context()); err != nil {
			return nil, err
		}
	}
	if c.o.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.o.UserAgent)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &HTTPError{Status: resp.StatusCode, URL: req.URL.Redacted()}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.o.MaxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.o.MaxBody {
		return nil, ErrTooLarge
	}
	return body, nil
}

// Get загружает адрес.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// GetJSON загружает адрес и разбирает JSON в v.
func (c *Client) GetJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// PostJSON отправляет in как JSON и разбирает ответ в out (nil — ответ не нужен).
func (c *Client) PostJSON(ctx context.Context, rawURL string, headers map[string]string, in, out any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	body, err := c.Do(req)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// Result — ответ, возможно из кэша.
type Result struct {
	Body    []byte
	Fetched time.Time // когда ответ получен с сервера
	Stale   bool      // обновить не удалось: это сохранённый ответ, причина — в Err
	Err     error
}

func cacheName(rawURL string) string {
	sum := sha1.Sum([]byte(rawURL))
	return "web/" + hex.EncodeToString(sum[:]) + ".body"
}

// Cached возвращает сохранённый ответ без сети: показать сразу, пока идёт обновление.
func (c *Client) Cached(rawURL string) (Result, bool) {
	if c.o.Cache == nil {
		return Result{}, false
	}
	body, at, err := c.o.Cache.ReadCache(cacheName(rawURL))
	if err != nil {
		return Result{}, false
	}
	return Result{Body: body, Fetched: at}, true
}

// Fetch возвращает ответ не старше maxAge: из кэша или с сервера. Если сервер недоступен,
// а сохранённый ответ есть — отдаёт его со Stale = true и без ошибки.
func (c *Client) Fetch(ctx context.Context, rawURL string, maxAge time.Duration) (Result, error) {
	cached, ok := c.Cached(rawURL)
	if ok && c.o.Now().Sub(cached.Fetched) < maxAge {
		return cached, nil
	}
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		if ok {
			cached.Stale, cached.Err = true, err
			return cached, nil
		}
		return Result{}, err
	}
	if c.o.Cache != nil {
		_ = c.o.Cache.WriteCache(cacheName(rawURL), body) // кэш можно потерять
	}
	return Result{Body: body, Fetched: c.o.Now()}, nil
}

// FetchJSON — Fetch с разбором JSON в v.
func (c *Client) FetchJSON(ctx context.Context, rawURL string, maxAge time.Duration, v any) (Result, error) {
	res, err := c.Fetch(ctx, rawURL, maxAge)
	if err != nil {
		return res, err
	}
	return res, json.Unmarshal(res.Body, v)
}

// URL собирает адрес с параметрами запроса: URL("https://x/api", "q", "Самара", "n", "5").
func URL(base string, kv ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}

// Explain переводит ошибку сети в текст для пользователя: что случилось и что сделать.
func Explain(err error) string {
	var he *HTTPError
	var dns *net.DNSError
	var cert *x509.UnknownAuthorityError
	var certInvalid x509.CertificateInvalidError
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, device.ErrNoKnownNet):
		return "Рядом нет знакомых сетей Wi-Fi. Подключитесь в настройках: Wi-Fi."
	case errors.Is(err, device.ErrWifiDisabled):
		return "Wi-Fi выключен. Включите его в быстрых настройках."
	case errors.Is(err, device.ErrWrongPass):
		return "Неверный пароль сети Wi-Fi."
	case errors.Is(err, device.ErrNoWifi):
		return "На этом устройстве нет Wi-Fi."
	case errors.Is(err, ErrTooLarge):
		return "Сервер прислал слишком много данных."
	case errors.As(err, &he):
		switch {
		case he.Status == http.StatusNotFound:
			return "Сервер не нашёл то, что запрошено."
		case he.Status == http.StatusTooManyRequests:
			return "Сервер просит подождать: слишком много запросов. Попробуйте позже."
		case he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden:
			return "Сервер отказал в доступе: проверьте ключ или вход."
		case he.Status >= 500:
			return fmt.Sprintf("Сервер не справился (код %d). Попробуйте позже.", he.Status)
		}
		return fmt.Sprintf("Сервер ответил ошибкой (код %d).", he.Status)
	case errors.As(err, &dns):
		return "Нет доступа в интернет: адрес сервера не найден."
	case errors.As(err, &cert), errors.As(err, &certInvalid):
		return "Не удалось проверить сертификат сервера. Проверьте дату и время на книге."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, device.ErrNetTimeout),
		errors.As(err, &netErr) && netErr.Timeout():
		return "Сервер не ответил вовремя. Проверьте Wi-Fi и попробуйте ещё раз."
	case errors.Is(err, context.Canceled):
		return "Загрузка отменена."
	}
	return "Не удалось загрузить данные. Проверьте Wi-Fi и попробуйте ещё раз."
}
