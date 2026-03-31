// Package util provides common utility functions for pointer handling and other operations.
package util

// StringPtr returns a pointer to the given string value.
// This is a helper function for creating pointer values in struct literals.
func StringPtr(s string) *string {
	return &s
}
