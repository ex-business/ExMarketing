package fact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     secureTransport(),
	}}
}

func (c *Client) Query(ctx context.Context, request marketing.FactQueryRequest) (marketing.FactQueryResponse, error) {
	if request.Provider.Provider != "HTTP" {
		return marketing.FactQueryResponse{}, errors.New("only HTTP fact provider is supported")
	}
	if err := validateEndpoint(request.Provider.Endpoint); err != nil {
		return marketing.FactQueryResponse{}, err
	}
	body, err := json.Marshal(request.Event)
	if err != nil {
		return marketing.FactQueryResponse{}, err
	}
	method := request.Provider.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, request.Provider.Endpoint, bytes.NewReader(body))
	if err != nil {
		return marketing.FactQueryResponse{}, err
	}
	req.Header.Set("content-type", "application/json")
	for key, value := range request.Provider.Headers {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return marketing.FactQueryResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return marketing.FactQueryResponse{}, errors.New("fact provider returned non-2xx status")
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return marketing.FactQueryResponse{}, err
	}
	return marketing.FactQueryResponse{FactCode: request.Provider.Code, Data: data}, nil
}

func validateEndpoint(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("fact endpoint must use http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("fact endpoint host is required")
	}
	if !hostAllowed(host) {
		return errors.New("fact endpoint host is not allowlisted")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if isPrivateOrLocalIP(ip) {
			return errors.New("fact endpoint resolves to private or local address")
		}
	}
	return nil
}

func hostAllowed(host string) bool {
	allowlist := os.Getenv("EX_MARKETING_FACT_ALLOWLIST")
	if allowlist == "" {
		return false
	}
	for _, allowed := range strings.Split(allowlist, ",") {
		allowed = strings.TrimSpace(strings.ToLower(allowed))
		if allowed == "" {
			continue
		}
		host = strings.ToLower(host)
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func isPrivateOrLocalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func secureTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if isPrivateOrLocalIP(ip) {
				return nil, errors.New("fact endpoint resolves to private or local address")
			}
		}
		if len(ips) == 0 {
			return nil, errors.New("fact endpoint did not resolve")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		return nil, nil
	}
	return transport
}
