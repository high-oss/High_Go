// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"fmt"
	"net/url"
	"strings"
)

// pathOf interpolates a path template, percent-encoding every value.
//
// Trading symbols legitimately contain '&' and spaces ("M&M-EQ", "NIFTY 50"),
// which change the request if interpolated raw. A value is also encoded
// against carrying a literal '/', so a parameter can never add a path
// segment. Encoding is not optional.
func pathOf(template string, params map[string]string) (string, error) {
	var b strings.Builder
	i := 0
	for i < len(template) {
		if template[i] == '{' {
			end := strings.IndexByte(template[i:], '}')
			if end == -1 {
				b.WriteByte(template[i])
				i++
				continue
			}
			name := template[i+1 : i+end]
			value, ok := params[name]
			if !ok {
				return "", fmt.Errorf("missing path parameter %q for %s", name, template)
			}
			b.WriteString(url.PathEscape(value))
			i += end + 1
			continue
		}
		b.WriteByte(template[i])
		i++
	}
	return b.String(), nil
}
