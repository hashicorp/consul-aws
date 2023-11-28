// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package catalog

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

const (
	ConsulAWSNodeName = "consul-aws"
	WaitTime          = 10
)

type consul struct {
	client       *api.Client
	log          hclog.Logger
	consulPrefix string
	awsPrefix    string
	services     map[string]service
	trigger      chan bool
	lock         sync.RWMutex
	toAWS        bool
	stale        bool
}

func (c *consul) getServices() map[string]service {
	c.lock.RLock()
	copy := c.services
	c.lock.RUnlock()
	return copy
}

func (c *consul) getService(name string) (service, bool) {
	c.lock.RLock()
	copy, ok := c.services[name]
	c.lock.RUnlock()
	return copy, ok
}

func (c *consul) getAWSID(name, host string, port int) (string, bool) {
	if n, ok := c.getNode(name, host, port); ok {
		return n.awsID, true
	}
	return "", false
}

func (c *consul) getNode(name, host string, port int) (node, bool) {
	c.lock.RLock()
	copy, ok := c.services[name]
	c.lock.RUnlock()
	if ok {
		if nodes, ok := copy.nodes[host]; ok {
			for _, n := range nodes {
				if n.port == port {
					return n, true
				}
			}
		}
	}
	return node{}, false
}

func (c *consul) getNodeForAWSID(name, id string) (node, bool) {
	c.lock.RLock()
	copy, ok := c.services[name]
	c.lock.RUnlock()
	if !ok {
		return node{}, ok
	}

	for _, nodes := range copy.nodes {
		for _, n := range nodes {
			if n.awsID == id {
				return n, true
			}
		}
	}
	return node{}, false
}

func (c *consul) setServices(services map[string]service) {
	c.lock.Lock()
	c.services = services
	c.lock.Unlock()
}

func (c *consul) setNode(k, h string, p int, n node) {
	c.lock.Lock()
	if s, ok := c.services[k]; ok {
		nodes := s.nodes
		if nodes == nil {
			nodes = map[string]map[int]node{}
		}
		ports := nodes[h]
		if ports == nil {
			ports = map[int]node{}
		}
		ports[p] = n
		nodes[h] = ports
		s.nodes = nodes
		c.services[k] = s
	}
	c.lock.Unlock()
}

func (c *consul) sync(aws *aws, stop, stopped chan struct{}) {
	defer close(stopped)
	for {
		select {
		case <-c.trigger:
			if !c.toAWS {
				continue
			}
			create := onlyInFirst(c.getServices(), aws.getServices())
			count := aws.create(create)
			if count > 0 {
				aws.log.Info("created", "count", fmt.Sprintf("%d", count))
			}

			remove := onlyInFirst(aws.getServices(), c.getServices())
			count = aws.remove(remove)
			if count > 0 {
				aws.log.Info("removed", "count", fmt.Sprintf("%d", count))
			}
		case <-stop:
			return
		}
	}
}

func (c *consul) transformNodes(cnodes []*api.CatalogService) map[string]map[int]node {
	nodes := map[string]map[int]node{}
	for _, n := range cnodes {
		// use Address instead of ServiceAddress; RabbitMQ updates the service
		// address to be its internal DNS instead which breaks stuff
		address := n.Address
		if nodes[address] == nil {
			nodes[address] = map[int]node{}
		}
		ports := nodes[address]
		ports[n.ServicePort] = node{name: n.Node, port: n.ServicePort, host: address, consulID: n.ServiceID, awsID: n.ServiceMeta[ConsulAWSID], attributes: n.ServiceMeta}
		nodes[address] = ports
	}
	return nodes
}

func (c *consul) fetchNodes(service string, tag string) ([]*api.CatalogService, error) {
	opts := &api.QueryOptions{AllowStale: c.stale}
	nodes, _, err := c.client.Catalog().Service(service, tag, opts)
	if err != nil {
		return nil, fmt.Errorf("error querying services, will retry: %s", err)
	}
	return nodes, err
}

func (c *consul) transformHealth(chealths api.HealthChecks) map[string]health {
	healths := map[string]health{}
	for _, h := range chealths {
		switch h.Status {
		case "passing":
			healths[h.ServiceID] = passing
		case "critical":
			healths[h.ServiceID] = critical
		default:
			healths[h.ServiceID] = unknown
		}
	}
	return healths
}

func (c *consul) fetchHealth(name string) (api.HealthChecks, error) {
	opts := &api.QueryOptions{AllowStale: c.stale}
	status, _, err := c.client.Health().Checks(name, opts)
	if err != nil {
		return nil, fmt.Errorf("error querying health, will retry: %s", err)
	}
	return status, nil
}

func (c *consul) fetchServices(waitIndex uint64) (map[string][]string, uint64, error) {
	opts := &api.QueryOptions{
		AllowStale: c.stale,
		WaitIndex:  waitIndex,
		WaitTime:   WaitTime * time.Second,
	}
	services, meta, err := c.client.Catalog().Services(opts)
	if err != nil {
		return services, 0, err
	}
	return services, meta.LastIndex, nil
}

func (c *consul) fetch(waitIndex uint64) (uint64, error) {
	cservices, waitIndex, err := c.fetchServices(waitIndex)
	if err != nil {
		return waitIndex, fmt.Errorf("error fetching services: %s", err)
	}
	services := map[string]service{}
	for id, s := range c.transformServices(cservices) {
		if len(s.tags) == 0 {
			services[id] = c.mapServiceWithTag(id, s, "")
		} else {
			for _, tag := range s.tags {
				services[tag+"."+id] = c.mapServiceWithTag(id, s, tag)
			}
		}
	}
	c.setServices(services)
	return waitIndex, nil
}

