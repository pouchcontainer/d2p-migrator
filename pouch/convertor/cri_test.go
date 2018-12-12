package convertor

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
	"github.com/pouchcontainer/d2p-migrator/utils"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	runtime "github.com/alibaba/pouch/cri/apis/v1alpha2"
)

func TestToCRIMetaJSON(t *testing.T) {
	// init a container meta with core parameters for CRI
	testCon := &localtypes.Container{
		ID:    "ce8999940e12e653d4308cc89a18e25fcf702dea250fc2428299bed5dfb03e92",
		Name:  "k8s_POD_nginx_default_4802cf32-f21e-11e8-b9c9-6c92bf2cf061_0",
		Image: "sha256:da86e6ba6ca197bf6bc5e9d900febd906b133eaa4750e6bed647b0fbe50ed43e",
		Config: &pouchtypes.ContainerConfig{
			Hostname: "nginx",
			SpecAnnotation: map[string]string{
				"kubernetes.io/config.seen":                "2018-11-27T16:27:30.623885666+08:00",
				"kubernetes.io/config.source":              "api",
				"pod.beta1.sigma.ali/container-state-spec": "{\"states\":{\"nginx\":\"running\"}}",
				"pod.beta1.sigma.ali/pod-spec-hash":        "1122abcd",
			},
			Labels: map[string]string{
				"ali/test":                                            "123456",
				"annotation.kubernetes.io/config.seen":                "2018-11-27T16:27:30.623885666+08:00",
				"annotation.kubernetes.io/config.source":              "api",
				"annotation.pod.beta1.sigma.ali/container-state-spec": "{\"states\":{\"nginx\":\"running\"}}",
				"annotation.pod.beta1.sigma.ali/pod-spec-hash":        "1122abcd",
				"io.kubernetes.pod.name":                              "nginx",
				"io.kubernetes.pod.namespace":                         "default",
				"io.kubernetes.pod.uid":                               "4802cf32-f21e-11e8-b9c9-6c92bf2cf061",
				"io.kubernetes.pouch.type":                            "sandbox",
			},
		},
		HostConfig: &pouchtypes.HostConfig{
			Runtime:     "runc",
			EnableLxcfs: false,
			Resources: pouchtypes.Resources{
				CgroupParent: "/kubepods/besteffort/pod4802cf32-f21e-11e8-b9c9-6c92bf2cf061",
			},
		},
		State: &pouchtypes.ContainerState{
			Pid: int64(159406),
		},
		NetworkSettings: &pouchtypes.NetworkSettings{
			Networks: map[string]*pouchtypes.EndpointSettings{
				"none": {
					EndpointID: "22e5df9fc43a97cc3f65fe89d18ebcc5cec2b1c6e33c170dda69922a705aad26",
					NetworkID:  "5f834b062553a2d9b8a91406fc2e75ea571f74a4b0c9bbe3666722a9e18c2f89",
				},
			},
		},
	}

	wantCRIMeta := localtypes.SandboxMeta{
		ID: "ce8999940e12e653d4308cc89a18e25fcf702dea250fc2428299bed5dfb03e92",
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      "nginx",
				Uid:       "4802cf32-f21e-11e8-b9c9-6c92bf2cf061",
				Namespace: "default",
			},
			Hostname:     "nginx",
			LogDirectory: "/var/log/pods/4802cf32-f21e-11e8-b9c9-6c92bf2cf061",
			Labels: map[string]string{
				"ali/test":                    "123456",
				"io.kubernetes.pod.name":      "nginx",
				"io.kubernetes.pod.namespace": "default",
				"io.kubernetes.pod.uid":       "4802cf32-f21e-11e8-b9c9-6c92bf2cf061",
			},
			Annotations: map[string]string{
				"kubernetes.io/config.seen":                "2018-11-27T16:27:30.623885666+08:00",
				"kubernetes.io/config.source":              "api",
				"pod.beta1.sigma.ali/container-state-spec": "{\"states\":{\"nginx\":\"running\"}}",
				"pod.beta1.sigma.ali/pod-spec-hash":        "1122abcd",
			},
			Linux: &runtime.LinuxPodSandboxConfig{
				CgroupParent: "/kubepods/besteffort/pod4802cf32-f21e-11e8-b9c9-6c92bf2cf061",
				SecurityContext: &runtime.LinuxSandboxSecurityContext{
					NamespaceOptions: &runtime.NamespaceOption{
						Pid: runtime.NamespaceMode_CONTAINER,
					},
				},
			},
		},
		Runtime:      "runc",
		LxcfsEnabled: false,
		NetNS:        "/proc/159406/ns/net",
	}

	gotCRIMeta, err := ToCRIMetaJSON(testCon)
	if err != nil {
		t.Fatalf("failed to generate CRI meta: %v", err)
	}

	// check general parameters
	if gotCRIMeta.ID != wantCRIMeta.ID || gotCRIMeta.Runtime != wantCRIMeta.Runtime || gotCRIMeta.LxcfsEnabled != wantCRIMeta.LxcfsEnabled {
		t.Errorf("wanted to (ID: %v, Runtime: %v, LxcfsEnabled: %v), got (%v, %v, %v)", wantCRIMeta.ID, wantCRIMeta.Runtime, wantCRIMeta.LxcfsEnabled, gotCRIMeta.ID, gotCRIMeta.Runtime, gotCRIMeta.LxcfsEnabled)
	}

	// check PodSandboxConfig
	if !reflect.DeepEqual(wantCRIMeta.Config, gotCRIMeta.Config) {
		t.Errorf("got wrong PodSandboxConfig, want %v, got %v", wantCRIMeta.Config, gotCRIMeta.Config)
	}
}

