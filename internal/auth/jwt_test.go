package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// You may need to import your jwt funcs, e.g.
// import . "github.com/johnstontu/Chirpy/internal/auth"

func TestMakeAndValidateJWT(t *testing.T) {
	secret := "supersecretkey"
	userID := uuid.New()
	expiresIn := 1 * time.Hour

	// 1) Generate a token
	token, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}

	// 2) Validate it
	gotID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT returned error: %v", err)
	}

	// 3) Check the IDs match
	if gotID != userID {
		t.Errorf("ValidateJWT = %q; want %q", gotID, userID)
	}
}

func TestGetBearerToken(t *testing.T) {
	const goodToken = "abc.def.ghi"

	tests := []struct {
		name    string
		headers http.Header
		want    string
		wantErr bool
	}{
		{
			name:    "no Authorization header",
			headers: http.Header{},
			wantErr: true,
		},
		{
			name: "wrong prefix",
			headers: http.Header{
				"Authorization": []string{"Token " + goodToken},
			},
			wantErr: true,
		},
		{
			name: "empty Bearer token",
			headers: http.Header{
				"Authorization": []string{"Bearer "},
			},
			wantErr: true,
		},
		{
			name: "valid Bearer token",
			headers: http.Header{
				"Authorization": []string{"Bearer " + goodToken},
			},
			want:    goodToken,
			wantErr: false,
		},
		{
			name: "extra whitespace",
			headers: http.Header{
				"Authorization": []string{"  Bearer   " + goodToken + "   "},
			},
			want:    goodToken,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetBearerToken(tc.headers)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GetBearerToken(%v) error = %v, wantErr=%v", tc.headers, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GetBearerToken(%v) = %q; want %q", tc.headers, got, tc.want)
			}
		})
	}
}
