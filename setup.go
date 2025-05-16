package edgecdnxprefixlist

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/ancientlore/go-avltree"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"gopkg.in/yaml.v3"
)

// init registers this plugin.
func init() { plugin.Register("edgecdnxprefixlist", setup) }

type EdgeCDNXPrefixListRouting struct {
	FilePath  string
	RoutingV4 *avltree.Tree
	RoutingV6 *avltree.Tree
}

type Prefix struct {
	V4 []AddressPrefix `yaml:"v4"`
	V6 []AddressPrefix `yaml:"v6"`
}

type AddressPrefix struct {
	Address string `yaml:"address"`
	Size    int    `yaml:"size"`
}

type RoutingInner struct {
	Location string `yaml:"location"`
	Prefix   Prefix `yaml:"prefix"`
}

type Routing struct {
	Routing RoutingInner `yaml:"routing"`
}

func RoutingV4Comparator(a, b net.IPNet) bool {
	// Compare the IP networks based on their CIDR notation
	return bytes.Compare(a.IP, b.IP) < 0
}

// setup is the function that gets called when the config parser see the token "example". Setup is responsible
// for parsing any extra options the example plugin may have. The first token this function sees is "example".
func setup(c *caddy.Controller) error {
	c.Next() // Ignore "example" and give us the next token.

	args := c.RemainingArgs()
	if len(args) != 1 {
		return plugin.Error("edgecdnxprefixlist", c.ArgErr())
	}

	routing := &EdgeCDNXPrefixListRouting{
		FilePath: args[0],
		RoutingV4: avltree.New(func(a interface{}, b interface{}) int {
			starta := a.(net.IPNet).IP.To4()
			enda := a.(net.IPNet).IP.To4()
			for i := 0; i < len(a.(net.IPNet).Mask); i++ {
				enda[i] |= ^a.(net.IPNet).Mask[i]
			}

			startb := b.(net.IPNet).IP.To4()
			endb := b.(net.IPNet).IP.To4()
			for i := 0; i < len(b.(net.IPNet).Mask); i++ {
				endb[i] |= ^b.(net.IPNet).Mask[i]
			}

			if bytes.Compare(enda, startb) == -1 {
				return -1
			}

			if bytes.Compare(starta, endb) == 1 {
				return 1
			}

			if bytes.Compare(starta, startb) == -1 && bytes.Compare(enda, startb) < 1 {
				return 0
			}

			log.Debug(fmt.Sprintf("this case should not happend, %v, %v, %v, %v", starta, enda, startb, endb))
			return 0
		}, 0),
		RoutingV6: avltree.New(func(a interface{}, b interface{}) int {
			starta := a.(net.IPNet).IP.To16()
			enda := a.(net.IPNet).IP.To16()
			for i := 0; i < len(a.(net.IPNet).Mask); i++ {
				enda[i] |= ^a.(net.IPNet).Mask[i]
			}

			startb := b.(net.IPNet).IP.To16()
			endb := b.(net.IPNet).IP.To16()
			for i := 0; i < len(b.(net.IPNet).Mask); i++ {
				endb[i] |= ^b.(net.IPNet).Mask[i]
			}

			if bytes.Compare(enda, startb) == -1 {
				return -1
			}

			if bytes.Compare(starta, endb) == 1 {
				return 1
			}

			if bytes.Compare(starta, startb) == -1 && bytes.Compare(enda, startb) < 1 {
				return 0
			}

			log.Debug(fmt.Sprintf("this case should not happend, %v, %v, %v, %v", starta, enda, startb, endb))
			return 0
		}, 0),
	}

	files, err := filepath.Glob(filepath.Join(routing.FilePath, "*.yaml"))
	if err != nil {
		return plugin.Error("edgecdnxprefixlist", err)
	}

	// Process each YAML file (e.g., validate or load into memory)
	for _, file := range files {
		// Example: Log the file name or perform further processing
		log.Debug(fmt.Printf("Found YAML file: %s\n", file))

		content, err := os.ReadFile(file)
		if err != nil {
			return plugin.Error("edgecdnxprefixlist", fmt.Errorf("failed to read file %s: %w", file, err))
		}

		var data Routing
		if err := yaml.Unmarshal(content, &data); err != nil {
			log.Debug(fmt.Sprintf("unmarshal error %v", err))
			return plugin.Error("edgecdnxprefixlist", fmt.Errorf("failed to parse YAML file %s: %w", file, err))
		}

		// TODO load prefixes to RBTree. Before that we have to check if prefixes are overlapping.

	}
	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return EdgeCDNXPrefixList{Next: next, Routing: routing}
	})

	// All OK, return a nil error.
	return nil
}
