package cache

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	baseDir string
	mu      sync.RWMutex
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func (s *Store) Dir() string {
	return s.baseDir
}

func (s *Store) hostDir(host string) string {
	return filepath.Join(s.baseDir, ".cache", host)
}

func (s *Store) Get(host, key string) (*Entry, []byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.hostDir(host)
	metaPath := filepath.Join(dir, key+".meta.json")
	bodyPath := filepath.Join(dir, key+".body")

	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil // cache miss
		}
		return nil, nil, fmt.Errorf("reading meta: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(metaData, &entry); err != nil {
		log.Printf("resemble: corrupt cache entry %s, treating as miss: %v", metaPath, err)
		return nil, nil, nil
	}

	body, err := os.ReadFile(bodyPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("resemble: missing body file %s, treating as miss", bodyPath)
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading body: %w", err)
	}

	return &entry, body, nil
}

func (s *Store) Put(host, key string, entry *Entry, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.hostDir(host)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	entry.Key = key
	entry.Version = 1
	entry.Response.BodyFile = key + ".body"
	entry.Response.BodySize = int64(len(body))

	if entry.CachedAt.IsZero() {
		entry.CachedAt = time.Now().UTC()
	}
	if entry.LastRefreshed.IsZero() {
		entry.LastRefreshed = time.Now().UTC()
	}

	metaData, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling meta: %w", err)
	}

	metaPath := filepath.Join(dir, key+".meta.json")
	bodyPath := filepath.Join(dir, key+".body")

	if err := os.WriteFile(bodyPath, body, 0644); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("writing meta: %w", err)
	}

	return nil
}

func (s *Store) Delete(host, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.hostDir(host)
	metaPath := filepath.Join(dir, key+".meta.json")
	bodyPath := filepath.Join(dir, key+".body")

	os.Remove(metaPath)
	os.Remove(bodyPath)
	return nil
}

func (s *Store) DeleteByPattern(pattern string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cacheDir := filepath.Join(s.baseDir, ".cache")
	count := 0

	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		relPath, _ := filepath.Rel(cacheDir, path)
		matched, _ := filepath.Match(pattern, relPath)
		if !matched {
			// Also try matching just the filename
			matched, _ = filepath.Match(pattern, filepath.Base(path))
		}
		if matched {
			bodyPath := strings.TrimSuffix(path, ".meta.json") + ".body"
			os.Remove(path)
			os.Remove(bodyPath)
			count++
		}
		return nil
	})

	return count, err
}

type Stats struct {
	TotalEntries int
	TotalSize    int64
	Hosts        map[string]int
	StaleEntries int
}

func (s *Store) Stats(defaultTTL time.Duration) (*Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &Stats{
		Hosts: make(map[string]int),
	}

	cacheDir := filepath.Join(s.baseDir, ".cache")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return stats, nil
	}

	hostDirs, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("reading cache dir: %w", err)
	}

	for _, hostDir := range hostDirs {
		if !hostDir.IsDir() {
			continue
		}
		host := hostDir.Name()
		entries, err := os.ReadDir(filepath.Join(cacheDir, host))
		if err != nil {
			continue
		}

		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".meta.json") {
				stats.TotalEntries++
				stats.Hosts[host]++

				// Check staleness
				metaPath := filepath.Join(cacheDir, host, e.Name())
				data, err := os.ReadFile(metaPath)
				if err != nil {
					continue
				}
				var entry Entry
				if json.Unmarshal(data, &entry) == nil {
					if entry.IsStale(defaultTTL) {
						stats.StaleEntries++
					}
				}
			}
			if !strings.HasSuffix(e.Name(), ".meta.json") {
				info, err := e.Info()
				if err == nil {
					stats.TotalSize += info.Size()
				}
			}
		}
	}

	return stats, nil
}

func (s *Store) List() ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entries []Entry
	cacheDir := filepath.Join(s.baseDir, ".cache")

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return entries, nil
	}

	filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var entry Entry
		if json.Unmarshal(data, &entry) == nil {
			entries = append(entries, entry)
		}
		return nil
	})

	return entries, nil
}
