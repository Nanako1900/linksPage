package uaclass

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// sharedFixtures is the single UA corpus shared with the TypeScript
// classifier (web/src/shared/ua.ts), so the two can never drift apart.
var sharedFixtures = filepath.Join("..", "..", "web", "src", "test", "fixtures", "user-agents.json")

type uaFixture struct {
	Name   string `json:"name"`
	UA     string `json:"ua"`
	WeChat bool   `json:"inWeChat"`
	QQ     bool   `json:"inQQ"`
	Mobile bool   `json:"mobile"`
}

func loadFixtures(t *testing.T) []uaFixture {
	t.Helper()
	data, err := os.ReadFile(sharedFixtures)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var doc struct {
		Cases []uaFixture `json:"cases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	if len(doc.Cases) < 40 {
		t.Fatalf("expected at least 40 fixtures, got %d", len(doc.Cases))
	}
	return doc.Cases
}

func TestClassifyFixtures(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range loadFixtures(t) {
		if seen[f.Name] {
			t.Fatalf("duplicate fixture name %q", f.Name)
		}
		seen[f.Name] = true
		t.Run(f.Name, func(t *testing.T) {
			got := Classify(f.UA)
			want := Class{InWeChat: f.WeChat, InQQ: f.QQ, Mobile: f.Mobile}
			if got != want {
				t.Errorf("Classify() = %+v, want %+v", got, want)
			}
			if got.InApp() != (f.WeChat || f.QQ) {
				t.Errorf("InApp() = %v", got.InApp())
			}
		})
	}
}

func TestClassifyRules(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want Class
	}{
		{"marker only wechat", "MicroMessenger", Class{InWeChat: true}},
		{"marker only qq", "QQ/1", Class{InQQ: true}},
		{"mqqbrowser is not qq", "MQQBrowser/6.2", Class{}},
		{"qqbrowser is not qq", "QQBrowser/11", Class{}},
		{"qqtheme is not qq", "QQTheme/1000", Class{}},
		{"case sensitive wechat", "micromessenger", Class{}},
		{"both apps", "MicroMessenger/8 QQ/9", Class{InWeChat: true, InQQ: true}},
		{"mobi", "Mobi", Class{Mobile: true}},
		{"ipad", "iPad", Class{Mobile: true}},
		{"ipod", "iPod", Class{Mobile: true}},
		{"openharmony", "OpenHarmony", Class{Mobile: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.ua); got != tt.want {
				t.Errorf("Classify(%q) = %+v, want %+v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestInApp(t *testing.T) {
	tests := []struct {
		c    Class
		want bool
	}{
		{Class{}, false},
		{Class{Mobile: true}, false},
		{Class{InWeChat: true}, true},
		{Class{InQQ: true}, true},
	}
	for _, tt := range tests {
		if got := tt.c.InApp(); got != tt.want {
			t.Errorf("%+v.InApp() = %v, want %v", tt.c, got, tt.want)
		}
	}
}

func TestMobileMarkersIsCopy(t *testing.T) {
	m := MobileMarkers()
	m[0] = "changed"
	if MobileMarkers()[0] == "changed" {
		t.Fatal("MobileMarkers must return a copy")
	}
}
