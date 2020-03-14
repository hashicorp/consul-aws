package catalog

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	x "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
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

type aws struct {
	lock         sync.RWMutex
	client       *sd.ServiceDiscovery
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

func (a *aws) sync(consul *consul, stop, stopped chan struct{}) {
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

func (a *aws) fetchNamespace(id string) (*sd.Namespace, error) {
	req := a.client.GetNamespaceRequest(&sd.GetNamespaceInput{Id: x.String(id)})
	resp, err := req.Send()
	if err != nil {
		return nil, err
	}
	return resp.Namespace, nil
}

func (a *aws) fetchServices() ([]sd.ServiceSummary, error) {
	req := a.client.ListServicesRequest(&sd.ListServicesInput{
		Filters: []sd.ServiceFilter{{
			Name:      sd.ServiceFilterNameNamespaceId,
			Condition: sd.FilterConditionEq,
			Values:    []string{a.namespace.id},
		}},
	})
	p := req.Paginate()
	services := []sd.ServiceSummary{}
	for p.Next() {
		services = append(services, p.CurrentPage().Services...)
	}
	return services, p.Err()
}

func (a *aws) transformServices(awsServices []sd.ServiceSummary) map[string]service {
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

func (a *aws) transformNamespace(awsNamespace *sd.Namespace) namespace {
	namespace := namespace{id: *awsNamespace.Id, name: *awsNamespace.Name}
	if awsNamespace.Type == sd.NamespaceTypeHttp {
		namespace.isHTTP = true
	}
	return namespace
}

func (a *aws) setupNamespace(id string) error {
	namespace, err := a.fetchNamespace(id)
	if err != nil {
		return err
	}
	a.namespace = a.transformNamespace(namespace)
	return nil
}

func (a *aws) fetch() error {
	awsService, err := a.fetchServices()
	if err != nil {
		return err
	}
	services := a.transformServices(awsService)
	for h, s := range services {
		var awsNodes []sd.InstanceSummary
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

func (a *aws) getNodeForConsulID(name, id string) (node, bool) {
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

func (a *aws) rekeyHealths(name string, healths map[string]health) map[string]health {
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

func statusFromAWS(aws sd.HealthStatus) health {
	var result health
	switch aws {
	case sd.HealthStatusHealthy:
		result = passing
	case sd.HealthStatusUnhealthy:
		result = critical
	case sd.HealthStatusUnknown:
		result = unknown
	}
	return result
}

func statusToCustomHealth(h health) sd.CustomHealthStatus {
	var result sd.CustomHealthStatus
	switch h {
	case passing:
		result = sd.CustomHealthStatusHealthy
	case critical:
		result = sd.CustomHealthStatusUnhealthy
	}
	return result
}

func (a *aws) fetchHealths(id string) (map[string]health, error) {
	req := a.client.GetInstancesHealthStatusRequest(&sd.GetInstancesHealthStatusInput{
		ServiceId: &id,
	})
	result := map[string]health{}
	p := req.Paginate()
	for p.Next() {
		for id, health := range p.CurrentPage().Status {
			result[id] = statusFromAWS(health)
		}
	}
	err := p.Err()
	if err != nil {
		if err, ok := err.(awserr.Error); ok {
			switch err.Code() {
			case sd.ErrCodeInstanceNotFound:
			default:
				return result, err
			}
		} else {
			return result, err
		}
	}
	return result, nil
}

func (a *aws) transformNodes(awsNodes []sd.InstanceSummary) map[string]map[int]node {
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

func (a *aws) fetchNodes(id string) ([]sd.InstanceSummary, error) {
	req := a.client.ListInstancesRequest(&sd.ListInstancesInput{
		ServiceId: &id,
	})
	p := req.Paginate()
	nodes := []sd.InstanceSummary{}
	for p.Next() {
		nodes = append(nodes, p.CurrentPage().Instances...)
	}
	return nodes, p.Err()
}

func (a *aws) discoverNodes(name string) ([]sd.InstanceSummary, error) {
	req := a.client.DiscoverInstancesRequest(&sd.DiscoverInstancesInput{
		HealthStatus:  sd.HealthStatusFilterHealthy,
		NamespaceName: x.String(a.namespace.name),
		ServiceName:   x.String(name),
	})
	resp, err := req.Send()
	if err != nil {
		return nil, err
	}
	nodes := []sd.InstanceSummary{}
	for _, i := range resp.Instances {
		nodes = append(nodes, sd.InstanceSummary{Id: i.InstanceId, Attributes: i.Attributes})
	}
	return nodes, nil
}

func (a *aws) getServices() map[string]service {
	a.lock.RLock()
	copy := a.services
	a.lock.RUnlock()
	return copy
}

func (a *aws) getService(name string) (service, bool) {
	a.lock.RLock()
	copy, ok := a.services[name]
	a.lock.RUnlock()
	return copy, ok
}

func (a *aws) setServices(services map[string]service) {
	a.lock.Lock()
	a.services = services
	a.lock.Unlock()
}

func (a *aws) create(services map[string]service) int {
	wg := sync.WaitGroup{}
	count := 0
	for k, s := range services {
		if s.fromAWS {
			continue
		}
		name := a.consulPrefix + k
		if len(s.awsID) == 0 {
			input := sd.CreateServiceInput{
				Description: &awsServiceDescription,
				Name:        &name,
				NamespaceId: &a.namespace.id,
			}
			if !a.namespace.isHTTP {
				input.DnsConfig = &sd.DnsConfig{
					DnsRecords: []sd.DnsRecord{
						{TTL: &a.dnsTTL, Type: sd.RecordTypeSrv},
					},
				}
			}
			req := a.client.CreateServiceRequest(&input)
			resp, err := req.Send()
			if err != nil {
				if err, ok := err.(awserr.Error); ok {
					switch err.Code() {
					case sd.ErrCodeServiceAlreadyExists:
						a.log.Info("service already exists", "name", name)
					}
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
					req := a.client.RegisterInstanceRequest(&sd.RegisterInstanceInput{
						ServiceId:  &serviceID,
						Attributes: attributes,
						InstanceId: &instanceID,
					})
					_, err := req.Send()
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
		// 		req := a.client.UpdateInstanceCustomHealthStatusRequest(&sd.UpdateInstanceCustomHealthStatusInput{
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

func (a *aws) remove(services map[string]service) int {
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
					req := a.client.DeregisterInstanceRequest(&sd.DeregisterInstanceInput{
						ServiceId:  &serviceID,
						InstanceId: &id,
					})
					_, err := req.Send()
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
		req := a.client.DeleteServiceRequest(&sd.DeleteServiceInput{
			Id: &s.awsID,
		})
		_, err := req.Send()
		if err != nil {
			a.log.Error("cannot remove services", "name", k, "id", s.awsID, "error", err.Error())
		} else {
			count++
		}
	}
	return count
}

func (a *aws) fetchIndefinetely(stop, stopped chan struct{}) {
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
