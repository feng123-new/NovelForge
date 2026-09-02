package qualitygate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type JSONRepairer interface {
	Repair(context.Context, string, []byte, error) ([]byte, error)
}

type StrictDecoder struct {
	MaxRepairs int
	Repairer   JSONRepairer
}

func (d StrictDecoder) Decode(ctx context.Context, schema string, raw []byte, target any) (int, error) {
	if target == nil {
		return 0, errors.New("structured output target is required")
	}
	attempts := 0
	current := append([]byte(nil), raw...)
	for {
		err := decodeOneStrict(current, target)
		if err == nil {
			if validator, ok := target.(interface{ Validate() error }); ok {
				if validationErr := validator.Validate(); validationErr == nil {
					return attempts, nil
				} else {
					err = validationErr
				}
			} else {
				return attempts, nil
			}
		}
		if attempts >= d.MaxRepairs || d.Repairer == nil {
			return attempts, fmt.Errorf("%s structured output invalid after %d repair(s): %w", schema, attempts, err)
		}
		repaired, repairErr := d.Repairer.Repair(ctx, schema, current, err)
		if repairErr != nil {
			return attempts, fmt.Errorf("%s structured output repair failed: %w", schema, repairErr)
		}
		attempts++
		current = append(current[:0], repaired...)
	}
}

func decodeOneStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values or trailing data are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return errors.New("trailing data are not allowed")
	}
	return nil
}
