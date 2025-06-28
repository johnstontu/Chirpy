package auth

import (
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
