package plugin_data

import (
	"context"

	"github.com/gravitational/trace"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/teleport/lib/auth/proto"
	"github.com/gravitational/teleport/lib/services"
)

// PluginDataInput is a general interface for custom user's structs which are serializable to raw maps.
type PluginDataInput interface {
	MarshalPluginDataMap() (map[string]string, error)
}

// PluginDataOutput is a general interface for custom user's structs which are de-serializable from raw maps.
type PluginDataOutput interface {
	UnmarshalPluginDataMap(map[string]string) error
}

// PluginDataMap is for cases when you don't want to declare a custom data type for your data.
// It's good for fast prototyping and also for debugging because anything could be (de)serialized from/to map.
type PluginDataMap map[string]string

type Client interface {
	// GetPluginData fetches plugin data of the specific resource.
	GetPluginData(ctx context.Context, kind string, resource string, data PluginDataOutput) error
	// UpdatePluginData updates plugin data of the specific resource comparing it with a previous value.
	UpdatePluginData(ctx context.Context, kind string, resource string, set PluginDataInput, expect PluginDataInput) error
}

type clt struct {
	clt    proto.AuthServiceClient
	plugin string
}

func NewClient(client proto.AuthServiceClient, plugin string) (Client, error) {
	return &clt{client, plugin}, nil
}

func (c *clt) GetPluginData(ctx context.Context, kind string, resource string, data PluginDataOutput) error {
	dataSeq, err := c.clt.GetPluginData(ctx, &services.PluginDataFilter{
		Kind:     kind,
		Resource: resource,
		Plugin:   c.plugin,
	})
	if err != nil {
		return utils.FromGRPC(err)
	}
	pluginDatas := dataSeq.GetPluginData()
	if len(pluginDatas) == 0 {
		return &trace.NotFoundError{Message: "No PluginData found"}
	}

	var pluginData services.PluginData = pluginDatas[0]
	entry := pluginData.Entries()[c.plugin]
	if entry == nil {
		// TODO: this one bothers me. I somehow want to differentiate "resource not found" and "resource's plugin data not found" errors
		return &trace.NotFoundError{Message: "No PluginData entry found for plugin"}
	}
	return data.UnmarshalPluginDataMap(entry.Data)
}

func (c *clt) UpdatePluginData(ctx context.Context, kind string, resource string, set PluginDataInput, expect PluginDataInput) (err error) {
	var setMap, expectMap map[string]string

	if set != nil {
		setMap, err = set.MarshalPluginDataMap()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if expect != nil {
		expectMap, err = expect.MarshalPluginDataMap()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	_, err = c.clt.UpdatePluginData(ctx, &services.PluginDataUpdateParams{
		Kind:     kind,
		Resource: resource,
		Plugin:   c.plugin,
		Set:      setMap,
		Expect:   expectMap,
	})
	return utils.FromGRPC(err)
}

func (m *PluginDataMap) UnmarshalPluginDataMap(data map[string]string) error {
	res := make(PluginDataMap)
	for key, val := range data {
		res[key] = val
	}
	*m = res
	return nil
}

func (m *PluginDataMap) MarshalPluginDataMap() (map[string]string, error) {
	res := make(map[string]string)
	if data := *m; data != nil {
		for key, val := range data {
			res[key] = val
		}
	}
	return res, nil
}
