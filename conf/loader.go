package conf

import (
	"bytes"
	"context"

	"github.com/pkg/errors"

	"github.com/sipt/shuttle/dns"
	"github.com/sipt/shuttle/global"
	"github.com/sipt/shuttle/group"
	"github.com/sipt/shuttle/rule"

	"github.com/sipt/shuttle/server"

	"github.com/sipt/shuttle/conf/marshal"
	"github.com/sipt/shuttle/conf/model"
	"github.com/sipt/shuttle/conf/storage"
)

// LoadConfig
// typ:
func LoadConfig(ctx context.Context, typ, encode string, params map[string]string, notify func()) (*model.Config, error) {
	s, err := storage.Get(typ, params)
	if err != nil {
		return nil, err
	}
	data, err := s.Load()
	if err != nil {
		return nil, err
	}
	m, err := marshal.Get(encode, params)
	if err != nil {
		return nil, err
	}
	config, err := m.UnMarshal(data)
	if err != nil {
		return nil, err
	}
	err = s.RegisterNotify(ctx, notify)
	if err != nil {
		return nil, err
	}
	buffer := bytes.NewBuffer(data)
	for _, v := range config.Include {
		c, err := storage.Get(v.Typ, v.Params)
		if err != nil {
			return nil, err
		}
		data, err = c.Load()
		if err != nil {
			return nil, err
		}
		buffer.WriteByte('\n')
		buffer.Write(data)
		err = c.RegisterNotify(ctx, notify)
		if err != nil {
			return nil, err
		}
	}
	config, err = m.UnMarshal(buffer.Bytes())
	if err != nil {
		return nil, err
	}
	config.Info.Name = s.Name()
	return config, nil
}

func ApplyConfig(ctx context.Context, config *model.Config) error {
	servers, err := server.ApplyConfig(config)
	if err != nil {
		return err
	}
	groups, err := group.ApplyConfig(ctx, config, servers)
	if err != nil {
		return err
	}

	proxyName := make(map[string]bool)
	for _, v := range servers {
		proxyName[v.Name()] = true
	}
	for _, v := range groups {
		proxyName[v.Name()] = true
	}
	defaultRule := &rule.Rule{
		Typ:   "FINAL",
		Proxy: server.Direct,
	}
	dnsHandle, err := dns.ApplyConfig(config, func(ctx context.Context, domain string) *dns.DNS { return nil })
	if err != nil {
		return errors.Wrapf(err, "[dns.ApplyConfig] failed")
	}
	ruleHandle, err := rule.ApplyConfig(config, proxyName, func(ctx context.Context, info rule.RequestInfo) *rule.Rule {
		return defaultRule
	}, dnsHandle)
	if err != nil {
		return errors.Wrapf(err, "[rule.ApplyConfig] failed")
	}
	profile, err := global.NewProfile(config, dnsHandle, ruleHandle, groups, servers)
	if err != nil {
		return errors.Wrapf(err, "create profile failed")
	}
	global.AddProfile(config.Info.Name, profile)
	global.AddNamespace("default", ctx, profile)
	return nil
}