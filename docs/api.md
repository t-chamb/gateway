# API Reference

## Packages
- [gateway.githedgehog.com/v1alpha1](#gatewaygithedgehogcomv1alpha1)


## gateway.githedgehog.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the gateway v1alpha1 API group.

### Resource Types
- [PeeringInterface](#peeringinterface)



#### PeeringInterface



PeeringInterface is the Schema for the peeringinterfaces API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `gateway.githedgehog.com/v1alpha1` | | |
| `kind` _string_ | `PeeringInterface` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PeeringInterfaceSpec](#peeringinterfacespec)_ |  |  |  |
| `status` _[PeeringInterfaceStatus](#peeringinterfacestatus)_ |  |  |  |


#### PeeringInterfaceSpec



PeeringInterfaceSpec defines the desired state of PeeringInterface.



_Appears in:_
- [PeeringInterface](#peeringinterface)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `foo` _string_ | Foo is an example field of PeeringInterface. Edit peeringinterface_types.go to remove/update |  |  |


#### PeeringInterfaceStatus



PeeringInterfaceStatus defines the observed state of PeeringInterface.



_Appears in:_
- [PeeringInterface](#peeringinterface)



