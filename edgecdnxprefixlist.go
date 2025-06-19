// Package example is a CoreDNS plugin that prints "example" to stdout on every packet received.
//
// It serves as an example CoreDNS plugin with numerous code comments.
package edgecdnxprefixlist

import (
	"context"
	"fmt"
	"net"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/coredns/coredns/plugin/metadata"

	"github.com/miekg/dns"
)

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin("edgecdnxprefixlist")

// Example is an example plugin to show how to write a plugin.
type EdgeCDNXPrefixList struct {
	Next    plugin.Handler
	Routing *EdgeCDNXPrefixListRouting
}

type EdgeCDNXPrefixListResponseWriter struct {
}

func (e EdgeCDNXPrefixList) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
}

func (g EdgeCDNXPrefixList) Metadata(ctx context.Context, state request.Request) context.Context {

	srcIP := net.ParseIP(state.IP())

	if o := state.Req.IsEdns0(); o != nil {
		for _, s := range o.Option {
			if e, ok := s.(*dns.EDNS0_SUBNET); ok {
				srcIP = e.Address
				break
			}
		}
	}
	metadata.SetValueFunc(ctx, g.Name()+"/location", func() string {
		if srcIP.To4() != nil {
			dest := g.Routing.RoutingV4.Find(PrefixTreeEntry{
				Prefix: net.IPNet{
					IP:   srcIP,
					Mask: net.CIDRMask(32, 32),
				},
			})

			if dest != nil {
				log.Debug(fmt.Sprintf("Found V4 prefix %s", dest))
				return dest.(PrefixTreeEntry).Location
			}
		}

		if srcIP.To16() != nil {
			dest := g.Routing.RoutingV6.Find(PrefixTreeEntry{
				Prefix: net.IPNet{
					IP:   srcIP,
					Mask: net.CIDRMask(128, 128),
				},
			})

			if dest != nil {
				log.Debug(fmt.Sprintf("Found V6 prefix %s", dest))
				return dest.(PrefixTreeEntry).Location
			}
		}
		return ""
	})

	return ctx
}

// Name implements the Handler interface.
func (e EdgeCDNXPrefixList) Name() string { return "edgecdnxprefixlist" }

// ResponsePrinter wrap a dns.ResponseWriter and will write example to standard output when WriteMsg is called.
type ResponsePrinter struct {
	dns.ResponseWriter
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter) *ResponsePrinter {
	return &ResponsePrinter{ResponseWriter: w}
}

// WriteMsg calls the underlying ResponseWriter's WriteMsg method and prints "example" to standard output.
func (r *ResponsePrinter) WriteMsg(res *dns.Msg) error {
	log.Info("edgecdnxprefixlist")
	return r.ResponseWriter.WriteMsg(res)
}
