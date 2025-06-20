package edgecdnxprefixlist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ancientlore/go-avltree"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	infrastructurev1alpha1 "github.com/EdgeCDN-X/edgecdnx-controller/api/v1alpha1"
)

// init registers this plugin.
func init() { plugin.Register("edgecdnxprefixlist", setup) }

type EdgeCDNXPrefixListRouting struct {
	Namespace string
	RoutingV4 *avltree.Tree
	RoutingV6 *avltree.Tree
}

type PrefixTreeEntry struct {
	Location string
	Prefix   net.IPNet
}

// setup is the function that gets called when the config parser see the token "example". Setup is responsible
// for parsing any extra options the example plugin may have. The first token this function sees is "example".
func setup(c *caddy.Controller) error {
	scheme := runtime.NewScheme()
	clientsetscheme.AddToScheme(scheme)
	infrastructurev1alpha1.AddToScheme(scheme)

	kubeconfig := ctrl.GetConfigOrDie()
	c.Next()

	args := c.RemainingArgs()
	if len(args) != 1 {
		return plugin.Error("edgecdnxprefixlist", c.ArgErr())
	}

	routing := &EdgeCDNXPrefixListRouting{
		Namespace: args[0],
		RoutingV4: avltree.New(func(a interface{}, b interface{}) int {
			starta := a.(PrefixTreeEntry).Prefix.IP.To4()
			enda := make(net.IP, len(starta))
			copy(enda, starta)

			for i := 0; i < len(a.(PrefixTreeEntry).Prefix.Mask); i++ {
				enda[i] |= ^a.(PrefixTreeEntry).Prefix.Mask[i]
			}

			startb := b.(PrefixTreeEntry).Prefix.IP.To4()
			endb := make(net.IP, len(startb))
			copy(endb, startb)

			for i := 0; i < len(b.(PrefixTreeEntry).Prefix.Mask); i++ {
				endb[i] |= ^b.(PrefixTreeEntry).Prefix.Mask[i]
			}

			if bytes.Compare(enda, startb) == -1 {
				return -1
			}

			if bytes.Compare(starta, endb) == 1 {
				return 1
			}

			return 0
		}, 0),
		RoutingV6: avltree.New(func(a interface{}, b interface{}) int {
			starta := a.(PrefixTreeEntry).Prefix.IP.To16()
			enda := make(net.IP, len(starta))
			copy(enda, starta)
			for i := 0; i < len(a.(PrefixTreeEntry).Prefix.Mask); i++ {
				enda[i] |= ^a.(PrefixTreeEntry).Prefix.Mask[i]
			}

			startb := b.(PrefixTreeEntry).Prefix.IP.To16()
			endb := make(net.IP, len(startb))
			copy(endb, startb)
			for i := 0; i < len(b.(PrefixTreeEntry).Prefix.Mask); i++ {
				endb[i] |= ^b.(PrefixTreeEntry).Prefix.Mask[i]
			}

			if bytes.Compare(enda, startb) == -1 {
				return -1
			}

			if bytes.Compare(starta, endb) == 1 {
				return 1
			}

			return 0
		}, 0),
	}
	clientSet, err := dynamic.NewForConfig(kubeconfig)
	if err != nil {
		return plugin.Error("edgecdnxservices", fmt.Errorf("failed to create dynamic client: %w", err))
	}
	fac := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clientSet, 10*time.Minute, args[0], nil)

	informer := fac.ForResource(schema.GroupVersionResource{
		Group:    infrastructurev1alpha1.GroupVersion.Group,
		Version:  infrastructurev1alpha1.GroupVersion.Version,
		Resource: "prefixlists",
	}).Informer()

	sem := &sync.RWMutex{}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			p_raw, ok := obj.(*unstructured.Unstructured)
			if !ok {
				log.Errorf("edgecdnxprefixlist: expected PrefixList object, got %T", p_raw)
				return
			}
			temp, err := json.Marshal(p_raw.Object)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to marshal PrefixList object: %v", err)
				return
			}
			prefixList := &infrastructurev1alpha1.PrefixList{}
			err = json.Unmarshal(temp, prefixList)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to unmarshal PrefixList object: %v", err)
				return
			}
			sem.Lock()
			defer sem.Unlock()
			for _, v := range prefixList.Spec.Prefix.V4 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Adding V4 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV4.Add(PrefixTreeEntry{
					Location: prefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			for _, v := range prefixList.Spec.Prefix.V6 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Adding V6 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV6.Add(PrefixTreeEntry{
					Location: prefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			log.Infof("edgecdnxprefixlist: Added PrefixList %s", prefixList.Name)
		},
		UpdateFunc: func(oldObj, newObj any) {
			p_old_raw, ok := oldObj.(*unstructured.Unstructured)
			if !ok {
				log.Errorf("edgecdnxprefixlist: expected PrefixList object, got %T", p_old_raw)
				return
			}
			temp, err := json.Marshal(p_old_raw.Object)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to marshal PrefixList object: %v", err)
				return
			}
			oldPrefixList := &infrastructurev1alpha1.PrefixList{}
			err = json.Unmarshal(temp, oldPrefixList)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to unmarshal PrefixList object: %v", err)
				return
			}

			p_new_raw, ok := newObj.(*unstructured.Unstructured)
			if !ok {
				log.Errorf("edgecdnxprefixlist: expected PrefixList object, got %T", p_new_raw)
				return
			}
			temp, err = json.Marshal(p_new_raw.Object)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to marshal PrefixList object: %v", err)
				return
			}
			newPrefixList := &infrastructurev1alpha1.PrefixList{}
			err = json.Unmarshal(temp, newPrefixList)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to unmarshal PrefixList object: %v", err)
				return
			}

			sem.Lock()
			defer sem.Unlock()
			for _, v := range oldPrefixList.Spec.Prefix.V4 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Removing V4 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV4.Remove(PrefixTreeEntry{
					Location: oldPrefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			for _, v := range oldPrefixList.Spec.Prefix.V6 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Removing V6 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV6.Remove(PrefixTreeEntry{
					Location: oldPrefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			for _, v := range newPrefixList.Spec.Prefix.V4 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Adding V4 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV4.Add(PrefixTreeEntry{
					Location: newPrefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			for _, v := range newPrefixList.Spec.Prefix.V6 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Adding V6 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV6.Add(PrefixTreeEntry{
					Location: newPrefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			log.Infof("edgecdnxprefixlist: Updated PrefixList %s", newPrefixList.Name)
		},
		DeleteFunc: func(obj any) {
			p_raw, ok := obj.(*unstructured.Unstructured)
			if !ok {
				log.Errorf("edgecdnxprefixlist: expected PrefixList object, got %T", p_raw)
				return
			}
			temp, err := json.Marshal(p_raw.Object)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to marshal PrefixList object: %v", err)
				return
			}
			prefixList := &infrastructurev1alpha1.PrefixList{}
			err = json.Unmarshal(temp, prefixList)
			if err != nil {
				log.Errorf("edgecdnxprefixlist: failed to unmarshal PrefixList object: %v", err)
				return
			}

			sem.Lock()
			defer sem.Unlock()
			for _, v := range prefixList.Spec.Prefix.V4 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Removing V4 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV4.Remove(PrefixTreeEntry{
					Location: prefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			for _, v := range prefixList.Spec.Prefix.V6 {
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", v.Address, v.Size))
				if err != nil {
					log.Error(fmt.Sprintf("parse cidr error %v", err))
					return
				}
				log.Debug(fmt.Sprintf("Removing V6 CIDR %s/%d\n", v.Address, v.Size))
				routing.RoutingV6.Remove(PrefixTreeEntry{
					Location: prefixList.Spec.Destination,
					Prefix:   *ipnet,
				})
			}
			log.Infof("edgecdnxprefixlist: Deleted PrefixList %s", prefixList.Name)
		},
	})

	factoryCloseChan := make(chan struct{})
	fac.Start(factoryCloseChan)

	c.OnShutdown(func() error {
		log.Infof("edgecdnxprefixlist: Shutting down informer")
		close(factoryCloseChan)
		fac.Shutdown()
		return nil
	})

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return EdgeCDNXPrefixList{Next: next, Routing: routing, Sync: &sync.RWMutex{}, InformerSynced: informer.HasSynced}
	})

	// All OK, return a nil error.
	return nil
}
