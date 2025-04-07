package common

import (
	"fmt"
	"strings"
)

// GetValueAsString получает строковое значение из строки таблицы по индексу колонки
func GetValueAsString(row []interface{}, columnIndex int) string {
	if columnIndex >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", row[columnIndex]))
}
