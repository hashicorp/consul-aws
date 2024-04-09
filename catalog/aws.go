// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package catalog

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	awssdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/hashicorp/go-hclog"
)

const (
	// ConsulAWSTag is used for imported services from AWS
	ConsulAWSTag    = "aws"
	ConsulSourceKey = "external-source"
	ConsulAWSNS     = "external-aws-ns"
	ConsulAWSID     = "external-aws-id"
)

type namespace struct {
	id     string
	name   string
	isHTTP bool
}

type awsSyncer struct {
	lock         sync.RWMutex
	client       *awssd.Client
	log          hclog.Logger
	namespace    namespace
	services     map[string]service
	trigger      chan bool
	consulPrefix string
	awsPrefix    string
	toConsul     bool
	pullInterval time.Duration
	dnsTTL       int64
}

var awsServiceDescription = "Imported from Consul"

func (a *awsSyncer) sync(consul *consul, stop, stopped chan struct{}) {
	defer close(stopped)
	for {
		select {
		case <-a.trigger:
			if !a.toConsul {
				continue
			}
			create := onlyInFirst(a.getServices(), consul.getServices())
			count := consul.create(create)
			if count > 0 {
				consul.log.Info("created", "count", fmt.Sprintf("%d", count))
			}

			remove := onlyInFirst(consul.getServices(), a.getServices())
			count = consul.remove(remove)
			if count > 0 {
				consul.log.Info("removed", "count", fmt.Sprintf("%d", count))
			}
		case <-stop:
			return
		}
	}
}

func (a *awsSyncer) fetchNamespace(id string) (*awssdtypes.Namespace, error) {
	resp, err := a.client.GetNamespace(context.Background(), &awssd.GetNamespaceInput{Id: aws.String(id)})
	if err != nil {
		return nil, err
	}
	return resp.Namespace, nil
}

func (a *awsSyncer) fetchServices() ([]awssdtypes.ServiceSummary, error) {
	paginator := awssd.NewListServicesPaginator(a.client, &awssd.ListServicesInput{
		Filters: []awssdtypes.ServiceFilter{{
			Name:      awssdtypes.ServiceFilterNameNamespaceId,
			Condition: awssdtypes.FilterConditionEq,
			Values:    []string{a.namespace.id},
		}},
	})

	services := []awssdtypes.ServiceSummary{}
	for paginator.HasMorePages() {
		p, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("error paging through services: %s", err)
		}
		services = append(services, p.Services...)
	}
	return services, nil
}

func (a *awsSyncer) transformServices(awsServices []awssdtypes.ServiceSummary) map[string]service {
	services := map[string]service{}
	for _, as := range awsServices {
		s := service{
			id:           *as.Id,
			name:         *as.Name,
			awsID:        *as.Id,
			awsNamespace: a.namespace.id,
		}
		if as.Description != nil && *as.Description == awsServiceDescription {
			s.fromConsul = true
			s.name = strings.TrimPrefix(s.name, a.consulPrefix)
		}

		services[s.name] = s
	}
	return services
}

func (a *awsSyncer) transformNamespace(awsNamespace *awssdtypes.Namespace) namespace {
	namespace := namespace{id: *awsNamespace.Id, name: *awsNamespace.Name}
	if awsNamespace.Type == awssdtypes.NamespaceTypeHttp {
		namespace.isHTTP = true
	}
	return namespace
}

func (a *awsSyncer) setupNamespace(id string) error {
	namespace, err := a.fetchNamespace(id)
	if err != nil {
		return err
	}
	a.namespace = a.transformNamespace(namespace)
	return nil
}

func (a *awsSyncer) fetch() error {
	awsService, err := a.fetchServices()
	if err != nil {
		return err
	}
	services := a.transformServices(awsService)
	for h, s := range services {
		var awsNodes []awssdtypes.InstanceSummary
		var err error
		name := s.name
		if s.fromConsul {
			name = a.consulPrefix + name
		}
		awsNodes, err = a.discoverNodes(name)
		if err != nil {
			a.log.Error("cannot discover nodes", "error", err)
			continue
		}

		nodes := a.transformNodes(awsNodes)
		if len(nodes) == 0 {
			continue
		}
		s.nodes = nodes

		healths, err := a.fetchHealths(s.awsID)
		if err != nil {
			a.log.Error("cannot fetch healths", "error", err)
		} else {
			if s.fromConsul {
				healths = a.rekeyHealths(s.name, healths)
			}
			s.healths = healths
		}

		services[h] = s
	}
	a.setServices(services)
	return nil
}

