package catalog

import (
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/command/flags"
)

func TestSync(t *testing.T) {
	// if len(os.Getenv("INTTEST")) == 0 {
	// 	t.Skip("no int test env")
	// }
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
	err = createServiceInConsul(c, cID, cName)
	if err != nil {
		t.Fatalf("error creating service in aws: %s", err)
	}

	namespaceID := os.Getenv("NAMESPACEID")
	if len(namespaceID) == 0 {
		namespaceID = "ns-aflsxmueeuewzthz"
	}
	aID, err := createServiceInAWS(a, namespaceID, "web")
	if err != nil {
		t.Fatalf("error creating service in aws: %s", err)
	}
	err = createInstanceInAWS(a, aID)
	if err != nil {
		t.Fatalf("error creating instance in aws: %s", err)
	}

	stop := make(chan struct{})
	stopped := make(chan struct{})
	go Sync(
		true, true, namespaceID,
		"consul_", "aws_",
		"0", 60, true,
		a, c,
		stop, stopped,
	)

	doneC := make(chan struct{})
	doneA := make(chan struct{})
	go checkForImportedAWSService(t, c, "aws_web", namespaceID, aID, doneC)
	go checkForImportedConsulService(t, a, namespaceID, "consul_redis", doneA)

	select {
	case <-time.After(10 * time.Second):
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
	deleteServiceInConsul(c, cID)

	select {
	case <-time.After(10 * time.Second):
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

func deleteServiceInConsul(c *api.Client, id string) {
	c.Catalog().Deregister(&api.CatalogDeregistration{Node: ConsulAWSNodeName, ServiceID: id}, nil)
}

func createServiceInAWS(a *sd.ServiceDiscovery, namespaceID, name string) (string, error) {
	ttl := int64(60)
	req := a.CreateServiceRequest(&sd.CreateServiceInput{
		DnsConfig: &sd.DnsConfig{
			DnsRecords: []sd.DnsRecord{
				{TTL: &ttl, Type: sd.RecordTypeA},
				{TTL: &ttl, Type: sd.RecordTypeSrv},
			},
			NamespaceId:   &namespaceID,
			RoutingPolicy: sd.RoutingPolicyMultivalue,
		},
		Name: &name,
	})
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

func checkForImportedAWSService(t *testing.T, c *api.Client, name, namespaceID, serviceID string, done chan struct{}) {
	for {
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
					t.Error("aws tag is missing on consul service")
					return
				}
				defer close(done)
				cservices, _, err := c.Catalog().Service(name, ConsulAWSTag, nil)
				if err != nil {
					return
				}
				if len(cservices) != 1 {
					t.Error("not 1 services")
					return
				}
				m := cservices[0].ServiceMeta
				if m["FUBAR"] != "BARFU" {
					t.Errorf("custom meta doesn't match: %s", m["FUBAR"])
				}
				if m[ConsulSourceKey] != ConsulAWSTag {
					t.Errorf("%s meta doesn't match: %s", ConsulSourceKey, m[ConsulSourceKey])
				}
				if m[ConsulAWSNS] != namespaceID {
					t.Errorf("%s meta doesn't match: %s", ConsulAWSNS, m[ConsulAWSNS])
				}
				if m[ConsulAWSID] != serviceID {
					t.Errorf("%s meta doesn't match: %s", ConsulAWSID, m[ConsulAWSID])
				}
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func checkForImportedConsulService(t *testing.T, a *sd.ServiceDiscovery, namespaceID, name string, done chan struct{}) {
	for {
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
						t.Error("consul description is missing on aws service")
						return
					}
					defer close(done)
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
						t.Errorf("couldn't get instance")
						return
					}
					m := instance.Attributes
					if m["AWS_INSTANCE_IPV4"] != "127.0.0.1" {
						t.Error("AWS_INSTANCE_IPV4 not correct")
					}
					if m["AWS_INSTANCE_PORT"] != "6379" {
						t.Error("AWS_INSTANCE_PORT not correct")
					}
					if m["BARFU"] != "FUBAR" {
						t.Error("custom meta not correct")
					}
					return
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
