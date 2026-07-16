package pluginregistry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// rejectDuplicateJSONKeys closes an ambiguity left by encoding/json's normal
// last-key-wins behavior. Registry producers, reviewers, and clients must all
// interpret the authenticated document as the same object graph.
func rejectDuplicateJSONKeys(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := walkStrictJSONValue(dec, "$"); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func walkStrictJSONValue(dec *json.Decoder, location string) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", location)
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q at %s", key, location)
			}
			seen[key] = struct{}{}
			if err := walkStrictJSONValue(dec, location+"."+key); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("object at %s has invalid terminator", location)
		}
	case '[':
		for index := 0; dec.More(); index++ {
			if err := walkStrictJSONValue(dec, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("array at %s has invalid terminator", location)
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delim, location)
	}
	return nil
}
