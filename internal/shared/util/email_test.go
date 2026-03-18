package util

import "testing"

func TestMaskEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			// local="john.doe", rest="example.com", domain="example", tld=".com"
			name:  "normal email",
			input: "john.doe@example.com",
			want:  "j***@e***.com",
		},
		{
			// local="a", rest="example.com", domain="example", tld=".com"
			name:  "single-char local part",
			input: "a@example.com",
			want:  "a***@e***.com",
		},
		{
			// at=-1, at <= 0 → "***"
			name:  "missing @ sign",
			input: "notanemail",
			want:  "***",
		},
		{
			// at=0, at <= 0 → "***"
			name:  "@ at start",
			input: "@example.com",
			want:  "***",
		},
		{
			// local="user", rest="nodot", dot=strings.LastIndex("nodot",".")=-1, -1<=0 → no-TLD branch
			// returns string(local[0])+"***@"+string(rest[0])+"***" = "u***@n***"
			name:  "no TLD dot in domain",
			input: "user@nodot",
			want:  "u***@n***",
		},
		{
			// local="alice", rest="gmail.com", domain="gmail", tld=".com"
			name:  "alice@gmail.com",
			input: "alice@gmail.com",
			want:  "a***@g***.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskEmail(tc.input)
			if got != tc.want {
				t.Errorf("MaskEmail(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
