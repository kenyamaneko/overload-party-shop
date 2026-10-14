package rest

import (
	"fmt"
	"strings"
)

// requiredField は契約で必須と定めた項目の名前と、リクエストから読み取った値の組。
type requiredField struct {
	name  string
	value string
}

// validateRequiredFields は値が空の必須項目があるとき、欠落した項目名をすべて含むエラーを返す。
func validateRequiredFields(fields ...requiredField) error {
	missing := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
}
