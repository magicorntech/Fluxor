package domain

import "testing"

func TestClusterValidate(t *testing.T) {
	tests := []struct {
		name    string
		cluster Cluster
		wantErr bool
	}{
		{"ok", Cluster{Name: "prod-eu", APIServer: "https://10.0.0.1:6443"}, false},
		{"empty name", Cluster{APIServer: "https://x"}, true},
		{"uppercase name", Cluster{Name: "Prod", APIServer: "https://x"}, true},
		{"leading dash", Cluster{Name: "-a", APIServer: "https://x"}, true},
		{"long name", Cluster{Name: "a123456789a123456789a123456789a123456789a123456789a123456789a123456789", APIServer: "https://x"}, true},
		{"bad scheme", Cluster{Name: "a", APIServer: "ftp://x"}, true},
		{"no host", Cluster{Name: "a", APIServer: "https://"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cluster.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
