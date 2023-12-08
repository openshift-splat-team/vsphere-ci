package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/softlayer/softlayer-go/services"
	"github.com/softlayer/softlayer-go/session"
)

type SoftLayerConnection struct {
	Username string
	ApiToken string
}

type Subnet struct {
	Cidr               int      `json:"cidr"`
	DnsServer          string   `json:"dnsServer"`
	MachineNetworkCidr string   `json:"machineNetworkCidr"`
	Gateway            string   `json:"gateway"`
	Mask               string   `json:"mask"`
	Network            string   `json:"network"`
	IpAddresses        []string `json:"ipAddresses"`
	VirtualCenter      string   `json:"virtualcenter"`
}

type CommandLineOptions struct {
	VCenter string
	Auth    string
}

const (
	networkVlanMask = `mask[id,name,vlanNumber,fullyQualifiedName,
subnets[id,ipAddressCount,gateway,cidr,netmask,networkIdentifier,subnetType,
ipAddresses[ipAddress,isNetwork,isBroadcast,isGateway]],
primaryRouter[hostname]]`
)

func commandLineOptions() *CommandLineOptions {
	vcenterPtr := flag.String("vcenter", "", "vCenter association json")
	authPtr := flag.String("auth", "", "IBM Authentication json")

	flag.Parse()

	if *vcenterPtr == "" || *authPtr == "" {
		fmt.Println("Error: Both vcenter and auth options are required.")
		flag.PrintDefaults()
		return nil
	}
	return &CommandLineOptions{
		VCenter: *vcenterPtr,
		Auth:    *authPtr,
	}
}

func parseVCenter(vcenterPath string) (map[string]map[string][]string, error) {
	vcenters := make(map[string]map[string][]string)

	fileBytes, err := os.ReadFile(vcenterPath)
	if err != nil {
		return vcenters, err
	}

	err = json.Unmarshal(fileBytes, &vcenters)

	if err != nil {
		return vcenters, err
	}

	return vcenters, nil
}
func parseAuths(authsPath string) ([]SoftLayerConnection, error) {
	var auths []SoftLayerConnection

	fileBytes, err := os.ReadFile(authsPath)
	if err != nil {
		return auths, err
	}
	err = json.Unmarshal(fileBytes, &auths)

	if err != nil {
		return auths, err
	}

	return auths, nil
}

func main() {
	subnetVlanMap := make(map[string]map[int]Subnet)
	options := commandLineOptions()

	vcenters, err := parseVCenter(options.VCenter)
	if err != nil {
		log.Fatal(err)
	}
	auths, err := parseAuths(options.Auth)
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range auths {
		log.Println(c.Username)
		sess := session.New(c.Username, c.ApiToken)
		service := services.GetAccountService(sess)

		networkVlans, err := service.Mask(networkVlanMask).GetNetworkVlans()

		if err != nil {
			log.Fatal(err)
		}

		for i, vlan := range networkVlans {
			if vlan.Name != nil {
				if strings.Contains(*vlan.Name, "ci") {
					for _, subnet := range vlan.Subnets {

						ipAddresses := make([]string, 0, *subnet.IpAddressCount)
						for _, ip := range subnet.IpAddresses {
							ipAddresses = append(ipAddresses, *ip.IpAddress)
						}

						log.Printf("Router hostname: %s, vlan id: %d", *vlan.PrimaryRouter.Hostname, *vlan.VlanNumber)

						if _, ok := subnetVlanMap[*vlan.PrimaryRouter.Hostname]; !ok {
							subnetVlanMap[*vlan.PrimaryRouter.Hostname] = make(map[int]Subnet)
						}

						// DNS services is provided by the gateway appliance which is also the
						// gateway for this subnet.
						virtualCenter := ""

						if _, ok := vcenters[c.Username]; ok {
							if _, ok = vcenters[c.Username][*vlan.PrimaryRouter.Hostname]; ok {
								numOfVCenters := len(vcenters[c.Username][*vlan.PrimaryRouter.Hostname])
								mod := i % numOfVCenters

								virtualCenter = vcenters[c.Username][*vlan.PrimaryRouter.Hostname][mod]
							}
						}

						subnetVlanMap[*vlan.PrimaryRouter.Hostname][*vlan.VlanNumber] = Subnet{
							Cidr:               *subnet.Cidr,
							DnsServer:          *subnet.Gateway,
							MachineNetworkCidr: fmt.Sprintf("%s/%d", *subnet.NetworkIdentifier, *subnet.Cidr),
							Gateway:            *subnet.Gateway,
							Mask:               *subnet.Netmask,
							Network:            *subnet.NetworkIdentifier,
							IpAddresses:        ipAddresses,
							VirtualCenter:      virtualCenter,
						}
					}
				}
			}
		}
	}
	jsonResults, err := json.MarshalIndent(subnetVlanMap, "", "    ")

	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("subnets.json", jsonResults, 0644)
	if err != nil {
		log.Fatal(err)
	}
}
