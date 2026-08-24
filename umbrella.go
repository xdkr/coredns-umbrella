package umbrella

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

type Umbrella struct {
	Next   plugin.Handler
	option optionTemplate
}

func (u *Umbrella) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
}

func (u *Umbrella) Name() string { return pluginName }
