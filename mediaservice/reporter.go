package mediaservice

import (
	"sync"
	"time"
)

// Reporter provides helper for rate-limited diagnostic messages from downloader
// All messages submitted at higher frequency than rate will be lost.
// Caller should read from Messages() on higher or equal frequency than rate specified, otherwise lagged messages
// are going to be lost.
type Reporter interface {
	Messages() <-chan string
	Submit(msg string, force bool)
	Close()
}

type dummyReporter struct {
}

func (*dummyReporter) Messages() <-chan string {
	return nil
}

func (*dummyReporter) Submit(msg string, force bool) {

}

func (*dummyReporter) Close() {

}

// NewDummyReporter returns new dummy reporter implementation
func NewDummyReporter() Reporter {
	return &dummyReporter{}
}

// NewReporter returns new reporter instance with provided rate limit and buffer size for Messages() channel
func NewReporter(rate time.Duration, buf int) Reporter {
	rep := &reporterImpl{
		m:        &sync.Mutex{},
		messages: make(chan string, buf),
		rate:     rate,
	}

	return rep
}

type reporterImpl struct {
	m        *sync.Mutex
	messages chan string
	rate     time.Duration
	last     int64
	acc      int64
}

func (r *reporterImpl) Messages() <-chan string {
	r.m.Lock()
	defer r.m.Unlock()

	return r.messages
}

func (r *reporterImpl) Close() {
	r.m.Lock()
	defer r.m.Unlock()

	if r.messages == nil {
		return
	}

	close(r.messages)

	r.messages = nil
}

func (r *reporterImpl) Submit(msg string, force bool) {
	now := time.Now().UnixNano()

	r.m.Lock()
	defer r.m.Unlock()

	if r.messages == nil {
		return
	}

	var el int64

	el, r.last = now-r.last, now

	if el < 0 && !force {
		return
	}

	r.acc += el

	if r.acc < int64(r.rate) {
		if !force {
			return
		}
	} else {
		r.acc = 0
	}

	r.messages <- msg
}
