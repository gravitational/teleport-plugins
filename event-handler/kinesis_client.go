/*
Copyright 2015-2022 Gravitational, Inc.

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
	"context"

	"github.com/gravitational/teleport-plugins/event-handler/lib"
	tlib "github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/trace"

	"github.com/aws/aws-sdk-go/aws"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kinesis"
	log "github.com/sirupsen/logrus"
)

// KinesisClient represents Kinesis client
type KinesisClient struct {
	// client kinesis client to sent requests
	client *kinesis.Kinesis
}

// NewKinesisClient creates new KinesisClient
func NewKinesisClient(c *KinesisConfig) (*KinesisClient, error) {
	opts := awsSession.Options{
		Config: aws.Config{
			Region:      aws.String(c.KinesisAwsRegion),
			CredentialsChainVerboseErrors: aws.Bool(true),
		},
	}
	// if the aws profile was set, set it in the aws session options
	if c.KinesisAwsProfile!= "" {
		opts.Profile = c.KinesisAwsProfile
		opts.SharedConfigState = awsSession.SharedConfigEnable
	}
	session, err := awsSession.NewSessionWithOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	client := kinesis.New(session)

	// check that kinesis stream name exists
	_, err = client.DescribeStreamSummary(&kinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(c.KinesisStreamName),
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &KinesisClient{client: client}, nil
}

// Send sends event to kinesis
func (k *KinesisClient) Send(ctx context.Context, streamName string, obj interface{}) error {
	b, err := lib.FastMarshal(obj)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithField("json", string(b)).Debug("JSON to send")

	output, err := k.client.PutRecord(&kinesis.PutRecordInput{
		Data:         []byte(b),
		StreamName:   aws.String(streamName),
		PartitionKey: aws.String("key1"), // TODO: pick partition key
	})
	if err != nil {
		// err returned by client.PutRecord() would never have status canceled
		if tlib.IsCanceled(ctx.Err()) {
			return trace.Wrap(ctx.Err())
		}

		return trace.Wrap(err)
	}

	log.WithField("shard", *output.ShardId).WithField("sequence number", *output.SequenceNumber).Debug("Event pushed")
	return nil
}
