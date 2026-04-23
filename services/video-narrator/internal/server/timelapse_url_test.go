package server

import (
	"testing"
)

func TestValidateTimelapseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rawURL       string
		allowedHosts []string
		wantErr      bool
	}{
		// ── valid cases ──────────────────────────────────────────────────────
		{
			name:    "plain https URL, no allowlist",
			rawURL:  "https://storage.example.com/timelapse.mp4",
			wantErr: false,
		},
		{
			name:    "plain http URL, no allowlist",
			rawURL:  "http://cdn.example.com/timelapse.mp4",
			wantErr: false,
		},
		{
			name:         "https URL matches allowlist host",
			rawURL:       "https://storage.example.com/timelapse.mp4",
			allowedHosts: []string{"storage.example.com"},
			wantErr:      false,
		},
		{
			name:         "https URL with port matches allowlist host:port",
			rawURL:       "https://storage.example.com:9000/timelapse.mp4",
			allowedHosts: []string{"storage.example.com:9000"},
			wantErr:      false,
		},
		{
			name:         "https URL host matches bare hostname in allowlist even when port present",
			rawURL:       "https://storage.example.com:9000/timelapse.mp4",
			allowedHosts: []string{"storage.example.com"},
			wantErr:      false,
		},

		// ── scheme rejections ────────────────────────────────────────────────
		{
			name:    "file:// scheme rejected",
			rawURL:  "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "ftp:// scheme rejected",
			rawURL:  "ftp://cdn.example.com/timelapse.mp4",
			wantErr: true,
		},
		{
			name:    "gopher:// scheme rejected",
			rawURL:  "gopher://cdn.example.com/",
			wantErr: true,
		},

		// ── private / loopback IP rejections ────────────────────────────────
		{
			name:    "loopback IPv4 rejected",
			rawURL:  "http://127.0.0.1/timelapse.mp4",
			wantErr: true,
		},
		{
			name:    "loopback IPv6 rejected",
			rawURL:  "http://[::1]/timelapse.mp4",
			wantErr: true,
		},
		{
			name:    "private 10.x.x.x rejected",
			rawURL:  "http://10.0.0.1/timelapse.mp4",
			wantErr: true,
		},
		{
			name:    "private 192.168.x.x rejected",
			rawURL:  "http://192.168.1.100/timelapse.mp4",
			wantErr: true,
		},
		{
			name:    "private 172.16.x.x rejected",
			rawURL:  "http://172.16.0.1/timelapse.mp4",
			wantErr: true,
		},
		{
			name:    "unspecified 0.0.0.0 rejected",
			rawURL:  "http://0.0.0.0/timelapse.mp4",
			wantErr: true,
		},

		// ── allowlist rejections ─────────────────────────────────────────────
		{
			name:         "host not in allowlist rejected",
			rawURL:       "https://evil.example.com/timelapse.mp4",
			allowedHosts: []string{"storage.example.com"},
			wantErr:      true,
		},

		// ── malformed URL ────────────────────────────────────────────────────
		{
			name:    "empty string rejected",
			rawURL:  "",
			wantErr: true,
		},
		{
			name:    "relative path rejected",
			rawURL:  "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "no scheme rejected",
			rawURL:  "storage.example.com/timelapse.mp4",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateTimelapseURL(tc.rawURL, tc.allowedHosts)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateTimelapseURL(%q, %v) error = %v, wantErr %v",
					tc.rawURL, tc.allowedHosts, err, tc.wantErr)
			}
		})
	}
}