func (c *consul) mapServiceWithTag(id string, s service, tag string) service {
	if s.fromAWS {
		id = c.awsPrefix + id
	}

	if cnodes, err := c.fetchNodes(id, tag); err == nil {
		s.nodes = c.transformNodes(cnodes)
	} else {
		c.log.Error("error fetching nodes", "error", err)
		return s
	}

	if chealths, err := c.fetchHealth(id); err == nil {
		s.healths = c.transformHealth(chealths)
	} else {
		// TODO (hans): decide what to do when health errors
		c.log.Error("error fetching health", "error", err)
	}

	if s.fromAWS {
		s.healths = c.rekeyHealths(s.name, s.healths)
	}

	return s
}

func (c *consul) transformServices(cservices map[string][]string) map[string]service {
	services := make(map[string]service, len(cservices))
	for k, tags := range cservices {
		s := service{id: k, name: k, consulID: k, tags: tags}
		for _, t := range tags {
			if t == ConsulAWSTag {
				s.fromAWS = true
				break
			}
		}
		if s.fromAWS {
			s.name = strings.TrimPrefix(k, c.awsPrefix)
		}
		services[s.name] = s
	}
	return services
}

func (c *consul) rekeyHealths(name string, healths map[string]health) map[string]health {
	rekeyed := map[string]health{}
	for id, h := range healths {
		host, port := hostPortFromID(id)
		if awsID, ok := c.getAWSID(name, host, port); ok {
			rekeyed[awsID] = h
		}
	}
	return rekeyed
}

func (c *consul) fetchIndefinetely(stop, stopped chan struct{}) {
	defer close(stopped)
	waitIndex := uint64(1)
	subsequentErrors := 0
	for {
		newIndex, err := c.fetch(waitIndex)
		if err != nil {
			c.log.Error("error fetching", "error", err.Error())
			subsequentErrors++
			if subsequentErrors > 10 {
				return
			}
			time.Sleep(500 * time.Millisecond)
		} else {
			subsequentErrors = 0
			waitIndex = newIndex
			c.trigger <- true
		}
		select {
		case <-stop:
			return
		default:
		}
	}
}

func (c *consul) create(services map[string]service) int {
	wg := sync.WaitGroup{}
	count := 0
	for k, s := range services {
		if s.fromConsul {
			continue
		}
		name := c.awsPrefix + k
		for h, nodes := range s.nodes {
			for _, n := range nodes {
				wg.Add(1)
				go func(ns, k, name, h string, n node) {
					defer wg.Done()
					id := id(k, h, n.port)
					meta := map[string]string{}
					for k, v := range n.attributes {
						meta[k] = v
					}
					meta[ConsulSourceKey] = ConsulAWSTag
					meta[ConsulAWSNS] = ns
					meta[ConsulAWSID] = n.awsID
					service := api.AgentService{
						ID:      id,
						Service: name,
						Tags:    []string{ConsulAWSTag},
						Address: h,
						Meta:    meta,
					}
					if n.port != 0 {
						service.Port = n.port
					}
					reg := api.CatalogRegistration{
						Node:           ConsulAWSNodeName,
						Address:        h,
						NodeMeta:       map[string]string{ConsulSourceKey: ConsulAWSTag},
						SkipNodeUpdate: true,
						Service:        &service,
					}
					_, err := c.client.Catalog().Register(&reg, nil)
					if err != nil {
						c.log.Error("cannot create service", "error", err.Error())
					} else {
						c.setNode(k, h, n.port, n)
						count++
					}
				}(s.awsNamespace, k, name, h, n)
			}
		}
		for awsID, h := range s.healths {
			n, ok := c.getNodeForAWSID(k, awsID)
			if !ok {
				continue
			}
			wg.Add(1)
			go func(serviceID string, n node, h health) {
				defer wg.Done()
				reg := api.CatalogRegistration{
					Node:           ConsulAWSNodeName,
					SkipNodeUpdate: true,
					Check: &api.AgentCheck{
						CheckID:   "check" + id(serviceID, n.host, n.port),
						ServiceID: id(serviceID, n.host, n.port),
						Node:      "consul-aws",
						Name:      "AWS Route53 Health Check",
						Status:    string(h),
					},
				}
				_, err := c.client.Catalog().Register(&reg, nil)
				if err != nil {
					c.log.Error("cannot create healthcheck", "id", id(k, n.host, n.port), "error", err.Error())
				} else {
					count++
				}
			}(k, n, h)
		}
	}
	wg.Wait()
	return count
}

func (c *consul) remove(services map[string]service) int {
	wg := sync.WaitGroup{}
	count := 0
	for k, s := range services {
		if !s.fromAWS {
			continue
		}
		for h, nodes := range s.nodes {
			for p := range nodes {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()
					_, err := c.client.Catalog().Deregister(&api.CatalogDeregistration{Node: ConsulAWSNodeName, ServiceID: id}, nil)
					if err != nil {
						c.log.Error("cannot remove service", "error", err.Error())
					} else {
						count++
					}
				}(id(k, h, p))
			}
		}
	}
	wg.Wait()
	return count
}
