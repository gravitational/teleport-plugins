/*
Copyright 2020-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

type DB struct{ *bolt.DB }
type SettingsBucket struct{ *bolt.Bucket }
type IssuesBucket struct{ *bolt.Bucket }

var ErrNoBucket = errors.New("No bucket created yet")

func OpenDB(path string) (DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: time.Second,
	})
	if err != nil {
		return DB{}, trace.Wrap(err)
	}
	return DB{db}, nil
}

func (db DB) projectBucketKey(projectID IntID) []byte {
	return []byte(fmt.Sprintf("project:%d", projectID))
}

func (db DB) updateProject(projectID IntID, fn func(*bolt.Bucket) error) error {
	return db.Update(func(tx *bolt.Tx) error {
		projectBucket, err := tx.CreateBucketIfNotExists(db.projectBucketKey(projectID))
		if err != nil {
			return trace.Wrap(err)
		}
		return fn(projectBucket)
	})
}

func (db DB) viewProject(projectID IntID, fn func(*bolt.Bucket) error) error {
	return db.View(func(tx *bolt.Tx) error {
		projectBucket := tx.Bucket(db.projectBucketKey(projectID))
		if projectBucket == nil {
			return trace.Wrap(ErrNoBucket)
		}
		return fn(projectBucket)
	})
}

func (db DB) UpdateSettings(projectID IntID, fn func(SettingsBucket) error) error {
	return db.updateProject(projectID, func(bucket *bolt.Bucket) error {
		bucket, err := bucket.CreateBucketIfNotExists([]byte(settingsBucketKey))
		if err != nil {
			return trace.Wrap(err)
		}
		return fn(SettingsBucket{bucket})
	})
}

func (db DB) ViewSettings(projectID IntID, fn func(SettingsBucket) error) error {
	return db.viewProject(projectID, func(bucket *bolt.Bucket) error {
		bucket = bucket.Bucket([]byte(settingsBucketKey))
		if bucket == nil {
			return trace.Wrap(ErrNoBucket)
		}
		return fn(SettingsBucket{bucket})
	})
}

func (db DB) UpdateIssues(projectID IntID, fn func(IssuesBucket) error) error {
	return db.updateProject(projectID, func(bucket *bolt.Bucket) error {
		bucket, err := bucket.CreateBucketIfNotExists([]byte(issuesBucketKey))
		if err != nil {
			return trace.Wrap(err)
		}
		return fn(IssuesBucket{bucket})
	})
}

func (db DB) ViewIssues(projectID IntID, fn func(IssuesBucket) error) error {
	return db.viewProject(projectID, func(bucket *bolt.Bucket) error {
		bucket = bucket.Bucket([]byte(issuesBucketKey))
		if bucket == nil {
			return trace.Wrap(ErrNoBucket)
		}
		return fn(IssuesBucket{bucket})
	})
}

func (s SettingsBucket) HookID() IntID {
	return BytesToIntID(s.Get([]byte(hookIDKey)))
}

func (s SettingsBucket) SetHookID(id IntID) error {
	return s.Put([]byte(hookIDKey), IntIDToBytes(id))
}

func (s SettingsBucket) labelKey(key string) []byte {
	return []byte(fmt.Sprintf("label/%s:name", key))
}

func (s SettingsBucket) GetLabel(key string) string {
	return string(s.Get(s.labelKey(key)))
}

func (s SettingsBucket) SetLabel(key string, label string) error {
	return s.Put(s.labelKey(key), []byte(label))
}

func (s SettingsBucket) GetLabels(keys ...string) map[string]string {
	mapping := make(map[string]string)
	for _, key := range keys {
		mapping[key] = s.GetLabel(key)
	}
	return mapping
}

func (s SettingsBucket) SetLabels(mapping map[string]string) error {
	for key, value := range mapping {
		if err := s.SetLabel(key, value); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func (i IssuesBucket) requestIDKey(issueID IntID) []byte {
	return []byte(fmt.Sprintf("%s:request-id", issueID))
}

func (i IssuesBucket) GetRequestID(issueID IntID) string {
	return string(i.Get(i.requestIDKey(issueID)))
}

func (i IssuesBucket) SetRequestID(issueID IntID, reqID string) error {
	return i.Put(i.requestIDKey(issueID), []byte(reqID))
}
