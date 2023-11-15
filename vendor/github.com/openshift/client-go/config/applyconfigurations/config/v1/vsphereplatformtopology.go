// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// VSpherePlatformTopologyApplyConfiguration represents an declarative configuration of the VSpherePlatformTopology type for use
// with apply.
type VSpherePlatformTopologyApplyConfiguration struct {
	Datacenter     *string  `json:"datacenter,omitempty"`
	ComputeCluster *string  `json:"computeCluster,omitempty"`
	Networks       []string `json:"networks,omitempty"`
	Datastore      *string  `json:"datastore,omitempty"`
	ResourcePool   *string  `json:"resourcePool,omitempty"`
	Folder         *string  `json:"folder,omitempty"`
}

// VSpherePlatformTopologyApplyConfiguration constructs an declarative configuration of the VSpherePlatformTopology type for use with
// apply.
func VSpherePlatformTopology() *VSpherePlatformTopologyApplyConfiguration {
	return &VSpherePlatformTopologyApplyConfiguration{}
}

// WithDatacenter sets the Datacenter field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Datacenter field is set to the value of the last call.
func (b *VSpherePlatformTopologyApplyConfiguration) WithDatacenter(value string) *VSpherePlatformTopologyApplyConfiguration {
	b.Datacenter = &value
	return b
}

// WithComputeCluster sets the ComputeCluster field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ComputeCluster field is set to the value of the last call.
func (b *VSpherePlatformTopologyApplyConfiguration) WithComputeCluster(value string) *VSpherePlatformTopologyApplyConfiguration {
	b.ComputeCluster = &value
	return b
}

// WithNetworks adds the given value to the Networks field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Networks field.
func (b *VSpherePlatformTopologyApplyConfiguration) WithNetworks(values ...string) *VSpherePlatformTopologyApplyConfiguration {
	for i := range values {
		b.Networks = append(b.Networks, values[i])
	}
	return b
}

// WithDatastore sets the Datastore field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Datastore field is set to the value of the last call.
func (b *VSpherePlatformTopologyApplyConfiguration) WithDatastore(value string) *VSpherePlatformTopologyApplyConfiguration {
	b.Datastore = &value
	return b
}

// WithResourcePool sets the ResourcePool field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ResourcePool field is set to the value of the last call.
func (b *VSpherePlatformTopologyApplyConfiguration) WithResourcePool(value string) *VSpherePlatformTopologyApplyConfiguration {
	b.ResourcePool = &value
	return b
}

// WithFolder sets the Folder field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Folder field is set to the value of the last call.
func (b *VSpherePlatformTopologyApplyConfiguration) WithFolder(value string) *VSpherePlatformTopologyApplyConfiguration {
	b.Folder = &value
	return b
}
