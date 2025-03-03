# API Proposal (manish)

This API propsoal is for the basis of connecting VPCs and externals in fabric with more advanced interconnection patters.  The API is primarily concerned with configuring routing and basic filtering between VPCs, but is designed so that it can later accomodate more advanced concepts like load balancing, advanced firewall, etc.

The API consists of two main objects, and Expose object which allows a VPC to expose IP addresses for use by other VPCs, and a Route object which allows a VPC to connect to addresses exposed via Expose.  As discussed below, NAT is implied when the exposed addresses via Expose or Route are different than the VPC addresses concerned internally.

For the purpose of the discussion below, consider the following VPC configurations:

- vpc1, has all addresses 10.1.0.0/16
- vpc2, has all addresses 10.2.0.0/16
- vpc3, has all addresses 10.3.0.0/16

- external1 is connected to the internet and peered via BGP to provider A
- external2 is connected to the internet and peered via BGP to provider B

## Expose

To expose a set of IP addresses for use by other VPCs and externals, an Expose object is used.  The gits of an expose object is as follows:
```
kind: gateway/expose
metadata:
  name: expose-10-1-1-0--24
spec:
  # When traffic is received at the exposed IPs we need to be able
  # to return traffic back.  This creates an implied route.
  # The default choice here should be "stateful" where we use a
  # flow table like a stateful firewall to return the exact traffic
  # to the host that generated the original flow, but this is effectively
  # flow-based routing which may not scale, so we should allow other options.
  # 
  # Valid values are: static, stateful
  #
  return-route-type: static
  expose:
    ip: 10.1.1.0/24 # These ips must belong to the VPC
    as: 192.168.1.0/24 # This implies static DNAT from these IPs to ip
    map-ports: # Support for PAT on dest
      - external: 80
        internal: 8080
  # Additional filter that will only allow connections from vpc2 and vpc3
  # but not other vpcs or externals.  We probably need a way to wildcard
  # vpcs or use a regexp or something to make it easier to specify sources.
  #
  # This does *not* imply any route though, this is just a filter as the
  # name suggests.  It will drop packets with other origins.
  filter:
    allow:
      - from: { vpc: vpc2 }
        to: { port: 80 }
      - from-vpc: { vpc: vpc2 }
        to: { port: 80 }
```

Note that an Expose object does not by itself create an routes in the gateway 
to allow traffic from any source to any destination, it just tells the gateway
that traffic from outside the VPC to these addresses should be allowed.

## Route

`Route` objects do roughly what the name implies, they create routes in the
gateway to allow traffic to flow from a source VPC to a set of IP addresses 
made visible by an `Expose` object.  

Because they are routes, the lookup is based only on destination IP in the VRF for the source VPC.  
```
# This route has no effect unless there is an Expose object with
# these IP addresses.
#
kind: gateway/route
metadata:
  name: web-traffic-vpc2-to-vpc1
spec:
  # Note, longer term we want this whole tuple to specifiy a VRF
  # for now, we'll live with just the vpc
  src: 
    vpc: vpc2 
    ip: 10.2.1.0/24
    as: 192.168.2.0/24
  

  # This set of IP addresses should match that in the expose object
  dest: { ip: 192.168.1.0/24 }
  # We could also allow something like to avoid duplicated IP names
  # dest:
    ip: { from: expose-10-1-1-0--24 }

  filter:
    - from: { vpc: vpc2 }
      to: { port: 80 } #Not required, but to demonstrate source filtering
```

Note that the `Route` object never specifies a destination vpc.  This is 
because the gateway itself never has dest VPC information that it cannot compute
from just the source VPC, source IP, source port, dest IP, and dest port.  
The dest VPC is just not part of the packet and so we should not have an API
that implies to the user that we can use it.

If there are multiple overlapping destinations, longest prefix matching will be used to choose the appropriate destination route.  This has implications for return routing
in the static case that can lead to error configurations.  This is discussed below.

## Simple Restrictions

* A vpc may not name any of it's internal IP addresses in more than one `Expose` object
* A vpc may not map the same external addresses in multiple `Expose` objects
* A vpc may not have 2 `Route` objects with exactly the same destination and source
* 2 vpcs may not have a Route using the same external ip addresses that name 
  IP addresses in the same destination block.
  * This is because there is no correct way to construct the return routes in these cases.
  * A single VPC may have multiple routes to overlapping destination IPs with different
    filter policies, or source IPs
