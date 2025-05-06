// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"fmt"

	"go.githedgehog.com/gateway-proto/pkg/dataplane"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
)

func buildDataplaneConfig(ag *gwintapi.GatewayAgent) (*dataplane.GatewayConfig, error) {
	cfg := &dataplane.GatewayConfig{
		Generation: ag.Generation,
		Device: &dataplane.Device{
			Driver:   dataplane.PacketDriver_KERNEL,
			Hostname: ag.Name,
			Loglevel: dataplane.LogLevel_DEBUG,
		},
		Underlay: &dataplane.Underlay{ // TODO replace with actual generated config
			Vrfs: []*dataplane.VRF{
				{
					Name: "default",
					Interfaces: []*dataplane.Interface{
						{
							Name:   "eth0",
							Ipaddr: "10.0.0.1/32",
							Type:   dataplane.IfType_IF_TYPE_ETHERNET,
							Role:   dataplane.IfRole_IF_ROLE_FABRIC,
						},
						{
							Name:   "eth1",
							Ipaddr: "10.0.0.1/32",
							Type:   dataplane.IfType_IF_TYPE_ETHERNET,
							Role:   dataplane.IfRole_IF_ROLE_EXTERNAL,
						},
					},
				},
			},
		},
		Overlay: &dataplane.Overlay{},
	}

	vpcSubnets := map[string]map[string]string{}

	for vpcName, vpc := range ag.Spec.VPCs {
		cfg.Overlay.Vpcs = append(cfg.Overlay.Vpcs, &dataplane.VPC{
			Name: vpcName,
			Id:   vpc.InternalID,
			Vni:  vpc.VNI,
		})

		vpcSubnets[vpcName] = map[string]string{}
		for subnetName, subnet := range vpc.Subnets {
			vpcSubnets[vpcName][subnetName] = subnet.CIDR
		}
	}

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

		cfg.Overlay.Peerings = append(cfg.Overlay.Peerings, p)
	}

	return cfg, nil
}
