package client

import (
	"aliyun-exporter/pkg/config"
	"aliyun-exporter/pkg/ratelimit"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

var ignores = map[string]struct{}{
	"timestamp": {},
	"Maximum":   {},
	"Minimum":   {},
	"Average":   {},
}

// map for all avaliable namespaces
// todo: is there a way to add desc into yaml file?
var allNamespaces = map[string]string{
	"acs_ecs_dashboard":              "Elastic Compute Service (ECS)",
	"acs_containerservice_dashboard": "Container Service for Swarm",
	"acs_kubernetes":                 "Container Service for Kubernetes (ACK)",
	"acs_oss_dashboard":              "Object Storage Service (OSS)",
	"acs_slb_dashboard":              "Server Load Balancer (SLB)",
	"acs_vpc_eip":                    "Elastic IP addresses (EIPs)",
	"acs_nat_gateway":                "NAT Gateway",
	"acs_anycast_eip":                "Anycast Elastic IP address (EIP)",
	"acs_rds_dashboard":              "ApsaraDB RDS",
	"acs_mongodb":                    "ApsaraDB for MongoDB",
	"acs_memcache":                   "ApsaraDB for Memcache",
	"acs_kvstore":                    "ApsaraDB for Redis",
	"acs_hitsdb":                     "Time Series Database (TSDB)",
	"acs_clickhouse":                 "ClickHouse",
	"acs_cds":                        "ApsaraDB for Cassandra",
	"waf":                            "Web Application Firewall (WAF)",
	"acs_elasticsearch":              "Elasticsearch",
	"acs_mns_new":                    "queues of Message Service (MNS)",
	"acs_kafka":                      "Message Queue for Apache Kafka",
	"acs_amqp":                       "Alibaba Cloud Message Queue for AMQP instances",
}

// AllNamespaces return allNamespaces
func AllNamespaces() map[string]string {
	return allNamespaces
}

// allNamesOfNamespaces return all avaliable namespaces
func allNamesOfNamespaces() []string {
	ss := make([]string, 0, len(allNamespaces))
	for name := range allNamespaces {
		ss = append(ss, name)
	}
	return ss
}

// Datapoint datapoint
type Datapoint map[string]interface{}

// Get return value for measure
func (d Datapoint) Get(measure string) float64 {
	v, ok := d[measure]
	if !ok {
		return 0
	}
	return v.(float64)
}

// Labels return labels that not in ignores
func (d Datapoint) Labels() []string {
	labels := make([]string, 0)
	for k := range d {
		if _, ok := ignores[k]; !ok {
			labels = append(labels, k)
		}
	}
	sort.Strings(labels)
	return labels
}

// Values return values for lables
func (d Datapoint) Values(labels ...string) []string {
	values := make([]string, 0, len(labels))
	for i := range labels {
		values = append(values, fmt.Sprintf("%s", d[labels[i]]))
	}
	return values
}

// MetricClient wrap cms.client
type MetricClient struct {
	cloudID string
	cms     *cms.Client
	logger  log.Logger
}

// NewMetricClient create metric Client
func NewMetricClient(cloudID, ak, secret, region string, logger log.Logger) (*MetricClient, error) {
	cmsClient, err := cms.NewClientWithAccessKey(region, ak, secret)
	if err != nil {
		return nil, err
	}
	//cmsClient.SetTransport(rt)
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &MetricClient{cloudID, cmsClient, logger}, nil
}

func (c *MetricClient) SetTransport(rate int) {
	rt := ratelimit.New(rate)
	c.cms.SetTransport(rt)
}

// retrive get datapoints for metric
func (c *MetricClient) retrive(sub string, name string, period string) ([]Datapoint, error) {
	req := cms.CreateDescribeMetricLastRequest()
	req.ReadTimeout = 50 * time.Second
	req.Namespace = sub
	req.MetricName = name
	req.Period = period
	resp, err := c.cms.DescribeMetricLast(req)
	if err != nil {
		return nil, err
	}

	var datapoints []Datapoint
	if err = json.Unmarshal([]byte(resp.Datapoints), &datapoints); err != nil {
		// some execpected error
		level.Debug(c.logger).Log("content", resp.GetHttpContentString(), "error", err)
		return nil, err
	}
	return datapoints, nil
}

// Collect do collect metrics into channel
func (c *MetricClient) Collect(namespace string, sub string, m *config.Metric, ch chan<- prometheus.Metric) {
	if m.Name == "" {
		level.Warn(c.logger).Log("msg", "metric name must been set")
		return
	}

	datapoints, err := c.retrive(sub, m.Name, m.Period)
	if err != nil {
		level.Error(c.logger).Log("msg", "failed to retrive datapoints", "cloudID", c.cloudID, "sub", sub, "name", m.String(), "error", err)
		return
	}
	// 此处增加对spec的过滤,只发送配置的instance的metric
	// 当配置文件没有配置spce的时候,打印提示，然后输出所有的实例的metric
	if len(m.Spec.Claim) == 0 {
		level.Warn(c.logger).Log("msg", "you could appoint \"spec\" expose to particular instance's metric", "metric", m.Name)
		for _, dp := range datapoints {
			val := dp.Get(m.Measure)
			ch <- prometheus.MustNewConstMetric(
				m.Desc(namespace, sub, dp.Labels()...),
				prometheus.GaugeValue,
				val,
				append(dp.Values(m.Dimensions...), c.cloudID)...,
			)
		}
	} else {
		for _, dp := range datapoints {
			for i := 0; i < len(m.Spec.Claim); i++ {
				for _, instance := range m.Spec.Claim[i].Instance {
					if dp["instanceId"].(string) == instance {
						val := dp.Get(m.Measure)
						ch <- prometheus.MustNewConstMetric(
							m.Desc(namespace, sub, dp.Labels()...),
							prometheus.GaugeValue,
							val,
							append(dp.Values(m.Dimensions...), c.cloudID, m.Spec.Claim[i].App, m.Spec.Claim[i].Team)...,
						)
					}
				}
			}
		}
	}

}

// DescribeMetricMetaList return metrics meta list
func (c *MetricClient) DescribeMetricMetaList(namespaces ...string) (map[string][]cms.Resource, error) {
	namespaces = filterNamespaces(namespaces...)
	m := make(map[string][]cms.Resource)
	for _, ns := range namespaces {
		req := cms.CreateDescribeMetricMetaListRequest()
		req.Namespace = ns
		req.PageSize = requests.NewInteger(100)
		resp, err := c.cms.DescribeMetricMetaList(req)
		if err != nil {
			return nil, err
		}
		level.Debug(c.logger).Log("content", resp.GetHttpContentString())
		m[ns] = resp.Resources.Resource
	}
	return m, nil
}

func filterNamespaces(namespaces ...string) []string {
	if len(namespaces) == 0 {
		return allNamesOfNamespaces()
	}
	filters := make([]string, 0)
	for _, ns := range namespaces {
		if ns == "all" {
			return allNamesOfNamespaces()
		}
		if _, ok := allNamespaces[ns]; ok {
			filters = append(filters, ns)
		}
	}
	return filters
}
