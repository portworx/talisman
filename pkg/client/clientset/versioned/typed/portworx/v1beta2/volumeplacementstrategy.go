/*
Copyright 2017 The Portworx Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package v1beta2

import (
	v1beta2 "github.com/portworx/talisman/pkg/apis/portworx/v1beta2"
	scheme "github.com/portworx/talisman/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// VolumePlacementStrategiesGetter has a method to return a VolumePlacementStrategyInterface.
// A group's client should implement this interface.
type VolumePlacementStrategiesGetter interface {
	VolumePlacementStrategies() VolumePlacementStrategyInterface
}

// VolumePlacementStrategyInterface has methods to work with VolumePlacementStrategy resources.
type VolumePlacementStrategyInterface interface {
	Create(*v1beta2.VolumePlacementStrategy) (*v1beta2.VolumePlacementStrategy, error)
	Update(*v1beta2.VolumePlacementStrategy) (*v1beta2.VolumePlacementStrategy, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1beta2.VolumePlacementStrategy, error)
	List(opts v1.ListOptions) (*v1beta2.VolumePlacementStrategyList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta2.VolumePlacementStrategy, err error)
	VolumePlacementStrategyExpansion
}

// volumePlacementStrategies implements VolumePlacementStrategyInterface
type volumePlacementStrategies struct {
	client rest.Interface
}

// newVolumePlacementStrategies returns a VolumePlacementStrategies
func newVolumePlacementStrategies(c *PortworxV1beta2Client) *volumePlacementStrategies {
	return &volumePlacementStrategies{
		client: c.RESTClient(),
	}
}

// Get takes name of the volumePlacementStrategy, and returns the corresponding volumePlacementStrategy object, and an error if there is any.
func (c *volumePlacementStrategies) Get(name string, options v1.GetOptions) (result *v1beta2.VolumePlacementStrategy, err error) {
	result = &v1beta2.VolumePlacementStrategy{}
	err = c.client.Get().
		Resource("volumeplacementstrategies").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of VolumePlacementStrategies that match those selectors.
func (c *volumePlacementStrategies) List(opts v1.ListOptions) (result *v1beta2.VolumePlacementStrategyList, err error) {
	result = &v1beta2.VolumePlacementStrategyList{}
	err = c.client.Get().
		Resource("volumeplacementstrategies").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested volumePlacementStrategies.
func (c *volumePlacementStrategies) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Resource("volumeplacementstrategies").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a volumePlacementStrategy and creates it.  Returns the server's representation of the volumePlacementStrategy, and an error, if there is any.
func (c *volumePlacementStrategies) Create(volumePlacementStrategy *v1beta2.VolumePlacementStrategy) (result *v1beta2.VolumePlacementStrategy, err error) {
	result = &v1beta2.VolumePlacementStrategy{}
	err = c.client.Post().
		Resource("volumeplacementstrategies").
		Body(volumePlacementStrategy).
		Do().
		Into(result)
	return
}

// Update takes the representation of a volumePlacementStrategy and updates it. Returns the server's representation of the volumePlacementStrategy, and an error, if there is any.
func (c *volumePlacementStrategies) Update(volumePlacementStrategy *v1beta2.VolumePlacementStrategy) (result *v1beta2.VolumePlacementStrategy, err error) {
	result = &v1beta2.VolumePlacementStrategy{}
	err = c.client.Put().
		Resource("volumeplacementstrategies").
		Name(volumePlacementStrategy.Name).
		Body(volumePlacementStrategy).
		Do().
		Into(result)
	return
}

// Delete takes name of the volumePlacementStrategy and deletes it. Returns an error if one occurs.
func (c *volumePlacementStrategies) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("volumeplacementstrategies").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *volumePlacementStrategies) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Resource("volumeplacementstrategies").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched volumePlacementStrategy.
func (c *volumePlacementStrategies) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta2.VolumePlacementStrategy, err error) {
	result = &v1beta2.VolumePlacementStrategy{}
	err = c.client.Patch(pt).
		Resource("volumeplacementstrategies").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