* 2 `Expose` objects may not expose the same IP addresses, as the gateway has no way
  to distinguish which IP address was intended as the destination in outbound routes

In the future, we can relax some of the IP addresses in multiple VPC restrictions by having some type of grouping or namespacing of IP blocks.  Then the uniqueness requirements apply only within the group.  Because these VRFs are completely separate the gateway would be able to distinguish which VPC in 2 different groups is the correct VPC and tag it appopropriately to avoid conflicts. Return routes would work the same way.

## Peering objects

To peer multiple VPCs requires the creation of many objects, an `Expose` object for each VPC and a set of `Route` object for each VPC and dest VPC.  To make this easier for users, there is a Peer object that creates these objects for the user:

```
kind: gateway/Peer
metadata: 
  name: peer-vpc1-vpc2
spec:
  vpcs:
    - vpc: vpc1
      ip: 10.1.1.0/24
      as: 192.168.10.0/24  # VPC 1's addresses are using NAT
    - vpc: vp2
      ip: 10.2.0.0/16
    - vpc: vpc3
      ip: 10.3.1.0/24
```

This would create an `Expose` object for each VPC, and 2 `Route` objects for each VPC which names the other 2 VPCs.

The one issue here is that if these VPCs also `Expose` other addresses, and these are the
same as the peering addresses, we'll get a conflict.  We should think about what the best
way to address this is, in a user friendly way.  Perhaps we allow the user to specify peering but ask the Peer object to use a particular `Expose` object instead of generating
one in some cases.  Then the `Peer` object setup has to check that the `Expose` object
specified has the correct config not to break peering.

Or perhaps, if there is an existing `Expose` object that conflicts with one that would
be auto generated, an error is raised if the configuration of the `Expose` object is
incompatible with the peering, with advice on how to make it compatible.

## Static Routing to Externals
```
kind: gateway/Expose
metadata: 
  name: expose-external1
spec:
  expose:
    external: "external1"
    ip: 0.0.0.0/0 # All addresses, no DNAT
    exclude: internally-allocated # Do we want this, or is it implied?
```

```
kind: gateway/Route
metadata: 
  name: vpc2-to-external1
spec:
  src:
    vpc: vpc2
    ip: 0.0.0.0/0
    as: 192.168.21.0/31
    nat-type: stateful
  dest:
    ip: { external: expose-external1 }
--
kind: gateway/Route
metadata: 
  name: vpc2-to-external2
spec:
  src:
    vpc: vpc2
    ip: 0.0.0.0/0
    as: 192.168.21.0/31
    nat-type: stateful
  dest:
    ip: { external: expose-external2 }
```

Initially we use LPM matching to route to the externals.  If two prefixes match both externals with the same length, we just pick one for now. Later (see below), we should allow more complex route configuration, filtering, and prioritization.

With routes to multiple externals, is there an issue if the NAT policy is different on each?  For stateful NAT, I think it is fine since the return routes would be looked 
up in the flow table.  

For static NAT, what if the NAT blocks are different and there is asymmetric return routing?  Perhaps that should be forbidden, or maybe it is just a warning to the user?

## Dynamic Routing to Externals

This needs some thought, but the idea would be to inject the relevant routes into
every VRF that has a `Route` object pointing to the `External` object.  We would
want to filter any routes for addresses that are explicitly in other `Expose` blocks.
We'd also want the `Route` and `Expose` objects to filter routes themselves and
prioritize different routes when connected to multiple externals.

## Implied Routes

Whenever a `Route` object names addresses in an `Expose` object, an obvious forward route is created.  However, there is also an implied return route.  In the case that the `Expose` object opts for a `stateful` return route, then we use stateful connection tracking ot decide the return route.  

However, in the case that `static` is chosen, then there is an implied route from
the addresses in the `Expose` block to the addresses in the `Route` block, or all IPs
in the `Route` block VPC, if no IP addresses are named.

Note(manish) I believe that the return route is always unique and well defined given the
restrictions above.  If not, we should discuss new restrictions and scenarios.

## Routing between externals

I propose treating externals like VPCs so then routing between externals just makes sense.  We just create `Route` blocks that name the `External` as a source.  However, we have to think through routing priority, filtering, etc. to make sure it all makes sense.