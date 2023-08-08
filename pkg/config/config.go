package config

import (
	capi "github.com/hashicorp/consul/api"
	"sigs.k8s.io/yaml"
)

type Credential struct {
	AccessKey       string `json:"accessKey"`
	AccessKeySecret string `json:"accessKeySecret"`
	Region          string `json:"region"`
}

// Config exporter config
type Config struct {
	Credentials map[string]Credential `json:"credentials"`
	// todo: add extra labels
	// mark!! 实际上labels应该加到metrics里面，因为需要区分资源属于哪些组
	//Labels  map[string]string    `json:"labels,omitempty"`
	Metrics map[string][]*Metric `json:"metrics"` // mapping for namespace and metrics
	//InstanceInfos []string             `json:"instanceInfos"`
}

// SetDefaults 设置默认值
func (c *Config) SetDefaults() {
	for key := range c.Credentials {
		if c.Credentials[key].Region == "" {
			credential := c.Credentials[key]
			credential.Region = "cn-hangzhou"
			c.Credentials[key] = credential
		}
	}
	for _, metrics := range c.Metrics {
		for i := range metrics {
			metrics[i].setDefaults() // Metrics的方法setDefaults
		}
	}
}

// Parse parse config from consul
func Parse(consul *capi.Client, key string) (*Config, error) {
	p, _, err := consul.KV().Get(key, nil)
	if err != nil {
		return nil, err
	}
	var cfg Config
	err = yaml.Unmarshal(p.Value, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
	// 	b, err := ioutil.ReadFile(path)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	var cfg Config
	// 	if err = yaml.Unmarshal(b, &cfg); err != nil {
	// 		return nil, err
	// 	}
	// 	cfg.SetDefaults()
	// 	return &cfg, nil
}
