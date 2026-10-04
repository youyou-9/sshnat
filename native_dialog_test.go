package main

import (
	"errors"
	"testing"
)

func TestNativeSaveCancellationAndRealFailures(t *testing.T) {
	cancelled := errors.New("cancelled by user")
	failed := errors.New("dialog creation failed")
	for _, test := range []struct {
		name, path string
		err        error
		wantErr    error
	}{
		{"Windows cancellation", "", cancelled, nil},
		{"empty selection", "", nil, nil},
		{"save selection", "backup.json", nil, nil},
		{"real failure", "", failed, failed},
		{"nonempty failed selection", "backup.json", cancelled, cancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, err := normalizeNativeSaveResult(test.path, test.err)
			if path != test.path || err != test.wantErr {
				t.Fatalf("native selection = (%q, %v), want (%q, %v)", path, err, test.path, test.wantErr)
			}
		})
	}
}
