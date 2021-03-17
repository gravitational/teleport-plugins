/*
Copyright 2020 Gravitational, Inc.

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

package filesessions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/session"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

// UploaderConfig sets up configuration for uploader service
type UploaderConfig struct {
	// ScanDir is data directory with the uploads
	ScanDir string
	// Clock is the clock replacement
	Clock clockwork.Clock
	// Context is an optional context
	Context context.Context
	// ScanPeriod is a uploader dir scan period
	ScanPeriod time.Duration
	// ConcurrentUploads sets up how many parallel uploads to schedule
	ConcurrentUploads int
	// Streamer is upstream streamer to upload events to
	Streamer events.Streamer
	// EventsC is an event channel used to signal events
	// used in tests
	EventsC chan events.UploadEvent
	// Component is used for logging purposes
	Component string
}

// CheckAndSetDefaults checks and sets default values of UploaderConfig
func (cfg *UploaderConfig) CheckAndSetDefaults() error {
	if cfg.Streamer == nil {
		return trace.BadParameter("missing parameter Streamer")
	}
	if cfg.ScanDir == "" {
		return trace.BadParameter("missing parameter ScanDir")
	}
	if cfg.ConcurrentUploads <= 0 {
		cfg.ConcurrentUploads = defaults.UploaderConcurrentUploads
	}
	if cfg.ScanPeriod <= 0 {
		cfg.ScanPeriod = defaults.UploaderScanPeriod
	}
	if cfg.Context == nil {
		cfg.Context = context.Background()
	}
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewRealClock()
	}
	if cfg.Component == "" {
		cfg.Component = teleport.ComponentUpload
	}
	return nil
}

// NewUploader creates new disk based session logger
func NewUploader(cfg UploaderConfig) (*Uploader, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	// completer scans for uploads that have been initiated, but not completed
	// by the client (aborted or crashed) and completed them
	handler, err := NewHandler(Config{
		Directory: cfg.ScanDir,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	uploadCompleter, err := events.NewUploadCompleter(events.UploadCompleterConfig{
		Uploader:  handler,
		Unstarted: true,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(cfg.Context)
	uploader := &Uploader{
		uploadCompleter: uploadCompleter,
		cfg:             cfg,
		log: log.WithFields(log.Fields{
			trace.Component: cfg.Component,
		}),
		cancel:    cancel,
		ctx:       ctx,
		semaphore: make(chan struct{}, cfg.ConcurrentUploads),
		eventsCh:  make(chan events.UploadEvent, cfg.ConcurrentUploads),
	}
	return uploader, nil
}

// Uploader periodically scans session records in a folder.
//
// Once it finds the sessions it opens parallel upload streams
// to the streaming server.
//
// It keeps checkpoints of the upload state and resumes
// the upload that have been aborted.
//
// The uploader completes the sessions that have been
// abandoned longer than the grace period.
//
// It marks corrupted session files to skip their processing.
//
type Uploader struct {
	semaphore chan struct{}

	cfg             UploaderConfig
	log             *log.Entry
	uploadCompleter *events.UploadCompleter

	cancel   context.CancelFunc
	ctx      context.Context
	eventsCh chan events.UploadEvent
}

func (u *Uploader) writeSessionError(sessionID session.ID, err error) error {
	if sessionID == "" {
		return trace.BadParameter("missing session ID")
	}
	path := u.sessionErrorFilePath(sessionID)
	return trace.ConvertSystemError(ioutil.WriteFile(path, []byte(err.Error()), 0600))
}

func (u *Uploader) checkSessionError(sessionID session.ID) (bool, error) {
	if sessionID == "" {
		return false, trace.BadParameter("missing session ID")
	}
	_, err := os.Stat(u.sessionErrorFilePath(sessionID))
	if err != nil {
		err = trace.ConvertSystemError(err)
		if trace.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Serve runs the uploader until stopped
func (u *Uploader) Serve() error {
	backoff, err := utils.NewLinear(utils.LinearConfig{
		Step:  u.cfg.ScanPeriod,
		Max:   u.cfg.ScanPeriod * 100,
		Clock: u.cfg.Clock,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	for {
		select {
		case <-u.ctx.Done():
			return nil
			// Successful and failed upload events are used to speed up and
			// slow down the scans and uploads.
		case event := <-u.eventsCh:
			switch {
			case event.Error == nil:
				backoff.ResetToDelay()
			case isSessionError(event.Error):
				u.log.WithError(event.Error).Warningf(
					"Failed to read session recording %v, will skip future uploads.", event.SessionID)
				if err := u.writeSessionError(session.ID(event.SessionID), event.Error); err != nil {
					u.log.WithError(err).Warningf(
						"Failed to write session %v error.", event.SessionID)
				}
			default:
				backoff.Inc()
				u.log.WithError(event.Error).Warningf(
					"Backing off, will retry after %v.", backoff.Duration())
			}
			// forward the event to channel that used in tests
			if u.cfg.EventsC != nil {
				select {
				case u.cfg.EventsC <- event:
				default:
					u.log.Warningf("Skip send event on a blocked channel.")
				}
			}
		// Tick at scan period but slow down (and speeds up) on errors.
		case <-backoff.After():
			var failed bool
			if err := u.uploadCompleter.CheckUploads(u.ctx); err != nil {
				if trace.Unwrap(err) != errContext {
					failed = true
					u.log.WithError(err).Warningf("Completer scan failed.")
				}
			}
			if _, err := u.Scan(); err != nil {
				if trace.Unwrap(err) != errContext {
					failed = true
					u.log.WithError(err).Warningf("Uploader scan failed.")
				}
			}
			if failed {
				backoff.Inc()
				u.log.Debugf("Scan failed, backing off, will retry after %v.", backoff.Duration())
			} else {
				backoff.ResetToDelay()
			}
		}
	}
}

// ScanStats provides scan statistics,
// used in tests
type ScanStats struct {
	// Scanned is how many uploads have been scanned
	Scanned int
	// Started is how many uploads have been started
	Started int
}

// Scan scans the streaming directory and uploads recordings
func (u *Uploader) Scan() (*ScanStats, error) {
	files, err := ioutil.ReadDir(u.cfg.ScanDir)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	var stats ScanStats
	for i := range files {
		fi := files[i]
		if fi.IsDir() {
			continue
		}
		ext := filepath.Ext(fi.Name())
		if ext == checkpointExt || ext == errorExt {
			continue
		}
		stats.Scanned++
		if err := u.startUpload(fi.Name()); err != nil {
			if trace.IsCompareFailed(err) {
				u.log.Debugf("Scan is skipping recording %v that is locked by another process.", fi.Name())
				continue
			}
			if trace.IsNotFound(err) {
				u.log.Debugf("Recording %v was uploaded by another process.", fi.Name())
				continue
			}
			if isSessionError(err) {
				u.log.WithError(err).Warningf("Skipped session recording %v.", fi.Name())
				continue
			}
			return nil, trace.Wrap(err)
		}
		stats.Started++
	}
	if stats.Scanned > 0 {
		u.log.Debugf("Scanned %v uploads, started %v in %v.", stats.Scanned, stats.Started, u.cfg.ScanDir)
	}
	return &stats, nil
}

// checkpointFilePath returns a path to checkpoint file for a session
func (u *Uploader) checkpointFilePath(sid session.ID) string {
	return filepath.Join(u.cfg.ScanDir, sid.String()+checkpointExt)
}

// sessionErrorFilePath returns a path to checkpoint file for a session
func (u *Uploader) sessionErrorFilePath(sid session.ID) string {
	return filepath.Join(u.cfg.ScanDir, sid.String()+errorExt)
}

// Close closes all operations
func (u *Uploader) Close() error {
	u.cancel()
	return u.uploadCompleter.Close()
}

type upload struct {
	sessionID      session.ID
	reader         *events.ProtoReader
	file           *os.File
	checkpointFile *os.File
}

// readStatus reads stream status
func (u *upload) readStatus() (*events.StreamStatus, error) {
	data, err := ioutil.ReadAll(u.checkpointFile)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	if len(data) == 0 {
		return nil, trace.NotFound("no status found")
	}
	var status events.StreamStatus
	err = utils.FastUnmarshal(data, &status)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &status, nil
}

// writeStatus writes stream status
func (u *upload) writeStatus(status events.StreamStatus) error {
	data, err := utils.FastMarshal(status)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = u.checkpointFile.Seek(0, 0)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	n, err := u.checkpointFile.Write(data)
	if err != nil {
		return trace.Wrap(err)
	}
	if n < len(data) {
		return trace.ConvertSystemError(io.ErrShortWrite)
	}
	return nil
}

// releaseFile releases file and associated resources
// in a correct order
func (u *upload) Close() error {
	return trace.NewAggregate(
		u.reader.Close(),
		utils.FSUnlock(u.file),
		u.file.Close(),
		utils.NilCloser(u.checkpointFile).Close(),
	)
}

func (u *upload) removeFiles() error {
	var errs []error
	if u.file != nil {
		errs = append(errs,
			trace.ConvertSystemError(os.Remove(u.file.Name())))
	}
	if u.checkpointFile != nil {
		errs = append(errs,
			trace.ConvertSystemError(os.Remove(u.checkpointFile.Name())))
	}
	return trace.NewAggregate(errs...)
}

func (u *Uploader) startUpload(fileName string) error {
	sessionID, err := sessionIDFromPath(fileName)
	if err != nil {
		return trace.Wrap(err)
	}
	sessionFilePath := filepath.Join(u.cfg.ScanDir, fileName)
	// Corrupted session records can clog the uploader
	// that will indefinitely try to upload them.
	isSessionError, err := u.checkSessionError(sessionID)
	if err != nil {
		return trace.Wrap(err)
	}
	if isSessionError {
		return sessionError{
			err: trace.BadParameter(
				"session recording %v is either corrupted or is using unsupported format, remove the file %v to correct the problem, remove the %v file to retry the upload",
				sessionID, sessionFilePath, u.sessionErrorFilePath(sessionID)),
		}
	}

	// Apparently, exclusive lock can be obtained only in RDWR mode on NFS
	sessionFile, err := os.OpenFile(sessionFilePath, os.O_RDWR, 0)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	if err := utils.FSTryWriteLock(sessionFile); err != nil {
		if e := sessionFile.Close(); e != nil {
			u.log.WithError(e).Warningf("Failed to close %v.", fileName)
		}
		return trace.Wrap(err)
	}

	upload := &upload{
		sessionID: sessionID,
		reader:    events.NewProtoReader(sessionFile),
		file:      sessionFile,
	}
	upload.checkpointFile, err = os.OpenFile(u.checkpointFilePath(sessionID), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		if err := upload.Close(); err != nil {
			u.log.WithError(err).Warningf("Failed to close upload.")
		}
		return trace.ConvertSystemError(err)
	}

	start := time.Now()
	if err := u.takeSemaphore(); err != nil {
		if err := upload.Close(); err != nil {
			u.log.WithError(err).Warningf("Failed to close upload.")
		}
		return trace.Wrap(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		u.log.Debugf("Semaphore acquired in %v for upload %v.", time.Since(start), fileName)
	}
	go func() {
		if err := u.upload(upload); err != nil {
			u.log.WithError(err).Warningf("Upload failed.")
			u.emitEvent(events.UploadEvent{
				SessionID: string(upload.sessionID),
				Error:     err,
				Created:   u.cfg.Clock.Now().UTC(),
			})
			return
		}
		u.emitEvent(events.UploadEvent{
			SessionID: string(upload.sessionID),
			Created:   u.cfg.Clock.Now().UTC(),
		})

	}()
	return nil
}

func (u *Uploader) upload(up *upload) error {
	defer u.releaseSemaphore()
	defer func() {
		if err := up.Close(); err != nil {
			u.log.WithError(err).Warningf("Failed to close upload.")
		}
	}()

	var stream events.Stream
	status, err := up.readStatus()
	if err != nil {
		if !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		stream, err = u.cfg.Streamer.CreateAuditStream(u.ctx, up.sessionID)
		if err != nil {
			return trace.Wrap(err)
		}
	} else {
		stream, err = u.cfg.Streamer.ResumeAuditStream(u.ctx, up.sessionID, status.UploadID)
		if err != nil {
			if !trace.IsNotFound(err) {
				return trace.Wrap(err)
			}
			u.log.WithError(err).Warningf(
				"Upload for sesion %v, upload ID %v is not found starting a new upload from scratch.",
				up.sessionID, status.UploadID)
			status = nil
			stream, err = u.cfg.Streamer.CreateAuditStream(u.ctx, up.sessionID)
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	defer func() {
		if err := stream.Close(u.ctx); err != nil {
			if trace.Unwrap(err) != io.EOF {
				u.log.WithError(err).Debugf("Failed to close stream.")
			}
		}
	}()

	// The call to CreateAuditStream is async. To learn
	// if it was successful get the first status update
	// sent by the server after create.
	select {
	case <-stream.Status():
	case <-time.After(defaults.NetworkRetryDuration):
		return trace.ConnectionProblem(nil, "timeout waiting for stream status update")
	case <-u.ctx.Done():
		return trace.ConnectionProblem(u.ctx.Err(), "operation has been cancelled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go u.monitorStreamStatus(u.ctx, up, stream, cancel)

	for {
		event, err := up.reader.Read(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return sessionError{err: trace.Wrap(err)}
		}
		// skip events that have been already submitted
		if status != nil && event.GetIndex() <= status.LastEventIndex {
			continue
		}
		if err := stream.EmitAuditEvent(u.ctx, event); err != nil {
			return trace.Wrap(err)
		}
	}

	if err := stream.Complete(u.ctx); err != nil {
		u.log.WithError(err).Error("Failed to complete upload.")
		return trace.Wrap(err)
	}

	// make sure that checkpoint writer goroutine finishes
	// before the files are closed to avoid async writes
	// the timeout is a defensive measure to avoid blocking
	// indefinitely in case of unforeseen error (e.g. write taking too long)
	wctx, wcancel := context.WithTimeout(ctx, defaults.DefaultDialTimeout)
	defer wcancel()

	<-wctx.Done()
	if errors.Is(wctx.Err(), context.DeadlineExceeded) {
		u.log.WithError(wctx.Err()).Warningf(
			"Checkpoint function failed to complete the write due to timeout. Possible slow disk write.")
	}

	// In linux it is possible to remove a file while holding a file descriptor
	if err := up.removeFiles(); err != nil {
		if !trace.IsNotFound(err) {
			u.log.WithError(err).Warningf("Failed to remove session files.")
		}
	}
	return nil
}

// monitorStreamStatus monitors stream's status
// and checkpoints the stream
func (u *Uploader) monitorStreamStatus(ctx context.Context, up *upload, stream events.Stream, cancel context.CancelFunc) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stream.Done():
			return
		case status := <-stream.Status():
			if err := up.writeStatus(status); err != nil {
				u.log.WithError(err).Debugf("Got stream status: %v.", status)
			}
		}
	}
}

var errContext = fmt.Errorf("context has closed")

func (u *Uploader) takeSemaphore() error {
	select {
	case u.semaphore <- struct{}{}:
		return nil
	case <-u.ctx.Done():
		return errContext
	}
}

func (u *Uploader) releaseSemaphore() error {
	select {
	case <-u.semaphore:
		return nil
	case <-u.ctx.Done():
		return errContext
	}
}

func (u *Uploader) emitEvent(e events.UploadEvent) {
	// This channel is used by scanner to slow down/speed up.
	select {
	case u.eventsCh <- e:
	default:
		// It's OK to drop the event if the Scan is overloaded.
		// These events are used in tests and to speed up and slow down
		// scans, so lost events will have little impact on the logic.
	}
}

func isSessionError(err error) bool {
	_, ok := trace.Unwrap(err).(sessionError)
	return ok
}

// sessionError highlights problems with session
// playback, corrupted files or incompatible disk format
type sessionError struct {
	err error
}

func (s sessionError) Error() string {
	return fmt.Sprintf(
		"session file could be corrupted or is using unsupported format: %v", s.err.Error())
}
