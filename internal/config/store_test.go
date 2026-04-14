package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSeparatesProfilesAndTokens(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertProfile(Profile{Name: "sandbox", BaseURL: "https://example.youtrack.cloud", DefaultProject: "SP"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveToken("sandbox", "perm:token"); err != nil {
		t.Fatal(err)
	}

	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if profiles.Profiles["sandbox"].BaseURL != "https://example.youtrack.cloud" {
		t.Fatalf("unexpected base URL: %#v", profiles.Profiles["sandbox"])
	}

	rawProfiles, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rawProfiles) == "perm:token" || string(rawProfiles) == "perm:token\n" {
		t.Fatalf("token leaked into profiles.json: %s", string(rawProfiles))
	}

	token, err := store.LoadToken("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if token != "perm:token" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func TestLoadTokenFailsOnUnsafePermissions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials", "sandbox.token")
	if err := os.WriteFile(path, []byte("perm:token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadToken("sandbox"); err == nil {
		t.Fatal("expected unsafe permission error")
	}
}
