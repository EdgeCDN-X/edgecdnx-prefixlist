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

	"github.com/miekg/dns"
)

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin("edgecdnxprefixlist")

// Example is an example plugin to show how to write a plugin.
type EdgeCDNXPrefixList struct {
	Next plugin.Handler
}

type EdgeCDNXPrefixListResponseWriter struct {
}

// ServeDNS implements the plugin.Handler interface. This method gets called when example is used
// in a Server.
func (e EdgeCDNXPrefixList) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// This function could be simpler. I.e. just fmt.Println("example") here, but we want to show
	// a slightly more complex example as to make this more interesting.
	// Here we wrap the dns.ResponseWriter in a new ResponseWriter and call the next plugin, when the
	// answer comes back, it will print "example".

	// Debug log that we've have seen the query. This will only be shown when the debug plugin is loaded.

	state := request.Request{W: w, Req: r}

	qname := state.Name()
	qtype := state.Type()

	log.Debug(fmt.Sprintf("edgecdnxprefixlist: %s %s", qname, qtype))

	res := new(dns.A)
	res.Hdr = dns.RR_Header{Name: "google.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 180}
	res.A = net.IPv4(192, 168, 0, 100) // Example IP address

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = append(m.Answer, res)
	state.SizeAndDo(m)
	m = state.Scrub(m)

	log.Debug(fmt.Sprintf("edgecdnxprefixlist: %v", m))

	err := w.WriteMsg(m)
	if err != nil {
		log.Error(fmt.Sprintf("edgecdnxprefixlist: %v", err))
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil

	// Export metric with the server label set to the current server handling the request.
	// requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	// Call next plugin (if any).
	// return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
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
