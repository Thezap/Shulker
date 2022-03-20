package resource

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	shulkermciov1alpha1 "shulkermc.io/m/v2/api/v1alpha1"
	common "shulkermc.io/m/v2/internal/resource"
)

type ProxyDeploymentResourceBuilder struct {
	Instance *shulkermciov1alpha1.ProxyDeployment
	Cluster  *shulkermciov1alpha1.MinecraftCluster
	Scheme   *runtime.Scheme
}

func (b *ProxyDeploymentResourceBuilder) ResourceBuilders() ([]common.ResourceBuilder, []common.ResourceBuilder) {
	builders := []common.ResourceBuilder{
		b.ProxyDeploymentDeployment(),
		b.ProxyDeploymentConfigMap(),
		b.ProxyDeploymentService(),
		b.ProxyDeploymentServiceAccount(),
		b.ProxyDeploymentRoleBinding(),
	}
	dirtyBuilders := []common.ResourceBuilder{}

	if b.Instance.Spec.DisruptionBudget.Enabled {
		builders = append(builders, b.ProxyDeploymentPodDisruptionBudget())
	} else {
		dirtyBuilders = append(dirtyBuilders, b.ProxyDeploymentPodDisruptionBudget())
	}

	return builders, dirtyBuilders
}

func (b *ProxyDeploymentResourceBuilder) getResourcePrefix() string {
	return b.Instance.Spec.ClusterRef.Name
}

func (b *ProxyDeploymentResourceBuilder) GetDeploymentName() string {
	return fmt.Sprintf("%s-proxy-%s", b.getResourcePrefix(), b.Instance.Name)
}

func (b *ProxyDeploymentResourceBuilder) getConfigMapName() string {
	return fmt.Sprintf("%s-proxy-config-%s", b.getResourcePrefix(), b.Instance.Name)
}

func (b *ProxyDeploymentResourceBuilder) getServiceName() string {
	return fmt.Sprintf("%s-proxy-%s", b.getResourcePrefix(), b.Instance.Name)
}

func (b *ProxyDeploymentResourceBuilder) getServiceAccountName() string {
	return fmt.Sprintf("%s-proxy-%s", b.getResourcePrefix(), b.Instance.Name)
}

func (b *ProxyDeploymentResourceBuilder) getRoleBindingName() string {
	return fmt.Sprintf("%s-proxy-%s", b.getResourcePrefix(), b.Instance.Name)
}

func (b *ProxyDeploymentResourceBuilder) getPodDisruptionBudgetName() string {
	return fmt.Sprintf("%s-proxy-%s", b.getResourcePrefix(), b.Instance.Name)
}

func (b *ProxyDeploymentResourceBuilder) GetPodSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: b.getLabels(),
	}
}

func (b *ProxyDeploymentResourceBuilder) getLabels() map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":             b.Instance.Name,
		"app.kubernetes.io/component":        "proxy",
		"app.kubernetes.io/part-of":          b.Instance.Spec.ClusterRef.Name,
		"app.kubernetes.io/created-by":       "shulker-operator",
		"minecraftcluster.shulkermc.io/name": b.Instance.Spec.ClusterRef.Name,
		"proxydeployment.shulkermc.io/name":  b.Instance.Name,
	}

	return labels
}
