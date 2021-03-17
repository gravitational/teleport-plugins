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

package dynamo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

// SetContinuousBackups enables continuous backups.
func SetContinuousBackups(ctx context.Context, svc *dynamodb.DynamoDB, tableName string) error {
	// Make request to AWS to update continuous backups settings.
	_, err := svc.UpdateContinuousBackupsWithContext(ctx, &dynamodb.UpdateContinuousBackupsInput{
		PointInTimeRecoverySpecification: &dynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: aws.Bool(true),
		},
		TableName: aws.String(tableName),
	})
	if err != nil {
		return convertError(err)
	}

	return nil
}

// AutoScalingParams defines auto scaling parameters for DynamoDB.
type AutoScalingParams struct {
	// ReadMaxCapacity is the maximum provisioned read capacity.
	ReadMaxCapacity int64
	// ReadMinCapacity is the minimum provisioned read capacity.
	ReadMinCapacity int64
	// ReadTargetValue is the ratio of consumed read to provisioned capacity.
	ReadTargetValue float64
	// WriteMaxCapacity is the maximum provisioned write capacity.
	WriteMaxCapacity int64
	// WriteMinCapacity is the minimum provisioned write capacity.
	WriteMinCapacity int64
	// WriteTargetValue is the ratio of consumed write to provisioned capacity.
	WriteTargetValue float64
}

// SetAutoScaling enables auto-scaling for the specified table with given configuration.
func SetAutoScaling(ctx context.Context, svc *applicationautoscaling.ApplicationAutoScaling, tableName string, params AutoScalingParams) error {
	// Define scaling targets. Defines minimum and maximum {read,write} capacity.
	if _, err := svc.RegisterScalableTarget(&applicationautoscaling.RegisterScalableTargetInput{
		MinCapacity:       aws.Int64(params.ReadMinCapacity),
		MaxCapacity:       aws.Int64(params.ReadMaxCapacity),
		ResourceId:        aws.String(fmt.Sprintf("%v/%v", resourcePrefix, tableName)),
		ScalableDimension: aws.String(applicationautoscaling.ScalableDimensionDynamodbTableReadCapacityUnits),
		ServiceNamespace:  aws.String(applicationautoscaling.ServiceNamespaceDynamodb),
	}); err != nil {
		return convertError(err)
	}
	if _, err := svc.RegisterScalableTarget(&applicationautoscaling.RegisterScalableTargetInput{
		MinCapacity:       aws.Int64(params.WriteMinCapacity),
		MaxCapacity:       aws.Int64(params.WriteMaxCapacity),
		ResourceId:        aws.String(fmt.Sprintf("%v/%v", resourcePrefix, tableName)),
		ScalableDimension: aws.String(applicationautoscaling.ScalableDimensionDynamodbTableWriteCapacityUnits),
		ServiceNamespace:  aws.String(applicationautoscaling.ServiceNamespaceDynamodb),
	}); err != nil {
		return convertError(err)
	}

	// Define scaling policy. Defines the ratio of {read,write} consumed capacity to
	// provisioned capacity DynamoDB will try and maintain.
	if _, err := svc.PutScalingPolicy(&applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String(fmt.Sprintf("%v-%v", tableName, readScalingPolicySuffix)),
		PolicyType:        aws.String(applicationautoscaling.PolicyTypeTargetTrackingScaling),
		ResourceId:        aws.String(fmt.Sprintf("%v/%v", resourcePrefix, tableName)),
		ScalableDimension: aws.String(applicationautoscaling.ScalableDimensionDynamodbTableReadCapacityUnits),
		ServiceNamespace:  aws.String(applicationautoscaling.ServiceNamespaceDynamodb),
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &applicationautoscaling.PredefinedMetricSpecification{
				PredefinedMetricType: aws.String(applicationautoscaling.MetricTypeDynamoDbreadCapacityUtilization),
			},
			TargetValue: aws.Float64(params.ReadTargetValue),
		},
	}); err != nil {
		return convertError(err)
	}
	if _, err := svc.PutScalingPolicy(&applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String(fmt.Sprintf("%v-%v", tableName, writeScalingPolicySuffix)),
		PolicyType:        aws.String(applicationautoscaling.PolicyTypeTargetTrackingScaling),
		ResourceId:        aws.String(fmt.Sprintf("%v/%v", resourcePrefix, tableName)),
		ScalableDimension: aws.String(applicationautoscaling.ScalableDimensionDynamodbTableWriteCapacityUnits),
		ServiceNamespace:  aws.String(applicationautoscaling.ServiceNamespaceDynamodb),
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &applicationautoscaling.PredefinedMetricSpecification{
				PredefinedMetricType: aws.String(applicationautoscaling.MetricTypeDynamoDbwriteCapacityUtilization),
			},
			TargetValue: aws.Float64(params.WriteTargetValue),
		},
	}); err != nil {
		return convertError(err)
	}

	return nil
}

const (
	readScalingPolicySuffix  = "read-target-tracking-scaling-policy"
	writeScalingPolicySuffix = "write-target-tracking-scaling-policy"
	resourcePrefix           = "table"
)
