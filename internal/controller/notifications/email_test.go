package notifications

import "testing"

func TestSanitizeHeader(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "CR injection stripped",
			input: "Subject\rX-Injected: evil",
			want:  "SubjectX-Injected: evil",
		},
		{
			name:  "LF injection stripped",
			input: "Subject\nX-Injected: evil",
			want:  "SubjectX-Injected: evil",
		},
		{
			name:  "CRLF combined stripped",
			input: "Subject\r\nX-Injected: evil",
			want:  "SubjectX-Injected: evil",
		},
		{
			name:  "clean input passes through unchanged",
			input: "Hello World",
			want:  "Hello World",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeHeader(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeHeader(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
