# Proposed API

All connections between VPCs is done via `Peering` object.

2 VPCs can only have a single `Peering` object between them.

External connections are modeled as VPCs where we can separately configure
how we map incoming traffic to thhe VPC (VNI, VLAN, QinQ, MPLS, etc.)

## Duplicate/Ambiguous routes

Here, there are no duplicate IP restrictions, if there is multipath you just get
ECMP. We can warn the user.  The policy is based on whatever route we pick.  However,
there are route metrics to prefer one path to the other.

This helps with the multiple external cases where one VPC is routing to 2 externals
and we want to use route metrics advertised via BGP to choose routes.

## Questions

Is this implementable?

frostman and mvachhar believe so, others should check

Do we need explict NAT for use cases that don't involve fabric?

Is all round trip routing stateless, or can we specify directional stateful routing?

frostman and mvachhar think that if the expose is not stateless, return routing can
be based on flow state.  How does this interact with other configuration?

## Use Cases

```yaml
# Static NAT, VPC 1 -> VPC 2 and vice versa
# VPC 2 exposes http port 80 on its private subnet 10.2.1.1/32
# Any IP from VPC 1 can connect to VPC 2 on 10.2.1.1/32
# All static, no dynamic/stateful
apiVersion: gateway.githedgehog.com/v1alpha1
kind: Peering
metadata:
  name: vpc-1--vpc-2
spec:
  vpc1:
    ips:
      - cidr: 10.1.1.0/24
      # - fromVPCSubnet: subnet1 # just a shorthand for the above
    as: # Means static Src/Dst NAT for vpc1
      - 192.168.1.0/24
    ingress:
      - allow:
          stateless: true # it's the only options supported in the first release
          tcp:
            dstPort: 443
  vpc2:
    ips:
      - cidr: 10.2.1.1/32
    ingress:
      - allow:
          stateless: true
          tcp:
            srcPort: 443
```

```yaml
# vpc-e1 is external 1 and vpc-e2 is external 2
# Both advertise a dynamic set of routes, up to and including the whole internet
apiVersion: gateway.githedgehog.com/v1alpha1
kind: Peering
metadata:
  name: vpc-e1--vpc-e2
spec:
  vpc-e1:
    ips:
      - cidr: 0.0.0.0/0
      - not: 10.0.0.0/8
      - not: 192.168.0.0/16
      - not: 1.2.3.0/24
  vpc-e2:
    ips:
      - cidr: 0.0.0.0/0
      - not: 10.0.0.0/8
      - not: 192.168.0.0/16
      - not: 3.2.1.0/30
```

```yaml
# internet access from vpc-1 using external vpc-e1
apiVersion: gateway.githedgehog.com/v1alpha1
kind: Peering
metadata:
  name: vpc-1--vpc-e1
spec:
  vpc-1:
    ips:
      - cidr: 10.1.1.0/24
  vpc-e1:
    ips:
      - cidr: 0.0.0.0/0
      - not: 10.0.0.0/8
      - not: 192.168.0.0/16
      - not: 3.2.1.0/30
    as: # Is this dynamic NAT since there are too few addresses here?
        # which direction is the NAT here?
        # or should this be on vpc-1
      - 192.168.1.0/30
```

```yaml
# vpc-1 connects to internet using vpc-e1 or vpc-e2 based on cost
apiVersion: gateway.githedgehog.com/v1alpha1
kind: Peering
metadata:
  name: vpc-1--vpc-e1
spec:
  vpc-1:
    ips:
      - cidr: 10.1.1.0/24
    as:
      - 192.168.1.0/30
    natType: stateful
  vpc-e1:
    metric: 0 # add 0 to the advertised route metrics
    # At what point do we not advertise these routes to the switch, how do we decide?
    ips:
      - cidr: 0.0.0.0/0
      - not: 10.0.0.0/8
      - not: 192.168.0.0/16
      - not: 1.2.3.0/30
---
apiVersion: gateway.githedgehog.com/v1alpha1
kind: Peering
metadata:
  name: vpc-1--vpc-e2
spec:
  vpc-1:
    ips:
      - cidr: 10.1.1.0/24
    as:
      - 192.168.1.0/30
    natType: stateful
  vpc-e2:
    metric: 10 # add 10 to the route metric advertised externally
    # At what point do we not advertise these routes to the switch, how do we decide?
    ips:
      - cidr: 0.0.0.0/0
      - not: 10.0.0.0/8
      - not: 192.168.0.0/16
      - not: 3.2.1.0/30
```

```yaml
# vpc-1 <> vpc-1 with overlapping subnets
apiVersion: gateway.githedgehog.com/v1alpha1
kind: Peering
metadata:
  name: vpc-1--vpc-2
spec:
  vpc-1:
    ips:
      - cidr: 10.1.1.0/24
      - not: 10.1.1.42/32
    as:
      - 192.168.1.0/24
  vpc-2:
    ips:
      - cidr: 10.1.1.0/24
    as:
      - 192.168.2.0/24

# { src: vpc-1,10.1.1.0/24 ; dst: 192.168.2.0/24 }
# { src: vpc-2,10.1.1.0/24 ; dst: 192.168.1.0/24 }
```
