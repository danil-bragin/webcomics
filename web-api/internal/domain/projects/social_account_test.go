package projects

import (
	"testing"
)

func TestNewSocialAccount_RequiresPlatform(t *testing.T) {
	if _, err := NewSocialAccount("", "label", "/p", nil); err != ErrSocialAccountNoPlatform {
		t.Errorf("expected ErrSocialAccountNoPlatform, got %v", err)
	}
	if _, err := NewSocialAccount("   ", "label", "/p", nil); err != ErrSocialAccountNoPlatform {
		t.Errorf("expected ErrSocialAccountNoPlatform for whitespace, got %v", err)
	}
}

func TestNewSocialAccount_OK(t *testing.T) {
	acct, err := NewSocialAccount("youtube_selenium", "main", "/path/to/profile", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("NewSocialAccount: %v", err)
	}
	if acct.Platform() != "youtube_selenium" {
		t.Errorf("platform: %s", acct.Platform())
	}
	if acct.FirefoxProfilePath() != "/path/to/profile" {
		t.Errorf("path: %s", acct.FirefoxProfilePath())
	}
	if acct.Extra()["k"] != "v" {
		t.Errorf("extra: %v", acct.Extra())
	}
}

func TestSocialAccount_UpdatePreservesEmptyPlatform(t *testing.T) {
	acct, _ := NewSocialAccount("twitter_selenium", "old", "/p1", nil)
	acct.Update("", "new label", "/p2", map[string]any{"new": true})
	if acct.Platform() != "twitter_selenium" {
		t.Errorf("empty platform should preserve previous: %s", acct.Platform())
	}
	if acct.Label() != "new label" {
		t.Errorf("label: %s", acct.Label())
	}
	if acct.FirefoxProfilePath() != "/p2" {
		t.Errorf("path: %s", acct.FirefoxProfilePath())
	}
	if acct.Extra()["new"] != true {
		t.Errorf("extra: %v", acct.Extra())
	}
}

func TestSocialAccount_NilExtraNormalisesToMap(t *testing.T) {
	acct, _ := NewSocialAccount("youtube_selenium", "", "", nil)
	if acct.Extra() == nil {
		t.Errorf("Extra should never be nil")
	}
}