func Test_toDNSConfig(t *testing.T) {
	var (
		resolvContent = `search hz.ali.com
nameserver 1.1.1.1
nameserver  8.8.8.8
nameserver 4.4.4.4

options timeout:2 attempts:2`

		expectedDNSConfig = &runtime.DNSConfig{
			Servers:  []string{"1.1.1.1", "8.8.8.8", "4.4.4.4"},
			Searches: []string{"hz.ali.com"},
			Options:  []string{"timeout:2", "attempts:2"},
		}
	)

	resolvTmpDir, err := ioutil.TempDir("", "test_generate_dnsconfig")
	if err != nil {
		t.Errorf("failed to create a tmp dir: %v", err)
	}
	defer os.RemoveAll(resolvTmpDir)

	resolvFile := filepath.Join(resolvTmpDir, "resolv.conf")
	if err := ioutil.WriteFile(resolvFile, []byte(resolvContent), 0644); err != nil {
		t.Errorf("failed to write content %s to file %s: %v", resolvContent, resolvFile, err)
	}

	got, err := toDNSConfig(resolvFile)
	if err != nil {
		t.Errorf("failed to generate dnsconfig: %v", err)
	}

	if !utils.StringSliceEqual(expectedDNSConfig.Servers, got.Servers) || !utils.StringSliceEqual(expectedDNSConfig.Searches, got.Searches) || !utils.StringSliceEqual(expectedDNSConfig.Options, got.Options) {
		t.Errorf("generate dnscofig, expected %v, got %v", expectedDNSConfig, got)
	}
}

func Test_getPodSandboxMetadataBySandboxName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		want    *runtime.PodSandboxMetadata
		wantErr bool
	}{
		{
			name: "testNameNotMatch",
			args: args{
				name: "test",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testOK",
			args: args{
				name: "k8s_POD_mysql_default_98e2377b-f2ea-11e8-89af-6c92bf2cf061_0",
			},
			want: &runtime.PodSandboxMetadata{
				Name:      "mysql",
				Namespace: "default",
				Uid:       "98e2377b-f2ea-11e8-89af-6c92bf2cf061",
				Attempt:   uint32(0),
			},
			wantErr: false,
		},
		{
			name: "testAttempNotNumber",
			args: args{
				name: "k8s_POD_mysql_default_98e2377b-f2ea-11e8-89af-6c92bf2cf061_notnum",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testLessOne",
			args: args{
				name: "k8s_POD_mysql_default_98e2377b-f2ea-11e8-89af-6c92bf2cf061",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testOneExtra",
			args: args{
				name: "k8s_POD_mysql_default_98e2377b-f2ea-11e8-89af-6c92bf2cf061_0_extra",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testPrefix",
			args: args{
				name: "xxx_k8s_POD_mysql_default_98e2377b-f2ea-11e8-89af-6c92bf2cf061_0_extra",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testPrefixNotOK",
			args: args{
				name: "test_POD_mysql_default_98e2377b-f2ea-11e8-89af-6c92bf2cf061_0",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testOtherCase1",
			args: args{
				name: "test_POD_mysql__default_98e2377b-f2ea-11e8-89af-6c92bf2cf061_0",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testOtherCase2",
			args: args{
				name: "k8s_POD_mysql_def_ault_98e2377b-f2ea-11e8-89af-6c92bf2cf061_0",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testOtherCase3",
			args: args{
				name: "k8s_POD_mysql_default__98e2377b-f2ea-11e8-89af-6c92bf2cf061_0",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "testOtherCase4",
			args: args{
				name: "k8s_POD_mysql_default_xx888&&&_0",
			},
			want: &runtime.PodSandboxMetadata{
				Name:      "mysql",
				Namespace: "default",
				Uid:       "xx888&&&",
				Attempt:   uint32(0),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPodSandboxMetadataBySandboxName(tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPodSandboxMetadataBySandboxName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPodSandboxMetadataBySandboxName() = %v, want %v", got, tt.want)
			}
		})
	}
}
