package catalog

import (
	"time"

	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

// Sync aws->consul and vice versa.
func Sync(toAWS, toConsul bool, namespaceID, consulPrefix, awsPrefix, awsPullInterval string, awsDNSTTL int64, stale bool, awsClient *sd.ServiceDiscovery, consulClient *api.Client, stop, stopped chan struct{}) {
	defer close(stopped)
	log := hclog.Default().Named("sync")
	consul := consul{
		client:       consulClient,
		log:          hclog.Default().Named("consul"),
		trigger:      make(chan bool, 1),
		consulPrefix: consulPrefix,
		awsPrefix:    awsPrefix,
		toAWS:        toAWS,
		stale:        stale,
	}
	pullInterval, err := time.ParseDuration(awsPullInterval)
	if err != nil {
		log.Error("cannot parse aws pull interval", "error", err)
		return
	}
	aws := aws{
		client:       awsClient,
		log:          hclog.Default().Named("aws"),
		trigger:      make(chan bool, 1),
		consulPrefix: consulPrefix,
		awsPrefix:    awsPrefix,
		toConsul:     toConsul,
		pullInterval: pullInterval,
		dnsTTL:       awsDNSTTL,
	}

	err = aws.setupNamespace(namespaceID)
	if err != nil {
		log.Error("cannot setup namespace", "error", err)
		return
	}

	fetchConsulStop := make(chan struct{})
	fetchConsulStopped := make(chan struct{})
	go consul.fetchIndefinetely(fetchConsulStop, fetchConsulStopped)
	fetchAWSStop := make(chan struct{})
	fetchAWSStopped := make(chan struct{})
	go aws.fetchIndefinetely(fetchAWSStop, fetchAWSStopped)

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
