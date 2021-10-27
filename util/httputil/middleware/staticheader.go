package middleware

import (
	"net/http"
	"net/textproto"
)

// ClientStaticHeaders provides middleware to statically add/set headers to outgoing http requests
type ClientStaticHeaders struct {
	Set map[string][]string
	Add map[string][]string
}

// Preprocess implementation
func (c *ClientStaticHeaders) Preprocess(req *http.Request) (*http.Request, error) {
	if c.Add != nil {
		for k, vs := range c.Add {
			key := textproto.CanonicalMIMEHeaderKey(k)

			req.Header[key] = append(req.Header[key], vs...)
		}
	}

	if c.Set != nil {
		for k, vs := range c.Set {
			key := textproto.CanonicalMIMEHeaderKey(k)

			req.Header[key] = vs
		}
	}

	return req, nil
}

// Postprocess noop
func (c *ClientStaticHeaders) Postprocess(resp *http.Response) (*http.Response, error) {
	return resp, nil
}
