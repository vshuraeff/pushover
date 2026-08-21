// package api provides the shared http transport for pushover api services.
package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxAttempts    = 3
	defaultBackoff = 15 * time.Millisecond
	maxBackoff     = 100 * time.Millisecond
)

// rate limits contains the application rate-limit headers returned by pushover.
type RateLimits struct {
	Limit     string `json:"limit"`
	Remaining string `json:"remaining"`
	Reset     string `json:"reset"`
}

// response is the common result envelope returned by client helpers.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	RateLimits RateLimits
}

// client sends requests to one pushover api base url.
type Client struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
	backoff    time.Duration
	sleep      func(time.Duration)
}

// option configures a client.
type Option func(*Client)

// with http client uses client for all requests.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// with retry backoff changes the initial retry delay.
func WithRetryBackoff(delay time.Duration) Option {
	return func(c *Client) {
		if delay >= 0 {
			c.backoff = delay
		}
	}
}

// with sleep replaces the backoff sleeper, primarily for deterministic tests.
func WithSleep(sleep func(time.Duration)) Option {
	return func(c *Client) {
		if sleep != nil {
			c.sleep = sleep
		}
	}
}

// new constructs a client for base url using version in every user-agent header.
func New(baseURL, version string, options ...Option) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = 2
	transport.DisableKeepAlives = false

	client := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		userAgent:  fmt.Sprintf("povr/%s (unofficial Pushover Open Client; +https://github.com/vshuraeff/pushover)", version),
		httpClient: &http.Client{Transport: transport},
		backoff:    defaultBackoff,
		sleep:      time.Sleep,
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}
	return client
}

// request option changes the behavior of one request.
type RequestOption func(*requestOptions)

type requestOptions struct {
	noRetry bool
}

// no retry disables retries for a single request.
func NoRetry() RequestOption {
	return func(options *requestOptions) {
		options.noRetry = true
	}
}

// file part describes the file included in a multipart request.
type FilePart struct {
	FieldName string
	FileName  string
	Reader    io.ReadSeeker
}

// get performs a get request with query parameters.
func (c *Client) Get(ctx context.Context, path string, query url.Values, options ...RequestOption) (*Response, error) {
	return c.do(ctx, options, func() (*http.Request, func(), error) {
		requestURL, err := c.requestURL(path, query)
		if err != nil {
			return nil, nil, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, nil, err
		}
		request.Header.Set("User-Agent", c.userAgent)
		return request, nil, nil
	})
}

// post form performs an application/x-www-form-urlencoded post request.
func (c *Client) PostForm(ctx context.Context, path string, values url.Values, options ...RequestOption) (*Response, error) {
	encoded := values.Encode()
	return c.do(ctx, options, func() (*http.Request, func(), error) {
		requestURL, err := c.requestURL(path, nil)
		if err != nil {
			return nil, nil, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), strings.NewReader(encoded))
		if err != nil {
			return nil, nil, err
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("User-Agent", c.userAgent)
		return request, nil, nil
	})
}

// post multipart performs a streaming multipart post request.
func (c *Client) PostMultipart(ctx context.Context, path string, fields url.Values, file FilePart, options ...RequestOption) (*Response, error) {
	if file.FieldName == "" || file.FileName == "" || file.Reader == nil {
		return nil, fmt.Errorf("multipart file requires a field name, file name, and reader")
	}

	position, err := file.Reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("record multipart reader position: %w", err)
	}

	return c.do(ctx, options, func() (*http.Request, func(), error) {
		if _, err := file.Reader.Seek(position, io.SeekStart); err != nil {
			return nil, nil, fmt.Errorf("reset multipart reader: %w", err)
		}

		requestURL, err := c.requestURL(path, nil)
		if err != nil {
			return nil, nil, err
		}
		reader, writer := io.Pipe()
		multipartWriter := multipart.NewWriter(writer)
		done := make(chan struct{})

		go func() {
			defer close(done)
			if err := writeMultipart(multipartWriter, fields, file); err != nil {
				_ = writer.CloseWithError(err)
				return
			}
			_ = writer.Close()
		}()

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), reader)
		if err != nil {
			_ = reader.Close()
			<-done
			return nil, nil, err
		}
		request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		request.Header.Set("User-Agent", c.userAgent)
		return request, func() { <-done }, nil
	})
}

func writeMultipart(writer *multipart.Writer, fields url.Values, file FilePart) error {
	for name, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(name, value); err != nil {
				return err
			}
		}
	}
	part, err := writer.CreateFormFile(file.FieldName, file.FileName)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file.Reader); err != nil {
		return err
	}
	return writer.Close()
}

func (c *Client) do(ctx context.Context, options []RequestOption, makeRequest func() (*http.Request, func(), error)) (*Response, error) {
	requestOptions := requestOptions{}
	for _, option := range options {
		if option != nil {
			option(&requestOptions)
		}
	}

	attempts := maxAttempts
	if requestOptions.noRetry {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		request, cleanup, err := makeRequest()
		if err != nil {
			return nil, err
		}

		httpResponse, err := c.httpClient.Do(request)
		if cleanup != nil {
			cleanup()
		}
		if err != nil {
			if attempt+1 < attempts && ctx.Err() == nil {
				c.sleep(c.retryDelay(attempt))
				continue
			}
			return nil, err
		}

		response, err := responseFromHTTP(httpResponse)
		if err != nil {
			return response, err
		}
		if httpResponse.StatusCode >= http.StatusInternalServerError && attempt+1 < attempts && ctx.Err() == nil {
			c.sleep(c.retryDelay(attempt))
			continue
		}
		return response, responseError(response)
	}

	return nil, fmt.Errorf("pushover request retry loop exited unexpectedly")
}

func (c *Client) retryDelay(attempt int) time.Duration {
	delay := c.backoff << attempt
	if delay > maxBackoff {
		return maxBackoff
	}
	return delay
}

func (c *Client) requestURL(path string, query url.Values) (*url.URL, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse api base URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("api base URL must be absolute")
	}
	reference, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parse request path: %w", err)
	}
	if reference.IsAbs() || reference.Host != "" {
		return nil, fmt.Errorf("request path must be relative to the API base URL")
	}

	base.Path = strings.TrimRight(base.Path, "/") + "/"
	reference.Path = strings.TrimLeft(reference.Path, "/")
	requestURL := base.ResolveReference(reference)
	values := requestURL.Query()
	for name, entries := range query {
		for _, entry := range entries {
			values.Add(name, entry)
		}
	}
	requestURL.RawQuery = values.Encode()
	return requestURL, nil
}

func responseFromHTTP(httpResponse *http.Response) (*Response, error) {
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(httpResponse.Body)
	response := &Response{
		StatusCode: httpResponse.StatusCode,
		Header:     httpResponse.Header.Clone(),
		Body:       body,
		RateLimits: RateLimits{
			Limit:     httpResponse.Header.Get("X-Limit-App-Limit"),
			Remaining: httpResponse.Header.Get("X-Limit-App-Remaining"),
			Reset:     httpResponse.Header.Get("X-Limit-App-Reset"),
		},
	}
	if err != nil {
		return response, fmt.Errorf("read pushover response: %w", err)
	}
	return response, nil
}

func responseError(response *Response) error {
	apiError := apiErrorFromBody(response.Body)
	if response.StatusCode == http.StatusTooManyRequests {
		return &QuotaError{Reset: response.RateLimits.Reset, APIError: apiError}
	}
	if apiError != nil {
		return apiError
	}
	if response.StatusCode >= http.StatusBadRequest {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return nil
}
