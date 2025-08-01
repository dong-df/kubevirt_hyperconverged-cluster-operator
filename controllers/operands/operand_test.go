package operands

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/kubevirt/hyperconverged-cluster-operator/controllers/commontestutils"
)

var _ = Describe("Test operator.go", func() {
	Context("Test applyAnnotationPatch", func() {
		It("Should fail for bad json", func() {
			obj := &cdiv1beta1.CDI{}

			err := applyAnnotationPatch(obj, `{]`)
			Expect(err).To(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "Expected error: %v\n", err)
		})

		It("Should fail for single patch object (instead of an array)", func() {
			obj := &cdiv1beta1.CDI{}

			err := applyAnnotationPatch(obj, `{"op": "add", "path": "/spec/config/featureGates/-", "value": "fg1"}`)
			Expect(err).To(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "Expected error: %v\n", err)
		})

		It("Should fail for unknown op in a patch object", func() {
			obj := &cdiv1beta1.CDI{}

			err := applyAnnotationPatch(obj, `[{"op": "unknown", "path": "/spec/config/featureGates/-", "value": "fg1"}]`)
			Expect(err).To(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "Expected error: %v\n", err)
		})

		It("Should fail for wrong path - not starts with '/spec/' - patch object", func() {
			obj := &cdiv1beta1.CDI{}

			err := applyAnnotationPatch(obj, `[{"op": "add", "path": "/config/featureGates/-", "value": "fg1"}]`)
			Expect(err).To(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "Expected error: %v\n", err)
		})

		It("Should fail for adding to a not exist object", func() {
			obj := &cdiv1beta1.CDI{}

			err := applyAnnotationPatch(obj, `[{"op": "add", "path": "/spec/config/filesystemOverhead/global", "value": "65"}]`)
			Expect(err).To(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "Expected error: %v\n", err)
		})

		It("Should fail for removing non-exist field", func() {
			obj := &cdiv1beta1.CDI{
				Spec: cdiv1beta1.CDISpec{
					Config: &cdiv1beta1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1beta1.FilesystemOverhead{},
					},
				},
			}

			err := applyAnnotationPatch(obj, `[{"op": "remove", "path": "/spec/config/filesystemOverhead/global"}]`)
			Expect(err).To(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "Expected error: %v\n", err)
		})

		It("Should apply annotation if everything is corrct", func() {
			obj := &cdiv1beta1.CDI{
				Spec: cdiv1beta1.CDISpec{
					Config: &cdiv1beta1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1beta1.FilesystemOverhead{},
					},
				},
			}

			Expect(
				applyAnnotationPatch(obj, `[{"op": "add", "path": "/spec/config/filesystemOverhead/global", "value": "55"}]`),
			).To(Succeed())

			Expect(obj.Spec.Config).NotTo(BeNil())
			Expect(obj.Spec.Config.FilesystemOverhead).NotTo(BeNil())
			Expect(obj.Spec.Config.FilesystemOverhead.Global).To(BeEquivalentTo("55"))
		})
	})

	Context("Test addCrToTheRelatedObjectList", func() {
		It("Should return error when apiVersion, kind and name missing", func() {
			hco := commontestutils.NewHco()
			req := commontestutils.NewReq(hco)
			found := &cdiv1beta1.CDI{}

			operand := GenericOperand{Scheme: commontestutils.GetScheme()}
			Expect(
				operand.addCrToTheRelatedObjectList(req, found),
			).To(MatchError(ContainSubstring("object reference must have, at a minimum: apiVersion, kind, and name")))
		})

		It("Should add into the list when it is missing", func() {
			hco := commontestutils.NewHco()
			req := commontestutils.NewReq(hco)
			found := &cdiv1beta1.CDI{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CDI",
					APIVersion: "cdi.kubevirt.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "cdi-kubevirt-hyperconverged",
				},
			}

			operand := GenericOperand{Scheme: commontestutils.GetScheme()}
			Expect(operand.addCrToTheRelatedObjectList(req, found)).To(Succeed())

			foundRef, err := reference.GetReference(operand.Scheme, found)
			Expect(err).ToNot(HaveOccurred())
			Expect(hco.Status.RelatedObjects).To(ContainElement(*foundRef))
		})

		It("Should update the list and set StatusDirty=true when the resourceVersion is different", func() {
			const oldVersion = "111"
			const newVersion = "112"
			hco := commontestutils.NewHco()
			req := commontestutils.NewReq(hco)
			found := &cdiv1beta1.CDI{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CDI",
					APIVersion: "cdi.kubevirt.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "cdi-kubevirt-hyperconverged",
					ResourceVersion: oldVersion,
				},
			}

			operand := GenericOperand{Scheme: commontestutils.GetScheme()}
			Expect(operand.addCrToTheRelatedObjectList(req, found)).To(Succeed())

			oldRef, err := reference.GetReference(operand.Scheme, found)
			Expect(err).ToNot(HaveOccurred())

			// update resource version
			found.ResourceVersion = newVersion
			Expect(operand.addCrToTheRelatedObjectList(req, found)).To(Succeed())

			newRef, err := reference.GetReference(operand.Scheme, found)
			Expect(err).ToNot(HaveOccurred())

			Expect(hco.Status.RelatedObjects).To(ContainElement(*newRef))
			Expect(hco.Status.RelatedObjects).ToNot(ContainElement(*oldRef))
			Expect(req.StatusDirty).To(BeTrue())
		})
	})

	Context("Test createNewCr", func() {

		It("Should successfully create an object", func() {
			hco := commontestutils.NewHco()
			req := commontestutils.NewReq(hco)

			expectedResource := newCDI()

			cl := commontestutils.InitClient([]client.Object{hco})

			res := NewEnsureResult(expectedResource)

			operand := GenericOperand{Scheme: commontestutils.GetScheme(), Client: cl}
			outRes := operand.createNewCr(req, expectedResource, res)
			Expect(outRes.Err).ToNot(HaveOccurred())
			Expect(outRes.Created).To(BeTrue())
			Expect(outRes.Deleted).To(BeFalse())
			Expect(outRes.Updated).To(BeFalse())
			Expect(outRes.Overwritten).To(BeFalse())
			Expect(outRes.UpgradeDone).To(BeFalse())

			foundResource := &cdiv1beta1.CDI{}
			Expect(
				cl.Get(context.TODO(),
					types.NamespacedName{Name: expectedResource.Name, Namespace: expectedResource.Namespace},
					foundResource),
			).To(Succeed())
		})

		It("Should not fail due to existing resourceVersions", func() {
			hco := commontestutils.NewHco()
			req := commontestutils.NewReq(hco)

			expectedResource := newCDI()
			expectedResource.ResourceVersion = "1234"
			cl := commontestutils.InitClient([]client.Object{hco})

			res := NewEnsureResult(expectedResource)

			operand := GenericOperand{Scheme: commontestutils.GetScheme(), Client: cl}
			outRes := operand.createNewCr(req, expectedResource, res)
			Expect(outRes.Err).ToNot(HaveOccurred())
			Expect(outRes.Created).To(BeTrue())
			Expect(outRes.Deleted).To(BeFalse())
			Expect(outRes.Updated).To(BeFalse())
			Expect(outRes.Overwritten).To(BeFalse())
			Expect(outRes.UpgradeDone).To(BeFalse())

			foundResource := &cdiv1beta1.CDI{}
			Expect(
				cl.Get(context.TODO(),
					types.NamespacedName{Name: expectedResource.Name, Namespace: expectedResource.Namespace},
					foundResource),
			).To(Succeed())
		})

		It("Should fail if the object was already there", func() {
			hco := commontestutils.NewHco()
			req := commontestutils.NewReq(hco)

			expectedResource := newCDI()

			cl := commontestutils.InitClient([]client.Object{hco, expectedResource})

			res := NewEnsureResult(expectedResource)

			operand := GenericOperand{Scheme: commontestutils.GetScheme(), Client: cl}
			outRes := operand.createNewCr(req, expectedResource, res)
			Expect(outRes.Err).To(MatchError(apierrors.IsAlreadyExists, "already exists error"))
			Expect(outRes.Created).To(BeFalse())
			Expect(outRes.Deleted).To(BeFalse())
			Expect(outRes.Updated).To(BeFalse())
			Expect(outRes.Overwritten).To(BeFalse())
			Expect(outRes.UpgradeDone).To(BeFalse())

		})

	})

})

func newCDI() *cdiv1beta1.CDI {
	return &cdiv1beta1.CDI{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CDI",
			APIVersion: "cdi.kubevirt.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cdi-test",
		},
	}
}
