package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Testing RKE2 to Kubernetes Version conversion", func() {
	machineVersion := "1.24.6"
	rke2Version := "v1.24.6+rke2r1"
	It("Should match RKE2 and Kubernetes version", func() {
		cpKubeVersion, err := Rke2ToKubeVersion(rke2Version)
		Expect(err).ToNot(HaveOccurred())
		Expect(cpKubeVersion).To(Equal(machineVersion))
	})
})

var _ = Describe(("Testing GetMapKeysAsString"), func() {
	It("Should return a slice of strings", func() {
		testMap := map[string][]byte{
			"hello": []byte("world"),
			"foo":   []byte("bar"),
		}
		keys := GetMapKeysAsString(testMap)
		keysValid := keys == "hello,foo" || keys == "foo,hello"
		Expect(keysValid).To(BeTrue())
	})
})
