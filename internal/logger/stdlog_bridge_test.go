package logger

import "testing"

func TestRedactSensitiveData(t *testing.T) {
	t.Parallel()

	const token = "f9LHsecret"

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "access token query param",
			text: `Get "https://botapi.max.ru/updates?access_token=f9LHsecret&limit=50"`,
			want: `Get "https://botapi.max.ru/updates?access_token=<redacted>&limit=50"`,
		},
		{
			name: "direct token string",
			text: "failed to call api with token f9LHsecret due to EOF",
			want: "failed to call api with token <redacted> due to EOF",
		},
		{
			name: "uppercase access token",
			text: "ACCESS_TOKEN=f9LHsecret",
			want: "ACCESS_TOKEN=<redacted>",
		},
		{
			name: "no sensitive data",
			text: "nothing to redact here",
			want: "nothing to redact here",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := redactSensitiveData(tt.text, token)
			if got != tt.want {
				t.Fatalf("redactSensitiveData() = %q, want %q", got, tt.want)
			}
		})
	}
}
