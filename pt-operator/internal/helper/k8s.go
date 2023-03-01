package helper

import (
	"context"
	"encoding/base64"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func KubeClientset(ctx context.Context, caText64 string, gkeEndpoint string) (*rest.Config, *kubernetes.Clientset, error) {
	l := log.FromContext(ctx)
	caText, err := base64.StdEncoding.DecodeString(caText64)
	if err != nil {
		l.Error(err, "failed to decode text")
	}
	config := &rest.Config{
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caText,
		},
		Host: gkeEndpoint, // such as: "https://34.81.126.228"
		AuthProvider: &clientcmdapi.AuthProviderConfig{
			Name: "google",
		},
	}
	clientset, err := kubernetes.NewForConfig(config)
	return config, clientset, err

}

func CreateObject(ctx context.Context, kubeClientset kubernetes.Interface, restConfig rest.Config, obj runtime.Object) (runtime.Object, error) {
	l := log.FromContext(ctx)
	// Create a REST mapper that tracks information about the available resources in the cluster.
	groupResources, err := restmapper.GetAPIGroupResources(kubeClientset.Discovery())
	if err != nil {
		return obj, err
	}
	rm := restmapper.NewDiscoveryRESTMapper(groupResources)

	// Get some metadata needed to make the REST request.
	gvk := obj.GetObjectKind().GroupVersionKind()
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
	mapping, err := rm.RESTMapping(gk, gvk.Version)
	if err != nil {
		return obj, err
	}

	name, err := meta.NewAccessor().Name(obj)
	if err != nil {
		return obj, err
	}
	l.Info("the name of object", "name", name)

	// Create a client specifically for creating the object.
	restClient, err := newRestClient(restConfig, mapping.GroupVersionKind.GroupVersion())
	if err != nil {
		return obj, err
	}

	// Use the REST helper to create the object in the "default" namespace.
	restHelper := resource.NewHelper(restClient, mapping)
	return restHelper.Create("default", false, obj)
}

func newRestClient(restConfig rest.Config, gv schema.GroupVersion) (rest.Interface, error) {
	restConfig.ContentConfig = resource.UnstructuredPlusDefaultContentConfig()
	restConfig.GroupVersion = &gv
	if len(gv.Group) == 0 {
		restConfig.APIPath = "/api"
	} else {
		restConfig.APIPath = "/apis"
	}

	return rest.RESTClientFor(&restConfig)
}
