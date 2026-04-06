package docs

import (
	"testing"

	swagv1 "github.com/swaggo/swag"
)

func TestGeneratedSwaggerRegistersWithV1Registry(t *testing.T) {
	t.Parallel()

	doc, err := swagv1.ReadDoc()
	if err != nil {
		t.Fatalf("ReadDoc() error = %v", err)
	}
	if doc == "" {
		t.Fatal("ReadDoc() returned empty swagger document")
	}
}
