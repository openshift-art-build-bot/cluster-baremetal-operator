/*

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

package provisioning

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsclientv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"
	metal3iov1alpha1 "github.com/openshift/cluster-baremetal-operator/api/v1alpha1"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
)

const (
	metal3AppName                    = "metal3"
	baremetalDeploymentName          = "metal3"
	baremetalSharedVolume            = "metal3-shared"
	metal3AuthRootDir                = "/auth"
	metal3TlsRootDir                 = "/certs"
	ironicCredentialsVolume          = "metal3-ironic-basic-auth"
	ironicrpcCredentialsVolume       = "metal3-ironic-rpc-basic-auth"
	inspectorCredentialsVolume       = "metal3-inspector-basic-auth"
	ironicTlsVolume                  = "metal3-ironic-tls"
	inspectorTlsVolume               = "metal3-inspector-tls"
	htpasswdEnvVar                   = "HTTP_BASIC_HTPASSWD" // #nosec
	mariadbPwdEnvVar                 = "MARIADB_PASSWORD"    // #nosec
	ironicInsecureEnvVar             = "IRONIC_INSECURE"
	inspectorInsecureEnvVar          = "IRONIC_INSPECTOR_INSECURE"
	ironicCertEnvVar                 = "IRONIC_CACERT_FILE"
	sshKeyEnvVar                     = "IRONIC_RAMDISK_SSH_KEY"
	externalIpEnvVar                 = "IRONIC_EXTERNAL_IP"
	cboOwnedAnnotation               = "baremetal.openshift.io/owned"
	cboLabelName                     = "baremetal.openshift.io/cluster-baremetal-operator"
	externalTrustBundleConfigMapName = "cbo-trusted-ca"
	pullSecretEnvVar                 = "IRONIC_AGENT_PULL_SECRET" // #nosec
)

var podTemplateAnnotations = map[string]string{
	"target.workload.openshift.io/management": `{"effect": "PreferredDuringScheduling"}`,
}

var deploymentRolloutStartTime = time.Now()
var deploymentRolloutTimeout = 5 * time.Minute

var sharedVolumeMount = corev1.VolumeMount{
	Name:      baremetalSharedVolume,
	MountPath: "/shared",
}

var ironicCredentialsMount = corev1.VolumeMount{
	Name:      ironicCredentialsVolume,
	MountPath: metal3AuthRootDir + "/ironic",
	ReadOnly:  true,
}

var rpcCredentialsMount = corev1.VolumeMount{
	Name:      ironicrpcCredentialsVolume,
	MountPath: metal3AuthRootDir + "/ironic-rpc",
	ReadOnly:  true,
}

var inspectorCredentialsMount = corev1.VolumeMount{
	Name:      inspectorCredentialsVolume,
	MountPath: metal3AuthRootDir + "/ironic-inspector",
	ReadOnly:  true,
}

var ironicTlsMount = corev1.VolumeMount{
	Name:      ironicTlsVolume,
	MountPath: metal3TlsRootDir + "/ironic",
	ReadOnly:  true,
}

var inspectorTlsMount = corev1.VolumeMount{
	Name:      inspectorTlsVolume,
	MountPath: metal3TlsRootDir + "/ironic-inspector",
	ReadOnly:  true,
}

var mariadbPassword = corev1.EnvVar{
	Name: mariadbPwdEnvVar,
	ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: baremetalSecretName,
			},
			Key: baremetalSecretKey,
		},
	},
}

var pullSecret = corev1.EnvVar{
	Name: pullSecretEnvVar,
	ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: pullSecretName,
			},
			Key: openshiftConfigSecretKey,
		},
	},
}

var metal3Volumes = []corev1.Volume{
	{
		Name: baremetalSharedVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	},
	imageVolume(),
	{
		Name: ironicCredentialsVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: ironicSecretName,
				Items: []corev1.KeyToPath{
					{Key: ironicUsernameKey, Path: ironicUsernameKey},
					{Key: ironicPasswordKey, Path: ironicPasswordKey},
					{Key: ironicConfigKey, Path: ironicConfigKey},
				},
			},
		},
	},
	{
		Name: ironicrpcCredentialsVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: ironicrpcSecretName,
				Items: []corev1.KeyToPath{
					{Key: ironicUsernameKey, Path: ironicUsernameKey},
					{Key: ironicPasswordKey, Path: ironicPasswordKey},
					{Key: ironicConfigKey, Path: ironicConfigKey},
				},
			},
		},
	},
	{
		Name: inspectorCredentialsVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: inspectorSecretName,
				Items: []corev1.KeyToPath{
					{Key: ironicUsernameKey, Path: ironicUsernameKey},
					{Key: ironicPasswordKey, Path: ironicPasswordKey},
					{Key: ironicConfigKey, Path: ironicConfigKey},
				},
			},
		},
	},
	{
		Name: "trusted-ca",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				Items: []corev1.KeyToPath{{Key: "ca-bundle.crt", Path: "tls-ca-bundle.pem"}},
				LocalObjectReference: corev1.LocalObjectReference{
					Name: externalTrustBundleConfigMapName,
				},
				Optional: pointer.BoolPtr(true),
			},
		},
	},
	{
		Name: ironicTlsVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: tlsSecretName,
			},
		},
	},
	{
		Name: inspectorTlsVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: tlsSecretName,
			},
		},
	},
}

func buildEnvVar(name string, baremetalProvisioningConfig *metal3iov1alpha1.ProvisioningSpec) corev1.EnvVar {
	value := getMetal3DeploymentConfig(name, baremetalProvisioningConfig)
	if value != nil {
		return corev1.EnvVar{
			Name:  name,
			Value: *value,
		}
	} else if name == provisioningIP && baremetalProvisioningConfig.ProvisioningNetwork == metal3iov1alpha1.ProvisioningNetworkDisabled {
		return corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		}
	}

	return corev1.EnvVar{
		Name: name,
	}
}

func setIronicHtpasswdHash(name string, secretName string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: ironicHtpasswdKey,
			},
		},
	}
}

func setIronicExternalIp(name string, config *metal3iov1alpha1.ProvisioningSpec) corev1.EnvVar {
	if config.ProvisioningNetwork != metal3iov1alpha1.ProvisioningNetworkDisabled && config.VirtualMediaViaExternalNetwork {
		return corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		}
	}
	return corev1.EnvVar{
		Name: name,
	}
}

func newMetal3InitContainers(info *ProvisioningInfo) []corev1.Container {
	initContainers := []corev1.Container{}

	// If the provisioning network is disabled, and the user hasn't requested a
	// particular provisioning IP on the machine CIDR, we have nothing for this container
	// to manage.
	if info.ProvConfig.Spec.ProvisioningIP != "" && info.ProvConfig.Spec.ProvisioningNetwork != metal3iov1alpha1.ProvisioningNetworkDisabled {
		initContainers = append(initContainers, createInitContainerStaticIpSet(info.Images, &info.ProvConfig.Spec))
	}

	// If the PreProvisioningOSDownloadURLs are set, we fetch the URLs of either CoreOS ISO and IPA assets or in some
	// cases only the IPA assets
	liveURLs := getPreProvisioningOSDownloadURLs(&info.ProvConfig.Spec)
	if len(liveURLs) > 0 {
		initContainers = append(initContainers, createInitContainerMachineOsDownloader(info, strings.Join(liveURLs, ","), true, true))

		// If the ISO URL is also specified, start the createInitContainerConfigureCoreOSIPA init container
		if info.ProvConfig.Spec.PreProvisioningOSDownloadURLs.IsoURL != "" {
			// Configure the LiveISO by embedding ignition and other startup files
			initContainers = append(initContainers, createInitContainerConfigureCoreOSIPA(info))
		}
	}
	// If the ProvisioningOSDownloadURL is set, we download the URL specified in it
	if info.ProvConfig.Spec.ProvisioningOSDownloadURL != "" {
		initContainers = append(initContainers, createInitContainerMachineOsDownloader(info, info.ProvConfig.Spec.ProvisioningOSDownloadURL, false, true))
	}

	// If the CoreOS IPA assets are not available we will use the IPA downloader
	if !isCoreOSIPAAvailable(&info.ProvConfig.Spec) {
		initContainers = append(initContainers, createInitContainerIpaDownloader(info.Images))
	}

	return injectProxyAndCA(initContainers, info.Proxy)
}

func createInitContainerIpaDownloader(images *Images) corev1.Container {
	initContainer := corev1.Container{
		Name:            "metal3-ipa-downloader",
		Image:           images.IpaDownloader,
		Command:         []string{"/usr/local/bin/get-resource.sh"},
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		VolumeMounts: []corev1.VolumeMount{imageVolumeMount},
		Env:          []corev1.EnvVar{},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}
	return initContainer
}

// This initContainer configures RHCOS Live ISO images by embedding the IPA
// agent ignition. See:
// https://github.com/openshift/ironic-image/blob/master/scripts/configure-coreos-ipa
func createInitContainerConfigureCoreOSIPA(info *ProvisioningInfo) corev1.Container {
	config := &info.ProvConfig.Spec
	initContainer := corev1.Container{
		Name:            "metal3-configure-coreos-ipa",
		Image:           info.Images.Ironic,
		Command:         []string{"/bin/configure-coreos-ipa"},
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolumeMount,
			imageVolumeMount,
			ironicCredentialsMount,
			ironicTlsMount,
		},
		Env: []corev1.EnvVar{
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
			buildSSHKeyEnvVar(info.SSHKey),
			pullSecret,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}
	initContainer.Env = envWithMasterMacAddresses(initContainer.Env, info.MasterMacAddresses)
	return initContainer
}

func ipOptionForMachineOsDownloader(info *ProvisioningInfo) string {
	var optionValue string
	switch info.NetworkStack {
	case NetworkStackV4:
		optionValue = "ip=dhcp"
	case NetworkStackV6:
		optionValue = "ip=dhcp6"
	case NetworkStackDual:
		optionValue = ""
	}
	return optionValue
}

func createInitContainerMachineOsDownloader(info *ProvisioningInfo, imageURLs string, useLiveImages, setIpOptions bool) corev1.Container {
	var command string
	if useLiveImages {
		command = "/usr/local/bin/get-live-images.sh"
	} else {
		command = "/usr/local/bin/get-resource.sh"
	}

	env := []corev1.EnvVar{
		{
			Name:  machineImageUrl,
			Value: imageURLs,
		},
	}
	if setIpOptions {
		env = append(env,
			corev1.EnvVar{
				Name:  ipOptions,
				Value: ipOptionForMachineOsDownloader(info),
			})
	}
	initContainer := corev1.Container{
		Name:            "metal3-machine-os-downloader",
		Image:           info.Images.MachineOsDownloader,
		Command:         []string{command},
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		VolumeMounts: []corev1.VolumeMount{imageVolumeMount},
		Env:          env,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}
	return initContainer
}

func createInitContainerStaticIpSet(images *Images, config *metal3iov1alpha1.ProvisioningSpec) corev1.Container {
	initContainer := corev1.Container{
		Name:            "metal3-static-ip-set",
		Image:           images.StaticIpManager,
		Command:         []string{"/set-static-ip"},
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Env: []corev1.EnvVar{
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}
	return initContainer
}

func newMetal3Containers(info *ProvisioningInfo) []corev1.Container {
	containers := []corev1.Container{
		createContainerMetal3BaremetalOperator(info.Images, &info.ProvConfig.Spec),
		createContainerMetal3Mariadb(info.Images),
		createContainerMetal3Httpd(info.Images, &info.ProvConfig.Spec, info.MasterMacAddresses, info.SSHKey),
		createContainerMetal3IronicConductor(info.Images, &info.ProvConfig.Spec, info.MasterMacAddresses, info.SSHKey),
		createContainerIronicInspectorRamdiskLogs(info.Images),
		createContainerMetal3IronicApi(info.Images, &info.ProvConfig.Spec, info.MasterMacAddresses),
		createContainerIronicDeployRamdiskLogs(info.Images),
		createContainerMetal3IronicInspector(info.Images, &info.ProvConfig.Spec, info.MasterMacAddresses),
	}

	// If the provisioning network is disabled, and the user hasn't requested a
	// particular provisioning IP on the machine CIDR, we have nothing for this container
	// to manage.
	if info.ProvConfig.Spec.ProvisioningIP != "" && info.ProvConfig.Spec.ProvisioningNetwork != metal3iov1alpha1.ProvisioningNetworkDisabled {
		containers = append(containers, createContainerMetal3StaticIpManager(info.Images, &info.ProvConfig.Spec, info.MasterMacAddresses))
	}

	if info.ProvConfig.Spec.ProvisioningNetwork != metal3iov1alpha1.ProvisioningNetworkDisabled {
		containers = append(containers, createContainerMetal3Dnsmasq(info.Images, &info.ProvConfig.Spec, info.MasterMacAddresses))
	}

	return injectProxyAndCA(containers, info.Proxy)
}

func getWatchNamespace(config *metal3iov1alpha1.ProvisioningSpec) corev1.EnvVar {
	if config.WatchAllNamespaces {
		return corev1.EnvVar{
			Name:  "WATCH_NAMESPACE",
			Value: "",
		}
	} else {
		return corev1.EnvVar{
			Name: "WATCH_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		}
	}
}

func buildSSHKeyEnvVar(sshKey string) corev1.EnvVar {
	return corev1.EnvVar{Name: sshKeyEnvVar, Value: sshKey}
}

func createContainerMetal3BaremetalOperator(images *Images, config *metal3iov1alpha1.ProvisioningSpec) corev1.Container {
	container := corev1.Container{
		Name:  "metal3-baremetal-operator",
		Image: images.BaremetalOperator,
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 60000,
				HostPort:      60000,
			},
		},
		Command:         []string{"/baremetal-operator"},
		Args:            []string{"--health-addr", ":9446"},
		ImagePullPolicy: "IfNotPresent",
		VolumeMounts: []corev1.VolumeMount{
			ironicCredentialsMount,
			inspectorCredentialsMount,
			ironicTlsMount,
		},
		Env: []corev1.EnvVar{
			getWatchNamespace(config),
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name:  "OPERATOR_NAME",
				Value: "baremetal-operator",
			},
			{
				Name:  ironicCertEnvVar,
				Value: metal3TlsRootDir + "/ironic/" + corev1.TLSCertKey,
			},
			{
				Name:  ironicInsecureEnvVar,
				Value: "true",
			},
			buildEnvVar(deployKernelUrl, config),
			buildEnvVar(deployRamdiskUrl, config),
			buildEnvVar(ironicEndpoint, config),
			buildEnvVar(ironicInspectorEndpoint, config),
			{
				Name:  "METAL3_AUTH_ROOT_DIR",
				Value: metal3AuthRootDir,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}
	return container
}

func createContainerMetal3Dnsmasq(images *Images, config *metal3iov1alpha1.ProvisioningSpec, macs []string) corev1.Container {
	container := corev1.Container{
		Name:            "metal3-dnsmasq",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command: []string{"/bin/rundnsmasq"},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolumeMount,
			imageVolumeMount,
		},
		Env: []corev1.EnvVar{
			buildEnvVar(httpPort, config),
			buildEnvVar(provisioningInterface, config),
			buildEnvVar(dhcpRange, config),
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("5m"),
				corev1.ResourceMemory: resource.MustParse("5Mi"),
			},
		},
	}

	container.Env = envWithMasterMacAddresses(container.Env, macs)

	return container
}

func envWithMasterMacAddresses(envVars []corev1.EnvVar, macs []string) []corev1.EnvVar {
	return append(envVars, corev1.EnvVar{
		Name:  "PROVISIONING_MACS",
		Value: strings.Join(macs, ","),
	})
}

func createContainerMetal3Mariadb(images *Images) corev1.Container {
	container := corev1.Container{
		Name:            "metal3-mariadb",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command:      []string{"/bin/runmariadb"},
		VolumeMounts: []corev1.VolumeMount{sharedVolumeMount},
		Env: []corev1.EnvVar{
			mariadbPassword,
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "mysql",
				ContainerPort: 3306,
				HostPort:      3306,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("15m"),
				corev1.ResourceMemory: resource.MustParse("80Mi"),
			},
		},
	}
	return container
}

func createContainerMetal3Httpd(images *Images, config *metal3iov1alpha1.ProvisioningSpec, macs []string, sshKey string) corev1.Container {
	port, _ := strconv.Atoi(baremetalHttpPort) // #nosec
	container := corev1.Container{
		Name:            "metal3-httpd",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command: []string{"/bin/runhttpd"},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolumeMount,
			imageVolumeMount,
			ironicTlsMount,
			inspectorTlsMount,
		},
		Env: []corev1.EnvVar{
			buildEnvVar(httpPort, config),
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
			buildSSHKeyEnvVar(sshKey),
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          httpPortName,
				ContainerPort: int32(port),
				HostPort:      int32(port),
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("5m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}

	container.Env = envWithMasterMacAddresses(container.Env, macs)

	return container
}

func createContainerMetal3IronicConductor(images *Images, config *metal3iov1alpha1.ProvisioningSpec, macs []string, sshKey string) corev1.Container {
	container := corev1.Container{
		Name:            "metal3-ironic-conductor",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command: []string{"/bin/runironic-conductor"},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolumeMount,
			imageVolumeMount,
			inspectorCredentialsMount,
			rpcCredentialsMount,
			ironicTlsMount,
			inspectorTlsMount,
		},
		Env: []corev1.EnvVar{
			mariadbPassword,
			{
				Name:  ironicInsecureEnvVar,
				Value: "true",
			},
			{
				Name:  inspectorInsecureEnvVar,
				Value: "true",
			},
			buildEnvVar(httpPort, config),
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
			buildSSHKeyEnvVar(sshKey),
			setIronicHtpasswdHash(htpasswdEnvVar, ironicrpcSecretName),
			setIronicExternalIp(externalIpEnvVar, config),
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "json-rpc",
				ContainerPort: 8089,
				HostPort:      8089,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	}

	container.Env = envWithMasterMacAddresses(container.Env, macs)

	return container
}

func createContainerMetal3IronicApi(images *Images, config *metal3iov1alpha1.ProvisioningSpec, macs []string) corev1.Container {
	container := corev1.Container{
		Name:            "metal3-ironic-api",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command: []string{"/bin/runironic-api"},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolumeMount,
			rpcCredentialsMount,
			ironicTlsMount,
		},
		Env: []corev1.EnvVar{
			mariadbPassword,
			{
				Name:  ironicInsecureEnvVar,
				Value: "true",
			},
			buildEnvVar(httpPort, config),
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
			setIronicHtpasswdHash(htpasswdEnvVar, ironicSecretName),
			setIronicExternalIp(externalIpEnvVar, config),
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "ironic",
				ContainerPort: 6385,
				HostPort:      6385,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("150m"),
				corev1.ResourceMemory: resource.MustParse("300Mi"),
			},
		},
	}

	container.Env = envWithMasterMacAddresses(container.Env, macs)

	return container
}

func createContainerIronicDeployRamdiskLogs(images *Images) corev1.Container {
	container := corev1.Container{
		Name:            "ironic-deploy-ramdisk-logs",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command:      []string{"/bin/runlogwatch.sh"},
		VolumeMounts: []corev1.VolumeMount{sharedVolumeMount},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("5Mi"),
			},
		},
	}
	return container
}

func createContainerMetal3IronicInspector(images *Images, config *metal3iov1alpha1.ProvisioningSpec, macs []string) corev1.Container {
	container := corev1.Container{
		Name:            "metal3-ironic-inspector",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command: []string{"/bin/runironic-inspector"},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolumeMount,
			ironicCredentialsMount,
			ironicTlsMount,
			inspectorTlsMount,
		},
		Env: []corev1.EnvVar{
			{
				Name:  ironicInsecureEnvVar,
				Value: "true",
			},
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
			setIronicHtpasswdHash(htpasswdEnvVar, inspectorSecretName),
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "inspector",
				ContainerPort: 5050,
				HostPort:      5050,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("40m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
	}

	container.Env = envWithMasterMacAddresses(container.Env, macs)

	return container
}

func createContainerIronicInspectorRamdiskLogs(images *Images) corev1.Container {
	container := corev1.Container{
		Name:            "ironic-inspector-ramdisk-logs",
		Image:           images.Ironic,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Command:      []string{"/bin/runlogwatch.sh"},
		VolumeMounts: []corev1.VolumeMount{sharedVolumeMount},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("5Mi"),
			},
		},
	}
	return container
}

func createContainerMetal3StaticIpManager(images *Images, config *metal3iov1alpha1.ProvisioningSpec, macs []string) corev1.Container {
	container := corev1.Container{
		Name:            "metal3-static-ip-manager",
		Image:           images.StaticIpManager,
		Command:         []string{"/refresh-static-ip"},
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.BoolPtr(true),
		},
		Env: []corev1.EnvVar{
			buildEnvVar(provisioningIP, config),
			buildEnvVar(provisioningInterface, config),
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("5m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	}

	container.Env = envWithMasterMacAddresses(container.Env, macs)

	return container
}

func newMetal3PodTemplateSpec(info *ProvisioningInfo, labels *map[string]string) *corev1.PodTemplateSpec {
	initContainers := newMetal3InitContainers(info)
	containers := newMetal3Containers(info)
	tolerations := []corev1.Toleration{
		{
			Key:      "node-role.kubernetes.io/master",
			Effect:   corev1.TaintEffectNoSchedule,
			Operator: corev1.TolerationOpExists,
		},
		{
			Key:      "CriticalAddonsOnly",
			Operator: corev1.TolerationOpExists,
		},
		{
			Key:               "node.kubernetes.io/not-ready",
			Effect:            corev1.TaintEffectNoExecute,
			Operator:          corev1.TolerationOpExists,
			TolerationSeconds: pointer.Int64Ptr(120),
		},
		{
			Key:               "node.kubernetes.io/unreachable",
			Effect:            corev1.TaintEffectNoExecute,
			Operator:          corev1.TolerationOpExists,
			TolerationSeconds: pointer.Int64Ptr(120),
		},
	}

	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: podTemplateAnnotations,
			Labels:      *labels,
		},
		Spec: corev1.PodSpec{
			Volumes:           metal3Volumes,
			InitContainers:    initContainers,
			Containers:        containers,
			HostNetwork:       true,
			DNSPolicy:         corev1.DNSClusterFirstWithHostNet,
			PriorityClassName: "system-node-critical",
			NodeSelector:      map[string]string{"node-role.kubernetes.io/master": ""},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: pointer.BoolPtr(false),
			},
			ServiceAccountName: "cluster-baremetal-operator",
			Tolerations:        tolerations,
		},
	}
}

func mountsWithTrustedCA(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	mounts = append(mounts, corev1.VolumeMount{
		MountPath: "/etc/pki/ca-trust/extracted/pem",
		Name:      "trusted-ca",
		ReadOnly:  true,
	})

	return mounts
}

func injectProxyAndCA(containers []corev1.Container, proxy *configv1.Proxy) []corev1.Container {
	var injectedContainers []corev1.Container

	for _, container := range containers {
		container.Env = envWithProxy(proxy, container.Env)
		container.VolumeMounts = mountsWithTrustedCA(container.VolumeMounts)
		injectedContainers = append(injectedContainers, container)
	}

	return injectedContainers
}

func envWithProxy(proxy *configv1.Proxy, envVars []corev1.EnvVar) []corev1.EnvVar {
	if proxy == nil {
		return envVars
	}

	if proxy.Status.HTTPProxy != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "HTTP_PROXY",
			Value: proxy.Status.HTTPProxy,
		})
	}
	if proxy.Status.HTTPSProxy != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "HTTPS_PROXY",
			Value: proxy.Status.HTTPSProxy,
		})
	}
	if proxy.Status.NoProxy != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "NO_PROXY",
			Value: proxy.Status.NoProxy,
		})
	}

	return envVars
}

func newMetal3Deployment(info *ProvisioningInfo) *appsv1.Deployment {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k8s-app":    metal3AppName,
			cboLabelName: stateService,
		},
	}
	podSpecLabels := map[string]string{
		"k8s-app":    metal3AppName,
		cboLabelName: stateService,
	}
	template := newMetal3PodTemplateSpec(info, &podSpecLabels)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baremetalDeploymentName,
			Namespace: info.Namespace,
			Annotations: map[string]string{
				cboOwnedAnnotation: "",
			},
			Labels: map[string]string{
				"k8s-app":    metal3AppName,
				cboLabelName: stateService,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: selector,
			Template: *template,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
	}
}

func getMetal3DeploymentSelector(client appsclientv1.DeploymentsGetter, targetNamespace string) (*metav1.LabelSelector, error) {
	existing, err := client.Deployments(targetNamespace).Get(context.Background(), baremetalDeploymentName, metav1.GetOptions{})
	if existing != nil && err == nil {
		return existing.Spec.Selector, nil
	}
	return nil, err
}

func EnsureMetal3Deployment(info *ProvisioningInfo) (updated bool, err error) {
	// Create metal3 deployment object based on current baremetal configuration
	// It will be created with the cboOwnedAnnotation

	metal3Deployment := newMetal3Deployment(info)
	expectedGeneration := resourcemerge.ExpectedDeploymentGeneration(metal3Deployment, info.ProvConfig.Status.Generations)

	err = controllerutil.SetControllerReference(info.ProvConfig, metal3Deployment, info.Scheme)
	if err != nil {
		err = fmt.Errorf("unable to set controllerReference on deployment: %w", err)
		return
	}

	deploymentRolloutStartTime = time.Now()
	deployment, updated, err := resourceapply.ApplyDeployment(info.Client.AppsV1(),
		info.EventRecorder, metal3Deployment, expectedGeneration)
	if err != nil {
		err = fmt.Errorf("unable to apply Metal3 deployment: %w", err)
		// Check if ApplyDeployment failed because the existing Pod had an outdated
		// Pod Selector.
		selector, get_err := getMetal3DeploymentSelector(info.Client.AppsV1(), info.Namespace)
		if get_err != nil || equality.Semantic.DeepEqual(selector, metal3Deployment.Spec.Selector) {
			return
		}
		// This is an older deployment with the incorrect Pod Selector.
		// Delete deployment now and re-create in the next reconcile.
		// The operator is watching deployments so the reconcile should be triggered when metal3 deployment
		// is deleted.
		if delete_err := DeleteMetal3Deployment(info); delete_err != nil {
			err = fmt.Errorf("unable to delete Metal3 deployment with incorrect Pod Selector: %w", delete_err)
			return
		}
	}
	if updated {
		resourcemerge.SetDeploymentGeneration(&info.ProvConfig.Status.Generations, deployment)
	}
	return updated, nil
}

func getDeploymentCondition(deployment *appsv1.Deployment) appsv1.DeploymentConditionType {
	for _, cond := range deployment.Status.Conditions {
		if cond.Status == corev1.ConditionTrue {
			return cond.Type
		}
	}
	return appsv1.DeploymentProgressing
}

// Provide the current state of metal3 deployment
func GetDeploymentState(client appsclientv1.DeploymentsGetter, targetNamespace string, config *metal3iov1alpha1.Provisioning) (appsv1.DeploymentConditionType, error) {
	existing, err := client.Deployments(targetNamespace).Get(context.Background(), baremetalDeploymentName, metav1.GetOptions{})
	if err != nil || existing == nil {
		// There were errors accessing the deployment.
		return appsv1.DeploymentReplicaFailure, err
	}
	deploymentState := getDeploymentCondition(existing)
	if deploymentState == appsv1.DeploymentProgressing && deploymentRolloutTimeout <= time.Since(deploymentRolloutStartTime) {
		return appsv1.DeploymentReplicaFailure, nil
	}
	return deploymentState, nil
}

func DeleteMetal3Deployment(info *ProvisioningInfo) error {
	return client.IgnoreNotFound(info.Client.AppsV1().Deployments(info.Namespace).Delete(context.Background(), baremetalDeploymentName, metav1.DeleteOptions{}))
}
