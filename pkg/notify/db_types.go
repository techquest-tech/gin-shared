package notify

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *StringSlice) Scan(value interface{}) error {
	if s == nil {
		return fmt.Errorf("scan target is nil")
	}
	if value == nil {
		*s = nil
		return nil
	}

	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			*s = nil
			return nil
		}
		var out []string
		if err := json.Unmarshal(v, &out); err != nil {
			return err
		}
		*s = out
		return nil
	case string:
		if v == "" {
			*s = nil
			return nil
		}
		var out []string
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return err
		}
		*s = out
		return nil
	default:
		return fmt.Errorf("unsupported scan type: %T", value)
	}
}

