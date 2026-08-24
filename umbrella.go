package umbrella

import (
	"context"
	"net/netip"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

type Umbrella struct {
	Next   plugin.Handler
	option optionTemplate
}

func (u *Umbrella) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	client, ok := clientIP(w)
	if !ok {
		return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
	}

	hadEDNS := r.IsEdns0() != nil
	request := r.Copy()
	opt := request.IsEdns0()
	if opt == nil {
		request.SetEdns0(dns.MinMsgSize, false)
		opt = request.IsEdns0()
	}

	removeOption(request, optionCode)
	opt.Option = append(opt.Option, u.option.option(client))

	writer := &responseWriter{ResponseWriter: w, stripOPT: !hadEDNS}
	return plugin.NextOrFailure(u.Name(), u.Next, ctx, writer, request)
}

func (u *Umbrella) Name() string { return pluginName }

func clientIP(w dns.ResponseWriter) (netip.Addr, bool) {
	state := request.Request{W: w}
	ip, err := netip.ParseAddr(state.IP())
	if err != nil {
		return netip.Addr{}, false
	}

	return ip.Unmap().WithZone(""), true
}

type responseWriter struct {
	dns.ResponseWriter
	stripOPT bool
}

func (w *responseWriter) WriteMsg(response *dns.Msg) error {
	if w.stripOPT {
		if !hasOPT(response) {
			return w.ResponseWriter.WriteMsg(response)
		}

		response = response.Copy()
		removeOPT(response)
		return w.ResponseWriter.WriteMsg(response)
	}

	if !hasOption(response, optionCode) {
		return w.ResponseWriter.WriteMsg(response)
	}

	response = response.Copy()
	removeOption(response, optionCode)
	return w.ResponseWriter.WriteMsg(response)
}

func hasOPT(message *dns.Msg) bool {
	for _, record := range message.Extra {
		if _, ok := record.(*dns.OPT); ok {
			return true
		}
	}
	return false
}

func hasOption(message *dns.Msg, code uint16) bool {
	for _, record := range message.Extra {
		opt, ok := record.(*dns.OPT)
		if !ok {
			continue
		}
		for _, option := range opt.Option {
			if option.Option() == code {
				return true
			}
		}
	}
	return false
}

func removeOPT(message *dns.Msg) {
	records := message.Extra[:0]
	for _, record := range message.Extra {
		if _, ok := record.(*dns.OPT); !ok {
			records = append(records, record)
		}
	}
	message.Extra = records
}

func removeOption(message *dns.Msg, code uint16) {
	for _, record := range message.Extra {
		opt, ok := record.(*dns.OPT)
		if !ok {
			continue
		}

		options := opt.Option[:0]
		for _, option := range opt.Option {
			if option.Option() != code {
				options = append(options, option)
			}
		}
		opt.Option = options
	}
}
