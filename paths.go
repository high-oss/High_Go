// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"fmt"
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
			b.WriteString(encodeURIComponent(value))
			i += end + 1
			continue
		}
		b.WriteByte(template[i])
		i++
	}
	return b.String(), nil
}

// encodeURIComponent mirrors JavaScript's encodeURIComponent: every byte
// outside RFC 3986's unreserved set (ALPHA / DIGIT / "-" / "." / "_" / "~")
// is percent-encoded, including '&', '/' and space. url.PathEscape is not
// this: it leaves sub-delimiters like '&' unescaped because they are valid
// inside an RFC 3986 path segment on their own — but the contract requires
// "M&M-EQ" to come out as "M%26M-EQ" so a trading symbol containing one never
// changes the request's shape.
func encodeURIComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
