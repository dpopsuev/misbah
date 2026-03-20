package daemon

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// NetworkIsolator creates per-container network namespaces with iptables rules
// that only allow traffic to the proxy. All other egress is dropped.
type NetworkIsolator struct {
	cfg    config.NetworkSection
	logger *metrics.Logger
	mu     sync.Mutex
	nextIP uint32 // last octet counter for container IPs
	netns  map[string]*isolatedNet
}

type isolatedNet struct {
	nsName   string
	vethHost string
	vethCont string
	ip       net.IP
}

// NewNetworkIsolator creates a new network isolator.
func NewNetworkIsolator(cfg config.NetworkSection, logger *metrics.Logger) *NetworkIsolator {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &NetworkIsolator{
		cfg:    cfg,
		logger: logger,
		nextIP: 1, // .1 is gateway, containers start at .2
		netns:  make(map[string]*isolatedNet),
	}
}

// Setup creates a network namespace for a container with iptables rules
// that only allow traffic to the proxy at proxyAddr.
// Returns the namespace name for use with `ip netns exec`.
func (ni *NetworkIsolator) Setup(containerName, proxyAddr string) (string, error) {
	ni.mu.Lock()
	defer ni.mu.Unlock()

	if _, exists := ni.netns[containerName]; exists {
		return "", fmt.Errorf("network namespace already exists for %s", containerName)
	}

	_, subnet, err := net.ParseCIDR(ni.cfg.Subnet)
	if err != nil {
		return "", fmt.Errorf("invalid subnet %s: %w", ni.cfg.Subnet, err)
	}

	// Allocate container IP
	ipNum := atomic.AddUint32(&ni.nextIP, 1)
	containerIP := make(net.IP, len(subnet.IP))
	copy(containerIP, subnet.IP)
	containerIP[3] = byte(ipNum)

	// Gateway is .1
	gatewayIP := make(net.IP, len(subnet.IP))
	copy(gatewayIP, subnet.IP)
	gatewayIP[3] = 1

	nsName := "misbah-" + containerName
	vethHost := "vh-" + containerName
	vethCont := "vc-" + containerName

	// Truncate veth names to 15 chars (kernel limit)
	if len(vethHost) > 15 {
		vethHost = vethHost[:15]
	}
	if len(vethCont) > 15 {
		vethCont = vethCont[:15]
	}

	// 1. Create named network namespace
	ns, err := netns.NewNamed(nsName)
	if err != nil {
		return "", fmt.Errorf("failed to create netns %s: %w", nsName, err)
	}
	ns.Close()

	// 2. Create veth pair
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: vethHost,
			MTU:  ni.cfg.MTU,
		},
		PeerName: vethCont,
	}
	if err := netlink.LinkAdd(veth); err != nil {
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to create veth pair: %w", err)
	}

	// 3. Get the netns handle for moving the peer
	nsHandle, err := netns.GetFromName(nsName)
	if err != nil {
		netlink.LinkDel(veth)
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to get netns handle: %w", err)
	}
	defer nsHandle.Close()

	// 4. Move container end into the namespace
	contLink, err := netlink.LinkByName(vethCont)
	if err != nil {
		netlink.LinkDel(veth)
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to find veth peer: %w", err)
	}
	if err := netlink.LinkSetNsFd(contLink, int(nsHandle)); err != nil {
		netlink.LinkDel(veth)
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to move veth to netns: %w", err)
	}

	// 5. Ensure bridge exists and attach host veth
	if err := ni.ensureBridge(gatewayIP, subnet); err != nil {
		netlink.LinkDel(veth)
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to setup bridge: %w", err)
	}

	hostLink, err := netlink.LinkByName(vethHost)
	if err != nil {
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to find host veth: %w", err)
	}

	bridge, err := netlink.LinkByName(ni.cfg.Bridge)
	if err != nil {
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to find bridge: %w", err)
	}
	if err := netlink.LinkSetMaster(hostLink, bridge.(*netlink.Bridge)); err != nil {
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to attach veth to bridge: %w", err)
	}
	if err := netlink.LinkSetUp(hostLink); err != nil {
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to bring up host veth: %w", err)
	}

	// 6. Configure the container namespace (IP, routes, iptables)
	ones, _ := subnet.Mask.Size()
	if err := ni.configureContainerNs(nsName, vethCont, containerIP, gatewayIP, ones, proxyAddr); err != nil {
		ni.cleanupNs(nsName)
		return "", fmt.Errorf("failed to configure container netns: %w", err)
	}

	ni.netns[containerName] = &isolatedNet{
		nsName:   nsName,
		vethHost: vethHost,
		vethCont: vethCont,
		ip:       containerIP,
	}

	ni.logger.Infof("Network namespace %s created (IP %s, proxy %s)", nsName, containerIP, proxyAddr)
	return nsName, nil
}

