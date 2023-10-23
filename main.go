package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func main() {
	var err error

	var zone string = os.Getenv("WGUSD_ZONE")
	var iface string = os.Getenv("WGUSD_IFACE")
	var fallbackEndpoint string = os.Getenv("WGUSD_FALLBACK")
	var verbosity int = 0

	flag.StringVarP(&zone, "zone", "z", zone, "Zone to query for SRV records")
	flag.StringVarP(&iface, "interface", "i", iface, "Wireguard interface to (re)configure")
	flag.CountVarP(&verbosity, "verbose", "v", "Verbose output (use multiple times to get debug output)")
	flag.StringVar(&fallbackEndpoint, "fallback", fallbackEndpoint, "Fallback endpoint, configured when lookup fails")

	flag.Parse()

	switch verbosity {
	case 0:
		log.SetLevel(log.WarnLevel)
	case 1:
		log.SetLevel(log.InfoLevel)
	case 2:
		log.SetLevel(log.DebugLevel)
	}

	if zone == "" {
		log.Fatal("No zone specified!")
	}

	if iface == "" {
		log.Info("No Wireguard interface specified, performing (dry-run) lookup only.")
	}

	var fallbackHost string
	var fallbackPort uint16
	if fallbackEndpoint != "" {
		fallbackHost, fallbackPort, err = splitHostPort(fallbackEndpoint)
		if err != nil {
			log.Fatalf("Cannot parse fallback endpoint %q: %v", fallbackEndpoint, err)
		}

		if fallbackHost == "" {
			log.Fatalf("Fallback host is empty (endpoint %q)", fallbackEndpoint)
		}
	}

	log.Infof("Looking up Wireguard endpoint for %q...", zone)
	hostname, port, err := lookupEndpoint(zone)
	if err != nil {
		log.Errorf("Error during SRV lookup: %v", err)

		if fallbackHost != "" {
			log.Infof("Using fallback: %s:%d", fallbackHost, fallbackPort)
			hostname = fallbackHost
			port = fallbackPort
		} else {
			log.Fatal("No fallback provided.")
		}
	} else {
		log.Infof("Retrieved Wireguard endpoint: %s:%d", hostname, port)
	}

	if iface == "" {
		fmt.Printf("%s:%d\n", hostname, port)
		log.Info("Dry run completed.")
		return
	}

	log.Infof("Reconfiguring interface %s...", iface)
	err = reconfigureInterface(iface, hostname, port)
	if err != nil {
		log.Errorf("Error reconfiguring interface %s with endpoint %s:%d : %v", iface, hostname, port, err)
	}
}

func splitHostPort(endpoint string) (string, uint16, error) {
	hostname, portstr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", 0, fmt.Errorf("split host:port from %q: %v", endpoint, err)
	}

	port64, err := strconv.ParseUint(portstr, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("parse port %q from %q: %v", portstr, endpoint, err)
	}

	port := uint16(port64)

	return hostname, port, nil
}

func lookupEndpoint(domain string) (string, uint16, error) {
	_, srvs, err := net.LookupSRV("wireguard", "udp", domain)
	if err != nil {
		return "", 0, fmt.Errorf("look up SRV record: %w", err)
	}

	if len(srvs) == 0 {
		return "", 0, fmt.Errorf("SRV lookup returned %d records", len(srvs))
	}

	var chosen *net.SRV = nil
	for _, srv := range srvs {
		log.Debugf("Found SRV record: %d %d %d %s", srv.Priority, srv.Weight, srv.Port, srv.Target)

		if chosen == nil {
			chosen = srv
		} else if srv.Priority < chosen.Priority {
			chosen = srv
		} else if srv.Priority == chosen.Priority && srv.Weight > chosen.Weight {
			chosen = srv
		}
	}

	log.Debugf("Preferred SRV record: %d %d %d %s", chosen.Priority, chosen.Weight, chosen.Port, chosen.Target)

	hostname := strings.TrimSuffix(chosen.Target, ".")
	port := chosen.Port

	return hostname, port, nil
}

func reconfigureInterface(iface string, hostname string, port uint16) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("create Wireguard client: %w", err)
	}
	defer client.Close()

	device, err := client.Device(iface)
	if err != nil {
		return fmt.Errorf("get Wireguard device %s: %v", iface, err)
	}

	if len(device.Peers) != 1 {
		return fmt.Errorf("cannot reconfigure device %s with %d configured endpoints", iface, len(device.Peers))
	}

	peer := device.Peers[0]
	log.Debugf("Reconfiguring peer with pubkey %s...", base64.StdEncoding.EncodeToString(peer.PublicKey[:]))

	endpoint := fmt.Sprintf("%s:%d", hostname, port)
	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return fmt.Errorf("resolve UDP address %s: %w", endpoint, err)
	}
	log.Debugf("Resolved UDP address: %s", addr)

	if addr.IP.Equal(peer.Endpoint.IP) && addr.Port == peer.Endpoint.Port {
		log.Debug("Nothing to do!")

		return nil
	}

	peerConfig := wgtypes.PeerConfig{
		PublicKey:  peer.PublicKey,
		UpdateOnly: true,
		Endpoint:   addr,
	}

	err = client.ConfigureDevice(device.Name, wgtypes.Config{
		ReplacePeers: false,
		Peers:        []wgtypes.PeerConfig{peerConfig},
	})
	if err != nil {
		return fmt.Errorf("reconfigure peer: %w", err)
	}

	log.Debug("Peer succesfully reconfigured.")

	return nil
}
