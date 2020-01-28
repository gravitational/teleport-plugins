/*
Copyright 2019 Gravitational, Inc.

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

package local

import (
	"bytes"
	"context"
	"time"

	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/services"

	"github.com/gravitational/trace"
)

// DynamicAccessService manages dynamic RBAC
type DynamicAccessService struct {
	backend.Backend
}

// NewDynamicAccessService returns new dynamic access service instance
func NewDynamicAccessService(backend backend.Backend) *DynamicAccessService {
	return &DynamicAccessService{Backend: backend}
}

func (s *DynamicAccessService) CreateAccessRequest(ctx context.Context, req services.AccessRequest) error {
	if err := req.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	item, err := itemFromAccessRequest(req)
	if err != nil {
		return trace.Wrap(err)
	}
	if _, err := s.Create(ctx, item); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (s *DynamicAccessService) SetAccessRequestState(ctx context.Context, name string, state services.RequestState) error {
	// Setting state is attempted multiple times in the event of concurrent writes.
	for i := 0; i < maxCmpAttempts; i++ {
		item, err := s.Get(ctx, accessRequestKey(name))
		if err != nil {
			if trace.IsNotFound(err) {
				return trace.NotFound("cannot set state of access request %q (not found)", name)
			}
			return trace.Wrap(err)
		}
		req, err := itemToAccessRequest(*item)
		if err != nil {
			return trace.Wrap(err)
		}
		if err := req.SetState(state); err != nil {
			return trace.Wrap(err)
		}
		// approved requests should have a resource expiry which matches
		// the underlying access expiry.
		if state.IsApproved() {
			req.SetExpiry(req.GetAccessExpiry())
		}
		newItem, err := itemFromAccessRequest(req)
		if err != nil {
			return trace.Wrap(err)
		}
		if _, err := s.CompareAndSwap(ctx, *item, newItem); err != nil {
			if trace.IsCompareFailed(err) {
				continue
			}
			return trace.Wrap(err)
		}
		return nil
	}
	return trace.CompareFailed("too many concurrent writes to access request %s", name)
}

func (s *DynamicAccessService) GetAccessRequest(ctx context.Context, name string) (services.AccessRequest, error) {
	item, err := s.Get(ctx, accessRequestKey(name))
	if err != nil {
		if trace.IsNotFound(err) {
			return nil, trace.NotFound("access request %q not found", name)
		}
		return nil, trace.Wrap(err)
	}
	req, err := itemToAccessRequest(*item)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return req, nil
}

func (s *DynamicAccessService) GetAccessRequests(ctx context.Context, filter services.AccessRequestFilter) ([]services.AccessRequest, error) {
	// Filters which specify ID are a special case since they will match exactly zero or one
	// possible requests.
	if filter.ID != "" {
		req, err := s.GetAccessRequest(ctx, filter.ID)
		if err != nil {
			if trace.IsNotFound(err) {
				// Caller is going to need to check the length of the returned slice anyway,
				// so propagating a NotFoundError is redundant.
				return nil, nil
			}
			return nil, trace.Wrap(err)
		}
		if !filter.Match(req) {
			return nil, nil
		}
		return []services.AccessRequest{req}, nil
	}
	result, err := s.GetRange(ctx, backend.Key(accessRequestsPrefix), backend.RangeEnd(backend.Key(accessRequestsPrefix)), backend.NoLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var requests []services.AccessRequest
	for _, item := range result.Items {
		if !bytes.HasSuffix(item.Key, []byte(paramsPrefix)) {
			continue
		}
		req, err := itemToAccessRequest(item)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if !filter.Match(req) {
			continue
		}
		requests = append(requests, req)
	}
	return requests, nil
}

func (s *DynamicAccessService) DeleteAccessRequest(ctx context.Context, name string) error {
	err := s.Delete(ctx, accessRequestKey(name))
	if err != nil {
		if trace.IsNotFound(err) {
			return trace.NotFound("cannot delete access request %q (not found)", name)
		}
		return trace.Wrap(err)
	}
	return nil
}

func (s *DynamicAccessService) DeleteAllAccessRequests(ctx context.Context) error {
	return trace.Wrap(s.DeleteRange(ctx, backend.Key(accessRequestsPrefix), backend.RangeEnd(backend.Key(accessRequestsPrefix))))
}

func (s *DynamicAccessService) UpsertAccessRequest(ctx context.Context, req services.AccessRequest) error {
	if err := req.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	item, err := itemFromAccessRequest(req)
	if err != nil {
		return trace.Wrap(err)
	}
	if _, err := s.Put(ctx, item); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (s *DynamicAccessService) GetPluginData(ctx context.Context, filter services.PluginDataFilter) ([]services.PluginData, error) {
	switch filter.Kind {
	case services.KindAccessRequest:
		data, err := s.getAccessRequestPluginData(ctx, filter)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return data, nil
	default:
		return nil, trace.BadParameter("unsupported resource kind %q", filter.Kind)
	}
}

func (s *DynamicAccessService) getAccessRequestPluginData(ctx context.Context, filter services.PluginDataFilter) ([]services.PluginData, error) {
	// Filters which specify Resource are a special case since they will match exactly zero or one
	// possible PluginData instances.
	if filter.Resource != "" {
		item, err := s.Get(ctx, pluginDataKey(services.KindAccessRequest, filter.Resource))
		if err != nil {
			if trace.IsNotFound(err) {
				// Caller is going to need to check the length of the returned slice anyway,
				// so propagating a NotFoundError is redundant.
				return nil, nil
			}
			return nil, trace.Wrap(err)
		}
		data, err := itemToPluginData(*item)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if !filter.Match(data) {
			return nil, nil
		}
		return []services.PluginData{data}, nil
	}
	prefix := backend.Key(pluginDataPrefix, services.KindAccessRequest)
	result, err := s.GetRange(ctx, prefix, backend.RangeEnd(prefix), backend.NoLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var matches []services.PluginData
	for _, item := range result.Items {
		if !bytes.HasSuffix(item.Key, []byte(paramsPrefix)) {
			continue
		}
		data, err := itemToPluginData(item)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if !filter.Match(data) {
			continue
		}
		matches = append(matches, data)
	}
	return matches, nil
}

func (s *DynamicAccessService) UpdatePluginData(ctx context.Context, params services.PluginDataUpdateParams) error {
	switch params.Kind {
	case services.KindAccessRequest:
		return trace.Wrap(s.updateAccessRequestPluginData(ctx, params))
	default:
		return trace.BadParameter("unsupported resource kind %q", params.Kind)
	}
}

func (s *DynamicAccessService) updateAccessRequestPluginData(ctx context.Context, params services.PluginDataUpdateParams) error {
	// Update is attempted multiple times in the event of concurrent writes.
	for i := 0; i < maxCmpAttempts; i++ {
		var create bool
		var data services.PluginData
		item, err := s.Get(ctx, pluginDataKey(services.KindAccessRequest, params.Resource))
		if err == nil {
			data, err = itemToPluginData(*item)
			if err != nil {
				return trace.Wrap(err)
			}
			create = false
		} else {
			if !trace.IsNotFound(err) {
				return trace.Wrap(err)
			}
			// In order to prevent orphaned plugin data, we automatically
			// configure new instances to expire shortly after the AccessRequest
			// to which they are associated.  This discrepency in expiry gives
			// plugins the ability to use stored data when handling an expiry
			// (OpDelete) event.
			req, err := s.GetAccessRequest(ctx, params.Resource)
			if err != nil {
				return trace.Wrap(err)
			}
			data, err = services.NewPluginData(params.Resource, services.KindAccessRequest)
			if err != nil {
				return trace.Wrap(err)
			}
			data.SetExpiry(req.GetAccessExpiry().Add(time.Hour))
			create = true
		}
		if err := data.Update(params); err != nil {
			return trace.Wrap(err)
		}
		if err := data.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		newItem, err := itemFromPluginData(data)
		if err != nil {
			return trace.Wrap(err)
		}
		if create {
			if _, err := s.Create(ctx, newItem); err != nil {
				if trace.IsAlreadyExists(err) {
					continue
				}
				return trace.Wrap(err)
			}
		} else {
			if _, err := s.CompareAndSwap(ctx, *item, newItem); err != nil {
				if trace.IsCompareFailed(err) {
					continue
				}
				return trace.Wrap(err)
			}
		}
		return nil
	}
	return trace.CompareFailed("too many concurrent writes to plugin data %s", params.Resource)
}

func itemFromAccessRequest(req services.AccessRequest) (backend.Item, error) {
	value, err := services.GetAccessRequestMarshaler().MarshalAccessRequest(req)
	if err != nil {
		return backend.Item{}, trace.Wrap(err)
	}
	return backend.Item{
		Key:     accessRequestKey(req.GetName()),
		Value:   value,
		Expires: req.Expiry(),
		ID:      req.GetResourceID(),
	}, nil
}

func itemToAccessRequest(item backend.Item) (services.AccessRequest, error) {
	req, err := services.GetAccessRequestMarshaler().UnmarshalAccessRequest(
		item.Value,
		services.WithResourceID(item.ID),
		services.WithExpires(item.Expires),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return req, nil
}

func itemFromPluginData(data services.PluginData) (backend.Item, error) {
	const maxObjectSize = 1000000 // slightly less than 1mb
	value, err := services.GetPluginDataMarshaler().MarshalPluginData(data)
	if err != nil {
		return backend.Item{}, trace.Wrap(err)
	}
	// enforce explicit limit on object size in order to prevent PluginData from
	// growing uncontrollably.
	if len(value) > maxObjectSize {
		return backend.Item{}, trace.BadParameter("plugin data size limit exceeded")
	}
	return backend.Item{
		Key:     pluginDataKey(data.GetSubKind(), data.GetName()),
		Value:   value,
		Expires: data.Expiry(),
		ID:      data.GetResourceID(),
	}, nil
}

func itemToPluginData(item backend.Item) (services.PluginData, error) {
	data, err := services.GetPluginDataMarshaler().UnmarshalPluginData(
		item.Value,
		services.WithResourceID(item.ID),
		services.WithExpires(item.Expires),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return data, nil
}

func accessRequestKey(name string) []byte {
	return backend.Key(accessRequestsPrefix, name, paramsPrefix)
}

func pluginDataKey(kind string, name string) []byte {
	return backend.Key(pluginDataPrefix, kind, name, paramsPrefix)
}

const (
	accessRequestsPrefix = "access_requests"
	pluginDataPrefix     = "plugin_data"
	maxCmpAttempts       = 7
)
