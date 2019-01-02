package subcommand

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ec2metadata"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	hclog "github.com/hashicorp/go-hclog"
)

type WithEC2MetadataRegion struct {
	Client *ec2metadata.EC2Metadata
}

func (p WithEC2MetadataRegion) GetRegion() (string, error) {
	return p.Client.Region()
}

func AWSConfig() (aws.Config, error) {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	region, ok := os.LookupEnv("AWS_REGION")
	if !ok {

		originalTimeout := cfg.HTTPClient.Timeout
		cfg.HTTPClient.Timeout = 2 * time.Second
		metaClient := ec2metadata.New(cfg)

		if !metaClient.Available() {
			panic("Metadata service cannot be reached.")
		}

		cfg.HTTPClient.Timeout = originalTimeout
		hclog.Default().Info("Autodetected region")
		region, _ = metaClient.Region()
	}

	cfg.Region = region
	return cfg, err
}
