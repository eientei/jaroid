// Package middleware provides http client/server middleware wrappers
package middleware

import "net/http"

// Client provides common interface for client-side http middleware
type Client interface {
	Preprocess(req *http.Request) (*http.Request, error)
	Postprocess(resp *http.Response) (*http.Response, error)
}

// Request provides function type to intercept http request
type Request func(req *http.Request) (*http.Request, error)

// Response provides function type to intercept http response
type Response func(resp *http.Response) (*http.Response, error)

type clientFunc struct {
	pre  Request
	post Response
}

func (c *clientFunc) Preprocess(req *http.Request) (*http.Request, error) {
	return c.pre(req)
}

func (c *clientFunc) Postprocess(resp *http.Response) (*http.Response, error) {
	return c.post(resp)
}

// NoopRequest request middleware that returns original request
func NoopRequest(req *http.Request) (*http.Request, error) {
	return req, nil
}

// NoopResponse response middleware that returns original response
func NoopResponse(resp *http.Response) (*http.Response, error) {
	return resp, nil
}

// ClientMiddlewareFunc returns client middleware instance using provided functions for pre- and post-processing
func ClientMiddlewareFunc(pre Request, post Response) Client {
	if pre == nil {
		pre = NoopRequest
	}

	if post == nil {
		post = NoopResponse
	}

	return &clientFunc{
		pre:  pre,
		post: post,
	}
}

// Transport provides generic http client middleware with intercepting callbacks
type Transport struct {
	Transport   http.RoundTripper
	Middlewares []Client
}

// RoundTrip implementation
func (c *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	for _, m := range c.Middlewares {
		req, err = m.Preprocess(req)
		if err != nil {
			return
		}
	}

	resp, err = c.Transport.RoundTrip(req)
	if err != nil {
		return
	}

	for _, m := range c.Middlewares {
		resp, err = m.Postprocess(resp)
		if err != nil {
			return
		}
	}

	return
}
