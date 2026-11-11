package terabox

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"
)

// MockRoundTripper implements http.RoundTripper
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) *http.Response
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req), nil
}

func TestNewClient(t *testing.T) {
	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) *http.Response {
				if req.URL.Path == "/main" {
					return &http.Response{
						StatusCode: 200,
						Body: io.NopCloser(bytes.NewBufferString(
							`"bdstoken":"mock_bds_token" window.jsToken%20%3D%20a%7D%3Bfn%28%22mock_js_token`,
						)),
						Header: make(http.Header),
					}
				}
				return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewBufferString(""))}
			},
		},
	}
	client, err := NewClient("ndus=test_cookie", mockClient)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Client is nil")
	}
}

func TestCookieDomain(t *testing.T) {
	cases := map[string]string{
		"www.terabox.com": "terabox.com",
		"dm.terabox.com":  "terabox.com",
		"dm.nephobox.com": "nephobox.com",
		"terabox.com":     "terabox.com",
		"localhost":       "localhost",
		"127.0.0.1":       "127.0.0.1",
	}
	for host, want := range cases {
		if got := cookieDomain(host); got != want {
			t.Errorf("cookieDomain(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestNewClientInvalidBaseURL(t *testing.T) {
	if _, err := NewClientWithBaseURL("ndus=test", "://bad", nil); err == nil {
		t.Fatal("expected an error for an invalid base URL")
	}
}

func TestNewClientExpiredCookie(t *testing.T) {
	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) *http.Response {
				if req.URL.Path == "/main" {
					// Logged-out page: bdstoken is present but empty.
					return &http.Response{
						StatusCode: 200,
						Body: io.NopCloser(bytes.NewBufferString(
							`var templateData = {"bdstoken":"","pcftoken":"x"};`,
						)),
						Header: make(http.Header),
					}
				}
				return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewBufferString(""))}
			},
		},
	}

	_, err := NewClient("ndus=expired", mockClient)
	if !errors.Is(err, ErrCookieExpired) {
		t.Fatalf("expected ErrCookieExpired, got: %v", err)
	}
}

func TestGetHomeInfo(t *testing.T) {
	// Mock response body
	mockResponse := `{"errno":0,"errmsg":"","data":{"username":"testuser","uk":"12345","sign1":"s1","timestamp":1234567890,"sign3":"s3"}}`

	// Create a mock HTTP client
	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) *http.Response {
				if req.URL.Path == "/main" {
					return &http.Response{
						StatusCode: 200,
						Body: io.NopCloser(bytes.NewBufferString(
							`"bdstoken":"mock_bds_token" window.jsToken%20%3D%20a%7D%3Bfn%28%22mock_js_token`,
						)),
						Header: make(http.Header),
					}
				}
				// Verify request URL
				if req.URL.Path != "/api/home/info" {
					t.Errorf("Expected path /api/home/info, got %s", req.URL.Path)
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
					Header:     make(http.Header),
				}
			},
		},
	}

	client, err := NewClient("ndus=test_cookie", mockClient)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Inject tokens manually since we are not testing login flow here
	client.jsToken = "test_js_token"

	info, err := client.getHomeInfo()
	if err != nil {
		t.Fatalf("getHomeInfo failed: %v", err)
	}

	if info.Errno != 0 {
		t.Errorf("Expected errno 0, got %d", info.Errno)
	}
	if info.Data.Username != "testuser" {
		t.Errorf("Expected username testuser, got %s", info.Data.Username)
	}
}
