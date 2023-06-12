// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package catalog

import (
	"time"

	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type SyncInput struct {
	ToAWS    bool
	ToConsul bool

	ConsulPrefix         string
	ConsulNamespace      string
	ConsulAdminPartition string

	AWSNamespaceID  string
	AWSPrefix       string
	AWSPullInterval string
	AWSDNSTTL       int64

	Stale bool

	AWSClient    *sd.ServiceDiscovery
	ConsulClient *api.Client
}

// Sync aws->consul and vice versa.
func Sync(input *SyncInput, stop, stopped chan struct{}) {
	defer close(stopped)
	log := hclog.Default().Named("sync")
	consul := consul{
		client:         input.ConsulClient,
		log:            hclog.Default().Named("consul"),
		trigger:        make(chan bool, 1),
		consulPrefix:   input.ConsulPrefix,
		awsPrefix:      input.AWSPrefix,
		toAWS:          input.ToAWS,
		stale:          input.Stale,
		namespace:      input.ConsulNamespace,
		adminPartition: input.ConsulAdminPartition,
	}
	pullInterval, err := time.ParseDuration(input.AWSPullInterval)
	if err != nil {
		log.Error("cannot parse aws pull interval", "error", err)
		return
	}
	aws := aws{
		client:       input.AWSClient,
		log:          hclog.Default().Named("aws"),
		trigger:      make(chan bool, 1),
		consulPrefix: input.ConsulPrefix,
		awsPrefix:    input.AWSPrefix,
		toConsul:     input.ToConsul,
		pullInterval: pullInterval,
		dnsTTL:       input.AWSDNSTTL,
	}

	err = aws.setupNamespace(input.AWSNamespaceID)
	if err != nil {
		log.Error("cannot setup namespace", "error", err)
		return
	}

	fetchConsulStop := make(chan struct{})
	fetchConsulStopped := make(chan struct{})
	go consul.fetchIndefinitely(fetchConsulStop, fetchConsulStopped)
	fetchAWSStop := make(chan struct{})
	fetchAWSStopped := make(chan struct{})
	go aws.fetchIndefinitely(fetchAWSStop, fetchAWSStopped)

	toConsulStop := make(chan struct{})
	toConsulStopped := make(chan struct{})
	toAWSStop := make(chan struct{})
	toAWSStopped := make(chan struct{})

	go aws.sync(&consul, toConsulStop, toConsulStopped)
	go consul.sync(&aws, toAWSStop, toAWSStopped)

	select {
	case <-stop:
		close(toConsulStop)
		close(toAWSStop)
		close(fetchConsulStop)
		close(fetchAWSStop)
		<-toConsulStopped
		<-toAWSStopped
		<-fetchAWSStopped
		<-fetchConsulStopped
	case <-fetchAWSStopped:
		log.Info("problem wit aws fetch. shutting down...")
		close(toConsulStop)
		close(toAWSStop)
		close(fetchConsulStop)
		<-toConsulStopped
		<-toAWSStopped
		<-fetchConsulStopped
	case <-fetchConsulStopped:
		log.Info("problem with consul fetch. shutting down...")
		close(toConsulStop)
		close(fetchAWSStop)
		close(toAWSStop)
		<-toConsulStopped
		<-toAWSStopped
		<-fetchAWSStopped
	case <-toConsulStopped:
		log.Info("problem with consul sync. shutting down...")
		close(fetchConsulStop)
		close(toAWSStop)
		close(fetchAWSStop)
		<-toAWSStopped
		<-fetchAWSStopped
		<-fetchConsulStopped
	case <-toAWSStopped:
		log.Info("problem with aws sync. shutting down...")
		close(toConsulStop)
		close(fetchConsulStop)
		close(fetchAWSStop)
		<-toConsulStopped
		<-fetchConsulStopped
		<-fetchAWSStopped
	}
}
