package subcommand

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
)

func AWSConfig() (aws.Config, error) {
	return external.LoadDefaultAWSConfig()
}
