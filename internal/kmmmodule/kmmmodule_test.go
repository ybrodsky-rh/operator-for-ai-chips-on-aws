/*
Copyright 2022.

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

package kmmmodule

import (
	"fmt"
	"os"

	awslabsv1beta1 "github.com/awslabs/operator-for-ai-chips-on-aws/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	v1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("setKMMModuleLoader", func() {
	It("KMM module creation - default input values", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Module",
				APIVersion: "kmm.sigs.x-k8s.io/v1beta1",
			},
		}
		input := awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage: "some image:tag",
			},
		}

		expectedYAMLFile, err := os.ReadFile("testdata/module_loader_test.yaml")
		Expect(err).To(BeNil())
		expectedMod := kmmv1beta1.Module{}
		expectedJSON, err := yaml.YAMLToJSON(expectedYAMLFile)
		Expect(err).To(BeNil())
		err = yaml.Unmarshal(expectedJSON, &expectedMod)
		Expect(err).To(BeNil())
		fmt.Printf("<%s>\n", expectedMod.Name)
		fmt.Printf("<%s>\n", expectedMod.Spec.ModuleLoader.Container.Modprobe.ModuleName)
		Expect(len(expectedMod.Spec.ModuleLoader.Container.KernelMappings)).To(Equal(1))

		expectedMod.Spec.ModuleLoader.Container.KernelMappings[0].ContainerImage = "some image:tag-$KERNEL_VERSION"
		expectedMod.Spec.Selector = map[string]string{"feature.node.kubernetes.io/pci-1d0f.present": "true"}

		err = setKMMModuleLoader(&mod, &input)

		Expect(err).To(BeNil())
		Expect(mod).To(BeComparableTo(expectedMod))
	})

	It("KMM module creation - user input values", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Module",
				APIVersion: "kmm.sigs.x-k8s.io/v1beta1",
			},
		}
		input := awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				UseInTreeDrivers: false,
				DriversImage:     "some driver image",
				Selector:         map[string]string{"some label": "some label value"},
				ImageRepoSecret:  &v1.LocalObjectReference{Name: "image repo secret name"},
			},
		}

		expectedYAMLFile, err := os.ReadFile("testdata/module_loader_test.yaml")
		Expect(err).To(BeNil())
		expectedMod := kmmv1beta1.Module{}
		expectedJSON, err := yaml.YAMLToJSON(expectedYAMLFile)
		Expect(err).To(BeNil())
		err = yaml.Unmarshal(expectedJSON, &expectedMod)
		Expect(err).To(BeNil())
		fmt.Printf("<%s>\n", expectedMod.Name)
		fmt.Printf("<%s>\n", expectedMod.Spec.ModuleLoader.Container.Modprobe.ModuleName)
		Expect(len(expectedMod.Spec.ModuleLoader.Container.KernelMappings)).To(Equal(1))

		expectedMod.Spec.ModuleLoader.Container.KernelMappings[0].ContainerImage = "some driver image-$KERNEL_VERSION"
		expectedMod.Spec.Selector = map[string]string{"some label": "some label value"}
		expectedMod.Spec.ImageRepoSecret = &v1.LocalObjectReference{Name: "image repo secret name"}

		err = setKMMModuleLoader(&mod, &input)

		Expect(err).To(BeNil())
		Expect(mod).To(BeComparableTo(expectedMod))
	})
})

var _ = Describe("SetKMMModuleAsDesired", func() {
	km := NewKMMModule(nil, scheme)

	It("should skip device plugin when DevicePluginImage is empty (DRA mode)", func() {
		mod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
		}
		input := &awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage:      "some image:tag",
				DevicePluginImage: "",
			},
		}

		err := km.SetKMMModuleAsDesired(mod, input)
		Expect(err).To(BeNil())
		Expect(mod.Spec.ModuleLoader).ToNot(BeNil())
		Expect(mod.Spec.DevicePlugin).To(BeNil())
	})

	It("should set device plugin when DevicePluginImage is provided", func() {
		mod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
		}
		input := &awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage:      "some image:tag",
				DevicePluginImage: "some device plugin image",
			},
		}

		err := km.SetKMMModuleAsDesired(mod, input)
		Expect(err).To(BeNil())
		Expect(mod.Spec.ModuleLoader).ToNot(BeNil())
		Expect(mod.Spec.DevicePlugin).ToNot(BeNil())
		Expect(mod.Spec.DevicePlugin.Container.Image).To(Equal("some device plugin image"))
	})

	It("should set DRA when DRADriverImage is provided", func() {
		mod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
		}
		input := &awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage:   "some image:tag",
				DRADriverImage: "some-dra-image:latest",
			},
		}

		err := km.SetKMMModuleAsDesired(mod, input)
		Expect(err).To(BeNil())
		Expect(mod.Spec.ModuleLoader).ToNot(BeNil())
		Expect(mod.Spec.DRA).ToNot(BeNil())
		Expect(mod.Spec.DevicePlugin).To(BeNil())
		Expect(mod.Spec.DRA.Container.Image).To(Equal("some-dra-image:latest"))
		Expect(mod.Spec.DRA.DriverName).To(Equal("neuron.aws.com"))
		Expect(mod.Spec.DRA.ServiceAccountName).To(Equal("awslabs-gpu-operator-dra-driver"))
		Expect(mod.Spec.DRA.Container.Command).To(Equal([]string{"k8s-neuron-dra-driver"}))
	})

	It("should set DRA with default DeviceClass when none specified", func() {
		mod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
		}
		input := &awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage:   "some image:tag",
				DRADriverImage: "some-dra-image:latest",
			},
		}

		err := km.SetKMMModuleAsDesired(mod, input)
		Expect(err).To(BeNil())
		Expect(mod.Spec.DRA.DeviceClasses).To(HaveLen(1))
		Expect(mod.Spec.DRA.DeviceClasses[0].Name).To(Equal("neuron.aws.com"))
		Expect(mod.Spec.DRA.DeviceClasses[0].Selectors).To(HaveLen(1))
		Expect(mod.Spec.DRA.DeviceClasses[0].Selectors[0].CEL.Expression).To(Equal(`device.driver == "neuron.aws.com"`))
	})

	It("should set DRA with custom DeviceClasses when specified", func() {
		mod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
		}
		input := &awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage:   "some image:tag",
				DRADriverImage: "some-dra-image:latest",
				DeviceClasses: []awslabsv1beta1.DeviceClassSpec{
					{
						Name: "custom-class-a",
						Selectors: []resourcev1.DeviceSelector{
							{
								CEL: &resourcev1.CELDeviceSelector{
									Expression: `device.driver == "custom.driver"`,
								},
							},
						},
					},
					{
						Name: "custom-class-b",
					},
				},
			},
		}

		err := km.SetKMMModuleAsDesired(mod, input)
		Expect(err).To(BeNil())
		Expect(mod.Spec.DRA.DeviceClasses).To(HaveLen(2))
		Expect(mod.Spec.DRA.DeviceClasses[0].Name).To(Equal("custom-class-a"))
		Expect(mod.Spec.DRA.DeviceClasses[0].Selectors).To(HaveLen(1))
		Expect(mod.Spec.DRA.DeviceClasses[1].Name).To(Equal("custom-class-b"))
	})

	It("DRA mode should clear DevicePlugin if previously set", func() {
		mod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
		}
		mod.Spec.DevicePlugin = &kmmv1beta1.DevicePluginSpec{
			Container: kmmv1beta1.CommonContainerSpec{
				Image: "old-device-plugin",
			},
		}
		input := &awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DriversImage:   "some image:tag",
				DRADriverImage: "some-dra-image:latest",
			},
		}

		err := km.SetKMMModuleAsDesired(mod, input)
		Expect(err).To(BeNil())
		Expect(mod.Spec.DevicePlugin).To(BeNil())
		Expect(mod.Spec.DRA).ToNot(BeNil())
	})
})

var _ = Describe("setKMMDRA", func() {
	It("sets correct container spec", func() {
		mod := &kmmv1beta1.Module{}
		input := &awslabsv1beta1.DeviceConfig{
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DRADriverImage: "my-dra-image:v1",
			},
		}

		setKMMDRA(mod, input)

		Expect(mod.Spec.DRA).ToNot(BeNil())
		Expect(mod.Spec.DRA.Container.Image).To(Equal("my-dra-image:v1"))
		Expect(mod.Spec.DRA.Container.Command).To(Equal([]string{"k8s-neuron-dra-driver"}))
		Expect(mod.Spec.DRA.Container.Resources.Requests[v1.ResourceCPU]).To(Equal(resource.MustParse("20m")))
		Expect(mod.Spec.DRA.Container.Resources.Requests[v1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
		Expect(mod.Spec.DRA.Container.Resources.Limits[v1.ResourceCPU]).To(Equal(resource.MustParse("20m")))
		Expect(mod.Spec.DRA.Container.Resources.Limits[v1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
		Expect(mod.Spec.DRA.DriverName).To(Equal("neuron.aws.com"))
		Expect(mod.Spec.DRA.ServiceAccountName).To(Equal("awslabs-gpu-operator-dra-driver"))
		Expect(mod.Spec.DevicePlugin).To(BeNil())
	})
})

var _ = Describe("setKMMDevicePlugin", func() {
	It("KMM module creation - default input values", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Module",
				APIVersion: "kmm.sigs.x-k8s.io/v1beta1",
			},
		}

		input := awslabsv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DevicePluginImage: "some device plugin image",
			},
		}

		expectedYAMLFile, err := os.ReadFile("testdata/device_plugin_test.yaml")
		Expect(err).To(BeNil())
		expectedMod := kmmv1beta1.Module{}
		expectedJSON, err := yaml.YAMLToJSON(expectedYAMLFile)
		Expect(err).To(BeNil())
		err = yaml.Unmarshal(expectedJSON, &expectedMod)
		Expect(err).To(BeNil())
		expectedMod.Spec.DevicePlugin.Container.Image = "some device plugin image"

		setKMMDevicePlugin(&mod, &input)

		Expect(mod).To(BeComparableTo(expectedMod))
	})

	It("KMM module creation - user input values", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "moduleName",
				Namespace: "moduleNamespace",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Module",
				APIVersion: "kmm.sigs.x-k8s.io/v1beta1",
			},
		}

		input := awslabsv1beta1.DeviceConfig{
			Spec: awslabsv1beta1.DeviceConfigSpec{
				DevicePluginImage: "some device plugin image",
			},
		}

		expectedYAMLFile, err := os.ReadFile("testdata/device_plugin_test.yaml")
		Expect(err).To(BeNil())
		expectedMod := kmmv1beta1.Module{}
		expectedJSON, err := yaml.YAMLToJSON(expectedYAMLFile)
		Expect(err).To(BeNil())
		err = yaml.Unmarshal(expectedJSON, &expectedMod)
		Expect(err).To(BeNil())

		expectedMod.Spec.DevicePlugin.Container.Image = "some device plugin image"

		setKMMDevicePlugin(&mod, &input)

		Expect(mod).To(Equal(expectedMod))
	})
})
