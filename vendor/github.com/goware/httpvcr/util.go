package httpvcr

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

// ModifyStringFunc is a function that modifies a given string and returns it
type ModifyStringFunc func(input string) string

// ModifyHTTPRequestBody changes the body of an HTTP request using a callback function
func ModifyHTTPRequestBody(request *http.Request, modifyFunc ModifyStringFunc) {
	if request.Body == nil {
		return
	}

	body, _ := ioutil.ReadAll(request.Body)
	request.Body.Close()

	newBody := modifyFunc(string(body))

	request.Body = ioutil.NopCloser(bytes.NewBufferString(newBody))
	request.ContentLength = int64(len(newBody))
}
