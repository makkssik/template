package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	api "github.com/makkssik/template/internal/generated/push"
)

func TestPushContractAndStatus(t *testing.T) {
	statuses := []int{
		http.StatusAccepted,
		http.StatusBadRequest,
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusFound,
	}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			driver, trip := uuid.New(), uuid.New()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				defer r.Body.Close()
				var body api.PushRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.URL.Path != "/api/v1/push" || body.Kind != api.REQUESTPOSITION || body.RecipientId != driver || body.RequestId == "" || body.Data == nil || (*body.Data)["trip_id"] != trip.String() {
					t.Errorf("invalid request %+v", body)
				}
				if r.Header.Get("traceparent") == "" {
					t.Error("missing traceparent")
				}
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(status)
			}))
			defer server.Close()
			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer func() {
				if err := tp.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			}()
			gateway, err := New(Config{
				URL:             server.URL,
				RequestTimeout:  time.Second,
				KeepAlive:       30 * time.Second,
				IdleConnTimeout: 90 * time.Second,
			}, tp)
			if err != nil {
				t.Fatal(err)
			}
			defer gateway.Close()
			ctx, span := tp.Tracer("test").Start(t.Context(), "worker")
			err = gateway.RequestPosition(ctx, driver, trip)
			span.End()
			if status == http.StatusAccepted && err != nil {
				t.Fatal(err)
			}
			if status != http.StatusAccepted {
				var response *StatusError
				if !errors.As(err, &response) || response.Status != status {
					t.Fatalf("wrong status error %v", err)
				}
			}
			if requests != 1 {
				t.Fatal("request retried or followed redirect")
			}
			if len(recorder.Ended()) != 2 {
				t.Fatal("missing HTTP client span")
			}
		})
	}
}

func TestPushTimeout(t *testing.T) {
	exited := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		defer close(exited)
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	gateway, err := New(Config{
		URL:             server.URL,
		RequestTimeout:  50 * time.Millisecond,
		KeepAlive:       30 * time.Second,
		IdleConnTimeout: 90 * time.Second,
	}, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	err = gateway.RequestPosition(t.Context(), uuid.New(), uuid.New())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("HTTP call leaked")
	}
}

func TestInvalidURL(t *testing.T) {
	withCredentials := url.URL{Scheme: "http", Host: "host", User: url.UserPassword("user", "secret")}
	tests := []struct{ name, url string }{
		{name: "empty URL"},
		{name: "unsupported scheme", url: "ftp://host"},
		{name: "embedded credentials", url: withCredentials.String()},
		{name: "query parameters", url: "http://host?a=b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(Config{
				URL:             tc.url,
				RequestTimeout:  time.Second,
				KeepAlive:       30 * time.Second,
				IdleConnTimeout: 90 * time.Second,
			}, noop.NewTracerProvider()); err == nil {
				t.Fatal("invalid URL accepted")
			}
		})
	}
}

func TestCloseReleasesIdleConnection(t *testing.T) {
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	gateway, err := New(Config{
		URL:             server.URL,
		RequestTimeout:  time.Second,
		KeepAlive:       30 * time.Second,
		IdleConnTimeout: 90 * time.Second,
	}, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	if err := gateway.RequestPosition(t.Context(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	gateway.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("idle HTTP connection not closed")
	}
}

func TestIdleConnectionTimeout(t *testing.T) {
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()

	gateway, err := New(Config{
		URL:             server.URL,
		RequestTimeout:  time.Second,
		KeepAlive:       30 * time.Second,
		IdleConnTimeout: 25 * time.Millisecond,
	}, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	if err := gateway.RequestPosition(t.Context(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("idle HTTP connection outlived configured timeout")
	}
}
