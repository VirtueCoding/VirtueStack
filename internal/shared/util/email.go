package util

import "strings"

// MaskEmail masks an email address for safe logging, keeping only the first
// character of the local part and the first character of the domain.
// Example: "john.doe@example.com" → "j***@e***.com"
// If the input is not a valid email format, "***" is returned.
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}

	local := email[:at]
	rest := email[at+1:]

	dot := strings.LastIndex(rest, ".")
	if dot <= 0 {
		if len(rest) == 0 {
			return "***"
		}
		return string(local[0]) + "***@" + string(rest[0]) + "***"
	}

	domain := rest[:dot]
	tld := rest[dot:] // includes the leading dot

	maskedLocal := string(local[0]) + "***"
	maskedDomain := string(domain[0]) + "***"

	return maskedLocal + "@" + maskedDomain + tld
}