// Teardown removes the network namespace for a container.
func (ni *NetworkIsolator) Teardown(containerName string) error {
	ni.mu.Lock()
	iso, ok := ni.netns[containerName]
	if !ok {
		ni.mu.Unlock()
		return fmt.Errorf("no network namespace for %s", containerName)
	}
	delete(ni.netns, containerName)
	ni.mu.Unlock()

	// Delete host veth (automatically removes peer)
	if link, err := netlink.LinkByName(iso.vethHost); err == nil {
		netlink.LinkDel(link)
	}

	ni.cleanupNs(iso.nsName)
	ni.logger.Infof("Network namespace %s removed", iso.nsName)
	return nil
}

// TeardownAll removes all network namespaces.
func (ni *NetworkIsolator) TeardownAll() {
	ni.mu.Lock()
	snapshot := make(map[string]*isolatedNet, len(ni.netns))
	for k, v := range ni.netns {
		snapshot[k] = v
	}
	ni.netns = make(map[string]*isolatedNet)
	ni.mu.Unlock()

	for _, iso := range snapshot {
		if link, err := netlink.LinkByName(iso.vethHost); err == nil {
			netlink.LinkDel(link)
		}
		ni.cleanupNs(iso.nsName)
	}
}

// NsName returns the namespace name for a container, or empty string if not found.
func (ni *NetworkIsolator) NsName(containerName string) string {
	ni.mu.Lock()
	defer ni.mu.Unlock()
	if iso, ok := ni.netns[containerName]; ok {
		return iso.nsName
	}
	return ""
}

func (ni *NetworkIsolator) ensureBridge(gatewayIP net.IP, subnet *net.IPNet) error {
	br, err := netlink.LinkByName(ni.cfg.Bridge)
	if err != nil {
		// Create bridge
		bridge := &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Name: ni.cfg.Bridge,
				MTU:  ni.cfg.MTU,
			},
		}
		if err := netlink.LinkAdd(bridge); err != nil {
			return fmt.Errorf("failed to create bridge: %w", err)
		}
		br = bridge
	}

	// Assign gateway IP if not already assigned
	addrs, _ := netlink.AddrList(br, netlink.FAMILY_V4)
	hasAddr := false
	for _, a := range addrs {
		if a.IP.Equal(gatewayIP) {
			hasAddr = true
			break
		}
	}
	if !hasAddr {
		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   gatewayIP,
				Mask: subnet.Mask,
			},
		}
		if err := netlink.AddrAdd(br, addr); err != nil {
			return fmt.Errorf("failed to add gateway IP to bridge: %w", err)
		}
	}

	if err := netlink.LinkSetUp(br); err != nil {
		return fmt.Errorf("failed to bring up bridge: %w", err)
	}

	return nil
}

// configureContainerNs runs inside the container's network namespace
// to set up the interface, routes, and iptables rules.
func (ni *NetworkIsolator) configureContainerNs(nsName, vethName string, containerIP, gatewayIP net.IP, prefixLen int, proxyAddr string) error {
	// Lock OS thread since we're switching network namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save current namespace
	origNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get current netns: %w", err)
	}
	defer origNs.Close()

	// Switch to container namespace
	targetNs, err := netns.GetFromName(nsName)
	if err != nil {
		return fmt.Errorf("failed to get target netns: %w", err)
	}
	defer targetNs.Close()

	if err := netns.Set(targetNs); err != nil {
		return fmt.Errorf("failed to switch to netns: %w", err)
	}
	defer netns.Set(origNs)

	// Bring up loopback
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("failed to find loopback: %w", err)
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		return fmt.Errorf("failed to bring up loopback: %w", err)
	}

	// Configure container veth
	link, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("failed to find container veth: %w", err)
	}

	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   containerIP,
			Mask: net.CIDRMask(prefixLen, 32),
		},
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add IP to container veth: %w", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up container veth: %w", err)
	}

	// Add default route via gateway
	route := &netlink.Route{
		Gw: gatewayIP,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("failed to add default route: %w", err)
	}

	// Parse proxy address for iptables
	proxyHost, proxyPort, err := net.SplitHostPort(proxyAddr)
	if err != nil {
		return fmt.Errorf("invalid proxy address %s: %w", proxyAddr, err)
	}

	// iptables: ALLOW loopback, proxy, established; DROP all else
	iptablesRules := [][]string{
		// Allow all loopback traffic
		{"-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"},
		// Allow traffic to the proxy
		{"-A", "OUTPUT", "-d", proxyHost, "-p", "tcp", "--dport", proxyPort, "-j", "ACCEPT"},
		// Allow established/related connections (responses)
		{"-A", "OUTPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		// Allow DNS to gateway (for name resolution through proxy)
		{"-A", "OUTPUT", "-d", gatewayIP.String(), "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		{"-A", "OUTPUT", "-d", gatewayIP.String(), "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		// DROP everything else
		{"-A", "OUTPUT", "-j", "DROP"},
	}

	for _, rule := range iptablesRules {
		cmd := exec.Command("iptables", rule...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("iptables %v failed: %s: %w", rule, string(out), err)
		}
	}

	return nil
}

func (ni *NetworkIsolator) cleanupNs(nsName string) {
	netns.DeleteNamed(nsName)
}
