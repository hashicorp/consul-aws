// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package catalog

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/stretchr/testify/require"
)

func TestAWSTransformNodes(t *testing.T) {
	a := awsSyncer{}
	nodes := []awssdtypes.InstanceSummary{
		{Id: aws.String("one"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "1"}},
		{Id: aws.String("two"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.2", "AWS_INSTANCE_PORT": "A"}},
		{Id: aws.String("three"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.3"}},
		{Id: aws.String("four"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "2"}},
		{Id: aws.String("five"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.4", "AWS_INSTANCE_PORT": "4", "custom": "aha"}},
	}
	expected := map[string]map[int]node{
		"1.1.1.1": {
			1: {port: 1, host: "1.1.1.1", awsID: "one", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "1"}},
			2: {port: 2, host: "1.1.1.1", awsID: "four", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "2"}},
		},
		"1.1.1.2": {
			0: {port: 0, host: "1.1.1.2", awsID: "two", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.2", "AWS_INSTANCE_PORT": "A"}},
		},
		"1.1.1.3": {
			0: {port: 0, host: "1.1.1.3", awsID: "three", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.3"}},
		},
		"1.1.1.4": {
			4: {port: 4, host: "1.1.1.4", awsID: "five", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.4", "AWS_INSTANCE_PORT": "4", "custom": "aha"}},
		},
	}
	require.Equal(t, expected, a.transformNodes(nodes))
}

func TestAWSTransformServices(t *testing.T) {
	a := awsSyncer{}
	services := []awssdtypes.ServiceSummary{
		{Id: aws.String("one"), Name: aws.String("web"), Description: &awsServiceDescription},
		{Id: aws.String("two"), Name: aws.String("redis")},
	}
	expected := map[string]service{
		"web":   {id: "one", name: "web", awsID: "one", fromConsul: true},
		"redis": {id: "two", name: "redis", awsID: "two", fromConsul: false},
	}
	require.Equal(t, expected, a.transformServices(services))
}
