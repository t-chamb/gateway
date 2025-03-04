# Api proposal

This API does not deviate much from what has been discussed in the past, but eases and improves some aspects and removes some requirements that were not completely defined and caused confusion.

## General ideas
* A vpc can have a collection of **Peering Interfaces** (pifs)
* The intended user experience is for the user to see a Pif just as a network interface, to the extent that it should be possible to bring a PIF administratively down or up by its owner. Also, a PIF may get some properties configured such as QoS, maximum bandwidth, etc.
* PIFs do nothing until “connected” by the user.
* A PIF can be connected to one or more remote pifs using a **virtual link**. In the future, we may consider that PIFs may be connected to other entities if needed, like virtual hubs, if that happens to simplify the model or its configuration.
* Users see virtual links exactly as cables joining PIFs.
* Each side of a virtual link (i.e. PIF) is independently managed. This is important for multi-tenancy and should be considered from day 0.
* This model only contains **Pifs** and **virtual links** as main objects.
* In this model, there does not exist a GW configuration per se. For the user, Pifs exist in their VPCs. Virtual links reference PIFs within vpcs and are external to both. Again, this is to allow for multi-tenancy where the creation of virtual links between VPCs managed by different third-parties may need to be coordinated by some "Fabric owner", which would honour the creation of a virtual link after some form of agreement between VPCs. The fact that the GW is not "mentioned" in this API does not preclude the standalone use of the Gateway in another environment, or its use with a non-Hedghehog Fabric.

In the following, I am considering that NAT is always needed to simplify the discussion. This does not impose any restriction conceptually: If NAT is not needed, assume that each address is natted to itself, transparently.

## Nomenclature:
* **Internal Ips**: the IP addresses that devices within VPCs use. Usually, these are RFC1918 private.
* **External IPs**: the IPs seen by devices external to the VPC. In the general case where NAT is used, these are the addresses that get advertised in BGP. Without NAT, the external IPs may just match the internal ones.
* **Mapping function**: configurable criteria to map External to internal addresses and vice-versa. The definition of this entity is purposedly left open since it may accommodate multiple use cases without the need to modify the rest of the model.

## Peering interfaces:

Every PIF has a name/Id unique within a VPC and specifies:

* What external addresses of a VPC are reachable / advertised over it. How these addresses are mapped to the internal addresses is irrelevant to the outside. If a VPC is providing a service, the rest of VPCs potentially consuming it only need to know the external addresses for it. Internally, the VPC owning that service may, at its sole discretion, map it to one or more internal ips, or to a load balancer or whatever other entity (firewall, IPs, …). This is mostly a DNAT function.
* What internal addresses can use a certain Pif to communicate to the outside (VPC, external entity). This requires mostly an SNAT function. If NAT is needed, an internal address cannot communicate to the outside if the mapping function does not provide it with an external address.

## Virtual links
Virtual links replace peering policies. A virtual link simply permits the communication between external addresses between two VPCs. A virtual link is agnostic to the communication semantics or patterns. It simply enables connectivity. PIFs govern the rest, like what is reachable or not and under which conditions (e.g. max-bandwidth), at each side of a virtual link.

## The mapping function:
The mapping function specifies how internal addresses and external addresses relate. We use the following notation:

a -> A means internal address a maps to external address A when egressing (SNAT).

a <- A means external address A shall be mapped to a (DNAT)

a <-> A means internal address a may be SNAT''ed to A when going to external world (if initiating communication) and that A may map to a when ingressing the VPC (DNAT)

p/l -> P/L means an address in range p/l may pick any address from P/L when egressing the VPC when initiating communications. If l > L the pool of external addresses P/L may not suffice and we may need to do PAT. But that challenge is inherent and independent of the model.

p/l <- P/L what you would expect.

p/l <-> P/L what you would expect.

## Sample configs
Sample snippets just for illustration, not conforming to anything, illustrating how the api could look like.
```
VPC: Vpc-1 (existing definition)
  PifX:
    Internal: [10.0.1.0/24] // these IPs may communicate through this pif
    External: [192.168.1.0/24] //... using addresses in this range
    Mapping function:  
		10.0.1.0/24 -> 192.168.1.0/24

    explanation: let any IP in 10.0.1.0/24 use this pif to reach to the destinations 
                 at any the other end(s) of the virtual link(s) it is connected to 
                 by mapping the addresses to any address in 192.1681.0/24.

    // other stuff that could go in a pif
    Max-rate: 200MB/s
    Qos: some-user-defined-qos-object (optional)
    Filtering:
       Ingress: user-filtering-rule-object (optional)
       Egress:  user-filtering-rule-object (optional)

VPC: Vpc-2 (existing definition)
  PifY:
    Internal: 10.0.2.0/24
    External: 192.168.2.0/24
    Mapping function: 
       1) 10.0.2.1 -> 192.168.2.29
       2) 10.0.2.2 <-> 192.168.2.30
       3) 10.0.2.8/30 <- 192.168.2.31
       4) 10.0.2.128/25 -> 192.168.2.128/25

	explanation:
      1) 10.0.2.1 can go outside as 192.168.2.29 (snat)
      2) 10.0.2.2 can go outside as 192.168.2.30(snat). 
         It can be contacted externally at 192.168.2.30
      3) 10.0.2.[8,9,10,11] can be contacted from outside as 192.168.2.31
      4) any ip in 10.0.2.128/25 will use any ip from 192.168.2.128/25 as external identity.
      5) 10.0.2.0/25 except for 10.0.2.[1,2,8,9,10,11] will not be reachable from outside nor are 
         allowed to get outside since the mapping function did not cover them

// at a distinct scope (Fabric)
Virtual links:
    left: 
      vpc: Vpc-1
      interface: PifX
    Right: 
      vpc: VPC-2
      interface: PifY
```



## How it differs from what was specified in the past
* no communication semantics or patterns. These are in the mapping function. Virtual links just define potential reachability.
* External addresses, while referred to by PIFs are not exclusive of  particular PIFs but VPCs. This is an important difference affecting resource usage.

## Pros of this model (IMO):
* feels natural to users: they plug cables and specify what can flow through them. Pifs indicate egress / ingress points.
* Having an interface object also feels natural, demarcates ownership (needed for multi-tenancy) and acts as an anchor point to extend the model in the future or provide certain functions (rate limitting, but also traffic inspection for instance).
* Can't be too distinct from the implementation. The mapping function may seem cumbersome, but may resemble what the gateway will do after all. It gives significant flexibility.
* The model is easy. The complexity (if any) lies in the mapping function. That's a single place where users shall concentrate care, which they should/will.

## Constraints
* An external addresses can belong to only one VPC.
* No other restriction is needed IMO, but happy to discuss why.
   * There may be some confusion if two vpcs are connected by multiple pifs, but I don't think this invalidates the model yet. If it did, we would require the disjointness restriction we had in the prior model.
  
