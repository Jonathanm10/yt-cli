package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Profile struct {
	Name           string `json:"name"`
	BaseURL        string `json:"baseUrl,omitempty"`
	DefaultProject string `json:"defaultProject,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

type ProfilesFile struct {
	ActiveProfile string             `json:"activeProfile,omitempty"`
	Profiles      map[string]Profile `json:"profiles,omitempty"`
}

type Store struct {
	BaseDir string
}

func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "yt-cli"), nil
}

func NewStore(baseDir string) (*Store, error) {
	if strings.TrimSpace(baseDir) == "" {
		var err error
		baseDir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	return &Store{BaseDir: baseDir}, nil
}

func (s *Store) Ensure() error {
	return os.MkdirAll(filepath.Join(s.BaseDir, "credentials"), 0o700)
}

func (s *Store) profilesPath() string { return filepath.Join(s.BaseDir, "profiles.json") }

func (s *Store) tokenPath(profile string) string {
	return filepath.Join(s.BaseDir, "credentials", sanitizeProfileName(profile)+".token")
}

func (s *Store) LoadProfiles() (ProfilesFile, error) {
	if err := s.Ensure(); err != nil {
		return ProfilesFile{}, err
	}
	path := s.profilesPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProfilesFile{Profiles: map[string]Profile{}}, nil
		}
		return ProfilesFile{}, err
	}
	var pf ProfilesFile
	if err := json.Unmarshal(b, &pf); err != nil {
		return ProfilesFile{}, err
	}
	if pf.Profiles == nil {
		pf.Profiles = map[string]Profile{}
	}
	return pf, nil
}

func (s *Store) SaveProfiles(pf ProfilesFile) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	if pf.Profiles == nil {
		pf.Profiles = map[string]Profile{}
	}
	return atomicWriteJSON(s.profilesPath(), pf, 0o600)
}

func (s *Store) UpsertProfile(profile Profile) error {
	pf, err := s.LoadProfiles()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	existing, ok := pf.Profiles[profile.Name]
	if ok && profile.CreatedAt == "" {
		profile.CreatedAt = existing.CreatedAt
	}
	if profile.CreatedAt == "" {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	pf.Profiles[profile.Name] = profile
	if pf.ActiveProfile == "" {
		pf.ActiveProfile = profile.Name
	}
	return s.SaveProfiles(pf)
}

func (s *Store) SetActiveProfile(name string) error {
	pf, err := s.LoadProfiles()
	if err != nil {
		return err
	}
	if _, ok := pf.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	pf.ActiveProfile = name
	return s.SaveProfiles(pf)
}

func (s *Store) SaveToken(profileName, token string) error {
	if strings.TrimSpace(profileName) == "" {
		return errors.New("profile name is required")
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	path := s.tokenPath(profileName)
	return atomicWrite(path, []byte(strings.TrimSpace(token)+"\n"), 0o600)
}

func (s *Store) LoadToken(profileName string) (string, error) {
	if strings.TrimSpace(profileName) == "" {
		return "", nil
	}
	path := s.tokenPath(profileName)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("token file %s has unsafe permissions %o", path, info.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *Store) DeleteToken(profileName string) error {
	path := s.tokenPath(profileName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) DeleteProfile(name string) error {
	pf, err := s.LoadProfiles()
	if err != nil {
		return err
	}
	delete(pf.Profiles, name)
	if pf.ActiveProfile == name {
		pf.ActiveProfile = ""
		names := make([]string, 0, len(pf.Profiles))
		for profileName := range pf.Profiles {
			names = append(names, profileName)
		}
		sort.Strings(names)
		if len(names) > 0 {
			pf.ActiveProfile = names[0]
		}
	}
	if err := s.SaveProfiles(pf); err != nil {
		return err
	}
	return s.DeleteToken(name)
}

func (s *Store) HasToken(profileName string) bool {
	token, err := s.LoadToken(profileName)
	return err == nil && token != ""
}

func (s *Store) ListProfiles() ([]Profile, string, error) {
	pf, err := s.LoadProfiles()
	if err != nil {
		return nil, "", err
	}
	names := make([]string, 0, len(pf.Profiles))
	for name := range pf.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Profile, 0, len(names))
	for _, name := range names {
		out = append(out, pf.Profiles[name])
	}
	return out, pf.ActiveProfile, nil
}

func sanitizeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "default"
	}
	name = strings.ReplaceAll(name, string(filepath.Separator), "-")
	name = strings.ReplaceAll(name, "..", "-")
	return name
}

func atomicWriteJSON(path string, v any, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return atomicWrite(path, b, mode)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
