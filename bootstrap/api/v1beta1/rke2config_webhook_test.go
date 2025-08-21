/*
Copyright 2024 SUSE LLC.

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

package v1beta1

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

func TestRKE2Config_ValidateCreate(t *testing.T) {
	tests := []struct {
		name      string
		spec      *RKE2ConfigSpec
		expectErr bool
	}{
		{
			name: "private registry no tls or auth secret",
			spec: &RKE2ConfigSpec{
				PrivateRegistriesConfig: Registry{
					Configs: map[string]RegistryConfig{
						"testing.io": {},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "private registry empty tls",
			spec: &RKE2ConfigSpec{
				PrivateRegistriesConfig: Registry{
					Configs: map[string]RegistryConfig{
						"testing.io": {
							TLS: TLSConfig{},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "private registry empty auth",
			spec: &RKE2ConfigSpec{
				PrivateRegistriesConfig: Registry{
					Configs: map[string]RegistryConfig{
						"testing.io": {
							AuthSecret: v1.ObjectReference{},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	validator := RKE2ConfigCustomValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			config := &RKE2Config{
				Spec: *tt.spec.DeepCopy(),
			}

			_, err := validator.ValidateCreate(context.Background(), config)

			if tt.expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func Test_correctArbitraryData(t *testing.T) {
	type args struct {
		arbitraryData map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "valid data",
			args: args{
				arbitraryData: map[string]string{
					"bootcmd": `
- touch /bootcmd.sentinel
- echo 192.168.1.130 us.archive.ubuntu.com > /etc/hosts
- [cloud-init-per, once, mymkfs, mkfs, /dev/vdb]`,
					"ntp": `
  enabled: true
  ntp_client: chrony`,
					"device_aliases:": "{my_alias: /dev/sdb, swap_disk: /dev/sdc}",
					"resize_rootfs":   "noblock",
					"list":            "['data']",
					"users": `
- name: capv
  ssh_authorized_keys:
  - ssh-rsa ABCDEFGHIJKLMNOPQRSTUVWXYZ caprke2@localhost.localdomain
  - ssh-rsa ABCDEFGHIJKLMNOPQRSTUVWXYZ turtles@localhost.localdomain
  sudo: ALL=(ALL) NOPASSWD:ALL`,
					"disk_setup": `ephemeral0:
  table_type: mbr
  layout: False
  overwrite: False`,
				},
			},
			want: map[string]string{
				"bootcmd":         "\n- touch /bootcmd.sentinel\n- echo 192.168.1.130 us.archive.ubuntu.com > /etc/hosts\n- - cloud-init-per\n  - once\n  - mymkfs\n  - mkfs\n  - /dev/vdb\n",
				"ntp":             "\n  enabled: true\n  ntp_client: chrony\n  ",
				"device_aliases:": "\n  my_alias: /dev/sdb\n  swap_disk: /dev/sdc\n  ",
				"resize_rootfs":   "noblock",
				"users":           "\n- name: capv\n  ssh_authorized_keys:\n    - ssh-rsa ABCDEFGHIJKLMNOPQRSTUVWXYZ caprke2@localhost.localdomain\n    - ssh-rsa ABCDEFGHIJKLMNOPQRSTUVWXYZ turtles@localhost.localdomain\n  sudo: ALL=(ALL) NOPASSWD:ALL\n",
				"list":            "\n- data\n",
				"disk_setup":      "\n  ephemeral0:\n    layout: false\n    overwrite: false\n    table_type: mbr\n  ",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			if err := correctArbitraryData(tt.args.arbitraryData); (err != nil) != tt.wantErr {
				t.Errorf("correctArbitraryData() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				Expect(tt.args.arbitraryData).To(Equal(tt.want))
			}
		})
	}
}
