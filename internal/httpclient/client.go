package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ResponseError struct {
	StatusCode int
	Body       string
	Payload    any
}

func (e *ResponseError) Error() string {
	if e.Body != "" {
		return e.Body
	}
	return fmt.Sprintf("request failed with status %d", e.StatusCode)
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	Debug      bool
	Stderr     io.Writer
}

func New(baseURL, token string, httpClient *http.Client, debug bool, stderr io.Writer) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTPClient: httpClient, Debug: debug, Stderr: stderr}
}

func (c *Client) DoJSON(ctx context.Context, method, path string, query url.Values, body any) (any, error) {
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	reqURL := c.BaseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	attempts := 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, err
		}
		c.applyHeaders(req)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		payload, readErr := readPayload(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		c.debugf("%s %s -> %d", method, req.URL.Path, resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return payload, nil
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < attempts {
			c.debugf("retrying %s %s after status %d (attempt %d/%d)", method, req.URL.Path, resp.StatusCode, attempt, attempts)
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			lastErr = &ResponseError{StatusCode: resp.StatusCode, Body: stringifyPayload(payload), Payload: payload}
			continue
		}
		return nil, &ResponseError{StatusCode: resp.StatusCode, Body: stringifyPayload(payload), Payload: payload}
	}
	return nil, lastErr
}

func (c *Client) DoMultipart(ctx context.Context, path string, query url.Values, fieldName string, filePaths []string) (any, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}
		part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
		if err != nil {
			f.Close()
			return nil, err
		}
		if _, err := io.Copy(part, f); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	reqURL := c.BaseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}
	bodyBytes := buf.Bytes()
	contentType := writer.FormDataContentType()
	attempts := 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		c.applyHeaders(req)
		req.Header.Set("Content-Type", contentType)
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		payload, readErr := readPayload(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		c.debugf("POST %s -> %d", req.URL.Path, resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return payload, nil
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < attempts {
			c.debugf("retrying POST %s after status %d (attempt %d/%d)", req.URL.Path, resp.StatusCode, attempt, attempts)
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			lastErr = &ResponseError{StatusCode: resp.StatusCode, Body: stringifyPayload(payload), Payload: payload}
			continue
		}
		return nil, &ResponseError{StatusCode: resp.StatusCode, Body: stringifyPayload(payload), Payload: payload}
	}
	return nil, lastErr
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Token))
	}
}

func (c *Client) debugf(format string, args ...any) {
	if !c.Debug || c.Stderr == nil {
		return
	}
	_, _ = fmt.Fprintf(c.Stderr, format+"\n", args...)
}

func readPayload(r io.Reader) (any, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var payload any
	if err := json.Unmarshal(b, &payload); err == nil {
		return payload, nil
	}
	return string(b), nil
}

func stringifyPayload(payload any) string {
	switch v := payload.(type) {
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
