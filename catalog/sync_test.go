// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package catalog

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	awssdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"

	"github.com/hashicorp/consul/api"

	"github.com/hashicorp/consul-aws/internal/flags"
)

// TestSync is an integration test that creates a service in Consul and AWS.
// Documentation on setup can be found in the engineering docs for `consul-aws`.
func TestSync(t *testing.T) {
	if len(os.Getenv("INTTEST")) == 0 {
		t.Skip("Set INTTEST=1 to enable integration tests")
	}
	namespaceID := os.Getenv("NAMESPACEID")
	if namespaceID == "" {
		t.Fatalf("The NAMESPACEID variable must be set.")
	}
	runSyncTest(t, namespaceID)
}

func runSyncTest(t *testing.T, namespaceID string) {
	// Test Setup
	config, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		t.Fatalf("Error retrieving AWS session: %s", err)
	}
	awssdClient := awssd.NewFromConfig(config)

	f := flags.HTTPFlags{}
	consulClient, err := f.APIClient()
	if err != nil {
		t.Fatalf("Error connecting to Consul agent: %s", err)
	}

	consulServiceID := "r1"
	consulServiceName := "redis"
	awsServiceName := "web"

	err = createServiceInConsul(consulClient, consulServiceID, consulServiceName)
	if err != nil {
		t.Fatalf("error creating service in Consul: %s", err)
	}

	awsServiceID, err := createServiceInAWS(awssdClient, namespaceID, awsServiceName)
	if err != nil {
		t.Fatalf("error creating service %s in aws: %s", awsServiceName, err)
	}
	err = createInstanceInAWS(awssdClient, awsServiceID)
	if err != nil {
		t.Fatalf("error creating instance in aws: %s", err)
	}

	stop := make(chan struct{})
	stopped := make(chan struct{})
	go Sync(
		true, true, namespaceID,
		"consul_", "aws_",
		"1s", 0, true,
		awssdClient, consulClient,
		stop, stopped,
	)

	doneC := make(chan struct{})
	doneA := make(chan struct{})
	go func() {
		if err := checkForImportedAWSService(consulClient, "aws_"+awsServiceName, namespaceID, awsServiceID, 100); err != nil {
			t.Error(err)
		} else {
			close(doneA)
		}
	}()
	go func() {
		if err := checkForImportedConsulService(awssdClient, namespaceID, "consul_"+consulServiceName, 100); err != nil {
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

	err = deleteInstanceInAWS(awssdClient, awsServiceID)
	if err != nil {
		t.Logf("error deregistering instance in AWS: %s", err)
	}
	err = deleteServiceInAWS(awssdClient, awsServiceID)
	if err != nil {
		t.Logf("error deleting service in AWS: %s", err)
	}
	err = deleteServiceInConsul(consulClient, consulServiceID)
	if err != nil {
		t.Logf("error deleting service in Consul: %s", err)
	}

	select {
	case <-time.After((WaitTime * 5) * time.Second):
	}
	if err = checkForImportedAWSService(consulClient, "aws_"+awsServiceName, namespaceID, awsServiceID, 1); err == nil {
		t.Error("Expected that the imported aws services is deleted")
	}
	if err = checkForImportedConsulService(awssdClient, namespaceID, "consul_"+consulServiceName, 1); err == nil {
		t.Error("Expected that the imported consul services is deleted")
	}

	close(stop)
	<-stopped
}
func createServiceInConsul(c *api.Client, id, name string) error {
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
		},
	}
	_, err := c.Catalog().Register(&reg, nil)
	return err
}

func deleteServiceInConsul(c *api.Client, id string) error {
	_, err := c.Catalog().Deregister(&api.CatalogDeregistration{Node: ConsulAWSNodeName, ServiceID: id}, nil)
	return err
}

func createServiceInAWS(a *awssd.Client, namespaceID, name string) (string, error) {
	ttl := int64(60)
	input := awssd.CreateServiceInput{
		Name:        &name,
		NamespaceId: &namespaceID,
		DnsConfig: &awssdtypes.DnsConfig{
			DnsRecords: []awssdtypes.DnsRecord{
				{TTL: &ttl, Type: awssdtypes.RecordTypeSrv},
			},
			RoutingPolicy: awssdtypes.RoutingPolicyMultivalue,
		},
		HealthCheckCustomConfig: &awssdtypes.HealthCheckCustomConfig{},
	}
	resp, err := a.CreateService(context.TODO(), &input)
	if err != nil {
		return "", err
	}
	return *resp.Service.Id, nil
}

func createInstanceInAWS(a *awssd.Client, serviceID string) error {
	_, err := a.RegisterInstance(context.TODO(), &awssd.RegisterInstanceInput{
		ServiceId:  &serviceID,
		InstanceId: &serviceID,
		Attributes: map[string]string{
			"AWS_INSTANCE_IPV4": "127.0.0.1",
			"AWS_INSTANCE_PORT": "8000",
			"FUBAR":             "BARFU",
		},
	})
	//if err != nil {
	//	return err
	//}
	//
	//time.Sleep(30 * time.Second) // This is a hack to wait for the instance to be created
	//_, err = a.UpdateInstanceCustomHealthStatus(context.TODO(), &awssd.UpdateInstanceCustomHealthStatusInput{
	//	InstanceId: &serviceID,
	//	ServiceId:  &serviceID,
	//	Status:     awssdtypes.CustomHealthStatusHealthy,
	//})
	return err
}

func deleteInstanceInAWS(a *awssd.Client, id string) error {
	_, err := a.DeregisterInstance(context.TODO(), &awssd.DeregisterInstanceInput{ServiceId: &id, InstanceId: &id})
	return err
}

func deleteServiceInAWS(a *awssd.Client, id string) error {
	var err error
	for i := 0; i < 50; i++ {
		_, err = a.DeleteService(context.TODO(), &awssd.DeleteServiceInput{Id: &id})
		if err != nil {
			time.Sleep(100 * time.Millisecond)
		} else {
			break
		}
	}
	return err
}

func checkForImportedAWSService(c *api.Client, name, namespaceID, serviceID string, repeat int) error {
	for i := 0; i < repeat; i++ {
		services, _, err := c.Catalog().Services(nil)
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
				cservices, _, err := c.Catalog().Service(name, ConsulAWSTag, nil)
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

func checkForImportedConsulService(a *awssd.Client, namespaceID, name string, repeat int) error {
	for i := 0; i < repeat; i++ {
		paginator := awssd.NewListServicesPaginator(a, &awssd.ListServicesInput{
			Filters: []awssdtypes.ServiceFilter{
				{
					Name:      awssdtypes.ServiceFilterNameNamespaceId,
					Condition: awssdtypes.FilterConditionEq,
					Values:    []string{namespaceID},
				},
			},
		})

		for paginator.HasMorePages() {
			p, err := paginator.NextPage(context.TODO())
			if err != nil {
				return fmt.Errorf("error paging through services: %s", err)
			}

			for _, s := range p.Services {
				if *s.Name == name {
					if !(s.Description != nil || *s.Description == awsServiceDescription) {
						return fmt.Errorf("consul description is missing on aws service")
					}
					var instance *awssdtypes.InstanceSummary
					for i := 0; i < 20; i++ {
						out, err := a.ListInstances(context.TODO(), &awssd.ListInstancesInput{
							ServiceId: s.Id,
						})
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
