// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package catalog

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/command/flags"
)

func TestSync(t *testing.T) {
	if len(os.Getenv("INTTEST")) == 0 {
		t.Skip("Set INTTEST=1 to enable integration tests")
	}
	awsNamespaceID := os.Getenv("NAMESPACEID")
	if len(awsNamespaceID) == 0 {
		awsNamespaceID = "ns-n5qqli2346hqood4"
	}
	runSyncTest(t, awsNamespaceID, "", "")
}

func runSyncTest(t *testing.T, awsNamespaceID, adminPartition, namespace string) {
	config, err := external.LoadDefaultAWSConfig()
	if err != nil {
		t.Fatalf("Error retrieving AWS session: %s", err)
	}
	a := sd.New(config)

	f := flags.HTTPFlags{}
	c, err := f.APIClient()
	if err != nil {
		t.Fatalf("Error connecting to Consul agent: %s", err)
	}

	cID := "r1"
	cName := "redis"
	aName := "web"

	err = tryCreateAdminPartition(c, adminPartition)
	if err != nil {
		t.Fatalf("error creating partition in Consul: %s", err)
	}

	err = tryCreateNamespace(c, namespace, adminPartition)
	if err != nil {
		t.Fatalf("error creating namespace in Consul: %s", err)
	}

	// Creating redis in Consul
	err = createServiceInConsul(c, cID, cName, namespace, adminPartition)
	if err != nil {
		t.Fatalf("error creating service in Consul: %s", err)
	}

	// Creating web and instance in AWS
	aID, err := createServiceInAWS(a, awsNamespaceID, aName)
	if err != nil {
		t.Fatalf("error creating service %s in aws: %s", aName, err)
	}
	err = createInstanceInAWS(a, aID)
	if err != nil {
		t.Fatalf("error creating instance in aws: %s", err)
	}

	stop := make(chan struct{})
	stopped := make(chan struct{})
	syncInput := &SyncInput{
		ToAWS:                true,
		ToConsul:             true,
		AWSNamespaceID:       awsNamespaceID,
		ConsulPrefix:         "consul_",
		AWSPrefix:            "aws_",
		AWSPullInterval:      "0",
		AWSDNSTTL:            0,
		Stale:                true,
		AWSClient:            a,
		ConsulClient:         c,
		ConsulNamespace:      namespace,
		ConsulAdminPartition: adminPartition,
	}

	go Sync(syncInput, stop, stopped)

	doneC := make(chan struct{})
	doneA := make(chan struct{})
	go func() {
		if err := checkForImportedAWSService(c, "aws_"+aName, awsNamespaceID, aID, 100, syncInput.ConsulNamespace, syncInput.ConsulAdminPartition); err != nil {
			t.Error(err)
		} else {
			close(doneA)
		}
	}()
	go func() {
		if err := checkForImportedConsulService(a, awsNamespaceID, "consul_"+cName, 100); err != nil {
			t.Error(err)
		} else {
			close(doneC)
		}
	}()

	select {
	case <-time.After(20 * time.Second):
	}

	select {
	case <-doneC:
	default:
		t.Error("service was not imported in consul")
	}
	select {
	case <-doneA:
	default:
		t.Error("service was not imported in aws")
	}

	err = deleteInstanceInAWS(a, aID)
	if err != nil {
		t.Logf("error deregistering instance: %s", err)
	}
	err = deleteServiceInAWS(a, aID)
	if err != nil {
		t.Logf("error deleting service: %s", err)
	}
	deleteServiceInConsul(c, cID, namespace, adminPartition)

	select {
	case <-time.After((WaitTime * 3) * time.Second):
	}
	if err = checkForImportedAWSService(c, "aws_"+aName, awsNamespaceID, aID, 1, syncInput.ConsulNamespace, syncInput.ConsulAdminPartition); err == nil {
		t.Error("Expected that the imported aws services is deleted")
	}
	if err = checkForImportedConsulService(a, awsNamespaceID, "consul_"+cName, 1); err == nil {
		t.Error("Expected that the imported consul services is deleted")
	}

	close(stop)
	<-stopped
}
func createServiceInConsul(c *api.Client, id, name string, namespace string, adminPartition string) error {
	reg := api.CatalogRegistration{
		Node:           ConsulAWSNodeName,
		Address:        "127.0.0.1",
		SkipNodeUpdate: true,
		Service: &api.AgentService{
			ID:      id,
			Service: name,
			Address: "127.0.0.1",
			Port:    6379,
			Meta: map[string]string{
				"BARFU": "FUBAR",
			},
			Namespace: namespace,
			Partition: adminPartition,
		},
		Partition: adminPartition,
	}

	_, err := c.Catalog().Register(&reg, nil)
	return err
}

func deleteServiceInConsul(c *api.Client, id, namespace, adminPartition string) {
	c.Catalog().Deregister(
		&api.CatalogDeregistration{
			Node:      ConsulAWSNodeName,
			ServiceID: id,
			Partition: adminPartition,
			Namespace: namespace,
		},
		&api.WriteOptions{
			Partition: adminPartition,
			Namespace: namespace,
		},
	)
}

