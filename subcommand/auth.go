// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package subcommand

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

func AWSConfig() (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(context.TODO())
}
