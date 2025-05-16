// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"fmt"
	"net/netip"

	"go.githedgehog.com/gateway-proto/pkg/dataplane"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
)

func buildDataplaneConfig(ag *gwintapi.GatewayAgent) (*dataplane.GatewayConfig, error) {
	protoIP, err := netip.ParsePrefix(ag.Spec.Gateway.ProtocolIP)
	if err != nil {
		return nil, fmt.Errorf("invalid ProtocolIP %s: %w", ag.Spec.Gateway.ProtocolIP, err)
	}

	ifaces := []*dataplane.Interface{
		{
			Name:   "lo",
			Ipaddr: ag.Spec.Gateway.ProtocolIP,
			Type:   dataplane.IfType_IF_TYPE_LOOPBACK,
			Role:   dataplane.IfRole_IF_ROLE_FABRIC,
		},
		{
			Name:   "vtep",
			Ipaddr: ag.Spec.Gateway.VTEPIP,
			Type:   dataplane.IfType_IF_TYPE_VTEP,
			Role:   dataplane.IfRole_IF_ROLE_FABRIC,
		},
	}
	for name, iface := range ag.Spec.Gateway.Interfaces {
		ifaces = append(ifaces, &dataplane.Interface{
			Name:   name,
			Ipaddr: iface.IP,
			Type:   dataplane.IfType_IF_TYPE_ETHERNET,
			Role:   dataplane.IfRole_IF_ROLE_FABRIC,
		})
	}

	neighs := []*dataplane.BgpNeighbor{}
	for _, neigh := range ag.Spec.Gateway.Neighbors {
		neighIP, err := netip.ParseAddr(neigh.IP)
		if err != nil {
			return nil, fmt.Errorf("invalid neighbor IP %s: %w", neigh.IP, err)
		}
		neighs = append(neighs, &dataplane.BgpNeighbor{
			Address:   neighIP.String(),
			RemoteAsn: fmt.Sprintf("%d", neigh.ASN),
			AfActivate: []dataplane.BgpAF{
				dataplane.BgpAF_IPV4_UNICAST,
				dataplane.BgpAF_L2VPN_EVPN,
			},
		})
	}

	vpcSubnets := map[string]map[string]string{}
	vpcs := []*dataplane.VPC{}
	for vpcName, vpc := range ag.Spec.VPCs {
		vpcs = append(vpcs, &dataplane.VPC{
			Name: vpcName,
			Id:   vpc.InternalID,
			Vni:  vpc.VNI,
		})

		vpcSubnets[vpcName] = map[string]string{}
		for subnetName, subnet := range vpc.Subnets {
			vpcSubnets[vpcName][subnetName] = subnet.CIDR
		}
	}

	peerings := []*dataplane.VpcPeering{}
	for peeringName, peering := range ag.Spec.Peerings {
		p := &dataplane.VpcPeering{
			Name: peeringName,
			For:  []*dataplane.PeeringEntryFor{},
		}

		for vpcName, vpc := range peering.Peering {
			exposes := []*dataplane.Expose{}

			for _, expose := range vpc.Expose {
				ips := []*dataplane.PeeringIPs{}
				as := []*dataplane.PeeringAs{}

				for _, ipEntry := range expose.IPs {
					// TODO validate
					switch {
					case ipEntry.CIDR != "":
						ips = append(ips, &dataplane.PeeringIPs{
							Rule: &dataplane.PeeringIPs_Cidr{Cidr: ipEntry.CIDR},
						})
					case ipEntry.Not != "":
						ips = append(ips, &dataplane.PeeringIPs{
							Rule: &dataplane.PeeringIPs_Not{Not: ipEntry.Not},
						})
					case ipEntry.VPCSubnet != "":
						if subnetCIDR, ok := vpcSubnets[vpcName][ipEntry.VPCSubnet]; ok {
							ips = append(ips, &dataplane.PeeringIPs{
								Rule: &dataplane.PeeringIPs_Cidr{Cidr: subnetCIDR},
							})
						} else {
							return nil, fmt.Errorf("unknown VPC subnet %s in peering %s / vpc %s", ipEntry.VPCSubnet, peeringName, vpcName) //nolint:goerr113
						}
					default:
						return nil, fmt.Errorf("invalid IP entry in peering %s / vpc %s: %v", peeringName, vpcName, ipEntry) //nolint:goerr113
					}
				}

				for _, asEntry := range expose.As {
					// TODO validate
					switch {
					case asEntry.CIDR != "":
						as = append(as, &dataplane.PeeringAs{
							Rule: &dataplane.PeeringAs_Cidr{Cidr: asEntry.CIDR},
						})
					case asEntry.Not != "":
						as = append(as, &dataplane.PeeringAs{
							Rule: &dataplane.PeeringAs_Not{Not: asEntry.Not},
						})
					default:
						return nil, fmt.Errorf("invalid IP entry in peering %s / vpc %s: %v", peeringName, vpcName, asEntry) //nolint:goerr113
					}
				}

				exposes = append(exposes, &dataplane.Expose{
					Ips: ips,
					As:  as,
				})
			}

			p.For = append(p.For, &dataplane.PeeringEntryFor{
				Vpc:    vpcName,
				Expose: exposes,
			})
		}

		peerings = append(peerings, p)
	}

	return &dataplane.GatewayConfig{
		Generation: ag.Generation,
		Device: &dataplane.Device{
			Driver:   dataplane.PacketDriver_KERNEL,
			Hostname: ag.Name,
			Loglevel: dataplane.LogLevel_DEBUG,
		},
		Underlay: &dataplane.Underlay{
			Vrfs: []*dataplane.VRF{
				{
					Name:       "default",
					Interfaces: ifaces,
					Router: &dataplane.RouterConfig{
						Asn:       fmt.Sprintf("%d", ag.Spec.Gateway.ASN),
						RouterId:  protoIP.Addr().String(),
						Neighbors: neighs,
						Ipv4Unicast: &dataplane.BgpAddressFamilyIPv4{
							RedistributeConnected: false,
							RedistributeStatic:    false,
						},
						L2VpnEvpn: &dataplane.BgpAddressFamilyL2VpnEvpn{
							AdvertiseAllVni: true,
						},
					},
				},
			},
		},
		Overlay: &dataplane.Overlay{
			Vpcs:     vpcs,
			Peerings: peerings,
		},
	}, nil
}
