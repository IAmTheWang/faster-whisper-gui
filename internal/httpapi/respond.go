package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// maxBodyBytes bounds JSON request bodies; every legitimate payload here is
// a handful of short fields.
const maxBodyBytes = 1 << 20

// decodeJSON reads a size-limited JSON body into v. Unknown fields and data
// after the JSON value are rejected, so a typo'd option fails loudly instead
// of being silently ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after the JSON body")
	}
	return nil
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeErrorMsg(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeErrorMsg(w, status, err.Error())
}
