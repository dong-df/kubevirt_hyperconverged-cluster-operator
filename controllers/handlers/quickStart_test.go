package handlers

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	consolev1 "github.com/openshift/api/console/v1"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kubevirt/hyperconverged-cluster-operator/controllers/commontestutils"
	hcoutil "github.com/kubevirt/hyperconverged-cluster-operator/pkg/util"
)

var _ = Describe("QuickStart tests", func() {

	schemeForTest := commontestutils.GetScheme()

	var (
		testLogger        = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)).WithName("quickstart_test")
		testFilesLocation = getTestFilesLocation() + "/quickstarts"
		hco               = commontestutils.NewHco()
	)

	Context("test GetQuickStartHandlers", func() {
		It("should use env var to override the yaml locations", func() {
			// create temp folder for the test
			dir := path.Join(os.TempDir(), fmt.Sprint(time.Now().UTC().Unix()))
			_ = os.Setenv(QuickStartManifestLocationVarName, dir)
			By("folder not exists", func() {
				cli := commontestutils.InitClient([]client.Object{})
				handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)

				Expect(err).ToNot(HaveOccurred())
				Expect(handlers).To(BeEmpty())
			})

			Expect(os.Mkdir(dir, 0744)).To(Succeed())
			defer os.RemoveAll(dir)

			By("folder is empty", func() {
				cli := commontestutils.InitClient([]client.Object{})
				handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)

				Expect(err).ToNot(HaveOccurred())
				Expect(handlers).To(BeEmpty())
			})

			nonYaml, err := os.OpenFile(path.Join(dir, "for_test.txt"), os.O_CREATE|os.O_WRONLY, 0644)
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(nonYaml.Name())

			_, err = fmt.Fprintln(nonYaml, `some text`)
			Expect(err).ToNot(HaveOccurred())
			_ = nonYaml.Close()

			By("no yaml files", func() {
				cli := commontestutils.InitClient([]client.Object{})
				handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)

				Expect(err).ToNot(HaveOccurred())
				Expect(handlers).To(BeEmpty())
			})

			Expect(commontestutils.CopyFile(path.Join(dir, "quickStart.yaml"), path.Join(testFilesLocation, "quickstart.yaml"))).To(Succeed())

			By("yaml file exists", func() {
				cli := commontestutils.InitClient([]client.Object{})
				handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)

				Expect(err).ToNot(HaveOccurred())
				Expect(handlers).To(HaveLen(1))
				Expect(quickstartNames).To(ContainElements("test-quick-start"))
			})
		})

		It("should return error if quickstart path is not a directory", func() {
			filePath := "/testFiles/quickstarts/quickstart.yaml"
			const currentDir = "/controllers/handlers"
			wd, _ := os.Getwd()
			if !strings.HasSuffix(wd, currentDir) {
				filePath = wd + currentDir + filePath
			} else {
				filePath = wd + filePath
			}

			// quickstart directory path of a file
			_ = os.Setenv(QuickStartManifestLocationVarName, filePath)
			By("check that GetQuickStartHandlers returns error")
			cli := commontestutils.InitClient([]client.Object{})
			handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)

			Expect(err).To(HaveOccurred())
			Expect(handlers).To(BeEmpty())
		})
	})

	Context("test quickStartHandler", func() {

		It("should create the ConsoleQuickStart resource if not exists", func() {
			_ = os.Setenv(QuickStartManifestLocationVarName, testFilesLocation)

			cli := commontestutils.InitClient([]client.Object{})
			handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)
			Expect(err).ToNot(HaveOccurred())
			Expect(handlers).To(HaveLen(1))
			Expect(quickstartNames).To(ContainElement("test-quick-start"))

			hco := commontestutils.NewHco()
			By("apply the quickstart CRs", func() {
				req := commontestutils.NewReq(hco)
				res := handlers[0].Ensure(req)
				Expect(res.Err).ToNot(HaveOccurred())
				Expect(res.Created).To(BeTrue())

				quickstartObjects := &consolev1.ConsoleQuickStartList{}
				Expect(cli.List(context.TODO(), quickstartObjects)).To(Succeed())
				Expect(quickstartObjects.Items).To(HaveLen(1))
				Expect(quickstartObjects.Items[0].Name).To(Equal("test-quick-start"))
			})
		})

		It("should update the ConsoleQuickStart resource if not not equal to the expected one", func() {
			Expect(os.Setenv(QuickStartManifestLocationVarName, testFilesLocation)).To(Succeed())

			exists, err := getQSsFromTestData(testFilesLocation)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).ToNot(BeNil())
			exists.Spec.DurationMinutes = exists.Spec.DurationMinutes * 2

			cli := commontestutils.InitClient([]client.Object{exists})
			handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)
			Expect(err).ToNot(HaveOccurred())
			Expect(handlers).To(HaveLen(1))
			Expect(quickstartNames).To(ContainElement("test-quick-start"))

			hco := commontestutils.NewHco()
			By("apply the quickstart CRs", func() {
				req := commontestutils.NewReq(hco)
				res := handlers[0].Ensure(req)
				Expect(res.Err).ToNot(HaveOccurred())
				Expect(res.Updated).To(BeTrue())

				quickstartObjects := &consolev1.ConsoleQuickStartList{}
				Expect(cli.List(context.TODO(), quickstartObjects)).To(Succeed())
				Expect(quickstartObjects.Items).To(HaveLen(1))
				Expect(quickstartObjects.Items[0].Name).To(Equal("test-quick-start"))
				// check that the existing object was reconciled
				Expect(quickstartObjects.Items[0].Spec.DurationMinutes).To(Equal(20))

				// ObjectReference should have been updated
				Expect(hco.Status.RelatedObjects).To(Not(BeNil()))
				objectRefOutdated, err := reference.GetReference(schemeForTest, exists)
				Expect(err).ToNot(HaveOccurred())
				objectRefFound, err := reference.GetReference(schemeForTest, &quickstartObjects.Items[0])
				Expect(err).ToNot(HaveOccurred())
				Expect(hco.Status.RelatedObjects).To(Not(ContainElement(*objectRefOutdated)))
				Expect(hco.Status.RelatedObjects).To(ContainElement(*objectRefFound))
			})
		})

		It("should reconcile managed labels to default without touching user added ones", func() {
			_ = os.Setenv(QuickStartManifestLocationVarName, testFilesLocation)

			const userLabelKey = "userLabelKey"
			const userLabelValue = "userLabelValue"

			cli := commontestutils.InitClient([]client.Object{})
			handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)
			Expect(err).ToNot(HaveOccurred())
			Expect(handlers).To(HaveLen(1))

			quickstartObjects := &consolev1.ConsoleQuickStartList{}

			req := commontestutils.NewReq(hco)

			By("apply the quickstart CRs", func() {
				res := handlers[0].Ensure(req)
				Expect(res.Err).ToNot(HaveOccurred())
				Expect(res.Created).To(BeTrue())

				Expect(cli.List(context.TODO(), quickstartObjects)).To(Succeed())
				Expect(quickstartObjects.Items).To(HaveLen(1))
				Expect(quickstartObjects.Items[0].Name).To(Equal("test-quick-start"))
			})

			expectedLabels := make(map[string]map[string]string)

			By("getting opinionated labels", func() {
				for _, quickstart := range quickstartObjects.Items {
					expectedLabels[quickstart.Name] = maps.Clone(quickstart.Labels)
				}
			})

			By("altering the quickstart objects", func() {
				for _, foundResource := range quickstartObjects.Items {
					for k, v := range expectedLabels[foundResource.Name] {
						foundResource.Labels[k] = "wrong_" + v
					}
					foundResource.Labels[userLabelKey] = userLabelValue
					err = cli.Update(context.TODO(), &foundResource)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			By("reconciling quickstart objects", func() {
				for _, handler := range handlers {
					res := handler.Ensure(req)
					Expect(res.UpgradeDone).To(BeFalse())
					Expect(res.Updated).To(BeTrue())
					Expect(res.Err).ToNot(HaveOccurred())
				}
			})

			foundResourcesList := &consolev1.ConsoleQuickStartList{}
			Expect(cli.List(context.TODO(), foundResourcesList)).To(Succeed())

			for _, foundResource := range foundResourcesList.Items {
				for k, v := range expectedLabels[foundResource.Name] {
					Expect(foundResource.Labels).To(HaveKeyWithValue(k, v))
				}
				Expect(foundResource.Labels).To(HaveKeyWithValue(userLabelKey, userLabelValue))
			}
		})

		It("should reconcile managed labels to default on label deletion without touching user added ones", func() {
			_ = os.Setenv(QuickStartManifestLocationVarName, testFilesLocation)

			const userLabelKey = "userLabelKey"
			const userLabelValue = "userLabelValue"

			cli := commontestutils.InitClient([]client.Object{})
			handlers, err := GetQuickStartHandlers(testLogger, cli, schemeForTest, hco)
			Expect(err).ToNot(HaveOccurred())
			Expect(handlers).To(HaveLen(1))

			quickstartObjects := &consolev1.ConsoleQuickStartList{}

			req := commontestutils.NewReq(hco)

			By("apply the quickstart CRs", func() {
				res := handlers[0].Ensure(req)
				Expect(res.Err).ToNot(HaveOccurred())
				Expect(res.Created).To(BeTrue())

				Expect(cli.List(context.TODO(), quickstartObjects)).To(Succeed())
				Expect(quickstartObjects.Items).To(HaveLen(1))
				Expect(quickstartObjects.Items[0].Name).To(Equal("test-quick-start"))
			})

			expectedLabels := make(map[string]map[string]string)

			By("getting opinionated labels", func() {
				for _, quickstart := range quickstartObjects.Items {
					expectedLabels[quickstart.Name] = maps.Clone(quickstart.Labels)
				}
			})

			By("altering the quickstart objects", func() {
				for _, foundResource := range quickstartObjects.Items {
					delete(foundResource.Labels, hcoutil.AppLabelVersion)
					foundResource.Labels[userLabelKey] = userLabelValue
					err = cli.Update(context.TODO(), &foundResource)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			By("reconciling quickstart objects", func() {
				for _, handler := range handlers {
					res := handler.Ensure(req)
					Expect(res.UpgradeDone).To(BeFalse())
					Expect(res.Updated).To(BeTrue())
					Expect(res.Err).ToNot(HaveOccurred())
				}
			})

			foundResourcesList := &consolev1.ConsoleQuickStartList{}
			Expect(cli.List(context.TODO(), foundResourcesList)).To(Succeed())

			for _, foundResource := range foundResourcesList.Items {
				for k, v := range expectedLabels[foundResource.Name] {
					Expect(foundResource.Labels).To(HaveKeyWithValue(k, v))
				}
				Expect(foundResource.Labels).To(HaveKeyWithValue(userLabelKey, userLabelValue))
			}
		})
	})
})

func getQSsFromTestData(testFilesLocation string) (*consolev1.ConsoleQuickStart, error) {
	dirEntries, err := os.ReadDir(testFilesLocation)
	if err != nil {
		return nil, err
	}

	for _, entry := range dirEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		filePath := path.Join(testFilesLocation, entry.Name())
		return getQSFromTestData(filePath)
	}

	return nil, nil
}

func getQSFromTestData(filePath string) (*consolev1.ConsoleQuickStart, error) {
	file, err := os.Open(filePath)

	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	return quickStartFromFile(file)
}
