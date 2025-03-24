# API Reference

## Packages
- [gateway.githedgehog.com/v1alpha1](#gatewaygithedgehogcomv1alpha1)


## gateway.githedgehog.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the gateway v1alpha1 API group.

### Resource Types
- [Peering](#peering)



#### Peering



Peering is the Schema for the peerings API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `gateway.githedgehog.com/v1alpha1` | | |
| `kind` _string_ | `Peering` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PeeringSpec](#peeringspec)_ |  |  |  |
| `status` _[PeeringStatus](#peeringstatus)_ |  |  |  |


#### PeeringSpec



PeeringSpec defines the desired state of Peering.



_Appears in:_
- [Peering](#peering)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `foo` _string_ | Foo is an example field of Peering. Edit peering_types.go to remove/update |  |  |


#### PeeringStatus



PeeringStatus defines the observed state of Peering.



_Appears in:_
- [Peering](#peering)



