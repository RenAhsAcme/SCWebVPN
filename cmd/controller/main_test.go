package main

import "testing"

func TestControllerRequiresExplicitConfig(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("Controller accepted a missing config path")
	}
}
