package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	_ "k8s.io/apimachinery/pkg/runtime" // Needed for `go get` to function properly

	v1apps "k8s.io/api/apps/v1"
	v1batch "k8s.io/api/batch/v1"
	v1core "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubectl/genericclioptions"
	"k8s.io/kubernetes/pkg/kubectl/genericclioptions/printers"
	"k8s.io/kubernetes/pkg/kubectl/genericclioptions/resource"
	"k8s.io/kubernetes/pkg/kubectl/scheme"
)

var (
	labelSelectorFlag string
	imageNameFlag     string
	imageShaFlag      string
	reformatAll       bool
	t                 interface{}
)

func init() {
	flag.StringVar(&labelSelectorFlag, "l", "", "Filter deployments by label.")
	flag.StringVar(&imageNameFlag, "image", "", "Image to modify. Only modifies containers that match this image/repository. If @sha256: is included, will use that as sha.")
	flag.StringVar(&imageShaFlag, "sha", "", "Set image version by sha.")
	flag.BoolVar(&reformatAll, "fmt", false, "Reformat even if version does not change.")
}

func main() {
	flag.Parse()
	if strings.Contains(imageNameFlag, "@sha256:") {
		splitName := strings.Split(imageNameFlag, "@sha256:")
		if len(splitName) == 2 {
			imageNameFlag, imageShaFlag = splitName[0], splitName[1]
		}
	}

	if flag.NArg() == 0 || ((imageNameFlag == "" || imageShaFlag == "") && !reformatAll) {
		flag.Usage()
		return
	}

	r := builder(flag.Args(), labelSelectorFlag).Do()
	if err := r.Err(); err != nil {
		panic(err)
	}

	modifiedObjectFiles, err := updateMatchingObjects(r, imageNameFlag, imageShaFlag)
	if err != nil {
		panic(err)
	}
	writeObjectFiles(modifiedObjectFiles)
}

func updateMatchingObjects(r *resource.Result, imageName, imageSha string) (map[string][]*resource.Info, error) {
	var (
		objectsByFile = map[string][]*resource.Info{}
		updatedFiles  = map[string]interface{}{}
	)
	err := r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		if objectsByFile[info.Source] == nil {
			objectsByFile[info.Source] = []*resource.Info{}
		}
		objectsByFile[info.Source] = append(objectsByFile[info.Source], info)
		switch o := info.Object.(type) {
		case *v1apps.Deployment:
			if updateContainerImage(o.Spec.Template.Spec.Containers, imageName, imageSha) || reformatAll {
				updatedFiles[info.Source] = t
			}
		case *v1batch.Job:
			if updateContainerImage(o.Spec.Template.Spec.Containers, imageName, imageSha) || reformatAll {
				updatedFiles[info.Source] = t
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
		return nil, err
	}
	modifiedObjectFiles := map[string][]*resource.Info{}
	for filename := range updatedFiles {
		modifiedObjectFiles[filename] = objectsByFile[filename]
	}
	return modifiedObjectFiles, nil
}

func writeObjectFiles(objectMap map[string][]*resource.Info) {
	for filename, objectList := range objectMap {
		writeObjectsToFile(objectList, filename)
	}
}

func SameObject(a, b *resource.Info) bool {
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Object.GetObjectKind().GroupVersionKind() == b.Object.GetObjectKind().GroupVersionKind()
}

func replaceUpdatedObjects(allObjects, updatedObjects []*resource.Info) {
	for idx, obj := range allObjects {
		if len(updatedObjects) > 0 && SameObject(obj, updatedObjects[0]) {
			allObjects[idx] = updatedObjects[0]
			updatedObjects = updatedObjects[1:]
		}
	}
}

func writeObjectsToFile(updatedObjectList []*resource.Info, filename string) (err error) {
	var (
		printer    = &printers.YAMLPrinter{}
		allObjects []*resource.Info
		configFile io.WriteCloser
	)

	// Read all objects to include filtered ones
	if allObjects, err = builder([]string{filename}, "").Do().Infos(); err == nil {
		// Merge new objects into full list
		replaceUpdatedObjects(allObjects, updatedObjectList)
		if configFile, err = os.OpenFile(filename, os.O_TRUNC|os.O_WRONLY, 0755); err == nil {
			defer configFile.Close()
			for _, info := range allObjects {
				fmt.Fprintln(configFile, "---")
				if err = printer.PrintObj(info.Object, configFile); err != nil {
					return
				}
			}
		}
	}
	return
}

func updateContainerImage(containerList []v1core.Container, imageName, imageSha string) (changed bool) {
	for idx, c := range containerList {
		if strings.HasPrefix(c.Image, imageName+":") || strings.HasPrefix(c.Image, imageName+"@sha256") {
			containerList[idx].Image = fmt.Sprintf("%s@sha256:%s", imageName, imageSha)
			changed = true
		}
	}
	return
}

func builder(paths []string, labelSelector string) *resource.Builder {
	b := resource.NewBuilder(genericclioptions.NewConfigFlags()).
		WithScheme(scheme.Scheme, v1apps.SchemeGroupVersion, v1core.SchemeGroupVersion, v1batch.SchemeGroupVersion).
		LabelSelector(labelSelector).
		NamespaceParam("default").DefaultNamespace().RequireNamespace().
		ExportParam(true).
		Local().
		Flatten()
	for _, path := range paths {
		b = b.Path(true, path)
	}
	return b
}
