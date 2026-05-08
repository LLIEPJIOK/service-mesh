package kube

import "testing"

func TestParseServiceAccountUsername(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		wantNS   string
		wantSA   string
		wantErr  bool
	}{
		{
			name:     "valid service account username",
			username: "system:serviceaccount:default:reviews",
			wantNS:   "default",
			wantSA:   "reviews",
		},
		{
			name:     "invalid prefix",
			username: "system:user:default:reviews",
			wantErr:  true,
		},
		{
			name:     "missing service account",
			username: "system:serviceaccount:default:",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			identity, err := parseServiceAccountUsername(tt.username)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			if identity.Namespace != tt.wantNS || identity.ServiceAccount != tt.wantSA {
				t.Fatalf("unexpected identity: got %s/%s want %s/%s", identity.Namespace, identity.ServiceAccount, tt.wantNS, tt.wantSA)
			}
		})
	}
}
