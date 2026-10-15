package lab

import (
	"strings"
	"testing"
)

func TestRedactorSanitization(t *testing.T) {
	r := NewCustomRedactor("/home/developer", "developer", "workstation-fedora-01")

	tests := []struct {
		name     string
		input    string
		expected string
		mustOmit []string
	}{
		{
			name:     "Authorization header with Bearer",
			input:    "HTTP/1.1 200 OK\nAuthorization: Bearer my-super-secret-jwt-token-value-1234\nContent-Type: text/plain",
			mustOmit: []string{"my-super-secret-jwt-token-value-1234"},
			expected: "[REDACTED]",
		},
		{
			name:     "Cookie and Set-Cookie headers",
			input:    "Cookie: session_id=abc123xyz789; tracking=true\nSet-Cookie: user_auth=secret_auth_hash",
			mustOmit: []string{"session_id=abc123xyz789", "secret_auth_hash"},
			expected: "[COOKIE REDACTED]",
		},
		{
			name:     "GitHub personal access token",
			input:    "fatal: remote error for ghp_012345678901234567890123456789012345: Bad credentials",
			mustOmit: []string{"ghp_012345678901234567890123456789012345"},
			expected: "[GITHUB_TOKEN REDACTED]",
		},
		{
			name:     "JWT token pattern",
			input:    "Decoded JWT: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c in response",
			mustOmit: []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
			expected: "[JWT REDACTED]",
		},
		{
			name:     "AWS Access Key ID",
			input:    "Loaded AWS credential from environment: AKIAIOSFODNN7EXAMPLE",
			mustOmit: []string{"AKIAIOSFODNN7EXAMPLE"},
			expected: "[AWS_KEY REDACTED]",
		},
		{
			name:     "Generic API key assignment",
			input:    "Connecting with api_key=\"sec_live_998877665544332211\" to endpoint",
			mustOmit: []string{"sec_live_998877665544332211"},
			expected: "[REDACTED]",
		},
		{
			name:     "Email addresses",
			input:    "Contact maintainer at dev-lead@example.com or support.team@service.org for help",
			mustOmit: []string{"dev-lead@example.com", "support.team@service.org"},
			expected: "[EMAIL REDACTED]",
		},
		{
			name:     "Home directory and username",
			input:    "Failed writing file to /home/developer/.config/app/settings.json by developer",
			mustOmit: []string{"/home/developer", "developer"},
			expected: "[HOME REDACTED]",
		},
		{
			name:     "Hostname",
			input:    "Host workstation-fedora-01 connected to cluster node",
			mustOmit: []string{"workstation-fedora-01"},
			expected: "[HOSTNAME REDACTED]",
		},
		{
			name:     "Public IP address (while preserving localhost)",
			input:    "Listening on 127.0.0.1:8080, remote peer 198.51.100.42 disconnected",
			mustOmit: []string{"198.51.100.42"},
			expected: "127.0.0.1", // must retain localhost
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := r.Redact(tc.input)

			if !strings.Contains(output, tc.expected) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.expected, output)
			}

			for _, omit := range tc.mustOmit {
				if strings.Contains(output, omit) {
					t.Errorf("output still contains sensitive string %q:\n%s", omit, output)
				}
			}
		})
	}
}
