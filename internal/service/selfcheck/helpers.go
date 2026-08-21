package selfcheck

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// stringReader returns a reader for a string; tiny helper kept here to avoid
// an extra import in selfcheck.go.
func stringReader(s string) io.Reader { return strings.NewReader(s) }

// decodeJSONBody decodes a JSON body from an io.Reader (the *http.Response.Body
// field is an io.ReadCloser, which satisfies io.Reader).
func decodeJSONBody(r io.Reader) func(v interface{}) error {
	return func(v interface{}) error {
		return json.NewDecoder(r).Decode(v)
	}
}

// respBody returns the body reader from an *http.Response.
func respBody(resp *http.Response) io.Reader { return resp.Body }
