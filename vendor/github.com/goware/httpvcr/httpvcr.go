package httpvcr

import (
	"context"
	"net/http"
	"sync"

	"github.com/pkg/errors"
)

type Mode uint32

const (
	ModeStopped Mode = iota
	ModeRecord
	ModeReplay
)

type VCR struct {
	options Options

	ctx       context.Context
	ctxCancel context.CancelFunc

	mode          Mode
	Cassette      *cassette
	FilterMap     map[string]string
	BeforeRequest func(Mode, *http.Request)
	URLRewriter   func(string) string

	originalTransport http.RoundTripper

	mu sync.Mutex
}

type Options struct {
	HTTPDefaultOverride bool
}

var DefaultOptions = Options{
	HTTPDefaultOverride: true,
}

func New(cassetteName string, opts ...Options) *VCR {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	return &VCR{
		options:   options,
		mode:      ModeStopped,
		Cassette:  &cassette{name: cassetteName},
		FilterMap: make(map[string]string),
	}
}

// Start starts a VCR session with the given cassette name.
// Records episodes if the cassette file does not exists.
// Otherwise plays back recorded episodes.
func (v *VCR) Start(ctx context.Context) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.mode != ModeStopped {
		panic("httpvcr: session already started!")
	}

	v.ctx, v.ctxCancel = context.WithCancel(ctx)

	v.originalTransport = http.DefaultTransport
	if v.options.HTTPDefaultOverride {
		http.DefaultTransport = v
	}

	if v.Cassette.Exists() {
		v.mode = ModeReplay
		v.Cassette.read()
	} else {
		v.mode = ModeRecord
	}
}

// Stop stops the VCR session and writes the cassette file (when recording)
func (v *VCR) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.mode == ModeStopped {
		return
	}

	if v.mode == ModeRecord {
		v.Cassette.write()
	}

	if v.options.HTTPDefaultOverride && v.originalTransport != nil {
		http.DefaultTransport = v.originalTransport
	}

	v.mode = ModeStopped
	v.ctxCancel()
}

func (v *VCR) Mode() Mode {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.mode
}

// FilterData allows replacement of sensitive data with a dummy-string
func (v *VCR) FilterResponseBody(original string, replacement string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.FilterMap[original] = replacement
}

func (v *VCR) RoundTrip(request *http.Request) (*http.Response, error) {
	if v.ctx.Err() == context.Canceled {
		return nil, errors.Errorf("httpvcr: stopped")
	}

	if v.mode == ModeStopped {
		return v.originalTransport.RoundTrip(request)
	}

	vcrReq := newVCRRequest(request, v.FilterMap)
	var vcrRes *vcrResponse

	if v.BeforeRequest != nil {
		v.BeforeRequest(v.mode, request)
	}

	if v.mode == ModeRecord {
		response, err := v.originalTransport.RoundTrip(request)
		if err != nil {
			return nil, err
		}

		vcrRes = newVCRResponse(response)

		if v.URLRewriter != nil {
			vcrReq.URL = v.URLRewriter(vcrReq.URL)
		}

		e := episode{Request: vcrReq, Response: vcrRes}
		v.Cassette.Episodes = append(v.Cassette.Episodes, e)

	} else {
		if v.URLRewriter != nil {
			vcrReq.URL = v.URLRewriter(vcrReq.URL)
		}

		e := v.Cassette.matchEpisode(vcrReq)
		vcrRes = e.Response
	}

	if v.mode == ModeReplay {
		if len(v.Cassette.Episodes) == 0 {
			v.Stop()
		}
	}

	return vcrRes.httpResponse(), nil
}

func (v *VCR) Done() <-chan struct{} {
	return v.ctx.Done()
}
