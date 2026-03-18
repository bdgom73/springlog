package cli

import "strings"

// shortLogger abbreviates a fully qualified logger name.
// e.g. "com.example.service.PaymentService" → "c.e.s.PaymentService"
func shortLogger(logger string) string {
	parts := strings.Split(logger, ".")
	if len(parts) <= 1 {
		return logger
	}
	short := make([]string, len(parts))
	for i, p := range parts[:len(parts)-1] {
		if len(p) > 0 {
			short[i] = string(p[0])
		}
	}
	short[len(parts)-1] = parts[len(parts)-1]
	return strings.Join(short, ".")
}
