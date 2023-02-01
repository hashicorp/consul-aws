// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package subcommand

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
)

func AWSConfig() (aws.Config, error) {
	return external.LoadDefaultAWSConfig()
}
