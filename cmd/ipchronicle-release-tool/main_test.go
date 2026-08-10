package main

import "testing"

func TestRunRejectsIncompleteCommands(t *testing.T) {
	for _, arguments := range [][]string{
		nil,
		{"unknown"},
		{"create", "--directory", t.TempDir()},
		{"verify", "extra"},
	} {
		if err := run(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly accepted", arguments)
		}
	}
}
