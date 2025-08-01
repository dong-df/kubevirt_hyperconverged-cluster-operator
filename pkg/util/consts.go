package util

// HCO common constants
const (
	OperatorNamespaceEnv               = "OPERATOR_NAMESPACE"
	OperatorWebhookModeEnv             = "WEBHOOK_MODE"
	ContainerAppName                   = "APP"
	ContainerOperatorApp               = "OPERATOR"
	ContainerWebhookApp                = "WEBHOOK"
	HcoKvIoVersionName                 = "HCO_KV_IO_VERSION"
	KubevirtVersionEnvV                = "KUBEVIRT_VERSION"
	KvVirtLauncherOSVersionEnvV        = "VIRT_LAUNCHER_OS_VERSION"
	CdiVersionEnvV                     = "CDI_VERSION"
	CnaoVersionEnvV                    = "NETWORK_ADDONS_VERSION"
	SspVersionEnvV                     = "SSP_VERSION"
	HppoVersionEnvV                    = "HPPO_VERSION"
	AaqVersionEnvV                     = "AAQ_VERSION"
	KVUIPluginImageEnvV                = "KV_CONSOLE_PLUGIN_IMAGE"
	KVUIProxyImageEnvV                 = "KV_CONSOLE_PROXY_IMAGE"
	PasstImageEnvV                     = "PASST_SIDECAR_IMAGE"
	PasstCNIImageEnvV                  = "PASST_CNI_IMAGE"
	HcoValidatingWebhook               = "validate-hco.kubevirt.io"
	HcoMutatingWebhookNS               = "mutate-ns-hco.kubevirt.io"
	PrometheusRuleCRDName              = "prometheusrules.monitoring.coreos.com"
	ServiceMonitorCRDName              = "servicemonitors.monitoring.coreos.com"
	DeschedulerCRDName                 = "kubedeschedulers.operator.openshift.io"
	NetworkAttachmentDefinitionCRDName = "network-attachment-definitions.k8s.cni.cncf.io"
	HcoMutatingWebhookHyperConverged   = "mutate-hyperconverged-hco.kubevirt.io"
	AppLabel                           = "app"
	UndefinedNamespace                 = ""
	OpenshiftNamespace                 = "openshift"
	APIVersionAlpha                    = "v1alpha1"
	APIVersionBeta                     = "v1beta1"
	CurrentAPIVersion                  = APIVersionBeta
	APIVersionGroup                    = "hco.kubevirt.io"
	APIVersion                         = APIVersionGroup + "/" + CurrentAPIVersion
	HyperConvergedKind                 = "HyperConverged"
	// Recommended labels by Kubernetes. See
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
	AppLabelPrefix    = "app.kubernetes.io"
	AppLabelVersion   = AppLabelPrefix + "/version"
	AppLabelManagedBy = AppLabelPrefix + "/managed-by"
	AppLabelPartOf    = AppLabelPrefix + "/part-of"
	AppLabelComponent = AppLabelPrefix + "/component"
	// Operator name for managed-by label
	OperatorName = "hco-operator"
	// Value for "part-of" label
	HyperConvergedCluster    = "hyperconverged-cluster"
	OpenshiftNodeSelectorAnn = "openshift.io/node-selector"
	KubernetesMetadataName   = "kubernetes.io/metadata.name"

	// PrometheusNSLabel is the monitoring NS enable label, if the value is "true"
	PrometheusNSLabel = "openshift.io/cluster-monitoring"

	// HyperConvergedName is the name of the HyperConverged resource that will be reconciled
	HyperConvergedName           = "kubevirt-hyperconverged"
	MetricsHost                  = "0.0.0.0"
	MetricsPort            int32 = 8443
	MetricsPortName              = "metrics"
	HealthProbeHost              = "0.0.0.0"
	HealthProbePort        int32 = 6060
	ReadinessEndpointName        = "/readyz"
	LivenessEndpointName         = "/livez"
	HCOWebhookPath               = "/validate-hco-kubevirt-io-v1beta1-hyperconverged"
	HCOMutatingWebhookPath       = "/mutate-hco-kubevirt-io-v1beta1-hyperconverged"
	HCONSWebhookPath             = "/mutate-ns-hco-kubevirt-io"
	WebhookPort                  = 4343
	WebhookPortName              = "webhook"

	WebhookCertName       = "apiserver.crt"
	WebhookKeyName        = "apiserver.key"
	DefaultWebhookCertDir = "/apiserver.local.config/certificates"

	CliDownloadsServerPort int32 = 8080
	UIPluginServerPort     int32 = 9443
	UIProxyServerPort      int32 = 8080

	APIServerCRName      = "cluster"
	DeschedulerCRName    = "cluster"
	DeschedulerNamespace = "openshift-kube-descheduler-operator"

	DataImportCronEnabledAnnotation = "dataimportcrontemplate.kubevirt.io/enable"

	HCOAnnotationPrefix = "hco.kubevirt.io/"

	// AllowEgressToDNSAndAPIServerLabel if this label is set, the network policy will allow egress to DNS and API server
	AllowEgressToDNSAndAPIServerLabel = HCOAnnotationPrefix + "allow-access-cluster-services"
	// AllowIngressToMetricsEndpointLabel if this label is set, the network policy will allow ingress to the metrics endpoint
	AllowIngressToMetricsEndpointLabel = HCOAnnotationPrefix + "allow-prometheus-access"
)

type AppComponent string

const (
	AppComponentCompute    AppComponent = "compute"
	AppComponentStorage    AppComponent = "storage"
	AppComponentNetwork    AppComponent = "network"
	AppComponentMonitoring AppComponent = "monitoring"
	AppComponentSchedule   AppComponent = "schedule"
	AppComponentDeployment AppComponent = "deployment"
	AppComponentUIPlugin   AppComponent = "kubevirt-console-plugin"
	AppComponentUIProxy    AppComponent = "kubevirt-apiserver-proxy"
	AppComponentUIConfig   AppComponent = "kubevirt-ui-config"
	AppComponentQuotaMngt  AppComponent = "quota-management"
)