func (a *awsSyncer) getNodeForConsulID(name, id string) (node, bool) {
	a.lock.RLock()
	copy, ok := a.services[name]
	a.lock.RUnlock()
	if !ok {
		return node{}, ok
	}
	for _, nodes := range copy.nodes {
		for _, n := range nodes {
			if n.consulID == id {
				return n, true
			}
		}
	}
	return node{}, false
}

func (a *awsSyncer) rekeyHealths(name string, healths map[string]health) map[string]health {
	rekeyed := map[string]health{}
	s, ok := a.getService(name)
	if !ok {
		return nil
	}
	for k, h := range s.healths {
		if n, ok := a.getNodeForConsulID(name, k); ok {
			rekeyed[id(k, n.host, n.port)] = h
		}
	}
	return rekeyed
}

func statusFromAWS(aws awssdtypes.HealthStatus) health {
	var result health
	switch aws {
	case awssdtypes.HealthStatusHealthy:
		result = passing
	case awssdtypes.HealthStatusUnhealthy:
		result = critical
	case awssdtypes.HealthStatusUnknown:
		result = unknown
	}
	return result
}

func (a *awsSyncer) fetchHealths(id string) (map[string]health, error) {
	paginator := awssd.NewGetInstancesHealthStatusPaginator(a.client, &awssd.GetInstancesHealthStatusInput{
		ServiceId: &id,
	})

	result := map[string]health{}
	for paginator.HasMorePages() {
		p, err := paginator.NextPage(context.TODO())

		if err != nil {
			var notFound *awssdtypes.InstanceNotFound
			if !errors.As(err, &notFound) {
				return nil, fmt.Errorf("error paging through healths: %s", err)
			}
		}

		for id, health := range p.Status {
			result[id] = statusFromAWS(health)
		}
	}

	return result, nil
}

func (a *awsSyncer) transformNodes(awsNodes []awssdtypes.InstanceSummary) map[string]map[int]node {
	nodes := map[string]map[int]node{}
	for _, an := range awsNodes {
		h := an.Attributes["AWS_INSTANCE_IPV4"]
		p := 0
		if an.Attributes["AWS_INSTANCE_PORT"] != "" {
			p, _ = strconv.Atoi(an.Attributes["AWS_INSTANCE_PORT"])
		}
		if nodes[h] == nil {
			nodes[h] = map[int]node{}
		}
		n := nodes[h]
		n[p] = node{port: p, host: h, awsID: *an.Id, attributes: an.Attributes}
		nodes[h] = n
	}
	return nodes
}

func (a *awsSyncer) fetchNodes(id string) ([]awssdtypes.InstanceSummary, error) {
	paginator := awssd.NewListInstancesPaginator(a.client, &awssd.ListInstancesInput{
		ServiceId: &id,
	})

	nodes := []awssdtypes.InstanceSummary{}
	for paginator.HasMorePages() {
		p, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("error paging through instances: %s", err)
		}
		nodes = append(nodes, p.Instances...)
	}
	return nodes, nil
}

func (a *awsSyncer) discoverNodes(name string) ([]awssdtypes.InstanceSummary, error) {
	resp, err := a.client.DiscoverInstances(context.TODO(), &awssd.DiscoverInstancesInput{
		HealthStatus:  awssdtypes.HealthStatusFilterHealthy,
		NamespaceName: aws.String(a.namespace.name),
		ServiceName:   aws.String(name),
	})
	if err != nil {
		return nil, err
	}
	nodes := []awssdtypes.InstanceSummary{}
	for _, i := range resp.Instances {
		nodes = append(nodes, awssdtypes.InstanceSummary{Id: i.InstanceId, Attributes: i.Attributes})
	}
	return nodes, nil
}

func (a *awsSyncer) getServices() map[string]service {
	a.lock.RLock()
	copy := a.services
	a.lock.RUnlock()
	return copy
}

func (a *awsSyncer) getService(name string) (service, bool) {
	a.lock.RLock()
	copy, ok := a.services[name]
	a.lock.RUnlock()
	return copy, ok
}

