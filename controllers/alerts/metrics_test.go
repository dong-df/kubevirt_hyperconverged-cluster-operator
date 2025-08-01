package alerts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevirt/hyperconverged-cluster-operator/controllers/common"
	"github.com/kubevirt/hyperconverged-cluster-operator/controllers/commontestutils"
	"github.com/kubevirt/hyperconverged-cluster-operator/pkg/monitoring/hyperconverged/metrics"
	"github.com/kubevirt/hyperconverged-cluster-operator/pkg/monitoring/hyperconverged/rules"
	hcoutil "github.com/kubevirt/hyperconverged-cluster-operator/pkg/util"
)

func TestAlerts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerts Suite")
}

var _ = Describe("alert tests", func() {
	var (
		ci            = commontestutils.ClusterInfoMock{}
		ee            = commontestutils.NewEventEmitterMock()
		ns            *corev1.Namespace
		req           *common.HcoRequest
		currentMetric float64
	)

	BeforeEach(func() {
		ee.Reset()
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: commontestutils.Namespace,
			},
		}

		req = commontestutils.NewReq(nil)
	})

	Context("test reconciler", func() {

		expectedEvents := []commontestutils.MockEvent{
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Created",
				Msg:       "Created PrometheusRule " + ruleName,
			},
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Created",
				Msg:       "Created Role " + roleName,
			},
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Created",
				Msg:       "Created RoleBinding " + roleName,
			},
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Created",
				Msg:       "Created Service " + serviceName,
			},
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Created",
				Msg:       "Created ServiceMonitor " + serviceName,
			},
		}

		It("should create all the resources if missing", func() {
			cl := commontestutils.InitClient([]client.Object{ns})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())

			hco := commontestutils.NewHco()
			req = commontestutils.NewReq(hco)
			Expect(r.UpdateRelatedObjects(req)).To(Succeed())
			Expect(req.StatusDirty).To(BeTrue())
			Expect(hco.Status.RelatedObjects).To(HaveLen(6))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
		})

		It("should not create network policy if the pod is without the np labels", func() {
			cl := commontestutils.InitClient([]client.Object{ns})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			np := &networkingv1.NetworkPolicy{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: policyName}, np)).To(MatchError(k8serrors.IsNotFound, "should return NotFound error"))
		})

		It("should create network policy if the pod is with the np labels", func() {
			pod := ci.GetPod()
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			pod.Labels[hcoutil.AllowEgressToDNSAndAPIServerLabel] = "true"
			pod.Labels[hcoutil.AllowIngressToMetricsEndpointLabel] = "true"

			DeferCleanup(func() {
				delete(pod.Labels, hcoutil.AllowEgressToDNSAndAPIServerLabel)
				delete(pod.Labels, hcoutil.AllowIngressToMetricsEndpointLabel)
			})

			cl := commontestutils.InitClient([]client.Object{ns})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			np := &networkingv1.NetworkPolicy{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: policyName}, np)).To(Succeed())

			hco := commontestutils.NewHco()
			req = commontestutils.NewReq(hco)
			Expect(r.UpdateRelatedObjects(req)).To(Succeed())
			Expect(req.StatusDirty).To(BeTrue())
			Expect(hco.Status.RelatedObjects).To(HaveLen(7))

			Expect(np.Spec.Egress).To(HaveLen(1))
			Expect(np.Spec.Egress[0].To).To(HaveLen(1))
			Expect(np.Spec.Egress[0].To[0].NamespaceSelector).ToNot(BeNil())
			Expect(np.Spec.Egress[0].To[0].NamespaceSelector.MatchLabels).To(HaveKeyWithValue("kubernetes.io/metadata.name", "openshift-monitoring"))

			expectedEventsWithNP := append(expectedEvents, commontestutils.MockEvent{
				EventType: corev1.EventTypeNormal,
				Reason:    "Created",
				Msg:       "Created NetworkPolicy " + policyName,
			})
			Expect(ee.CheckEvents(expectedEventsWithNP)).To(BeTrue())
		})

		It("should fail on error", func() {
			cl := commontestutils.InitClient([]client.Object{ns})
			fakeError := fmt.Errorf("fake error")
			cl.InitiateCreateErrors(func(obj client.Object) error {
				if obj.GetObjectKind().GroupVersionKind().Kind == "Service" {
					return fakeError
				}
				return nil
			})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			err := r.Reconcile(req, false)
			Expect(err).To(MatchError(fakeError))
		})
	})

	Context("test PrometheusRule", func() {
		BeforeEach(func() {
			currentMetric, _ = metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)

			err := rules.SetupRules()
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.Unsetenv(runbookURLTemplateEnv)
			Expect(err).ToNot(HaveOccurred())
		})

		expectedEvents := []commontestutils.MockEvent{
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Updated",
				Msg:       "Updated PrometheusRule " + ruleName,
			},
		}

		It("should update the labels if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			existRule.Labels = map[string]string{
				"wrongKey1": "wrongValue1",
				"wrongKey2": "wrongValue2",
				"wrongKey3": "wrongValue3",
			}

			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())

			Expect(pr.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, commontestutils.KeysFromSSMap(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring))))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should add the labels if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			existRule.Labels = nil

			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())

			Expect(pr.Labels).To(Equal(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified", func() {
			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(pr.OwnerReferences).To(HaveLen(1))
			Expect(pr.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(pr.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(pr.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(pr.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified; not HCO triggered", func() {

			req.HCOTriggered = false
			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(pr.OwnerReferences).To(HaveLen(1))
			Expect(pr.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(pr.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(pr.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(pr.OwnerReferences[0].UID).To(Equal(deployment.UID))

			overrideExpectedEvents := []commontestutils.MockEvent{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    "Overwritten",
					Msg:       "Overwritten PrometheusRule " + ruleName,
				},
			}

			Expect(ee.CheckEvents(overrideExpectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric + 1))
		})

		It("should update the referenceOwner if missing", func() {
			owner := metav1.OwnerReference{}
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			existRule.OwnerReferences = nil
			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(pr.OwnerReferences).To(HaveLen(1))
			Expect(pr.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(pr.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(pr.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(pr.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the spec if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())

			existRule.Spec.Groups[1].Rules = []monitoringv1.Rule{
				existRule.Spec.Groups[1].Rules[0],
				existRule.Spec.Groups[1].Rules[2],
				existRule.Spec.Groups[1].Rules[3],
			}
			// modify the first rule
			existRule.Spec.Groups[1].Rules[0].Alert = "modified alert"

			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())
			newRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			Expect(pr.Spec).To(Equal(newRule.Spec))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the spec if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())

			existRule.Spec = monitoringv1.PrometheusRuleSpec{}

			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())
			newRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			Expect(pr.Spec).To(Equal(newRule.Spec))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should use the default runbook URL template when no ENV Variable is set", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			promRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())

			for _, group := range promRule.Spec.Groups {
				for _, rule := range group.Rules {
					if rule.Alert != "" {
						if rule.Annotations["runbook_url"] != "" {
							Expect(rule.Annotations["runbook_url"]).To(Equal(fmt.Sprintf(defaultRunbookURLTemplate, rule.Alert)))
						}
					}
				}
			}
		})

		It("should use the desired runbook URL template when its ENV Variable is set", func() {
			desiredRunbookURLTemplate := "desired/runbookURL/template/%s"
			os.Setenv(runbookURLTemplateEnv, desiredRunbookURLTemplate)

			err := rules.SetupRules()
			Expect(err).ToNot(HaveOccurred())

			owner := getDeploymentReference(ci.GetDeployment())
			promRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())

			for _, group := range promRule.Spec.Groups {
				for _, rule := range group.Rules {
					if rule.Alert != "" {
						if rule.Annotations["runbook_url"] != "" {
							Expect(rule.Annotations["runbook_url"]).To(Equal(fmt.Sprintf(desiredRunbookURLTemplate, rule.Alert)))
						}
					}
				}
			}
		})

		DescribeTable("test the OverwrittenModificationsCount", func(hcoTriggered, upgradeMode, firstLoop bool, expectedCountDelta float64) {
			req.HCOTriggered = hcoTriggered
			req.UpgradeMode = upgradeMode

			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRule, err := rules.BuildPrometheusRule(commontestutils.Namespace, owner)
			Expect(err).ToNot(HaveOccurred())
			cl := commontestutils.InitClient([]client.Object{ns, existRule})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, firstLoop)).To(Succeed())
			pr := &monitoringv1.PrometheusRule{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: ruleName}, pr)).To(Succeed())

			Expect(metrics.GetOverwrittenModificationsCount(monitoringv1.PrometheusRuleKind, ruleName)).To(BeEquivalentTo(currentMetric + expectedCountDelta))
		},
			Entry("should not increase the counter if it HCO triggered, in upgrade mode and in the first loop", true, true, true, float64(0)), // can't really happen
			Entry("should not increase the counter if it HCO triggered, not in upgrade mode but in the first loop", true, false, true, float64(0)),
			Entry("should not increase the counter if it HCO triggered, in upgrade mode but not in the first loop", true, true, false, float64(0)),
			Entry("should not increase the counter if it HCO triggered, not in upgrade mode and not in the first loop", true, false, false, float64(0)),

			Entry("should not increase the counter if it not HCO triggered, in upgrade mode and in the first loop", false, true, true, float64(0)), // can't really happen
			Entry("should not increase the counter if it not HCO triggered, not in upgrade mode but in the first loop", false, false, true, float64(0)),
			Entry("should not increase the counter if it not HCO triggered, in upgrade mode and not in the first loop", false, true, false, float64(0)),
			Entry("should increase the counter if it not HCO triggered, not in upgrade mode and not in the first loop", false, false, false, float64(1)),
		)
	})

	Context("test Role", func() {
		BeforeEach(func() {
			currentMetric, _ = metrics.GetOverwrittenModificationsCount("Role", roleName)
		})

		expectedEvents := []commontestutils.MockEvent{
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Updated",
				Msg:       "Updated Role " + roleName,
			},
		}

		It("should update the labels if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRole := newRole(owner, commontestutils.Namespace)
			existRole.Labels = map[string]string{
				"wrongKey1": "wrongValue1",
				"wrongKey2": "wrongValue2",
				"wrongKey3": "wrongValue3",
			}

			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())

			Expect(role.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, commontestutils.KeysFromSSMap(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring))))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the labels if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRole := newRole(owner, commontestutils.Namespace)
			existRole.Labels = nil

			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())

			Expect(role.Labels).To(Equal(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified", func() {
			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRole := newRole(owner, commontestutils.Namespace)
			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(role.OwnerReferences).To(HaveLen(1))
			Expect(role.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(role.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(role.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(role.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified; not HCO triggered", func() {
			req.HCOTriggered = false

			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRole := newRole(owner, commontestutils.Namespace)
			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(role.OwnerReferences).To(HaveLen(1))
			Expect(role.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(role.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(role.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(role.OwnerReferences[0].UID).To(Equal(deployment.UID))

			overrideExpectedEvents := []commontestutils.MockEvent{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    "Overwritten",
					Msg:       "Overwritten Role " + roleName,
				},
			}

			Expect(ee.CheckEvents(overrideExpectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric + 1))
		})

		It("should update the referenceOwner if missing", func() {
			owner := metav1.OwnerReference{}
			existRole := newRole(owner, commontestutils.Namespace)
			existRole.OwnerReferences = nil
			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(role.OwnerReferences).To(HaveLen(1))
			Expect(role.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(role.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(role.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(role.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Rules if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRole := newRole(owner, commontestutils.Namespace)

			existRole.Rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{"wrongGroup1"},
					Resources: []string{"wrongResource1", "wrongResource2", "wrongResource3", "wrongResource4"},
					Verbs:     []string{"list", "update"},
				},
				{
					APIGroups: []string{"wrongGroup2"},
					Verbs:     []string{"list", "update", "help"},
				},
			}

			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())
			Expect(role.Rules).To(HaveLen(1))
			Expect(role.Rules[0].APIGroups).To(Equal([]string{""}))
			Expect(role.Rules[0].Resources).To(Equal([]string{"services", "endpoints", "pods"}))
			Expect(role.Rules[0].Verbs).To(Equal([]string{"get", "list", "watch"}))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Rules if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRole := newRole(owner, commontestutils.Namespace)

			existRole.Rules = nil

			cl := commontestutils.InitClient([]client.Object{ns, existRole})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			role := &rbacv1.Role{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, role)).To(Succeed())
			Expect(role.Rules).To(HaveLen(1))
			Expect(role.Rules[0].APIGroups).To(Equal([]string{""}))
			Expect(role.Rules[0].Resources).To(Equal([]string{"services", "endpoints", "pods"}))
			Expect(role.Rules[0].Verbs).To(Equal([]string{"get", "list", "watch"}))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Role", roleName)).To(BeEquivalentTo(currentMetric))
		})
	})

	Context("test RoleBinding", func() {
		BeforeEach(func() {
			currentMetric, _ = metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)
		})

		expectedEvents := []commontestutils.MockEvent{
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Updated",
				Msg:       "Updated RoleBinding " + roleName,
			},
		}

		It("should update the labels if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)
			existRB.Labels = map[string]string{
				"wrongKey1": "wrongValue1",
				"wrongKey2": "wrongValue2",
				"wrongKey3": "wrongValue3",
			}

			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())

			Expect(rb.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, commontestutils.KeysFromSSMap(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring))))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the labels if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)
			existRB.Labels = nil

			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())

			Expect(rb.Labels).To(Equal(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified", func() {
			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)
			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(rb.OwnerReferences).To(HaveLen(1))
			Expect(rb.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(rb.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(rb.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(rb.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified; not HCO triggered", func() {
			req.HCOTriggered = false

			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)
			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(rb.OwnerReferences).To(HaveLen(1))
			Expect(rb.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(rb.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(rb.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(rb.OwnerReferences[0].UID).To(Equal(deployment.UID))

			overrideExpectedEvents := []commontestutils.MockEvent{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    "Overwritten",
					Msg:       "Overwritten RoleBinding " + roleName,
				},
			}

			Expect(ee.CheckEvents(overrideExpectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric + 1))
		})

		It("should update the referenceOwner if missing", func() {
			owner := metav1.OwnerReference{}
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)
			existRB.OwnerReferences = nil
			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(rb.OwnerReferences).To(HaveLen(1))
			Expect(rb.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(rb.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(rb.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(rb.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the RoleRef if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)

			existRB.RoleRef = rbacv1.RoleRef{
				APIGroup: "wrongAPIGroup",
				Kind:     "wrongKind",
				Name:     "wrongName",
			}

			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())
			Expect(rb.RoleRef.APIGroup).To(Equal(rbacv1.GroupName))
			Expect(rb.RoleRef.Kind).To(Equal("Role"))
			Expect(rb.RoleRef.Name).To(Equal(roleName))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the RoleRef if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)

			existRB.RoleRef = rbacv1.RoleRef{}

			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())
			Expect(rb.RoleRef.APIGroup).To(Equal(rbacv1.GroupName))
			Expect(rb.RoleRef.Kind).To(Equal("Role"))
			Expect(rb.RoleRef.Name).To(Equal(roleName))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Subjects if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)

			existRB.Subjects = []rbacv1.Subject{
				{
					Kind:      "wrongKind1",
					Name:      "wrongName1",
					Namespace: "wrongNamespace1",
				},
				{
					Kind:      "wrongKind2",
					Name:      "wrongName2",
					Namespace: "wrongNamespace2",
				},
			}

			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
			Expect(rb.Subjects[0].Name).To(Equal("prometheus-k8s"))
			Expect(rb.Subjects[0].Namespace).To(Equal(getMonitoringNamespace(ci)))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Subjects if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existRB := newRoleBinding(owner, commontestutils.Namespace, ci)

			existRB.Subjects = nil

			cl := commontestutils.InitClient([]client.Object{ns, existRB})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			rb := &rbacv1.RoleBinding{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: roleName}, rb)).To(Succeed())
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
			Expect(rb.Subjects[0].Name).To(Equal("prometheus-k8s"))
			Expect(rb.Subjects[0].Namespace).To(Equal(getMonitoringNamespace(ci)))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("RoleBinding", roleName)).To(BeEquivalentTo(currentMetric))
		})
	})

	Context("test Service", func() {
		BeforeEach(func() {
			currentMetric, _ = metrics.GetOverwrittenModificationsCount("Service", serviceName)
		})

		expectedEvents := []commontestutils.MockEvent{
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Updated",
				Msg:       "Updated Service " + serviceName,
			},
		}

		It("should update the labels if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewMetricsService(commontestutils.Namespace, owner)
			existSM.Labels = map[string]string{
				"wrongKey1": "wrongValue1",
				"wrongKey2": "wrongValue2",
				"wrongKey3": "wrongValue3",
			}

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())

			Expect(svc.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, commontestutils.KeysFromSSMap(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring))))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the labels if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewMetricsService(commontestutils.Namespace, owner)
			existSM.Labels = nil

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())

			Expect(svc.Labels).To(Equal(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified", func() {
			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existSM := NewMetricsService(commontestutils.Namespace, owner)
			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(svc.OwnerReferences).To(HaveLen(1))
			Expect(svc.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(svc.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(svc.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(svc.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified; No HCO triggered", func() {
			req.HCOTriggered = false

			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existSM := NewMetricsService(commontestutils.Namespace, owner)
			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(svc.OwnerReferences).To(HaveLen(1))
			Expect(svc.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(svc.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(svc.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(svc.OwnerReferences[0].UID).To(Equal(deployment.UID))

			overrideExpectedEvents := []commontestutils.MockEvent{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    "Overwritten",
					Msg:       "Overwritten Service " + serviceName,
				},
			}

			Expect(ee.CheckEvents(overrideExpectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric + 1))
		})

		It("should update the referenceOwner if missing", func() {
			owner := metav1.OwnerReference{}
			existSM := NewMetricsService(commontestutils.Namespace, owner)
			existSM.OwnerReferences = nil
			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(svc.OwnerReferences).To(HaveLen(1))
			Expect(svc.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(svc.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(svc.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(svc.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Spec if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewMetricsService(commontestutils.Namespace, owner)

			existSM.Spec = corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port:     1234,
						Name:     "wrongName",
						Protocol: corev1.ProtocolUDP,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 1234,
						},
					},
				},
				Selector: map[string]string{
					"wrongKey1": "wrongValue1",
					"wrongKey2": "wrongValue2",
				},
			}

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(hcoutil.MetricsPort))
			Expect(svc.Spec.Ports[0].Name).To(Equal(operatorPortName))
			Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.IntOrString{Type: intstr.Int, IntVal: hcoutil.MetricsPort}))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Spec if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewMetricsService(commontestutils.Namespace, owner)

			existSM.Spec = corev1.ServiceSpec{}

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			svc := &corev1.Service{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, svc)).To(Succeed())
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(hcoutil.MetricsPort))
			Expect(svc.Spec.Ports[0].Name).To(Equal(operatorPortName))
			Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.IntOrString{Type: intstr.Int, IntVal: hcoutil.MetricsPort}))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("Service", serviceName)).To(BeEquivalentTo(currentMetric))
		})
	})

	Context("test ServiceMonitor", func() {
		BeforeEach(func() {
			currentMetric, _ = metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)
		})

		expectedEvents := []commontestutils.MockEvent{
			{
				EventType: corev1.EventTypeNormal,
				Reason:    "Updated",
				Msg:       "Updated ServiceMonitor " + serviceName,
			},
		}

		It("should update the labels if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)
			existSM.Labels = map[string]string{
				"wrongKey1": "wrongValue1",
				"wrongKey2": "wrongValue2",
				"wrongKey3": "wrongValue3",
			}

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())

			Expect(sm.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, commontestutils.KeysFromSSMap(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring))))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the labels if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)
			existSM.Labels = nil

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())

			Expect(sm.Labels).To(Equal(hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)))
			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified", func() {
			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)
			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(sm.OwnerReferences).To(HaveLen(1))
			Expect(sm.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(sm.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(sm.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(sm.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the referenceOwner if modified; no HCO triggered", func() {
			req.HCOTriggered = false

			owner := metav1.OwnerReference{
				APIVersion:         "wrongAPIVersion",
				Kind:               "wrongKind",
				Name:               "wrongName",
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
				UID:                "0987654321",
			}
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)
			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(sm.OwnerReferences).To(HaveLen(1))
			Expect(sm.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(sm.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(sm.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(sm.OwnerReferences[0].UID).To(Equal(deployment.UID))

			overrideExpectedEvents := []commontestutils.MockEvent{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    "Overwritten",
					Msg:       "Overwritten ServiceMonitor " + serviceName,
				},
			}

			Expect(ee.CheckEvents(overrideExpectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric + 1))
		})

		It("should update the referenceOwner if missing", func() {
			owner := metav1.OwnerReference{}
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)
			existSM.OwnerReferences = nil
			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())

			deployment := ci.GetDeployment()

			Expect(sm.OwnerReferences).To(HaveLen(1))
			Expect(sm.OwnerReferences[0].Name).To(Equal(deployment.Name))
			Expect(sm.OwnerReferences[0].Kind).To(Equal("Deployment"))
			Expect(sm.OwnerReferences[0].APIVersion).To(Equal(appsv1.GroupName + "/v1"))
			Expect(sm.OwnerReferences[0].UID).To(Equal(deployment.UID))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Spec if modified", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)

			existSM.Spec = monitoringv1.ServiceMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"wrongKey1": "wrongValue1",
						"wrongKey2": "wrongValue2",
					},
				},
				Endpoints: []monitoringv1.Endpoint{{Port: "wrongPort", Path: "/metrics"}},
			}

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())
			Expect(sm.Spec.Selector).To(Equal(metav1.LabelSelector{MatchLabels: hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)}))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal(operatorPortName))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric))
		})

		It("should update the Spec if it's missing", func() {
			owner := getDeploymentReference(ci.GetDeployment())
			existSM := NewServiceMonitor(commontestutils.Namespace, owner)

			existSM.Spec = monitoringv1.ServiceMonitorSpec{}

			cl := commontestutils.InitClient([]client.Object{ns, existSM})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())
			sm := &monitoringv1.ServiceMonitor{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: r.namespace, Name: serviceName}, sm)).To(Succeed())
			Expect(sm.Spec.Selector).To(Equal(metav1.LabelSelector{MatchLabels: hcoutil.GetLabels(hcoutil.HyperConvergedName, hcoutil.AppComponentMonitoring)}))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal(operatorPortName))

			Expect(ee.CheckEvents(expectedEvents)).To(BeTrue())
			Expect(metrics.GetOverwrittenModificationsCount("ServiceMonitor", serviceName)).To(BeEquivalentTo(currentMetric))
		})
	})

	Context("test Namespace", func() {

		DescribeTable("validate the annotation and the label", func(nsGenerator func() *corev1.Namespace) {
			cl := commontestutils.InitClient([]client.Object{nsGenerator()})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			foundNS := &corev1.Namespace{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Name: r.namespace}, foundNS)).To(Succeed())
			Expect(foundNS.Annotations).ToNot(BeEmpty())
			annotation, ok := foundNS.Annotations[hcoutil.OpenshiftNodeSelectorAnn]
			Expect(ok).To(BeTrue())
			Expect(annotation).To(BeEmpty())

			label, ok := foundNS.Labels[hcoutil.PrometheusNSLabel]
			Expect(ok).To(BeTrue())
			Expect(label).To(Equal("true"))
		},
			Entry("when the annotations and the labels fields are nil", func() *corev1.Namespace { return ns }),
			Entry("when the annotations and the labels fields are empty", func() *corev1.Namespace {
				ns.Annotations = map[string]string{}
				ns.Labels = map[string]string{}
				return ns
			}),
			Entry("when the annotation is not empty", func() *corev1.Namespace {
				ns.Annotations = map[string]string{hcoutil.OpenshiftNodeSelectorAnn: "notEmpty"}
				return ns
			}),
			Entry("when the label is empty", func() *corev1.Namespace {
				ns.Labels = map[string]string{hcoutil.PrometheusNSLabel: ""}
				return ns
			}),
			Entry("when the label is false", func() *corev1.Namespace {
				ns.Labels = map[string]string{hcoutil.PrometheusNSLabel: "false"}
				return ns
			}),
			Entry("when the label is wrong", func() *corev1.Namespace {
				ns.Labels = map[string]string{hcoutil.PrometheusNSLabel: "wrong"}
				return ns
			}),
		)

		It("should not modify other labels", func() {
			ns.Labels = map[string]string{"aaa": "AAA", "bbb": "BBB"}
			cl := commontestutils.InitClient([]client.Object{ns})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			foundNS := &corev1.Namespace{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Name: r.namespace}, foundNS)).To(Succeed())

			Expect(foundNS.Labels).To(HaveLen(3))
			Expect(foundNS.Labels).To(HaveKeyWithValue("aaa", "AAA"))
			Expect(foundNS.Labels).To(HaveKeyWithValue("bbb", "BBB"))
			Expect(foundNS.Labels).To(HaveKeyWithValue(hcoutil.PrometheusNSLabel, "true"))
		})

		It("should not modify other annotations", func() {
			ns.Annotations = map[string]string{"aaa": "AAA", "bbb": "BBB"}
			cl := commontestutils.InitClient([]client.Object{ns})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(Succeed())

			foundNS := &corev1.Namespace{}
			Expect(cl.Get(context.Background(), client.ObjectKey{Name: r.namespace}, foundNS)).To(Succeed())

			Expect(foundNS.Annotations).To(HaveLen(3))
			Expect(foundNS.Annotations).To(HaveKeyWithValue("aaa", "AAA"))
			Expect(foundNS.Annotations).To(HaveKeyWithValue("bbb", "BBB"))
			Expect(foundNS.Annotations).To(HaveKeyWithValue(hcoutil.OpenshiftNodeSelectorAnn, ""))
		})

		It("should return error if can't read the namespace", func() {
			cl := commontestutils.InitClient([]client.Object{})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(HaveOccurred())
		})

		It("should return error if failed to read the namespace", func() {
			cl := commontestutils.InitClient([]client.Object{ns})
			err := errors.New("fake error")
			cl.InitiateGetErrors(func(_ client.ObjectKey) error {
				return err
			})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(MatchError(err))
		})

		It("should return error if can't update the namespace", func() {
			cl := commontestutils.InitClient([]client.Object{ns})
			err := errors.New("fake error")
			cl.InitiateUpdateErrors(func(_ client.Object) error {
				return err
			})
			r := NewMonitoringReconciler(ci, cl, ee, commontestutils.GetScheme())

			Expect(r.Reconcile(req, false)).To(MatchError(err))
		})
	})
})
