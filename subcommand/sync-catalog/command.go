package synccatalog

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"

	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/consul-aws/catalog"
	"github.com/hashicorp/consul-aws/subcommand"
	"github.com/hashicorp/consul/command/flags"
	"github.com/mitchellh/cli"
)

const DefaultPollInterval = "30s"

// Command is the command for syncing the A
type Command struct {
	UI cli.Ui

	flags                         *flag.FlagSet
	http                          *flags.HTTPFlags
	flagToConsul                  bool
	flagToAWS                     bool
	flagAWSNamespaceID            string
	flagAWSServicePrefix          string
	flagAWSDeprecatedPullInterval string
	flagAWSPollInterval           string
	flagAWSDNSTTL                 int64
	flagConsulServicePrefix       string
	flagConsulDomain              string

	once sync.Once
	help string
}

func (c *Command) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.BoolVar(&c.flagToConsul, "to-consul", false,
		"If true, AWS services will be synced to Consul. (Defaults to false)")
	c.flags.BoolVar(&c.flagToAWS, "to-aws", false,
		"If true, Consul services will be synced to AWS. (Defaults to false)")
	c.flags.StringVar(&c.flagAWSNamespaceID, "aws-namespace-id",
		"", "The AWS namespace to sync with Consul services.")
	c.flags.StringVar(&c.flagAWSServicePrefix, "aws-service-prefix",
		"", "A prefix to prepend to all services written to AWS from Consul. "+
			"If this is not set then services will have no prefix.")
	c.flags.StringVar(&c.flagConsulServicePrefix, "consul-service-prefix",
		"", "A prefix to prepend to all services written to Consul from AWS. "+
			"If this is not set then services will have no prefix.")
	c.flags.StringVar(&c.flagAWSDeprecatedPullInterval, "aws-pull-interval",
		DefaultPollInterval, "[DEPRECATED] The interval between fetching from AWS CloudMap. "+
			"Accepts a sequence of decimal numbers, each with optional "+
			"fraction and a unit suffix, such as \"300ms\", \"10s\", \"1.5m\". "+
			"Defaults to 30s)")
	c.flags.StringVar(&c.flagAWSPollInterval, "aws-poll-interval",
		DefaultPollInterval, "The interval between fetching from AWS CloudMap. "+
			"Accepts a sequence of decimal numbers, each with optional "+
			"fraction and a unit suffix, such as \"300ms\", \"10s\", \"1.5m\". "+
			"Defaults to 30s)")
	c.flags.Int64Var(&c.flagAWSDNSTTL, "aws-dns-ttl",
		60, "DNS TTL for services created in AWS CloudMap in seconds. (Defaults to 60)")

	c.http = &flags.HTTPFlags{}
	flags.Merge(c.flags, c.http.ClientFlags())
	flags.Merge(c.flags, c.http.ServerFlags())
	c.help = flags.Usage(help, c.flags)
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)
	if err := c.flags.Parse(args); err != nil {
		return 1
	}
	if len(c.flags.Args()) > 0 {
		c.UI.Error("Should have no non-flag arguments.")
		return 1
	}
	if len(c.flagAWSNamespaceID) == 0 {
		c.UI.Error("Please provide -aws-namespace-id.")
		return 1
	}
	config, err := subcommand.AWSConfig()
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error retrieving AWS session: %s", err))
		return 1
	}
	awsClient := sd.New(config)

	consulClient, err := c.http.APIClient()
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error connecting to Consul agent: %s", err))
		return 1
	}

	pollInterval := c.flagAWSPollInterval
	if pollInterval == DefaultPollInterval && c.flagAWSDeprecatedPullInterval != DefaultPollInterval {
		c.UI.Info("Please use -aws-poll-interval instead of the deprecated -aws-pull-interval")
		pollInterval = c.flagAWSDeprecatedPullInterval
	}

	stop := make(chan struct{})
	stopped := make(chan struct{})
	go catalog.Sync(
		c.flagToAWS, c.flagToConsul, c.flagAWSNamespaceID,
		c.flagConsulServicePrefix, c.flagAWSServicePrefix,
		pollInterval, c.flagAWSDNSTTL, c.getStaleWithDefaultTrue(),
		awsClient, consulClient,
		stop, stopped,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	select {
	// Unexpected failure
	case <-stopped:
		return 1
	case <-sigCh:
		c.UI.Info("shutting down...")
		close(stop)
		<-stopped
	}
	return 0
}

func (c *Command) getStaleWithDefaultTrue() bool {
	stale := true
	c.flags.Visit(func(f *flag.Flag) {
		if f.Name == "stale" {
			stale = c.http.Stale()
			return
		}
	})
	return stale
}

func (c *Command) Synopsis() string { return synopsis }
func (c *Command) Help() string {
	c.once.Do(c.init)
	return c.help
}

const synopsis = "Sync AWS services and Consul services."
const help = `
Usage: consul-aws sync-catalog [options]

  Sync AWS services, and more with the Consul service catalog.
  This enables AWS services to discover and communicate with external
  services, and allows external services to discover and communicate with
  AWS services.

`
