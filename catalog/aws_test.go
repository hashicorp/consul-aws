package catalog

import (
	"testing"

	x "github.com/aws/aws-sdk-go-v2/aws"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdi "github.com/aws/aws-sdk-go-v2/service/servicediscovery/servicediscoveryiface"
	"github.com/stretchr/testify/require"
)

type mSDClint struct {
	sdi.ServiceDiscoveryAPI
}

func (m *mSDClint) CreateServiceRequest(input *sd.CreateServiceRequest) (*sd.CreateServiceOutput, error) {
	return nil, nil
}

func (m *mSDClint) RegisterInstanceRequest(input *sd.RegisterInstanceRequest) (*sd.RegisterInstanceOutput, error) {
	return nil, nil
}

func TestAWSTransformNodes(t *testing.T) {
	a := aws{}
	nodes := []sd.InstanceSummary{
		{Id: x.String("one"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "1"}},
		{Id: x.String("two"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.2", "AWS_INSTANCE_PORT": "A"}},
		{Id: x.String("three"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.3"}},
		{Id: x.String("four"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "2"}},
		{Id: x.String("five"), Attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.4", "AWS_INSTANCE_PORT": "4", "custom": "aha"}},
	}
	expected := map[string]map[int]node{
		"1.1.1.1": {
			1: {port: 1, host: "1.1.1.1", awsID: "one", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "1"}},
			2: {port: 2, host: "1.1.1.1", awsID: "four", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.1", "AWS_INSTANCE_PORT": "2"}},
		},
		"1.1.1.2": {
			0: {port: 0, host: "1.1.1.2", awsID: "two", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.2", "AWS_INSTANCE_PORT": "A"}},
		},
		"1.1.1.3": {
			0: {port: 0, host: "1.1.1.3", awsID: "three", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.3"}},
		},
		"1.1.1.4": {
			4: {port: 4, host: "1.1.1.4", awsID: "five", attributes: map[string]string{"AWS_INSTANCE_IPV4": "1.1.1.4", "AWS_INSTANCE_PORT": "4", "custom": "aha"}},
		},
	}
	require.Equal(t, expected, a.transformNodes(nodes))
}

func TestAWSTransformServices(t *testing.T) {
	a := aws{}
	services := []sd.ServiceSummary{
		{Id: x.String("one"), Name: x.String("web"), Description: &awsServiceDescription},
		{Id: x.String("two"), Name: x.String("redis")},
	}
	expected := map[string]service{
		"web":   {id: "one", name: "web", awsID: "one", fromConsul: true},
		"redis": {id: "two", name: "redis", awsID: "two", fromConsul: false},
	}
	require.Equal(t, expected, a.transformServices(services))
}

func TestAWSTransformNamespace(t *testing.T) {
	a := aws{}
	type variant struct {
		namespace sd.Namespace
		expected  namespace
	}
	variants := []variant{
		{namespace: sd.Namespace{Name: x.String("A"), Id: x.String("1"), Type: sd.NamespaceTypeDnsPublic}, expected: namespace{name: "A", id: "1", isHTTP: false}},
		{namespace: sd.Namespace{Name: x.String("B"), Id: x.String("2"), Type: sd.NamespaceTypeHttp}, expected: namespace{name: "B", id: "2", isHTTP: true}},
	}

	for _, v := range variants {
		require.Equal(t, v.expected, a.transformNamespace(&v.namespace))
	}
}
