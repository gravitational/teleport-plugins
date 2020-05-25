package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/gravitational/trace"
	bolt "go.etcd.io/bbolt"
)

const (
	settingsBucketKey = "settings"
	issuesBucketKey   = "issues"
	hookIDKey         = "project-hook-id"
)

type DB struct {
	*bolt.DB
	projectID IntID
}

type Settings struct {
	*bolt.Bucket
}

type Issues struct {
	*bolt.Bucket
}

var ErrNoBucket = errors.New("No bucket created yet")

func OpenDB(path string, projectID IntID) (DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: time.Second,
	})
	if err != nil {
		return DB{}, trace.Wrap(err)
	}
	return DB{db, projectID}, nil
}

func (db *DB) projectBucketKey() []byte {
	return []byte(fmt.Sprintf("project:%d", db.projectID))
}

func (db *DB) updateProject(fn func(*bolt.Bucket) error) error {
	return db.Update(func(tx *bolt.Tx) error {
		projectBucket, err := tx.CreateBucketIfNotExists(db.projectBucketKey())
		if err != nil {
			return trace.Wrap(err)
		}
		return fn(projectBucket)
	})
}

func (db *DB) viewProject(fn func(*bolt.Bucket) error) error {
	return db.View(func(tx *bolt.Tx) error {
		projectBucket := tx.Bucket(db.projectBucketKey())
		if projectBucket == nil {
			return trace.Wrap(ErrNoBucket)
		}
		return fn(projectBucket)
	})
}

func (db *DB) UpdateSettings(fn func(Settings) error) error {
	return db.updateProject(func(bucket *bolt.Bucket) error {
		issuesBucket, err := bucket.CreateBucketIfNotExists([]byte(settingsBucketKey))
		if err != nil {
			return trace.Wrap(err)
		}
		return fn(Settings{issuesBucket})
	})
}

func (db *DB) ViewSettings(fn func(Settings) error) error {
	return db.viewProject(func(bucket *bolt.Bucket) error {
		settingsBucket := bucket.Bucket([]byte(settingsBucketKey))
		if settingsBucket == nil {
			return trace.Wrap(ErrNoBucket)
		}
		return fn(Settings{settingsBucket})
	})
}

func (db *DB) UpdateIssues(fn func(Issues) error) error {
	return db.updateProject(func(bucket *bolt.Bucket) error {
		issuesBucket, err := bucket.CreateBucketIfNotExists([]byte(issuesBucketKey))
		if err != nil {
			return trace.Wrap(err)
		}
		return fn(Issues{issuesBucket})
	})
}

func (db *DB) ViewIssues(fn func(Issues) error) error {
	return db.viewProject(func(bucket *bolt.Bucket) error {
		issuesBucket := bucket.Bucket([]byte(issuesBucketKey))
		if issuesBucket == nil {
			return trace.Wrap(ErrNoBucket)
		}
		return fn(Issues{issuesBucket})
	})
}

func (s *Settings) HookID() IntID {
	return BytesToIntID(s.Get([]byte(hookIDKey)))
}

func (s *Settings) SetHookID(id IntID) error {
	return s.Put([]byte(hookIDKey), IntIDToBytes(id))
}

func (s *Settings) labelKey(key string) []byte {
	return []byte(fmt.Sprintf("label/%s:name", key))
}

func (s *Settings) GetLabel(key string) string {
	return string(s.Get(s.labelKey(key)))
}

func (s *Settings) SetLabel(key string, label string) error {
	return s.Put(s.labelKey(key), []byte(label))
}

func (s *Settings) GetLabels(keys ...string) map[string]string {
	mapping := make(map[string]string)
	for _, key := range keys {
		mapping[key] = s.GetLabel(key)
	}
	return mapping
}

func (s *Settings) SetLabels(mapping map[string]string) error {
	for key, value := range mapping {
		if err := s.SetLabel(key, value); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func (i *Issues) requestIDKey(issueID IntID) []byte {
	return []byte(fmt.Sprintf("%s:request-id", issueID))
}

func (i *Issues) GetRequestID(issueID IntID) string {
	return string(i.Get(i.requestIDKey(issueID)))
}

func (i *Issues) SetRequestID(issueID IntID, reqID string) error {
	return i.Put(i.requestIDKey(issueID), []byte(reqID))
}
