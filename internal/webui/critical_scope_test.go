package webui

import (
	"regexp"
	"strings"
	"testing"
)

// The critical CSS is unlayered, so it would override the app's layered
// component styles after hydration if it matched the app's class names.
// Every class or element rule except the base resets must be scoped to the
// fallback wrapper (webui_test checks the markup carries it).
func TestCriticalCSSIsScopedToFallback(t *testing.T) {
	ruleRe := regexp.MustCompile(`(?m)^([^@{}][^{}]*)\{`)
	base := map[string]bool{"*": true, "*::before": true, "*::after": true, "html": true, "body": true}
	for _, m := range ruleRe.FindAllStringSubmatch(CriticalCSS(), -1) {
		for _, sel := range strings.Split(m[1], ",") {
			sel = strings.TrimSpace(sel)
			if !base[sel] && !strings.HasPrefix(sel, fallbackScope+" ") {
				t.Errorf("critical CSS selector %q is not scoped to %s", sel, fallbackScope)
			}
		}
	}
}

const fallbackScope = ".lp-fb"