func (a *awsSyncer) setServices(services map[string]service) {
	a.lock.Lock()
	a.services = services
	a.lock.Unlock()
}

func (a *awsSyncer) create(services map[string]service) int {
	wg := sync.WaitGroup{}
	count := 0
	for k, s := range services {
		if s.fromAWS {
			continue
		}
		name := a.consulPrefix + k
		if len(s.awsID) == 0 {
			input := awssd.CreateServiceInput{
				Description: &awsServiceDescription,
				Name:        &name,
				NamespaceId: &a.namespace.id,
			}
			if !a.namespace.isHTTP {
				input.DnsConfig = &awssdtypes.DnsConfig{
					DnsRecords: []awssdtypes.DnsRecord{
						{TTL: &a.dnsTTL, Type: awssdtypes.RecordTypeSrv},
					},
				}
			}
			resp, err := a.client.CreateService(context.TODO(), &input)
			if err != nil {
				var alreadyExists *awssdtypes.ServiceAlreadyExists
				if !errors.As(err, &alreadyExists) {
					a.log.Info("service already exists", "name", name)
				} else {
					a.log.Error("cannot create services in AWS", "error", err.Error())
				}
				continue
			}
			s.awsID = *resp.Service.Id
			count++
		}
		for h, nodes := range s.nodes {
			for _, n := range nodes {
				wg.Add(1)
				go func(serviceID, name, h string, n node) {
					wg.Done()
					instanceID := id(serviceID, h, n.port)
					attributes := n.attributes
					attributes["AWS_INSTANCE_IPV4"] = h
					attributes["AWS_INSTANCE_PORT"] = fmt.Sprintf("%d", n.port)
					_, err := a.client.RegisterInstance(context.TODO(), &awssd.RegisterInstanceInput{
						ServiceId:  &serviceID,
						Attributes: attributes,
						InstanceId: &instanceID,
					})
					if err != nil {
						a.log.Error("cannot create nodes", "error", err.Error())
					}
				}(s.awsID, name, h, n)
			}
		}
		// for instanceID, h := range s.healths {
		// 	wg.Add(1)
		// 	go func(serviceID, instanceID string, h health) {
		// 		defer wg.Done()
		// 		req := a.client.UpdateInstanceCustomHealthStatusRequest(&awssd.UpdateInstanceCustomHealthStatusInput{
		// 			ServiceId:  &serviceID,
		// 			InstanceId: &instanceID,
		// 			Status:     statusToCustomHealth(h),
		// 		})
		// 		_, err := req.Send()
		// 		if err != nil {
		// 			a.log.Error("cannot create custom health", "error", err.Error())
		// 		}
		// 	}(s.awsID, instanceID, h)
		// }
	}
	wg.Wait()
	return count
}

func (a *awsSyncer) remove(services map[string]service) int {
	wg := sync.WaitGroup{}
	for _, s := range services {
		if !s.fromConsul || len(s.awsID) == 0 {
			continue
		}
		for h, nodes := range s.nodes {
			for _, n := range nodes {
				wg.Add(1)
				go func(serviceID, id string) {
					defer wg.Done()
					_, err := a.client.DeregisterInstance(context.TODO(), &awssd.DeregisterInstanceInput{
						ServiceId:  &serviceID,
						InstanceId: &id,
					})
					if err != nil {
						a.log.Error("cannot remove instance", "error", err.Error())
					}
				}(s.awsID, id(s.awsID, h, n.port))
			}
		}
	}
	wg.Wait()

	count := 0
	for k, s := range services {
		if !s.fromConsul || len(s.awsID) == 0 {
			continue
		}
		origService, _ := a.getService(k)
		if len(s.nodes) < len(origService.nodes) {
			continue
		}
		_, err := a.client.DeleteService(context.TODO(), &awssd.DeleteServiceInput{
			Id: &s.awsID,
		})
		if err != nil {
			a.log.Error("cannot remove services", "name", k, "id", s.awsID, "error", err.Error())
		} else {
			count++
		}
	}
	return count
}

func (a *awsSyncer) fetchIndefinetely(stop, stopped chan struct{}) {
	defer close(stopped)
	for {
		err := a.fetch()
		if err != nil {
			a.log.Error("error fetching", "error", err.Error())
		} else {
			a.trigger <- true
		}
		select {
		case <-stop:
			return
		case <-time.After(a.pullInterval):
			continue
		}
	}
}
