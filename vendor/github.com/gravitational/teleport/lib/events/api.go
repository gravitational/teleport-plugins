/*
Copyright 2015-2020 Gravitational, Inc.

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

package events

import (
	"context"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/gravitational/teleport/lib/session"

	"github.com/gravitational/trace"
)

const (
	// EventType is event type/kind
	EventType = "event"
	// EventID is a unique event identifier
	EventID = "uid"
	// EventCode is a code that uniquely identifies a particular event type
	EventCode = "code"
	// EventTime is event time
	EventTime = "time"
	// EventLogin is OS login
	EventLogin = "login"
	// EventUser is teleport user name
	EventUser = "user"
	// EventProtocol specifies protocol that was captured
	EventProtocol = "proto"
	// EventProtocolsSSH specifies SSH as a type of captured protocol
	EventProtocolSSH = "ssh"
	// EventProtocolKube specifies kubernetes as a type of captured protocol
	EventProtocolKube = "kube"
	// LocalAddr is a target address on the host
	LocalAddr = "addr.local"
	// RemoteAddr is a client (user's) address
	RemoteAddr = "addr.remote"
	// EventCursor is an event ID (used as cursor value for enumeration, not stored)
	EventCursor = "id"

	// EventIndex is an event index as received from the logging server
	EventIndex = "ei"

	// EventNamespace is a namespace of the session event
	EventNamespace = "namespace"

	// SessionPrintEvent event happens every time a write occurs to
	// temirnal I/O during a session
	SessionPrintEvent = "print"

	// SessionPrintEventBytes says how many bytes have been written into the session
	// during "print" event
	SessionPrintEventBytes = "bytes"

	// SessionEventTimestamp is an offset (in milliseconds) since the beginning of the
	// session when the terminal IO event happened
	SessionEventTimestamp = "ms"

	// SessionEvent indicates that session has been initiated
	// or updated by a joining party on the server
	SessionStartEvent = "session.start"

	// SessionEndEvent indicates that a session has ended
	SessionEndEvent = "session.end"

	// SessionUploadEvent indicates that session has been uploaded to the external storage
	SessionUploadEvent = "session.upload"

	// URL is used for a session upload URL
	URL = "url"

	// SessionEventID is a unique UUID of the session.
	SessionEventID = "sid"

	// SessionServerID is the UUID of the server the session occurred on.
	SessionServerID = "server_id"

	// SessionServerHostname is the hostname of the server the session occurred on.
	SessionServerHostname = "server_hostname"

	// SessionServerAddr is the address of the server the session occurred on.
	SessionServerAddr = "server_addr"

	// SessionStartTime is the timestamp at which the session began.
	SessionStartTime = "session_start"

	// SessionEndTime is the timestamp at which the session ended.
	SessionEndTime = "session_stop"

	// SessionEnhancedRecording is used to indicate if the recording was an
	// enhanced recording or not.
	SessionEnhancedRecording = "enhanced_recording"

	// SessionInteractive is used to indicate if the session was interactive
	// (has PTY attached) or not (exec session).
	SessionInteractive = "interactive"

	// SessionParticipants is a list of participants in the session.
	SessionParticipants = "participants"

	// SessionServerLabels are the labels (static and dynamic) of the server the
	// session occurred on.
	SessionServerLabels = "server_labels"

	// SessionByteOffset is the number of bytes written to session stream since
	// the beginning
	SessionByteOffset = "offset"

	// SessionJoinEvent indicates that someone joined a session
	SessionJoinEvent = "session.join"
	// SessionLeaveEvent indicates that someone left a session
	SessionLeaveEvent = "session.leave"

	// Data transfer events.
	SessionDataEvent = "session.data"
	DataTransmitted  = "tx"
	DataReceived     = "rx"

	// ClientDisconnectEvent is emitted when client is disconnected
	// by the server due to inactivity or any other reason
	ClientDisconnectEvent = "client.disconnect"

	// Reason is a field that specifies reason for event, e.g. in disconnect
	// event it explains why server disconnected the client
	Reason = "reason"

	// UserLoginEvent indicates that a user logged into web UI or via tsh
	UserLoginEvent = "user.login"
	// LoginMethod is the event field indicating how the login was performed
	LoginMethod = "method"
	// LoginMethodLocal represents login with username/password
	LoginMethodLocal = "local"
	// LoginMethodClientCert represents login with client certificate
	LoginMethodClientCert = "client.cert"
	// LoginMethodOIDC represents login with OIDC
	LoginMethodOIDC = "oidc"
	// LoginMethodSAML represents login with SAML
	LoginMethodSAML = "saml"
	// LoginMethodGithub represents login with Github
	LoginMethodGithub = "github"

	// UserUpdatedEvent is emitted when the user is updated.
	UserUpdatedEvent = "user.update"

	// UserDeleteEvent is emitted when the user is deleted.
	UserDeleteEvent = "user.delete"

	// UserCreateEvent is emitted when the user is created.
	UserCreateEvent = "user.create"

	// UserPasswordChangeEvent is when the user changes their own password.
	UserPasswordChangeEvent = "user.password_change"

	// UserExpires is when the user will expire.
	UserExpires = "expires"

	// UserRoles is a list of roles for the user.
	UserRoles = "roles"

	// IdentityAttributes is a map of user attributes
	// received from identity provider
	IdentityAttributes = "attributes"

	// UserConnector is the connector used to create the user.
	UserConnector = "connector"

	// AccessRequestCreateEvent is emitted when a new access request is created.
	AccessRequestCreateEvent = "access_request.create"
	// AccessRequestUpdateEvent is emitted when a request's state is updated.
	AccessRequestUpdateEvent = "access_request.update"
	// AccessRequestDelegator is used by teleport plugins to indicate the identity
	// which caused them to update state.
	AccessRequestDelegator = "delegator"
	// AccessRequestState is the state of a request.
	AccessRequestState = "state"
	// AccessRequestID is the ID of an access request.
	AccessRequestID = "id"

	// UpdatedBy indicates the user who modified some resource:
	//  - updating a request state
	//  - updating a user record
	UpdatedBy = "updated_by"

	// ResetPasswordTokenCreateEvent is emitted when a new reset password token is created.
	ResetPasswordTokenCreateEvent = "reset_password_token.create"
	// ResetPasswordTokenTTL is TTL of reset password token.
	ResetPasswordTokenTTL = "ttl"

	// FieldName contains name, e.g. resource name, etc.
	FieldName = "name"

	// ExecEvent is an exec command executed by script or user on
	// the server side
	ExecEvent        = "exec"
	ExecEventCommand = "command"
	ExecEventCode    = "exitCode"
	ExecEventError   = "exitError"

	// SubsystemEvent is the result of the execution of a subsystem.
	SubsystemEvent = "subsystem"
	SubsystemName  = "name"
	SubsystemError = "exitError"

	// X11 forwarding event
	X11ForwardEvent   = "x11-forward"
	X11ForwardSuccess = "success"
	X11ForwardErr     = "error"

	// Port forwarding event
	PortForwardEvent   = "port"
	PortForwardAddr    = "addr"
	PortForwardSuccess = "success"
	PortForwardErr     = "error"

	// AuthAttemptEvent is authentication attempt that either
	// succeeded or failed based on event status
	AuthAttemptEvent   = "auth"
	AuthAttemptSuccess = "success"
	AuthAttemptErr     = "error"
	AuthAttemptMessage = "message"

	// SCPEvent means data transfer that occurred on the server
	SCPEvent          = "scp"
	SCPPath           = "path"
	SCPLengh          = "len"
	SCPAction         = "action"
	SCPActionUpload   = "upload"
	SCPActionDownload = "download"

	// ResizeEvent means that some user resized PTY on the client
	ResizeEvent  = "resize"
	TerminalSize = "size" // expressed as 'W:H'

	// SessionUploadIndex is a very large number of the event index
	// to indicate that this is the last event in the chain
	// used for the last event of the sesion - session upload
	SessionUploadIndex = math.MaxInt32
	// SessionDataIndex is a very large number of the event index
	// to indicate one of the last session events, used to report
	// data transfer
	SessionDataIndex = math.MaxInt32 - 1

	// SessionCommandEvent is emitted when an executable is run within a session.
	SessionCommandEvent = "session.command"

	// SessionDiskEvent is emitted when a file is opened within an session.
	SessionDiskEvent = "session.disk"

	// SessionNetworkEvent is emitted when a network connection is initated with a
	// session.
	SessionNetworkEvent = "session.network"

	// PID is the ID of the process.
	PID = "pid"

	// PPID is the PID of the parent process.
	PPID = "ppid"

	// CgroupID is the internal cgroupv2 ID of the event.
	CgroupID = "cgroup_id"

	// Program is name of the executable.
	Program = "program"

	// Path is the full path to the executable.
	Path = "path"

	// Argv is the list of arguments to the program. Note, the first element does
	// not contain the name of the process.
	Argv = "argv"

	// ReturnCode is the return code of execve.
	ReturnCode = "return_code"

	// Flags are the flags passed to open.
	Flags = "flags"

	// SrcAddr is the source IP address of the connection.
	SrcAddr = "src_addr"

	// DstAddr is the destination IP address of the connection.
	DstAddr = "dst_addr"

	// DstPort is the destination port of the connection.
	DstPort = "dst_port"

	// TCPVersion is the version of TCP (4 or 6).
	TCPVersion = "version"

	// RoleCreatedEvent fires when role is created/updated.
	RoleCreatedEvent = "role.created"
	// RoleDeletedEvent fires when role is deleted.
	RoleDeletedEvent = "role.deleted"

	// TrustedClusterCreateEvent is the event for creating a trusted cluster.
	TrustedClusterCreateEvent = "trusted_cluster.create"
	// TrustedClusterDeleteEvent is the event for removing a trusted cluster.
	TrustedClusterDeleteEvent = "trusted_cluster.delete"
	// TrustedClusterTokenCreateEvent is the event for
	// creating new join token for a trusted cluster.
	TrustedClusterTokenCreateEvent = "trusted_cluster_token.create"

	// GithubConnectorCreatedEvent fires when a Github connector is created/updated.
	GithubConnectorCreatedEvent = "github.created"
	// GithubConnectorDeletedEvent fires when a Github connector is deleted.
	GithubConnectorDeletedEvent = "github.deleted"
	// OIDCConnectorCreatedEvent fires when OIDC connector is created/updated.
	OIDCConnectorCreatedEvent = "oidc.created"
	// OIDCConnectorDeletedEvent fires when OIDC connector is deleted.
	OIDCConnectorDeletedEvent = "oidc.deleted"
	// SAMLConnectorCreatedEvent fires when SAML connector is created/updated.
	SAMLConnectorCreatedEvent = "saml.created"
	// SAMLConnectorDeletedEvent fires when SAML connector is deleted.
	SAMLConnectorDeletedEvent = "saml.deleted"

	// SessionRejected fires when a user's attempt to create an authenticated
	// session has been rejected due to exceeding a session control limit.
	SessionRejectedEvent = "session.rejected"

	// AppSessionStartEvent is emitted when a user is issued an application certificate.
	AppSessionStartEvent = "app.session.start"

	// AppSessionChunkEvent is emitted at the start of a 5 minute chunk on each
	// proxy. This chunk is used to buffer 5 minutes of audit events at a time
	// for applications.
	AppSessionChunkEvent = "app.session.chunk"

	// AppSessionRequestEvent is an HTTP request and response.
	AppSessionRequestEvent = "app.session.request"

	// SessionRejectedReasonMaxConnections indicates that a session.rejected event
	// corresponds to enforcement of the max_connections control.
	SessionRejectedReasonMaxConnections = "max_connections limit reached"
	// SessionRejectedReasonMaxSessions indicates that a session.rejected event
	// corresponds to enforcement of the max_sessions control.
	SessionRejectedReasonMaxSessions = "max_sessions limit reached"

	// Maximum is an event field specifying a maximal value (e.g. the value
	// of `max_connections` for a `session.rejected` event).
	Maximum = "max"

	// KubeRequestEvent fires when a proxy handles a generic kubernetes
	// request.
	KubeRequestEvent = "kube.request"
)

const (
	// MaxChunkBytes defines the maximum size of a session stream chunk that
	// can be requested via AuditLog.GetSessionChunk(). Set to 5MB
	MaxChunkBytes = 1024 * 1024 * 5
)

const (
	// V1 is the V1 version of slice chunks API,
	// it is 0 because it was not defined before
	V1 = 0
	// V2 is the V2 version of slice chunks  API
	V2 = 2
	// V3 is almost like V2, but it assumes
	// that session recordings are being uploaded
	// at the end of the session, so it skips writing session event index
	// on the fly
	V3 = 3
)

// AuditEvent represents audit event
type AuditEvent interface {
	// ProtoMarshaler implements efficient
	// protobuf marshaling methods
	ProtoMarshaler

	// GetID returns unique event ID
	GetID() string
	// SetID sets unique event ID
	SetID(id string)

	// GetCode returns event short diagnostic code
	GetCode() string
	// SetCode sets unique event diagnostic code
	SetCode(string)

	// GetType returns event type
	GetType() string
	// SetCode sets unique type
	SetType(string)

	// GetTime returns event time
	GetTime() time.Time
	// SetTime sets event time
	SetTime(time.Time)

	// GetIndex gets event index - a non-unique
	// monotonically incremented number
	// in the event sequence
	GetIndex() int64
	// SetIndex sets event index
	SetIndex(idx int64)
}

// ProtoMarshaler implements marshaler interface
type ProtoMarshaler interface {
	// Size returns size of the object when marshaled
	Size() (n int)

	// MarshalTo marshals the object to sized buffer
	MarshalTo(dAtA []byte) (int, error)
}

// ServerMetadataGetter represents interface
// that provides information about its server id
type ServerMetadataGetter interface {
	// GetServerID returns event server ID
	GetServerID() string

	// GetServerNamespace returns event server namespace
	GetServerNamespace() string
}

// ServerMetadataSetter represents interface
// that provides information about its server id
type ServerMetadataSetter interface {
	// SetServerID sets server ID of the event
	SetServerID(string)

	// SetServerNamespace returns event server namespace
	SetServerNamespace(string)
}

// SessionMetadataGetter represents interface
// that provides information about events' session metadata
type SessionMetadataGetter interface {
	// GetSessionID returns event session ID
	GetSessionID() string
}

// SessionMetadataSetter represents interface
// that sets session metadata
type SessionMetadataSetter interface {
	// SetSessionID sets event session ID
	SetSessionID(string)
}

// SetCode is a shortcut that sets code for the audit event
func SetCode(event AuditEvent, code string) AuditEvent {
	event.SetCode(code)
	return event
}

// Emitter creates and manages audit log streams
type Emitter interface {
	// Emit emits a single audit event
	EmitAuditEvent(context.Context, AuditEvent) error
}

// Streamer creates and resumes event streams for session IDs
type Streamer interface {
	// CreateAuditStream creates event stream
	CreateAuditStream(context.Context, session.ID) (Stream, error)
	// ResumeAuditStream resumes the stream for session upload that
	// has not been completed yet.
	ResumeAuditStream(ctx context.Context, sid session.ID, uploadID string) (Stream, error)
}

// StreamPart represents uploaded stream part
type StreamPart struct {
	// Number is a part number
	Number int64
	// ETag is a part e-tag
	ETag string
}

// StreamUpload represents stream multipart upload
type StreamUpload struct {
	// ID is unique upload ID
	ID string
	// SessionID is a session ID of the upload
	SessionID session.ID
	// Initiated contains the timestamp of when the upload
	// was initiated, not always initialized
	Initiated time.Time
}

// String returns user friendly representation of the upload
func (u StreamUpload) String() string {
	return fmt.Sprintf("Upload(session=%v, id=%v, initiated=%v)", u.SessionID, u.ID, u.Initiated)
}

// CheckAndSetDefaults checks and sets default values
func (u *StreamUpload) CheckAndSetDefaults() error {
	if u.ID == "" {
		return trace.BadParameter("missing parameter ID")
	}
	if u.SessionID == "" {
		return trace.BadParameter("missing parameter SessionID")
	}
	return nil
}

// MultipartUploader handles multipart uploads and downloads for session streams
type MultipartUploader interface {
	// CreateUpload creates a multipart upload
	CreateUpload(ctx context.Context, sessionID session.ID) (*StreamUpload, error)
	// CompleteUpload completes the upload
	CompleteUpload(ctx context.Context, upload StreamUpload, parts []StreamPart) error
	// UploadPart uploads part and returns the part
	UploadPart(ctx context.Context, upload StreamUpload, partNumber int64, partBody io.ReadSeeker) (*StreamPart, error)
	// ListParts returns all uploaded parts for the completed upload in sorted order
	ListParts(ctx context.Context, upload StreamUpload) ([]StreamPart, error)
	// ListUploads lists uploads that have been initiated but not completed with
	// earlier uploads returned first
	ListUploads(ctx context.Context) ([]StreamUpload, error)
}

// Stream is used to create continuous ordered sequence of events
// associated with a session.
type Stream interface {
	// Emitter allows stream to emit audit event in the context of the event stream
	Emitter
	// Status returns channel broadcasting updates about the stream state:
	// last event index that was uploaded and the upload ID
	Status() <-chan StreamStatus
	// Done returns channel closed when streamer is closed
	// should be used to detect sending errors
	Done() <-chan struct{}
	// Complete closes the stream and marks it finalized,
	// releases associated resources, in case of failure,
	// closes this stream on the client side
	Complete(ctx context.Context) error
	// Close flushes non-uploaded flight stream data without marking
	// the stream completed and closes the stream instance
	Close(ctx context.Context) error
}

// StreamWriter implements io.Writer to be plugged into the multi-writer
// associated with every session. It forwards session stream to the audit log
type StreamWriter interface {
	io.Writer
	Stream
}

// StreamEmitter supports submitting single events and streaming
// session events
type StreamEmitter interface {
	Emitter
	Streamer
}

// IAuditLog is the primary (and the only external-facing) interface for AuditLogger.
// If you wish to implement a different kind of logger (not filesystem-based), you
// have to implement this interface
type IAuditLog interface {
	// Closer releases connection and resources associated with log if any
	io.Closer

	// EmitAuditEventLegacy emits audit in legacy format
	// DELETE IN: 5.0.0
	EmitAuditEventLegacy(Event, EventFields) error

	// DELETE IN: 2.7.0
	// This method is no longer necessary as nodes and proxies >= 2.7.0
	// use UploadSessionRecording method.
	// PostSessionSlice sends chunks of recorded session to the event log
	PostSessionSlice(SessionSlice) error

	// UploadSessionRecording uploads session recording to the audit server
	UploadSessionRecording(r SessionRecording) error

	// GetSessionChunk returns a reader which can be used to read a byte stream
	// of a recorded session starting from 'offsetBytes' (pass 0 to start from the
	// beginning) up to maxBytes bytes.
	//
	// If maxBytes > MaxChunkBytes, it gets rounded down to MaxChunkBytes
	GetSessionChunk(namespace string, sid session.ID, offsetBytes, maxBytes int) ([]byte, error)

	// Returns all events that happen during a session sorted by time
	// (oldest first).
	//
	// after tells to use only return events after a specified cursor Id
	//
	// This function is usually used in conjunction with GetSessionReader to
	// replay recorded session streams.
	GetSessionEvents(namespace string, sid session.ID, after int, includePrintEvents bool) ([]EventFields, error)

	// SearchEvents is a flexible way to find events. The format of a query string
	// depends on the implementing backend. A recommended format is urlencoded
	// (good enough for Lucene/Solr)
	//
	// Pagination is also defined via backend-specific query format.
	//
	// The only mandatory requirement is a date range (UTC). Results must always
	// show up sorted by date (newest first)
	SearchEvents(fromUTC, toUTC time.Time, query string, limit int) ([]EventFields, error)

	// SearchSessionEvents returns session related events only. This is used to
	// find completed session.
	SearchSessionEvents(fromUTC time.Time, toUTC time.Time, limit int) ([]EventFields, error)

	// WaitForDelivery waits for resources to be released and outstanding requests to
	// complete after calling Close method
	WaitForDelivery(context.Context) error
}

// EventFields instance is attached to every logged event
type EventFields map[string]interface{}

// String returns a string representation of an event structure
func (f EventFields) AsString() string {
	return fmt.Sprintf("%s: login=%s, id=%v, bytes=%v",
		f.GetString(EventType),
		f.GetString(EventLogin),
		f.GetInt(EventCursor),
		f.GetInt(SessionPrintEventBytes))
}

// GetType returns the type (string) of the event
func (f EventFields) GetType() string {
	return f.GetString(EventType)
}

// GetID returns the unique event ID
func (f EventFields) GetID() string {
	return f.GetString(EventID)
}

// GetCode returns the event code
func (f EventFields) GetCode() string {
	return f.GetString(EventCode)
}

// GetTimestamp returns the event timestamp (when it was emitted)
func (f EventFields) GetTimestamp() time.Time {
	return f.GetTime(EventTime)
}

// GetString returns a string representation of a logged field
func (f EventFields) GetString(key string) string {
	val, found := f[key]
	if !found {
		return ""
	}
	v, _ := val.(string)
	return v
}

// GetString returns an int representation of a logged field
func (f EventFields) GetInt(key string) int {
	val, found := f[key]
	if !found {
		return 0
	}
	v, ok := val.(int)
	if !ok {
		f, ok := val.(float64)
		if ok {
			v = int(f)
		}
	}
	return v
}

// GetString returns an int representation of a logged field
func (f EventFields) GetTime(key string) time.Time {
	val, found := f[key]
	if !found {
		return time.Time{}
	}
	v, ok := val.(time.Time)
	if !ok {
		s := f.GetString(key)
		v, _ = time.Parse(time.RFC3339, s)
	}
	return v
}

// HasField returns true if the field exists in the event.
func (f EventFields) HasField(key string) bool {
	_, ok := f[key]
	return ok
}
