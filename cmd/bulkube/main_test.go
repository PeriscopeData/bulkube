package main

import (
	"io/ioutil"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"testing"
)

func TestBulkUpdate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BulkUpdate Suite")
}

const podTrainerYaml = `apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: null
  labels:
    k8s-app: pod-trainer
  name: pod-trainer
spec:
  automountServiceAccountToken: false
  containers:
  - env:
    - name: ECOSYSTEM_NAME
      value: k8s.guardian.us-west-2.pdops.net
    image: test-repo/training-sim@sha256:ce9762ec9a423afd10ec4cdb929af496cfad1a875298ab86735ae791fe98cca6
    name: pod-trainer
    resources: {}
status: {}
`

const gopherTrainerYaml = `apiVersion: v1
kind: Service
metadata:
  name: gopher-trainer
  labels:
    k8s-app: gopher-trainer
spec:
  ports:
    - port: 1800
      targetPort: 1804
  selector:
    k8s-app: gopher-trainer
  type: ClusterIP
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    k8s-app: gopher-trainer
    k8s-group: arena
  name: gopher-trainer
spec:
  replicas: 1
  selector:
    matchLabels:
      k8s-app: gopher-trainer
  template:
    metadata:
      labels:
        k8s-app: gopher-trainer
    spec:
      containers:
        - name: training-sim
          image: test-repo/training-sim@sha256:ce9762ec9a423afd10ec4cdb929af496cfad1a875298ab86735ae791fe98cca6
`

const snakeTrainerYaml = `apiVersion: v1
kind: ServiceAccount
metadata:
  creationTimestamp: null
  labels:
    k8s-app: snake-trainer
  name: snake-trainer
---
apiVersion: v1
data:
  config.yaml: STUFF
kind: ConfigMap
metadata:
  creationTimestamp: null
  labels:
    k8s-app: snake-trainer
  name: snake-map
---
apiVersion: apps/v1
kind: Deployment
metadata:
  creationTimestamp: null
  labels:
    k8s-app: snake-trainer
    k8s-group: arena
  name: snake-trainer
spec:
  replicas: 1
  selector:
    matchLabels:
      k8s-app: snake-trainer
  strategy: {}
  template:
    metadata:
      creationTimestamp: null
      labels:
        k8s-app: snake-trainer
    spec:
      containers:
      - image: test-repo/training-sim@sha256:ce9762ec9a423afd10ec4cdb929af496cfad1a875298ab86735ae791fe98cca6
        name: training-sim
        resources: {}
      - image: test-repo/pit-guard:latest
        name: pit-operator
        resources: {}
status: {}
`

var _ = Context("Updates matching files", func() {
	var (
		tmpDir string
		err    error
		paths  []string
	)

	BeforeEach(func() {
		format.TruncatedDiff = false
		tmpDir, err = ioutil.TempDir("", "bulk_update_test")
		if err != nil {
			panic(err)
		}
		ioutil.WriteFile(tmpDir+"/pod_trainer.yaml", []byte(podTrainerYaml), 0755)
		ioutil.WriteFile(tmpDir+"/gopher_trainer.yaml", []byte(gopherTrainerYaml), 0755)
		ioutil.WriteFile(tmpDir+"/snake_trainer.yaml", []byte(snakeTrainerYaml), 0755)
		paths = []string{tmpDir + "/gopher_trainer.yaml", tmpDir + "/snake_trainer.yaml", tmpDir + "/pod_trainer.yaml"}
	})

	ExpectFileMatchesContent := func(filename, content string) {
		data, err := ioutil.ReadFile(filename)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(string(data)).To(Equal(content))
	}

	It("Updates the version of matching containers", func() {
		r := builder(paths, "k8s-app=snake-trainer").Do()
		updatedObjectMap, _ := updateMatchingObjects(r, "test-repo/training-sim", "abcdef")
		writeObjectFiles(updatedObjectMap)
		ExpectFileMatchesContent(tmpDir+"/snake_trainer.yaml", strings.Replace(snakeTrainerYaml, "ce9762ec9a423afd10ec4cdb929af496cfad1a875298ab86735ae791fe98cca6", "abcdef", -1))
		ExpectFileMatchesContent(tmpDir+"/gopher_trainer.yaml", gopherTrainerYaml)
	})

	It("Updates the version of matching containers in a pod", func() {
		r := builder(paths, "k8s-app=pod-trainer").Do()
		updatedObjectMap, _ := updateMatchingObjects(r, "test-repo/training-sim", "abcdef")
		writeObjectFiles(updatedObjectMap)
		ExpectFileMatchesContent(tmpDir+"/pod_trainer.yaml", strings.Replace(podTrainerYaml, "ce9762ec9a423afd10ec4cdb929af496cfad1a875298ab86735ae791fe98cca6", "abcdef", -1))
	})

	It("Writes all objects in the file, even those skipped by the filter", func() {
		r := builder(paths, "k8s-app=snake-trainer,k8s-group=arena").Do()
		updatedObjectMap, _ := updateMatchingObjects(r, "test-repo/training-sim", "abcdef")
		writeObjectFiles(updatedObjectMap)
		ExpectFileMatchesContent(tmpDir+"/snake_trainer.yaml", strings.Replace(snakeTrainerYaml, "ce9762ec9a423afd10ec4cdb929af496cfad1a875298ab86735ae791fe98cca6", "abcdef", -1))
		ExpectFileMatchesContent(tmpDir+"/gopher_trainer.yaml", gopherTrainerYaml)
	})

	DescribeTable("builder()",
		func(labelSelector string, expectedLength int) {
			infos, err := builder([]string{tmpDir}, labelSelector).Do().Infos()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(infos).Should(HaveLen(expectedLength))
		},
		Entry("snake file", "k8s-app=snake-trainer", 3),
		Entry("gopher file", "k8s-app=gopher-trainer", 2),
		Entry("pod file", "k8s-app=pod-trainer", 1),
		Entry("group", "k8s-group=arena", 2),
		Entry("all", "", 6),
		Entry("gopher arena", "k8s-group=arena,k8s-app=gopher-trainer", 1),
	)

	Describe("updateMatchingObject", func() {
		It("Returns all files that have updated objects", func() {
			r := builder(paths, "").Do()
			updatedObjectMap, _ := updateMatchingObjects(r, "test-repo/training-sim", "abcdef")
			Expect(updatedObjectMap).Should(HaveKey(tmpDir + "/snake_trainer.yaml"))
			Expect(updatedObjectMap).Should(HaveKey(tmpDir + "/gopher_trainer.yaml"))
		})

		It("Only returns files that have updated objects", func() {
			r := builder(paths, "").Do()
			updatedObjectMap, _ := updateMatchingObjects(r, "test-repo/pit-guard", "abcdef")
			Expect(updatedObjectMap).Should(HaveKey(tmpDir + "/snake_trainer.yaml"))
			Expect(updatedObjectMap).ShouldNot(HaveKey(tmpDir + "/gopher_trainer.yaml"))
		})

		It("Returns all objects in files that have modified updated objects", func() {
			r := builder(paths, "").Do()
			updatedObjectMap, _ := updateMatchingObjects(r, "test-repo/training-sim", "abcdef")
			Expect(updatedObjectMap[tmpDir+"/snake_trainer.yaml"]).Should(HaveLen(3))
			Expect(updatedObjectMap[tmpDir+"/gopher_trainer.yaml"]).Should(HaveLen(2))
		})
	})
})