func createServiceInAWS(a *sd.ServiceDiscovery, namespaceID, name string) (string, error) {
	ttl := int64(60)
	input := sd.CreateServiceInput{
		Name:        &name,
		NamespaceId: &namespaceID,
		DnsConfig: &sd.DnsConfig{
			DnsRecords: []sd.DnsRecord{
				{TTL: &ttl, Type: sd.RecordTypeSrv},
			},
			RoutingPolicy: sd.RoutingPolicyMultivalue,
		},
	}
	req := a.CreateServiceRequest(&input)
	resp, err := req.Send()
	if err != nil {
		return "", err
	}
	return *resp.Service.Id, nil
}

func createInstanceInAWS(a *sd.ServiceDiscovery, serviceID string) error {
	req := a.RegisterInstanceRequest(&sd.RegisterInstanceInput{
		ServiceId:  &serviceID,
		InstanceId: &serviceID,
		Attributes: map[string]string{
			"AWS_INSTANCE_IPV4": "127.0.0.1",
			"AWS_INSTANCE_PORT": "8000",
			"FUBAR":             "BARFU",
		},
	})
	_, err := req.Send()
	return err
}

func deleteInstanceInAWS(a *sd.ServiceDiscovery, id string) error {
	req := a.DeregisterInstanceRequest(&sd.DeregisterInstanceInput{ServiceId: &id, InstanceId: &id})
	_, err := req.Send()
	return err
}

func deleteServiceInAWS(a *sd.ServiceDiscovery, id string) error {
	var err error
	for i := 0; i < 50; i++ {
		req := a.DeleteServiceRequest(&sd.DeleteServiceInput{Id: &id})
		_, err = req.Send()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
		} else {
			break
		}
	}
	return err
}

func checkForImportedAWSService(c *api.Client, name, namespaceID, serviceID string, repeat int, namespace, partition string) error {
	for i := 0; i < repeat; i++ {
		opts := &api.QueryOptions{
			Namespace: namespace,
			Partition: partition,
		}
		services, _, err := c.Catalog().Services(opts)
		if err == nil {
			if tags, ok := services[name]; ok {
				found := false
				for _, t := range tags {
					if t == ConsulAWSTag {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("aws tag is missing on consul service")
				}
				cservices, _, err := c.Catalog().Service(name, ConsulAWSTag, opts)
				if err != nil {
					return err
				}
				if len(cservices) != 1 {
					return fmt.Errorf("not 1 services")
				}
				m := cservices[0].ServiceMeta
				if m["FUBAR"] != "BARFU" {
					return fmt.Errorf("custom meta doesn't match: %s", m["FUBAR"])
				}
				if m[ConsulSourceKey] != ConsulAWSTag {
					return fmt.Errorf("%s meta doesn't match: %s", ConsulSourceKey, m[ConsulSourceKey])
				}
				if m[ConsulAWSNS] != namespaceID {
					return fmt.Errorf("%s meta doesn't match: expected: %s actual: %s", ConsulAWSNS, namespaceID, m[ConsulAWSNS])
				}
				if m[ConsulAWSID] != serviceID {
					return fmt.Errorf("%s meta doesn't match: expected: %s, actual: %s", ConsulAWSID, serviceID, m[ConsulAWSID])
				}
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("shrug")
}

func checkForImportedConsulService(a *sd.ServiceDiscovery, namespaceID, name string, repeat int) error {
	for i := 0; i < repeat; i++ {
		req := a.ListServicesRequest(&sd.ListServicesInput{
			Filters: []sd.ServiceFilter{{
				Name:      sd.ServiceFilterNameNamespaceId,
				Condition: sd.FilterConditionEq,
				Values:    []string{namespaceID},
			}},
		})
		p := req.Paginate()
		for p.Next() {
			for _, s := range p.CurrentPage().Services {
				if *s.Name == name {
					if !(s.Description != nil || *s.Description == awsServiceDescription) {
						return fmt.Errorf("consul description is missing on aws service")
					}
					var instance *sd.InstanceSummary
					for i := 0; i < 20; i++ {
						ireq := a.ListInstancesRequest(&sd.ListInstancesInput{
							ServiceId: s.Id,
						})
						out, err := ireq.Send()
						if err != nil {
							continue
						}
						if len(out.Instances) != 1 {
							time.Sleep(200 * time.Millisecond)
							continue
						}
						instance = &out.Instances[0]
					}
					if instance == nil {
						return fmt.Errorf("couldn't get instance")
					}
					m := instance.Attributes

					if m["AWS_INSTANCE_IPV4"] != "127.0.0.1" {
						return fmt.Errorf("AWS_INSTANCE_IPV4 not correct")
					}
					if m["AWS_INSTANCE_PORT"] != "6379" {
						return fmt.Errorf("AWS_INSTANCE_PORT not correct")
					}
					if m["BARFU"] != "FUBAR" {
						return fmt.Errorf("custom meta not correct")
					}
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("shrug")
}

func tryCreateAdminPartition(c *api.Client, name string) error {
	if name == "" {
		return nil
	}

	partition := &api.Partition{
		Name:        name,
		Description: "Partition created for integration tests",
	}
	_, _, err := c.Partitions().Create(context.TODO(), partition, nil)
	if err != nil {
		return err
	}

	return nil
}

func tryCreateNamespace(c *api.Client, name string, partition string) error {
	if name == "" {
		return nil
	}

	namespace := &api.Namespace{
		Name:        name,
		Description: "Namespace created for integration tests",
		Partition:   partition,
	}

	_, _, err := c.Namespaces().Create(namespace, nil)
	if err != nil {
		return err
	}

	return nil
}
