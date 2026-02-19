package registry

import (
	"crypto/rand"
	"fmt"
)

// generateID creates a short random hex ID.
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
