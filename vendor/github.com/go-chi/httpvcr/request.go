package httpvcr

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

type vcrRequest struct {
	// Header is intentionally not included and is not used for episode matching
	Method string
	URL    string
	Body   []byte
}

func newVCRRequest(request *http.Request, filterMap map[string]string) *vcrRequest {
	var body []byte
	if request.Body != nil {
		body, _ = ioutil.ReadAll(request.Body)
		request.Body.Close()
		request.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		for original, replacement := range filterMap {
			body = bytes.Replace(body, []byte(original), []byte(replacement), -1)
		}
	}

	return &vcrRequest{
		Method: request.Method,
		URL:    request.URL.String(),
		Body:   body,
	}
}
