package catalog

import (
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
)

func TestConsulRekeyHealths(t *testing.T) {
	type variant struct {
		nodes    map[string]map[int]node
		healths  map[string]health
		expected map[string]health
	}
	variants := []variant{
		{
			nodes:    map[string]map[int]node{},
			healths:  map[string]health{},
			expected: map[string]health{},
		},
		{
			nodes: map[string]map[int]node{
				"1.1.1.1": {8000: {port: 8000, awsID: "X1"}},
			},
			healths: map[string]health{
				"web_1.1.1.1_8000": passing,
			},
			expected: map[string]health{
				"X1": passing,
			},
		},
	}

	for _, v := range variants {
		c := consul{
			services: map[string]service{
				"s1": {
					nodes:   v.nodes,
					healths: v.healths,
				},
			},
		}
		require.Equal(t, v.expected, c.rekeyHealths("s1", c.services["s1"].healths))
	}
}

func TestConsulTransformServices(t *testing.T) {
	c := consul{awsPrefix: "aws_"}
	services := map[string][]string{"s1": {"abc"}, "aws_s2": {ConsulAWSTag}}
	expected := map[string]service{"s1": {id: "s1", name: "s1", consulID: "s1"}, "s2": {id: "aws_s2", name: "s2", consulID: "aws_s2", fromAWS: true}}

	require.Equal(t, expected, c.transformServices(services))
}

func TestConsulTransformNodes(t *testing.T) {
	c := consul{}
	nodes := []*api.CatalogService{
		{
			ServiceAddress: "1.1.1.1",
			ServicePort:    1,
			ServiceID:      "s1",
			ServiceMeta:    map[string]string{ConsulAWSID: "aws1"},
		},
		{
			Address:     "1.1.1.2",
			ServicePort: 1,
			ServiceID:   "s1",
			ServiceMeta: map[string]string{ConsulAWSID: "aws1"},
		},
		{
			Address:     "1.1.1.3",
			ServicePort: 3,
			ServiceID:   "s2",
			ServiceMeta: map[string]string{"A": "B"},
		},
	}
	expected := map[string]map[int]node{
		"1.1.1.1": {1: {port: 1, host: "1.1.1.1", awsID: "aws1", consulID: "s1", attributes: map[string]string{ConsulAWSID: "aws1"}}},
		"1.1.1.2": {1: {port: 1, host: "1.1.1.2", awsID: "aws1", consulID: "s1", attributes: map[string]string{ConsulAWSID: "aws1"}}},
		"1.1.1.3": {3: {port: 3, host: "1.1.1.3", consulID: "s2", attributes: map[string]string{"A": "B"}}},
	}
	require.Equal(t, expected, c.transformNodes(nodes))
}

func TestConsulTransformHeath(t *testing.T) {
	c := consul{}
	healths := api.HealthChecks{
		&api.HealthCheck{Status: "passing", ServiceID: "s1"},
		&api.HealthCheck{Status: "critical", ServiceID: "s2"},
		&api.HealthCheck{Status: "warning", ServiceID: "s3"},
	}
	expected := map[string]health{
		"s1": passing,
		"s2": critical,
		"s3": unknown,
	}
	require.Equal(t, expected, c.transformHealth(healths))
}
