package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type TestEnv struct {
	NATSURL    string
	DBDSN      string
	APIServerURL string
}

func LoadTestEnv() *TestEnv {
	return &TestEnv{
		NATSURL:    getEnvOrDefault("NATS_URL", "nats://localhost:4222"),
		DBDSN:      getEnvOrDefault("HNB_TEST_POSTGRES_DSN", "host=127.0.0.1 port=5432 dbname=hnb_test user=postgres password=test123 sslmode=disable"),
		APIServerURL: getEnvOrDefault("HNB_API_URL", "http://localhost:8080"),
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type NATSTestHelper struct {
	NC  *nats.Conn
	JS  jetstream.JetStream
	Ctx context.Context
}

func NewNATSTestHelper(url string) (*NATSTestHelper, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	return &NATSTestHelper{
		NC:  nc,
		JS:  js,
		Ctx: context.Background(),
	}, nil
}

func (h *NATSTestHelper) PublishAndWait(subject string, payload any, responseSubject string, timeout time.Duration) (*nats.Msg, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	msg, err := h.NC.Request(subject, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}

	return msg, nil
}

func (h *NATSTestHelper) PublishJetStream(subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	_, err = h.JS.Publish(h.Ctx, subject, data)
	return err
}

func (h *NATSTestHelper) SubscribeAndCollect(subject string, count int, timeout time.Duration) ([]*nats.Msg, error) {
	msgs := make(chan *nats.Msg, count)
	sub, err := h.NC.Subscribe(subject, func(msg *nats.Msg) {
		msgs <- msg
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	defer sub.Unsubscribe()

	var result []*nats.Msg
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for i := 0; i < count; i++ {
		select {
		case msg := <-msgs:
			result = append(result, msg)
		case <-timer.C:
			return result, fmt.Errorf("timeout after %d/%d messages", len(result), count)
		}
	}

	return result, nil
}

func (h *NATSTestHelper) Close() {
	h.NC.Close()
}

type APITestHelper struct {
	BaseURL string
	Client  *http.Client
}

func NewAPITestHelper(baseURL string) *APITestHelper {
	return &APITestHelper{
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *APITestHelper) Do(method, path string, body any, token string) (*http.Response, error) {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
	}

	req, err := http.NewRequest(method, h.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	if body != nil {
		req.Body = http.NoBody
		req.ContentLength = int64(len(reqBody))
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	return h.Client.Do(req)
}

func (h *APITestHelper) Login(username, password string) (string, error) {
	body := map[string]string{"username": username, "password": password}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", h.BaseURL+"/api/v1/auth/login", nil)
	if err != nil {
		return "", err
	}
	req.Body = http.NoBody
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	token, _ := result["access_token"].(string)
	return token, nil
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}