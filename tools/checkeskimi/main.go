package main

import (
	"fmt"
	"strings"
	"github.com/DoimoJr/planck-proxy/internal/classify"
)

func main() {
	for _, target := range []string{"ittpx.eskimi.com", "dsp-ap.eskimi.com"} {
		t := strings.ToLower(target)
		for _, ai := range classify.AIDomains() {
			if strings.Contains(t, ai) {
				fmt.Printf("FP: %q matcha pattern AI %q\n", target, ai)
			}
		}
	}
}
