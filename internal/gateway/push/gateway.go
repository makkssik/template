package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	api "github.com/makkssik/template/internal/generated/push"
)

type Gateway struct {
	client    *http.Client
	transport *http.Transport
	endpoint  string
}

type Config struct {
	URL             string
	RequestTimeout  time.Duration
	KeepAlive       time.Duration
	IdleConnTimeout time.Duration
}

func New(cfg Config, tp trace.TracerProvider) (*Gateway, error) {
	endpoint, err := pushEndpoint(cfg.URL)
	if err != nil {
		return nil, err
	}
	if cfg.RequestTimeout <= 0 || cfg.KeepAlive <= 0 || cfg.IdleConnTimeout <= 0 {
		return nil, errors.New("push timeouts must be positive")
	}

	dialer := &net.Dialer{
		Timeout:   cfg.RequestTimeout,
		KeepAlive: cfg.KeepAlive,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   cfg.RequestTimeout,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
	client := &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: otelhttp.NewTransport(
			transport,
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithPropagators(propagation.TraceContext{}),
		),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Gateway{
		client:    client,
		transport: transport,
		endpoint:  endpoint,
	}, nil
}

func pushEndpoint(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return "", errors.New("PUSH_HTTP_URL must be an absolute URL with a host")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("PUSH_HTTP_URL must use HTTP or HTTPS")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("PUSH_HTTP_URL must not contain credentials, query or fragment")
	}
	return u.JoinPath("api/v1/push").String(), nil
}

func (g *Gateway) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	data := map[string]string{"trip_id": tripID.String()}
	body, err := json.Marshal(api.PushRequest{
		RequestId:   uuid.NewString(),
		RecipientId: driverID,
		Kind:        api.REQUESTPOSITION,
		Data:        &data,
	})
	if err != nil {
		return fmt.Errorf("encode push: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("push transport: %w", err)
	}
	defer response.Body.Close()
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode != http.StatusAccepted {
		return &StatusError{Status: response.StatusCode}
	}
	if readErr != nil {
		return fmt.Errorf("read push response: %w", readErr)
	}
	return nil
}

func (g *Gateway) Close() {
	g.transport.CloseIdleConnections()
}

type StatusError struct {
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service returned HTTP %d", e.Status)
}
